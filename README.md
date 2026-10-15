# Fanout Bench

Reproducible OpenTelemetry performance evidence for [Fanout](https://github.com/labstack/fanout).

Fanout Bench drives the official OpenTelemetry `telemetrygen` client, measures what Fanout actually accepts, samples the server throughout the run, and emits publication-ready JSON and Markdown. It is deliberately harder to fool than a one-line requests-per-second script.

## What the headline means

The headline is:

```text
Fanout accepted rows ÷ the interval between the counter scrapes bracketing the phase
```

Accepted rows come from `fanout_ingest_rows_total{signal=…}` at the Fanout server. They do **not** come from the offered rate, telemetrygen's intended count, or a client-side queue. The three units are named precisely:

- traces are reported as accepted **spans**;
- logs are reported as accepted **log records**;
- metrics are reported as accepted **metric data points**.

With pinned telemetrygen v0.157, each generated trace exports one parent and one child span. The generator's limiter charges once for each span, so its configured rate is already aggregate spans/s; Fanout Bench does not apply a hidden multiplier.

## Quick start

Prerequisites: [hcloud](https://github.com/hetznercloud/cli) with an active context (or `HCLOUD_TOKEN`), a registered Hetzner SSH key available to your SSH client, Go 1.27+, `ssh-keygen`, [just](https://github.com/casey/just), and SSH/SCP.

```sh
export FANOUT_BENCH_HCLOUD_SSH_KEY='benchmark-key'
just hetzner --fanout-version v2026.10.1-alpha --preset quick
```

The script runs `just build` before provisioning, creates a private network and two fresh Hetzner Cloud VMs with unique names, waits for SSH on both, runs the harness for each group, and downloads the evidence. Cleanup attempts to delete both servers and then the network on every exit, including failures and Ctrl-C. After three failed attempts it names each resource requiring manual deletion and exits nonzero. Hetzner bills every started hour for each VM, so grouping runs shares the billing time of both machines.

The defaults are a `ccx33` for Fanout (8 dedicated vCPUs, 32 GB RAM) and a separate `ccx33` for generators (8 dedicated vCPUs, 32 GB RAM), both in `fsn1` on `ubuntu-24.04`. OTLP travels over a private network (`10.77.0.0/16`, cloud subnet `10.77.0.0/24`) in that location’s network zone. Override the VM types with `FANOUT_BENCH_HCLOUD_TYPE` and `FANOUT_BENCH_HCLOUD_GENERATOR_TYPE`, and location/image with `FANOUT_BENCH_HCLOUD_LOCATION` and `FANOUT_BENCH_HCLOUD_IMAGE`. The hourly VM cost is **twice the ccx33 hourly rate**, plus applicable public IPv4 charges and tax; budget for both machines for every started hour. Check the selected location’s current rates before provisioning. `FANOUT_BENCH_HCLOUD_SSH_KEY` also accepts comma-separated key names. Each invocation shares one temporary known-hosts file across its groups, leaving your SSH known-hosts file untouched. Pass `--ssh-identity ~/.ssh/benchmark` if the registered key is not available through your SSH agent.

Separate complete sets of `fanout-bench run` flags with a standalone `--`. Groups must be nonempty and use matching SSH port and identity settings (defaults: port 22 and the SSH client's default identity). All groups run in order even if one fails, with a one-line summary per group at the end and a nonzero exit if any failed. They share both VMs, the private network, and both host descriptions; each harness run creates fresh stages and a fresh Fanout data directory. Both VMs and the network are deleted after the last group.

A plain `--host` run on any clean SSH-accessible Linux host runs Fanout and telemetrygen together on that host:

```sh
just build
./bin/fanout-bench run \
  --host 'root@<host>' \
  --host-description 'Linux benchmark host' \
  --fanout-version v2026.10.1-alpha \
  --preset quick
```

Replace `<host>` with the address of your host. The remote harness:

1. detects the host's Linux architecture over SSH;
2. downloads the matching Fanout release and verifies it against the release `SHA256SUMS`;
3. cross-builds pinned telemetrygen and the benchmark agent for the host;
4. uploads all three binaries to a private, isolated `/tmp/fanout-bench-*` directory;
5. starts a fresh Fanout data directory on one loopback-only random port;
6. creates one-time admin, ingest, and query credentials without printing them;
7. runs every phase and then `fanout repair verify`;
8. downloads `report.json`, `report.md`, and `samples.jsonl` locally;
9. stops Fanout and removes the remote run directory.

For an existing host, pass `--ssh-identity ~/.ssh/benchmark`, `--ssh-port 2222`, or `--ssh-known-hosts <file>` when needed. `--host-description <text>` records the provider or machine configuration beside the detected CPU model. Reports redact the SSH address and omit the run-scoped known-hosts path from reproduction commands. `--keep-remote` preserves the Fanout directory on an existing host for investigation; split runs still remove the generator stage and ephemeral SSH key. The Hetzner script always deletes both VMs and the network.

To use two existing clean Linux hosts, add all three split flags together:

```sh
./bin/fanout-bench run \
  --host 'root@<fanout-public>' \
  --generator-host 'root@<generator-public>' \
  --generator-address 10.77.0.3 \
  --fanout-listen 10.77.0.2 \
  --host-description 'Fanout host' \
  --generator-host-description 'Generator host' \
  --fanout-version v2026.10.1-alpha --preset quick
```

The public SSH targets are used by your local machine for provisioning. The two IP flags must be reachable between the hosts, with SSH from Fanout to the generator and OTLP from the generator to Fanout. Provisioning detects both Linux architectures, builds the harness for Fanout and telemetrygen for the generator, and uploads each binary only where it runs. A local temporary ed25519 key grants the Fanout host access to the generator for that run; its public key has a unique authorized-keys marker, and the private key is uploaded with mode `0600` into the Fanout stage. Both stages use mode `0700`.

`managed` remains the sole orchestrator on the Fanout host: it binds Fanout to the supplied private IP on a free port, performs setup, samples metrics, issues read load, verifies storage, and writes the report. Every telemetrygen process runs through SSH on the generator host, bounded by a remote timeout. Cleanup removes the marked authorized-key line, the private key, and both stages. Reports record both host descriptions and `generator_on_target_host: false`; reproduction commands redact both public targets and omit the private IP flags.

Nothing is installed system-wide. Existing Fanout services and data directories are not touched. Neither remote host needs a Go toolchain or Docker daemon, and Fanout needs no preconfigured account. The Fanout host needs an SSH client; the generator host needs an SSH server and GNU `timeout` (included in the default Ubuntu image).

> `telemetrygen` currently accepts OTLP headers through a command-line flag and prints those headers in its startup configuration. Fanout Bench suppresses and redacts child output, but the short-lived ingest token can still be visible to local process-listing tools while a generator is running. Use a dedicated token and revoke it after the run.

## Suites

| Preset | Sweep | Per step | Sustained proof | Intended use |
|---|---|---:|---:|---|
| `quick` | 1 topology per signal and mixed | 5s | 10s | installation smoke test |
| `standard` | 1, 2, 4, 8 processes per signal | 15s | 60s | engineering comparison |
| `publish` | 1, 2, 4, 8, 16, 32, 64 processes per signal | 30s | 5m | repeatable public evidence |

The default topology is 8 workers per process at 2,500 items/s per worker with 1,000-item OTLP batches. A mixed step launches the same topology independently for traces, logs, and metrics. Without read load, the sustained proof reruns the mixed topology with the highest **accepted** throughput. With `--query-rate`, it chooses the highest-throughput topology that also completed at least 95% of the requested reads with no errors or scheduler shedding. If no sweep point qualifies, it repeats the best read-completion point and fails the sustained-read quality gate rather than publishing a write-only peak as mixed capacity. Both reports record the selected sustained process count per signal. The wider publish sweep adds independent exporters to get past telemetrygen's per-process limits; see [the source analysis](docs/telemetrygen.md).

For publication runs, dedicate the default ccx33 to Fanout and use the separate ccx33 for generators, with three repetitions. Run ingest-only and read-under-write as two groups sharing the same fresh pair of VMs; see [the publication commands](docs/publishing.md#publication-runs). For example:

```sh
scripts/hetzner.sh \
  --fanout-version v2026.10.1-alpha \
  --preset publish --repeat 3 \
  --output results/publish-ingest-2026-10-14 \
  -- \
  --fanout-version v2026.10.1-alpha \
  --preset publish --repeat 3 --query-rate 20 \
  --output results/publish-read-write-2026-10-14
```

Publication runs measure Fanout’s ingest ceiling with all eight ccx33 vCPUs available to Fanout; telemetrygen runs on the separate ccx33 over the private network. A plain `--host` run without the three split flags remains a same-host run, and its report warns about generator contention.

## Read and write together

The write-only suite answers “how quickly can Fanout ingest?” It does not answer “what happens to dashboard latency while ingesting?” Add automatically authenticated read load to mixed phases:

```sh
scripts/hetzner.sh \
  --fanout-version v2026.10.1-alpha \
  --preset standard \
  --query-rate 50
```

The harness uses a bounded open-loop scheduler targeting user operations per second at 15-minute, 1-hour, 6-hour, 12-hour, and 24-hour windows. Operations rotate through dashboard refresh, service-map trace drill-down, and trace detail. A refresh sends the full System overview panel batch exactly as the UI's initial all-visible load does, then requests annotations over the returned time-panel bounds. Layout widths model a fixed 1664-pixel viewport and are rounded as in the UI. Drill-down uses `POST /api/panels/exemplars` with the selected service and the panel response's absolute window. Trace detail uses `GET /api/traces/:id` with a discovered ID, namespace, and that same window, retained separately for each range; an empty detail is a failure. Until a trace is discovered, detail slots perform drill-down; until a selectable panel is cached, those slots perform dashboard refresh instead. Each slot performs one user operation. The current System overview has no log drill or standalone log/trace search operation. Each scheduled read measures every request in its operation, and any request or panel error fails the operation. The report records per-operation counts and latency in its methodology alongside the dashboard definition hash; aggregate fields and quality gates retain their schema.

It reports attempted and completed rates, phase-end cancellations, explicit scheduler shedding, failures, mean, p50, p95, p99, and maximum latency. The concurrency bound prevents a stalled server from creating unbounded requests.

## Output

Every run produces one self-contained directory:

```text
results/20261014T190000Z/
├── report.json     authoritative schema-versioned result
├── report.md       readable, publication-ready report
├── samples.jsonl   per-second Fanout resource samples
├── fanout-repair.txt  post-run authoritative-storage verification
└── fanout.log      setup-token-scrubbed server log
```

The terminal output is optimized for decisions:

```text
╭────────────────────────────────────────────────────────────────────╮
│  FANOUT BENCH  •  accepted-row performance, with evidence          │
╰────────────────────────────────────────────────────────────────────╯
  Target       10.77.0.2:39281  (plaintext gRPC)
  Generator    telemetrygen v0.157.0  same-host=false
  Suite        standard  repeat=1  18 phases  ~5.2 min load time
  Evidence     Fanout counters @ /metrics  samples every 1.0s

  ◇ mixed-p4-r1        offered 240,000 items/s for 15s
     … accepted …/s  ratio …%  CPU … cores  peak RSS …

╭─ RESULTS ───────────────────────────────────────────────────────────╮
  Workload         Accepted/s  Offered/s  Avg CPU     Peak RSS
  Traces              …            …       …           …
  Logs                …            …       …           …
  Metric points       …            …       …           …
  Mixed peak          …            …       …           …
  Mixed sustained     …            …       …           …
├─ QUALITY GATES ─────────────────────────────────────────────────────┤
  ✓ generator-processes      all exited successfully
  ✓ server-dropped-rows      zero
  ✓ server-restarts          none
  ✓ authoritative-counters   Fanout accepted rows in every phase
  ✓ sustained-query-load     …/… completed/s, 0 errors, 0 shed
  ✓ storage-repair-verify    authoritative Parquet storage verified
╰─ PASS — report is internally valid ────────────────────────────────╯
```

“Valid” means the evidence is internally trustworthy. It does not mean every offered target was sustained. Each phase is separately classified as `target-sustained`, `generator-limited`, `saturated`, `near-saturation`, `saturated-or-generator-limited`, or `invalid`. Split runs record average generator-host CPU cores and logical CPUs for every trial, from `/proc/stat` reads before and after the phase. After validity and `target-sustained` checks, generator CPU at or above 90% means `generator-limited`; below 90%, an accepted-throughput gain under 10% over the preceding process count for the same signal and repetition means `saturated`. Same-host runs retain the acceptance-ratio fallback. See [metric definitions](docs/metrics.md) for the full rule.

Regenerate Markdown without rerunning load:

```sh
./bin/fanout-bench render \
  --input results/publish-2026-10-14/report.json \
  --output results/publish-2026-10-14/report.md
```

## Metrics captured

For every phase the report includes:

- offered and accepted items/s, exact accepted counts, and acceptance ratio;
- accepted spans, log records, and metric points separately;
- generator process count/failures and Fanout dropped-row/restart checks;
- average Fanout CPU cores, peak/end RSS, peak/end Go heap, allocations, GC pause;
- peak ingest queue depth, Parquet file count/bytes, and optional local data-directory growth;
- a raw time series in `samples.jsonl` for independent charting;
- attempted/completed read rate, latency, failures, scheduler shedding, and phase-end cancellations when query load is enabled.

See [docs/metrics.md](docs/metrics.md) for definitions and [docs/publishing.md](docs/publishing.md) for the public-results checklist.

## Controlled overrides

Presets make comparisons easy. Overrides are intended for diagnostics and ceiling exploration:

```sh
./bin/fanout-bench run \
  --preset standard \
  --processes 16 \
  --workers 8 \
  --rate 2500 \
  --duration 30s
```

`--rate` is per telemetrygen worker. For mixed load, total offered items/s is:

```text
3 signals × processes-per-signal × workers-per-process × rate-per-worker
```

Avoid unbounded telemetrygen runs. Its SDK queues may drop locally while the console continues to count generated data. Fanout Bench always uses bounded rates and treats Fanout's accepted counter as authoritative.

## Development

```sh
just test
```

The project uses only the Go standard library at runtime. The generator is a separately pinned official OpenTelemetry binary, making its version visible and replaceable without pulling the Collector dependency graph into the harness.

## Existing local Fanout process

Fresh Hetzner Cloud VMs are the publication path. For local development against an already-running Fanout, install telemetrygen, provide short-lived credentials through environment variables, and omit `--host`:

```sh
just telemetrygen
export FANOUT_BENCH_INGEST_TOKEN='short-lived-benchmark-token'
export FANOUT_BENCH_METRICS_TOKEN='matching-Fanout-metrics-token'

./bin/fanout-bench doctor --telemetrygen ./bin/telemetrygen
./bin/fanout-bench run \
  --preset quick \
  --telemetrygen ./bin/telemetrygen \
  --endpoint localhost:7520 \
  --metrics-url http://localhost:7520/metrics \
  --insecure \
  --same-host
```

Managed runs set `FANOUT_ADDR` to one random port shared by OTLP/gRPC, OTLP/HTTP, the API, and `/metrics`. Split runs bind the private listen IP; same-host runs bind loopback. Metrics stay private: a per-run `FANOUT_METRICS_TOKEN` is sent as `Authorization: Bearer …` and never written to artifacts. MCP, alerts, and chat stay disabled.

Private local metrics endpoints use `FANOUT_BENCH_METRICS_TOKEN`. Local authenticated read load uses `FANOUT_BENCH_QUERY_COOKIE`. Secrets have no value flags and are never persisted. telemetrygen does expose its OTLP header in its process arguments while running, so use only a dedicated short-lived token and revoke it afterward. Managed remote runs create and destroy this token automatically.
