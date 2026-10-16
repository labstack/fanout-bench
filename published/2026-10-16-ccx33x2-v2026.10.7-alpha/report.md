# Fanout performance report

> **VALID** — 16 Oct 2026 00:51 UTC · `20261016T005135Z`

## Headline results

| Workload | Fanout accepted/s | Offered/s | Avg Fanout CPU | Peak RSS |
|---|---:|---:|---:|---:|
| Traces (spans) | 284,067 | 640,000 | 6.28 cores | 1.9 GiB |
| Logs (records) | 347,721 | 1,280,000 | 6.82 cores | 19.6 GiB |
| Metrics (data points) | 543,942 | 1,280,000 | 6.01 cores | 6.7 GiB |
| Mixed peak | 471,506 | 1,920,000 | 7.12 cores | 4.6 GiB |
| Mixed sustained | 422,408 | 1,920,000 | 7.34 cores | 13.5 GiB |

**Meaning:** accepted throughput is the delta of `fanout_ingest_rows_total` divided by the interval between the two counter scrapes that bracket the phase. It is not telemetrygen's requested or locally generated count.

**Sustained mixed topology:** 32 processes per signal, 8 workers per process.

## Test identity

| Setting | Value |
|---|---|
| Fanout version | v2026.10.7-alpha |
| Fanout commit | unknown |
| Fanout artifact | https://github.com/labstack/fanout/releases/download/v2026.10.7-alpha/fanout_v2026.10.7-alpha_linux_amd64.tar.gz#sha256=d1b602130463b38a03f7575c52c1c2178213839dbf276294abd57ce662c8756b |
| Harness | 8a2a28a (`8a2a28aafa828fc7967ba82a481c7cdb5c7e921e`) |
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
| Workload SHA-256 | `044643f8cee4484d30fb6dcbbda9522f1200dc040ecb0cf15d4869346e0ad053` |

## Complete phase breakdown

| Phase | Signal | Topology (processes per signal × workers) | Duration | Offered/s | Accepted/s | Ratio | Fanout CPU cores | Generator CPU cores / logical CPUs | Peak RSS | Queue peak | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| warmup | mixed | 1×8 workers | 15.3s | 60,000 | 40,898 | 69.6% | 1.56 | 0.33 / 8 | 497.3 MiB | 4 | saturated-or-generator-limited |
| traces-p1-r1 | traces | 1×8 workers | 30.3s | 20,000 | 14,632 | 73.9% | 1.36 | 0.14 / 8 | 597.5 MiB | 0 | saturated-or-generator-limited |
| traces-p2-r1 | traces | 2×8 workers | 30.3s | 40,000 | 29,266 | 73.9% | 2.47 | 0.30 / 8 | 839.2 MiB | 0 | saturated-or-generator-limited |
| traces-p4-r1 | traces | 4×8 workers | 30.3s | 80,000 | 58,523 | 73.9% | 2.95 | 0.64 / 8 | 1.0 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r1 | traces | 8×8 workers | 30.4s | 160,000 | 110,068 | 69.7% | 4.41 | 1.20 / 8 | 2.3 GiB | 2 | saturated-or-generator-limited |
| traces-p16-r1 | traces | 16×8 workers | 30.6s | 320,000 | 195,738 | 62.4% | 5.13 | 2.45 / 8 | 1.8 GiB | 4 | saturated-or-generator-limited |
| traces-p32-r1 | traces | 32×8 workers | 31.1s | 640,000 | 284,067 | 46.0% | 6.28 | 4.51 / 8 | 1.9 GiB | 16 | saturated-or-generator-limited |
| traces-p64-r1 | traces | 64×8 workers | 32.3s | 1,280,000 | 243,696 | 20.5% | 5.71 | 6.78 / 8 | 2.1 GiB | 6 | saturated |
| logs-p1-r1 | logs | 1×8 workers | 30.3s | 20,000 | 13,469 | 68.0% | 2.42 | 0.09 / 8 | 2.1 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r1 | logs | 2×8 workers | 30.3s | 40,000 | 26,532 | 67.0% | 1.03 | 0.19 / 8 | 1.8 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r1 | logs | 4×8 workers | 30.4s | 80,000 | 50,601 | 64.2% | 6.90 | 0.36 / 8 | 7.9 GiB | 2 | saturated-or-generator-limited |
| logs-p8-r1 | logs | 8×8 workers | 30.5s | 160,000 | 100,809 | 64.1% | 2.97 | 0.74 / 8 | 3.7 GiB | 24 | saturated-or-generator-limited |
| logs-p16-r1 | logs | 16×8 workers | 30.7s | 320,000 | 180,023 | 57.5% | 4.98 | 1.45 / 8 | 5.2 GiB | 17 | saturated-or-generator-limited |
| logs-p32-r1 | logs | 32×8 workers | 31.4s | 640,000 | 319,885 | 52.3% | 5.92 | 2.92 / 8 | 2.0 GiB | 189 | saturated-or-generator-limited |
| logs-p64-r1 | logs | 64×8 workers | 32.0s | 1,280,000 | 299,902 | 33.0% | 5.06 | 3.09 / 8 | 5.5 GiB | 256 | saturated |
| metrics-p1-r1 | metrics | 1×8 workers | 30.3s | 20,000 | 12,431 | 68.7% | 5.15 | 0.08 / 8 | 3.8 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r1 | metrics | 2×8 workers | 30.3s | 40,000 | 27,036 | 68.4% | 0.51 | 0.17 / 8 | 1.8 GiB | 5 | saturated-or-generator-limited |
| metrics-p4-r1 | metrics | 4×8 workers | 30.3s | 80,000 | 52,642 | 66.5% | 4.01 | 0.33 / 8 | 2.4 GiB | 13 | saturated-or-generator-limited |
| metrics-p8-r1 | metrics | 8×8 workers | 30.4s | 160,000 | 106,147 | 67.2% | 1.48 | 0.65 / 8 | 775.9 MiB | 3 | saturated-or-generator-limited |
| metrics-p16-r1 | metrics | 16×8 workers | 30.5s | 320,000 | 205,136 | 65.2% | 2.47 | 1.43 / 8 | 895.8 MiB | 17 | saturated-or-generator-limited |
| metrics-p32-r1 | metrics | 32×8 workers | 30.9s | 640,000 | 341,351 | 55.0% | 4.57 | 2.54 / 8 | 3.4 GiB | 94 | saturated-or-generator-limited |
| metrics-p64-r1 | metrics | 64×8 workers | 31.7s | 1,280,000 | 540,103 | 44.6% | 5.84 | 4.65 / 8 | 5.2 GiB | 115 | saturated-or-generator-limited |
| mixed-p1-r1 | mixed | 1×8 workers | 30.4s | 60,000 | 40,269 | 68.1% | 4.92 | 0.31 / 8 | 4.8 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r1 | mixed | 2×8 workers | 30.4s | 120,000 | 81,076 | 68.4% | 4.00 | 0.64 / 8 | 3.7 GiB | 6 | saturated-or-generator-limited |
| mixed-p4-r1 | mixed | 4×8 workers | 30.5s | 240,000 | 154,626 | 65.9% | 5.10 | 1.37 / 8 | 4.1 GiB | 26 | saturated-or-generator-limited |
| mixed-p8-r1 | mixed | 8×8 workers | 30.8s | 480,000 | 273,902 | 58.7% | 6.06 | 2.77 / 8 | 4.2 GiB | 22 | saturated-or-generator-limited |
| mixed-p16-r1 | mixed | 16×8 workers | 31.7s | 960,000 | 364,301 | 40.1% | 6.40 | 4.55 / 8 | 4.1 GiB | 177 | saturated-or-generator-limited |
| mixed-p32-r1 | mixed | 32×8 workers | 33.1s | 1,920,000 | 471,506 | 27.1% | 7.12 | 7.25 / 8 | 4.6 GiB | 256 | generator-limited |
| mixed-p64-r1 | mixed | 64×8 workers | 36.3s | 3,840,000 | 441,460 | 13.9% | 6.95 | 7.51 / 8 | 5.0 GiB | 256 | generator-limited |
| traces-p1-r2 | traces | 1×8 workers | 30.3s | 20,000 | 14,592 | 73.8% | 5.09 | 0.14 / 8 | 4.4 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r2 | traces | 2×8 workers | 30.3s | 40,000 | 29,231 | 73.8% | 5.61 | 0.28 / 8 | 6.2 GiB | 0 | saturated-or-generator-limited |
| traces-p4-r2 | traces | 4×8 workers | 30.3s | 80,000 | 58,233 | 73.6% | 3.68 | 0.58 / 8 | 4.1 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r2 | traces | 8×8 workers | 30.5s | 160,000 | 103,638 | 65.8% | 6.08 | 1.17 / 8 | 4.4 GiB | 2 | saturated-or-generator-limited |
| traces-p16-r2 | traces | 16×8 workers | 30.6s | 320,000 | 182,702 | 58.2% | 5.38 | 2.43 / 8 | 4.5 GiB | 5 | saturated-or-generator-limited |
| traces-p32-r2 | traces | 32×8 workers | 31.0s | 640,000 | 193,559 | 31.3% | 6.39 | 4.16 / 8 | 4.5 GiB | 14 | saturated |
| traces-p64-r2 | traces | 64×8 workers | 32.5s | 1,280,000 | 267,916 | 22.7% | 6.28 | 6.99 / 8 | 5.0 GiB | 17 | saturated-or-generator-limited |
| logs-p1-r2 | logs | 1×8 workers | 30.3s | 20,000 | 13,471 | 68.0% | 2.46 | 0.10 / 8 | 5.0 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r2 | logs | 2×8 workers | 30.4s | 40,000 | 26,878 | 68.0% | 3.15 | 0.19 / 8 | 3.8 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r2 | logs | 4×8 workers | 30.6s | 80,000 | 49,616 | 63.2% | 7.72 | 0.36 / 8 | 4.8 GiB | 6 | saturated-or-generator-limited |
| logs-p8-r2 | logs | 8×8 workers | 30.6s | 160,000 | 86,980 | 55.5% | 5.21 | 0.66 / 8 | 12.8 GiB | 17 | saturated-or-generator-limited |
| logs-p16-r2 | logs | 16×8 workers | 30.5s | 320,000 | 185,542 | 59.0% | 3.86 | 1.50 / 8 | 5.8 GiB | 11 | saturated-or-generator-limited |
| logs-p32-r2 | logs | 32×8 workers | 31.5s | 640,000 | 299,291 | 49.1% | 5.60 | 2.75 / 8 | 11.0 GiB | 129 | saturated-or-generator-limited |
| logs-p64-r2 | logs | 64×8 workers | 32.5s | 1,280,000 | 342,495 | 29.0% | 7.39 | 3.70 / 8 | 19.2 GiB | 256 | saturated-or-generator-limited |
| metrics-p1-r2 | metrics | 1×8 workers | 30.4s | 20,000 | 13,396 | 67.9% | 5.20 | 0.09 / 8 | 18.6 GiB | 1 | saturated-or-generator-limited |
| metrics-p2-r2 | metrics | 2×8 workers | 30.4s | 40,000 | 26,846 | 67.9% | 4.62 | 0.16 / 8 | 13.8 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r2 | metrics | 4×8 workers | 30.3s | 80,000 | 53,374 | 67.5% | 3.37 | 0.35 / 8 | 18.3 GiB | 5 | saturated-or-generator-limited |
| metrics-p8-r2 | metrics | 8×8 workers | 30.4s | 160,000 | 95,655 | 60.7% | 5.71 | 0.63 / 8 | 16.5 GiB | 0 | saturated-or-generator-limited |
| metrics-p16-r2 | metrics | 16×8 workers | 30.6s | 320,000 | 179,831 | 57.3% | 5.63 | 1.27 / 8 | 4.8 GiB | 28 | saturated-or-generator-limited |
| metrics-p32-r2 | metrics | 32×8 workers | 30.9s | 640,000 | 338,542 | 54.6% | 4.50 | 2.60 / 8 | 8.5 GiB | 42 | saturated-or-generator-limited |
| metrics-p64-r2 | metrics | 64×8 workers | 31.8s | 1,280,000 | 543,942 | 45.0% | 6.01 | 4.84 / 8 | 6.7 GiB | 231 | saturated-or-generator-limited |
| mixed-p1-r2 | mixed | 1×8 workers | 30.3s | 60,000 | 41,362 | 69.7% | 4.44 | 0.33 / 8 | 8.0 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r2 | mixed | 2×8 workers | 30.4s | 120,000 | 81,652 | 69.0% | 4.45 | 0.66 / 8 | 8.6 GiB | 1 | saturated-or-generator-limited |
| mixed-p4-r2 | mixed | 4×8 workers | 30.4s | 240,000 | 148,973 | 63.0% | 5.34 | 1.33 / 8 | 8.8 GiB | 16 | saturated-or-generator-limited |
| mixed-p8-r2 | mixed | 8×8 workers | 30.9s | 480,000 | 244,888 | 52.6% | 6.77 | 2.57 / 8 | 9.1 GiB | 37 | saturated-or-generator-limited |
| mixed-p16-r2 | mixed | 16×8 workers | 32.1s | 960,000 | 325,619 | 36.3% | 6.53 | 4.29 / 8 | 9.0 GiB | 204 | saturated-or-generator-limited |
| mixed-p32-r2 | mixed | 32×8 workers | 33.6s | 1,920,000 | 429,750 | 25.0% | 6.95 | 6.89 / 8 | 9.7 GiB | 256 | saturated-or-generator-limited |
| mixed-p64-r2 | mixed | 64×8 workers | 36.5s | 3,840,000 | 422,667 | 13.4% | 6.60 | 7.49 / 8 | 9.1 GiB | 256 | generator-limited |
| traces-p1-r3 | traces | 1×8 workers | 30.4s | 20,000 | 14,377 | 73.0% | 4.92 | 0.14 / 8 | 8.8 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r3 | traces | 2×8 workers | 30.4s | 40,000 | 28,622 | 72.4% | 4.08 | 0.28 / 8 | 8.8 GiB | 0 | saturated-or-generator-limited |
| traces-p4-r3 | traces | 4×8 workers | 30.6s | 80,000 | 54,233 | 69.3% | 5.70 | 0.57 / 8 | 9.0 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r3 | traces | 8×8 workers | 30.6s | 160,000 | 104,037 | 66.2% | 4.61 | 1.24 / 8 | 8.8 GiB | 0 | saturated-or-generator-limited |
| traces-p16-r3 | traces | 16×8 workers | 30.8s | 320,000 | 150,683 | 48.4% | 5.48 | 2.25 / 8 | 9.1 GiB | 8 | saturated-or-generator-limited |
| traces-p32-r3 | traces | 32×8 workers | 31.3s | 640,000 | 183,113 | 29.8% | 6.07 | 4.22 / 8 | 9.4 GiB | 3 | saturated-or-generator-limited |
| traces-p64-r3 | traces | 64×8 workers | 32.3s | 1,280,000 | 225,110 | 19.0% | 6.03 | 6.84 / 8 | 9.4 GiB | 14 | saturated-or-generator-limited |
| logs-p1-r3 | logs | 1×8 workers | 30.3s | 20,000 | 13,139 | 66.5% | 7.39 | 0.09 / 8 | 12.7 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r3 | logs | 2×8 workers | 30.5s | 40,000 | 26,437 | 67.2% | 4.24 | 0.19 / 8 | 15.2 GiB | 5 | saturated-or-generator-limited |
| logs-p4-r3 | logs | 4×8 workers | 30.4s | 80,000 | 52,532 | 66.6% | 4.35 | 0.39 / 8 | 9.1 GiB | 0 | saturated-or-generator-limited |
| logs-p8-r3 | logs | 8×8 workers | 30.4s | 160,000 | 104,131 | 66.0% | 2.44 | 0.79 / 8 | 6.1 GiB | 20 | saturated-or-generator-limited |
| logs-p16-r3 | logs | 16×8 workers | 30.8s | 320,000 | 170,625 | 54.7% | 5.63 | 1.39 / 8 | 18.8 GiB | 42 | saturated-or-generator-limited |
| logs-p32-r3 | logs | 32×8 workers | 30.9s | 640,000 | 247,015 | 39.8% | 5.71 | 2.22 / 8 | 17.4 GiB | 111 | saturated-or-generator-limited |
| logs-p64-r3 | logs | 64×8 workers | 32.0s | 1,280,000 | 347,721 | 28.9% | 6.82 | 3.47 / 8 | 19.6 GiB | 256 | saturated-or-generator-limited |
| metrics-p1-r3 | metrics | 1×8 workers | 30.4s | 20,000 | 13,192 | 66.8% | 4.30 | 0.08 / 8 | 9.1 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r3 | metrics | 2×8 workers | 30.4s | 40,000 | 26,708 | 67.7% | 5.22 | 0.17 / 8 | 18.6 GiB | 1 | saturated-or-generator-limited |
| metrics-p4-r3 | metrics | 4×8 workers | 30.3s | 80,000 | 32,336 | 57.5% | 3.16 | 0.19 / 8 | 18.5 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r3 | metrics | 8×8 workers | 30.4s | 160,000 | 99,629 | 63.2% | 3.85 | 0.64 / 8 | 11.9 GiB | 33 | saturated-or-generator-limited |
| metrics-p16-r3 | metrics | 16×8 workers | 30.6s | 320,000 | 184,239 | 58.9% | 4.98 | 1.27 / 8 | 18.8 GiB | 51 | saturated-or-generator-limited |
| metrics-p32-r3 | metrics | 32×8 workers | 30.9s | 640,000 | 263,805 | 43.4% | 5.77 | 1.90 / 8 | 18.8 GiB | 104 | saturated-or-generator-limited |
| metrics-p64-r3 | metrics | 64×8 workers | 43.0s | 1,280,000 | 293,700 | 32.9% | 5.48 | 2.52 / 8 | 19.5 GiB | 256 | saturated-or-generator-limited |
| mixed-p1-r3 | mixed | 1×8 workers | 30.4s | 60,000 | 41,301 | 69.8% | 5.60 | 0.31 / 8 | 12.0 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r3 | mixed | 2×8 workers | 30.4s | 120,000 | 80,662 | 68.1% | 4.82 | 0.65 / 8 | 12.2 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r3 | mixed | 4×8 workers | 30.6s | 240,000 | 130,087 | 55.4% | 4.87 | 1.20 / 8 | 12.6 GiB | 21 | saturated-or-generator-limited |
| mixed-p8-r3 | mixed | 8×8 workers | 30.9s | 480,000 | 219,064 | 47.0% | 5.92 | 2.36 / 8 | 12.6 GiB | 27 | saturated-or-generator-limited |
| mixed-p16-r3 | mixed | 16×8 workers | 31.6s | 960,000 | 330,233 | 36.3% | 6.33 | 4.24 / 8 | 12.5 GiB | 47 | saturated-or-generator-limited |
| mixed-p32-r3 | mixed | 32×8 workers | 32.9s | 1,920,000 | 429,505 | 24.6% | 6.84 | 7.02 / 8 | 12.6 GiB | 194 | saturated-or-generator-limited |
| mixed-p64-r3 | mixed | 64×8 workers | 37.3s | 3,840,000 | 384,338 | 12.4% | 6.80 | 7.29 / 8 | 13.4 GiB | 256 | generator-limited |
| mixed-sustained | mixed | 32×8 workers | 303.0s | 1,920,000 | 422,408 | 22.2% | 7.34 | 7.02 / 8 | 13.5 GiB | 256 | saturated-or-generator-limited |

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
fanout-bench run --host 'root@<host>' --fanout-version 'v2026.10.7-alpha' --preset publish --repeat 3 --telemetrygen-version 'v0.157.0' --batch-size 1000 --sample-interval 1s --cooldown 15s --generator-host 'root@<generator-host>' --generator-address '<generator-address>' --fanout-listen '<fanout-listen>' --host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04' --generator-host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04'
```

Raw evidence: `report.json` (authoritative structured report) and `samples.jsonl` (time-series resource samples, including cumulative `rollup_seconds`, `write_gate_hold_seconds`, `write_gate_wait_seconds`, `compaction_seconds_total`, `compactions_total`, `publish_wait_seconds_total`, and `ingest_backpressure_seconds_total`, plus the current `compaction_backlog_batches` gauge when available).
