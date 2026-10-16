# Fanout v2026.10.7-alpha on two Hetzner ccx33s

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
| Total items | 422,408 |
| Spans | 51,358 |
| Log records | 184,883 |
| Metric points | 186,178 |

Fanout averaged 7.33 cores, with a peak RSS of 13.5 GiB. The generator host averaged 7.01 of 8 cores.

Median peaks across the three repetitions:

| Workload | Median peak | Processes per signal | Range | Generator CPU (median) |
|---|---:|---:|---:|---:|
| Mixed | 429,749 | 32 | 429,504–471,506 | 7.02 / 8 |
| Traces (spans) | 243,696 | 64 | 225,110–267,916 | 6.83 / 8 |
| Logs (records) | 342,494 | 64 | 299,902–347,721 | 3.46 / 8 |
| Metrics (data points) | 540,102 | 64 | 293,700–543,941 | 4.64 / 8 |

| File | Contents |
|---|---|
| `report.md` | Human-readable report, every phase |
| `report.json` | Authoritative structured report |
| `samples.jsonl` | One-second resource and background-work samples |
| `summary.json` | Medians, ranges and the sustained composition derived from `report.json` |
| `fanout.log` | Fanout's log for the run |
| `fanout-repair.txt` | `fanout repair verify` output |

- **Release:** `fanout_v2026.10.7-alpha_linux_amd64.tar.gz`, SHA-256
  `d1b602130463b38a03f7575c52c1c2178213839dbf276294abd57ce662c8756b`.
- **Harness:** commit `8a2a28aafa828fc7967ba82a481c7cdb5c7e921e`, clean.

Reproduce:

```sh
FANOUT_BENCH_HCLOUD_SSH_KEY='<key>' scripts/hetzner.sh \
  --fanout-version v2026.10.7-alpha --preset publish --repeat 3 \
  --output results/publish-v2026.10.7-alpha
```
