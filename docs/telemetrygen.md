# telemetrygen v0.157.0 throughput limits

The configured 20,000 items/s per process is an approximate generation target, not an achieved send rate. There is no hard-coded 13,000 items/s ceiling. The source has per-item timer pacing, a single export lane for traces, and synchronous export pauses for log and metric workers. Adding processes adds independent pacing loops and exporters, which explains why the split run can scale while Fanout still has spare CPU.

This analysis uses the supplied local module-cache source, including its pinned dependencies: OpenTelemetry Go trace SDK and trace/metric exporters v1.44.0, log exporter v0.20.0, and `golang.org/x/time` v0.15.0 (`telemetrygen/go.mod:12–28`). References below are relative to those module roots.

## Rate and batching

`--workers` creates generator goroutines. Each gets the full `--rate`, so eight workers at 2,500 request 20,000 items/s. Each worker creates a token limiter with **burst 1** and waits for each item. A trace with one child consumes two tokens: one for the parent and one for the child; the rate already counts spans. `--batch-size 1000` batches delivery but does not batch limiter reservations.

At 2,500/s, token spacing is 400 microseconds. `rate.Wait` uses a Go timer when it must wait, and accumulated tokens are capped at one. Timer wakeup and scheduler delays can therefore reduce generation below the requested rate even on a host with spare CPU; a late worker cannot catch up with a large burst. The source establishes this mechanism, but does not quantify how much of the observed shortfall came from pacing versus export latency.

| Source | Evidence |
|---|---|
| telemetrygen `internal/config/config.go:264–265,306–307,334` | Per-worker rate, worker count, batch flag/size; batching defaults on |
| telemetrygen `pkg/traces/traces.go:148–167` | One trace goroutine per worker, each given the configured rate |
| telemetrygen `pkg/traces/worker.go:100,107,142` | Burst-one limiter and separate parent/child waits |
| telemetrygen `pkg/logs/worker.go:49,93` | Burst-one limiter and one wait per record |
| telemetrygen `pkg/metrics/worker.go:100,219` | Burst-one limiter and one wait per metric |
| x/time `rate/rate.go:241–249,274–289,393–399` | Timer-based waits and burst cap |

## Export concurrency and drops

**Traces:** all workers share one `BatchSpanProcessor` and one OTLP exporter. The processor collects up to 1,000 spans or waits up to one second, then calls the exporter synchronously from its single queue-processing goroutine. The gRPC call waits for the receiver's response before returning. Eight workers do not create eight concurrent trace exports.

The trace SDK queue defaults to **2,048 spans** (unless overridden through the SDK environment). telemetrygen sets the batch size and timeout but never enables `WithBlocking`. When the queue fills, `enqueueDrop` increments its dropped counter and discards spans instead of blocking generation. These losses happen before Fanout and do not increment Fanout's dropped-row counter. A successful telemetrygen exit therefore does not prove every generated span reached Fanout.

**Logs and metrics:** each worker constructs its own exporter and buffer. Reaching the batch size calls `Export` inline; generation in that worker pauses until export completes. Exporters serialize calls within each exporter, but exporters belong to different workers, so a process can have up to eight concurrent log or metric exports. These paths call exporters directly: no SDK batch-processor queue is involved and there is no corresponding SDK queue-full drop. Remaining buffered items are flushed at the end.

| Source | Evidence |
|---|---|
| telemetrygen `pkg/traces/traces.go:58,71–98` | One exporter and shared processor; batch timeout and size; no blocking option |
| trace SDK `trace/batch_span_processor.go:22,98–112,126–129` | Default queue size and one processor goroutine |
| trace SDK `trace/batch_span_processor.go:285–309,321–355` | Synchronous batch exports, then queue processing resumes |
| trace SDK `trace/batch_span_processor.go:392–398,417–431` | Default nonblocking queue-full drops |
| trace exporter `exporter.go:30–40`; trace gRPC exporter `client.go:232–249` | Conversion, upload, synchronous gRPC response and retry handling |
| telemetrygen `pkg/logs/logs.go:72–106`; `pkg/metrics/metrics.go:70–107` | Exporter created separately for each worker |
| telemetrygen `pkg/logs/worker.go:115–152`; `pkg/metrics/worker.go:241–280` | Synchronous batch export, buffer reset and final flush |
| log gRPC exporter `exporter.go:61–73`, `client.go:171–187` | Exporter lock and synchronous RPC/retry |
| metric gRPC exporter `exporter.go:92–106`, `client.go:129–145` | Exporter lock and synchronous RPC/retry |

## Acknowledgement latency

Let `B` be batch size, `W` workers, `r` requested rate per worker, and `L` time spent exporting a full batch, including encoding, transport, server acknowledgement and any retries.

- **Traces:** generation and export overlap through the queue. Long-run throughput is bounded approximately by `min(actual generation rate, B / L)`. Larger `W` can fill the queue faster without increasing export concurrency. Longer acknowledgements lower throughput and can cause queue drops. At 1,000 spans/batch, 13,000 spans/s would correspond to roughly 77 ms per serial export **if export were the active bottleneck**.
- **Logs/metrics:** each worker alternates filling a batch and exporting it. Ignoring startup/final partial batches, per-process throughput is approximately `W × B / (actual batch-fill time + L)`. Even with ideal pacing, batch fill is about `B / r`, so the idealized rate is `W × B / (B / r + L)`, rather than a latency-independent `W × r`. Burst-one pacing prevents recovering most tokens missed during export.

The supplied split report's single-process trials accepted approximately 13.5k spans/s, 12.5k records/s and 12.5k points/s. Similar rates across these different export paths cannot be attributed solely to the serialized trace processor. Neither that report nor `samples.jsonl` measures generator pacing or RPC acknowledgement latency, so the exact contribution of each mechanism is unresolved. New samples include Fanout's [rollup, write-gate, compaction, publish-wait, and ingest-backpressure metrics](metrics.md#background-work-samples) to help attribute server work. The wider process sweep and generator CPU measurements provide the next attribution evidence without changing the workload shape.
