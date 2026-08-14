package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// fixedWindowIPRateLimiter unit tests
// ---------------------------------------------------------------------------

func TestFixedWindowIPRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := newFixedWindowIPRateLimiter(3)
	for i := 0; i < 3; i++ {
		result := rl.allow("1.2.3.4")
		if !result.Allowed {
			t.Fatalf("request %d: expected Allowed=true, got false (remaining=%d)", i+1, result.Remaining)
		}
		if result.Limit != 3 {
			t.Fatalf("Limit = %d, want 3", result.Limit)
		}
	}
}

func TestFixedWindowIPRateLimiter_BlocksAtThreshold(t *testing.T) {
	rl := newFixedWindowIPRateLimiter(2)

	// First two requests pass.
	if r := rl.allow("10.0.0.1"); !r.Allowed {
		t.Fatal("first request should be allowed")
	}
	if r := rl.allow("10.0.0.1"); !r.Allowed {
		t.Fatal("second request should be allowed")
	}

	// Third request exceeds the limit.
	result := rl.allow("10.0.0.1")
	if result.Allowed {
		t.Fatal("third request should be blocked")
	}
	if result.Remaining != 0 {
		t.Fatalf("Remaining = %d, want 0 when blocked", result.Remaining)
	}
	if result.ResetAt.IsZero() {
		t.Fatal("ResetAt should be non-zero when blocked")
	}
}

func TestFixedWindowIPRateLimiter_PerIPIsolation(t *testing.T) {
	rl := newFixedWindowIPRateLimiter(2)
	// Exhaust IP A's budget.
	rl.allow("1.1.1.1")
	rl.allow("1.1.1.1")
	if r := rl.allow("1.1.1.1"); r.Allowed {
		t.Fatal("IP A should be blocked after 2 requests")
	}
	// IP B should still have its own budget.
	if r := rl.allow("2.2.2.2"); !r.Allowed {
		t.Fatal("IP B should be allowed (separate budget)")
	}
}

func TestFixedWindowIPRateLimiter_DisabledWhenLimitZero(t *testing.T) {
	rl := newFixedWindowIPRateLimiter(0)
	for i := 0; i < 100; i++ {
		result := rl.allow("3.3.3.3")
		if !result.Allowed {
			t.Fatalf("request %d should be allowed when limiter disabled", i)
		}
		if result.Limit != 0 {
			t.Fatalf("Limit = %d, want 0 for disabled limiter", result.Limit)
		}
	}
}

func TestFixedWindowIPRateLimiter_RemainingDecrements(t *testing.T) {
	rl := newFixedWindowIPRateLimiter(3)
	r1 := rl.allow("4.4.4.4")
	if r1.Remaining != 2 {
		t.Fatalf("after 1st request Remaining = %d, want 2", r1.Remaining)
	}
	r2 := rl.allow("4.4.4.4")
	if r2.Remaining != 1 {
		t.Fatalf("after 2nd request Remaining = %d, want 1", r2.Remaining)
	}
	r3 := rl.allow("4.4.4.4")
	if r3.Remaining != 0 {
		t.Fatalf("after 3rd request Remaining = %d, want 0", r3.Remaining)
	}
}

// ---------------------------------------------------------------------------
// ProxyRateLimit middleware tests
// ---------------------------------------------------------------------------

// rateLimitTestHandler is a no-op next handler that records whether it ran.
func rateLimitTestHandler(called *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestProxyRateLimit_BlocksAfterThreshold(t *testing.T) {
	mw := ProxyRateLimit(2)
	var called bool
	h := mw(rateLimitTestHandler(&called))

	// Two requests pass.
	for i := 0; i < 2; i++ {
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.RemoteAddr = "5.5.5.5:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
		if !called {
			t.Fatalf("request %d: next handler not called", i+1)
		}
	}

	// Third request is blocked.
	called = false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if called {
		t.Fatal("next handler should NOT be called when rate limited")
	}
}

func TestProxyRateLimit_429HasJSONErrorBody(t *testing.T) {
	mw := ProxyRateLimit(1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust the single-request budget.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req1.RemoteAddr = "6.6.6.6:1234"
	h.ServeHTTP(rec1, req1)

	// Second request → 429 with JSON body.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.RemoteAddr = "6.6.6.6:1234"
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
	ct := rec2.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 429 body: %v (body=%q)", err, rec2.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("429 body missing error field: %s", rec2.Body.String())
	}
}

func TestProxyRateLimit_SetsRateLimitHeaders(t *testing.T) {
	mw := ProxyRateLimit(5)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "7.7.7.7:1234"
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("X-RateLimit-Limit"); got != "5" {
		t.Fatalf("X-RateLimit-Limit = %q, want %q", got, "5")
	}
	if got := rec.Header().Get("X-RateLimit-Remaining"); got != "4" {
		t.Fatalf("X-RateLimit-Remaining = %q, want %q", got, "4")
	}
	if got := rec.Header().Get("X-RateLimit-Reset"); got == "" {
		t.Fatal("X-RateLimit-Reset header is missing")
	}
}

func TestProxyRateLimit_SetsRetryAfterOn429(t *testing.T) {
	mw := ProxyRateLimit(1)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request passes.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req1.RemoteAddr = "8.8.8.8:1234"
	h.ServeHTTP(rec1, req1)

	// Second is blocked and must include Retry-After.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.RemoteAddr = "8.8.8.8:1234"
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
	if ra := rec2.Header().Get("Retry-After"); ra == "" {
		t.Fatal("Retry-After header is missing on 429")
	}
}

func TestProxyRateLimit_DisabledWhenRPMZero(t *testing.T) {
	mw := ProxyRateLimit(0)
	var called bool
	h := mw(rateLimitTestHandler(&called))

	// 200 requests should all pass when limiter is disabled.
	for i := 0; i < 200; i++ {
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (limiter disabled)", i+1, rec.Code)
		}
		if !called {
			t.Fatalf("request %d: handler not called", i+1)
		}
	}
}

func TestProxyRateLimit_PerIPIsolation(t *testing.T) {
	mw := ProxyRateLimit(1)
	var called bool
	h := mw(rateLimitTestHandler(&called))

	// IP A exhausts its single-request budget.
	recA := httptest.NewRecorder()
	reqA := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqA.RemoteAddr = "10.0.0.1:1234"
	h.ServeHTTP(recA, reqA)
	if recA.Code != http.StatusOK {
		t.Fatalf("IP A first request: status = %d, want 200", recA.Code)
	}

	// IP B should still get its own request.
	called = false
	recB := httptest.NewRecorder()
	reqB := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqB.RemoteAddr = "10.0.0.2:1234"
	h.ServeHTTP(recB, reqB)
	if recB.Code != http.StatusOK {
		t.Fatalf("IP B first request: status = %d, want 200 (per-IP isolation)", recB.Code)
	}
	if !called {
		t.Fatal("IP B handler not called (per-IP isolation)")
	}
}

// ---------------------------------------------------------------------------
// ProxyGlobalTokenRateLimit middleware tests
// ---------------------------------------------------------------------------

// globalTokenAuthHandler injects a ProxyAuthContext with the given source into
// the request context before delegating to next. Used to simulate the
// post-ProxyAuth state without running the full auth chain.
func globalTokenAuthHandler(source string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &ProxyAuthContext{
			Token:  "test-global-token",
			Source: source,
		}
		r = r.WithContext(WithProxyAuth(r.Context(), pac))
		next.ServeHTTP(w, r)
	})
}

func TestProxyGlobalTokenRateLimit_BlocksGlobalTokenAfterThreshold(t *testing.T) {
	mw := ProxyGlobalTokenRateLimit(2)
	var called bool
	// Simulate auth resolving source="global" before the global cap runs.
	h := globalTokenAuthHandler("global", mw(rateLimitTestHandler(&called)))

	for i := 0; i < 2; i++ {
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200", i+1, rec.Code)
		}
	}

	// Third request exceeds the global cap.
	called = false
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if called {
		t.Fatal("handler should not be called when global cap exceeded")
	}
}

func TestProxyGlobalTokenRateLimit_PassesThroughManagedKeys(t *testing.T) {
	mw := ProxyGlobalTokenRateLimit(1)
	var called bool
	// Managed keys should bypass the global cap entirely.
	h := globalTokenAuthHandler("managed", mw(rateLimitTestHandler(&called)))

	for i := 0; i < 10; i++ {
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("managed request %d: status = %d, want 200 (managed bypass)", i+1, rec.Code)
		}
		if !called {
			t.Fatalf("managed request %d: handler not called", i+1)
		}
	}
}

func TestProxyGlobalTokenRateLimit_DisabledWhenRPMZero(t *testing.T) {
	mw := ProxyGlobalTokenRateLimit(0)
	var called bool
	h := globalTokenAuthHandler("global", mw(rateLimitTestHandler(&called)))

	for i := 0; i < 100; i++ {
		called = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (disabled)", i+1, rec.Code)
		}
	}
}

func TestProxyGlobalTokenRateLimit_429HasJSONBody(t *testing.T) {
	mw := ProxyGlobalTokenRateLimit(1)
	h := globalTokenAuthHandler("global", mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	// First passes.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(rec1, req1)

	// Second is blocked.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
	var body map[string]string
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal body: %v (body=%q)", err, rec2.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("429 body missing error field: %s", rec2.Body.String())
	}
}

func TestProxyGlobalTokenRateLimit_PassesWhenNoAuthContext(t *testing.T) {
	mw := ProxyGlobalTokenRateLimit(1)
	var called bool
	// No auth context in the request (e.g. middleware misconfigured before auth)
	// should pass through, not block.
	h := mw(rateLimitTestHandler(&called))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("no-auth-context: status = %d, want 200 (pass-through)", rec.Code)
	}
	if !called {
		t.Fatal("handler not called when auth context is nil")
	}
}

// ---------------------------------------------------------------------------
// globalTokenRateLimiter window-rollover test (injectable via nowFn pattern)
// ---------------------------------------------------------------------------

func TestGlobalTokenRateLimiter_WindowRollover(t *testing.T) {
	rl := newGlobalTokenRateLimiter(2)
	rl.window = 50 * time.Millisecond // short window for fast test

	// Exhaust the cap.
	if r := rl.allow(); !r.Allowed {
		t.Fatal("first should be allowed")
	}
	if r := rl.allow(); !r.Allowed {
		t.Fatal("second should be allowed")
	}
	if r := rl.allow(); r.Allowed {
		t.Fatal("third should be blocked")
	}

	// Wait for the window to roll over.
	time.Sleep(60 * time.Millisecond)

	// After rollover, the budget resets.
	result := rl.allow()
	if !result.Allowed {
		t.Fatal("request after window rollover should be allowed")
	}
	if result.Remaining != 1 {
		t.Fatalf("Remaining after rollover = %d, want 1", result.Remaining)
	}
}
