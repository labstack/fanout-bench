#!/usr/bin/env bash
set -euo pipefail

if [[ $# -eq 0 ]]; then
    echo "Usage: scripts/hetzner.sh <fanout-bench run flags…> [-- <fanout-bench run flags…>]…" >&2
    echo "Each nonempty group runs in order on one VM; SSH port and identity must agree across groups." >&2
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
server_name="fanout-bench-$(date -u +%Y%m%dT%H%M%SZ)-$nonce"
# Invoked indirectly by the EXIT trap.
# shellcheck disable=SC2329
cleanup() {
    local status=$? attempt deleted=false
    trap - EXIT
    trap '' INT TERM
    for attempt in 1 2 3; do
        if hcloud server delete "$server_name" >/dev/null; then
            deleted=true
            break
        fi
        if [[ $attempt -lt 3 ]]; then
            sleep 2
        fi
    done
    if [[ $deleted == false ]]; then
        echo "Failed to delete Hetzner server $server_name after three attempts; delete it manually." >&2
        if [[ $status -eq 0 ]]; then
            status=1
        fi
    fi
    rm -rf -- "$run_dir"
    exit "$status"
}
# Arm cleanup before creation so an interrupted or failed create is covered too.
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
hcloud server create --name "$server_name" --type "$server_type" \
    --location "$location" --image "$image" --label purpose=fanout-bench \
    "${key_args[@]}" >/dev/null
ip=$(hcloud server ip "$server_name")
if [[ -z "$ip" ]]; then
    echo "Hetzner server $server_name has no IPv4 address" >&2
    exit 1
fi

ssh_args=(-o BatchMode=yes -o StrictHostKeyChecking=accept-new
    -o "UserKnownHostsFile=$known_hosts" -o ConnectTimeout=5
    -o ConnectionAttempts=1 -o ServerAliveInterval=5 -o ServerAliveCountMax=1
    -p "$ssh_port")
if [[ -n "$ssh_identity" ]]; then
    ssh_args+=(-i "$ssh_identity")
fi
deadline=$((SECONDS + 300))
until ssh "${ssh_args[@]}" "root@$ip" true 2>/dev/null; do
    if (( SECONDS >= deadline )); then
        echo "Timed out waiting for SSH on Hetzner server $server_name" >&2
        exit 1
    fi
    sleep 2
done

statuses=()
status=0
offset=1
for group_length in "${group_lengths[@]}"; do
    if bin/fanout-bench run "${@:offset:group_length}" \
        --host "root@$ip" --ssh-known-hosts "$known_hosts" \
        --host-description "Hetzner Cloud $server_type, $location, $image"; then
        statuses+=(0)
    else
        statuses+=("$?")
        status=1
    fi
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
