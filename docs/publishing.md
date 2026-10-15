# Publishing checklist

A performance number is useful only when another engineer can explain and reproduce it.

## Required evidence

- [ ] exact Fanout release, commit when applicable, and verified binary/release artifact checksum;
- [ ] exact fanout-bench commit and clean/dirty state;
- [ ] exact telemetrygen version;
- [ ] Hetzner host description, CPU model, architecture, logical CPU count, memory, OS, and generator placement;
- [ ] transport, authentication path, batch size, topology, rate, duration, and repetitions;
- [ ] raw `report.json`, `report.md`, and `samples.jsonl` from the same run;
- [ ] three publish-preset repetitions on a fresh default `ccx33` VM;
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

> On a Hetzner Cloud ccx33 (8 dedicated vCPUs, 32 GB RAM) with generators sharing the host, Fanout accepted X mixed OpenTelemetry items/s for five minutes. The composition was A spans, B log records, and C metric points; dashboard read p95 was Y ms. This is an observed same-host result, not an isolated backend ceiling.

Avoid:

> Fanout does X events/s.

The second sentence discards the machine, duration, signal mix, generator placement, read behavior, and definition of “event.”

## Publication runs

Every publication run uses one invocation of `scripts/hetzner.sh` with the default Hetzner Cloud `ccx33` (8 dedicated vCPUs, 32 GB RAM), `fsn1`, and `ubuntu-24.04`. Use the publish preset with three repetitions in each group. Separate the ingest-only and read-under-write groups with a standalone `--` so both share one fresh VM while each gets a fresh remote directory and Fanout data directory:

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

The script uses the active `hcloud` context or `HCLOUD_TOKEN`, records the VM type, location, and image alongside the detected hardware, rebuilds the harness before provisioning, and waits for SSH once. Groups must be nonempty and use matching SSH port and identity settings; they share a temporary known-hosts file and host description. Every group runs even if an earlier one fails, downloads its own evidence, and receives a one-line summary at the end; any failed group makes the invocation exit nonzero. The script attempts VM deletion after the last group and on early exits, including Ctrl-C. Failed deletion after three attempts prints the unique server name for manual cleanup and exits nonzero. Hetzner bills every started hour, so both groups share the same VM billing time.

The harness verifies `fanout_v2026.10.1-alpha_linux_amd64.tar.gz` against `SHA256SUMS` on the ccx33. Managed scrapes use private `/metrics` bearer authentication. Read load covers System overview dashboard refresh (batched panels followed by annotations), service-map exemplar drill-down, and trace detail over 15-minute through 24-hour windows. Publish the per-operation methodology, counts and latency, and dashboard spec hash alongside the requested and completed operation rates.
