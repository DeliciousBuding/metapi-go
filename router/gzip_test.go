// gzip_test.go — verifies the SPA static asset pipeline gzips compressible
// responses (js/css/html/svg/json) when the client accepts gzip, passes
// already-compressed binaries (png) through untouched, preserves cache and
// security headers, and falls back to identity encoding otherwise.

package router

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
)

// newSPAFallbackTestRouter mounts setupSPAFallback on a chi router with the
// SecurityHeaders middleware (as production New does) over a small in-memory
// dist, so the gzip behavior can be asserted without a database or the real
// embedded web build.
func newSPAFallbackTestRouter(t *testing.T) chi.Router {
	t.Helper()

	jsBody := strings.Repeat("console.log('metapi static asset');", 512)
	cssBody := strings.Repeat(".metapi{color:var(--primary)}\n", 256)
	htmlBody := "<!doctype html><html><body>metapi</body></html>"
	testFS := fstest.MapFS{
		"dist/index.html":            {Data: []byte(htmlBody)},
		"dist/static/js/app.js":      {Data: []byte(jsBody)},
		"dist/static/css/app.css":    {Data: []byte(cssBody)},
		"dist/static/image/logo.png": {Data: []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 2048))},
		"dist/logo.svg":              {Data: []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>")},
	}

	r := chi.NewRouter()
	r.Use(SecurityHeaders)
	setupSPAFallback(r, testFS)
	return r
}

func serveRequest(
	t *testing.T,
	r chi.Router,
	path, acceptEncoding string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decompressBody(t *testing.T, body io.Reader) string {
	t.Helper()
	reader, err := gzip.NewReader(body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decompressed body: %v", err)
	}
	return string(raw)
}

func TestStaticAssetsServeGzipForCompressibleTypes(t *testing.T) {
	r := newSPAFallbackTestRouter(t)

	tests := []struct {
		name     string
		path     string
		contains string
	}{
		{
			name:     "javascript",
			path:     "/static/js/app.js",
			contains: "console.log('metapi static asset');",
		},
		{
			name:     "css",
			path:     "/static/css/app.css",
			contains: ".metapi{color:var(--primary)}",
		},
		{
			name:     "spa index fallback",
			path:     "/some/client/route",
			contains: "<!doctype html>",
		},
		{
			name:     "root svg",
			path:     "/logo.svg",
			contains: "<svg",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rec := serveRequest(t, r, test.path, "gzip, deflate, br")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want %q", got, "gzip")
			}
			if vary := rec.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
				t.Fatalf("Vary = %q, want it to include Accept-Encoding", vary)
			}
			if rec.Header().Get("Content-Length") != "" {
				t.Fatalf("Content-Length = %q, want empty (compressed responses must not advertise the uncompressed size)", rec.Header().Get("Content-Length"))
			}
			// SecurityHeaders CSP must survive the gzip wrapper.
			if got := rec.Header().Get("Content-Security-Policy"); got == "" {
				t.Fatal("Content-Security-Policy header lost through gzip wrapper")
			}
			body := decompressBody(t, rec.Body)
			if !strings.Contains(body, test.contains) {
				t.Fatalf("decompressed body missing %q; got %q", test.contains, body[:min(64, len(body))])
			}
		})
	}
}

func TestStaticAssetsKeepIdentityWithoutGzipAcceptance(t *testing.T) {
	r := newSPAFallbackTestRouter(t)

	rec := serveRequest(t, r, "/static/js/app.js", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if !strings.Contains(rec.Body.String(), "console.log('metapi static asset');") {
		t.Fatal("uncompressed body corrupted")
	}
}

func TestStaticAssetsDoNotCompressAlreadyCompressedBinaries(t *testing.T) {
	r := newSPAFallbackTestRouter(t)

	rec := serveRequest(t, r, "/static/image/logo.png", "gzip")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("png Content-Encoding = %q, want empty (already-compressed binaries must not be re-compressed)", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", got)
	}
	// Cache header must still be present on the pass-through path.
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q, want immutable cache header preserved", got)
	}
}
