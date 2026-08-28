package bench

import (
	"strings"
	"testing"
)

func TestParsePrometheusAndCountAcceptedSignals(t *testing.T) {
	input := `# HELP fanout_ingest_rows_total rows
fanout_ingest_rows_total{signal="spans"} 123
fanout_ingest_rows_total{signal="logs"} 45
fanout_ingest_rows_total{signal="metrics"} 67
fanout_other{operation="publish,\"atomic\"",result="ok"} 2
`
	snap, err := parsePrometheus(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	got := acceptedCounts(snap)
	if got.Spans != 123 || got.LogRecords != 45 || got.MetricPoints != 67 {
		t.Fatalf("counts=%+v", got)
	}
	if value := snap.total("fanout_other", map[string]string{"operation": "publish,\"atomic\""}); value != 2 {
		t.Fatalf("escaped label total=%v", value)
	}
}

func TestParsePrometheusRejectsMalformedEvidence(t *testing.T) {
	if _, err := parsePrometheus(strings.NewReader("metric{bad=oops} 1\n")); err == nil {
		t.Fatal("expected malformed label error")
	}
}

// Prometheus exposition permits a trailing comma in a label set. Rejecting it
// would abort a phase at its very first scrape.
func TestParsePrometheusAcceptsTrailingLabelComma(t *testing.T) {
	snap, err := parsePrometheus(strings.NewReader("fanout_ingest_rows_total{signal=\"spans\",} 5\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := snap.total("fanout_ingest_rows_total", map[string]string{"signal": "spans"}); got != 5 {
		t.Fatalf("total=%v; want 5", got)
	}
}
