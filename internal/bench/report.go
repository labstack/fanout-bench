package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

func WriteArtifacts(dir string, report Report) error {
	if dir == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	jsonData, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	jsonData = append(jsonData, '\n')
	if err := atomicWrite(filepath.Join(dir, "report.json"), jsonData); err != nil {
		return err
	}
	if err := atomicWrite(filepath.Join(dir, "report.md"), []byte(Markdown(report))); err != nil {
		return err
	}
	var raw bytes.Buffer
	encoder := json.NewEncoder(&raw)
	for _, trial := range report.Trials {
		for _, sample := range trial.Samples {
			if err := encoder.Encode(struct {
				Trial string `json:"trial"`
				ResourceSample
			}{trial.Spec.Name, sample}); err != nil {
				return err
			}
		}
	}
	return atomicWrite(filepath.Join(dir, "samples.jsonl"), raw.Bytes())
}

func LoadReport(path string) (Report, error) {
	file, err := os.Open(path)
	if err != nil {
		return Report{}, err
	}
	defer file.Close()
	var report Report
	decoder := json.NewDecoder(io.LimitReader(file, 128<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return Report{}, err
	}
	if report.SchemaVersion != SchemaVersion {
		return Report{}, fmt.Errorf("unsupported schema %q", report.SchemaVersion)
	}
	return report, nil
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fanout-bench-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func printRunHeader(w io.Writer, r *Report, plan []PhaseSpec) {
	minutes := 0.0
	for _, phase := range plan {
		minutes += phase.Duration.Seconds() / 60
	}
	fmt.Fprintln(w, "╭────────────────────────────────────────────────────────────────────╮")
	fmt.Fprintln(w, "│  FANOUT BENCH  •  accepted-row performance, with evidence          │")
	fmt.Fprintln(w, "╰────────────────────────────────────────────────────────────────────╯")
	t := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintf(t, "  Target\t%s  (%s)\n", r.Settings.Endpoint, r.Settings.Transport)
	fmt.Fprintf(t, "  Fanout\t%s  commit=%s\n", display(r.Build.FanoutVersion), display(r.Build.FanoutCommit))
	fmt.Fprintf(t, "  Generator\ttelemetrygen %s  same-host=%t\n", display(r.Build.TelemetrygenVersion), r.Host.GeneratorLocal)
	fmt.Fprintf(t, "  Machine\t%s/%s  %s  %d logical CPUs  %s RAM\n", r.Host.OS, r.Host.Architecture, display(r.Host.CPUModel), r.Host.LogicalCPUs, formatBytes(float64(r.Host.MemoryBytes)))
	fmt.Fprintf(t, "  Suite\t%s  repeat=%d  %d phases  ~%.1f min load time\n", r.Settings.Preset, r.Settings.Repeat, len(plan), minutes)
	fmt.Fprintf(t, "  Evidence\tFanout counters @ %s  samples every %.1fs\n", r.Settings.MetricsURL, r.Settings.SampleSeconds)
	fmt.Fprintf(t, "  Workload ID\t%s\n", r.WorkloadSHA256[:16])
	t.Flush()
}

func printTrialLine(w io.Writer, t Trial) {
	fmt.Fprintf(w, "     %s accepted %s/s  ratio %5.1f%%  CPU %.2f cores  peak RSS %s  %s\n",
		resultIcon(t.Classification), formatNumber(t.AcceptedPerSecond), t.AcceptanceRatio*100,
		t.Resources.AverageCPUCores, formatBytes(t.Resources.PeakRSSBytes), t.Classification)
	if t.QueryLatency.Count > 0 {
		fmt.Fprintf(w, "       reads %.1f/s  attempted %s  p50 %.1fms  p95 %.1fms  p99 %.1fms  errors %d  shed %d\n", perSecond(float64(t.QueryLatency.Count), t.ElapsedSeconds), formatNumber(float64(t.QueryAttempts)), t.QueryLatency.P50Ms, t.QueryLatency.P95Ms, t.QueryLatency.P99Ms, t.QueryFailures, t.QueryShed)
	}
}

func printSummary(w io.Writer, r Report) {
	fmt.Fprintln(w, "\n╭─ RESULTS ───────────────────────────────────────────────────────────╮")
	t := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	fmt.Fprintln(t, "  Workload\tAccepted/s\tOffered/s\tAvg CPU\tPeak RSS")
	for _, row := range []struct {
		name string
		peak Peak
	}{{"Traces", r.Summary.PeakTraces}, {"Logs", r.Summary.PeakLogs}, {"Metric points", r.Summary.PeakMetrics}, {"Mixed peak", r.Summary.PeakMixed}, {"Mixed sustained", r.Summary.SustainedMixed}} {
		fmt.Fprintf(t, "  %s\t%s\t%s\t%.2f cores\t%s\n", row.name, formatNumber(row.peak.AcceptedPerSecond), formatNumber(row.peak.OfferedPerSecond), row.peak.AverageCPUCores, formatBytes(row.peak.PeakRSSBytes))
	}
	t.Flush()
	fmt.Fprintln(w, "├─ QUALITY GATES ─────────────────────────────────────────────────────┤")
	for _, gate := range r.Summary.Gates {
		fmt.Fprintf(w, "  %s %-24s %s\n", gateIcon(gate.Passed), gate.Name, gate.Detail)
	}
	verdict := "PASS — report is internally valid"
	if !r.Summary.Passed {
		verdict = "FAIL — do not publish this run"
	}
	fmt.Fprintf(w, "╰─ %s ─────────────────────────────────────────────────────╯\n", verdict)
}

func Markdown(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Fanout performance report\n\n")
	fmt.Fprintf(&b, "> **%s** — %s · `%s`\n\n", map[bool]string{true: "VALID", false: "INVALID"}[r.Summary.Passed], r.StartedAt.Format("2 Jan 2006 15:04 MST"), r.RunID)
	fmt.Fprintf(&b, "## Headline results\n\n")
	fmt.Fprintf(&b, "| Workload | Fanout accepted/s | Offered/s | Avg Fanout CPU | Peak RSS |\n|---|---:|---:|---:|---:|\n")
	for _, row := range []struct {
		name string
		peak Peak
	}{{"Traces (spans)", r.Summary.PeakTraces}, {"Logs (records)", r.Summary.PeakLogs}, {"Metrics (data points)", r.Summary.PeakMetrics}, {"Mixed peak", r.Summary.PeakMixed}, {"Mixed sustained", r.Summary.SustainedMixed}} {
		fmt.Fprintf(&b, "| %s | %s | %s | %.2f cores | %s |\n", row.name, formatNumber(row.peak.AcceptedPerSecond), formatNumber(row.peak.OfferedPerSecond), row.peak.AverageCPUCores, formatBytes(row.peak.PeakRSSBytes))
	}
	fmt.Fprintf(&b, "\n**Meaning:** accepted throughput is the delta of `fanout_ingest_rows_total` divided by the interval between the two counter scrapes that bracket the phase. It is not telemetrygen's requested or locally generated count.\n\n")
	for _, t := range r.Trials {
		if t.Spec.Kind == "sustained" {
			fmt.Fprintf(&b, "**Sustained mixed topology:** %d processes per signal, %d workers per process.\n\n", t.Spec.ProcessesPerSignal, t.Spec.WorkersPerProcess)
		}
	}
	fmt.Fprintf(&b, "## Test identity\n\n| Setting | Value |\n|---|---|\n")
	machine := display(r.Host.CPUModel)
	if r.Host.Description != "" {
		machine += " · " + r.Host.Description
	}
	settings := [][2]string{{"Fanout version", display(r.Build.FanoutVersion)}, {"Fanout commit", display(r.Build.FanoutCommit)}, {"Fanout artifact", display(r.Build.FanoutArtifact)}, {"Harness", r.Build.HarnessVersion + " (`" + r.Build.HarnessCommit + "`)"}, {"telemetrygen", display(r.Build.TelemetrygenVersion)}, {"Machine", fmt.Sprintf("%s/%s · %s · %d logical CPUs · %s RAM", r.Host.OS, r.Host.Architecture, machine, r.Host.LogicalCPUs, formatBytes(float64(r.Host.MemoryBytes)))}, {"Generator host", display(r.Host.GeneratorDescription)}, {"Generator placement", map[bool]string{true: "same host as Fanout", false: "separate/remote"}[r.Host.GeneratorLocal]}, {"Preset", r.Settings.Preset}, {"Transport", r.Settings.Transport}, {"Batch size", fmt.Sprintf("%d", r.Settings.BatchSize)}, {"Trace shape", "1 parent + 1 child span per telemetrygen trace tick"}, {"Repeat", fmt.Sprintf("%d", r.Settings.Repeat)}, {"Sample interval", fmt.Sprintf("%.1fs", r.Settings.SampleSeconds)}, {"Workload SHA-256", "`" + r.WorkloadSHA256 + "`"}}
	for _, row := range settings {
		fmt.Fprintf(&b, "| %s | %s |\n", row[0], row[1])
	}
	fmt.Fprintf(&b, "\n## Complete phase breakdown\n\n| Phase | Signal | Topology (processes per signal × workers) | Duration | Offered/s | Accepted/s | Ratio | Fanout CPU cores | Generator CPU cores / logical CPUs | Peak RSS | Queue peak | Result |\n|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, t := range r.Trials {
		generatorCPU := "n/a"
		if t.GeneratorCPU != nil {
			generatorCPU = fmt.Sprintf("%.2f / %d", t.GeneratorCPU.AverageCPUCores, t.GeneratorCPU.LogicalCPUs)
		}
		fmt.Fprintf(&b, "| %s | %s | %d×%d workers | %.1fs | %s | %s | %.1f%% | %.2f | %s | %s | %.0f | %s |\n", t.Spec.Name, t.Spec.Signal, t.Spec.ProcessesPerSignal, t.Spec.WorkersPerProcess, t.ElapsedSeconds, formatNumber(t.OfferedPerSecond), formatNumber(t.AcceptedPerSecond), t.AcceptanceRatio*100, t.Resources.AverageCPUCores, generatorCPU, formatBytes(t.Resources.PeakRSSBytes), t.Resources.PeakQueueDepth, t.Classification)
	}
	fmt.Fprintf(&b, "\n## Quality gates\n\n")
	for _, gate := range r.Summary.Gates {
		fmt.Fprintf(&b, "- %s **%s:** %s\n", map[bool]string{true: "✅", false: "❌"}[gate.Passed], gate.Name, gate.Detail)
	}
	if r.Settings.QueryRate > 0 {
		fmt.Fprintf(&b, "\n## Read-under-write latency\n\nThe mixed phases used a bounded open-loop scheduler targeting %.0f authenticated user operations/s across System overview dashboard refresh (batched panels and annotations), exemplar drill-down, and trace detail. Latency covers every request in an operation; operation kinds and their counts/latencies appear in the methodology.\n\n", r.Settings.QueryRate)
		fmt.Fprintf(&b, "| Phase | Target/s | Completed/s | Attempted | p50 | p95 | p99 | Errors | Shed | Canceled at end |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, t := range r.Trials {
			if t.Spec.Signal != Mixed || t.Spec.QueryRate <= 0 {
				continue
			}
			fmt.Fprintf(&b, "| %s | %.0f | %.1f | %s | %.1fms | %.1fms | %.1fms | %d | %d | %d |\n", t.Spec.Name, t.Spec.QueryRate, perSecond(float64(t.QueryLatency.Count), t.ElapsedSeconds), formatNumber(float64(t.QueryAttempts)), t.QueryLatency.P50Ms, t.QueryLatency.P95Ms, t.QueryLatency.P99Ms, t.QueryFailures, t.QueryShed, t.QueryCanceled)
		}
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "\n## Methodology and limits\n\n%s\n\n", r.Settings.Methodology)
	for _, warning := range r.Summary.Warnings {
		fmt.Fprintf(&b, "- %s\n", warning)
	}
	fmt.Fprintf(&b, "\n## Reproduce\n\n```sh\n%s\n```\n\nRaw evidence: `report.json` (authoritative structured report) and `samples.jsonl` (time-series resource samples, including cumulative `rollup_seconds`, `write_gate_hold_seconds`, `write_gate_wait_seconds`, `compaction_seconds_total`, `compactions_total`, `publish_wait_seconds_total`, and `ingest_backpressure_seconds_total`, plus the current `compaction_backlog_batches` gauge when available).\n", r.ReproductionCommand)
	return b.String()
}

func reproductionCommand(c Config) string {
	command := reproductionRunCommand(c)
	if c.HostDescription != "" {
		command += " --host-description " + shellQuote(c.HostDescription)
	}
	if c.GeneratorHostDescription != "" {
		command += " --generator-host-description " + shellQuote(c.GeneratorHostDescription)
	}
	return command
}

func redactedHost(host string) string {
	if user, _, ok := strings.Cut(host, "@"); ok {
		return user + "@<host>"
	}
	return "<host>"
}

func reproductionRunCommand(c Config) string {
	if c.Host != "" {
		parts := []string{"fanout-bench run", "--host " + shellQuote(redactedHost(c.Host)), "--fanout-version " + shellQuote(c.FanoutVersion), "--preset " + c.Preset, "--repeat " + fmt.Sprint(c.Repeat), "--telemetrygen-version " + shellQuote(c.TelemetrygenVersion), "--batch-size " + fmt.Sprint(c.BatchSize), "--sample-interval " + c.SampleInterval.String(), "--cooldown " + c.Cooldown.String()}
		if c.GeneratorHost != "" {
			parts = append(parts, "--generator-host 'root@<generator-host>'", "--generator-address '<generator-address>'", "--fanout-listen '<fanout-listen>'")
		}
		if c.SSHPort != 0 && c.SSHPort != 22 {
			parts = append(parts, "--ssh-port "+fmt.Sprint(c.SSHPort))
		}
		if c.SSHIdentity != "" {
			parts = append(parts, "--ssh-identity "+shellQuote(c.SSHIdentity))
		}
		if c.QueryRate > 0 {
			parts = append(parts, "--query-rate "+fmt.Sprint(c.QueryRate))
		}
		if c.ProcessOverride > 0 {
			parts = append(parts, "--processes "+fmt.Sprint(c.ProcessOverride))
		}
		if c.WorkerOverride > 0 {
			parts = append(parts, "--workers "+fmt.Sprint(c.WorkerOverride))
		}
		if c.RateOverride > 0 {
			parts = append(parts, "--rate "+fmt.Sprint(c.RateOverride))
		}
		if c.DurationOverride > 0 {
			parts = append(parts, "--duration "+c.DurationOverride.String())
		}
		return strings.Join(parts, " ")
	}
	env := []string{"FANOUT_BENCH_INGEST_TOKEN=…"}
	parts := []string{"fanout-bench run", "--preset " + c.Preset, "--endpoint " + shellQuote(c.Endpoint), "--metrics-url " + shellQuote(c.MetricsURL), "--fanout-version " + shellQuote(c.FanoutVersion), "--repeat " + fmt.Sprint(c.Repeat)}
	if c.Insecure {
		parts = append(parts, "--insecure")
	}
	if c.SameHost {
		parts = append(parts, "--same-host")
	}
	if c.FanoutCommit != "" {
		parts = append(parts, "--fanout-commit "+shellQuote(c.FanoutCommit))
	}
	if c.FanoutArtifact != "" {
		parts = append(parts, "--fanout-artifact "+shellQuote(c.FanoutArtifact))
	}
	if c.QueryRate > 0 {
		env = append(env, "FANOUT_BENCH_QUERY_COOKIE=…")
		parts = append(parts, "--query-url "+shellQuote(c.QueryURL), "--query-rate "+fmt.Sprint(c.QueryRate))
	}
	return strings.Join(append(env, parts...), " ")
}

func shellQuote(v string) string { return "'" + strings.ReplaceAll(v, "'", "'\\''") + "'" }
func phaseIcon(kind string) string {
	if kind == "warmup" {
		return "◌"
	}
	if kind == "sustained" {
		return "◆"
	}
	return "◇"
}
func resultIcon(class string) string {
	if class == "invalid" {
		return "✗"
	}
	if class == "target-sustained" {
		return "✓"
	}
	return "△"
}
func gateIcon(ok bool) string {
	if ok {
		return "✓"
	}
	return "✗"
}
func display(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}
func formatNumber(v float64) string {
	n := fmt.Sprintf("%.0f", v)
	sign := ""
	if strings.HasPrefix(n, "-") {
		sign, n = "-", n[1:]
	}
	for i := len(n) - 3; i > 0; i -= 3 {
		n = n[:i] + "," + n[i:]
	}
	return sign + n
}
func formatBytes(v float64) string {
	if v <= 0 {
		return "n/a"
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}
