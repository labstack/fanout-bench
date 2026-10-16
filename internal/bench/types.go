package bench

import "time"

const SchemaVersion = "fanout-bench/v1"

type Signal string

const (
	Traces  Signal = "traces"
	Logs    Signal = "logs"
	Metrics Signal = "metrics"
	Mixed   Signal = "mixed"
)

var Signals = []Signal{Traces, Logs, Metrics}

type Config struct {
	Host                     string
	HostDescription          string
	GeneratorHost            string
	GeneratorAddress         string
	FanoutListen             string
	GeneratorHostDescription string
	GeneratorSSH             string
	GeneratorSSHPort         int
	GeneratorSSHIdentity     string
	GeneratorSSHKnownHosts   string
	SSHKnownHosts            string
	SSHIdentity              string
	SSHPort                  int
	KeepRemote               bool
	Preset                   string
	Endpoint                 string
	MetricsURL               string
	Insecure                 bool
	Telemetrygen             string
	Token                    string
	MetricsToken             string
	OutputDir                string
	FanoutVersion            string
	FanoutCommit             string
	FanoutArtifact           string
	DataDir                  string
	SampleInterval           time.Duration
	GeneratorTimeout         time.Duration
	Cooldown                 time.Duration
	Repeat                   int
	QueryURL                 string
	QueryCookie              string
	QueryRate                float64
	SameHost                 bool
	ProcessOverride          int
	WorkerOverride           int
	RateOverride             float64
	DurationOverride         time.Duration
	BatchSize                int
	TelemetrygenVersion      string
}

type PhaseSpec struct {
	Name               string        `json:"name"`
	Kind               string        `json:"kind"`
	Signal             Signal        `json:"signal"`
	Duration           time.Duration `json:"-"`
	DurationSeconds    float64       `json:"duration_seconds"`
	ProcessesPerSignal int           `json:"processes_per_signal"`
	WorkersPerProcess  int           `json:"workers_per_process"`
	RatePerWorker      float64       `json:"rate_per_worker"`
	BatchSize          int           `json:"batch_size"`
	QueryRate          float64       `json:"query_rate,omitempty"`
	Repetition         int           `json:"repetition"`
}

func (p PhaseSpec) OfferedPerSecond() float64 {
	// telemetrygen's trace worker charges its limiter once for the parent and
	// once for the child, so --rate is already aggregate spans/s. Logs and
	// metric points use the same one-item-per-token convention.
	signals := 1
	if p.Signal == Mixed {
		signals = len(Signals)
	}
	return float64(signals*p.ProcessesPerSignal*p.WorkersPerProcess) * p.RatePerWorker
}

type Host struct {
	Description          string `json:"description,omitempty"`
	GeneratorDescription string `json:"generator_description,omitempty"`
	OS                   string `json:"os"`
	Architecture         string `json:"architecture"`
	CPUModel             string `json:"cpu_model,omitempty"`
	LogicalCPUs          int    `json:"logical_cpus"`
	MemoryBytes          uint64 `json:"memory_bytes,omitempty"`
	GoVersion            string `json:"go_version"`
	Hostname             string `json:"hostname,omitempty"`
	GeneratorLocal       bool   `json:"generator_on_target_host"`
}

type Build struct {
	HarnessVersion      string `json:"harness_version"`
	HarnessCommit       string `json:"harness_commit"`
	HarnessDirty        bool   `json:"harness_dirty"`
	TelemetrygenVersion string `json:"telemetrygen_version"`
	FanoutVersion       string `json:"fanout_version"`
	FanoutCommit        string `json:"fanout_commit,omitempty"`
	FanoutArtifact      string `json:"fanout_artifact,omitempty"`
}

type Settings struct {
	Preset           string        `json:"preset"`
	Endpoint         string        `json:"endpoint"`
	MetricsURL       string        `json:"metrics_url"`
	Transport        string        `json:"transport"`
	Repeat           int           `json:"repeat"`
	SampleInterval   time.Duration `json:"-"`
	SampleSeconds    float64       `json:"sample_interval_seconds"`
	GeneratorTimeout time.Duration `json:"-"`
	TimeoutSeconds   float64       `json:"generator_timeout_seconds"`
	Cooldown         time.Duration `json:"-"`
	CooldownSeconds  float64       `json:"cooldown_seconds"`
	DataDir          string        `json:"data_dir,omitempty"`
	QueryURL         string        `json:"query_url,omitempty"`
	QueryRate        float64       `json:"query_rate,omitempty"`
	BatchSize        int           `json:"batch_size"`
	TraceChildSpans  int           `json:"trace_child_spans"`
	Methodology      string        `json:"methodology"`
}

type Counts struct {
	Spans        float64 `json:"spans"`
	LogRecords   float64 `json:"log_records"`
	MetricPoints float64 `json:"metric_points"`
}

func (c Counts) Total() float64 { return c.Spans + c.LogRecords + c.MetricPoints }

type ResourceSample struct {
	AtSeconds                      float64            `json:"at_seconds"`
	CPUSeconds                     float64            `json:"cpu_seconds"`
	RSSBytes                       float64            `json:"rss_bytes"`
	HeapBytes                      float64            `json:"heap_bytes"`
	QueueDepth                     float64            `json:"queue_depth"`
	ParquetFiles                   float64            `json:"parquet_files"`
	ParquetBytes                   float64            `json:"parquet_bytes"`
	PublishWaiters                 float64            `json:"publish_waiters"`
	PublishTimeouts                float64            `json:"publish_timeouts_total"`
	IngestRowsTotal                map[string]float64 `json:"ingest_rows_total,omitempty"`
	RollupSeconds                  map[string]float64 `json:"rollup_seconds,omitempty"`
	WriteGateHoldSeconds           map[string]float64 `json:"write_gate_hold_seconds,omitempty"`
	WriteGateWaitSeconds           map[string]float64 `json:"write_gate_wait_seconds,omitempty"`
	CompactionSecondsTotal         float64            `json:"compaction_seconds_total,omitempty"`
	CompactionsTotal               float64            `json:"compactions_total,omitempty"`
	PublishWaitSecondsTotal        float64            `json:"publish_wait_seconds_total,omitempty"`
	IngestBackpressureSecondsTotal float64            `json:"ingest_backpressure_seconds_total,omitempty"`
	CompactionBacklogBatches       float64            `json:"compaction_backlog_batches,omitempty"`
}

type Latency struct {
	Count  int64   `json:"count"`
	MeanMs float64 `json:"mean_ms"`
	P50Ms  float64 `json:"p50_ms"`
	P95Ms  float64 `json:"p95_ms"`
	P99Ms  float64 `json:"p99_ms"`
	MaxMs  float64 `json:"max_ms"`
}

type Resources struct {
	AverageCPUCores float64 `json:"average_cpu_cores"`
	PeakRSSBytes    float64 `json:"peak_rss_bytes"`
	EndRSSBytes     float64 `json:"end_rss_bytes"`
	PeakHeapBytes   float64 `json:"peak_heap_bytes"`
	EndHeapBytes    float64 `json:"end_heap_bytes"`
	PeakQueueDepth  float64 `json:"peak_queue_depth"`
	ParquetFiles    float64 `json:"parquet_files_end"`
	ParquetBytes    float64 `json:"parquet_bytes_end"`
	DiskGrowthBytes float64 `json:"disk_growth_bytes,omitempty"`
	AllocBytes      float64 `json:"allocated_bytes"`
	GCPauseSeconds  float64 `json:"gc_pause_seconds"`
}

type Gate struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type GeneratorCPU struct {
	AverageCPUCores float64 `json:"average_cpu_cores"`
	LogicalCPUs     int     `json:"logical_cpus"`
}

type Trial struct {
	Spec                PhaseSpec        `json:"settings"`
	StartedAt           time.Time        `json:"started_at"`
	ElapsedSeconds      float64          `json:"elapsed_seconds"`
	OfferedPerSecond    float64          `json:"offered_items_per_second"`
	Accepted            Counts           `json:"accepted"`
	AcceptedPerSecond   float64          `json:"accepted_items_per_second"`
	AcceptanceRatio     float64          `json:"acceptance_ratio"`
	RowsDropped         float64          `json:"rows_dropped"`
	GeneratorProcesses  int              `json:"generator_processes"`
	GeneratorFailures   int              `json:"generator_failures"`
	GeneratorCPU        *GeneratorCPU    `json:"generator_cpu,omitempty"`
	ProcessRestarted    bool             `json:"fanout_restarted"`
	Resources           Resources        `json:"resources"`
	QueryLatency        Latency          `json:"query_latency_ms,omitempty"`
	QueryKindCompleted  map[string]int64 `json:"query_completed_by_kind,omitempty"`
	QueryAttempts       int64            `json:"query_attempts,omitempty"`
	QueryFailures       int64            `json:"query_failures,omitempty"`
	QueryShed           int64            `json:"query_shed,omitempty"`
	QueryCanceled       int64            `json:"query_canceled_at_phase_end,omitempty"`
	Classification      string           `json:"classification"`
	ClassificationNotes []string         `json:"classification_notes,omitempty"`
	Samples             []ResourceSample `json:"samples"`
}

type Peak struct {
	TrialName         string  `json:"trial_name"`
	OfferedPerSecond  float64 `json:"offered_items_per_second"`
	AcceptedPerSecond float64 `json:"accepted_items_per_second"`
	AverageCPUCores   float64 `json:"average_cpu_cores"`
	PeakRSSBytes      float64 `json:"peak_rss_bytes"`
}

type Summary struct {
	PeakTraces     Peak     `json:"peak_traces"`
	PeakLogs       Peak     `json:"peak_logs"`
	PeakMetrics    Peak     `json:"peak_metrics"`
	PeakMixed      Peak     `json:"peak_mixed"`
	SustainedMixed Peak     `json:"sustained_mixed"`
	Accepted       Counts   `json:"accepted_total"`
	CooldownRSS    float64  `json:"cooldown_rss_bytes,omitempty"`
	CooldownHeap   float64  `json:"cooldown_heap_bytes,omitempty"`
	Gates          []Gate   `json:"quality_gates"`
	Passed         bool     `json:"passed"`
	Warnings       []string `json:"warnings,omitempty"`
}

type Report struct {
	SchemaVersion       string    `json:"schema_version"`
	RunID               string    `json:"run_id"`
	StartedAt           time.Time `json:"started_at"`
	FinishedAt          time.Time `json:"finished_at"`
	Build               Build     `json:"build"`
	Host                Host      `json:"host"`
	Settings            Settings  `json:"settings"`
	WorkloadSHA256      string    `json:"workload_sha256"`
	Trials              []Trial   `json:"trials"`
	Summary             Summary   `json:"summary"`
	ReproductionCommand string    `json:"reproduction_command"`
}
