package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureReport() Report {
	return Report{SchemaVersion: SchemaVersion, RunID: "test", StartedAt: time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC), Build: Build{HarnessVersion: "v1", HarnessCommit: "abc", TelemetrygenVersion: "v0.157.0", FanoutVersion: "v2026.10.1-alpha"}, Host: Host{OS: "linux", Architecture: "amd64", CPUModel: "AMD EPYC", LogicalCPUs: 8, MemoryBytes: 32 << 30, GeneratorLocal: true}, Settings: Settings{Preset: "quick", Transport: "plaintext gRPC", BatchSize: 1000, SampleSeconds: 1, Methodology: "accepted"}, WorkloadSHA256: strings.Repeat("a", 64), Trials: []Trial{{Spec: PhaseSpec{Name: "traces", Signal: Traces, Kind: "sweep", ProcessesPerSignal: 1, WorkersPerProcess: 2}, ElapsedSeconds: 5, OfferedPerSecond: 2000, AcceptedPerSecond: 1900, AcceptanceRatio: .95, Accepted: Counts{Spans: 9500}, Resources: Resources{AverageCPUCores: 1.2, PeakRSSBytes: 100 << 20}, Classification: "target-sustained"}}, Summary: Summary{PeakTraces: Peak{"traces", 2000, 1900, 1.2, 100 << 20}, Passed: true, Gates: []Gate{{"counters", true, "ok"}}, Warnings: []string{"synthetic"}}, ReproductionCommand: "FANOUT_BENCH_INGEST_TOKEN=… fanout-bench run"}
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
	got := reproductionCommand(Config{Preset: "quick", Endpoint: "localhost:7520", MetricsURL: "http://localhost", FanoutVersion: "x", Repeat: 1, QueryURL: "http://localhost", QueryRate: 5})
	if !strings.HasPrefix(got, "FANOUT_BENCH_INGEST_TOKEN=… FANOUT_BENCH_QUERY_COOKIE=… fanout-bench run") {
		t.Fatalf("bad command: %s", got)
	}
}

func TestRemoteReproductionCommandIncludesSSHConnection(t *testing.T) {
	got := reproductionCommand(Config{Host: "root@192.0.2.10", SSHKnownHosts: "/tmp/run/known_hosts", SSHPort: 2222, SSHIdentity: "/keys/bench key", Preset: "quick", FanoutVersion: "v2026.10.1-alpha", TelemetrygenVersion: "v0.157.0", Repeat: 1, BatchSize: 1000, SampleInterval: time.Second, Cooldown: time.Second})
	for _, want := range []string{"--host 'root@<host>'", "--ssh-port 2222", "--ssh-identity '/keys/bench key'"} {
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

func TestReproductionCommandRedactsAddress(t *testing.T) {
	for _, target := range []struct{ host, redacted string }{
		{"root@192.0.2.10", "root@<host>"},
		{"benchmark@example.test", "benchmark@<host>"},
		{"192.0.2.10", "<host>"},
		{"root@[2001:db8::1]", "root@<host>"},
	} {
		t.Run(target.host, func(t *testing.T) {
			report := fixtureReport()
			report.ReproductionCommand = reproductionCommand(Config{Host: target.host, SSHKnownHosts: "/tmp/run/known_hosts"})
			data, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), target.host) || strings.Contains(string(data), "ssh-known-hosts") || strings.Contains(string(data), "/tmp/run/known_hosts") {
				t.Fatalf("report retained address or run-scoped known-hosts: %s", data)
			}
			if !strings.Contains(report.ReproductionCommand, "--host "+shellQuote(target.redacted)) {
				t.Fatalf("missing redacted host: %s", report.ReproductionCommand)
			}
		})
	}
}

func TestHostDescriptionRecordedAndRendered(t *testing.T) {
	// Cancel before load: report initialization must still preserve host identity.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	description := "Hetzner Cloud ccx33, fsn1, ubuntu-24.04"
	generator := filepath.Join(t.TempDir(), "telemetrygen")
	if err := os.WriteFile(generator, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	runner := Runner{Config: Config{Host: "root@192.0.2.10", HostDescription: description, Telemetrygen: generator, Token: "test-token", Endpoint: "localhost:7520", MetricsURL: "http://localhost:7520/metrics", Preset: "quick", Repeat: 1, BatchSize: 1000, SampleInterval: time.Second, GeneratorTimeout: time.Second}}
	report, err := runner.Run(ctx)
	if err == nil || report == nil {
		t.Fatalf("expected partial report on cancellation, got report=%v err=%v", report, err)
	}
	dir := t.TempDir()
	if err := WriteArtifacts(dir, *report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Host struct {
			Description string `json:"description"`
		} `json:"host"`
	}
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Host.Description != description {
		t.Fatalf("recorded description=%q", recorded.Host.Description)
	}
	markdown, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(markdown), display(report.Host.CPUModel)+" · "+description) {
		t.Fatal("host description missing beside CPU model")
	}
}

func TestSplitHostPlacementAndDescriptionRecordedAndRendered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	managed := ManagedConfig{ProvisionedHost: "root@<host>", HostDescription: "Hetzner Cloud ccx33", GeneratorHostDescription: "Hetzner Cloud ccx43", GeneratorSSH: "root@10.77.0.3", GeneratorSSHIdentity: "/stage/key", TelemetrygenPath: "/remote/only/telemetrygen", Preset: "quick", Repeat: 1, BatchSize: 1000, SampleInterval: time.Second, TelemetrygenVersion: "v0.157.0"}
	cfg := managedRunnerConfig(managed, "10.77.0.2:12345", "/stage/data")
	cfg.Token = "test-token"
	runner := Runner{Config: cfg}
	report, err := runner.Run(ctx)
	if err == nil || report == nil {
		t.Fatalf("expected a partial report, got report=%v err=%v", report, err)
	}
	dir := t.TempDir()
	if err := WriteArtifacts(dir, *report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var recorded struct {
		Host struct {
			Description          string `json:"description"`
			GeneratorDescription string `json:"generator_description"`
			GeneratorLocal       bool   `json:"generator_on_target_host"`
		} `json:"host"`
	}
	if err := json.Unmarshal(data, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded.Host.GeneratorLocal || recorded.Host.Description != managed.HostDescription || recorded.Host.GeneratorDescription != managed.GeneratorHostDescription {
		t.Fatalf("host=%+v", recorded.Host)
	}
	markdown, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ccx33", "| Generator host | Hetzner Cloud ccx43 |", "| Generator placement | separate/remote |"} {
		if !strings.Contains(string(markdown), want) {
			t.Errorf("Markdown missing %q", want)
		}
	}
	loaded, err := LoadReport(filepath.Join(dir, "report.json"))
	if err != nil || loaded.Host.GeneratorDescription != managed.GeneratorHostDescription {
		t.Fatalf("roundtrip lost generator host: %+v, %v", loaded.Host, err)
	}
}

func TestReproductionCommandRedactsGeneratorHostAndDropsAddresses(t *testing.T) {
	for _, host := range []string{"root@192.0.2.11", "bench@generator.example", "root@[2001:db8::3]", "192.0.2.11"} {
		cfg := Config{Host: "root@192.0.2.10", GeneratorHost: host, GeneratorAddress: "10.77.0.3", FanoutListen: "10.77.0.2", GeneratorSSH: "root@10.77.0.3", GeneratorSSHIdentity: "/stage/key", GeneratorSSHKnownHosts: "/stage/known_hosts", GeneratorHostDescription: "Hetzner ccx43"}
		got := reproductionCommand(cfg)
		for _, want := range []string{"--host 'root@<host>'", "--generator-host 'root@<generator-host>'", "--generator-address '<generator-address>'", "--fanout-listen '<fanout-listen>'", "--generator-host-description 'Hetzner ccx43'"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in %s", want, got)
			}
		}
		for _, unwanted := range []string{host, "192.0.2.10", cfg.GeneratorAddress, cfg.FanoutListen, "--generator-ssh", "/stage/"} {
			if strings.Contains(got, unwanted) {
				t.Errorf("unexpected %q in %s", unwanted, got)
			}
		}
	}
}
