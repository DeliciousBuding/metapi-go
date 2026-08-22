package proxyhandler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// allFailedAlertRouter is a TokenRouterInterface stub for the terminal
// all-failed paths (selection error / dead upstream / exhausted retries).
type allFailedAlertRouter struct {
	selected *routing.SelectedChannel
	next     *routing.SelectedChannel
	selErr   error
	failures int
}

func (r *allFailedAlertRouter) SelectChannel(_ context.Context, _ string, _ routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if r.selErr != nil {
		return nil, r.selErr
	}
	return r.selected, nil
}

func (r *allFailedAlertRouter) SelectNextChannel(_ context.Context, _ string, _ []int64, _ routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if r.selErr != nil {
		return nil, r.selErr
	}
	return r.next, nil
}

func (r *allFailedAlertRouter) SelectPreferredChannel(_ context.Context, _ string, _ int64, _ routing.DownstreamRoutingPolicy, _ []int64) (*routing.SelectedChannel, error) {
	return nil, nil
}

func (r *allFailedAlertRouter) RecordSuccess(_ context.Context, _ int64, _ float64, _ float64, _ *string, _ *int64) error {
	return nil
}

func (r *allFailedAlertRouter) RecordFailure(_ context.Context, _ int64, _ routing.SiteRuntimeFailureContext, _ *int64) error {
	r.failures++
	return nil
}

// setupAllFailedAlertDB wires an in-memory DB as the process store so the
// terminal all-failed exits can write events via store.GetDB().
func setupAllFailedAlertDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	store.OverrideDB(db)
	t.Cleanup(func() {
		store.OverrideDB(nil)
		db.Close()
	})
	return db
}

func countAllFailedProxyEvents(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM events WHERE type = 'proxy'`); err != nil {
		t.Fatalf("count proxy events: %v", err)
	}
	return n
}

func lastAllFailedProxyEventMessage(t *testing.T, db *store.DB) string {
	t.Helper()
	var msg string
	if err := db.Get(&msg, `SELECT message FROM events WHERE type = 'proxy' ORDER BY id DESC LIMIT 1`); err != nil {
		t.Fatalf("load proxy event: %v", err)
	}
	return msg
}

// No channel can serve the model: first selection fails, the handler answers
// 503 "No available channels" and must write a proxy all-failed event.
func TestDispatchNoAvailableChannelsWritesProxyAllFailedEvent(t *testing.T) {
	db := setupAllFailedAlertDB(t)
	router := &allFailedAlertRouter{selErr: errors.New("no routes for model")}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	doReq := func() int {
		req := makeProxyReq("POST", "/v1/chat/completions",
			`{"model":"gpt-nochan-alert","messages":[{"role":"user","content":"hi"}]}`)
		rec := httptest.NewRecorder()
		HandleChatCompletions(rec, req)
		return rec.Code
	}

	if code := doReq(); code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("expected 1 proxy event after all-failed exit, got %d", n)
	}
	msg := lastAllFailedProxyEventMessage(t, db)
	if !strings.Contains(msg, "gpt-nochan-alert") || !strings.Contains(msg, "no available channels") {
		t.Fatalf("event message missing model/reason: %q", msg)
	}

	// Second request within the cooldown window: throttled, no new event.
	if code := doReq(); code != http.StatusServiceUnavailable {
		t.Fatalf("second status = %d, want 503", code)
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("throttle failed: expected 1 proxy event after repeat, got %d", n)
	}
}

// Dead upstream (connection refused): attempts fail, selection gives up, the
// handler surfaces the last upstream error and must write one proxy event —
// and only one across repeated requests.
func TestDispatchDeadUpstreamWritesProxyAllFailedEventOnce(t *testing.T) {
	db := setupAllFailedAlertDB(t)

	// Create a test server and close it immediately so its port refuses connections.
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	dead.Close()

	router := &allFailedAlertRouter{selected: &routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 11, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 3, URL: dead.URL, Status: "active"},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-dead-alert",
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	doReq := func() int {
		req := makeProxyReq("POST", "/v1/chat/completions",
			`{"model":"gpt-dead-alert","messages":[{"role":"user","content":"hi"}]}`)
		rec := httptest.NewRecorder()
		HandleChatCompletions(rec, req)
		return rec.Code
	}

	if code := doReq(); code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", code)
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("expected 1 proxy event after dead-upstream failure, got %d", n)
	}
	msg := lastAllFailedProxyEventMessage(t, db)
	if !strings.Contains(msg, "gpt-dead-alert") || !strings.Contains(msg, "Upstream request failed") {
		t.Fatalf("event message missing model/reason: %q", msg)
	}
	if router.failures == 0 {
		t.Fatal("expected recorded channel failures for dead upstream")
	}

	// Second request within the cooldown window: throttled, no new event.
	if code := doReq(); code != http.StatusBadGateway {
		t.Fatalf("second status = %d, want 502", code)
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("throttle failed: expected 1 proxy event after repeat, got %d", n)
	}
}

// Retry loop exhausts on a saturated site: the handler answers 503
// "All channels exhausted" and must write a proxy all-failed event.
func TestDispatchAllChannelsExhaustedWritesProxyAllFailedEvent(t *testing.T) {
	db := setupAllFailedAlertDB(t)

	channel := &routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: 21, Enabled: true},
		Account:     store.Account{ID: 7, Status: "active"},
		Site:        store.Site{ID: 5, URL: "http://127.0.0.1:9", Status: "active", MaxConcurrency: 1},
		TokenValue:  "upstream-token",
		ActualModel: "gpt-exhausted-alert",
	}
	router := &allFailedAlertRouter{selected: channel, next: channel}
	// Hold the only site slot so every attempt is skipped without dispatch.
	limiter := proxy.NewSiteConcurrencyLimiter()
	slot, acquired := limiter.TryAcquire(5, 1)
	if !acquired {
		t.Fatal("expected to pre-acquire the site slot")
	}
	t.Cleanup(slot.Release)

	SetUpstreamConfig(&UpstreamConfig{Router: router, SiteLimiter: limiter})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	req := makeProxyReq("POST", "/v1/chat/completions",
		`{"model":"gpt-exhausted-alert","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	HandleChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "All channels exhausted") {
		t.Fatalf("body = %q, want All channels exhausted", rec.Body.String())
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("expected 1 proxy event after exhausted retries, got %d", n)
	}
	msg := lastAllFailedProxyEventMessage(t, db)
	if !strings.Contains(msg, "gpt-exhausted-alert") || !strings.Contains(msg, "all channels exhausted") {
		t.Fatalf("event message missing model/reason: %q", msg)
	}
	if router.failures != 0 {
		t.Fatalf("failures = %d, want 0 (saturation must not record failure)", router.failures)
	}
}
