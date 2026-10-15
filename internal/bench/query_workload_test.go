package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func systemSpec(t *testing.T) []byte {
	t.Helper()
	// DefaultSpec from Fanout 870c90db, including defaults_sql.go and spec.go.
	spec, err := os.ReadFile("testdata/system-overview-v2026.10.1-alpha.json")
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func decodeBody(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestSystemOperationsMatchUIRequestConstruction(t *testing.T) {
	spec := systemSpec(t)
	targets, method, err := systemQueryTargets(spec)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{"check_latency", "check_states", "containers", "endpoints", "error_logs", "error_services", "errors", "hosts", "latency", "latency_p95", "requests", "service_health", "service_map", "traffic", "versions"}
	if len(targets) != 15 {
		t.Fatalf("targets=%d", len(targets))
	}
	for w, window := range queryWindows {
		refresh, drill, detail := targets[w*3], targets[w*3+1], targets[w*3+2]
		if refresh.name != "dashboard-refresh" || drill.name != "drill-traces" || drill.panelID != "service_map" || detail.name != "trace-detail" {
			t.Fatalf("incorrect operations: %+v", targets[w*3:w*3+3])
		}
		widths := map[string]any{}
		panelIDs := []any{}
		for _, id := range ids {
			widths[id] = float64(800)
			panelIDs = append(panelIDs, id)
		}
		for _, id := range []string{"check_latency", "check_states", "containers", "hosts", "service_health", "service_map", "versions"} {
			widths[id] = float64(1600)
		}
		widths["error_services"], widths["endpoints"] = float64(500), float64(1100)
		want := map[string]any{"dashboard": decodeBody(t, spec), "panels": panelIDs, "time": map[string]any{"range": window, "refresh": "30s"}, "vars": map[string]any{"service": "$__all"}, "widths": widths, "compare": false}
		if got := decodeBody(t, refresh.body); !reflect.DeepEqual(got, want) {
			t.Fatalf("UI panel batch mismatch: got %v want %v", got, want)
		}
		selection := drillSelection{from: "2026-10-01T12:00:00.000Z", to: "2026-10-01T13:00:00.000Z", dimensions: map[string]string{"service": "checkout"}}
		wantDrill := map[string]any{"dashboard": decodeBody(t, spec), "panel_id": "service_map", "kind": "traces", "time": map[string]any{"from": selection.from, "to": selection.to, "refresh": "off"}, "from": selection.from, "to": selection.to, "dimensions": map[string]any{"service": "checkout"}, "vars": map[string]any{"service": "$__all"}}
		if got := decodeBody(t, exemplarBody(drill, selection)); !reflect.DeepEqual(got, wantDrill) {
			t.Fatalf("UI exemplar mismatch: %v", got)
		}
		// drill.tsx sends logs only for a logs-source panel with a logs selection.
		drill.kind, drill.panelID = "logs", "error_logs"
		wantDrill["kind"], wantDrill["panel_id"] = "logs", "error_logs"
		if got := decodeBody(t, exemplarBody(drill, selection)); !reflect.DeepEqual(got, wantDrill) {
			t.Fatalf("UI logs exemplar mismatch: %v", got)
		}
	}
	for _, name := range []string{"dashboard-refresh", "drill-traces", "trace-detail", "SHA-256"} {
		if !strings.Contains(method, name) {
			t.Errorf("methodology omits %s", name)
		}
	}
}

// Mirrors a panel batch response, including the effective window from discovery
// and a selectable service-map node. No invented search dashboard is needed.
func panelBatch(t *testing.T, query queryTarget, from, to int64) []byte {
	t.Helper()
	results := []any{}
	for _, id := range query.panels {
		result := map[string]any{"id": id, "status": "empty", "from_ms": from, "to_ms": to}
		if id == "service_map" {
			result["status"] = "ok"
			result["frame"] = map[string]any{"columns": []any{map[string]string{"name": "kind"}, map[string]string{"name": "service"}}, "values": [][]string{{"node"}, {"checkout"}}}
		}
		results = append(results, result)
	}
	body, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestWorkloadDiscoversAuthenticatedBuiltInDashboard(t *testing.T) {
	spec := systemSpec(t)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("Cookie") != "fanout_session=test" || r.Method != http.MethodGet {
			t.Error("discovery missing authentication")
		}
		switch r.URL.Path {
		case "/api/dashboards":
			fmt.Fprint(w, `{"dashboards":[{"id":"system","name":"System overview","is_default":true}]}`)
		case "/api/dashboards/system":
			_ = json.NewEncoder(w).Encode(map[string]json.RawMessage{"spec": spec})
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	targets, _, err := loadQueryTargets(context.Background(), server.Client(), server.URL, "fanout_session=test")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(targets) != 15 {
		t.Fatalf("requests=%d targets=%d", requests, len(targets))
	}
}

func TestRefreshIncludesAnnotationsAndMeasuresWholeOperation(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost || r.Header.Get("Cookie") != "fanout_session=test" || r.Header.Get("Fanout-Request") != "1" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("UI headers/method missing")
		}
		time.Sleep(15 * time.Millisecond)
		switch r.URL.Path {
		case "/api/panels/query":
			w.Write(panelBatch(t, targets[0], 1000, 2000))
		case "/api/annotations":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !reflect.DeepEqual(body, map[string]string{"from": "1970-01-01T00:00:01.000Z", "to": "1970-01-01T00:00:02.000Z"}) {
				t.Errorf("annotation body=%v", body)
			}
			fmt.Fprint(w, `{"deploys":[],"anomalies":[]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	q := newQueryRun()
	q.targets = targets
	q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", 0)
	if !reflect.DeepEqual(paths, []string{"/api/panels/query", "/api/annotations"}) || q.failures != 0 || q.hist.snapshot().Count != 1 || q.hist.snapshot().MeanMs < 30 {
		t.Fatalf("paths=%v failures=%d latency=%+v", paths, q.failures, q.hist.snapshot())
	}
	if summary := q.operationSummary("mixed"); !strings.Contains(summary, "| dashboard-refresh | 1 | 1 | 0 |") {
		t.Errorf("missing kind evidence: %s", summary)
	}
}

func TestTraceReferencesRemainScopedToDiscoveryWindow(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	q := newQueryRun()
	q.targets = targets
	bounds := map[string]drillSelection{}
	for i, window := range queryWindows {
		from, to := int64(1000+i*10000), int64(2000+i*10000)
		if _, ok := q.panelResponse(panelBatch(t, targets[i*3], from, to), targets[i*3]); !ok {
			t.Fatal("invalid panel batch")
		}
		selection := q.selections[window+"/service_map"]
		bounds[window] = selection
		// Exemplar discovery preserves the absolute panel-response bounds.
		body := []byte(fmt.Sprintf(`{"traces":[{"trace_id":"trace-%s","namespace":"bench namespace"}]}`, window))
		if !q.exemplarResponse(body, targets[i*3+1], selection) {
			t.Fatal("invalid exemplars")
		}
	}
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		window := queryWindows[count]
		count++
		params := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/api/traces/trace-"+window || params.Get("namespace") != "bench namespace" || params.Get("from") != bounds[window].from || params.Get("to") != bounds[window].to || params.Get("limit") != "200" || len(params) != 4 || r.Header.Get("Fanout-Request") != "1" {
			t.Errorf("incorrect detail request %s", r.URL)
		}
		fmt.Fprint(w, `{"data":{"spans":[{"span_id":"span"}]}}`)
	}))
	defer server.Close()
	for i := range queryWindows {
		q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", i*3+2)
	}
	if q.failures != 0 || q.hist.snapshot().Count != 5 {
		t.Fatalf("failures=%d completed=%d", q.failures, q.hist.snapshot().Count)
	}
}

func TestTraceDetailFallsBackOnlyWithinItsOwnWindow(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	q := newQueryRun()
	q.targets = targets
	q.traces["15m"] = traceReference{"other-window", "ns", "old-from", "old-to"}
	q.selections["1h/service_map"] = drillSelection{"2026-10-01T12:00:00.000Z", "2026-10-01T13:00:00.000Z", map[string]string{"service": "checkout"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/panels/exemplars" {
			t.Errorf("wrong fallback path %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !reflect.DeepEqual(body, decodeBody(t, exemplarBody(targets[4], q.selections["1h/service_map"]))) {
			t.Error("incorrect fallback exemplar body")
		}
		fmt.Fprint(w, `{"traces":[{"trace_id":"this-window","namespace":"ns"}]}`)
	}))
	defer server.Close()
	q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", 5)
	if q.failures != 0 || q.traces["1h"].id != "this-window" || q.traces["15m"].id != "other-window" {
		t.Fatalf("traces=%v failures=%d", q.traces, q.failures)
	}
	if !strings.Contains(q.operationSummary("mixed"), "| drill-traces | 1 | 1 |") {
		t.Fatal("fallback reported as detail")
	}
}

func TestSustainedQueryGateFailsWhenExemplarsStayEmpty(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/panels/query":
			w.Write(panelBatch(t, targets[0], 1000, 2000))
		case "/api/annotations":
			fmt.Fprint(w, `{}`)
		case "/api/panels/exemplars":
			fmt.Fprint(w, `{"traces":[]}`)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	q := newQueryRun()
	q.targets = targets
	for i := 0; i < 2*len(targets); i++ {
		q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", i)
	}
	trial := Trial{
		Spec:           PhaseSpec{Kind: "sustained", Signal: Mixed, QueryRate: 30},
		ElapsedSeconds: 1, Accepted: Counts{Spans: 1},
		QueryLatency: q.hist.snapshot(), QueryFailures: q.failures,
		QueryKindCompleted: q.completedByKind(),
	}
	if trial.QueryLatency.Count != 30 || trial.QueryFailures != 0 || trial.QueryKindCompleted["dashboard-refresh"] != 10 || trial.QueryKindCompleted["drill-traces"] != 20 || trial.QueryKindCompleted["trace-detail"] != 0 {
		t.Fatalf("unexpected operation counts: %+v", trial)
	}
	if queryTargetMet(trial) {
		t.Fatal("query target met without a successful trace-detail operation")
	}
	summary := summarize(Report{Settings: Settings{QueryRate: 30}, Trials: []Trial{trial}}, 0, 0)
	for _, gate := range summary.Gates {
		if gate.Name == "sustained-query-load" {
			if gate.Passed || summary.Passed || !strings.Contains(gate.Detail, "zero successful operations: trace-detail") {
				t.Fatalf("missing trace detail did not fail the gate: %+v", gate)
			}
			return
		}
	}
	t.Fatal("sustained query gate missing")
}

func TestEmptyDiscoveredTraceDetailFails(t *testing.T) {
	for _, response := range []string{`{"data":{"spans":[]}}`, `{"data":{}}`, `{"data":{"spans":null}}`} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, response) }))
			defer server.Close()
			q := newQueryRun()
			q.targets = []queryTarget{{name: "trace-detail", window: "1h"}}
			q.traces["1h"] = traceReference{"trace", "ns", "2026-10-01T12:00:00.000Z", "2026-10-01T13:00:00.000Z"}
			q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", 0)
			if q.failures != 1 || q.hist.snapshot().Count != 0 {
				t.Fatal("empty detail counted as success")
			}
		})
	}
}

func TestAnyOperationRequestOrPanelErrorFails(t *testing.T) {
	for _, defect := range []string{"panel-error", "missing-panel", "annotation-http", "annotation-json", "exemplar-http"} {
		t.Run(defect, func(t *testing.T) {
			targets, _, _ := systemQueryTargets(systemSpec(t))
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/panels/query":
					body := panelBatch(t, targets[0], 1000, 2000)
					if defect == "panel-error" {
						body = []byte(strings.Replace(string(body), `"status":"empty"`, `"status":"error"`, 1))
					}
					if defect == "missing-panel" {
						body = []byte(`{"results":[{"id":"traffic","status":"empty"}]}`)
					}
					w.Write(body)
				case "/api/annotations":
					if defect == "annotation-http" {
						http.Error(w, "failed", 500)
					} else if defect == "annotation-json" {
						fmt.Fprint(w, `broken`)
					} else {
						fmt.Fprint(w, `{}`)
					}
				case "/api/panels/exemplars":
					http.Error(w, "failed", 500)
				default:
					t.Errorf("unexpected route %s", r.URL.Path)
				}
			}))
			defer server.Close()
			q := newQueryRun()
			q.targets = targets
			index := 0
			if defect == "exemplar-http" {
				index = 1
				q.selections["15m/service_map"] = drillSelection{"2026-10-01T12:00:00.000Z", "2026-10-01T13:00:00.000Z", map[string]string{"service": "checkout"}}
			}
			q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", index)
			if q.failures != 1 || q.hist.snapshot().Count != 0 {
				t.Fatalf("failures=%d completed=%d", q.failures, q.hist.snapshot().Count)
			}
		})
	}
}

func TestEmptyDashboardDoesNotInventDrillSelection(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path == "/api/annotations" {
			fmt.Fprint(w, `{}`)
			return
		}
		if r.URL.Path != "/api/panels/query" {
			t.Errorf("invented selection: %s", r.URL.Path)
		}
		body := decodeBody(t, panelBatch(t, targets[0], 1000, 2000))
		for _, raw := range body["results"].([]any) {
			result := raw.(map[string]any)
			delete(result, "frame")
			result["status"] = "empty"
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()
	q := newQueryRun()
	q.targets = targets
	q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", 2)
	if requests != 2 || q.failures != 0 || q.hist.snapshot().Count != 1 || !strings.Contains(q.operationSummary("mixed"), "| dashboard-refresh | 1 | 1 |") {
		t.Fatalf("requests=%d failures=%d summary=%s", requests, q.failures, q.operationSummary("mixed"))
	}
}

func TestAnnotationWindowUnionAndDisabledAnnotations(t *testing.T) {
	query := queryTarget{timePanels: map[string]bool{"traffic": true, "latency": true}, annotations: true}
	body := []byte(`{"results":[{"id":"traffic","status":"empty","from_ms":2000,"to_ms":5000},{"id":"latency","status":"empty","from_ms":1000,"to_ms":3000},{"id":"requests","status":"empty","from_ms":0,"to_ms":9000}]}`)
	q := newQueryRun()
	annotations, ok := q.panelResponse(body, query)
	if !ok || !reflect.DeepEqual(decodeBody(t, annotations), map[string]any{"from": "1970-01-01T00:00:01.000Z", "to": "1970-01-01T00:00:05.000Z"}) {
		t.Fatalf("annotations=%s ok=%t", annotations, ok)
	}
	query.annotations = false
	if annotations, ok = q.panelResponse(body, query); !ok || annotations != nil {
		t.Fatal("disabled annotations requested")
	}
	query.annotations, query.timePanels = true, nil
	if annotations, ok = q.panelResponse(body, query); !ok || annotations != nil {
		t.Fatal("annotations requested without time panels")
	}
}

func TestUIBatchIncludesTextIDsAndRoundedGridWidths(t *testing.T) {
	spec := []byte(`{"name":"System overview","panels":[{"id":"z","viz":"text","width":12},{"id":"a","viz":"stat","width":8,"grid":{"w":1}}],"time":{"refresh":"off","compare":"previous_period"}}`)
	targets, _, err := systemQueryTargets(spec)
	if err != nil {
		t.Fatal(err)
	}
	body := decodeBody(t, targets[0].body)
	if !reflect.DeepEqual(body["panels"], []any{"a", "z"}) || !reflect.DeepEqual(body["widths"], map[string]any{"a": float64(100), "z": float64(1600)}) || body["compare"] != true {
		t.Fatalf("batch=%v", body)
	}
	q := newQueryRun()
	if _, ok := q.panelResponse([]byte(`{"results":[{"id":"a","status":"empty"}]}`), targets[0]); !ok {
		t.Fatal("text panel incorrectly required a result")
	}
}

func TestOperationEvidencePreservesSchemaAndWorkloadIdentity(t *testing.T) {
	report := fixtureReport()
	before := workloadHash(report.Settings, nil)
	q := newQueryRun()
	q.kinds["drill-traces"] = &operationStats{dispatched: 1, hist: newHistogram()}
	q.kinds["drill-traces"].hist.record(time.Millisecond)
	report.Settings.Methodology += q.operationSummary("mixed")
	if workloadHash(report.Settings, nil) != before {
		t.Fatal("read outcomes changed workload identity")
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var restored Report
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&restored); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(Markdown(restored), "| drill-traces | 1 | 1 |") {
		t.Fatal("operation evidence lost during report roundtrip")
	}
}

func TestDiscoveryFallbackKeepsOneUserOperationPerSlot(t *testing.T) {
	targets, _, _ := systemQueryTargets(systemSpec(t))
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/api/panels/query":
			w.Write(panelBatch(t, targets[0], 1000, 2000))
		case "/api/annotations":
			fmt.Fprint(w, `{}`)
		case "/api/panels/exemplars":
			fmt.Fprint(w, `{"traces":[{"trace_id":"trace","namespace":"ns"}]}`)
		case "/api/traces/trace":
			fmt.Fprint(w, `{"data":{"spans":[{"span_id":"span"}]}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	q := newQueryRun()
	q.targets = targets
	for i, want := range [][]string{{"/api/panels/query", "/api/annotations"}, {"/api/panels/exemplars"}, {"/api/traces/trace"}} {
		paths = nil
		q.request(context.Background(), server.Client(), server.URL, "fanout_session=test", 2)
		if !reflect.DeepEqual(paths, want) {
			t.Fatalf("slot %d: paths=%v want %v", i, paths, want)
		}
	}
	if q.failures != 0 || q.hist.snapshot().Count != 3 {
		t.Fatalf("failures=%d latency=%+v", q.failures, q.hist.snapshot())
	}
}
