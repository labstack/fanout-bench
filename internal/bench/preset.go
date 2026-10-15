package bench

import (
	"fmt"
	"time"
)

func Plan(cfg Config) ([]PhaseSpec, error) {
	if cfg.Repeat <= 0 {
		return nil, fmt.Errorf("repeat must be positive")
	}
	if cfg.BatchSize <= 0 {
		return nil, fmt.Errorf("batch size must be positive")
	}
	type shape struct {
		processes []int
		workers   int
		rate      float64
		trial     time.Duration
		sustain   time.Duration
		warmup    time.Duration
	}
	var s shape
	switch cfg.Preset {
	case "quick":
		s = shape{[]int{1}, 2, 1_000, 5 * time.Second, 10 * time.Second, 2 * time.Second}
	case "standard":
		s = shape{[]int{1, 2, 4, 8}, 8, 2_500, 15 * time.Second, 60 * time.Second, 10 * time.Second}
	case "publish":
		s = shape{[]int{1, 2, 4, 8, 16, 32, 64}, 8, 2_500, 30 * time.Second, 5 * time.Minute, 15 * time.Second}
	default:
		return nil, fmt.Errorf("unknown preset %q (want quick, standard, or publish)", cfg.Preset)
	}
	if cfg.ProcessOverride > 0 {
		s.processes = []int{cfg.ProcessOverride}
	}
	if cfg.WorkerOverride > 0 {
		s.workers = cfg.WorkerOverride
	}
	if cfg.RateOverride > 0 {
		s.rate = cfg.RateOverride
	}
	if cfg.DurationOverride > 0 {
		s.trial = cfg.DurationOverride
		s.sustain = cfg.DurationOverride
	}
	makePhase := func(name, kind string, signal Signal, d time.Duration, processes, repetition int) PhaseSpec {
		return PhaseSpec{
			Name: name, Kind: kind, Signal: signal, Duration: d,
			DurationSeconds: d.Seconds(), ProcessesPerSignal: processes,
			WorkersPerProcess: s.workers, RatePerWorker: s.rate,
			BatchSize: cfg.BatchSize, Repetition: repetition,
		}
	}
	phases := []PhaseSpec{makePhase("warmup", "warmup", Mixed, s.warmup, 1, 1)}
	for repetition := 1; repetition <= cfg.Repeat; repetition++ {
		for _, signal := range Signals {
			for _, processes := range s.processes {
				name := fmt.Sprintf("%s-p%d-r%d", signal, processes, repetition)
				phases = append(phases, makePhase(name, "sweep", signal, s.trial, processes, repetition))
			}
		}
		for _, processes := range s.processes {
			name := fmt.Sprintf("mixed-p%d-r%d", processes, repetition)
			phase := makePhase(name, "sweep", Mixed, s.trial, processes, repetition)
			phase.QueryRate = cfg.QueryRate
			phases = append(phases, phase)
		}
	}
	// ProcessesPerSignal=0 means select the topology with the highest accepted
	// mixed throughput from the sweep, then prove it over a longer window.
	sustained := makePhase("mixed-sustained", "sustained", Mixed, s.sustain, 0, 1)
	sustained.QueryRate = cfg.QueryRate
	phases = append(phases, sustained)
	return phases, nil
}
