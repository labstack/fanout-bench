package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

const maxPanelIDs = 128
const otherPanelID = "__other__"

// DashboardPanel contains bounded, phase-local evidence, including unsuccessful
// refreshes. ElapsedMS is the server duration, not the HTTP operation latency.
type DashboardPanel struct {
	ID              string           `json:"id"`
	Statuses        map[string]int64 `json:"statuses"`
	Returned        int64            `json:"returned"`
	Cached          int64            `json:"cached"`
	CachedPercent   float64          `json:"cached_percent"`
	FailedRefreshes int64            `json:"failed_refreshes"`
	ElapsedMS       Latency          `json:"elapsed_ms"`
	FreshToCount    int64            `json:"fresh_to_count"`
	FreshToMinMS    *int64           `json:"fresh_to_min_ms,omitempty"`
	FreshToMaxMS    *int64           `json:"fresh_to_max_ms,omitempty"`
}

type panelStats struct {
	DashboardPanel
	hist *histogram
}

func panelStatus(r panelResult) string {
	// The current server returns status=error, reason=busy for admission
	// failures, and presents execution deadlines as a public error message.
	if r.Reason == "busy" || r.Status == "busy" {
		return "busy"
	}
	if r.Reason == "timeout" || r.Status == "timeout" ||
		strings.HasPrefix(r.Error, "The query took longer than ") ||
		strings.HasPrefix(r.Error, "Panels took longer than ") ||
		r.Error == "context deadline exceeded" {
		return "timeout"
	}
	if r.Error != "" {
		return "error"
	}
	switch r.Status {
	case "ok", "empty", "error", "missing":
		return r.Status
	default:
		return "error"
	}
}

// decodePanelResults reads the API's one-result-per-line stream. A truncated
// stream retains its complete results; the remaining panels are missing, and
// the refresh still fails validation.
func decodePanelResults(body []byte) ([]panelResult, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	var results []panelResult
	for {
		var result panelResult
		err := decoder.Decode(&result)
		if err == io.EOF {
			return results, len(results) > 0
		}
		if err != nil || result.Status == "" {
			return results, false
		}
		results = append(results, result)
	}
}

// The panel endpoint opts into the UI's stream protocol. HTTP failures have no server
// elapsed_ms, so they never contribute invented latency observations.
func queryPanelHTTP(ctx context.Context, client *http.Client, base, cookie, path string, body []byte) ([]byte, bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, false, "missing"
	}
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Fanout-Request", "1")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/x-ndjson")
	resp, err := client.Do(req)
	if err != nil {
		return nil, false, "missing"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		status := "missing"
		if resp.StatusCode == http.StatusServiceUnavailable {
			// Only Fanout's explicit busy code attributes rejection to panels.
			var failure struct {
				Code string `json:"code"`
			}
			_ = json.NewDecoder(io.LimitReader(resp.Body, maxQueryBody)).Decode(&failure)
			if failure.Code == "busy" {
				status = "busy"
			}
		} else if resp.StatusCode == http.StatusGatewayTimeout {
			status = "timeout"
		}
		return nil, false, status
	}
	response, err := io.ReadAll(io.LimitReader(resp.Body, maxQueryBody+1))
	if len(response) > maxQueryBody {
		return nil, false, "missing"
	}
	return response, err == nil, "missing"
}

func (q *queryRun) recordPanels(query queryTarget, results map[string]panelResult, absent string, attributeFailures bool) {
	text := map[string]bool{}
	expected := map[string]bool{}
	for _, id := range query.panels {
		expected[id] = true
	}
	var dashboard struct{ Panels []struct{ ID, Viz string } }
	_ = json.Unmarshal(query.dashboard, &dashboard)
	for _, p := range dashboard.Panels {
		text[p.ID] = p.Viz == "text"
	}
	q.panelsMu.Lock()
	defer q.panelsMu.Unlock()
	record := func(id string, result panelResult, returned bool) {
		if len(id) > 128 || id == "" || len(expected) > 0 && !expected[id] {
			id = otherPanelID
		}
		stats := q.panels[id]
		if stats == nil {
			named := len(q.panels)
			if q.panels[otherPanelID] != nil {
				named--
			}
			if named >= maxPanelIDs && id != otherPanelID {
				id = otherPanelID
			}
			stats = q.panels[id]
			if stats == nil {
				stats = &panelStats{DashboardPanel: DashboardPanel{ID: id, Statuses: map[string]int64{}}, hist: newHistogram()}
				q.panels[id] = stats
			}
		}
		stats.Statuses[result.Status]++
		if attributeFailures && result.Status != "ok" && result.Status != "empty" {
			stats.FailedRefreshes++
		}
		if !returned {
			return
		}
		stats.Returned++
		if result.Cached {
			stats.Cached++
		}
		if result.ElapsedMS != nil {
			stats.hist.recordMS(*result.ElapsedMS)
		}
		if result.FreshToMS != nil {
			v := *result.FreshToMS
			stats.FreshToCount++
			if stats.FreshToMinMS == nil || v < *stats.FreshToMinMS {
				stats.FreshToMinMS = &v
			}
			if stats.FreshToMaxMS == nil || v > *stats.FreshToMaxMS {
				stats.FreshToMaxMS = &v
			}
		}
	}
	for id, result := range results {
		record(id, result, true)
	}
	for _, id := range query.panels {
		if _, found := results[id]; !found && !text[id] {
			record(id, panelResult{Status: absent}, false)
		}
	}
}

func (q *queryRun) panelSummary() []DashboardPanel {
	q.panelsMu.Lock()
	defer q.panelsMu.Unlock()
	var panels []DashboardPanel
	for _, stats := range q.panels {
		panel := stats.DashboardPanel
		panel.Statuses = make(map[string]int64)
		for _, status := range []string{"ok", "empty", "busy", "error", "timeout", "missing"} {
			panel.Statuses[status] = stats.Statuses[status]
		}
		panel.ElapsedMS = stats.hist.snapshot()
		if panel.Returned > 0 {
			panel.CachedPercent = round(100*float64(panel.Cached)/float64(panel.Returned), 2)
		}
		panels = append(panels, panel)
	}
	sort.Slice(panels, func(i, j int) bool { return panels[i].ID < panels[j].ID })
	return panels
}

func renderDashboardPanels(b *strings.Builder, trials []Trial) {
	for _, trial := range trials {
		if len(trial.DashboardPanels) == 0 {
			continue
		}
		fmt.Fprintf(b, "\n### Dashboard panels — %s\n\nServer elapsed_ms includes returned failures and cache hits; missing durations are excluded. Cached %% uses returned panel results. Quantiles use fixed buckets (~1%% plus 0.01ms precision).\n\n", trial.Spec.Name)
		fmt.Fprint(b, "| Panel id | ok | empty | busy | error | timeout | missing | Cached % | p50 | p95 | p99 | Failed refreshes |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|\n")
		for _, p := range trial.DashboardPanels {
			id := strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(p.ID)
			if trial.Spec.Kind == "sustained" && p.FailedRefreshes > 0 {
				id += " ⚠"
			}
			fmt.Fprintf(b, "| %s | %d | %d | %d | %d | %d | %d | %.1f%% | %s | %s | %s | %d |\n", id, p.Statuses["ok"], p.Statuses["empty"], p.Statuses["busy"], p.Statuses["error"], p.Statuses["timeout"], p.Statuses["missing"], p.CachedPercent, panelQuantile(p.ElapsedMS.Count, p.ElapsedMS.P50Ms), panelQuantile(p.ElapsedMS.Count, p.ElapsedMS.P95Ms), panelQuantile(p.ElapsedMS.Count, p.ElapsedMS.P99Ms), p.FailedRefreshes)
		}
		if trial.Spec.Kind == "sustained" {
			fmt.Fprint(b, "\n⚠ Panel caused failed sustained refreshes. Counts can overlap when multiple panels fail in one refresh. Request or annotation failures may also fail a refresh.\n")
		}
	}
}

func panelQuantile(count int64, ms float64) string {
	if count == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1fms", ms)
}

// Preserve both bounded server labels. Admission histogram counts are the
// admitted/rejected/canceled/timeout counters; sums measure admission wait.
type PanelMetrics map[string]map[string]map[string]float64

func panelMetricSamples(s *snapshot) PanelMetrics {
	var result PanelMetrics
	if s == nil {
		return nil
	}
	for _, v := range s.samples {
		switch v.name {
		case "fanout_panel_query_duration_seconds_count", "fanout_panel_query_duration_seconds_sum",
			"fanout_panel_admission_duration_seconds_count", "fanout_panel_admission_duration_seconds_sum":
		default:
			continue
		}
		panel, status := v.labels["panel"], v.labels["status"]
		if panel == "" || status == "" {
			continue
		}
		if result == nil {
			result = map[string]map[string]map[string]float64{}
		}
		if result[v.name] == nil {
			result[v.name] = map[string]map[string]float64{}
		}
		if result[v.name][panel] == nil {
			result[v.name][panel] = map[string]float64{}
		}
		result[v.name][panel][status] += v.value
	}
	return result
}
