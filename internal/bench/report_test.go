package bench

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureReport() Report {
	return Report{SchemaVersion: SchemaVersion, RunID: "test", StartedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC), Build: Build{HarnessVersion: "v1", HarnessCommit: "abc", TelemetrygenVersion: "v0.157.0", FanoutVersion: "v2026.8.11"}, Host: Host{OS: "darwin", Architecture: "arm64", CPUModel: "M3 Max", LogicalCPUs: 14, MemoryBytes: 36 << 30, GeneratorLocal: true}, Settings: Settings{Preset: "quick", Transport: "plaintext gRPC", BatchSize: 1000, SampleSeconds: 1, Methodology: "accepted"}, WorkloadSHA256: strings.Repeat("a", 64), Trials: []Trial{{Spec: PhaseSpec{Name: "traces", Signal: Traces, Kind: "sweep", ProcessesPerSignal: 1, WorkersPerProcess: 2}, ElapsedSeconds: 5, OfferedPerSecond: 2000, AcceptedPerSecond: 1900, AcceptanceRatio: .95, Accepted: Counts{Spans: 9500}, Resources: Resources{AverageCPUCores: 1.2, PeakRSSBytes: 100 << 20}, Classification: "target-sustained"}}, Summary: Summary{PeakTraces: Peak{"traces", 2000, 1900, 1.2, 100 << 20}, Passed: true, Gates: []Gate{{"counters", true, "ok"}}, Warnings: []string{"synthetic"}}, ReproductionCommand: "FANOUT_BENCH_INGEST_TOKEN=… fanout-bench run"}
}

func TestArtifactsAreCompleteAndContainNoSecrets(t *testing.T) {
	dir := t.TempDir()
	report := fixtureReport()
	if err := WriteArtifacts(dir, report); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"report.json", "report.md", "samples.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"Bearer ", "session="} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("report leaked %q", secret)
		}
	}
	loaded, err := LoadReport(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != "test" {
		t.Fatalf("run ID=%q", loaded.RunID)
	}
}

func TestMarkdownExplainsAcceptedRowsAndLimits(t *testing.T) {
	got := Markdown(fixtureReport())
	for _, want := range []string{"Fanout accepted/s", "fanout_ingest_rows_total", "Complete phase breakdown", "Quality gates", "Methodology and limits", "same host"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestReproductionCommandPutsSecretEnvironmentFirst(t *testing.T) {
	got := reproductionCommand(Config{Preset: "quick", Endpoint: "localhost:4317", MetricsURL: "http://localhost", FanoutVersion: "x", Repeat: 1, QueryURL: "http://localhost", QueryRate: 5})
	if !strings.HasPrefix(got, "FANOUT_BENCH_INGEST_TOKEN=… FANOUT_BENCH_QUERY_COOKIE=… fanout-bench run") {
		t.Fatalf("bad command: %s", got)
	}
}

func TestRemoteReproductionCommandIncludesSSHConnection(t *testing.T) {
	got := reproductionCommand(Config{Host: "root@bench", SSHPort: 2222, SSHIdentity: "/keys/bench key", Preset: "quick", FanoutVersion: "v1", TelemetrygenVersion: "v2", Repeat: 1, BatchSize: 1000, SampleInterval: time.Second, Cooldown: time.Second})
	for _, want := range []string{"--host 'root@bench'", "--ssh-port 2222", "--ssh-identity '/keys/bench key'"} {
		if !strings.Contains(got, want) {
			t.Errorf("command missing %q: %s", want, got)
		}
	}
}

func TestFormatNumberGroupsNegativeValues(t *testing.T) {
	for input, want := range map[float64]string{-123456: "-123,456", -1234: "-1,234", -999: "-999", 1234567: "1,234,567", 0: "0"} {
		if got := formatNumber(input); got != want {
			t.Fatalf("formatNumber(%v)=%q; want %q", input, got, want)
		}
	}
}

// A trial round-tripped from a truncated or hand-edited report can carry a zero
// duration. Rendering must not print +Inf into the published tables.
func TestRenderedRatesSurviveZeroElapsedTrial(t *testing.T) {
	report := fixtureReport()
	report.Trials[0].ElapsedSeconds = 0
	report.Trials[0].Spec.QueryRate = 10
	report.Trials[0].Spec.Signal = Mixed
	report.Trials[0].QueryLatency = Latency{Count: 5}
	report.Settings.QueryRate = 10
	markdown := Markdown(report)
	var console strings.Builder
	printTrialLine(&console, report.Trials[0])
	for name, text := range map[string]string{"markdown": markdown, "console": console.String()} {
		if strings.Contains(text, "Inf") || strings.Contains(text, "NaN") {
			t.Fatalf("%s output contains a non-finite rate", name)
		}
	}
}
