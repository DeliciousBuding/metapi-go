package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLdohProxy_PathTraversalRejected is the path-traversal regression: a
// monitor session cookie must not let its holder escape the LDOH base
// subpath by smuggling ".." segments through /monitor-proxy/ldoh/*. net/http
// has already percent-decoded the request target into r.URL.Path before the
// handler runs, so the encoded variant is asserted in its decoded form.
func TestLdohProxy_PathTraversalRejected(t *testing.T) {
	receivedPaths := make(chan string, 16)
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPaths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	targets := []struct {
		name string
		path string
	}{
		{"single dot-dot escapes base", "/monitor-proxy/ldoh/../etc/passwd"},
		{"double dot-dot escapes host root", "/monitor-proxy/ldoh/../../etc/passwd"},
		{"mid-path dot-dot escapes subpath", "/monitor-proxy/ldoh/api/../../../secret"},
		{"trailing dot-dot reaches parent", "/monitor-proxy/ldoh/dashboard/.."},
	}
	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			// httptest.NewRequest normalizes some path forms; overwrite
			// URL.Path directly so the exact traversal input is preserved
			// (same technique as the resolveLdohProxyPath unit tests).
			req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/seed")
			req.URL.Path = tc.path
			rec := httptest.NewRecorder()
			env.router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400 (traversal must be rejected before upstream)",
					rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), "invalid proxy path") {
				t.Fatalf("body = %q, want the invalid proxy path error", rec.Body.String())
			}
		})
	}

	// Percent-encoded variant: pass the encoded target through request
	// parsing so URL.Path carries the decoded form exactly as a real server
	// would present it to the handler.
	t.Run("percent-encoded dot-dot segments", func(t *testing.T) {
		req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/%2e%2e/%2e%2e/etc/passwd")
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d body=%s, want 400 for encoded traversal", rec.Code, rec.Body.String())
		}
	})

	select {
	case upstreamPath := <-receivedPaths:
		t.Fatalf("upstream received traversal path %q; the \"..\" segments escaped the LDOH base", upstreamPath)
	default:
	}
}

// TestLdohProxy_DottedSegmentPathsStillProxied guards against over-cleaning:
// paths that merely contain dots inside a segment (a common shape for static
// asset filenames) are not traversal and must keep flowing to the upstream.
func TestLdohProxy_DottedSegmentPathsStillProxied(t *testing.T) {
	var receivedPath string
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/seed")
	req.URL.Path = "/monitor-proxy/ldoh/static/app..min.js"
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 (dotted segment is not traversal)", rec.Code, rec.Body.String())
	}
	if receivedPath != "/static/app..min.js" {
		t.Fatalf("upstream path = %q, want /static/app..min.js", receivedPath)
	}
}

// TestLdohProxy_DoubleEncodedTraversalRejected covers the L4 gap: net/http
// decodes the request target once, so "%252e%252e%252fadmin" reaches the
// handler as "%2e%2e%2fadmin". An upstream that decodes again would see
// "../admin", so the traversal check must decode a second round before
// deciding the path is safe.
func TestLdohProxy_DoubleEncodedTraversalRejected(t *testing.T) {
	receivedPaths := make(chan string, 4)
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPaths <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	})

	// One decode happens during request parsing; the handler must not trust
	// the resulting literal "%2e%2e%2f" segment.
	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/%252e%252e%252fadmin")
	if req.URL.Path != "/monitor-proxy/ldoh/%2e%2e%2fadmin" {
		t.Fatalf("precondition: URL.Path = %q, want the once-decoded form", req.URL.Path)
	}
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400 for double-encoded traversal", rec.Code, rec.Body.String())
	}
	select {
	case upstreamPath := <-receivedPaths:
		t.Fatalf("upstream received double-encoded traversal path %q", upstreamPath)
	default:
	}

	// Backslash separators are normalized to "/" by some upstream stacks.
	req = monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/seed")
	req.URL.Path = `/monitor-proxy/ldoh/..\..\admin`
	rec = httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for backslash traversal", rec.Code)
	}
}

// TestLdohProxy_ResponseUsesIsolatedCSP pins the M17 mitigation: proxied
// third-party content must never execute as same-origin script.
func TestLdohProxy_ResponseUsesIsolatedCSP(t *testing.T) {
	env := newMonitorProxyEnv(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>third-party</body></html>"))
	})

	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/dashboard"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != monitorProxyCSP {
		t.Fatalf("Content-Security-Policy = %q, want %q", got, monitorProxyCSP)
	}
	for _, directive := range []string{"script-src 'none'", "sandbox", "default-src 'none'"} {
		if !strings.Contains(monitorProxyCSP, directive) {
			t.Fatalf("monitor proxy CSP %q missing %q", monitorProxyCSP, directive)
		}
	}
}
