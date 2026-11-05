# Publishing checklist

A performance number is useful only when another engineer can explain and reproduce it.

## Required evidence

- [ ] exact Fanout release, commit when applicable, and verified binary/release artifact checksum;
- [ ] exact fanout-bench commit and clean/dirty state;
- [ ] exact telemetrygen version;
- [ ] Fanout and generator Hetzner host descriptions, Fanout CPU model, architecture, logical CPU count, memory, OS, and generator placement;
- [ ] transport, authentication path, batch size, topology, rate, duration, and repetitions;
- [ ] average generator-host CPU cores and logical CPUs for each split trial, plus the selected sustained process count per signal;
- [ ] raw `report.json`, `report.md`, and `samples.jsonl` from the same run, retaining the [background-work fields](metrics.md#background-work-samples) for rollups, write gates, compaction, publish waits, and ingest backpressure when available;
- [ ] three publish-preset repetitions with Fanout alone on a fresh default `ccx33` VM and generators on a separate `ccx33`;
- [ ] signal-specific peak results and a sustained mixed result;
- [ ] a separate read-under-write result when claiming interactive performance;
- [ ] requested, completed, failed, and shed operation rates beside read latency;
- [ ] all evidence-quality gates passing;
- [ ] limitations stated next to the headline, not hidden in a footnote.

## Recommended presentation

Lead with two numbers, not one:

1. the sustained mixed accepted throughput, because it is closest to an observability workload;
2. read p95/p99 under that write load, because ingestion that freezes the product is not a win.

Then publish the separate span, log-record, and metric-point sweeps. They explain workload sensitivity and make peer comparisons possible. Low-level writer microbenchmarks can explain engineering progress, but they must be labeled as component measurements and never presented as end-to-end OTLP throughput.

Use median values across repetitions for the headline and retain every raw repetition. Report variability; do not select only the fastest run.

## Language

Prefer:

> On a Hetzner Cloud ccx33 (8 dedicated vCPUs, 32 GB RAM) with telemetrygen on a separate ccx33 (8 dedicated vCPUs, 32 GB RAM) in the same location over a private network, Fanout accepted X mixed OpenTelemetry items/s for five minutes. The composition was A spans, B log records, and C metric points; dashboard read p95 was Y ms. The Fanout ccx33 ran Fanout and the benchmark orchestrator; all generator processes ran on the separate generator ccx33.

Avoid:

> Fanout does X events/s.

The second sentence discards the machine, duration, signal mix, generator placement, read behavior, and definition of “event.”

## Publication runs

The publish sweep uses 1, 2, 4, 8, 16, 32, and 64 processes per signal, each with 8 workers at 2,500 items/s and 1,000-item batches. Sweep steps last 30 seconds after a 15-second warmup; the selected mixed topology then runs for five minutes. Reports state the sustained process count per signal. Selection still uses highest accepted mixed throughput meeting the read target, with the best read-completion fallback if none qualifies. Generator CPU and sweep scaling determine [limit attribution](metrics.md#limit-classification).

With 8 generator vCPUs, the highest sweep points can be generator-limited, and the report classifies them as such.

Every publication run uses one invocation of `scripts/hetzner.sh` with Fanout alone on the default Hetzner Cloud `ccx33` (8 dedicated vCPUs, 32 GB RAM) and generators on a separate `ccx33` (8 dedicated vCPUs, 32 GB RAM), both in `fsn1` on `ubuntu-24.04`. The two VMs share a private network (`10.77.0.0/16`, cloud subnet `10.77.0.0/24`) in the location’s network zone. Public IPs serve provisioning; generators send OTLP to Fanout over private IPs. `FANOUT_BENCH_HCLOUD_TYPE` and `FANOUT_BENCH_HCLOUD_GENERATOR_TYPE` override the Fanout and generator types respectively; location, image, and SSH-key settings apply to both. Use the publish preset with three repetitions in each group. Separate the ingest-only and read-under-write groups with a standalone `--` so both groups share the fresh pair of VMs and network while each gets fresh stages and a Fanout data directory:

```sh
export FANOUT_BENCH_HCLOUD_SSH_KEY='benchmark-key'
scripts/hetzner.sh \
  --fanout-version v2026.10.1-alpha \
  --preset publish --repeat 3 \
  --output results/publish-ingest-2026-10-14 \
  -- \
  --fanout-version v2026.10.1-alpha \
  --preset publish --repeat 3 --query-rate 20 \
  --output results/publish-read-write-2026-10-14
```

The script uses the active `hcloud` context or `HCLOUD_TOKEN`, records both VM types, location, and image alongside the detected Fanout hardware, rebuilds the harness before provisioning, and waits for SSH on both VMs. Groups must be nonempty and use matching SSH port and identity settings; they share a temporary known-hosts file and both host descriptions. Every group runs even if an earlier one fails, downloads its own evidence, and receives a one-line summary at the end; any failed group makes the invocation exit nonzero. The script attempts to delete both servers and then the network after the last group and on early exits, including Ctrl-C. Failed deletion after three attempts prints each resource name for manual cleanup and exits nonzero. Hetzner bills every started hour on both VMs. The hourly VM cost is **twice the ccx33 hourly rate** for the chosen location, plus applicable public IPv4 charges and tax; a run crossing an hour boundary incurs another started hour on each VM. Confirm current rates before provisioning; grouping the runs shares the billing time of both machines.

The local provisioner detects both Linux platforms, builds the harness for Fanout and telemetrygen for the generator, and uploads only the binaries needed by each host. It generates an ephemeral ed25519 key, appends the public key to the generator’s root authorized keys with a unique marker, and puts the private key (`0600`) in the Fanout stage (`0700`). `managed` on Fanout owns setup, lifecycle, sampling, reads, storage verification, and reporting. Telemetrygen runs on the generator through private-network SSH with a bounded remote timeout; generator CPU reads reuse that SSH setup before and after each phase. Cleanup removes the marked key and both stages. Reports record the generator description in the host section and `generator_on_target_host: false`; reproduction commands redact both public targets and omit the private addresses.

A plain `fanout-bench run --host root@<host>` without a generator host remains a same-host run on any clean Linux host. It is useful for diagnostics, but generator CPU contention limits what it says about Fanout’s own ingest ceiling.

Before uploading to the ccx33, the local provisioner verifies `fanout_v2026.10.1-alpha_linux_amd64.tar.gz` against the release `SHA256SUMS`. Managed scrapes use private `/metrics` bearer authentication. Read load covers System overview dashboard refresh (batched panels followed by annotations), service-map exemplar drill-down, and trace detail over 15-minute through 24-hour windows. Publish the per-operation methodology, counts and latency, and dashboard spec hash alongside the requested and completed operation rates.

## Faster validation runs

Validation keeps the accepted-counter measurements, generator checks, read-completion gate, storage repair verification, warmup, and five-minute mixed sustained proof. It is intended for release/engineering validation; publication still requires the full `publish` sweeps and repetitions above.

Use three independent VM pairs to overlap ingest, reads near 70% load, and reads at saturation:

```sh
export FANOUT_BENCH_HCLOUD_SSH_KEY='benchmark-key'
scripts/hetzner.sh --parallel-groups \
  --fanout-version v2026.10.1-alpha \
  --preset validate --repeat 3 \
  --output results/validate-ingest \
  -- \
  --fanout-version v2026.10.1-alpha \
  --preset validate --repeat 1 --signals mixed --query-rate 20 --rate 210 \
  --output results/validate-reads-70 \
  -- \
  --fanout-version v2026.10.1-alpha \
  --preset validate --repeat 1 --signals mixed --query-rate 20 \
  --output results/validate-reads-saturated
```

Replace the release and output directories for each run. `--rate 210` is per worker: it approximates 70% for the previously measured target and must be recalibrated when capacity changes; it is not an automatic 70% controller. `--signals mixed` keeps the mixed warmup, mixed sweep and mixed sustained phase, skipping the duplicate traces/logs/metrics sweeps in both read groups. Reports label those single-signal peaks as skipped.

`validate` has the same maximum process counts (1 through 64), durations, workers, rates and batch sizes as `publish`. Within each signal and repetition it skips remaining sweep points after two consecutive accepted-throughput gains strictly below 5%, or an acceptance ratio below 50% and below half the preceding point. Comparisons require valid points; warmup, other signals and other repetitions cannot trigger a stop. The next repetition starts at the first point again. JSON `sweep_stops` and Markdown record the last measured phase, reason and exact skipped phases. Sustained topology selection still uses the best measured mixed point satisfying the read target, with the existing failing fallback when none qualifies. Early stopping can miss a later throughput recovery, so validation evidence does not replace a full ceiling sweep for publication. `publish` never stops early.

Budget **about two hours wall time**, down from about four, using the observed 1.5-hour ingest group plus roughly 30 minutes of setup, verification and download as the longest path. Early stopping may shorten that further; it is not a guaranteed duration. Mixed-only read groups should finish sooner and their pairs are deleted as soon as each group finishes. Six VMs run at peak concurrency; ensure account quota and local build/network capacity allow that.

For cost planning, if ingest occupies its pair for two started hours and each mixed-only read group stays within one started hour, the run bills **8 ccx33 VM-hours**: `2 × (2 + 1 + 1)`. That is about the VM cost of the previous four-hour run on one pair. If both read groups cross into a second hour, budget 12 VM-hours instead. Multiply by the selected location's current ccx33 hourly rate, then add applicable public IPv4 charges and tax. These are estimates based on the observed timings, not live price quotes; check current rates before provisioning.

`--parallel-groups` must be the first script argument. The parent builds once and launches each group on its own isolated pair and network, printing the group number and all resource names. Each child retains the sequential script's cleanup and deletion retries; the parent waits for every child, including cleanup, and returns nonzero for any run/provisioning/deletion failure. Ctrl-C or TERM signals all active children and waits for their cleanup, including interrupted creates. Failed deletion names resources requiring manual removal. Use distinct `--output` paths; omitted outputs receive unique per-group directories under `results/parallel-*`. Without `--parallel-groups`, groups continue sharing one pair sequentially.
