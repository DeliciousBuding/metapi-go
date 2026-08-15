package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/go-chi/chi/v5"
)

// ldohTestCookieValue is a stable, sufficiently long LDOH session cookie value
// used to seed the settings store for proxy passthrough tests. It mirrors the
// minimum length (24 chars after '=') enforced by saveConfig.
var ldohTestCookieValue = "ld_auth_session=" + strings.Repeat("a", 24)

// monitorProxyTestEnv wires a mock LDOH upstream and returns a router + config
// ready to serve /monitor-proxy/ldoh/* requests. The mock server is registered
// for t.Cleanup automatically, the LDOH cookie is seeded in settings, and
// LDOHBaseURL is pointed at the mock upstream.
type monitorProxyTestEnv struct {
	router   chi.Router
	cfg      *config.Config
	upstream *httptest.Server
}

// newMonitorProxyEnv builds the standard LDOH proxy test environment: an
// in-memory SQLite DB, the monitor routes registered on a chi router, and the
// LDOHBaseURL pointed at a mock upstream. The stored LDOH cookie is seeded so
// only the proxy/auth paths remain to be exercised by the caller.
func newMonitorProxyEnv(t *testing.T, upstreamHandler http.HandlerFunc) monitorProxyTestEnv {
	t.Helper()
	db, r, cfg := setupOpsAdminStubsTest(t)
	upstream := httptest.NewServer(upstreamHandler)
	t.Cleanup(upstream.Close)
	cfg.LDOHBaseURL = upstream.URL
	if err := upsertSettingDB(db.DB, ldohCookieSettingKey, ldohTestCookieValue); err != nil {
		t.Fatalf("seed ldoh cookie: %v", err)
	}
	return monitorProxyTestEnv{router: r, cfg: cfg, upstream: upstream}
}

// monitorProxyRequest builds a request against the proxy surface carrying a
// valid monitor session cookie derived from cfg.AuthToken.
func monitorProxyRequest(method, target string, cfg *config.Config) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.AddCookie(&http.Cookie{
		Name:  monitorAuthCookie,
		Value: deriveMonitorSessionToken(cfg.AuthToken),
	})
	return req
}

// ---- resolveLdohProxyPath ----

func TestResolveLdohProxyPath_RootAndSubpaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"exact root collapses to empty", "/monitor-proxy/ldoh", ""},
		{"root with trailing slash collapses to empty", "/monitor-proxy/ldoh/", ""},
		{"single segment preserved", "/monitor-proxy/ldoh/dashboard", "dashboard"},
		{"nested segments preserved verbatim", "/monitor-proxy/ldoh/api/users/42", "api/users/42"},
		{"trailing slash on subpath kept", "/monitor-proxy/ldoh/panel/", "panel/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// httptest.NewRequest cleans some path forms; start from a seed
			// path then overwrite URL.Path directly to preserve exact input.
			req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/seed", nil)
			req.URL.Path = tc.path
			if got := resolveLdohProxyPath(req); got != tc.want {
				t.Fatalf("resolveLdohProxyPath path=%q got=%q want=%q", tc.path, got, tc.want)
			}
		})
	}
}

func TestResolveLdohProxyPath_QueryParamsDoNotLeakIntoPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/dashboard?foo=bar&baz=qux", nil)
	if got := resolveLdohProxyPath(req); got != "dashboard" {
		t.Fatalf("got=%q want=%q (query string must not be treated as path)", got, "dashboard")
	}
}

func TestResolveLdohProxyPath_DotDotTraversalIsNotSanitized(t *testing.T) {
	// resolveLdohProxyPath only trims the proxy prefix; it does not normalize
	// "../" segments. This test pins the current behavior so a future
	// hardening pass that adds sanitization must update it intentionally.
	req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/seed", nil)
	req.URL.Path = "/monitor-proxy/ldoh/../etc/passwd"
	got := resolveLdohProxyPath(req)
	const want = "../etc/passwd"
	if got != want {
		t.Fatalf("got=%q want=%q (traversal should currently pass through unsanitized)", got, want)
	}
}

func TestResolveLdohProxyPath_ChiWildcardFallback(t *testing.T) {
	// The prefix branch wins for any /monitor-proxy/ldoh/... path, so the
	// chi.URLParam(r,"*") fallback is only reachable when URL.Path does NOT
	// start with the proxy prefix (e.g. a routed request with a rewritten
	// path). Seed a chi route context to exercise the fallback directly.
	cases := []struct {
		star string
		want string
	}{
		{"wildcard-no-slash", "wildcard-no-slash"},
		{"/leading-slash-stripped", "leading-slash-stripped"},
		{"/deep/nested/path", "deep/nested/path"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.star, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/unrelated", nil)
			rctx := chi.NewRouteContext()
			if tc.star != "" {
				rctx.URLParams.Add("*", tc.star)
			}
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			if got := resolveLdohProxyPath(req); got != tc.want {
				t.Fatalf("wildcard star=%q got=%q want=%q", tc.star, got, tc.want)
			}
		})
	}
}

func TestResolveLdohProxyPath_UnknownPathWithoutWildcardReturnsEmpty(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/totally/unrelated", nil)
	if got := resolveLdohProxyPath(req); got != "" {
		t.Fatalf("got=%q want empty (no prefix match and no chi wildcard)", got)
	}
}

// ---- rewriteProxyText ----

func TestRewriteProxyText_RewritesRootAnchors(t *testing.T) {
	const baseURL = "https://ldoh.example.com"
	input := `<html><head>` +
		`<script src="/_next/static/chunk.js"></script>` +
		`<link href="/styles/main.css" rel="stylesheet">` +
		`</head><body>` +
		`<img src="/img/logo.png">` +
		`<form action="/login" method="post"></form>` +
		`<a href="/dashboard">dash</a>` +
		`</body></html>`
	got := rewriteProxyText(input, baseURL)
	checks := map[string]string{
		"src root":         `src="/monitor-proxy/ldoh/_next/static/chunk.js"`,
		"href root":        `href="/monitor-proxy/ldoh/styles/main.css"`,
		"img src root":     `src="/monitor-proxy/ldoh/img/logo.png"`,
		"action root":      `action="/monitor-proxy/ldoh/login"`,
		"anchor href root": `href="/monitor-proxy/ldoh/dashboard"`,
	}
	for name, want := range checks {
		if !strings.Contains(got, want) {
			t.Fatalf("%s: rewritten body missing %q\ngot: %s", name, want, got)
		}
	}
}

func TestRewriteProxyText_RewritesBaseURLPrefix(t *testing.T) {
	const baseURL = "https://ldoh.example.com"
	input := `fetch("https://ldoh.example.com/api/users")` +
		`link("https://ldoh.example.com/static/app.js")`
	got := rewriteProxyText(input, baseURL)
	if !strings.Contains(got, `"/monitor-proxy/ldoh/api/users"`) {
		t.Fatalf("baseURL /api replacement missing: %s", got)
	}
	if !strings.Contains(got, `/monitor-proxy/ldoh/static/app.js`) {
		t.Fatalf("baseURL /static replacement missing: %s", got)
	}
	// The original upstream host must no longer appear verbatim in the body.
	if strings.Contains(got, baseURL+"/") {
		t.Fatalf("upstream baseURL leaked into rewritten body: %s", got)
	}
}

func TestRewriteProxyText_RewritesEscapedJSONSlashes(t *testing.T) {
	// Source maps / inline scripts sometimes escape "/" as "\/". The replacer
	// has an escaped-baseURL entry so those are rewritten too.
	const baseURL = "https://ldoh.example.com"
	input := `{"url":"https:\/\/ldoh.example.com\/api\/x"}`
	got := rewriteProxyText(input, baseURL)
	if !strings.Contains(got, `\/monitor-proxy\/ldoh\/api\/x`) {
		t.Fatalf("escaped baseURL not rewritten: %s", got)
	}
}

func TestRewriteProxyText_RewritesApiShorthandQuoted(t *testing.T) {
	const baseURL = "https://ldoh.example.com"
	input := `<a href="/api/me">me</a>`
	got := rewriteProxyText(input, baseURL)
	if !strings.Contains(got, `href="/monitor-proxy/ldoh/api/me"`) {
		t.Fatalf("/api quoted shorthand not rewritten: %s", got)
	}
}

func TestRewriteProxyText_NonMatchingBodyUntouched(t *testing.T) {
	const baseURL = "https://ldoh.example.com"
	input := `{"json":"value","no":"anchors"}`
	if got := rewriteProxyText(input, baseURL); got != input {
		t.Fatalf("body without anchors was mutated:\nwant: %s\ngot:  %s", input, got)
	}
}

// ---- rewriteLocationHeader ----

func TestRewriteLocationHeader(t *testing.T) {
	const baseURL = "https://ldoh.example.com"
	cases := []struct {
		name     string
		location string
		want     string
	}{
		{"empty returns empty", "", ""},
		{"absolute upstream path rewritten", baseURL + "/login", "/monitor-proxy/ldoh/login"},
		{"absolute upstream nested path rewritten", baseURL + "/dashboard/settings", "/monitor-proxy/ldoh/dashboard/settings"},
		{"root-relative path rewritten", "/login", "/monitor-proxy/ldoh/login"},
		{"foreign host left untouched", "https://other.example.com/foo", "https://other.example.com/foo"},
		{"relative path without leading slash left untouched", "relative/path", "relative/path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewriteLocationHeader(tc.location, baseURL); got != tc.want {
				t.Fatalf("rewriteLocationHeader(%q) = %q, want %q", tc.location, got, tc.want)
			}
		})
	}
}

// ---- shouldRewriteProxyBody ----

func TestShouldRewriteProxyBody(t *testing.T) {
	cases := []struct {
		contentType string
		want        bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"application/json", true},
		{"application/json; charset=utf-8", true},
		{"text/css", true},
		{"application/javascript", true},
		{"text/javascript", true},
		{"TEXT/HTML", true}, // case-insensitive
		{"image/png", false},
		{"application/octet-stream", false},
		{"text/plain", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.contentType, func(t *testing.T) {
			if got := shouldRewriteProxyBody(tc.contentType); got != tc.want {
				t.Fatalf("shouldRewriteProxyBody(%q) = %v, want %v", tc.contentType, got, tc.want)
			}
		})
	}
}

// ---- normalizeLdohCookie ----

func TestNormalizeLdohCookie(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty returns empty", "", ""},
		{"whitespace only returns empty", "   \t  ", ""},
		{"bare value gets prefix", "abc123def456", "ld_auth_session=abc123def456"},
		{"already prefixed preserved exactly", "ld_auth_session=abc123def456", "ld_auth_session=abc123def456"},
		{"prefixed with attributes keeps only first pair", "ld_auth_session=abc123; Path=/; HttpOnly", "ld_auth_session=abc123"},
		{"prefixed value with surrounding whitespace trimmed", "  ld_auth_session=abc123  ", "ld_auth_session=abc123"},
		{"prefixed with internal whitespace in value kept", "ld_auth_session=ab c123", "ld_auth_session=ab c123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeLdohCookie(tc.raw); got != tc.want {
				t.Fatalf("normalizeLdohCookie(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestNormalizeLdohCookie_NonLeadingLdohPairFallsBackToPrefix(t *testing.T) {
	// When "ld_auth_session=" appears but is NOT the first pair (e.g.
	// "foo=bar; ld_auth_session=abc"), the first-pair guard fails and the
	// function falls back to prefixing the whole trimmed blob. This pins the
	// current behavior; a future strict parser may reject this input instead.
	raw := "foo=bar; ld_auth_session=abc"
	got := normalizeLdohCookie(raw)
	const want = "ld_auth_session=foo=bar; ld_auth_session=abc"
	if got != want {
		t.Fatalf("got=%q want=%q (non-leading pair fallback)", got, want)
	}
}

// ---- saveConfig / getConfig persistence ----

func TestMonitorSaveConfig_BareValueIsNormalizedBeforePersist(t *testing.T) {
	db, r, _ := setupOpsAdminStubsTest(t)

	bareValue := strings.Repeat("b", 30)
	resp := doPutJSON(t, r, "/api/monitor/config", map[string]any{
		"ldohCookie": bareValue,
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("save status = %d body=%s", resp.Code, resp.Body.String())
	}
	stored := getSettingValue(db.DB, ldohCookieSettingKey)
	if stored != "ld_auth_session="+bareValue {
		t.Fatalf("stored = %q, want normalized %q", stored, "ld_auth_session="+bareValue)
	}
}

func TestMonitorSaveConfig_EmptyBodyClearsStoredCookie(t *testing.T) {
	db, r, _ := setupOpsAdminStubsTest(t)
	if err := upsertSettingDB(db.DB, ldohCookieSettingKey, ldohTestCookieValue); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp := doPutJSON(t, r, "/api/monitor/config", map[string]any{
		"ldohCookie": "",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("clear status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ldohCookieConfigured"] != false {
		t.Fatalf("expected configured=false after clear, got %v", body["ldohCookieConfigured"])
	}
	if getSettingValue(db.DB, ldohCookieSettingKey) != "" {
		t.Fatal("stored cookie was not cleared")
	}
}

func TestMonitorGetConfig_UnconfiguredReportsFalse(t *testing.T) {
	_, r, _ := setupOpsAdminStubsTest(t)

	resp := doGet(t, r, "/api/monitor/config")
	if resp.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["ldohCookieConfigured"] != false {
		t.Fatalf("unconfigured should report configured=false, got %v", body["ldohCookieConfigured"])
	}
	if masked, _ := body["ldohCookieMasked"].(string); masked != "" {
		t.Fatalf("unconfigured masked should be empty, got %q", masked)
	}
}

// ---- ldohProxy end-to-end with a mock upstream ----

func TestLdohProxy_SuccessfulPassthrough(t *testing.T) {
	var receivedCookie string
	var receivedPath string
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		receivedCookie = r.Header.Get("Cookie")
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Upstream-Marker", "ldoh")
		_, _ = w.Write([]byte(`{"ok":true,"service":"ldoh"}`))
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/api/me", env.cfg)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if receivedCookie != ldohTestCookieValue {
		t.Fatalf("upstream Cookie = %q, want stored %q", receivedCookie, ldohTestCookieValue)
	}
	if receivedPath != "/api/me" {
		t.Fatalf("upstream path = %q, want /api/me", receivedPath)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	// The proxy only forwards Content-Type/Location/Cache-Control, so we
	// assert against one of those rather than a synthetic custom header.
	if !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("body = %q, want passthrough JSON", rec.Body.String())
	}
}

func TestLdohProxy_AuthRejection(t *testing.T) {
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be reached when auth fails")
	})

	t.Run("missing session cookie is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/", nil)
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d body=%s, want 401", rec.Code, rec.Body.String())
		}
	})

	t.Run("invalid session cookie is 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/", nil)
		req.AddCookie(&http.Cookie{Name: monitorAuthCookie, Value: "garbage-not-a-session"})
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d body=%s, want 401", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "Missing or invalid monitor session") {
			t.Fatalf("body = %q, want invalid session message", rec.Body.String())
		}
	})

	t.Run("raw auth token rejected as session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/monitor-proxy/ldoh/", nil)
		req.AddCookie(&http.Cookie{Name: monitorAuthCookie, Value: env.cfg.AuthToken})
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("raw AuthToken must not be accepted as session; status=%d", rec.Code)
		}
	})
}

func TestLdohProxy_NoCookieConfiguredReturns400PlainText(t *testing.T) {
	// Distinct from auth failure: a valid session but no stored LDOH cookie
	// yields a 400 plain-text body (parity with the TS implementation).
	_, r, cfg := setupOpsAdminStubsTest(t)
	cfg.LDOHBaseURL = "" // no upstream needed; guard fires first
	// Deliberately do NOT seed the ldoh cookie.

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/", cfg)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain (parity with TS)", ct)
	}
	if !strings.Contains(rec.Body.String(), "LDOH cookie not configured") {
		t.Fatalf("body = %q, want unconfigured message", rec.Body.String())
	}
}

func TestLdohProxy_QueryParamsForwardedToUpstream(t *testing.T) {
	var receivedQuery string
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/api/list?foo=bar&page=2&foo=baz", env.cfg)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	parsed, err := url.ParseQuery(receivedQuery)
	if err != nil {
		t.Fatalf("parse upstream query: %v", err)
	}
	if parsed.Get("foo") != "bar" {
		t.Fatalf("upstream foo = %q, want bar (first value)", parsed.Get("foo"))
	}
	if foos := parsed["foo"]; len(foos) != 2 || foos[0] != "bar" || foos[1] != "baz" {
		t.Fatalf("upstream multi-value foo = %v, want [bar baz]", foos)
	}
	if parsed.Get("page") != "2" {
		t.Fatalf("upstream page = %q, want 2", parsed.Get("page"))
	}
}

func TestLdohProxy_RedirectNotFollowed(t *testing.T) {
	const loginPath = "/login"
	var upstreamHits int
	var env monitorProxyTestEnv
	env = newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		http.Redirect(w, r, env.cfg.LDOHBaseURL+loginPath, http.StatusFound)
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/protected", env.cfg)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d body=%s, want 302 (redirect must not be followed)", rec.Code, rec.Body.String())
	}
	if upstreamHits != 1 {
		t.Fatalf("upstream hits = %d, want exactly 1 (no follow)", upstreamHits)
	}
	loc := rec.Header().Get("Location")
	if loc != "/monitor-proxy/ldoh"+loginPath {
		t.Fatalf("Location = %q, want rewritten %q", loc, "/monitor-proxy/ldoh"+loginPath)
	}
}

func TestLdohProxy_HtmlBodyFromUpstreamIsRewritten(t *testing.T) {
	// End-to-end: a real mock upstream serving text/html with root-anchored
	// links should have its body rewritten by the proxy before the client
	// sees it. The mock server URL is used as baseURL so both the baseURL
	// prefix and src="/ forms are exercised.
	var env monitorProxyTestEnv
	env = newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><body>` +
			`<a href="/dashboard">dash</a>` +
			`<img src="/img/logo.png">` +
			`<a href="` + env.cfg.LDOHBaseURL + `/api/me">me</a>` +
			`</body></html>`))
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/panel", env.cfg)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`href="/monitor-proxy/ldoh/dashboard"`,
		`src="/monitor-proxy/ldoh/img/logo.png"`,
		`/monitor-proxy/ldoh/api/me`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("proxied HTML body missing %q\nbody: %s", want, body)
		}
	}
	// The raw upstream host must not leak through un-rewritten.
	if strings.Contains(body, env.cfg.LDOHBaseURL+"/") {
		t.Fatalf("upstream host leaked into rewritten body: %s", body)
	}
}

func TestLdohProxy_NonRewritableContentTypePassedThrough(t *testing.T) {
	payload := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/logo.png", env.cfg)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", ct)
	}
	got := rec.Body.Bytes()
	if len(got) != len(payload) {
		t.Fatalf("binary body length = %d, want %d (verbatim passthrough)", len(got), len(payload))
	}
	for i, b := range payload {
		if got[i] != b {
			t.Fatalf("binary byte %d = %d, want %d (must not be rewritten)", i, got[i], b)
		}
	}
}
