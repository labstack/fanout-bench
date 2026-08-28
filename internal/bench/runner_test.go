package bench

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestGeneratorArgsMatchPinnedTelemetrygenCLI(t *testing.T) {
	cfg := Config{Endpoint: "localhost:4317", Token: "token with space", GeneratorTimeout: 30, Insecure: true}
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
		got, _ := classify(tt.trial)
		if got != tt.want {
			t.Errorf("classify(%+v)=%q want %q", tt.trial, got, tt.want)
		}
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
