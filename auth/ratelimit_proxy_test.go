package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
	var body map[string]any
	if err := json.Unmarshal(rec2.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 429 body: %v (body=%q)", err, rec2.Body.String())
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok || errObj["message"] == nil || errObj["type"] != "rate_limit_error" {
		t.Fatalf("429 body is not the OpenAI error envelope: %s", rec2.Body.String())
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
