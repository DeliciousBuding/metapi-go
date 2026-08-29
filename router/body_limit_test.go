package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/web"
)

// ---------------------------------------------------------------------------
// BodyLimitPathAware middleware tests
// ---------------------------------------------------------------------------

func TestBodyLimitPathAware_Returns413OnOversizedContentLength(t *testing.T) {
	mw := BodyLimitPathAware(100, 200) // 100 bytes default, 200 bytes upload
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = 101 // exceeds 100-byte general limit
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (413)", rec.Code, http.StatusRequestEntityTooLarge)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 413 body: %v (body=%q)", err, rec.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("413 body missing error field: %s", rec.Body.String())
	}
}

func TestBodyLimitPathAware_AllowsUnderLimit(t *testing.T) {
	mw := BodyLimitPathAware(100, 200)
	var called bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("hello"))
	req.ContentLength = 5
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !called {
		t.Fatal("handler should be called for under-limit body")
	}
}

func TestBodyLimitPathAware_UsesHigherLimitForFileUploadPaths(t *testing.T) {
	mw := BodyLimitPathAware(100, 200) // 100 general, 200 upload
	var called bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// /v1/files path should use the 200-byte file upload limit, not the 100-byte general.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader("x"))
	req.ContentLength = 150 // > 100 general but < 200 upload
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/files with 150 bytes: status = %d, want 200 (higher upload limit)", rec.Code)
	}
	if !called {
		t.Fatal("handler should be called for /v1/files under upload limit")
	}
}

func TestBodyLimitPathAware_UsesHigherLimitForImagesPaths(t *testing.T) {
	mw := BodyLimitPathAware(100, 200)
	var called bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	// /v1/images/edits should use the 200-byte file upload limit.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader("x"))
	req.ContentLength = 180
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("/v1/images/edits with 180 bytes: status = %d, want 200 (higher upload limit)", rec.Code)
	}
	if !called {
		t.Fatal("handler should be called for /v1/images/edits under upload limit")
	}
}

func TestBodyLimitPathAware_RejectsFileUploadExceedingHigherLimit(t *testing.T) {
	mw := BodyLimitPathAware(100, 200)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// /v1/files with 201 bytes exceeds even the 200-byte upload limit.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader("x"))
	req.ContentLength = 201
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("/v1/files with 201 bytes: status = %d, want %d (413)", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestBodyLimitPathAware_NoLimitWhenDefaultZero(t *testing.T) {
	mw := BodyLimitPathAware(0, 0)
	var called bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("x"))
	req.ContentLength = 999999
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no limit when default=0)", rec.Code)
	}
	if !called {
		t.Fatal("handler should be called when no limit is set")
	}
}

// ---------------------------------------------------------------------------
// Integration: /v1 rate limiting via the full router
// ---------------------------------------------------------------------------

func TestV1ProxyRateLimitReturns429(t *testing.T) {
	cfg := &config.Config{
		RequestBodyLimit:  config.DefaultRequestBodyLimit,
		ProxyRateLimitRPM: 2,
		// very low limit for fast testing,
	}
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:         "admin-token",
		ProxyToken:        "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := New(cfg, web.Dist)

	// Two requests from the same IP without auth — both hit the per-IP limiter.
	// Without auth they would get 401, but the rate limiter runs before auth
	// and passes (count < limit). We expect 401 for both (auth fails) but the
	// limiter still counts them.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		r.ServeHTTP(rec, req)
		// 401 because no auth token — that's fine, the limiter passed.
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d, want 401 (no auth)", i+1, rec.Code)
		}
	}

	// Third request: same IP now over the per-IP limit → 429 from the limiter.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third request: status = %d, want %d (429 rate limited)", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("X-RateLimit-Limit"); got != "2" {
		t.Fatalf("X-RateLimit-Limit = %q, want %q", got, "2")
	}
	if got := rec.Header().Get("Retry-After"); got == "" {
		t.Fatal("Retry-After header missing on 429")
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal 429 body: %v (body=%q)", err, rec.Body.String())
	}
	if body["error"] == "" {
		t.Fatalf("429 body missing error field: %s", rec.Body.String())
	}
}

func TestV1ProxyRateLimitDisabledWhenRPMZero(t *testing.T) {
	cfg := &config.Config{
		RequestBodyLimit:  config.DefaultRequestBodyLimit,
		ProxyRateLimitRPM: 0,
		// disabled,
	}
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:         "admin-token",
		ProxyToken:        "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := New(cfg, web.Dist)

	// Fire many requests; all should get 401 (auth) not 429 (rate limited).
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.RemoteAddr = "192.0.2.2:1234"
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("request %d: got 429 but rate limiter is disabled", i+1)
		}
	}
}

func TestV1ProxyRateLimitPerIPIsolation(t *testing.T) {
	cfg := &config.Config{
		RequestBodyLimit:  config.DefaultRequestBodyLimit,
		ProxyRateLimitRPM: 1,
	}
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:         "admin-token",
		ProxyToken:        "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := New(cfg, web.Dist)

	// IP A: one request, exhausts its single-request budget.
	recA := httptest.NewRecorder()
	reqA := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqA.RemoteAddr = "192.0.2.10:1234"
	r.ServeHTTP(recA, reqA)

	// IP B: different IP, should NOT be rate limited.
	recB := httptest.NewRecorder()
	reqB := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqB.RemoteAddr = "192.0.2.11:1234"
	r.ServeHTTP(recB, reqB)

	if recB.Code == http.StatusTooManyRequests {
		t.Fatalf("IP B got 429 but should have its own budget (per-IP isolation)")
	}
}

// ---------------------------------------------------------------------------
// Integration: body limit 413 via the full router
// ---------------------------------------------------------------------------

func TestV1BodyLimitReturns413OnOversizedRequest(t *testing.T) {
	cfg := &config.Config{
		RequestBodyLimit: 1 * 1024,
		// 1 KB — tiny limit for testing,
	}
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := New(cfg, web.Dist)

	// Craft a body with Content-Length exceeding the limit.
	rec := httptest.NewRecorder()
	body := strings.Repeat("x", 2048)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", "2048")
	req.Header.Set("Authorization", "Bearer proxy-token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d (413 oversized body)", rec.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestV1FileUploadRouteUsesHigherBodyLimit(t *testing.T) {
	cfg := &config.Config{
		RequestBodyLimit:    1 * 1024,
		// 1 KB general
		FileUploadLimitBytes: 100 * 1024,
		// 100 KB for uploads
		ProxyRateLimitRPM:   0,
		// disable rate limiting for this test,
	}
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:           "admin-token",
		ProxyToken:          "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := New(cfg, web.Dist)

	// A 2 KB request to /v1/files should NOT be rejected by the general 1 KB
	// limit because the file upload route uses the higher 100 KB limit.
	rec := httptest.NewRecorder()
	body := strings.Repeat("x", 2048)
	req := httptest.NewRequest(http.MethodPost, "/v1/files", strings.NewReader(body))
	req.Header.Set("Content-Length", "2048")
	req.Header.Set("Authorization", "Bearer proxy-token")
	r.ServeHTTP(rec, req)

	if rec.Code == http.StatusRequestEntityTooLarge {
		t.Fatalf("/v1/files with 2 KB got 413 but should use the higher upload limit (100 KB)")
	}
}
