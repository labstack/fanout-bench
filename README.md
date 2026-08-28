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

Prerequisites: Go 1.27+, [just](https://github.com/casey/just), SSH, and a clean SSH-accessible Linux host. The host may be a cloud instance, dedicated machine, or bare metal.

```sh
just build

./bin/fanout-bench run \
  --host root@203.0.113.10 \
  --fanout-version v2026.8.11 \
  --preset standard \
  --query-rate 50
```

That one command:

1. detects the host's Linux architecture over SSH;
2. downloads the matching Fanout release and verifies it against the release `SHA256SUMS`;
3. cross-builds pinned telemetrygen and the benchmark agent for the host;
4. uploads all three binaries to a private, isolated `/tmp/fanout-bench-*` directory;
5. starts a fresh Fanout data directory on loopback-only random ports;
6. creates one-time admin, ingest, and query credentials without printing them;
7. runs every phase and then `fanout repair verify`;
8. downloads `report.json`, `report.md`, and `samples.jsonl` locally;
9. stops Fanout and removes the remote run directory.

Pass `--ssh-identity ~/.ssh/bench` or `--ssh-port 2222` when needed. `--keep-remote` preserves the isolated remote directory for investigation; teardown is the default.

Nothing is installed system-wide. Existing Fanout services and data directories are not touched. The remote host needs no Go toolchain, Docker daemon, or preconfigured Fanout account.

> `telemetrygen` currently accepts OTLP headers through a command-line flag and prints those headers in its startup configuration. Fanout Bench suppresses and redacts child output, but the short-lived ingest token can still be visible to local process-listing tools while a generator is running. Use a dedicated token and revoke it after the run.

## Suites

| Preset | Sweep | Per step | Sustained proof | Intended use |
|---|---|---:|---:|---|
| `quick` | 1 topology per signal and mixed | 5s | 10s | installation smoke test |
| `standard` | 1, 2, 4, 8 processes per signal | 15s | 60s | engineering comparison |
| `publish` | 1, 2, 4, 8, 16 processes per signal | 30s | 5m | repeatable public evidence |

The default topology is 8 workers per process at 2,500 items/s per worker with 1,000-item OTLP batches. A mixed step launches the same topology independently for traces, logs, and metrics. Without read load, the sustained proof reruns the mixed topology with the highest **accepted** throughput. With `--query-rate`, it chooses the highest-throughput topology that also completed at least 95% of the requested reads with no errors or scheduler shedding. If no sweep point qualifies, it repeats the best read-completion point and fails the sustained-read quality gate rather than publishing a write-only peak as mixed capacity.

For a publication run, use at least three repetitions:

```sh
./bin/fanout-bench run \
  --host root@bench-host \
  --fanout-version v2026.8.11 \
  --preset publish \
  --repeat 3 \
  --query-rate 50 \
  --output results/publish-2026-08-28
```

Provisioned runs deliberately place Fanout and generators on the same remote host, measuring the complete single-node product under load. The report labels this an observed same-host ceiling and warns that generator contention can make it lower than Fanout's isolated backend ceiling.

## Read and write together

The write-only suite answers “how quickly can Fanout ingest?” It does not answer “what happens to dashboard latency while ingesting?” Add automatically authenticated read load to mixed phases:

```sh
./bin/fanout-bench run \
  --host root@bench-host \
  --fanout-version v2026.8.11 \
  --preset standard \
  --query-rate 50
```

The harness uses a bounded open-loop scheduler and rotates across overview, topology, performance, trace, and log queries with 15-minute through 24-hour windows. It reports attempted and completed rates, phase-end cancellations, explicit scheduler shedding, failures, mean, p50, p95, p99, and maximum latency. The concurrency bound prevents a stalled server from creating unbounded requests.

## Output

Every run produces one self-contained directory:

```text
results/20260828T190000Z/
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
  Target       localhost:4317  (plaintext gRPC)
  Generator    telemetrygen v0.157.0  same-host=true
  Suite        standard  repeat=1  18 phases  ~5.2 min load time
  Evidence     Fanout counters @ /-/metrics  samples every 1.0s

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

“Valid” means the evidence is internally trustworthy. It does not mean every offered target was sustained. Each phase is separately classified as `target-sustained`, `near-saturation`, `saturated-or-generator-limited`, or `invalid`.

Regenerate Markdown without rerunning load:

```sh
./bin/fanout-bench render \
  --input results/publish-2026-08-28/report.json \
  --output results/publish-2026-08-28/report.md
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

Remote provisioning is the publication path. For local development against an already-running Fanout, install telemetrygen, provide short-lived credentials through environment variables, and omit `--host`:

```sh
just telemetrygen
export FANOUT_BENCH_INGEST_TOKEN='short-lived-benchmark-token'

./bin/fanout-bench doctor --telemetrygen ./bin/telemetrygen
./bin/fanout-bench run \
  --preset quick \
  --telemetrygen ./bin/telemetrygen \
  --endpoint localhost:4317 \
  --metrics-url http://localhost:7520/-/metrics \
  --insecure \
  --same-host
```

Private local metrics endpoints use `FANOUT_BENCH_METRICS_TOKEN`. Local authenticated read load uses `FANOUT_BENCH_QUERY_COOKIE`. Secrets have no value flags and are never persisted. telemetrygen does expose its OTLP header in its process arguments while running, so use only a dedicated short-lived token and revoke it afterward. Managed remote runs create and destroy this token automatically.
