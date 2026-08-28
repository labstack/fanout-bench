# Publishing checklist

A performance number is useful only when another engineer can explain and reproduce it.

## Required evidence

- [ ] exact Fanout release, commit when applicable, and verified binary/release artifact checksum;
- [ ] exact fanout-bench commit and clean/dirty state;
- [ ] exact telemetrygen version;
- [ ] CPU model, architecture, logical CPU count, memory, OS, and generator placement;
- [ ] transport, authentication path, batch size, topology, rate, duration, and repetitions;
- [ ] raw `report.json`, `report.md`, and `samples.jsonl` from the same run;
- [ ] at least three publish-preset repetitions;
- [ ] signal-specific peak results and a sustained mixed result;
- [ ] a separate read-under-write result when claiming interactive performance;
- [ ] requested, completed, failed, and shed query rates beside read latency;
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

> On an Apple M3 Max with generators sharing the host, Fanout accepted X mixed OpenTelemetry items/s for five minutes. The composition was A spans, B log records, and C metric points; dashboard read p95 was Y ms. This is an observed same-host result, not an isolated backend ceiling.

Avoid:

> Fanout does X events/s.

The second sentence discards the machine, duration, signal mix, generator placement, read behavior, and definition of “event.”
