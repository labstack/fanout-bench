package bench

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Runner struct {
	Config       Config
	Version      string
	Out          io.Writer
	DeferSummary bool
	client       *http.Client
	queryTargets []queryTarget
	readSummary  string
}

func (r *Runner) Run(ctx context.Context) (*Report, error) {
	if err := r.validate(); err != nil {
		return nil, err
	}
	if r.Out == nil {
		r.Out = io.Discard
	}
	if r.client == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConns = 512
		transport.MaxIdleConnsPerHost = 512
		transport.MaxConnsPerHost = 512
		r.client = &http.Client{Timeout: 15 * time.Second, Transport: transport}
	}
	plan, err := Plan(r.Config)
	if err != nil {
		return nil, err
	}
	var readMethod string
	if r.Config.QueryRate > 0 {
		r.queryTargets, readMethod, err = loadQueryTargets(ctx, r.client, r.Config.QueryURL, r.Config.QueryCookie)
		if err != nil {
			return nil, fmt.Errorf("read workload: %s", redact(err.Error(), r.Config.Token, r.Config.QueryCookie, r.Config.MetricsToken))
		}
	}
	started := time.Now().UTC()
	report := &Report{
		SchemaVersion: SchemaVersion,
		RunID:         started.Format("20060102T150405Z"),
		StartedAt:     started,
		Build:         r.buildManifest(ctx),
		Host:          detectHost(r.Config.SameHost),
		Settings:      settingsFrom(r.Config),
	}
	report.Host.Description = r.Config.HostDescription
	report.Host.GeneratorDescription = r.Config.GeneratorHostDescription
	report.Settings.Methodology += readMethod
	report.WorkloadSHA256 = workloadHash(report.Settings, plan)
	report.ReproductionCommand = reproductionCommand(r.Config)
	printRunHeader(r.Out, report, plan)

	for i := range plan {
		if err := ctx.Err(); err != nil {
			return r.abandon(report, err)
		}
		spec := plan[i]
		if spec.Kind == "sustained" && spec.ProcessesPerSignal == 0 {
			best := bestMixedTopology(report.Trials)
			if best <= 0 {
				return r.abandon(report, errors.New("cannot select sustained topology: mixed sweep accepted no data"))
			}
			spec.ProcessesPerSignal = best
			plan[i] = spec
		}
		fmt.Fprintf(r.Out, "\n  %s  %-18s offered %s items/s for %s\n", phaseIcon(spec.Kind), spec.Name, formatNumber(spec.OfferedPerSecond()), spec.Duration)
		trial, err := r.runPhase(ctx, spec)
		if err != nil {
			return r.abandon(report, fmt.Errorf("phase %s: %w", spec.Name, err))
		}
		report.Trials = append(report.Trials, trial)
		report.Settings.Methodology += r.readSummary
		printTrialLine(r.Out, trial)
	}

	if r.Config.Cooldown > 0 {
		fmt.Fprintf(r.Out, "\n  ◌ cooldown %s (measuring retained memory)\n", r.Config.Cooldown)
		select {
		case <-ctx.Done():
			return r.abandon(report, ctx.Err())
		case <-time.After(r.Config.Cooldown):
		}
		if snap, err := r.scrape(ctx); err == nil {
			report.Summary.CooldownRSS = snap.total("process_resident_memory_bytes", nil)
			report.Summary.CooldownHeap = snap.total("go_memstats_heap_alloc_bytes", nil)
		}
	}
	report.FinishedAt = time.Now().UTC()
	report.Summary = summarize(*report, report.Summary.CooldownRSS, report.Summary.CooldownHeap)
	if !r.DeferSummary {
		printSummary(r.Out, *report)
	}
	return report, nil
}

// abandon returns the phases that did complete alongside the error. A suite is
// tens of minutes of load; discarding every finished phase because the last one
// failed throws away evidence that is still worth inspecting. The report is
// marked failed and carries the reason, so it can never be mistaken for a
// publishable result.
func (r *Runner) abandon(report *Report, cause error) (*Report, error) {
	report.FinishedAt = time.Now().UTC()
	report.Summary = summarize(*report, report.Summary.CooldownRSS, report.Summary.CooldownHeap)
	report.Summary.Gates = append(report.Summary.Gates, Gate{"run-completed", false, redact(cause.Error(), r.Config.Token, r.Config.QueryCookie, r.Config.MetricsToken)})
	report.Summary.Passed = false
	report.Summary.Warnings = append(report.Summary.Warnings, "This run ended before the suite finished. The phases recorded here are partial evidence and must not be published as a result.")
	return report, cause
}

func (r *Runner) validate() error {
	cfg := r.Config
	switch {
	case strings.TrimSpace(cfg.Token) == "":
		return errors.New("ingest token is required via the configured environment variable")
	case strings.TrimSpace(cfg.Endpoint) == "":
		return errors.New("OTLP endpoint is required")
	case strings.TrimSpace(cfg.MetricsURL) == "":
		return errors.New("Fanout metrics URL is required; accepted server rows are the benchmark authority")
	case cfg.SampleInterval <= 0:
		return errors.New("sample interval must be positive")
	case cfg.GeneratorTimeout <= 0:
		return errors.New("generator timeout must be positive")
	case cfg.Repeat <= 0:
		return errors.New("repeat must be positive")
	case cfg.QueryRate < 0:
		return errors.New("query rate cannot be negative")
	case cfg.QueryRate > 10_000:
		return errors.New("query rate cannot exceed 10000 operations/s")
	case cfg.QueryRate > 0 && strings.TrimSpace(cfg.QueryURL) == "":
		return errors.New("query URL is required when query rate is positive")
	case cfg.QueryRate > 0 && strings.TrimSpace(cfg.QueryCookie) == "":
		return errors.New("query session cookie is required when query rate is positive")
	}
	if cfg.GeneratorSSH != "" {
		return requireCommand("ssh")
	}
	info, err := os.Stat(cfg.Telemetrygen)
	if err != nil {
		return fmt.Errorf("telemetrygen: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("telemetrygen path %q is a directory", cfg.Telemetrygen)
	}
	return nil
}

func (r *Runner) runPhase(ctx context.Context, spec PhaseSpec) (Trial, error) {
	before, err := r.scrape(ctx)
	if err != nil {
		return Trial{}, fmt.Errorf("baseline metrics: %w", err)
	}
	beforeDisk, _ := directorySize(r.Config.DataDir)
	started := time.Now()
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	samplesDone := make(chan []ResourceSample, 1)
	go r.sample(phaseCtx, started, samplesDone)
	queries := newQueryRun()
	if spec.QueryRate > 0 {
		queries.targets = r.queryTargets
		// Queries run under the parent context, not the phase context, so that
		// finish() can drain in-flight requests instead of cancelling them.
		queries.start(ctx, r.client, r.Config.QueryURL, r.Config.QueryCookie, spec.QueryRate)
	}

	failures, processCount, runErr := r.runGenerators(ctx, spec)
	elapsed := time.Since(started).Seconds()
	if runErr != nil && ctx.Err() != nil {
		cancel()
		<-samplesDone
		queries.finish()
		return Trial{}, ctx.Err()
	}
	// Scrape immediately: the accepted delta covers the interval between the two
	// scrapes, so any wait inserted here would credit rows accepted during that
	// wait to a shorter interval and inflate the headline rate.
	after, err := r.scrape(ctx)
	// window is that scrape-to-scrape interval, and is the only honest
	// denominator for the accepted delta. elapsed remains the offered-load
	// duration and stays the denominator for completed queries.
	window := time.Since(started).Seconds()
	cancel()
	samples := <-samplesDone
	queries.finish()
	if err != nil {
		return Trial{}, fmt.Errorf("final metrics: %w", err)
	}
	afterDisk, _ := directorySize(r.Config.DataDir)
	accepted := subtractCounts(acceptedCounts(after), acceptedCounts(before))
	if accepted.Spans < 0 || accepted.LogRecords < 0 || accepted.MetricPoints < 0 {
		return Trial{}, errors.New("Fanout ingest counters moved backwards (process restart or wrong metrics target)")
	}
	dropped := after.total("fanout_rows_dropped_total", nil) - before.total("fanout_rows_dropped_total", nil)
	restarted := before.total("process_start_time_seconds", nil) != after.total("process_start_time_seconds", nil)
	resources := resourceDelta(before, after, samples, window)
	if afterDisk >= beforeDisk {
		resources.DiskGrowthBytes = float64(afterDisk - beforeDisk)
	}
	offeredItems := spec.OfferedPerSecond() * spec.Duration.Seconds()
	ratio := 0.0
	if offeredItems > 0 {
		ratio = accepted.Total() / offeredItems
	}
	trial := Trial{
		Spec: spec, StartedAt: started.UTC(), ElapsedSeconds: round(elapsed, 3),
		OfferedPerSecond: spec.OfferedPerSecond(), Accepted: accepted,
		AcceptedPerSecond: perSecond(accepted.Total(), window), AcceptanceRatio: ratio,
		RowsDropped: dropped, GeneratorProcesses: processCount, GeneratorFailures: failures,
		ProcessRestarted: restarted, Resources: resources,
		QueryLatency: queries.hist.snapshot(), QueryAttempts: queries.attempts, QueryFailures: queries.failures,
		QueryKindCompleted: queries.completedByKind(),
		QueryShed:          queries.shed, QueryCanceled: queries.canceled,
		Samples: samples,
	}
	trial.Classification, trial.ClassificationNotes = classify(trial)
	r.readSummary = queries.operationSummary(spec.Name)
	if trial.Spec.QueryRate > 0 && !queryTargetMet(trial) {
		trial.ClassificationNotes = append(trial.ClassificationNotes, "the requested query rate was not sustained")
	}
	if runErr != nil {
		trial.ClassificationNotes = append(trial.ClassificationNotes, redact(runErr.Error(), r.Config.Token, r.Config.QueryCookie, r.Config.MetricsToken))
	}
	return trial, nil
}

func (r *Runner) runGenerators(ctx context.Context, spec PhaseSpec) (int, int, error) {
	signals := []Signal{spec.Signal}
	if spec.Signal == Mixed {
		signals = Signals
	}
	type result struct{ err error }
	results := make(chan result, len(signals)*spec.ProcessesPerSignal)
	var wg sync.WaitGroup
	for _, signal := range signals {
		for process := 0; process < spec.ProcessesPerSignal; process++ {
			wg.Add(1)
			go func(signal Signal) {
				defer wg.Done()
				results <- result{err: r.runGenerator(ctx, signal, spec)}
			}(signal)
		}
	}
	wg.Wait()
	close(results)
	failures := 0
	var first error
	for result := range results {
		if result.err != nil {
			failures++
			if first == nil {
				first = result.err
			}
		}
	}
	return failures, len(signals) * spec.ProcessesPerSignal, first
}

func (r *Runner) runGenerator(ctx context.Context, signal Signal, spec PhaseSpec) error {
	args := generatorArgs(r.Config, signal, spec)
	cmd, cancel := generatorCommand(ctx, r.Config, spec.Duration, args...)
	defer cancel()
	// telemetrygen logs its full configuration, including headers. Never stream
	// child output or persist argv. Retain only a bounded tail for redacted errors.
	output := &boundedBuffer{limit: 16 << 10}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("telemetrygen %s: %w: %s", signal, err, redact(output.String(), r.Config.Token, r.Config.QueryCookie, r.Config.MetricsToken))
	}
	return nil
}

// The remote timeout bounds orphaned generators if the SSH connection is lost.
// Give export draining ten seconds of margin beyond the configured timeouts.
func generatorCommand(ctx context.Context, cfg Config, duration time.Duration, args ...string) (*exec.Cmd, context.CancelFunc) {
	if cfg.GeneratorSSH == "" {
		return exec.CommandContext(ctx, cfg.Telemetrygen, args...), func() {}
	}
	limit := duration + cfg.GeneratorTimeout + 10*time.Second
	// Bound the SSH client too, allowing five seconds beyond the remote timeout.
	ctx, cancel := context.WithTimeout(ctx, limit+5*time.Second)
	remote := append([]string{"exec", "timeout", "--signal=TERM", strconv.FormatFloat(limit.Seconds(), 'f', -1, 64) + "s", cfg.Telemetrygen}, args...)
	for i := range remote {
		remote[i] = shellQuote(remote[i])
	}
	port := cfg.GeneratorSSHPort
	if port == 0 {
		port = 22
	}
	ssh := []string{"-p", strconv.Itoa(port), "-i", cfg.GeneratorSSHIdentity,
		"-o", "ConnectTimeout=10", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3",
		"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=" + cfg.GeneratorSSHKnownHosts, cfg.GeneratorSSH, "--", strings.Join(remote, " ")}
	return exec.CommandContext(ctx, "ssh", ssh...), cancel
}

func generatorArgs(cfg Config, signal Signal, spec PhaseSpec) []string {
	args := []string{
		string(signal), "--otlp-endpoint", cfg.Endpoint,
		"--duration", spec.Duration.String(),
		"--rate", strconv.FormatFloat(spec.RatePerWorker, 'f', -1, 64),
		"--workers", strconv.Itoa(spec.WorkersPerProcess),
		"--batch-size", strconv.Itoa(spec.BatchSize),
		"--timeout", cfg.GeneratorTimeout.String(),
		"--otlp-header", "Authorization=" + strconv.Quote("Bearer "+cfg.Token),
	}
	if cfg.Insecure {
		args = append(args, "--otlp-insecure")
	}
	if signal == Traces {
		args = append(args, "--child-spans", "1")
	}
	return args
}

func (r *Runner) scrape(ctx context.Context) (*snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Config.MetricsURL, nil)
	if err != nil {
		return nil, err
	}
	if r.Config.MetricsToken != "" {
		req.Header.Set("Authorization", "Bearer "+r.Config.MetricsToken)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return parsePrometheus(io.LimitReader(resp.Body, 32<<20))
}

func (r *Runner) sample(ctx context.Context, started time.Time, done chan<- []ResourceSample) {
	ticker := time.NewTicker(r.Config.SampleInterval)
	defer ticker.Stop()
	var samples []ResourceSample
	for {
		select {
		case <-ctx.Done():
			done <- samples
			return
		case <-ticker.C:
			snap, err := r.scrape(ctx)
			if err != nil {
				continue
			}
			samples = append(samples, resourceSample(snap, time.Since(started).Seconds()))
		}
	}
}

func resourceSample(s *snapshot, at float64) ResourceSample {
	return ResourceSample{
		AtSeconds: round(at, 3), CPUSeconds: s.total("process_cpu_seconds_total", nil),
		RSSBytes: s.total("process_resident_memory_bytes", nil), HeapBytes: s.total("go_memstats_heap_alloc_bytes", nil),
		QueueDepth: s.total("fanout_ingest_queue_depth", nil), ParquetFiles: s.total("fanout_parquet_files", nil),
		ParquetBytes: s.total("fanout_parquet_size_bytes", nil), PublishWaiters: s.total("fanout_parquet_publish_waiters", nil),
		PublishTimeouts: s.total("fanout_parquet_publish_timeouts_total", nil),
	}
}

func resourceDelta(before, after *snapshot, samples []ResourceSample, elapsed float64) Resources {
	result := Resources{
		EndRSSBytes: after.total("process_resident_memory_bytes", nil), EndHeapBytes: after.total("go_memstats_heap_alloc_bytes", nil),
		ParquetFiles: after.total("fanout_parquet_files", nil), ParquetBytes: after.total("fanout_parquet_size_bytes", nil),
		AllocBytes:     after.total("go_memstats_alloc_bytes_total", nil) - before.total("go_memstats_alloc_bytes_total", nil),
		GCPauseSeconds: after.total("go_gc_duration_seconds_sum", nil) - before.total("go_gc_duration_seconds_sum", nil),
	}
	if elapsed > 0 {
		result.AverageCPUCores = (after.total("process_cpu_seconds_total", nil) - before.total("process_cpu_seconds_total", nil)) / elapsed
	}
	result.PeakRSSBytes, result.PeakHeapBytes = result.EndRSSBytes, result.EndHeapBytes
	for _, sample := range samples {
		result.PeakRSSBytes = math.Max(result.PeakRSSBytes, sample.RSSBytes)
		result.PeakHeapBytes = math.Max(result.PeakHeapBytes, sample.HeapBytes)
		result.PeakQueueDepth = math.Max(result.PeakQueueDepth, sample.QueueDepth)
	}
	return result
}

func classify(t Trial) (string, []string) {
	var notes []string
	if t.GeneratorFailures > 0 {
		notes = append(notes, "one or more telemetrygen processes failed")
	}
	if t.RowsDropped > 0 {
		notes = append(notes, "Fanout reported dropped rows")
	}
	if t.ProcessRestarted {
		notes = append(notes, "Fanout restarted during the trial")
	}
	if len(notes) > 0 {
		return "invalid", notes
	}
	switch {
	case t.Accepted.Total() == 0:
		return "invalid", []string{"Fanout accepted no telemetry"}
	case t.AcceptanceRatio >= .95:
		return "target-sustained", nil
	case t.AcceptanceRatio >= .80:
		return "near-saturation", []string{"accepted less than 95% of the requested workload"}
	default:
		return "saturated-or-generator-limited", []string{"the target was not sustained; separate load-generator hosts are needed to attribute the limit"}
	}
}

func bestMixedTopology(trials []Trial) int {
	bestRate, processes := -1.0, 0
	fallbackCompletion, fallbackRate, fallbackProcesses := -1.0, -1.0, 0
	for _, t := range trials {
		if t.Spec.Signal != Mixed || t.Spec.Kind != "sweep" || t.GeneratorFailures != 0 {
			continue
		}
		completion := 1.0
		if t.Spec.QueryRate > 0 && t.ElapsedSeconds > 0 {
			completion = perSecond(float64(t.QueryLatency.Count), t.ElapsedSeconds) / t.Spec.QueryRate
		}
		if completion > fallbackCompletion || (completion == fallbackCompletion && t.AcceptedPerSecond > fallbackRate) {
			fallbackCompletion, fallbackRate, fallbackProcesses = completion, t.AcceptedPerSecond, t.Spec.ProcessesPerSignal
		}
		if queryTargetMet(t) && t.AcceptedPerSecond > bestRate {
			bestRate, processes = t.AcceptedPerSecond, t.Spec.ProcessesPerSignal
		}
	}
	if processes == 0 {
		return fallbackProcesses
	}
	return processes
}

func queryTargetMet(t Trial) bool {
	if t.Spec.QueryRate <= 0 {
		return true
	}
	if t.ElapsedSeconds <= 0 {
		return false
	}
	if t.Spec.Kind == "sustained" && len(missingQueryKinds(t)) > 0 {
		return false
	}
	completedRate := perSecond(float64(t.QueryLatency.Count), t.ElapsedSeconds)
	return completedRate >= t.Spec.QueryRate*.95 && t.QueryFailures == 0 && t.QueryShed == 0
}

func missingQueryKinds(t Trial) []string {
	var missing []string
	for _, kind := range []string{"dashboard-refresh", "drill-traces", "trace-detail"} {
		if t.QueryKindCompleted[kind] == 0 {
			missing = append(missing, kind)
		}
	}
	return missing
}

func summarize(report Report, cooldownRSS, cooldownHeap float64) Summary {
	s := Summary{CooldownRSS: cooldownRSS, CooldownHeap: cooldownHeap, Passed: true}
	for _, trial := range report.Trials {
		if trial.Spec.Kind != "warmup" {
			s.Accepted.Spans += trial.Accepted.Spans
			s.Accepted.LogRecords += trial.Accepted.LogRecords
			s.Accepted.MetricPoints += trial.Accepted.MetricPoints
		}
		peak := Peak{trial.Spec.Name, trial.OfferedPerSecond, trial.AcceptedPerSecond, trial.Resources.AverageCPUCores, trial.Resources.PeakRSSBytes}
		if trial.Spec.Kind == "sustained" {
			s.SustainedMixed = peak
		}
		if trial.Spec.Kind != "sweep" {
			continue
		}
		switch trial.Spec.Signal {
		case Traces:
			if peak.AcceptedPerSecond > s.PeakTraces.AcceptedPerSecond {
				s.PeakTraces = peak
			}
		case Logs:
			if peak.AcceptedPerSecond > s.PeakLogs.AcceptedPerSecond {
				s.PeakLogs = peak
			}
		case Metrics:
			if peak.AcceptedPerSecond > s.PeakMetrics.AcceptedPerSecond {
				s.PeakMetrics = peak
			}
		case Mixed:
			if peak.AcceptedPerSecond > s.PeakMixed.AcceptedPerSecond {
				s.PeakMixed = peak
			}
		}
	}
	genOK, dropsOK, restartsOK, dataOK := true, true, true, true
	for _, t := range report.Trials {
		genOK = genOK && t.GeneratorFailures == 0
		dropsOK = dropsOK && t.RowsDropped == 0
		restartsOK = restartsOK && !t.ProcessRestarted
		dataOK = dataOK && t.Accepted.Total() > 0
	}
	s.Gates = []Gate{
		{"generator-processes", genOK, boolDetail(genOK, "all exited successfully", "at least one failed")},
		{"server-dropped-rows", dropsOK, boolDetail(dropsOK, "zero", "non-zero")},
		{"server-restarts", restartsOK, boolDetail(restartsOK, "none", "detected")},
		{"authoritative-counters", dataOK, boolDetail(dataOK, "Fanout accepted rows in every phase", "a phase accepted no rows")},
	}
	if report.Settings.QueryRate > 0 {
		queryOK := false
		detail := "sustained mixed phase missing"
		for _, t := range report.Trials {
			if t.Spec.Kind != "sustained" {
				continue
			}
			queryOK = queryTargetMet(t)
			completedRate := perSecond(float64(t.QueryLatency.Count), t.ElapsedSeconds)
			detail = fmt.Sprintf("%.1f/%.1f completed/s, %d errors, %d shed", completedRate, t.Spec.QueryRate, t.QueryFailures, t.QueryShed)
			if missing := missingQueryKinds(t); len(missing) > 0 {
				detail += "; zero successful operations: " + strings.Join(missing, ", ")
			}
			break
		}
		s.Gates = append(s.Gates, Gate{"sustained-query-load", queryOK, detail})
	}
	for _, gate := range s.Gates {
		s.Passed = s.Passed && gate.Passed
	}
	s.Warnings = []string{
		"Results describe this exact host, Fanout build, configuration, dataset, and workload; they are not universal capacity claims.",
		"telemetrygen payloads are intentionally synthetic and highly compressible; storage size is not representative of production telemetry.",
		"Offered load is a target. Headline throughput uses Fanout's accepted-row counters, never telemetrygen's intended or generated count.",
	}
	if report.Host.GeneratorLocal {
		s.Warnings = append(s.Warnings, "Fanout and load generators shared one host; generator contention can make the observed peak lower than Fanout's backend ceiling.")
	}
	if report.Settings.QueryRate == 0 {
		s.Warnings = append(s.Warnings, "This suite measured writes only. Configure query load to claim mixed read/write performance.")
	}
	return s
}

func settingsFrom(c Config) Settings {
	transport := "TLS"
	if c.Insecure {
		transport = "plaintext gRPC"
	}
	return Settings{Preset: c.Preset, Endpoint: c.Endpoint, MetricsURL: c.MetricsURL, Transport: transport, Repeat: c.Repeat,
		SampleInterval: c.SampleInterval, SampleSeconds: c.SampleInterval.Seconds(), GeneratorTimeout: c.GeneratorTimeout,
		TimeoutSeconds: c.GeneratorTimeout.Seconds(), Cooldown: c.Cooldown, CooldownSeconds: c.Cooldown.Seconds(), DataDir: c.DataDir,
		QueryURL: c.QueryURL, QueryRate: c.QueryRate, BatchSize: c.BatchSize, TraceChildSpans: 1,
		Methodology: "Fanout server-side accepted-row counter deltas divided by measured wall time; telemetrygen emits one parent and one child span per trace tick."}
}

func workloadHash(settings Settings, plan []PhaseSpec) string {
	// Per-operation latency results share the methodology field; only its
	// workload definition belongs in the workload identity.
	settings.Methodology, _, _ = strings.Cut(settings.Methodology, "\n\nRead operations for ")
	payload, _ := json.Marshal(struct {
		Settings Settings
		Plan     []PhaseSpec
	}{settings, plan})
	hash := sha256.Sum256(payload)
	return hex.EncodeToString(hash[:])
}

func (r *Runner) buildManifest(ctx context.Context) Build {
	b := Build{HarnessVersion: r.Version, HarnessCommit: "unknown", FanoutVersion: r.Config.FanoutVersion, FanoutCommit: r.Config.FanoutCommit, FanoutArtifact: r.Config.FanoutArtifact, TelemetrygenVersion: r.Config.TelemetrygenVersion}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				b.HarnessCommit = setting.Value
			case "vcs.modified":
				b.HarnessDirty = setting.Value == "true"
			}
		}
	}
	if b.TelemetrygenVersion == "" {
		cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		cmd, cancelGenerator := generatorCommand(cmdCtx, r.Config, 0, "--version")
		defer cancelGenerator()
		output := &boundedBuffer{limit: 16 << 10}
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Run(); err == nil {
			b.TelemetrygenVersion = strings.TrimSpace(redact(output.String(), r.Config.Token, r.Config.QueryCookie, r.Config.MetricsToken))
		}
		if b.TelemetrygenVersion == "" {
			b.TelemetrygenVersion = "unknown"
		}
	}
	return b
}

func detectHost(sameHost bool) Host {
	hostname, _ := os.Hostname()
	h := Host{OS: runtime.GOOS, Architecture: runtime.GOARCH, LogicalCPUs: runtime.NumCPU(), GoVersion: runtime.Version(), Hostname: hostname, GeneratorLocal: sameHost}
	if runtime.GOOS == "darwin" {
		if out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output(); err == nil {
			h.CPUModel = strings.TrimSpace(string(out))
		}
		if out, err := exec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
			h.MemoryBytes, _ = strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		}
	} else if runtime.GOOS == "linux" {
		if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "model name") {
					if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
						h.CPUModel = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}
		if h.CPUModel == "" || h.CPUModel == "-" {
			if data, err := os.ReadFile("/sys/firmware/devicetree/base/model"); err == nil {
				h.CPUModel = strings.TrimSpace(strings.TrimRight(string(data), "\x00"))
			}
		}
		if h.CPUModel == "" || h.CPUModel == "-" {
			if data, err := exec.Command("lscpu").Output(); err == nil {
				for _, line := range strings.Split(string(data), "\n") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 && strings.TrimSpace(parts[0]) == "Vendor ID" {
						h.CPUModel = strings.TrimSpace(parts[1])
						break
					}
				}
			}
		}
		if data, err := os.ReadFile("/proc/meminfo"); err == nil {
			fmt.Sscanf(string(data), "MemTotal: %d kB", &h.MemoryBytes)
			h.MemoryBytes *= 1024
		}
	}
	return h
}

func directorySize(path string) (uint64, error) {
	if path == "" {
		return 0, nil
	}
	var total uint64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += uint64(info.Size())
		}
		return nil
	})
	return total, err
}

type boundedBuffer struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.limit {
		b.buf = append([]byte(nil), b.buf[len(b.buf)-b.limit:]...)
	}
	return len(p), nil
}
func (b *boundedBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.buf) }

func redact(value string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}
func round(v float64, places int) float64 { p := math.Pow10(places); return math.Round(v*p) / p }

// perSecond keeps a zero or missing duration out of the reports: a trial read
// back from a truncated or hand-edited report would otherwise render +Inf.
func perSecond(count, seconds float64) float64 {
	if seconds <= 0 {
		return 0
	}
	return count / seconds
}

func boolDetail(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}
