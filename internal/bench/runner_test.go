package bench

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunPhaseStopsQueriesBeforeFinalGeneratorCPURead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var dispatched atomic.Int64
	firstQuery := make(chan struct{}, 1)
	scrapes := 0
	r := &Runner{
		Config:       Config{GeneratorSSH: "unused", MetricsURL: "http://bench/metrics", QueryURL: "http://bench", SampleInterval: time.Hour},
		queryTargets: newQueryRun().targets,
		client: &http.Client{Transport: releaseTransport(func(req *http.Request) (*http.Response, error) {
			body := `{"results":[{"status":"empty"}]}`
			if req.URL.Path == "/metrics" {
				scrapes++
				if scrapes == 2 {
					// Keep the phase open until a query is in flight.
					select {
					case <-firstQuery:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				body = "fanout_rows_accepted_total{signal=\"logs\"} 1\n"
			} else {
				dispatched.Add(1)
				select {
				case firstQuery <- struct{}{}:
				default:
				}
				select {
				case <-time.After(20 * time.Millisecond):
				case <-req.Context().Done():
					return nil, req.Context().Err()
				}
			}
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		})},
	}
	cpuReads := 0
	var beforeCPURead, afterCPURead int64
	r.readGeneratorCPU = func(ctx context.Context) (cpuStat, error) {
		cpuReads++
		if cpuReads == 2 {
			beforeCPURead = dispatched.Load()
			select {
			case <-time.After(100 * time.Millisecond):
			case <-ctx.Done():
				return cpuStat{}, ctx.Err()
			}
			afterCPURead = dispatched.Load()
		}
		return cpuStat{total: uint64(cpuReads * 100), idle: uint64(cpuReads * 50), logicalCPUs: 1}, nil
	}
	// No generator processes are needed to exercise the phase boundary.
	trial, err := r.runPhase(ctx, PhaseSpec{Signal: Logs, QueryRate: 100}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cpuReads != 2 || beforeCPURead == 0 {
		t.Fatalf("CPU reads=%d queries before final read=%d; workload was not exercised", cpuReads, beforeCPURead)
	}
	if afterCPURead != beforeCPURead {
		t.Fatalf("queries dispatched during final CPU read: %d", afterCPURead-beforeCPURead)
	}
	if trial.QueryLatency.Count != dispatched.Load() || trial.QueryFailures != 0 || trial.QueryCanceled != 0 {
		t.Fatalf("in-flight queries did not drain: dispatched=%d completed=%d failures=%d canceled=%d", dispatched.Load(), trial.QueryLatency.Count, trial.QueryFailures, trial.QueryCanceled)
	}
}

func TestGeneratorCommandWrapsSSHAndQuotesEveryArgument(t *testing.T) {
	cfg := Config{Telemetrygen: "/stage/generator's binary", GeneratorSSH: "root@10.77.0.3", GeneratorSSHPort: 2222, GeneratorSSHIdentity: "/stage/key with spaces", GeneratorSSHKnownHosts: "/stage/known hosts", GeneratorTimeout: 30 * time.Second}
	args := []string{"logs", "--otlp-header", "Authorization=\"Bearer token's $(touch /bad); value\""}
	cmd, cancel := generatorCommand(context.Background(), cfg, 5*time.Second, args...)
	defer cancel()
	want := []string{"ssh", "-p", "2222", "-i", "/stage/key with spaces", "-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "UserKnownHostsFile=/stage/known hosts", "root@10.77.0.3", "--",
		"'exec' 'timeout' '--signal=TERM' '45s' '/stage/generator'\\''s binary' 'logs' '--otlp-header' 'Authorization=\"Bearer token'\\''s $(touch /bad); value\"'"}
	if !slices.Equal(cmd.Args, want) {
		t.Fatalf("args=%q\nwant=%q", cmd.Args, want)
	}
	probe, cancelProbe := generatorCommand(context.Background(), cfg, 0, "--version")
	defer cancelProbe()
	if !slices.Equal(probe.Args[:len(probe.Args)-1], cmd.Args[:len(cmd.Args)-1]) {
		t.Fatalf("version probe SSH options differ: %q", probe.Args)
	}
	if got := probe.Args[len(probe.Args)-1]; got != "'exec' 'timeout' '--signal=TERM' '40s' '/stage/generator'\\''s binary' '--version'" {
		t.Fatalf("version command=%s", got)
	}
	cfg.GeneratorSSHPort = 0
	defaultPort, cancelDefault := generatorCommand(context.Background(), cfg, 0, "--version")
	defer cancelDefault()
	if defaultPort.Args[1] != "-p" || defaultPort.Args[2] != "22" {
		t.Fatalf("default port missing: %q", defaultPort.Args)
	}
	cfg.GeneratorSSH = ""
	local, cancelLocal := generatorCommand(context.Background(), cfg, 5*time.Second, args...)
	defer cancelLocal()
	if !slices.Equal(local.Args, append([]string{cfg.Telemetrygen}, args...)) {
		t.Fatalf("same-host command changed: %q", local.Args)
	}
}

func TestGeneratorCommandClientDeadline(t *testing.T) {
	if os.Getenv("FANOUT_BENCH_TEST_SILENT_CHILD") == "1" {
		time.Sleep(time.Minute)
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, probe := range []bool{false, true} {
		name := "launch"
		if probe {
			name = "version"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx, stop := context.WithTimeout(context.Background(), 20*time.Second)
			defer stop()
			args, duration := []string{"logs"}, time.Millisecond
			if probe {
				args, duration = []string{"--version"}, 0
			}
			cmd, cancel := generatorCommand(ctx, Config{GeneratorSSH: "root@generator", Telemetrygen: "/telemetrygen"}, duration, args...)
			defer cancel()
			// Exercise the client's context with a silent local child; never run SSH.
			cmd.Path = executable
			cmd.Args = []string{executable, "-test.run=^TestGeneratorCommandClientDeadline$"}
			cmd.Env = append(os.Environ(), "FANOUT_BENCH_TEST_SILENT_CHILD=1")
			started := time.Now()
			if err := cmd.Run(); err == nil {
				t.Fatal("silent child escaped the client deadline")
			}
			elapsed := time.Since(started)
			if elapsed < 14*time.Second || elapsed > 18*time.Second || ctx.Err() != nil {
				t.Fatalf("client deadline took %s, parent error=%v; want remote limit + 5s", elapsed, ctx.Err())
			}
		})
	}
}

func TestGeneratorArgsMatchPinnedTelemetrygenCLI(t *testing.T) {
	cfg := Config{Endpoint: "localhost:7520", Token: "token with space", GeneratorTimeout: 30, Insecure: true}
	spec := PhaseSpec{Duration: 5, RatePerWorker: 2500, WorkersPerProcess: 8, BatchSize: 1000}
	args := generatorArgs(cfg, Traces, spec)
	for _, want := range []string{"--otlp-header", "Authorization=\"Bearer token with space\"", "--child-spans", "1", "--otlp-insecure"} {
		if !slices.Contains(args, want) {
			t.Errorf("args missing %q: %#v", want, args)
		}
	}
	if strings.Contains(strings.Join(args, " "), "--header ") {
		t.Fatal("obsolete telemetrygen header flag")
	}
}

func TestClassifySeparatesValidityFromSaturation(t *testing.T) {
	tests := []struct {
		trial Trial
		want  string
	}{
		{Trial{Accepted: Counts{Spans: 100}, AcceptanceRatio: 1}, "target-sustained"},
		{Trial{Accepted: Counts{Spans: 90}, AcceptanceRatio: .9}, "near-saturation"},
		{Trial{Accepted: Counts{Spans: 10}, AcceptanceRatio: .1}, "saturated-or-generator-limited"},
		{Trial{Accepted: Counts{Spans: 100}, AcceptanceRatio: 1, RowsDropped: 1}, "invalid"},
	}
	for _, tt := range tests {
		got, _ := classify(tt.trial, nil)
		if got != tt.want {
			t.Errorf("classify(%+v)=%q want %q", tt.trial, got, tt.want)
		}
	}
}

func TestClassifySplitGeneratorCPUAndSweepPlateau(t *testing.T) {
	baseline := Trial{Spec: PhaseSpec{Name: "logs-p16-r2", Kind: "sweep", Signal: Logs, Repetition: 2, ProcessesPerSignal: 16}, AcceptedPerSecond: 100, Classification: "target-sustained"}
	for _, tc := range []struct {
		name   string
		cores  float64
		rate   float64
		ratio  float64
		change func(*Trial, *Trial)
		want   string
	}{
		{name: "90 percent boundary", cores: 14.4, rate: 200, ratio: .5, want: "generator-limited"},
		{name: "CPU limit precedes plateau", cores: 16, rate: 100, ratio: .5, want: "generator-limited"},
		{name: "plateau below CPU threshold", cores: 14.39, rate: 109.99, ratio: .5, want: "saturated"},
		{name: "declining throughput", cores: 8, rate: 80, ratio: .5, want: "saturated"},
		{name: "exactly ten percent gain", cores: 8, rate: 110, ratio: .5, want: "saturated-or-generator-limited"},
		{name: "still scaling", cores: 8, rate: 190, ratio: .5, want: "saturated-or-generator-limited"},
		{name: "target unchanged despite CPU", cores: 16, rate: 100, ratio: .95, want: "target-sustained"},
		{name: "target unchanged despite plateau", cores: 8, rate: 100, ratio: .95, want: "target-sustained"},
		{name: "CPU before near saturation", cores: 16, rate: 200, ratio: .9, want: "generator-limited"},
		{name: "plateau before near saturation", cores: 8, rate: 100, ratio: .9, want: "saturated"},
		{name: "same host fallback", rate: 100, ratio: .5, change: func(t, _ *Trial) { t.GeneratorCPU = nil }, want: "saturated-or-generator-limited"},
		{name: "first process count", cores: 8, rate: 100, ratio: .5, change: func(t, _ *Trial) { t.Spec.ProcessesPerSignal = 1 }, want: "saturated-or-generator-limited"},
		{name: "other signal", cores: 8, rate: 100, ratio: .5, change: func(_, p *Trial) { p.Spec.Signal = Traces }, want: "saturated-or-generator-limited"},
		{name: "other repetition", cores: 8, rate: 100, ratio: .5, change: func(_, p *Trial) { p.Spec.Repetition = 1 }, want: "saturated-or-generator-limited"},
		{name: "warmup is not baseline", cores: 8, rate: 100, ratio: .5, change: func(_, p *Trial) { p.Spec.Kind = "warmup" }, want: "saturated-or-generator-limited"},
		{name: "sustained has no plateau comparison", cores: 8, rate: 100, ratio: .5, change: func(t, _ *Trial) { t.Spec.Kind = "sustained" }, want: "saturated-or-generator-limited"},
		{name: "invalid baseline", cores: 8, rate: 100, ratio: .5, change: func(_, p *Trial) { p.Classification = "invalid" }, want: "saturated-or-generator-limited"},
		{name: "zero baseline", cores: 8, rate: 100, ratio: .5, change: func(_, p *Trial) { p.AcceptedPerSecond = 0 }, want: "saturated-or-generator-limited"},
		{name: "invalid overrides CPU", cores: 16, rate: 100, ratio: 1, change: func(t, _ *Trial) { t.GeneratorFailures = 1 }, want: "invalid"},
		{name: "no accepted data", cores: 16, ratio: .5, change: func(t, _ *Trial) { t.Accepted = Counts{} }, want: "invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			previous := baseline
			trial := Trial{Spec: PhaseSpec{Kind: "sweep", Signal: Logs, Repetition: 2, ProcessesPerSignal: 32}, Accepted: Counts{LogRecords: 1}, AcceptedPerSecond: tc.rate, AcceptanceRatio: tc.ratio, GeneratorCPU: &GeneratorCPU{AverageCPUCores: tc.cores, LogicalCPUs: 16}}
			if tc.change != nil {
				tc.change(&trial, &previous)
			}
			// Older points must not replace the immediately preceding process count.
			older := baseline
			older.Spec.ProcessesPerSignal = 8
			older.AcceptedPerSecond = 1
			got, _ := classify(trial, []Trial{previous, older})
			if got != tc.want {
				t.Fatalf("classification=%q want %q", got, tc.want)
			}
		})
	}
}

func TestSecretRedactionAndBoundedOutput(t *testing.T) {
	b := &boundedBuffer{limit: 8}
	_, _ = b.Write([]byte("012345secret"))
	if got := b.String(); got != "45secret" {
		t.Fatalf("bounded tail=%q", got)
	}
	if got := redact("Bearer secret; cookie=session", "secret", "session"); strings.Contains(got, "secret") || strings.Contains(got, "session") {
		t.Fatalf("redaction failed: %q", got)
	}
}

func TestBestMixedTopologyUsesAcceptedThroughput(t *testing.T) {
	trials := []Trial{{Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 8}, AcceptedPerSecond: 100}, {Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 4}, AcceptedPerSecond: 200}}
	if got := bestMixedTopology(trials); got != 4 {
		t.Fatalf("got %d want 4", got)
	}
}

func TestBestMixedTopologyRequiresRequestedQueryLoad(t *testing.T) {
	trials := []Trial{
		{Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 8, QueryRate: 100}, ElapsedSeconds: 10, AcceptedPerSecond: 200, QueryLatency: Latency{Count: 800}, QueryShed: 10},
		{Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 4, QueryRate: 100}, ElapsedSeconds: 10, AcceptedPerSecond: 150, QueryLatency: Latency{Count: 980}},
	}
	if got := bestMixedTopology(trials); got != 4 {
		t.Fatalf("got %d want 4", got)
	}
}

func TestBestMixedTopologyFallsBackToBestReadCompletion(t *testing.T) {
	trials := []Trial{
		{Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 8, QueryRate: 100}, ElapsedSeconds: 10, AcceptedPerSecond: 200, QueryLatency: Latency{Count: 100}},
		{Spec: PhaseSpec{Signal: Mixed, Kind: "sweep", ProcessesPerSignal: 1, QueryRate: 100}, ElapsedSeconds: 10, AcceptedPerSecond: 50, QueryLatency: Latency{Count: 500}},
	}
	if got := bestMixedTopology(trials); got != 1 {
		t.Fatalf("got %d want 1", got)
	}
}

// A suite that dies in a late phase must still hand back what it measured, and
// that partial report must be unmistakably marked as unpublishable.
func TestAbandonKeepsCompletedPhasesAndMarksReportFailed(t *testing.T) {
	runner := &Runner{Config: Config{Token: "ingest-secret"}}
	report := &Report{Trials: []Trial{{Spec: PhaseSpec{Name: "traces", Kind: "sweep"}, Accepted: Counts{Spans: 10}, ElapsedSeconds: 5}}}
	got, err := runner.abandon(report, errors.New("phase mixed: final metrics: ingest-secret leaked"))
	if err == nil {
		t.Fatal("abandon dropped the cause")
	}
	if got == nil || len(got.Trials) != 1 {
		t.Fatalf("partial evidence was discarded: %+v", got)
	}
	if got.Summary.Passed {
		t.Fatal("partial report claims the gates passed")
	}
	if got.FinishedAt.IsZero() {
		t.Fatal("partial report has no finish time")
	}
	var reason string
	for _, gate := range got.Summary.Gates {
		if gate.Name == "run-completed" {
			reason = gate.Detail
		}
	}
	if reason == "" {
		t.Fatalf("no run-completed gate: %+v", got.Summary.Gates)
	}
	if strings.Contains(reason, "ingest-secret") {
		t.Fatalf("gate detail leaked the ingest token: %q", reason)
	}
}
