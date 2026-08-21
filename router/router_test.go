package router

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/web"
)

func TestHealthAndReadyBypassAuthAndIncludeSecurityHeaders(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
		DbType:           store.DialectSQLite,
		DbUrl:            filepath.Join(dataDir, "router-ready.db"),
		DataDir:          dataDir,
	}
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	t.Cleanup(func() {
		_ = store.CloseDatabase()
	})

	r := New(cfg, web.Dist)

	for _, path := range []string{"/health", "/ready"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
			}
			assertSecurityHeaders(t, rec)
			if got := rec.Header().Get("X-Request-Id"); strings.TrimSpace(got) == "" {
				t.Fatal("X-Request-Id response header is empty")
			}
		})
	}
}

func assertSecurityHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	expected := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Permissions-Policy":      "camera=(), microphone=(), geolocation=(), payment=(), usb=()",
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline' https://static.cloudflareinsights.com; style-src 'self' 'unsafe-inline'; img-src 'self' https://api.dicebear.com; connect-src 'self'; frame-src 'self' https://check.linux.do; frame-ancestors 'none'",
	}
	for header, want := range expected {
		if got := rec.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestAdminRouteStillRequiresAuth(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/debug/vars", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("admin route without auth status = %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestAboutRouteRequiresAuthAndServesBuildInfoWithoutDatabase
//
// /api/about is registered outside the db != nil block (every field comes from
// the linker or the Go runtime), so it must still answer when the database was
// never initialized — while staying behind the admin auth middleware like the
// rest of /api.
func TestAboutRouteRequiresAuthAndServesBuildInfoWithoutDatabase(t *testing.T) {
	if err := store.CloseDatabase(); err != nil {
		t.Fatalf("CloseDatabase: %v", err)
	}
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/about", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/api/about without auth status = %d, want 401", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/about", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/about status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode about: %v", err)
	}
	if body["goVersion"] == "" {
		t.Fatalf("goVersion empty in %v, want the runtime version", body)
	}
	if body["version"] == "" {
		t.Fatalf("version empty in %v, want the injected binary version", body)
	}
}

func TestAdminRoutesAreMountedWithoutDoubleAPIPrefix(t *testing.T) {
	dataDir := t.TempDir()
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
		DbType:           store.DialectSQLite,
		DbUrl:            filepath.Join(dataDir, "router-admin.db"),
		DataDir:          dataDir,
	}
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	t.Cleanup(func() {
		_ = store.CloseDatabase()
	})

	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/settings/auth/info", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("admin auth info status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode auth info: %v", err)
	}
	if got := body["masked"]; got != "admi****oken" {
		t.Fatalf("masked token = %q, want %q", got, "admi****oken")
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/api/settings/auth/info", nil)
	req.Header.Set("Authorization", "Bearer admin-token")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("double-prefixed admin route status = %d, want 404", rec.Code)
	}
}

func TestAdminCORSDefaultDoesNotAllowCrossOrigin(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/desktop/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("admin CORS allow origin = %q, want empty by default", got)
	}
}

func TestAdminCORSAllowsConfiguredOriginsOnly(t *testing.T) {
	cfg := &config.Config{
		AuthToken:               "admin-token",
		ProxyToken:              "proxy-token",
		RequestBodyLimit:        config.DefaultRequestBodyLimit,
		AdminCorsAllowedOrigins: []string{"https://admin.example.com"},
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/desktop/health", nil)
	req.Header.Set("Origin", "https://admin.example.com")
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example.com" {
		t.Fatalf("configured admin origin header = %q, want https://admin.example.com", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/desktop/health", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured admin origin header = %q, want empty", got)
	}
}

func TestProxyCORSRemainsWildcard(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Origin", "https://client.example.com")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("proxy CORS allow origin = %q, want *", got)
	}
}

func TestSPAFallbackRootBypassesProxyAuth(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("SPA root status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("SPA root content-type = %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<html") {
		t.Fatalf("SPA root body does not look like HTML: %q", body[:min(len(body), 120)])
	}
}

// TestRootPublicFilesServedBeforeSPAFallback
//
// Regression for the 2026-08-02 v0.8.50 login observation: /logo.png answered
// 200 text/html (SPA fallback) because only /assets/* was served statically, so
// the login <img> rendered blank. Root public files copied by Vite into dist
// root (logo, favicons) must be served as their real content type.
func TestRootPublicFilesServedBeforeSPAFallback(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	cases := []struct {
		name   string
		wantCT string
	}{
		{name: "/logo.png", wantCT: "image/png"},
		{name: "/favicon.png", wantCT: "image/png"},
		{name: "/favicon-64.png", wantCT: "image/png"},
		{name: "/logo.svg", wantCT: "image/svg+xml"},
		{name: "/favicon.svg", wantCT: "image/svg+xml"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.name, nil)
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", tc.name, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != tc.wantCT {
			t.Fatalf("GET %s content-type = %q, want %q", tc.name, ct, tc.wantCT)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("GET %s body empty", tc.name)
		}
	}
}

func TestNonV1ProxyAliasStillRequiresProxyAuth(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/chat/completions", strings.NewReader(`{"model":"gpt-test"}`))
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("non-/v1 proxy alias without auth status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHealthCORSRemainsWildcard(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Origin", "https://monitor.example.com")
	r.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("health CORS allow origin = %q, want *", got)
	}
}

func TestDownstreamPricingRequiresProxyAuth(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	// No Authorization header → must be rejected at the proxy-auth edge
	// (the pricing catalog is downstream-key-gated, not public).
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/pricing", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("/v1/pricing without downstream key status = %d, want 401", rec.Code)
	}
}

// findDistAsset returns a real built filename under web/dist for the given
// subdirectory, preferring an "index*" entry so tests track the actual
// Rsbuild output instead of hardcoding content-hash filenames.
func findDistAsset(t *testing.T, dir string, suffix string) string {
	t.Helper()
	entries, err := fs.ReadDir(web.Dist, dir)
	if err != nil {
		t.Fatalf("fs.ReadDir(%s): %v", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "index") && strings.HasSuffix(entry.Name(), suffix) {
			return entry.Name()
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return entry.Name()
		}
	}
	t.Fatalf("no %q asset found under web/dist/%s", suffix, dir)
	return ""
}

// TestStaticAssetsServedWithRealContentType
//
// Regression for the Rsbuild migration: web/dist/static/js|css|font assets
// fell through to the SPA fallback (200 text/html), so nosniff browsers
// refused them and the embedded single-binary UI stayed blank.
func TestStaticAssetsServedWithRealContentType(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	jsName := findDistAsset(t, "dist/static/js", ".js")
	cssName := findDistAsset(t, "dist/static/css", ".css")
	fontName := findDistAsset(t, "dist/static/font", ".woff2")

	cases := []struct {
		name   string
		path   string
		wantCT string
	}{
		{name: "js chunk", path: "/static/js/" + jsName, wantCT: "javascript"},
		{name: "css", path: "/static/css/" + cssName, wantCT: "text/css"},
		{name: "font", path: "/static/font/" + fontName, wantCT: "font/woff2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d body=%s", tc.path, rec.Code, rec.Body.String())
			}
			ct := rec.Header().Get("Content-Type")
			if !strings.Contains(ct, tc.wantCT) {
				t.Fatalf("GET %s content-type = %q, want it to contain %q", tc.path, ct, tc.wantCT)
			}
			if strings.Contains(ct, "html") {
				t.Fatalf("GET %s content-type = %q, SPA fallback answered for a static asset", tc.path, ct)
			}
			if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
				t.Fatalf("GET %s cache-control = %q, want immutable", tc.path, got)
			}
			if rec.Body.Len() == 0 {
				t.Fatalf("GET %s body is empty", tc.path)
			}
		})
	}
}

// TestStaticAssetMissingReturns404NotSPAFallback
//
// Regression: a missing /static/* file must 404 from the file server instead
// of being answered by the SPA fallback with 200 text/html.
func TestStaticAssetMissingReturns404NotSPAFallback(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/js/__missing_asset__.js", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing static asset status = %d, want 404 (SPA fallback would be 200 text/html)", rec.Code)
	}
}

// TestSPAFallbackServesClientRoutesAfterStaticMount
//
// After the /static/ mount, non-API client routes must still fall back to
// index.html.
func TestSPAFallbackServesClientRoutesAfterStaticMount(t *testing.T) {
	cfg := &config.Config{
		AuthToken:        "admin-token",
		ProxyToken:       "proxy-token",
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("client route status = %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("client route content-type = %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "<html") {
		t.Fatalf("client route body does not look like HTML: %q", body[:min(len(body), 120)])
	}
}
