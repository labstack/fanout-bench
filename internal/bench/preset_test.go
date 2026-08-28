package bench

import (
	"testing"
	"time"
)

func TestStandardPlanIsDeterministicAndBalanced(t *testing.T) {
	cfg := Config{Preset: "standard", Repeat: 1, BatchSize: 1000}
	got, err := Plan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 18 {
		t.Fatalf("phases=%d want 18", len(got))
	}
	if got[0].Kind != "warmup" || got[len(got)-1].Kind != "sustained" {
		t.Fatalf("unexpected boundaries: %#v / %#v", got[0], got[len(got)-1])
	}
	for _, signal := range Signals {
		count := 0
		for _, phase := range got {
			if phase.Kind == "sweep" && phase.Signal == signal {
				count++
			}
		}
		if count != 4 {
			t.Errorf("%s sweep phases=%d want 4", signal, count)
		}
	}
}

func TestOfferedRateUsesItemsNotTraceContainers(t *testing.T) {
	phase := PhaseSpec{Signal: Traces, ProcessesPerSignal: 4, WorkersPerProcess: 8, RatePerWorker: 2500}
	if got, want := phase.OfferedPerSecond(), 80_000.0; got != want {
		t.Fatalf("got %v want %v", got, want)
	}
	phase.Signal = Mixed
	if got, want := phase.OfferedPerSecond(), 240_000.0; got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestOverridesProduceOneDiagnosticTopology(t *testing.T) {
	plan, err := Plan(Config{Preset: "standard", Repeat: 1, BatchSize: 512, ProcessOverride: 3, WorkerOverride: 5, RateOverride: 7, DurationOverride: 2 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range plan[1:] {
		if phase.Kind == "sustained" {
			continue
		}
		if phase.ProcessesPerSignal != 3 || phase.WorkersPerProcess != 5 || phase.RatePerWorker != 7 || phase.Duration != 2*time.Second {
			t.Fatalf("override missing: %#v", phase)
		}
	}
}
