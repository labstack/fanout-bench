package bench

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var queryTargets = []string{
	"/api/observability/overview?window=15m&limit=100",
	"/api/observability/topology?window=1h&limit=100",
	"/api/observability/performance?window=6h&limit=100",
	"/api/observability/trace?window=12h&limit=100",
	"/api/observability/logs?window=24h&limit=100",
}

type queryRun struct {
	wg       sync.WaitGroup
	hist     *histogram
	failures int64
	attempts int64
	shed     int64
	canceled int64
}

func newQueryRun() *queryRun { return &queryRun{hist: newHistogram()} }

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
	jobs := make(chan int)
	q.wg.Add(workers + 1)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer q.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case targetIndex := <-jobs:
					q.request(ctx, client, base, cookie, targetIndex)
				}
			}
		}()
	}
	go func() {
		defer q.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		targetIndex := 0
		for {
			select {
			case <-ctx.Done():
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

func (q *queryRun) request(ctx context.Context, client *http.Client, base, cookie string, targetIndex int) {
	target := strings.TrimRight(base, "/") + queryTargets[targetIndex%len(queryTargets)]
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		atomic.AddInt64(&q.failures, 1)
		return
	}
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			atomic.AddInt64(&q.canceled, 1)
		} else {
			atomic.AddInt64(&q.failures, 1)
		}
		return
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || readErr != nil || !json.Valid(body) {
		atomic.AddInt64(&q.failures, 1)
		return
	}
	q.hist.record(time.Since(started))
}

func (q *queryRun) wait() { q.wg.Wait() }

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
