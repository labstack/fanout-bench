package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func outcomeTarget() queryTarget {
	return queryTarget{name: "dashboard-refresh", body: []byte(`{}`), panels: []string{"requests", "traffic", "latency", "hosts", "containers", "versions", "endpoints", "service_map", "text"}, dashboard: json.RawMessage(`{"panels":[{"id":"text","viz":"text"}]}`)}
}

func outcomeFixture(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/panel-outcomes.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func panelsByID(panels []DashboardPanel) map[string]DashboardPanel {
	result := map[string]DashboardPanel{}
	for _, panel := range panels {
		result[panel.ID] = panel
	}
	return result
}

func TestPanelOutcomeFixturesThroughRefresh(t *testing.T) {
	body := outcomeFixture(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/queries/run" || r.Header.Get("Accept") != "application/x-ndjson" {
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Write(body)
	}))
	defer server.Close()
	q := newQueryRun()
	q.targets = []queryTarget{outcomeTarget()}
	q.request(context.Background(), server.Client(), server.URL, "session=test", 0)
	if q.failures != 1 || q.hist.snapshot().Count != 0 {
		t.Fatalf("failed refresh escaped validation: %+v", q)
	}
	panels := q.panelSummary()
	got := panelsByID(panels)
	for id, status := range map[string]string{"requests": "ok", "traffic": "empty", "latency": "error", "hosts": "busy", "containers": "timeout", "versions": "busy", "endpoints": "timeout", "service_map": "missing"} {
		p := got[id]
		if p.Statuses[status] != 1 {
			t.Errorf("%s: %+v", id, p)
		}
		failed := int64(1)
		if status == "ok" || status == "empty" {
			failed = 0
		}
		if p.FailedRefreshes != failed {
			t.Errorf("%s failures=%d", id, p.FailedRefreshes)
		}
	}
	if len(panels) != 8 || got["requests"].CachedPercent != 100 || got["requests"].FreshToCount != 1 || *got["requests"].FreshToMinMS != 1000 || got["containers"].ElapsedMS.P95Ms != 10000 || got["traffic"].ElapsedMS.Count != 1 || got["service_map"].ElapsedMS.Count != 0 {
		t.Fatalf("incorrect evidence: %+v", panels)
	}
}

func TestPanelAggregationConcurrentAndBounded(t *testing.T) {
	q := newQueryRun()
	query := queryTarget{panels: []string{"requests"}}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := []byte(fmt.Sprintf(`{"id":"requests","status":"ok","elapsed_ms":%d,"cached":%t,"fresh_to_ms":%d}`, i+1, i%2 == 0, i))
			if _, ok := q.panelResponse(body, query); !ok {
				t.Error("valid response rejected")
			}
		}(i)
	}
	wg.Wait()
	p := q.panelSummary()[0]
	if p.Statuses["ok"] != 100 || p.Returned != 100 || p.Cached != 50 || p.CachedPercent != 50 || p.FreshToCount != 100 || *p.FreshToMinMS != 0 || *p.FreshToMaxMS != 99 || p.ElapsedMS.MeanMs != 50.5 {
		t.Fatalf("aggregate=%+v", p)
	}
	for _, v := range []struct{ got, want float64 }{{p.ElapsedMS.P50Ms, 50}, {p.ElapsedMS.P95Ms, 95}, {p.ElapsedMS.P99Ms, 99}} {
		if math.Abs(v.got-v.want) > v.want*.01+.01 {
			t.Errorf("quantile=%f want ~%f", v.got, v.want)
		}
	}
	// A snapshot is detached from future status updates.
	q.panelResponse([]byte(`{"id":"requests","status":"error","elapsed_ms":10000}`), query)
	if p.Statuses["error"] != 0 {
		t.Fatal("snapshot mutated")
	}
	for i := 0; i < 1000; i++ {
		q.panelResponse([]byte(fmt.Sprintf(`{"id":"panel-%d","status":"ok","elapsed_ms":1}`, i)), queryTarget{})
	}
	if len(q.panels) > maxPanelIDs+1 || q.panels[otherPanelID] == nil {
		t.Fatal("panel ID cardinality is unbounded")
	}
	if len(q.panels["requests"].hist.buckets) != 2048 {
		t.Fatal("histogram is not fixed size")
	}
}

func TestPanelPartialMalformedAndDuplicateResponses(t *testing.T) {
	for _, tc := range []struct {
		name, body, status string
		count              int64
	}{
		{"partial", "{\"id\":\"a\",\"status\":\"ok\",\"elapsed_ms\":42}\n{\"id\":", "ok", 1},
		{"missing", ``, "missing", 0},
		{"malformed", `broken`, "missing", 0},
		{"duplicate", `{"id":"a","status":"ok","elapsed_ms":1}
{"id":"a","status":"empty","elapsed_ms":2}`, "error", 1},
		{"unknown-status", `{"id":"a","status":"unexpected"}`, "error", 0},
		{"error-overrides-ok", `{"id":"a","status":"ok","error":"failed"}`, "error", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := newQueryRun()
			if _, ok := q.panelResponse([]byte(tc.body), queryTarget{panels: []string{"a", "b"}}); ok {
				t.Fatal("invalid response accepted")
			}
			got := panelsByID(q.panelSummary())
			if got["a"].Statuses[tc.status] != 1 || got["a"].ElapsedMS.Count != tc.count || got["b"].Statuses["missing"] != 1 {
				t.Fatalf("got=%+v", got)
			}
		})
	}
}

func TestWholeBatchRejectionAndDeadlineEvidence(t *testing.T) {
	for _, tc := range []struct {
		code         int
		body, status string
	}{
		{503, `{"code":"busy","message":"server busy"}`, "busy"},
		{503, `{"code":"unavailable"}`, "missing"},
		{504, `{"message":"deadline exceeded"}`, "timeout"},
		{500, `{"message":"failed"}`, "missing"},
	} {
		t.Run(fmt.Sprint(tc.code, tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.code); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			q := newQueryRun()
			q.targets = []queryTarget{outcomeTarget()}
			q.request(context.Background(), server.Client(), server.URL, "session=test", 0)
			if q.failures != 1 {
				t.Fatal("HTTP failure counted as success")
			}
			for _, p := range q.panelSummary() {
				if p.Statuses[tc.status] != 1 || p.ElapsedMS.Count != 0 || p.Returned != 0 {
					t.Fatalf("got=%+v", p)
				}
			}
		})
	}
}

func TestCanceledRefreshDoesNotBlameMissingPanels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := newQueryRun()
	q.targets = []queryTarget{outcomeTarget()}
	q.request(ctx, http.DefaultClient, "http://localhost:1", "session=test", 0)
	if q.failures != 0 || q.canceled != 1 {
		t.Fatalf("failures=%d canceled=%d", q.failures, q.canceled)
	}
	for _, p := range q.panelSummary() {
		if p.Statuses["missing"] != 1 || p.FailedRefreshes != 0 {
			t.Fatalf("cancellation blamed server: %+v", p)
		}
	}
}

type panelRoundTripper func(*http.Request) (*http.Response, error)

func (f panelRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type panelReadError struct{}

func (panelReadError) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestPanelReadFailureRetainsOutcomesButFailsOperation(t *testing.T) {
	// A read error after complete streamed results is still an HTTP failure,
	// even if the following annotation request succeeds.
	client := &http.Client{Transport: panelRoundTripper(func(r *http.Request) (*http.Response, error) {
		var body io.Reader = strings.NewReader(`{}`)
		if r.URL.Path == "/api/queries/run" {
			body = io.MultiReader(strings.NewReader(`{"id":"a","status":"ok","elapsed_ms":42,"from_ms":1000,"to_ms":2000}`), panelReadError{})
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(body), Header: http.Header{}}, nil
	})}
	q := newQueryRun()
	q.targets = []queryTarget{{name: "dashboard-refresh", panels: []string{"a"}, annotations: true, timePanels: map[string]bool{"a": true}}}
	q.request(context.Background(), client, "http://fanout.test", "session=test", 0)
	p := q.panelSummary()[0]
	if q.failures != 1 || p.Statuses["ok"] != 1 || p.ElapsedMS.P95Ms != 42 || p.FailedRefreshes != 0 {
		t.Fatalf("read error evidence lost: failures=%d panel=%+v", q.failures, p)
	}
}

func TestPanelDiscoveryBounds(t *testing.T) {
	for _, ids := range [][]string{{"duplicate", "duplicate"}, {strings.Repeat("x", 129)}, {otherPanelID}, make([]string, maxPanelIDs+1)} {
		panels := []map[string]string{}
		for _, id := range ids {
			panels = append(panels, map[string]string{"id": id, "viz": "stat"})
		}
		spec, _ := json.Marshal(map[string]any{"name": "System overview", "panels": panels})
		if _, _, err := systemQueryTargets(spec); err == nil {
			t.Fatalf("unbounded/invalid discovery accepted: %v", ids)
		}
	}
}

func TestPanelReportAndServerMetricArtifacts(t *testing.T) {
	metrics := `fanout_panel_query_duration_seconds_count{panel="latency",status="error",instance="a"} 2
fanout_panel_query_duration_seconds_count{panel="latency",status="error",instance="b"} 3
fanout_panel_query_duration_seconds_sum{panel="latency",status="error"} 50
fanout_panel_admission_duration_seconds_sum{panel="latency",status="admitted"} 1.5
fanout_panel_admission_duration_seconds_count{panel="latency",status="admitted"} 7
fanout_panel_admission_duration_seconds_count{panel="hosts",status="rejected"} 4
fanout_panel_admission_duration_seconds_count{panel="hosts",status="canceled"} 1
fanout_panel_admission_duration_seconds_count{panel="hosts",status="timeout"} 2
fanout_panel_admission_duration_seconds_count{panel="hosts",status="admitted"} 0
fanout_panel_query_duration_seconds_bucket{panel="latency",status="error",le="10"} 5
`
	snap, err := parsePrometheus(strings.NewReader(metrics))
	if err != nil {
		t.Fatal(err)
	}
	sample := resourceSample(snap, 1)
	want := PanelMetrics{
		"fanout_panel_query_duration_seconds_count":     {"latency": {"error": 5}},
		"fanout_panel_query_duration_seconds_sum":       {"latency": {"error": 50}},
		"fanout_panel_admission_duration_seconds_sum":   {"latency": {"admitted": 1.5}},
		"fanout_panel_admission_duration_seconds_count": {"latency": {"admitted": 7}, "hosts": {"rejected": 4, "canceled": 1, "timeout": 2, "admitted": 0}},
	}
	if !reflect.DeepEqual(sample.PanelMetrics, want) {
		t.Fatalf("samples=%+v", sample.PanelMetrics)
	}
	data, _ := json.Marshal(resourceSample(&snapshot{}, 0))
	if strings.Contains(string(data), "panel_metrics") {
		t.Fatal("a sample without panel metrics must omit them")
	}
	q := newQueryRun()
	q.panelResponse(outcomeFixture(t), outcomeTarget())
	report := fixtureReport()
	report.Trials = []Trial{{Spec: PhaseSpec{Name: "mixed-sustained", Kind: "sustained"}, DashboardPanels: q.panelSummary(), Samples: []ResourceSample{sample}}, {Spec: PhaseSpec{Name: "mixed-sweep", Kind: "sweep"}, DashboardPanels: q.panelSummary()}}
	dir := t.TempDir()
	if err := WriteArtifacts(dir, report); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadReport(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(report.Trials, restored.Trials) {
		t.Fatal("panel evidence lost in JSON roundtrip")
	}
	md, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Dashboard panels — mixed-sustained", "Dashboard panels — mixed-sweep", "| containers ⚠ |", "| service_map ⚠ |", "| requests | 1 | 0 |", "Cached %", "Failed refreshes", "n/a"} {
		if !strings.Contains(string(md), fragment) {
			t.Errorf("Markdown missing %q", fragment)
		}
	}
	if strings.Contains(string(md), "| requests ⚠ |") {
		t.Error("healthy panel flagged as failure")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "samples.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Trial string
		ResourceSample
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	if row.Trial != "mixed-sustained" || !reflect.DeepEqual(row.PanelMetrics, want) {
		t.Fatalf("JSONL evidence=%+v", row)
	}
}
