# Metric definitions

Metrics are scraped from `/metrics` using `Authorization: Bearer …` with `FANOUT_METRICS_TOKEN` (local harness input: `FANOUT_BENCH_METRICS_TOKEN`). Managed runs generate a private token in memory.

## Throughput

| Field | Definition |
|---|---|
| offered items/s | configured processes × workers × per-worker rate; multiplied by three for mixed load |
| accepted count | end minus start of Fanout's per-signal `fanout_ingest_rows_total` counter |
| accepted items/s | total accepted count ÷ the interval between the counter scrapes bracketing the phase |
| acceptance ratio | accepted count ÷ (offered items/s × requested generator duration) |

Wall time includes generator startup and final batch delivery. This makes throughput conservative and prevents final flush time from disappearing from the denominator.

telemetrygen exports a parent and child span per generated trace, but its limiter consumes one token for each span. Its `--rate` is therefore already aggregate spans/s; Fanout Bench does not apply another trace-shape multiplier.

## Server resources

| Field | Source and calculation |
|---|---|
| average CPU cores | delta of `process_cpu_seconds_total` ÷ that same scrape-to-scrape interval |
| peak/end RSS | sampled `process_resident_memory_bytes` |
| peak/end Go heap | sampled `go_memstats_heap_alloc_bytes` |
| allocations | delta of `go_memstats_alloc_bytes_total` |
| GC pause | delta of `go_gc_duration_seconds_sum` |
| queue peak | maximum sampled `fanout_ingest_queue_depth` |
| Parquet files/bytes | `fanout_parquet_files` and `fanout_parquet_size_bytes` |
| disk growth | recursive regular-file size delta when `--data-dir` is provided |

CPU is the Fanout process only. On a same-host run it does not include telemetrygen CPU, but telemetrygen still competes with Fanout for machine resources. RSS and heap are sampled during the phase; an end-only measurement would miss transient peaks.

The sampled series above, plus `fanout_rows_dropped_total`, `process_start_time_seconds`, `fanout_parquet_publish_waiters`, and `fanout_parquet_publish_timeouts_total`, come from Fanout v2026.10.1-alpha and its registered Go/process collectors.

## Read workload

Authenticated reads rotate equally across user operations `dashboard-refresh`, `drill-traces` (System overview service-map selection), and `trace-detail`, at 15m, 1h, 6h, 12h, and 24h. A refresh follows `page.tsx`, `use-panel-results.ts`, and `refresh.ts`: one sorted batch of all initially visible panel IDs with the full dashboard spec, range and refresh setting, service All, comparison per spec, and layout widths rounded to 100 pixels for a fixed 1664-pixel viewport. It then sends `POST /api/annotations` with the union of the returned time-panel bounds, when annotations are enabled and those bounds are present.

Drill-down follows `drill-state.ts` and `drill.tsx`: select the first service-map node, then `POST /api/panels/exemplars` with the original dashboard, panel ID, kind, absolute panel window (`refresh: off`), full-window selection, service dimension, and variables. Trace detail is a separate UI operation using `GET /api/traces/:id` with the discovered namespace, original absolute window, and limit 200. References and selections are retained separately per range. Without a discovered trace, detail slots perform drill-down; without a cached selection, the slot performs dashboard refresh instead. A later slot can select the returned node, so each tick remains one user operation. Fallbacks are reported under the operation actually performed. The current System overview defines no log drill or standalone log/trace search, so synthetic search panels are omitted.

One scheduled read is one operation. Its latency runs until every request completes; any failed request, panel error, missing panel result, or empty discovered trace detail fails the operation. Responses remain bounded at 32 MiB; responses exceeding this cap fail because their contents cannot be validated. The existing methodology string records the fetched dashboard spec SHA-256 and per-phase operation kinds, dispatched/completed counts, failures, cancellations, p95, and p99. Aggregate latency fields retain their schema.

## Validity

A report passes its evidence-quality gates when:

1. every telemetrygen process exits successfully;
2. Fanout reports no dropped rows;
3. the Fanout process start time does not change;
4. Fanout's accepted-row counter advances in every phase.

When read load is enabled, the sustained mixed phase must also complete at least 95% of the requested operation rate with zero operation failures and zero scheduler shedding. The report records operations canceled exactly at the phase boundary separately; they are not hidden as successful work.

The read scheduler is open-loop and bounded. `query_attempts` counts scheduled operations, `query_latency_ms.count` counts valid completed operations, `query_shed` counts ticks rejected because the concurrency bound was full, and `query_canceled_at_phase_end` counts in-flight operations canceled when load stopped.

Saturation is not an evidence failure. A valid phase can fail to sustain its offered target, which is the point of a capacity sweep.

## Interpretation limits

- Synthetic telemetrygen payloads are small and repetitive, so storage compression is not production-representative.
- One span, log record, and metric data point do not have equal byte or compute cost. The mixed headline is an item count and the report preserves its composition.
- A same-host observed peak is a whole-machine result, not proof of Fanout's isolated backend ceiling.
- A short peak is not a sustained capacity claim. Publish the long mixed proof alongside signal-specific sweeps.
- A stress peak that misses its read target is not serviceable mixed capacity. Use the sustained topology and its read-load gate for that claim.
- No benchmark number transfers automatically across CPU models, storage devices, filesystems, retention settings, or Fanout versions.
