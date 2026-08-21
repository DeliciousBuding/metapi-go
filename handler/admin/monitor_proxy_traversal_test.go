package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLdohProxy_PathTraversalRejected is the Wave 4 S-line M1 regression: a
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
			req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/seed", env.cfg)
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
		req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/%2e%2e/%2e%2e/etc/passwd", env.cfg)
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

	req := monitorProxyRequest(http.MethodGet, "/monitor-proxy/ldoh/seed", env.cfg)
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
