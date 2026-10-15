package bench

import (
	"slices"
	"testing"
	"time"
)

func TestPublishPlanExtendsEverySweepAndKeepsShape(t *testing.T) {
	plan, err := Plan(Config{Preset: "publish", Repeat: 2, BatchSize: 1000, QueryRate: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 58 {
		t.Fatalf("phases=%d want 58", len(plan))
	}
	for repetition := 1; repetition <= 2; repetition++ {
		for _, signal := range []Signal{Traces, Logs, Metrics, Mixed} {
			var processes []int
			for _, phase := range plan {
				if phase.Kind != "sweep" || phase.Repetition != repetition || phase.Signal != signal {
					continue
				}
				processes = append(processes, phase.ProcessesPerSignal)
				if phase.WorkersPerProcess != 8 || phase.RatePerWorker != 2500 || phase.BatchSize != 1000 || phase.Duration != 30*time.Second || phase.DurationSeconds != 30 {
					t.Fatalf("publish shape changed: %+v", phase)
				}
				if (signal == Mixed && phase.QueryRate != 20) || (signal != Mixed && phase.QueryRate != 0) {
					t.Fatalf("query load changed: %+v", phase)
				}
			}
			if !slices.Equal(processes, []int{1, 2, 4, 8, 16, 32, 64}) {
				t.Errorf("%s repetition %d: processes=%v", signal, repetition, processes)
			}
		}
	}
	if p := plan[0]; p.Kind != "warmup" || p.Duration != 15*time.Second || p.ProcessesPerSignal != 1 {
		t.Fatalf("warmup changed: %+v", p)
	}
	if p := plan[len(plan)-1]; p.Kind != "sustained" || p.Signal != Mixed || p.Duration != 5*time.Minute || p.ProcessesPerSignal != 0 || p.QueryRate != 20 {
		t.Fatalf("sustained selection or shape changed: %+v", p)
	}
}

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
