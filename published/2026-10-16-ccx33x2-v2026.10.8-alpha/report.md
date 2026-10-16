# Fanout performance report

> **VALID** — 16 Oct 2026 17:36 UTC · `20261016T173645Z`

## Headline results

| Workload | Fanout accepted/s | Offered/s | Avg Fanout CPU | Peak RSS |
|---|---:|---:|---:|---:|
| Traces (spans) | 295,566 | 640,000 | 6.22 cores | 2.0 GiB |
| Logs (records) | 393,976 | 1,280,000 | 6.85 cores | 11.6 GiB |
| Metrics (data points) | 561,000 | 1,280,000 | 6.26 cores | 3.0 GiB |
| Mixed peak | 433,339 | 1,920,000 | 7.07 cores | 4.6 GiB |
| Mixed sustained | 415,859 | 1,920,000 | 7.24 cores | 11.4 GiB |

**Meaning:** accepted throughput is the delta of `fanout_ingest_rows_total` divided by the interval between the two counter scrapes that bracket the phase. It is not telemetrygen's requested or locally generated count.

**Sustained mixed topology:** 32 processes per signal, 8 workers per process.

## Test identity

| Setting | Value |
|---|---|
| Fanout version | v2026.10.8-alpha |
| Fanout commit | unknown |
| Fanout artifact | https://github.com/labstack/fanout/releases/download/v2026.10.8-alpha/fanout_v2026.10.8-alpha_linux_amd64.tar.gz#sha256=b314920221693683b7d6ff42787a37e82bee1fbdd7ddcc8647d1905f7629ac6b |
| Harness | 8f5864b (`8f5864ba488d9802f68250d85f67b6658ab13af5`) |
| telemetrygen | v0.157.0 |
| Machine | linux/amd64 · AMD EPYC-Milan Processor · Hetzner Cloud ccx33, fsn1, ubuntu-24.04 · 8 logical CPUs · 30.6 GiB RAM |
| Generator host | Hetzner Cloud ccx33, fsn1, ubuntu-24.04 |
| Generator placement | separate/remote |
| Preset | publish |
| Transport | plaintext gRPC |
| Batch size | 1000 |
| Trace shape | 1 parent + 1 child span per telemetrygen trace tick |
| Repeat | 3 |
| Sample interval | 1.0s |
| Workload SHA-256 | `f6904e2452aff00d5b13787b6846777d40352948d123eb2687caa100638eb6ac` |

## Complete phase breakdown

| Phase | Signal | Topology (processes per signal × workers) | Duration | Offered/s | Accepted/s | Ratio | Fanout CPU cores | Generator CPU cores / logical CPUs | Peak RSS | Queue peak | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| warmup | mixed | 1×8 workers | 15.4s | 60,000 | 38,883 | 66.6% | 1.06 | 0.83 / 8 | 508.3 MiB | 0 | saturated-or-generator-limited |
| traces-p1-r1 | traces | 1×8 workers | 30.4s | 20,000 | 13,699 | 69.4% | 0.84 | 0.32 / 8 | 569.2 MiB | 0 | saturated-or-generator-limited |
| traces-p2-r1 | traces | 2×8 workers | 30.4s | 40,000 | 27,318 | 69.2% | 1.91 | 0.70 / 8 | 940.3 MiB | 0 | saturated-or-generator-limited |
| traces-p4-r1 | traces | 4×8 workers | 30.4s | 80,000 | 54,500 | 69.0% | 2.20 | 1.56 / 8 | 1.0 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r1 | traces | 8×8 workers | 30.5s | 160,000 | 107,347 | 68.2% | 4.09 | 2.26 / 8 | 1.6 GiB | 1 | saturated-or-generator-limited |
| traces-p16-r1 | traces | 16×8 workers | 30.6s | 320,000 | 209,724 | 66.8% | 4.79 | 3.18 / 8 | 1.9 GiB | 6 | saturated-or-generator-limited |
| traces-p32-r1 | traces | 32×8 workers | 31.0s | 640,000 | 295,566 | 47.8% | 6.22 | 5.34 / 8 | 2.0 GiB | 5 | saturated-or-generator-limited |
| traces-p64-r1 | traces | 64×8 workers | 32.5s | 1,280,000 | 231,153 | 19.6% | 5.44 | 7.39 / 8 | 1.8 GiB | 14 | generator-limited |
| logs-p1-r1 | logs | 1×8 workers | 30.4s | 20,000 | 12,634 | 64.1% | 2.66 | 0.21 / 8 | 1.8 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r1 | logs | 2×8 workers | 30.5s | 40,000 | 25,771 | 65.5% | 0.52 | 0.46 / 8 | 1.4 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r1 | logs | 4×8 workers | 30.5s | 80,000 | 49,973 | 63.4% | 2.16 | 0.94 / 8 | 3.9 GiB | 0 | saturated-or-generator-limited |
| logs-p8-r1 | logs | 8×8 workers | 30.6s | 160,000 | 101,109 | 64.5% | 2.02 | 1.63 / 8 | 1.0 GiB | 2 | saturated-or-generator-limited |
| logs-p16-r1 | logs | 16×8 workers | 30.9s | 320,000 | 144,120 | 46.3% | 7.50 | 1.70 / 8 | 8.6 GiB | 34 | saturated-or-generator-limited |
| logs-p32-r1 | logs | 32×8 workers | 31.3s | 640,000 | 300,455 | 49.0% | 4.70 | 3.14 / 8 | 7.9 GiB | 104 | saturated-or-generator-limited |
| logs-p64-r1 | logs | 64×8 workers | 32.0s | 1,280,000 | 393,976 | 32.9% | 6.85 | 4.63 / 8 | 11.6 GiB | 256 | saturated-or-generator-limited |
| metrics-p1-r1 | metrics | 1×8 workers | 30.4s | 20,000 | 12,983 | 65.8% | 1.91 | 0.17 / 8 | 2.2 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r1 | metrics | 2×8 workers | 30.4s | 40,000 | 25,338 | 64.2% | 3.35 | 0.34 / 8 | 9.0 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r1 | metrics | 4×8 workers | 30.4s | 80,000 | 47,331 | 60.0% | 3.00 | 0.70 / 8 | 9.3 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r1 | metrics | 8×8 workers | 30.5s | 160,000 | 84,021 | 53.3% | 4.78 | 1.14 / 8 | 6.3 GiB | 42 | saturated-or-generator-limited |
| metrics-p16-r1 | metrics | 16×8 workers | 30.6s | 320,000 | 200,063 | 63.8% | 3.52 | 1.55 / 8 | 3.8 GiB | 51 | saturated-or-generator-limited |
| metrics-p32-r1 | metrics | 32×8 workers | 30.8s | 640,000 | 351,972 | 56.5% | 3.14 | 2.86 / 8 | 2.5 GiB | 92 | saturated-or-generator-limited |
| metrics-p64-r1 | metrics | 64×8 workers | 31.6s | 1,280,000 | 561,000 | 46.2% | 6.26 | 5.31 / 8 | 3.0 GiB | 82 | saturated-or-generator-limited |
| mixed-p1-r1 | mixed | 1×8 workers | 30.4s | 60,000 | 39,149 | 66.1% | 3.93 | 0.78 / 8 | 2.8 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r1 | mixed | 2×8 workers | 30.5s | 120,000 | 77,784 | 65.8% | 4.30 | 1.77 / 8 | 4.1 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r1 | mixed | 4×8 workers | 30.5s | 240,000 | 155,962 | 66.1% | 4.71 | 1.87 / 8 | 4.3 GiB | 0 | saturated-or-generator-limited |
| mixed-p8-r1 | mixed | 8×8 workers | 30.9s | 480,000 | 216,861 | 46.5% | 6.27 | 3.03 / 8 | 4.3 GiB | 77 | saturated-or-generator-limited |
| mixed-p16-r1 | mixed | 16×8 workers | 31.5s | 960,000 | 379,219 | 41.5% | 6.44 | 5.78 / 8 | 4.6 GiB | 48 | saturated-or-generator-limited |
| mixed-p32-r1 | mixed | 32×8 workers | 33.3s | 1,920,000 | 433,339 | 25.0% | 7.07 | 7.46 / 8 | 4.6 GiB | 172 | generator-limited |
| mixed-p64-r1 | mixed | 64×8 workers | 37.8s | 3,840,000 | 385,274 | 12.6% | 6.86 | 7.35 / 8 | 5.1 GiB | 256 | generator-limited |
| traces-p1-r2 | traces | 1×8 workers | 30.6s | 20,000 | 13,620 | 69.6% | 6.11 | 0.32 / 8 | 5.8 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r2 | traces | 2×8 workers | 30.6s | 40,000 | 27,131 | 69.1% | 5.11 | 0.70 / 8 | 4.4 GiB | 1 | saturated-or-generator-limited |
| traces-p4-r2 | traces | 4×8 workers | 30.4s | 80,000 | 54,472 | 69.1% | 3.58 | 1.56 / 8 | 4.2 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r2 | traces | 8×8 workers | 30.6s | 160,000 | 109,337 | 69.6% | 4.26 | 2.19 / 8 | 4.1 GiB | 1 | saturated-or-generator-limited |
| traces-p16-r2 | traces | 16×8 workers | 30.7s | 320,000 | 187,925 | 60.1% | 5.32 | 2.76 / 8 | 4.4 GiB | 3 | saturated-or-generator-limited |
| traces-p32-r2 | traces | 32×8 workers | 31.1s | 640,000 | 248,901 | 40.3% | 6.23 | 4.84 / 8 | 4.2 GiB | 11 | saturated-or-generator-limited |
| traces-p64-r2 | traces | 64×8 workers | 32.4s | 1,280,000 | 213,246 | 18.0% | 6.60 | 6.95 / 8 | 4.4 GiB | 15 | saturated |
| logs-p1-r2 | logs | 1×8 workers | 30.4s | 20,000 | 12,829 | 65.0% | 3.63 | 0.22 / 8 | 5.0 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r2 | logs | 2×8 workers | 30.4s | 40,000 | 25,759 | 65.3% | 0.46 | 0.45 / 8 | 3.6 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r2 | logs | 4×8 workers | 30.4s | 80,000 | 49,485 | 62.7% | 2.93 | 0.92 / 8 | 4.5 GiB | 1 | saturated-or-generator-limited |
| logs-p8-r2 | logs | 8×8 workers | 30.6s | 160,000 | 103,201 | 65.8% | 1.92 | 1.44 / 8 | 4.7 GiB | 4 | saturated-or-generator-limited |
| logs-p16-r2 | logs | 16×8 workers | 30.7s | 320,000 | 184,552 | 59.0% | 6.21 | 2.02 / 8 | 5.0 GiB | 8 | saturated-or-generator-limited |
| logs-p32-r2 | logs | 32×8 workers | 31.0s | 640,000 | 282,251 | 45.6% | 4.70 | 3.46 / 8 | 5.3 GiB | 22 | saturated-or-generator-limited |
| logs-p64-r2 | logs | 64×8 workers | 32.6s | 1,280,000 | 387,579 | 32.9% | 6.04 | 4.92 / 8 | 5.6 GiB | 256 | saturated-or-generator-limited |
| metrics-p1-r2 | metrics | 1×8 workers | 30.5s | 20,000 | 12,979 | 66.0% | 4.34 | 0.17 / 8 | 4.4 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r2 | metrics | 2×8 workers | 30.6s | 40,000 | 25,334 | 64.6% | 7.91 | 0.37 / 8 | 13.6 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r2 | metrics | 4×8 workers | 30.6s | 80,000 | 47,784 | 60.9% | 4.82 | 0.74 / 8 | 18.9 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r2 | metrics | 8×8 workers | 30.5s | 160,000 | 105,921 | 67.4% | 1.59 | 1.45 / 8 | 2.9 GiB | 1 | saturated-or-generator-limited |
| metrics-p16-r2 | metrics | 16×8 workers | 30.7s | 320,000 | 189,639 | 60.7% | 3.83 | 1.80 / 8 | 18.7 GiB | 19 | saturated-or-generator-limited |
| metrics-p32-r2 | metrics | 32×8 workers | 31.1s | 640,000 | 257,260 | 41.7% | 6.62 | 2.58 / 8 | 17.9 GiB | 37 | saturated-or-generator-limited |
| metrics-p64-r2 | metrics | 64×8 workers | 31.7s | 1,280,000 | 524,878 | 43.3% | 5.77 | 4.87 / 8 | 11.3 GiB | 227 | saturated-or-generator-limited |
| mixed-p1-r2 | mixed | 1×8 workers | 30.5s | 60,000 | 39,041 | 66.1% | 4.72 | 0.80 / 8 | 5.9 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r2 | mixed | 2×8 workers | 30.4s | 120,000 | 78,573 | 66.5% | 4.15 | 1.59 / 8 | 6.9 GiB | 2 | saturated-or-generator-limited |
| mixed-p4-r2 | mixed | 4×8 workers | 30.7s | 240,000 | 146,792 | 62.6% | 5.14 | 1.65 / 8 | 7.0 GiB | 2 | saturated-or-generator-limited |
| mixed-p8-r2 | mixed | 8×8 workers | 31.0s | 480,000 | 223,985 | 48.3% | 6.05 | 2.55 / 8 | 7.2 GiB | 61 | saturated-or-generator-limited |
| mixed-p16-r2 | mixed | 16×8 workers | 31.4s | 960,000 | 359,086 | 39.3% | 6.34 | 4.90 / 8 | 7.6 GiB | 56 | saturated-or-generator-limited |
| mixed-p32-r2 | mixed | 32×8 workers | 33.1s | 1,920,000 | 402,062 | 23.1% | 7.17 | 7.34 / 8 | 7.8 GiB | 161 | generator-limited |
| mixed-p64-r2 | mixed | 64×8 workers | 37.1s | 3,840,000 | 384,289 | 12.4% | 6.77 | 7.41 / 8 | 8.2 GiB | 256 | generator-limited |
| traces-p1-r3 | traces | 1×8 workers | 30.6s | 20,000 | 13,619 | 69.4% | 4.78 | 0.34 / 8 | 7.1 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r3 | traces | 2×8 workers | 30.5s | 40,000 | 26,662 | 67.8% | 4.75 | 0.73 / 8 | 7.0 GiB | 0 | saturated-or-generator-limited |
| traces-p4-r3 | traces | 4×8 workers | 30.6s | 80,000 | 52,886 | 67.3% | 5.54 | 1.57 / 8 | 7.7 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r3 | traces | 8×8 workers | 30.7s | 160,000 | 105,220 | 67.3% | 4.72 | 2.06 / 8 | 7.1 GiB | 0 | saturated-or-generator-limited |
| traces-p16-r3 | traces | 16×8 workers | 30.8s | 320,000 | 149,404 | 48.1% | 5.21 | 2.82 / 8 | 7.5 GiB | 5 | saturated-or-generator-limited |
| traces-p32-r3 | traces | 32×8 workers | 31.3s | 640,000 | 207,920 | 33.9% | 5.96 | 5.19 / 8 | 7.8 GiB | 1 | saturated-or-generator-limited |
| traces-p64-r3 | traces | 64×8 workers | 32.2s | 1,280,000 | 221,096 | 18.6% | 5.84 | 7.51 / 8 | 7.8 GiB | 14 | generator-limited |
| logs-p1-r3 | logs | 1×8 workers | 30.4s | 20,000 | 12,628 | 64.0% | 6.26 | 0.22 / 8 | 7.6 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r3 | logs | 2×8 workers | 30.4s | 40,000 | 24,687 | 62.6% | 2.39 | 0.44 / 8 | 7.9 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r3 | logs | 4×8 workers | 30.4s | 80,000 | 49,365 | 62.6% | 3.46 | 0.94 / 8 | 9.8 GiB | 6 | saturated-or-generator-limited |
| logs-p8-r3 | logs | 8×8 workers | 30.5s | 160,000 | 101,384 | 64.4% | 2.34 | 1.99 / 8 | 8.8 GiB | 0 | saturated-or-generator-limited |
| logs-p16-r3 | logs | 16×8 workers | 30.9s | 320,000 | 151,645 | 48.8% | 7.38 | 2.38 / 8 | 10.1 GiB | 55 | saturated-or-generator-limited |
| logs-p32-r3 | logs | 32×8 workers | 31.1s | 640,000 | 234,547 | 38.0% | 7.10 | 2.58 / 8 | 14.7 GiB | 148 | saturated-or-generator-limited |
| logs-p64-r3 | logs | 64×8 workers | 32.0s | 1,280,000 | 353,862 | 29.5% | 7.20 | 4.29 / 8 | 9.8 GiB | 256 | saturated-or-generator-limited |
| metrics-p1-r3 | metrics | 1×8 workers | 30.4s | 20,000 | 13,064 | 66.2% | 2.40 | 0.17 / 8 | 6.3 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r3 | metrics | 2×8 workers | 30.5s | 40,000 | 26,069 | 66.2% | 3.80 | 0.36 / 8 | 18.5 GiB | 1 | saturated-or-generator-limited |
| metrics-p4-r3 | metrics | 4×8 workers | 30.5s | 80,000 | 48,182 | 61.2% | 5.31 | 0.72 / 8 | 18.6 GiB | 20 | saturated-or-generator-limited |
| metrics-p8-r3 | metrics | 8×8 workers | 30.5s | 160,000 | 86,388 | 54.9% | 4.28 | 1.31 / 8 | 18.0 GiB | 43 | saturated-or-generator-limited |
| metrics-p16-r3 | metrics | 16×8 workers | 30.7s | 320,000 | 190,894 | 61.1% | 3.82 | 2.20 / 8 | 18.9 GiB | 10 | saturated-or-generator-limited |
| metrics-p32-r3 | metrics | 32×8 workers | 31.0s | 640,000 | 301,283 | 48.7% | 5.36 | 3.07 / 8 | 19.0 GiB | 151 | saturated-or-generator-limited |
| metrics-p64-r3 | metrics | 64×8 workers | 31.8s | 1,280,000 | 472,496 | 39.2% | 6.06 | 5.33 / 8 | 17.6 GiB | 142 | saturated-or-generator-limited |
| mixed-p1-r3 | mixed | 1×8 workers | 30.5s | 60,000 | 38,296 | 64.8% | 5.44 | 0.83 / 8 | 9.3 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r3 | mixed | 2×8 workers | 30.5s | 120,000 | 75,221 | 63.8% | 5.28 | 1.81 / 8 | 10.0 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r3 | mixed | 4×8 workers | 30.6s | 240,000 | 146,445 | 62.3% | 4.86 | 2.68 / 8 | 10.2 GiB | 7 | saturated-or-generator-limited |
| mixed-p8-r3 | mixed | 8×8 workers | 30.9s | 480,000 | 238,993 | 51.3% | 6.06 | 3.51 / 8 | 10.5 GiB | 72 | saturated-or-generator-limited |
| mixed-p16-r3 | mixed | 16×8 workers | 31.8s | 960,000 | 335,283 | 37.0% | 6.35 | 5.78 / 8 | 10.7 GiB | 118 | saturated-or-generator-limited |
| mixed-p32-r3 | mixed | 32×8 workers | 33.5s | 1,920,000 | 357,956 | 20.8% | 7.00 | 7.27 / 8 | 11.5 GiB | 181 | generator-limited |
| mixed-p64-r3 | mixed | 64×8 workers | 37.4s | 3,840,000 | 414,315 | 13.5% | 7.06 | 7.28 / 8 | 11.1 GiB | 256 | generator-limited |
| mixed-sustained | mixed | 32×8 workers | 303.5s | 1,920,000 | 415,859 | 21.9% | 7.24 | 7.44 / 8 | 11.4 GiB | 256 | generator-limited |

## Quality gates

- ✅ **generator-processes:** all exited successfully
- ✅ **server-dropped-rows:** zero
- ✅ **server-restarts:** none
- ✅ **authoritative-counters:** Fanout accepted rows in every phase
- ✅ **storage-repair-verify:** authoritative Parquet storage verified

## Methodology and limits

Fanout server-side accepted-row counter deltas divided by measured wall time; telemetrygen emits one parent and one child span per trace tick. Split runs bracket each phase with generator-host /proc/stat reads over the generator SSH setup, including startup and export draining. Generator CPU cores = logical CPUs × (total tick delta − idle and iowait tick delta) / total tick delta; guest ticks are not counted twice. Invalid trials take precedence; accepting at least 95% of the target remains target-sustained. Otherwise generator CPU at or above 90% is generator-limited; below 90%, a sweep gain under 10% over the preceding process count for the same signal and repetition is saturated. Without a valid previous sweep point or a plateau, the existing acceptance-ratio classification applies; same-host runs retain that fallback. The sustained phase uses the mixed sweep topology with highest accepted throughput meeting the read target, or the best read-completion fallback if none qualifies.

- Results describe this exact host, Fanout build, configuration, dataset, and workload; they are not universal capacity claims.
- telemetrygen payloads are intentionally synthetic and highly compressible; storage size is not representative of production telemetry.
- Offered load is a target. Headline throughput uses Fanout's accepted-row counters, never telemetrygen's intended or generated count.
- This suite measured writes only. Configure query load to claim mixed read/write performance.

## Reproduce

```sh
fanout-bench run --host 'root@<host>' --fanout-version 'v2026.10.8-alpha' --preset publish --repeat 3 --telemetrygen-version 'v0.157.0' --batch-size 1000 --sample-interval 1s --cooldown 15s --generator-host 'root@<generator-host>' --generator-address '<generator-address>' --fanout-listen '<fanout-listen>' --host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04' --generator-host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04'
```

Raw evidence: `report.json` (authoritative structured report) and `samples.jsonl` (time-series resource samples, including cumulative `ingest_rows_total` by signal (`spans`, `logs`, `metrics`), `rollup_seconds`, `write_gate_hold_seconds`, `write_gate_wait_seconds`, `compaction_seconds_total`, `compactions_total`, `publish_wait_seconds_total`, and `ingest_backpressure_seconds_total`, plus the current `compaction_backlog_batches` gauge when available).
