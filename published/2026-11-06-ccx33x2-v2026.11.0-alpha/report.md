# Fanout performance report

> **VALID** — 6 Nov 2026 04:15 UTC · `20261106T041554Z`

## Headline results

| Workload | Fanout accepted/s | Offered/s | Avg Fanout CPU | Peak RSS |
|---|---:|---:|---:|---:|
| Traces (spans) | 330,450 | 1,280,000 | 6.97 cores | 4.6 GiB |
| Logs (records) | 648,672 | 1,280,000 | 6.02 cores | 3.4 GiB |
| Metrics (data points) | 532,625 | 1,280,000 | 7.13 cores | 9.6 GiB |
| Mixed peak | 485,193 | 3,840,000 | 7.13 cores | 3.8 GiB |
| Mixed sustained | 507,311 | 3,840,000 | 7.73 cores | 10.6 GiB |

**Meaning:** accepted throughput is the delta of `fanout_ingest_rows_total` divided by the interval between the two counter scrapes that bracket the phase. It is not telemetrygen's requested or locally generated count.

**Sustained mixed topology:** 64 processes per signal, 8 workers per process.

## Test identity

| Setting | Value |
|---|---|
| Fanout version | v2026.11.0-alpha |
| Fanout commit | unknown |
| Fanout artifact | https://github.com/labstack/fanout/releases/download/v2026.11.0-alpha/fanout_v2026.11.0-alpha_linux_amd64.tar.gz#sha256=1e83657b9eb0ff7e7ad406bd5162642badf087c380448deebd1f88b183656ddc |
| Harness | 5462cbb (`5462cbbcde9c49521c918c45f45312493aa4bb24`) |
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
| Workload SHA-256 | `63b464c361f857f876690cfe618e06ee40706f7d39ed28f9c9e3c2eee7a725af` |

## Complete phase breakdown

| Phase | Signal | Topology (processes per signal × workers) | Duration | Offered/s | Accepted/s | Ratio | Fanout CPU cores | Generator CPU cores / logical CPUs | Peak RSS | Queue peak | Result |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| warmup | mixed | 1×8 workers | 15.4s | 60,000 | 38,173 | 65.5% | 1.11 | 0.83 / 8 | 270.3 MiB | 0 | saturated-or-generator-limited |
| traces-p1-r1 | traces | 1×8 workers | 30.4s | 20,000 | 13,705 | 69.4% | 0.67 | 0.34 / 8 | 370.6 MiB | 0 | saturated-or-generator-limited |
| traces-p2-r1 | traces | 2×8 workers | 30.4s | 40,000 | 27,283 | 69.1% | 2.09 | 0.71 / 8 | 776.7 MiB | 0 | saturated-or-generator-limited |
| traces-p4-r1 | traces | 4×8 workers | 30.4s | 80,000 | 54,716 | 69.2% | 2.51 | 1.56 / 8 | 641.2 MiB | 0 | saturated-or-generator-limited |
| traces-p8-r1 | traces | 8×8 workers | 30.4s | 160,000 | 111,893 | 71.0% | 4.26 | 2.29 / 8 | 1.9 GiB | 0 | saturated-or-generator-limited |
| traces-p16-r1 | traces | 16×8 workers | 30.6s | 320,000 | 205,825 | 65.6% | 4.58 | 2.77 / 8 | 1.4 GiB | 0 | saturated-or-generator-limited |
| traces-p32-r1 | traces | 32×8 workers | 36.9s | 640,000 | 232,227 | 44.6% | 6.62 | 3.90 / 8 | 6.0 GiB | 0 | saturated-or-generator-limited |
| traces-p64-r1 | traces | 64×8 workers | 32.3s | 1,280,000 | 330,450 | 27.8% | 6.97 | 7.24 / 8 | 4.6 GiB | 3 | generator-limited |
| logs-p1-r1 | logs | 1×8 workers | 30.4s | 20,000 | 12,720 | 64.5% | 6.07 | 0.23 / 8 | 7.5 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r1 | logs | 2×8 workers | 30.4s | 40,000 | 25,184 | 63.9% | 2.89 | 0.48 / 8 | 3.8 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r1 | logs | 4×8 workers | 30.4s | 80,000 | 49,782 | 63.1% | 0.94 | 1.04 / 8 | 2.4 GiB | 0 | saturated-or-generator-limited |
| logs-p8-r1 | logs | 8×8 workers | 30.5s | 160,000 | 97,248 | 61.8% | 2.18 | 1.87 / 8 | 6.1 GiB | 1 | saturated-or-generator-limited |
| logs-p16-r1 | logs | 16×8 workers | 30.5s | 320,000 | 197,366 | 62.8% | 2.11 | 2.13 / 8 | 2.6 GiB | 0 | saturated-or-generator-limited |
| logs-p32-r1 | logs | 32×8 workers | 30.9s | 640,000 | 370,209 | 59.6% | 4.70 | 3.68 / 8 | 6.2 GiB | 28 | saturated-or-generator-limited |
| logs-p64-r1 | logs | 64×8 workers | 31.8s | 1,280,000 | 648,672 | 53.7% | 6.02 | 6.51 / 8 | 3.4 GiB | 1 | saturated-or-generator-limited |
| metrics-p1-r1 | metrics | 1×8 workers | 30.5s | 20,000 | 12,816 | 65.1% | 3.33 | 0.18 / 8 | 5.5 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r1 | metrics | 2×8 workers | 30.5s | 40,000 | 25,409 | 64.5% | 1.23 | 0.38 / 8 | 2.0 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r1 | metrics | 4×8 workers | 30.4s | 80,000 | 50,263 | 63.7% | 2.41 | 0.78 / 8 | 3.7 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r1 | metrics | 8×8 workers | 30.5s | 160,000 | 97,296 | 61.8% | 1.65 | 1.41 / 8 | 2.7 GiB | 0 | saturated-or-generator-limited |
| metrics-p16-r1 | metrics | 16×8 workers | 30.6s | 320,000 | 194,544 | 62.1% | 2.88 | 2.10 / 8 | 2.8 GiB | 11 | saturated-or-generator-limited |
| metrics-p32-r1 | metrics | 32×8 workers | 30.9s | 640,000 | 377,992 | 60.8% | 4.08 | 2.98 / 8 | 3.4 GiB | 28 | saturated-or-generator-limited |
| metrics-p64-r1 | metrics | 64×8 workers | 32.7s | 1,280,000 | 488,431 | 41.6% | 6.03 | 4.71 / 8 | 4.1 GiB | 87 | saturated-or-generator-limited |
| mixed-p1-r1 | mixed | 1×8 workers | 30.4s | 60,000 | 38,820 | 65.6% | 4.14 | 0.86 / 8 | 2.5 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r1 | mixed | 2×8 workers | 30.5s | 120,000 | 76,587 | 65.0% | 5.12 | 1.72 / 8 | 2.8 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r1 | mixed | 4×8 workers | 30.5s | 240,000 | 155,707 | 66.0% | 5.01 | 2.15 / 8 | 3.6 GiB | 0 | saturated-or-generator-limited |
| mixed-p8-r1 | mixed | 8×8 workers | 30.9s | 480,000 | 270,231 | 58.0% | 6.50 | 2.96 / 8 | 3.3 GiB | 16 | saturated-or-generator-limited |
| mixed-p16-r1 | mixed | 16×8 workers | 31.7s | 960,000 | 429,748 | 47.3% | 7.04 | 5.40 / 8 | 3.7 GiB | 42 | saturated-or-generator-limited |
| mixed-p32-r1 | mixed | 32×8 workers | 34.9s | 1,920,000 | 450,588 | 27.3% | 7.23 | 6.92 / 8 | 5.5 GiB | 175 | saturated |
| mixed-p64-r1 | mixed | 64×8 workers | 37.2s | 3,840,000 | 485,193 | 15.7% | 7.13 | 7.35 / 8 | 3.8 GiB | 256 | generator-limited |
| traces-p1-r2 | traces | 1×8 workers | 30.4s | 20,000 | 13,709 | 69.5% | 5.71 | 0.33 / 8 | 4.7 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r2 | traces | 2×8 workers | 30.4s | 40,000 | 27,295 | 69.2% | 5.37 | 0.71 / 8 | 4.5 GiB | 0 | saturated-or-generator-limited |
| traces-p4-r2 | traces | 4×8 workers | 30.4s | 80,000 | 54,462 | 69.1% | 4.38 | 1.56 / 8 | 2.7 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r2 | traces | 8×8 workers | 30.5s | 160,000 | 110,326 | 70.1% | 5.65 | 2.25 / 8 | 5.4 GiB | 0 | saturated-or-generator-limited |
| traces-p16-r2 | traces | 16×8 workers | 30.7s | 320,000 | 192,605 | 61.7% | 5.64 | 2.55 / 8 | 3.8 GiB | 0 | saturated-or-generator-limited |
| traces-p32-r2 | traces | 32×8 workers | 35.5s | 640,000 | 164,978 | 30.5% | 6.75 | 3.95 / 8 | 8.6 GiB | 0 | saturated |
| traces-p64-r2 | traces | 64×8 workers | 32.2s | 1,280,000 | 297,345 | 25.0% | 7.43 | 7.30 / 8 | 7.0 GiB | 7 | generator-limited |
| logs-p1-r2 | logs | 1×8 workers | 30.5s | 20,000 | 12,625 | 64.2% | 5.82 | 0.22 / 8 | 9.8 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r2 | logs | 2×8 workers | 30.5s | 40,000 | 24,974 | 63.6% | 5.95 | 0.46 / 8 | 5.5 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r2 | logs | 4×8 workers | 30.4s | 80,000 | 50,115 | 63.6% | 2.55 | 1.02 / 8 | 7.5 GiB | 3 | saturated-or-generator-limited |
| logs-p8-r2 | logs | 8×8 workers | 30.5s | 160,000 | 98,038 | 62.4% | 2.17 | 1.79 / 8 | 7.1 GiB | 1 | saturated-or-generator-limited |
| logs-p16-r2 | logs | 16×8 workers | 30.6s | 320,000 | 196,366 | 62.6% | 2.56 | 2.18 / 8 | 6.4 GiB | 0 | saturated-or-generator-limited |
| logs-p32-r2 | logs | 32×8 workers | 31.0s | 640,000 | 364,916 | 59.0% | 5.24 | 3.66 / 8 | 7.4 GiB | 22 | saturated-or-generator-limited |
| logs-p64-r2 | logs | 64×8 workers | 31.8s | 1,280,000 | 558,932 | 46.3% | 6.21 | 5.93 / 8 | 6.8 GiB | 95 | saturated-or-generator-limited |
| metrics-p1-r2 | metrics | 1×8 workers | 30.5s | 20,000 | 12,801 | 65.0% | 4.03 | 0.17 / 8 | 6.3 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r2 | metrics | 2×8 workers | 30.4s | 40,000 | 25,580 | 64.9% | 1.29 | 0.36 / 8 | 3.5 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r2 | metrics | 4×8 workers | 30.4s | 80,000 | 50,759 | 64.3% | 2.56 | 0.80 / 8 | 5.3 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r2 | metrics | 8×8 workers | 30.5s | 160,000 | 98,618 | 62.6% | 2.06 | 1.48 / 8 | 6.4 GiB | 0 | saturated-or-generator-limited |
| metrics-p16-r2 | metrics | 16×8 workers | 30.6s | 320,000 | 196,265 | 62.6% | 3.42 | 2.09 / 8 | 6.4 GiB | 0 | saturated-or-generator-limited |
| metrics-p32-r2 | metrics | 32×8 workers | 30.9s | 640,000 | 378,840 | 60.9% | 4.37 | 3.09 / 8 | 6.6 GiB | 1 | saturated-or-generator-limited |
| metrics-p64-r2 | metrics | 64×8 workers | 31.8s | 1,280,000 | 486,759 | 40.4% | 6.35 | 4.64 / 8 | 6.9 GiB | 133 | saturated-or-generator-limited |
| mixed-p1-r2 | mixed | 1×8 workers | 30.3s | 60,000 | 39,227 | 66.1% | 5.47 | 0.82 / 8 | 5.4 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r2 | mixed | 2×8 workers | 30.6s | 120,000 | 76,741 | 65.1% | 6.15 | 1.69 / 8 | 6.4 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r2 | mixed | 4×8 workers | 30.8s | 240,000 | 155,156 | 66.4% | 5.91 | 2.17 / 8 | 6.7 GiB | 2 | saturated-or-generator-limited |
| mixed-p8-r2 | mixed | 8×8 workers | 30.9s | 480,000 | 262,749 | 56.4% | 7.01 | 2.90 / 8 | 7.0 GiB | 0 | saturated-or-generator-limited |
| mixed-p16-r2 | mixed | 16×8 workers | 31.6s | 960,000 | 380,026 | 41.8% | 7.37 | 5.00 / 8 | 6.9 GiB | 39 | saturated-or-generator-limited |
| mixed-p32-r2 | mixed | 32×8 workers | 33.1s | 1,920,000 | 442,361 | 25.4% | 7.37 | 7.20 / 8 | 7.6 GiB | 223 | saturated-or-generator-limited |
| mixed-p64-r2 | mixed | 64×8 workers | 37.0s | 3,840,000 | 453,181 | 14.6% | 7.24 | 7.39 / 8 | 7.2 GiB | 256 | generator-limited |
| traces-p1-r3 | traces | 1×8 workers | 30.5s | 20,000 | 13,633 | 69.4% | 6.63 | 0.32 / 8 | 6.8 GiB | 0 | saturated-or-generator-limited |
| traces-p2-r3 | traces | 2×8 workers | 30.5s | 40,000 | 27,227 | 69.1% | 6.10 | 0.70 / 8 | 6.3 GiB | 0 | saturated-or-generator-limited |
| traces-p4-r3 | traces | 4×8 workers | 30.4s | 80,000 | 54,584 | 69.2% | 5.63 | 1.50 / 8 | 6.5 GiB | 0 | saturated-or-generator-limited |
| traces-p8-r3 | traces | 8×8 workers | 30.6s | 160,000 | 106,840 | 68.0% | 6.35 | 2.22 / 8 | 6.8 GiB | 0 | saturated-or-generator-limited |
| traces-p16-r3 | traces | 16×8 workers | 30.7s | 320,000 | 156,783 | 50.2% | 6.84 | 2.44 / 8 | 6.9 GiB | 2 | saturated-or-generator-limited |
| traces-p32-r3 | traces | 32×8 workers | 31.3s | 640,000 | 243,741 | 39.7% | 7.37 | 4.65 / 8 | 9.2 GiB | 1 | saturated-or-generator-limited |
| traces-p64-r3 | traces | 64×8 workers | 36.5s | 1,280,000 | 192,120 | 18.3% | 6.94 | 6.25 / 8 | 10.5 GiB | 16 | saturated |
| logs-p1-r3 | logs | 1×8 workers | 30.5s | 20,000 | 12,759 | 64.8% | 5.82 | 0.22 / 8 | 8.5 GiB | 0 | saturated-or-generator-limited |
| logs-p2-r3 | logs | 2×8 workers | 30.5s | 40,000 | 25,111 | 63.8% | 5.92 | 0.47 / 8 | 8.5 GiB | 0 | saturated-or-generator-limited |
| logs-p4-r3 | logs | 4×8 workers | 30.5s | 80,000 | 50,053 | 63.6% | 2.07 | 1.00 / 8 | 8.8 GiB | 0 | saturated-or-generator-limited |
| logs-p8-r3 | logs | 8×8 workers | 30.5s | 160,000 | 95,989 | 61.0% | 1.99 | 1.72 / 8 | 8.1 GiB | 0 | saturated-or-generator-limited |
| logs-p16-r3 | logs | 16×8 workers | 30.5s | 320,000 | 197,201 | 62.7% | 1.91 | 2.21 / 8 | 7.5 GiB | 0 | saturated-or-generator-limited |
| logs-p32-r3 | logs | 32×8 workers | 31.0s | 640,000 | 366,244 | 59.1% | 5.78 | 3.64 / 8 | 9.4 GiB | 0 | saturated-or-generator-limited |
| logs-p64-r3 | logs | 64×8 workers | 31.7s | 1,280,000 | 622,229 | 51.3% | 5.82 | 6.38 / 8 | 8.9 GiB | 28 | saturated-or-generator-limited |
| metrics-p1-r3 | metrics | 1×8 workers | 30.4s | 20,000 | 12,728 | 64.6% | 3.46 | 0.17 / 8 | 7.6 GiB | 0 | saturated-or-generator-limited |
| metrics-p2-r3 | metrics | 2×8 workers | 30.4s | 40,000 | 25,459 | 64.6% | 0.83 | 0.37 / 8 | 7.8 GiB | 0 | saturated-or-generator-limited |
| metrics-p4-r3 | metrics | 4×8 workers | 30.4s | 80,000 | 50,252 | 63.7% | 2.49 | 0.79 / 8 | 7.9 GiB | 0 | saturated-or-generator-limited |
| metrics-p8-r3 | metrics | 8×8 workers | 30.4s | 160,000 | 98,095 | 62.2% | 1.69 | 1.42 / 8 | 7.8 GiB | 3 | saturated-or-generator-limited |
| metrics-p16-r3 | metrics | 16×8 workers | 30.6s | 320,000 | 195,099 | 62.2% | 3.07 | 2.07 / 8 | 7.9 GiB | 0 | saturated-or-generator-limited |
| metrics-p32-r3 | metrics | 32×8 workers | 30.9s | 640,000 | 359,873 | 57.8% | 5.30 | 2.93 / 8 | 9.2 GiB | 47 | saturated-or-generator-limited |
| metrics-p64-r3 | metrics | 64×8 workers | 31.8s | 1,280,000 | 532,625 | 44.2% | 7.13 | 5.03 / 8 | 9.6 GiB | 106 | saturated-or-generator-limited |
| mixed-p1-r3 | mixed | 1×8 workers | 30.5s | 60,000 | 38,640 | 65.5% | 6.24 | 0.81 / 8 | 9.0 GiB | 0 | saturated-or-generator-limited |
| mixed-p2-r3 | mixed | 2×8 workers | 30.5s | 120,000 | 76,483 | 64.7% | 6.60 | 1.71 / 8 | 8.9 GiB | 0 | saturated-or-generator-limited |
| mixed-p4-r3 | mixed | 4×8 workers | 30.7s | 240,000 | 149,499 | 63.8% | 6.49 | 2.16 / 8 | 9.1 GiB | 0 | saturated-or-generator-limited |
| mixed-p8-r3 | mixed | 8×8 workers | 30.9s | 480,000 | 261,501 | 56.2% | 7.34 | 2.93 / 8 | 9.2 GiB | 3 | saturated-or-generator-limited |
| mixed-p16-r3 | mixed | 16×8 workers | 31.4s | 960,000 | 323,029 | 35.2% | 7.33 | 4.75 / 8 | 9.8 GiB | 34 | saturated-or-generator-limited |
| mixed-p32-r3 | mixed | 32×8 workers | 33.3s | 1,920,000 | 432,734 | 25.0% | 7.47 | 7.14 / 8 | 9.8 GiB | 219 | saturated-or-generator-limited |
| mixed-p64-r3 | mixed | 64×8 workers | 36.3s | 3,840,000 | 460,640 | 14.5% | 7.22 | 7.49 / 8 | 10.1 GiB | 211 | generator-limited |
| mixed-sustained | mixed | 64×8 workers | 306.8s | 3,840,000 | 507,311 | 13.5% | 7.73 | 7.93 / 8 | 10.6 GiB | 256 | generator-limited |

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
fanout-bench run --host 'root@<host>' --fanout-version 'v2026.11.0-alpha' --preset publish --repeat 3 --telemetrygen-version 'v0.157.0' --batch-size 1000 --sample-interval 1s --cooldown 15s --generator-host 'root@<generator-host>' --generator-address '<generator-address>' --fanout-listen '<fanout-listen>' --ssh-identity '<ssh-identity>' --host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04' --generator-host-description 'Hetzner Cloud ccx33, fsn1, ubuntu-24.04'
```

Raw evidence: `report.json` (authoritative structured report) and `samples.jsonl` (time-series resource samples, including cumulative `ingest_rows_total` by signal (`spans`, `logs`, `metrics`), `rollup_seconds`, `write_gate_hold_seconds`, `write_gate_wait_seconds`, `compaction_seconds_total`, `compactions_total`, `publish_wait_seconds_total`, `ingest_backpressure_seconds_total`, `ingest_throttled_total`, `ingest_admission_rows_per_second`, `compaction_retired_batches_total`, and `compaction_pass_seconds_total`, plus the current `compaction_backlog_batches` gauge when available).
