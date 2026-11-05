package bench

import (
	"strings"
	"testing"
)

func TestSweepStop(t *testing.T) {
	point := func(p int, rate, ratio float64) Trial {
		return Trial{Spec: PhaseSpec{Name: "point", Kind: "sweep", Signal: Mixed, Repetition: 1, ProcessesPerSignal: p}, Accepted: Counts{Spans: rate}, AcceptedPerSecond: rate, AcceptanceRatio: ratio}
	}
	remaining := []PhaseSpec{{Name: "skip", Kind: "sweep", Signal: Mixed, Repetition: 1, ProcessesPerSignal: 8}, {Name: "next-repeat", Kind: "sweep", Signal: Mixed, Repetition: 2}, {Name: "logs", Kind: "sweep", Signal: Logs, Repetition: 1}, {Name: "sustained", Kind: "sustained", Signal: Mixed}}
	for _, tc := range []struct {
		name   string
		points []Trial
		want   string
	}{
		{"one point", []Trial{point(1, 100, 1)}, ""},
		{"one flat gain", []Trial{point(1, 100, 1), point(2, 104, .6)}, ""},
		{"plateau", []Trial{point(1, 100, 1), point(2, 104, .6), point(4, 108, .4)}, "two consecutive"},
		{"decline", []Trial{point(1, 100, 1), point(2, 99, .9), point(4, 90, .8)}, "two consecutive"},
		{"exact five percent", []Trial{point(1, 100, 1), point(2, 105, .6), point(4, 108, .4)}, ""},
		{"recovery", []Trial{point(1, 100, 1), point(2, 104, .6), point(4, 120, .4)}, ""},
		{"collapse", []Trial{point(1, 100, 1), point(2, 110, .4)}, "acceptance ratio"},
		{"half boundary", []Trial{point(1, 100, 1), point(2, 110, .5)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stop := sweepStop(remaining, tc.points)
			if tc.want == "" {
				if stop != nil {
					t.Fatal(stop)
				}
				return
			}
			if stop == nil || !strings.Contains(stop.Reason, tc.want) || len(stop.SkippedPhases) != 1 || stop.SkippedPhases[0] != "skip" {
				t.Fatal(stop)
			}
			if sweepStop(nil, tc.points) != nil {
				t.Fatal("recorded stop without skipped work")
			}
		})
	}
	for _, invalid := range []func(*Trial){func(t *Trial) { t.GeneratorFailures = 1 }, func(t *Trial) { t.RowsDropped = 1 }, func(t *Trial) { t.ProcessRestarted = true }, func(t *Trial) { t.Accepted = Counts{} }, func(t *Trial) { t.Classification = "invalid" }} {
		points := []Trial{point(1, 100, 1), point(2, 101, .6), point(4, 102, .4)}
		invalid(&points[1])
		if sweepStop(remaining, points) != nil {
			t.Fatal("invalid point justified stop")
		}
	}
	points := []Trial{point(1, 100, 1), point(2, 101, .6), point(4, 102, .4)}
	points[0].Spec.Repetition = 2
	if sweepStop(remaining, points) != nil {
		t.Fatal("crossed repetition")
	}
	points[0].Spec.Signal = Logs
	if sweepStop(remaining, points) != nil {
		t.Fatal("crossed signal")
	}
}

func TestMixedCoverageAndStopReport(t *testing.T) {
	cfg := Config{SignalSelection: "mixed", Preset: "validate"}
	report := Report{Settings: settingsFrom(cfg), SweepStops: []SweepStop{{AfterPhase: "mixed-p4-r1", Reason: "plateau", SkippedPhases: []string{"mixed-p8-r1"}}}}
	// Coverage is added by Run; stopping evidence is rendered independently.
	md := Markdown(report)
	for _, want := range []string{"mixed-p4-r1", "plateau", "mixed-p8-r1"} {
		if !strings.Contains(md, want) {
			t.Fatal(want)
		}
	}
	if !strings.Contains(reproductionCommand(cfg), "--signals 'mixed'") {
		t.Fatal("missing reproduction selection")
	}
}
