package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The gemini-cli and antigravity onboarding flows poll an upstream until it
// reports done, sleeping between attempts (2s / 5s / 2s). Those waits and the
// requests around them used to be ctx-free, so a cancelled exchange or refresh
// still ran the whole poll budget — up to maxAttempts x interval — and the
// upstream calls on that chain could not be cancelled either.
//
// These tests drive the real functions against a local upstream and pin, per
// poll loop: the wait is cut short, ctx.Err() surfaces, and no further poll
// reaches the upstream after the cancellation.

// withGeminiCliUpstreamSwap points the cloudcode internal API at a local server
// for the duration of the test, mirroring withAntigravityEndpointSwap.
func withGeminiCliUpstreamSwap(t *testing.T, upstreamURL string) {
	t.Helper()
	original := geminiCliUpstreamBaseURL
	geminiCliUpstreamBaseURL = upstreamURL
	t.Cleanup(func() { geminiCliUpstreamBaseURL = original })
}

// newOnboardPollServer answers loadCodeAssist with a tier list and no project
// (which is what makes both flows enter their onboarding poll loop) and answers
// onboardUser with the given payload, counting the polls.
func newOnboardPollServer(t *testing.T, onboardResponse string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var onboardCalls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "loadCodeAssist"):
			_, _ = w.Write([]byte(`{"allowedTiers":[{"id":"free-tier","isDefault":true}]}`))
		case strings.Contains(r.URL.Path, "onboardUser"):
			onboardCalls.Add(1)
			_, _ = w.Write([]byte(onboardResponse))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &onboardCalls
}

func cancelAfter(cancel context.CancelFunc, d time.Duration) {
	go func() {
		time.Sleep(d)
		cancel()
	}()
}

// TestPerformGeminiCliSetup_AutoOnboardPollHonoursContext covers the project
// discovery loop (geminiCliAutoOnboardMaxAttempts x 2s): onboarding never
// reports done, so an uncancelled run would poll for up to 30s.
func TestPerformGeminiCliSetup_AutoOnboardPollHonoursContext(t *testing.T) {
	server, onboardCalls := newOnboardPollServer(t, `{"done":false}`)
	withGeminiCliUpstreamSwap(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfter(cancel, 150*time.Millisecond)

	start := time.Now()
	projectID, err := performGeminiCliSetup(ctx, "gem-access", "", nil)
	elapsed := time.Since(start)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 1.5s (the auto-onboard poll slept its 2s interval despite the cancelled ctx)", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if projectID != "" {
		t.Fatalf("projectID = %q, want empty on a cancelled onboard", projectID)
	}
	if got := onboardCalls.Load(); got != 1 {
		t.Fatalf("onboardUser polls = %d, want 1 (the loop kept polling after cancellation)", got)
	}
}

// TestPerformGeminiCliSetup_OnboardPollHonoursContext covers the second loop —
// the one that polls with an explicit project (geminiCliOnboardMaxAttempts x 5s,
// i.e. up to 30s of uninterruptible waiting before the fix).
func TestPerformGeminiCliSetup_OnboardPollHonoursContext(t *testing.T) {
	server, onboardCalls := newOnboardPollServer(t, `{"done":false}`)
	withGeminiCliUpstreamSwap(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfter(cancel, 150*time.Millisecond)

	start := time.Now()
	projectID, err := performGeminiCliSetup(ctx, "gem-access", "explicit-project", nil)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want < 2s (the onboard poll slept its 5s interval despite the cancelled ctx)", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if projectID != "" {
		t.Fatalf("projectID = %q, want empty on a cancelled onboard", projectID)
	}
	if got := onboardCalls.Load(); got != 1 {
		t.Fatalf("onboardUser polls = %d, want 1 (the loop kept polling after cancellation)", got)
	}
}

// TestPerformGeminiCliSetup_OnboardCompletesWithoutCancellation is the
// counterpart: a live ctx must not change the outcome — the loops still poll and
// still resolve the project the upstream reports.
func TestPerformGeminiCliSetup_OnboardCompletesWithoutCancellation(t *testing.T) {
	server, _ := newOnboardPollServer(t, `{"done":true,"response":{"cloudaicompanionProject":"onboarded-project"}}`)
	withGeminiCliUpstreamSwap(t, server.URL)

	start := time.Now()
	projectID, err := performGeminiCliSetup(context.Background(), "gem-access", "", nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("performGeminiCliSetup: %v", err)
	}
	if projectID != "onboarded-project" {
		t.Fatalf("projectID = %q, want onboarded-project", projectID)
	}
	if elapsed > time.Second {
		t.Fatalf("elapsed = %v, want no polling wait when the first attempt is done", elapsed)
	}
}

// TestFetchAntigravityProjectID_OnboardPollHonoursContext is the same property
// for the antigravity flow (antigravityOnboardMaxAttempts x 2s). The function
// swallows upstream failures by design, but a finished ctx is not an upstream
// condition: it must surface so the caller can tell an abort from "no project".
func TestFetchAntigravityProjectID_OnboardPollHonoursContext(t *testing.T) {
	server, onboardCalls := newOnboardPollServer(t, `{"done":false}`)
	withAntigravityEndpointSwap(t, "", "", server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelAfter(cancel, 150*time.Millisecond)

	start := time.Now()
	projectID, err := fetchAntigravityProjectID(ctx, "ag-access", nil)
	elapsed := time.Since(start)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 1.5s (the onboard poll slept its 2s interval despite the cancelled ctx)", elapsed)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if projectID != "" {
		t.Fatalf("projectID = %q, want empty on a cancelled onboard", projectID)
	}
	if got := onboardCalls.Load(); got != 1 {
		t.Fatalf("onboardUser polls = %d, want 1 (the loop kept polling after cancellation)", got)
	}
}

// TestCallGeminiCliInternalAPI_HonoursContext pins the other half of the gap: an
// already-finished ctx must not produce an upstream round trip at all.
func TestCallGeminiCliInternalAPI_HonoursContext(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	withGeminiCliUpstreamSwap(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := callGeminiCliInternalAPI(ctx, "gem-access", "loadCodeAssist", map[string]interface{}{}, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0 (the request ignored the cancelled ctx)", got)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %v, want an immediate failure", elapsed)
	}
}

// TestCallAntigravityInternalAPI_HonoursContext is the antigravity counterpart.
func TestCallAntigravityInternalAPI_HonoursContext(t *testing.T) {
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	withAntigravityEndpointSwap(t, "", "", server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := callAntigravityInternalAPI(ctx, "ag-access", "loadCodeAssist", map[string]interface{}{}, nil)
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want %v", err, context.Canceled)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("upstream hits = %d, want 0 (the request ignored the cancelled ctx)", got)
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("elapsed = %v, want an immediate failure", elapsed)
	}
}
