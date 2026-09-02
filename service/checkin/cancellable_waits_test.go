package checkin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// Every wait that sits between two network calls has to be cancellable,
// otherwise a shutdown or an exhausted budget still pays the full sleep and the
// upstream call that follows it. This package has two such waits: the
// transient-retry backoff inside CheckinAccount (2-3s) and the same-site pacing
// inside a CheckinAll round (~1s per account). Both used to be a bare
// time.Sleep.
//
// Each test below drives the real entry point against an httptest upstream and
// pins three properties: the wait is cut short, the outcome carries ctx.Err()
// semantics, and no further upstream call is made after the cancellation.

// upstreamHit records one request the test upstream served.
type upstreamHit struct {
	path string
	at   time.Time
}

// hitLog collects upstream hits so a test can assert both how many calls
// happened and whether any of them landed after the ctx ended.
type hitLog struct {
	mu   sync.Mutex
	hits []upstreamHit
}

func (l *hitLog) record(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.hits = append(l.hits, upstreamHit{path: path, at: time.Now()})
}

func (l *hitLog) count(path string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, h := range l.hits {
		if h.path == path {
			n++
		}
	}
	return n
}

// after reports how many hits landed at or after the given instant.
func (l *hitLog) after(cutoff time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, h := range l.hits {
		if !h.at.Before(cutoff) {
			n++
		}
	}
	return n
}

func (l *hitLog) snapshot() []upstreamHit {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]upstreamHit(nil), l.hits...)
}

// newTransientCheckinServer answers /api/user/checkin with a 429, which
// classifies as a transient failure and therefore makes CheckinAccount enter its
// retry backoff. Every request is recorded with its arrival time.
func newTransientCheckinServer(t *testing.T) (*httptest.Server, *hitLog) {
	t.Helper()
	log := &hitLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r.URL.Path)
		if r.URL.Path != "/api/user/checkin" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"success":false,"message":"rate limited"}`))
	}))
	t.Cleanup(server.Close)
	return server, log
}

// TestCheckinAccount_TransientRetryBackoffHonoursContext pins the backoff before
// the single transient retry. checkinTimeout is the budget of the ctx that
// CheckinAccount derives for the account, so shrinking it below the 2s floor of
// transientRetryBackoff makes the wait expire mid-backoff: a cancellable wait
// ends the checkin at ~400ms with the ctx error and no retry, a bare sleep runs
// the full 2-3s and then issues the retried upstream call anyway.
func TestCheckinAccount_TransientRetryBackoffHonoursContext(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}

	original := checkinTimeout
	checkinTimeout = 400 * time.Millisecond
	t.Cleanup(func() { checkinTimeout = original })
	budget := checkinTimeout

	upstream, hits := newTransientCheckinServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	accountID := insertEdgeAccount(t, db, siteID, "some-token", buildEdgeExtraConfig(t, 1, "", ""))

	start := time.Now()
	result := CheckinAccount(cfg, db.DB, accountID, &CheckinOptions{
		SkipEvent: true, SkipNotification: true, ScheduleMode: "manual",
	})
	elapsed := time.Since(start)

	// 1. The wait was interrupted instead of running its 2-3s course.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 1.5s (the transient-retry backoff ignored the account ctx and slept it out)", elapsed)
	}
	// 2. The outcome carries the ctx failure, and keeps the upstream message the
	//    failure classifier reads.
	if result.Success {
		t.Fatalf("result = %+v, want a failure", result)
	}
	if result.Status != CheckinFailed {
		t.Fatalf("status = %v, want %v", result.Status, CheckinFailed)
	}
	if !strings.Contains(result.Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("result.Message = %q, want it to carry %q", result.Message, context.DeadlineExceeded.Error())
	}
	if !strings.Contains(result.Message, "rate limited") {
		t.Fatalf("result.Message = %q, want it to keep the original upstream failure", result.Message)
	}
	// 3. No upstream call after the budget expired — the retry must not run.
	if late := hits.after(start.Add(budget + 250*time.Millisecond)); late != 0 {
		t.Fatalf("%d upstream call(s) landed after the ctx expired; a cancelled backoff must not retry. hits=%v", late, hits.snapshot())
	}
	if got := hits.count("/api/user/checkin"); got != 1 {
		t.Fatalf("checkin calls = %d, want 1 (the retry ran despite the cancelled ctx). hits=%v", got, hits.snapshot())
	}
}

// newCountingCheckinServer answers /api/user/checkin with a success and counts
// the calls, so a round test can tell how many accounts actually reached the
// upstream.
func newCountingCheckinServer(t *testing.T) (*httptest.Server, *hitLog) {
	t.Helper()
	log := &hitLog{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.record(r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/user/checkin":
			_, _ = w.Write([]byte(`{"success":true,"message":"checkin success","data":{"reward":"5"}}`))
		case "/api/user/self":
			_, _ = w.Write([]byte(`{"success":true,"data":{"username":"edge-user","id":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, log
}

// TestCheckinAllContext_SameSitePacingHonoursContext pins the pacing wait inside
// a round. Two accounts on one site cost one pacing delay (~1s) between them;
// cancelling the round ctx during that delay must stop the round there, so the
// second account never reaches the upstream and the round returns in a fraction
// of the delay.
func TestCheckinAllContext_SameSitePacingHonoursContext(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}
	swapNotifySend(t)

	upstream, hits := newCountingCheckinServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	insertEdgeAccount(t, db, siteID, "token-1", buildEdgeExtraConfig(t, 1, "", ""))
	insertEdgeAccount(t, db, siteID, "token-2", buildEdgeExtraConfig(t, 2, "", ""))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// The first account is checked in within milliseconds, so this lands in
		// the middle of the pacing wait that precedes the second one.
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	results := CheckinAllContext(ctx, cfg, db.DB, nil, "manual")
	elapsed := time.Since(start)

	// 1. The pacing wait was cut short instead of running its ~1s course.
	if elapsed > 900*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 900ms (the same-site pacing wait ignored the round ctx)", elapsed)
	}
	// 2. The round reports the ctx outcome rather than pretending every account
	//    was processed.
	if len(results) != 1 {
		t.Fatalf("results = %+v, want exactly the one account processed before the cancellation", results)
	}
	// 3. The second account never reached the upstream.
	if got := hits.count("/api/user/checkin"); got != 1 {
		t.Fatalf("upstream checkin calls = %d, want 1 (the round kept going after cancellation). hits=%v", got, hits.snapshot())
	}
}

// TestCheckinAllContext_KeepsPacingWhenNotCancelled is the counterpart: making
// the wait cancellable must not shorten it. An uncancelled round still spaces
// the two same-site accounts by the pacing delay and processes both.
func TestCheckinAllContext_KeepsPacingWhenNotCancelled(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}
	swapNotifySend(t)

	upstream, hits := newCountingCheckinServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	insertEdgeAccount(t, db, siteID, "token-1", buildEdgeExtraConfig(t, 1, "", ""))
	insertEdgeAccount(t, db, siteID, "token-2", buildEdgeExtraConfig(t, 2, "", ""))

	start := time.Now()
	results := CheckinAllContext(context.Background(), cfg, db.DB, nil, "manual")
	elapsed := time.Since(start)

	// sameSitePacingDelay() floors at 1s; allow a little slack for timer
	// granularity but never accept a round that skipped the pacing.
	if elapsed < 950*time.Millisecond {
		t.Fatalf("elapsed = %v, want >= ~1s (the pacing delay must stay as long as before)", elapsed)
	}
	if len(results) != 2 {
		t.Fatalf("results = %+v, want both accounts", results)
	}
	if got := hits.count("/api/user/checkin"); got != 2 {
		t.Fatalf("upstream checkin calls = %d, want 2", got)
	}
}
