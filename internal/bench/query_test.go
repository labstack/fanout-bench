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
	queries.finish()

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

// The phase ends while responses are still being read. Those reads must not be
// reported as server failures, because a single failure fails the query gate.
func TestQueryRunFinishDrainsInFlightRequestsWithoutFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"rows":[]}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queries := newQueryRun()
	queries.start(ctx, &http.Client{Timeout: 5 * time.Second}, server.URL, "session=test", 50)
	time.Sleep(400 * time.Millisecond)
	queries.finish()

	if queries.failures != 0 || queries.canceled != 0 {
		t.Fatalf("failures=%d canceled=%d; end-of-phase requests were aborted instead of drained", queries.failures, queries.canceled)
	}
	completed := queries.hist.snapshot().Count
	if completed != queries.attempts-queries.shed {
		t.Fatalf("completed=%d attempts=%d shed=%d; dispatched requests went unaccounted", completed, queries.attempts, queries.shed)
	}
}

// A tick is shed only under real backpressure. Sheds recorded because a worker
// goroutine had not been scheduled yet would fail the query gate on a healthy
// server.
func TestQueryRunDoesNotShedWhileWorkersAreAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	queries := newQueryRun()
	queries.start(ctx, &http.Client{Timeout: time.Second}, server.URL, "session=test", 20)
	time.Sleep(300 * time.Millisecond)
	queries.finish()

	if queries.shed != 0 {
		t.Fatalf("shed=%d on an idle server", queries.shed)
	}
	if queries.attempts == 0 {
		t.Fatal("scheduler never ticked")
	}
}

// A response larger than the read cap is a success: the server answered, only
// the tail was left unread.
func TestQueryRunTreatsOversizeResponseAsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"rows":"`))
		block := make([]byte, 1<<20)
		for i := range block {
			block[i] = 'a'
		}
		for written := 0; written <= maxQueryBody; written += len(block) {
			if _, err := w.Write(block); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	queries := newQueryRun()
	queries.request(context.Background(), &http.Client{Timeout: 30 * time.Second}, server.URL, "session=test", 0)

	if queries.failures != 0 {
		t.Fatalf("failures=%d; a truncated read was counted as a server error", queries.failures)
	}
	if got := queries.hist.snapshot().Count; got != 1 {
		t.Fatalf("completed=%d; want 1", got)
	}
}
