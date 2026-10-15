package bench

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var queryWindows = []string{"15m", "1h", "6h", "12h", "24h"}

type queryTarget struct {
	name, window  string
	body          []byte
	dashboard     json.RawMessage
	panels        []string
	timePanels    map[string]bool
	drills        map[string]string
	panelID, kind string
	annotations   bool
}

// Read the owner's built-in dashboard once before measurement. Passing its
// complete spec preserves the release's SQL, variables and panel definitions.
func loadQueryTargets(ctx context.Context, client *http.Client, base, cookie string) ([]queryTarget, string, error) {
	get := func(path string, value any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Cookie", cookie)
		req.Header.Set("Fanout-Request", "1")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("dashboard discovery status %d", resp.StatusCode)
		}
		return json.NewDecoder(io.LimitReader(resp.Body, 512<<10)).Decode(value)
	}
	var list struct {
		Dashboards []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			IsDefault bool   `json:"is_default"`
		} `json:"dashboards"`
	}
	if err := get("/api/dashboards", &list); err != nil {
		return nil, "", err
	}
	for _, item := range list.Dashboards {
		if !item.IsDefault || item.Name != "System overview" {
			continue
		}
		var record struct {
			Spec json.RawMessage `json:"spec"`
		}
		if err := get("/api/dashboards/"+url.PathEscape(item.ID), &record); err != nil {
			return nil, "", err
		}
		return systemQueryTargets(record.Spec)
	}
	return nil, "", errors.New("built-in System overview dashboard not found")
}

// page.tsx initially makes every panel visible. Model that full-dashboard
// load at a fixed 1664px viewport (1600px container), including all widths.
func systemQueryTargets(spec json.RawMessage) ([]queryTarget, string, error) {
	var dashboard struct {
		Name string `json:"name"`
		Time struct {
			Refresh string `json:"refresh"`
			Compare string `json:"compare"`
		} `json:"time"`
		Annotations struct {
			Deploys   *bool `json:"deploys"`
			Anomalies *bool `json:"anomalies"`
		} `json:"annotations"`
		Panels []struct {
			ID    string `json:"id"`
			Viz   string `json:"viz"`
			Width int    `json:"width"`
			Grid  *struct {
				W int `json:"w"`
			} `json:"grid"`
		} `json:"panels"`
	}
	if err := json.Unmarshal(spec, &dashboard); err != nil {
		return nil, "", err
	}
	if dashboard.Name != "System overview" || len(dashboard.Panels) == 0 {
		return nil, "", errors.New("invalid System overview spec")
	}
	ids := []string{}
	widths := map[string]int{}
	timePanels := map[string]bool{}
	drills := map[string]string{}
	var drillIDs []string
	for _, p := range dashboard.Panels {
		ids = append(ids, p.ID)
		width := p.Width
		if width == 0 {
			width = 6
		}
		if p.Grid != nil {
			width = p.Grid.W
		}
		// use-panel-results.ts rounds to the nearest 100, with a 100px minimum.
		widths[p.ID] = max(100, int(float64(1600*width)/1200+.5)*100)
		timePanels[p.ID] = p.Viz == "timeseries" || p.Viz == "heatmap" || p.Viz == "state_timeline"
		if p.Viz == "service_map" {
			drills[p.ID] = "traces"
			drillIDs = append(drillIDs, p.ID)
		}
	}
	sort.Strings(ids)
	refresh := dashboard.Time.Refresh
	if refresh == "" {
		refresh = "30s"
	}
	annotations := !(dashboard.Annotations.Deploys != nil && !*dashboard.Annotations.Deploys && dashboard.Annotations.Anomalies != nil && !*dashboard.Annotations.Anomalies)
	var targets []queryTarget
	kinds := []string{"dashboard-refresh"}
	for _, id := range drillIDs {
		kinds = append(kinds, "drill-"+drills[id]+" ("+id+")")
	}
	if len(drillIDs) > 0 {
		kinds = append(kinds, "trace-detail")
	}
	for _, window := range queryWindows {
		body, _ := json.Marshal(map[string]any{
			"dashboard": spec, "panels": ids, "time": map[string]string{"range": window, "refresh": refresh},
			"vars": map[string]string{"service": "$__all"}, "widths": widths, "compare": dashboard.Time.Compare == "previous_period",
		})
		base := queryTarget{window: window, body: body, dashboard: spec, panels: ids, timePanels: timePanels, drills: drills, annotations: annotations}
		base.name = "dashboard-refresh"
		targets = append(targets, base)
		for _, id := range drillIDs {
			drill := base
			drill.name, drill.panelID, drill.kind = "drill-"+drills[id], id, drills[id]
			targets = append(targets, drill)
		}
		if len(drillIDs) > 0 {
			detail := base
			detail.name, detail.panelID, detail.kind = "trace-detail", drillIDs[0], drills[drillIDs[0]]
			targets = append(targets, detail)
		}
	}
	digest := sha256.Sum256(spec)
	method := fmt.Sprintf(" Reads rotate equally through user operations %s at 15m, 1h, 6h, 12h and 24h. Dashboard refresh models the initial all-visible panel batch (sorted IDs, a 1664px viewport, rounded layout widths, service All, comparison per spec), followed by annotations over returned time-panel bounds. Drill-down selects the first service-map node over its returned full window. Trace detail reuses the discovered trace's namespace and absolute window; without a reference its slot performs drill-down; without a cached selection the slot performs dashboard refresh instead. Each scheduled read measures all requests in its operation and fails on any request or panel error, missing panel, or empty discovered trace detail. No standalone trace search or log search is issued by this UI. System overview spec SHA-256: %x.", strings.Join(kinds, ", "), digest)
	return targets, method, nil
}

type traceReference struct{ id, namespace, from, to string }
type drillSelection struct {
	from, to   string
	dimensions map[string]string
}

func traceDetailPath(trace traceReference) string {
	params := url.Values{"namespace": {trace.namespace}, "from": {trace.from}, "to": {trace.to}, "limit": {"200"}}
	return "/api/traces/" + url.PathEscape(trace.id) + "?" + params.Encode()
}

func exemplarBody(query queryTarget, selection drillSelection) []byte {
	body, _ := json.Marshal(map[string]any{
		"dashboard": query.dashboard, "panel_id": query.panelID, "kind": query.kind,
		"time": map[string]string{"from": selection.from, "to": selection.to, "refresh": "off"},
		"from": selection.from, "to": selection.to, "dimensions": selection.dimensions,
		"vars": map[string]string{"service": "$__all"},
	})
	return body
}

// JS Date.toISOString() always includes milliseconds.
func uiTime(ms int64) string { return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z") }

type panelResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error"`
	From   *int64 `json:"from_ms"`
	To     *int64 `json:"to_ms"`
	Frame  *struct {
		Columns []struct {
			Name string `json:"name"`
		} `json:"columns"`
		Values [][]json.RawMessage `json:"values"`
	} `json:"frame"`
}

func (q *queryRun) panelResponse(body []byte, query queryTarget) ([]byte, bool) {
	var response struct {
		Results []panelResult `json:"results"`
	}
	if json.Unmarshal(body, &response) != nil || len(response.Results) == 0 {
		return nil, false
	}
	seen := map[string]bool{}
	valid := true
	var from, to int64
	for _, result := range response.Results {
		if seen[result.ID] || (result.Status != "ok" && result.Status != "empty") || result.Error != "" {
			valid = false
		}
		seen[result.ID] = true
		if result.From == nil || result.To == nil || *result.From >= *result.To {
			continue
		}
		if query.timePanels[result.ID] {
			if from == 0 || *result.From < from {
				from = *result.From
			}
			if *result.To > to {
				to = *result.To
			}
		}
		if _, drill := query.drills[result.ID]; !drill || result.Frame == nil || result.Status != "ok" || result.Error != "" {
			continue
		}
		selection := drillSelection{from: uiTime(*result.From), to: uiTime(*result.To), dimensions: map[string]string{}}
		// service_map's UI selection contains {service} from the selected node.
		serviceColumn, kindColumn := -1, -1
		for i, column := range result.Frame.Columns {
			if column.Name == "service" {
				serviceColumn = i
			}
			if column.Name == "kind" {
				kindColumn = i
			}
		}
		if serviceColumn >= 0 && serviceColumn < len(result.Frame.Values) {
			for row, cell := range result.Frame.Values[serviceColumn] {
				var service, kind string
				_ = json.Unmarshal(cell, &service)
				if kindColumn >= 0 && kindColumn < len(result.Frame.Values) && row < len(result.Frame.Values[kindColumn]) {
					_ = json.Unmarshal(result.Frame.Values[kindColumn][row], &kind)
				}
				if service != "" && (kindColumn < 0 || kind == "node") {
					selection.dimensions["service"] = service
					break
				}
			}
		}
		if len(selection.dimensions) > 0 {
			q.traceMu.Lock()
			q.selections[query.window+"/"+result.ID] = selection
			q.traceMu.Unlock()
		}
	}
	var dashboard struct{ Panels []struct{ ID, Viz string } }
	_ = json.Unmarshal(query.dashboard, &dashboard)
	for _, id := range query.panels {
		// Text panels are omitted from API results.
		text := false
		for _, p := range dashboard.Panels {
			if p.ID == id && p.Viz == "text" {
				text = true
			}
		}
		if !seen[id] && !text {
			valid = false
		}
	}
	if query.annotations && from < to {
		body, _ := json.Marshal(map[string]string{"from": uiTime(from), "to": uiTime(to)})
		return body, valid
	}
	return nil, valid
}

func (q *queryRun) exemplarResponse(body []byte, query queryTarget, selection drillSelection) bool {
	var response struct {
		Traces []struct {
			ID        string `json:"trace_id"`
			Namespace string `json:"namespace"`
		} `json:"traces"`
		Logs json.RawMessage `json:"logs"`
	}
	if json.Unmarshal(body, &response) != nil {
		return false
	}
	if query.kind == "logs" {
		return len(response.Logs) > 0 && string(response.Logs) != "null"
	}
	if response.Traces == nil {
		return false
	}
	for _, trace := range response.Traces {
		if trace.ID == "" {
			continue
		}
		q.traceMu.Lock()
		q.traces[query.window] = traceReference{trace.ID, trace.Namespace, selection.from, selection.to}
		q.traceMu.Unlock()
		break
	}
	return true
}

func traceDetailResponse(body []byte) bool {
	var response struct {
		Data struct {
			Spans []json.RawMessage `json:"spans"`
		} `json:"data"`
	}
	return json.Unmarshal(body, &response) == nil && len(response.Data.Spans) > 0
}
