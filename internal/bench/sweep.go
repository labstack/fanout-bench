package bench

// sweepStop only compares successful points in one signal/repetition. Invalid
// evidence must remain visible to the quality gates, never justify a plateau.
func sweepStop(remaining []PhaseSpec, trials []Trial) *SweepStop {
	if len(trials) == 0 {
		return nil
	}
	current := trials[len(trials)-1]
	if current.Spec.Kind != "sweep" || !validSweepPoint(current) {
		return nil
	}
	previous := previousSweepTrial(current.Spec, trials)
	if previous == nil || !validSweepPoint(*previous) {
		return nil
	}
	reason := ""
	if current.AcceptanceRatio < .5 && current.AcceptanceRatio < .5*previous.AcceptanceRatio {
		reason = "acceptance ratio collapsed below 50% and below half the previous point"
	} else if before := previousSweepTrial(previous.Spec, trials); before != nil && validSweepPoint(*before) &&
		current.AcceptedPerSecond < 1.05*previous.AcceptedPerSecond && previous.AcceptedPerSecond < 1.05*before.AcceptedPerSecond {
		reason = "two consecutive accepted-throughput gains below 5%"
	}
	if reason == "" {
		return nil
	}
	stop := &SweepStop{AfterPhase: current.Spec.Name, Reason: reason}
	for _, spec := range remaining {
		if spec.Kind == "sweep" && spec.Signal == current.Spec.Signal && spec.Repetition == current.Spec.Repetition {
			stop.SkippedPhases = append(stop.SkippedPhases, spec.Name)
		}
	}
	if len(stop.SkippedPhases) == 0 {
		return nil
	}
	return stop
}

func validSweepPoint(t Trial) bool {
	return t.Classification != "invalid" && t.GeneratorFailures == 0 && t.RowsDropped == 0 && !t.ProcessRestarted && t.Accepted.Total() > 0 && t.AcceptedPerSecond > 0
}
