package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
)

// safeRecorder is a thread-safe http.ResponseWriter so a test goroutine can
// poll the buffered body while the handler (running in another goroutine)
// writes SSE events. httptest.ResponseRecorder's *bytes.Buffer is NOT safe
// for concurrent read/write, so we use this for incremental-streaming tests.
type safeRecorder struct {
	mu     sync.Mutex
	body   bytes.Buffer
	header http.Header
	status int
}

func newSafeRecorder() *safeRecorder {
	return &safeRecorder{header: make(http.Header)}
}

func (r *safeRecorder) Header() http.Header { return r.header }

func (r *safeRecorder) WriteHeader(code int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status = code
}

func (r *safeRecorder) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.Write(b)
}

// Flush is a no-op: writes are already in the buffer. Implementing http.Flusher
// lets the handler obtain a flusher via type assertion.
func (r *safeRecorder) Flush() {}

func (r *safeRecorder) BodyString() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body.String()
}

func (r *safeRecorder) StatusCode() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

// adminTimedProbe is a scheduler.ChannelHealthProbe that sleeps a per-channel
// duration before returning. Used to prove SSE events are written
// incrementally (the fast channel's event must appear before the slow one).
type adminTimedProbe struct {
	mu     sync.Mutex
	calls  []int64
	delays map[int64]time.Duration
}

func (p *adminTimedProbe) ProbeChannel(_ context.Context, target scheduler.ProbeTarget) (scheduler.ProbeOutcome, error) {
	p.mu.Lock()
	p.calls = append(p.calls, target.ChannelID)
	delay := time.Duration(0)
	if d, ok := p.delays[target.ChannelID]; ok {
		delay = d
	}
	p.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return scheduler.ProbeOutcome{Status: "success", LatencyMs: float64(delay.Milliseconds())}, nil
}

func (p *adminTimedProbe) startedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// adminReleaseProbe blocks every ProbeChannel call on a shared release
// channel so the test controls exactly when in-flight probes finish.
type adminReleaseProbe struct {
	mu      sync.Mutex
	calls   []int64
	release chan struct{}
}

func (p *adminReleaseProbe) ProbeChannel(_ context.Context, target scheduler.ProbeTarget) (scheduler.ProbeOutcome, error) {
	p.mu.Lock()
	p.calls = append(p.calls, target.ChannelID)
	p.mu.Unlock()
	<-p.release
	return scheduler.ProbeOutcome{Status: "success"}, nil
}

func (p *adminReleaseProbe) startedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

// noopRecorder is a scheduler.ChannelHealthRecorder that records nothing.
// probeOne requires a non-nil recorder; this satisfies it without mutating
// routing health (the admin probe surface is a read-only-ish diagnostic).
type noopRecorder struct{}

func (noopRecorder) RecordProbeSuccess(context.Context, int64, float64, *string, *int64) error {
	return nil
}
func (noopRecorder) RecordProbeFailure(context.Context, int64, *int, *string, *string, *int64) error {
	return nil
}

// seedProbeTargetsForSite inserts a site + account + route + N channels with
// the given source models and returns the channel IDs (in insert order).
func seedProbeTargetsForSite(t *testing.T, db *store.DB, models []string) (siteID int64, chanIDs []int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"ProbeStreamSite", "https://probe.example.com", "openai", "active", now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, _ = res.LastInsertId()
	res, err = db.Exec(`INSERT INTO accounts (site_id, access_token, status, created_at, updated_at) VALUES (?, ?, 'active', ?, ?)`,
		siteID, "tok", now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, _ := res.LastInsertId()
	res, err = db.Exec(`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at) VALUES (?, 'pattern', 'weighted', true, ?, ?)`,
		"m-*", now, now)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	routeID, _ := res.LastInsertId()
	for _, m := range models {
		res, err = db.Exec(`INSERT INTO route_channels (route_id, account_id, source_model, enabled) VALUES (?, ?, ?, true)`,
			routeID, accountID, m)
		if err != nil {
			t.Fatalf("insert channel %s: %v", m, err)
		}
		cid, _ := res.LastInsertId()
		chanIDs = append(chanIDs, cid)
	}
	return siteID, chanIDs
}

// setupAdminGlobalScheduler points store.GetDB() at the in-memory test DB and
// installs a global model-probe scheduler with the given probe executor, so
// the admin probe-now / probe-stream handlers use the fake instead of a real
// upstream. Returns a cleanup func that restores the global state.
func setupAdminGlobalScheduler(t *testing.T, db *store.DB, probe scheduler.ChannelHealthProbe) func() {
	t.Helper()
	store.OverrideDB(db)
	sched := scheduler.NewModelProbeScheduler(&config.Config{
		ModelAvailabilityProbeTimeoutMs: 5000,
	})
	sched.SetProbeExecutor(probe)
	sched.SetHealthRecorder(noopRecorder{})
	scheduler.SetGlobalModelProbeScheduler(sched)
	return func() {
		scheduler.SetGlobalModelProbeScheduler(nil)
		store.OverrideDB(nil)
	}
}

// TestSites_ProbeStream_IncrementalSSE verifies the probe-stream handler
// writes each probe-result SSE event as the probe completes, not buffered and
// replayed after all probes finish. With a fast channel (20ms) and a slow
// channel (200ms), the fast channel's event must appear in the response
// body well before the slow channel could have finished — proving real
// incremental streaming through the SSE handler.
func TestSites_ProbeStream_IncrementalSSE(t *testing.T) {
	db, r := setupSitesTest(t)
	cleanup := setupAdminGlobalScheduler(t, db, &adminTimedProbe{
		delays: map[int64]time.Duration{}, // filled after we know channel IDs
	})
	defer cleanup()

	// Seed two channels; the probe executor needs per-channel delays so we
	// re-inject it once IDs are known.
	siteID, chanIDs := seedProbeTargetsForSite(t, db, []string{"fast-model", "slow-model"})
	if len(chanIDs) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(chanIDs))
	}
	// Replace the global scheduler's probe with one that knows the real IDs.
	probe := &adminTimedProbe{delays: map[int64]time.Duration{
		chanIDs[0]: 20 * time.Millisecond,  // fast-model
		chanIDs[1]: 200 * time.Millisecond, // slow-model
	}}
	scheduler.GetGlobalModelProbeScheduler().SetProbeExecutor(probe)

	rec := newSafeRecorder()
	req := httptest.NewRequest("GET", "/api/sites/"+itoa(siteID)+"/probe-stream", nil)

	start := time.Now()
	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait for the fast channel's result event. Under the old batched-replay
	// behavior, no probe-result would appear until ~200ms (all probes done).
	deadline := start.Add(150 * time.Millisecond)
	var fastAt time.Duration
	for time.Now().Before(deadline) {
		if strings.Contains(rec.BodyString(), "fast-model") {
			fastAt = time.Since(start)
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if fastAt == 0 {
		t.Fatalf("fast-model result never appeared before 150ms (body=%q)", rec.BodyString())
	}
	if fastAt >= 150*time.Millisecond {
		t.Fatalf("fast-model event arrived at %v — looks buffered until all probes finished (want < 150ms)", fastAt)
	}
	// The slow channel must NOT have appeared yet (it finishes at 200ms).
	if strings.Contains(rec.BodyString(), "slow-model") {
		t.Fatalf("slow-model appeared at %v before its 200ms delay — incremental contract broken", fastAt)
	}

	// Now wait for the full stream to complete.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe-stream handler did not return within 2s")
	}

	body := rec.BodyString()
	if !strings.Contains(body, "probe-start") {
		t.Error("expected probe-start event")
	}
	if !strings.Contains(body, "complete") {
		t.Error("expected complete event")
	}
	if !strings.Contains(body, "[DONE]") {
		t.Error("expected [DONE] sentinel")
	}
	if got := strings.Count(body, "event: probe-result"); got != 2 {
		t.Fatalf("expected 2 probe-result events, got %d (body=%q)", got, body)
	}
	if rec.StatusCode() != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.StatusCode())
	}
}

// TestSites_ProbeStream_CancelStopsProbing verifies that cancelling the
// request context (client disconnect) stops launching new probes: with 12
// channels and 8-way concurrency, the first 8 start, then a cancel must
// prevent channels 9-12 from ever starting. The handler returns without
// writing the complete / [DONE] closing events because the client is gone.
func TestSites_ProbeStream_CancelStopsProbing(t *testing.T) {
	db, r := setupSitesTest(t)
	releaseProbe := &adminReleaseProbe{release: make(chan struct{})}
	cleanup := setupAdminGlobalScheduler(t, db, releaseProbe)
	defer cleanup()

	models := make([]string, 12)
	for i := range models {
		models[i] = "m" + itoa(int64(i))
	}
	siteID, _ := seedProbeTargetsForSite(t, db, models)
	scheduler.GetGlobalModelProbeScheduler().SetProbeExecutor(releaseProbe)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/api/sites/"+itoa(siteID)+"/probe-stream", nil).WithContext(ctx)
	rec := newSafeRecorder()

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(rec, req)
		close(done)
	}()

	// Wait until the 8-slot semaphore is saturated (8 probes started).
	saturateDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(saturateDeadline) {
		if releaseProbe.startedCount() == 8 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got := releaseProbe.startedCount(); got != 8 {
		t.Fatalf("expected 8 probes started before cancel, got %d", got)
	}

	// Cancel the request (client disconnect). No 9th-12th probe may start.
	cancel()
	time.Sleep(40 * time.Millisecond)
	if got := releaseProbe.startedCount(); got != 8 {
		t.Fatalf("after cancel, %d probes started (want 8 — no new probes should start)", got)
	}

	// Release the 8 in-flight probes so the handler can return.
	close(releaseProbe.release)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("probe-stream handler did not return within 2s after in-flight probes were released")
	}

	body := rec.BodyString()
	if !strings.Contains(body, "probe-start") {
		t.Error("expected probe-start event to have been written before cancel")
	}
	// The client is gone: no probe-result, no complete, no [DONE].
	if strings.Contains(body, "event: probe-result") {
		t.Errorf("expected no probe-result events after cancel, got body=%q", body)
	}
	if strings.Contains(body, "complete") {
		t.Errorf("expected no complete event after client disconnect, got body=%q", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Errorf("expected no [DONE] sentinel after client disconnect, got body=%q", body)
	}
	if got := releaseProbe.startedCount(); got != 8 {
		t.Fatalf("final started count = %d, want 8 (cancel must not launch the remaining targets)", got)
	}
}
