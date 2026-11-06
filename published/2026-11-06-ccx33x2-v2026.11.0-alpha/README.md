# Fanout v2026.11.0-alpha on two Hetzner ccx33s

Fanout ran alone on a Hetzner Cloud ccx33 (8 dedicated AMD EPYC-Milan vCPUs,
32 GB RAM, fsn1, Ubuntu 24.04). telemetrygen v0.157.0 ran on a separate ccx33
in the same location, sending plaintext OTLP/gRPC over a private network. The
run used the publish preset with three repetitions: sweeps of 1 to 64
telemetrygen processes per signal, 8 workers per process, 1,000-item batches,
then five minutes of mixed load at 64 processes per signal. Ingest only. Reads under sustained ingest did not meet the read gate and are tracked in labstack/fanout#419.

Every evidence gate passed: all generator processes exited successfully,
Fanout dropped no rows and did not restart, and `fanout repair verify`
validated the Parquet store.

| Sustained mixed, five minutes | Accepted/s |
|---|---:|
| Total items | 507,311 |
| Spans | 61,571 |
| Log records | 224,112 |
| Metric points | 221,628 |

Fanout averaged 7.73 cores, with a peak RSS of 10.6 GiB. The generator host averaged 7.93 of 8 cores.

The sustained phase is classified generator-limited.

Rates are rounded to whole items/s. The sustained composition uses the same
counter-scrape interval as total accepted throughput.

Median peaks across the three repetitions:

| Workload | Median peak | Processes per signal | Range | Generator CPU (median) |
|---|---:|---:|---:|---:|
| Mixed | 460,640 | 64 | 453,181–485,193 | 7.39 / 8 |
| Traces (spans) | 297,345 | 64 | 192,120–330,450 | 7.24 / 8 |
| Logs (records) | 622,229 | 64 | 558,932–648,672 | 6.38 / 8 |
| Metrics (data points) | 488,431 | 64 | 486,759–532,625 | 4.71 / 8 |

| File | Contents |
|---|---|
| `report.md` | Human-readable report, every phase |
| `report.json` | Authoritative structured report |
| `samples.jsonl` | One-second resource and background-work samples |
| `summary.json` | Medians, ranges and the sustained composition derived from `report.json` |
| `fanout.log` | Fanout's log for the run |
| `fanout-repair.txt` | `fanout repair verify` output |

- **Release:** `fanout_v2026.11.0-alpha_linux_amd64.tar.gz`, SHA-256
  `1e83657b9eb0ff7e7ad406bd5162642badf087c380448deebd1f88b183656ddc`.
- **Harness:** commit `5462cbbcde9c49521c918c45f45312493aa4bb24`, clean.

The published reproduction commands replace the local SSH-key path with
`<ssh-identity>`; the source log already redacts its setup token as `[REDACTED]`.

Reproduce:

```sh
FANOUT_BENCH_HCLOUD_SSH_KEY='<key>' scripts/hetzner.sh \
  --fanout-version v2026.11.0-alpha --preset publish --repeat 3 \
  --output results/publish-v2026.11.0-alpha
```
