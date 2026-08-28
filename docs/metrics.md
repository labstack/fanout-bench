# Metric definitions

## Throughput

| Field | Definition |
|---|---|
| offered items/s | configured processes × workers × per-worker rate; multiplied by three for mixed load |
| accepted count | end minus start of Fanout's per-signal `fanout_ingest_rows_total` counter |
| accepted items/s | total accepted count ÷ measured process wall time |
| acceptance ratio | accepted count ÷ (offered items/s × requested generator duration) |

Wall time includes generator startup and final batch delivery. This makes throughput conservative and prevents final flush time from disappearing from the denominator.

telemetrygen exports a parent and child span per generated trace, but its limiter consumes one token for each span. Its `--rate` is therefore already aggregate spans/s; Fanout Bench does not apply another trace-shape multiplier.

## Server resources

| Field | Source and calculation |
|---|---|
| average CPU cores | delta of `process_cpu_seconds_total` ÷ wall time |
| peak/end RSS | sampled `process_resident_memory_bytes` |
| peak/end Go heap | sampled `go_memstats_heap_alloc_bytes` |
| allocations | delta of `go_memstats_alloc_bytes_total` |
| GC pause | delta of `go_gc_duration_seconds_sum` |
| queue peak | maximum sampled `fanout_ingest_queue_depth` |
| Parquet files/bytes | `fanout_parquet_files` and `fanout_parquet_size_bytes` |
| disk growth | recursive regular-file size delta when `--data-dir` is provided |

CPU is the Fanout process only. On a same-host run it does not include telemetrygen CPU, but telemetrygen still competes with Fanout for machine resources. RSS and heap are sampled during the phase; an end-only measurement would miss transient peaks.

## Validity

A report passes its evidence-quality gates when:

1. every telemetrygen process exits successfully;
2. Fanout reports no dropped rows;
3. the Fanout process start time does not change;
4. Fanout's accepted-row counter advances in every phase.

When read load is enabled, the sustained mixed phase must also complete at least 95% of the requested query rate with zero request failures and zero scheduler shedding. The report records requests canceled exactly at the phase boundary separately; they are not hidden as successful work.

The read scheduler is open-loop and bounded. `query_attempts` counts scheduled requests, `query_latency_ms.count` counts valid completed responses, `query_shed` counts ticks rejected because the concurrency bound was full, and `query_canceled_at_phase_end` counts in-flight requests canceled when load stopped.

Saturation is not an evidence failure. A valid phase can fail to sustain its offered target, which is the point of a capacity sweep.

## Interpretation limits

- Synthetic telemetrygen payloads are small and repetitive, so storage compression is not production-representative.
- One span, log record, and metric data point do not have equal byte or compute cost. The mixed headline is an item count and the report preserves its composition.
- A same-host observed peak is a whole-machine result, not proof of Fanout's isolated backend ceiling.
- A short peak is not a sustained capacity claim. Publish the long mixed proof alongside signal-specific sweeps.
- A stress peak that misses its read target is not serviceable mixed capacity. Use the sustained topology and its read-load gate for that claim.
- No benchmark number transfers automatically across CPU models, storage devices, filesystems, retention settings, or Fanout versions.
