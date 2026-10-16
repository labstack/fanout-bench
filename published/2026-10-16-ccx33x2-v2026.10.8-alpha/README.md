# Fanout v2026.10.8-alpha on two Hetzner ccx33s

Fanout ran alone on a Hetzner Cloud ccx33 (8 dedicated AMD EPYC-Milan vCPUs,
32 GB RAM, fsn1, Ubuntu 24.04). telemetrygen v0.157.0 ran on a separate ccx33
in the same location, sending plaintext OTLP/gRPC over a private network. The
run used the publish preset with three repetitions: sweeps of 1 to 64
telemetrygen processes per signal, 8 workers per process, 1,000-item batches,
then five minutes of mixed load at 32 processes per signal. Ingest only.

Every evidence gate passed: all generator processes exited successfully,
Fanout dropped no rows and did not restart, and `fanout repair verify`
validated the Parquet store.

| Sustained mixed, five minutes | Accepted/s |
|---|---:|
| Total items | 415,859 |
| Spans | 57,037 |
| Log records | 179,134 |
| Metric points | 179,688 |

Fanout averaged 7.24 cores, with a peak RSS of 11.4 GiB. The generator host averaged 7.44 of 8 cores.

Rates are rounded to whole items/s. The sustained composition uses the same
counter-scrape interval as total accepted throughput.

Median peaks across the three repetitions:

| Workload | Median peak | Processes per signal | Range | Generator CPU (median) |
|---|---:|---:|---:|---:|
| Mixed | 402,062 | 32 | 357,956–433,339 | 7.34 / 8 |
| Traces (spans) | 248,901 | 32 | 207,920–295,566 | 5.19 / 8 |
| Logs (records) | 387,579 | 64 | 353,862–393,976 | 4.63 / 8 |
| Metrics (data points) | 524,878 | 64 | 472,496–561,000 | 5.31 / 8 |

| File | Contents |
|---|---|
| `report.md` | Human-readable report, every phase |
| `report.json` | Authoritative structured report |
| `samples.jsonl` | One-second resource and background-work samples |
| `summary.json` | Medians, ranges and the sustained composition derived from `report.json` |
| `fanout.log` | Fanout's log for the run |
| `fanout-repair.txt` | `fanout repair verify` output |

- **Release:** `fanout_v2026.10.8-alpha_linux_amd64.tar.gz`, SHA-256
  `b314920221693683b7d6ff42787a37e82bee1fbdd7ddcc8647d1905f7629ac6b`.
- **Harness:** commit `8f5864ba488d9802f68250d85f67b6658ab13af5`, clean.

Reproduce:

```sh
FANOUT_BENCH_HCLOUD_SSH_KEY='<key>' scripts/hetzner.sh \
  --fanout-version v2026.10.8-alpha --preset publish --repeat 3 \
  --output results/publish-v2026.10.8-alpha
```
