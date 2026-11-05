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

func TestLocalPlanKeepsPublishShapeOnUpperSweepPoints(t *testing.T) {
	plan, err := Plan(Config{Preset: "local", Repeat: 1, BatchSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 14 {
		t.Fatalf("phases=%d want 14", len(plan))
	}
	if p := plan[0]; p.Kind != "warmup" || p.Duration != 15*time.Second {
		t.Fatalf("warmup changed: %+v", p)
	}
	if p := plan[len(plan)-1]; p.Kind != "sustained" || p.Signal != Mixed || p.Duration != 5*time.Minute || p.ProcessesPerSignal != 0 {
		t.Fatalf("sustained changed: %+v", p)
	}
	var total time.Duration
	for _, signal := range []Signal{Traces, Logs, Metrics, Mixed} {
		var processes []int
		for _, phase := range plan {
			total += 0
			if phase.Kind != "sweep" || phase.Signal != signal {
				continue
			}
			processes = append(processes, phase.ProcessesPerSignal)
			if phase.WorkersPerProcess != 8 || phase.RatePerWorker != 2500 || phase.Duration != 30*time.Second {
				t.Fatalf("local shape changed: %+v", phase)
			}
		}
		if !slices.Equal(processes, []int{8, 16, 32}) {
			t.Errorf("%s: processes=%v", signal, processes)
		}
	}
	for _, phase := range plan {
		total += phase.Duration
	}
	if total > 12*time.Minute {
		t.Errorf("load time %s exceeds the 12 minute budget", total)
	}
}

func TestMixedOnlyPlan(t *testing.T) {
	for _, preset := range []string{"publish", "validate"} {
		plan, err := Plan(Config{Preset: preset, Repeat: 3, BatchSize: 1000, SignalSelection: "mixed", QueryRate: 20})
		if err != nil {
			t.Fatal(err)
		}
		if len(plan) != 23 {
			t.Fatalf("%s phases=%d", preset, len(plan))
		}
		for _, p := range plan {
			if p.Signal != Mixed {
				t.Fatalf("unexpected single-signal phase: %+v", p)
			}
			if p.Kind != "warmup" && p.QueryRate != 20 {
				t.Fatal("missing reads")
			}
		}
		last := plan[len(plan)-1]
		if last.Kind != "sustained" || last.Duration != 5*time.Minute || last.ProcessesPerSignal != 0 {
			t.Fatal(last)
		}
	}
	if _, err := Plan(Config{Preset: "publish", SignalSelection: "logs", Repeat: 1, BatchSize: 1000}); err == nil {
		t.Fatal("invalid selection accepted")
	}
}

func TestValidationUsesPublishShape(t *testing.T) {
	cfg := Config{Preset: "validate", Repeat: 3, BatchSize: 1000}
	a, err := Plan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Preset = "publish"
	b, err := Plan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a, b) {
		t.Fatal("validation changed load shape")
	}
	if settingsFrom(cfg).EarlyStop {
		t.Fatal("publish early stop enabled")
	}
	cfg.Preset = "validate"
	if !settingsFrom(cfg).EarlyStop {
		t.Fatal("validate early stop disabled")
	}
}
