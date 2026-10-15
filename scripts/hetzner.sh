#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo "Usage: scripts/hetzner.sh <fanout-bench run flags…> [-- <fanout-bench run flags…>]…" >&2
    echo "Each nonempty group shares the Fanout and generator VMs; SSH port and identity must agree across groups." >&2
    exit 2
fi

# Validate every group before building or provisioning. Keep the original "$@"
# intact so slicing it below preserves argument boundaries and quoting.
group_lengths=()
group_args=()
group_length=0
group_count=0
ssh_port=22
ssh_identity=
validate_group() {
    local group_ssh_port=22 group_ssh_identity=
    if [[ $group_length -eq 0 ]]; then
        echo "Run groups must not be empty" >&2
        exit 2
    fi
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --ssh-identity|--ssh-port|-ssh-identity|-ssh-port)
                if [[ $# -lt 2 ]]; then
                    echo "Missing value for $1" >&2
                    exit 2
                fi
                if [[ $1 == *ssh-identity ]]; then
                    group_ssh_identity=$2
                else
                    group_ssh_port=$2
                fi
                shift
                ;;
            --ssh-identity=*|-ssh-identity=*) group_ssh_identity=${1#*=} ;;
            --ssh-port=*|-ssh-port=*) group_ssh_port=${1#*=} ;;
        esac
        shift
    done
    if [[ $group_count -eq 0 ]]; then
        ssh_port=$group_ssh_port
        ssh_identity=$group_ssh_identity
    elif [[ $group_ssh_port != "$ssh_port" || $group_ssh_identity != "$ssh_identity" ]]; then
        echo "SSH port and identity must agree across all run groups" >&2
        exit 2
    fi
    group_lengths+=("$group_length")
    group_count=$((group_count + 1))
}
for arg in "$@"; do
    if [[ $arg == -- ]]; then
        # The explicit check also avoids expanding an empty array under Bash 3.
        if [[ $group_length -eq 0 ]]; then
            echo "Run groups must not be empty" >&2
            exit 2
        fi
        validate_group "${group_args[@]}"
        group_args=()
        group_length=0
    else
        group_args+=("$arg")
        group_length=$((group_length + 1))
    fi
done
if [[ $group_length -eq 0 ]]; then
    echo "Run groups must not be empty" >&2
    exit 2
fi
validate_group "${group_args[@]}"

: "${FANOUT_BENCH_HCLOUD_SSH_KEY:?Set FANOUT_BENCH_HCLOUD_SSH_KEY to a registered Hetzner SSH key name (or comma-separated names)}"
server_type=${FANOUT_BENCH_HCLOUD_TYPE:-ccx33}
generator_type=${FANOUT_BENCH_HCLOUD_GENERATOR_TYPE:-ccx33}
location=${FANOUT_BENCH_HCLOUD_LOCATION:-fsn1}
image=${FANOUT_BENCH_HCLOUD_IMAGE:-ubuntu-24.04}

for command in hcloud ssh go just od; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "Required command not found: $command" >&2
        exit 1
    fi
done

repo_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
just build

key_args=()
IFS=',' read -r -a keys <<< "$FANOUT_BENCH_HCLOUD_SSH_KEY"
for key in "${keys[@]}"; do
    if [[ -z "$key" ]]; then
        echo "Hetzner SSH key names must not be empty" >&2
        exit 2
    fi
    key_args+=(--ssh-key "$key")
done

run_dir=$(mktemp -d "${TMPDIR:-/tmp}/fanout-bench-hetzner.XXXXXX")
known_hosts="$run_dir/known_hosts"
touch "$known_hosts"
nonce=$(od -An -N4 -tx1 /dev/urandom | tr -d ' \n')
name="fanout-bench-$(date -u +%Y%m%dT%H%M%SZ)-$nonce"
fanout_name="$name-fanout"
generator_name="$name-generator"
network_attempted=false
fanout_attempted=false
generator_attempted=false
# Deletion can race an interrupted create. Confirm absence rather than treating
# a missing resource as a cleanup failure; retry API and transient state errors.
# Invoked indirectly by the EXIT trap's cleanup function.
# shellcheck disable=SC2317,SC2329
delete_resource() {
    local kind=$1 resource=$2 attempt names check absent
    for attempt in 1 2 3; do
        if hcloud "$kind" delete "$resource" >/dev/null; then
            return 0
        fi
        if names=$(hcloud "$kind" list --selector purpose=fanout-bench -o noheader -o columns=name) &&
            [[ $'\n'$names$'\n' != *$'\n'$resource$'\n'* ]]; then
            absent=true
            for ((check = 0; check < 5; check++)); do
                sleep 2
                if ! names=$(hcloud "$kind" list --selector purpose=fanout-bench -o noheader -o columns=name) ||
                    [[ $'\n'$names$'\n' == *$'\n'$resource$'\n'* ]]; then
                    absent=false
                    break
                fi
            done
            if [[ $absent == true ]]; then
                return 0
            fi
        fi
        if [[ $attempt -lt 3 ]]; then
            sleep 2
        fi
    done
    echo "Failed to delete Hetzner $kind $resource after three attempts; delete it manually." >&2
    return 1
}
# Invoked indirectly by the EXIT trap.
# shellcheck disable=SC2317,SC2329
cleanup() {
    local status=$?
    trap - EXIT
    trap '' INT TERM
    if [[ $fanout_attempted == true ]]; then
        delete_resource server "$fanout_name" || status=1
    fi
    if [[ $generator_attempted == true ]]; then
        delete_resource server "$generator_name" || status=1
    fi
    if [[ $network_attempted == true ]]; then
        delete_resource network "$name" || status=1
    fi
    rm -rf -- "$run_dir"
    exit "$status"
}
# Arm cleanup before creation so an interrupted or failed create is covered too.
benchmark_pid=
create_in_flight=false
interrupted_status=
# Invoked indirectly by the signal traps.
# shellcheck disable=SC2317,SC2329
interrupt() {
    trap '' INT TERM
    if [[ $create_in_flight == true ]]; then
        interrupted_status=$2
        return
    fi
    if [[ -n $benchmark_pid ]]; then
        kill -s "$1" "$benchmark_pid" 2>/dev/null || true
        wait "$benchmark_pid" || true
    fi
    exit "$2"
}
trap cleanup EXIT
trap 'interrupt INT 130' INT
trap 'interrupt TERM 143' TERM
# Keep long-running creates in children too, so traps run promptly while waiting.
run_child() {
    "$@" <&0 &
    benchmark_pid=$!
    local status=0
    wait "$benchmark_pid" || status=$?
    create_in_flight=false
    if [[ -n $interrupted_status ]]; then
        # The signal interrupted wait, not the create. Give the create two
        # minutes to finish, then kill it; cleanup deletes resources by name.
        local deadline=$((SECONDS + 120))
        while kill -0 "$benchmark_pid" 2>/dev/null && (( SECONDS < deadline )); do
            sleep 1
        done
        kill -s KILL "$benchmark_pid" 2>/dev/null || true
        wait "$benchmark_pid" 2>/dev/null || true
        exit "$interrupted_status"
    fi
    benchmark_pid=
    return "$status"
}
run_create() {
    create_in_flight=true
    run_child "$@"
}
zone=$(hcloud location describe "$location" -o 'format={{.NetworkZone}}')
if [[ -z "$zone" ]]; then
    echo "Hetzner location $location has no network zone" >&2
    exit 1
fi
network_attempted=true
run_create hcloud network create --name "$name" --ip-range 10.77.0.0/16 \
    --label purpose=fanout-bench >/dev/null
run_create hcloud network add-subnet "$name" --type cloud --network-zone "$zone" \
    --ip-range 10.77.0.0/24 >/dev/null
fanout_attempted=true
run_create hcloud server create --name "$fanout_name" --type "$server_type" \
    --location "$location" --image "$image" --label purpose=fanout-bench \
    --network "$name" "${key_args[@]}" >/dev/null
generator_attempted=true
run_create hcloud server create --name "$generator_name" --type "$generator_type" \
    --location "$location" --image "$image" --label purpose=fanout-bench \
    --network "$name" "${key_args[@]}" >/dev/null
fanout_ip=$(hcloud server ip "$fanout_name")
generator_ip=$(hcloud server ip "$generator_name")
fanout_private=$(hcloud server describe "$fanout_name" -o 'format={{(index .PrivateNet 0).IP}}')
generator_private=$(hcloud server describe "$generator_name" -o 'format={{(index .PrivateNet 0).IP}}')
if [[ -z "$fanout_ip" || -z "$generator_ip" || -z "$fanout_private" || -z "$generator_private" ]]; then
    echo "Hetzner servers must have public and private IPv4 addresses" >&2
    exit 1
fi

ssh_args=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new
    -o "UserKnownHostsFile=$known_hosts" -o ConnectTimeout=5
    -o ConnectionAttempts=1 -o ServerAliveInterval=5 -o ServerAliveCountMax=1
    -p "$ssh_port")
if [[ -n "$ssh_identity" ]]; then
    ssh_args+=(-i "$ssh_identity")
fi
for ip in "$fanout_ip" "$generator_ip"; do
    deadline=$((SECONDS + 300))
    until run_child ssh "${ssh_args[@]}" "root@$ip" true 2>/dev/null; do
        if (( SECONDS >= deadline )); then
            echo "Timed out waiting for SSH on Hetzner server at $ip" >&2
            exit 1
        fi
        sleep 2
    done
done

statuses=()
status=0
offset=1
for group_length in "${group_lengths[@]}"; do
    bin/fanout-bench run "${@:offset:group_length}" \
        --host "root@$fanout_ip" --ssh-known-hosts "$known_hosts" \
        --generator-host "root@$generator_ip" --generator-address "$generator_private" \
        --fanout-listen "$fanout_private" \
        --host-description "Hetzner Cloud $server_type, $location, $image" \
        --generator-host-description "Hetzner Cloud $generator_type, $location, $image" <&0 &
    benchmark_pid=$!
    if wait "$benchmark_pid"; then
        statuses+=(0)
    else
        statuses+=("$?")
        status=1
    fi
    benchmark_pid=
    offset=$((offset + group_length + 1))
done

for index in "${!statuses[@]}"; do
    if [[ ${statuses[index]} -eq 0 ]]; then
        printf 'Group %d/%d: PASS\n' "$((index + 1))" "${#statuses[@]}"
    else
        printf 'Group %d/%d: FAIL (exit %d)\n' "$((index + 1))" "${#statuses[@]}" "${statuses[index]}"
    fi
done
exit "$status"
