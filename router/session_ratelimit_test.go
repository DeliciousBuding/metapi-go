package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
)

// TestFailedAuthIsRateLimited proves the #1034 middleware order: the per-IP
// limiter runs BEFORE AdminAuth, so credential brute force (401/403) consumes
// the bucket instead of bypassing it. Uses a tiny bucket (1 rps / burst 1)
// so the test is deterministic without sleeping.
func TestFailedAuthIsRateLimited(t *testing.T) {
	cfg := &config.Config{
		AuthToken:           "admin-token",
		ProxyToken:          "proxy-token",
		RequestBodyLimit:    config.DefaultRequestBodyLimit,
		AdminRateLimitRPS:   1,
		AdminRateLimitBurst: 1,
	}
	r := New(cfg, web.Dist)

	sawTooMany := false
	sawAuthFailure := false
	for i := 0; i < 6; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/debug/vars", nil)
		req.Header.Set("Authorization", "Bearer wrong-token")
		r.ServeHTTP(rec, req)
		switch rec.Code {
		case http.StatusForbidden, http.StatusUnauthorized:
			sawAuthFailure = true
		case http.StatusTooManyRequests:
			sawTooMany = true
		default:
			t.Fatalf("attempt %d unexpected status %d", i, rec.Code)
		}
		if sawTooMany {
			break
		}
	}
	if !sawAuthFailure {
		t.Fatal("no auth failure observed before the bucket drained")
	}
	if !sawTooMany {
		t.Fatal("failed-auth flood never hit 429 — rate limiter is not in front of AdminAuth")
	}
}

// TestLoginEndpointIsRateLimited harder: POST /api/auth/login is public
// (bypasses AdminAuth) but must still sit behind both the general admin
// bucket and the strict /api/auth/* bucket.
func TestLoginEndpointIsRateLimited(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		AuthToken:           "admin-token",
		ProxyToken:          "proxy-token",
		RequestBodyLimit:    config.DefaultRequestBodyLimit,
		AuthRateLimitRPS:    1,
		AuthRateLimitBurst:  1,
		AdminRateLimitRPS:   100,
		AdminRateLimitBurst: 100,
		DbType:              store.DialectSQLite,
		DbUrl:               filepath.Join(dataDir, "login-ratelimit.db"),
		DataDir:             dataDir,
	}
	// Login needs the session store (503 without one, fail-closed).
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	t.Cleanup(func() { _ = store.CloseDatabase() })
	r := New(cfg, web.Dist)

	sawTooMany := false
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			sawTooMany = true
			break
		}
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d unexpected status %d", i, rec.Code)
		}
	}
	if !sawTooMany {
		t.Fatal("login flood never hit 429")
	}
}
