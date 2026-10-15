package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// maxQueryBody bounds a single response. Oversized responses fail because
// their panel errors and discovered trace contents cannot be validated.
const maxQueryBody = 32 << 20

type queryRun struct {
	targets    []queryTarget
	traceMu    sync.Mutex
	traces     map[string]traceReference
	selections map[string]drillSelection
	kindsMu    sync.Mutex
	kinds      map[string]*operationStats
	wg         sync.WaitGroup
	hist       *histogram
	failures   int64
	attempts   int64
	shed       int64
	canceled   int64
	stop       chan struct{}
	stopOnce   sync.Once
	cancel     context.CancelFunc
}

type operationStats struct {
	dispatched, failures, canceled int64
	hist                           *histogram
}

func newQueryRun() *queryRun {
	return &queryRun{targets: []queryTarget{{name: "dashboard-refresh", body: []byte(`{}`)}}, traces: map[string]traceReference{}, selections: map[string]drillSelection{}, kinds: map[string]*operationStats{}, hist: newHistogram(), stop: make(chan struct{}), cancel: func() {}}
}

func (q *queryRun) start(ctx context.Context, client *http.Client, base, cookie string, rate float64) {
	// Two seconds of concurrency keeps the schedule open-loop through normal
	// latency spikes. The hard cap prevents a stalled target from creating an
	// unbounded number of requests or goroutines.
	workers := int(math.Ceil(rate * 2))
	if workers < 4 {
		workers = 4
	}
	if workers > 512 {
		workers = 512
	}
	interval := time.Duration(float64(time.Second) / rate)
	// The queue is buffered so that a tick is shed only when every worker is
	// busy and the queue is full. An unbuffered channel would also shed
	// whenever no worker happened to be parked in its receive at that instant,
	// which reports backpressure the server never caused.
	jobs := make(chan int, workers)
	runCtx, cancel := context.WithCancel(ctx)
	q.cancel = cancel
	q.wg.Add(workers + 1)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer q.wg.Done()
			for targetIndex := range jobs {
				q.request(runCtx, client, base, cookie, targetIndex)
			}
		}()
	}
	go func() {
		defer q.wg.Done()
		defer close(jobs)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		targetIndex := 0
		for {
			select {
			case <-runCtx.Done():
				return
			case <-q.stop:
				return
			case <-ticker.C:
				atomic.AddInt64(&q.attempts, 1)
				select {
				case jobs <- targetIndex:
					targetIndex++
				default:
					atomic.AddInt64(&q.shed, 1)
				}
			}
		}
	}()
}

// finish stops the schedule and lets in-flight requests complete before the
// query context is canceled. Cancelling first would abort responses that were
// already being read, and those abort as read errors rather than transport
// errors, which the trial would then count as server failures.
func (q *queryRun) finish() {
	q.stopOnce.Do(func() { close(q.stop) })
	q.wg.Wait()
	q.cancel()
}

func (q *queryRun) request(ctx context.Context, client *http.Client, base, cookie string, targetIndex int) {
	query := q.targets[targetIndex%len(q.targets)]
	kind := query.name
	started := time.Now()
	success := false
	defer func() {
		elapsed := time.Since(started)
		q.kindsMu.Lock()
		defer q.kindsMu.Unlock()
		stats := q.kinds[kind]
		if stats == nil {
			stats = &operationStats{hist: newHistogram()}
			q.kinds[kind] = stats
		}
		stats.dispatched++
		if success {
			q.hist.record(elapsed)
			stats.hist.record(elapsed)
		} else if ctx.Err() != nil {
			atomic.AddInt64(&q.canceled, 1)
			stats.canceled++
		} else {
			atomic.AddInt64(&q.failures, 1)
			stats.failures++
		}
	}()
	refresh := func() bool {
		body, ok := queryHTTP(ctx, client, base, cookie, http.MethodPost, "/api/panels/query", query.body)
		if !ok {
			return false
		}
		annotations, valid := q.panelResponse(body, query)
		// refresh.ts requests annotations after the panel batch, even if a
		// panel reports an error. Count the whole operation as failed either way.
		if annotations != nil {
			_, ok = queryHTTP(ctx, client, base, cookie, http.MethodPost, "/api/annotations", annotations)
		}
		return valid && ok
	}
	if kind == "dashboard-refresh" {
		success = refresh()
		return
	}
	q.traceMu.Lock()
	trace := q.traces[query.window]
	q.traceMu.Unlock()
	if kind == "trace-detail" && trace.id != "" {
		body, ok := queryHTTP(ctx, client, base, cookie, http.MethodGet, traceDetailPath(trace), nil)
		success = ok && traceDetailResponse(body)
		return
	}
	// Without a trace reference, this slot performs the UI exemplar selection
	// operation. Report its actual kind instead of a successful detail read.
	kind = "drill-" + query.kind
	q.traceMu.Lock()
	selection, found := q.selections[query.window+"/"+query.panelID]
	q.traceMu.Unlock()
	if !found {
		// A click needs a previously loaded panel. Use this slot for a refresh;
		// the next slot can select its result, keeping one user operation per tick.
		kind = "dashboard-refresh"
		success = refresh()
		return
	}
	body, ok := queryHTTP(ctx, client, base, cookie, http.MethodPost, "/api/panels/exemplars", exemplarBody(query, selection))
	success = ok && q.exemplarResponse(body, query, selection)
}

func queryHTTP(ctx context.Context, client *http.Client, base, cookie, method, path string, body []byte) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, false
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Fanout-Request", "1")
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxQueryBody+1))
	return response, err == nil && resp.StatusCode == http.StatusOK && len(response) <= maxQueryBody && json.Valid(response)
}

func (q *queryRun) completedByKind() map[string]int64 {
	q.kindsMu.Lock()
	defer q.kindsMu.Unlock()
	completed := make(map[string]int64, len(q.kinds))
	for kind, stats := range q.kinds {
		completed[kind] = stats.hist.snapshot().Count
	}
	return completed
}

// Keep per-operation latency evidence inside the methodology string.
func (q *queryRun) operationSummary(phase string) string {
	q.kindsMu.Lock()
	defer q.kindsMu.Unlock()
	if len(q.kinds) == 0 {
		return ""
	}
	var names []string
	for name := range q.kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nRead operations for %s (fallback slots are attributed to the operation performed):\n\n| Kind | Dispatched | Completed | Failures | Canceled | p95 | p99 |\n|---|---:|---:|---:|---:|---:|---:|\n", phase)
	for _, name := range names {
		stats := q.kinds[name]
		latency := stats.hist.snapshot()
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %d | %.1fms | %.1fms |\n", name, stats.dispatched, latency.Count, stats.failures, stats.canceled, latency.P95Ms, latency.P99Ms)
	}
	return b.String()
}

type histogram struct {
	mu     sync.Mutex
	values []float64
	sum    float64
	max    float64
}

func newHistogram() *histogram { return &histogram{} }

func (h *histogram) record(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	h.mu.Lock()
	h.values = append(h.values, ms)
	h.sum += ms
	if ms > h.max {
		h.max = ms
	}
	h.mu.Unlock()
}

func (h *histogram) snapshot() Latency {
	h.mu.Lock()
	values := append([]float64(nil), h.values...)
	sum, max := h.sum, h.max
	h.mu.Unlock()
	if len(values) == 0 {
		return Latency{}
	}
	sort.Float64s(values)
	quantile := func(q float64) float64 {
		index := int(math.Ceil(q*float64(len(values)))) - 1
		if index < 0 {
			index = 0
		}
		if index >= len(values) {
			index = len(values) - 1
		}
		return round(values[index], 2)
	}
	return Latency{Count: int64(len(values)), MeanMs: round(sum/float64(len(values)), 2), P50Ms: quantile(.5), P95Ms: quantile(.95), P99Ms: quantile(.99), MaxMs: round(max, 2)}
}
