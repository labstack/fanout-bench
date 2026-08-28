package bench

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestQueryRunMaintainsOpenLoopScheduleThroughLatency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	queries := newQueryRun()
	queries.start(ctx, &http.Client{Timeout: time.Second}, server.URL, "session=test", 100)
	<-ctx.Done()
	queries.wait()

	if queries.attempts < 45 {
		t.Fatalf("attempts=%d; scheduler fell below its open-loop target", queries.attempts)
	}
	if got := queries.hist.snapshot().Count; got < 35 {
		t.Fatalf("completed=%d; requests were serialized by latency", got)
	}
	if queries.shed > 2 || queries.failures != 0 {
		t.Fatalf("shed=%d failures=%d", queries.shed, queries.failures)
	}
}
