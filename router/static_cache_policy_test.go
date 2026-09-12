package router

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/deliciousbuding/metapi-go/web"
)

// setupSPAFallback serves one cache rule: a content-hashed name is immutable,
// every other name revalidates. Only /static/* is hashed, so the files the
// frontend build copies into the dist root — logos, favicons, bootstrap.js,
// theme-init.js — and the index.html the fallback answers must all come back
// no-cache. An unhashed name served immutable is pinned for a year in every
// browser that already loaded it: a replaced logo never appears and a fixed
// bootstrap script never runs, and neither shows up as an error anywhere.
//
// The five root images were served immutable until this test existed, which is
// why it runs against the shipped dist rather than a fixture — the contract
// worth pinning is what a released binary answers, and a synthetic MapFS would
// keep passing no matter which root files the build actually emits.

// newEmbeddedSPARouter mounts the SPA fallback over the embedded dist, the same
// way TestEmbeddedSpaReferencesAreServedAsAssets does.
func newEmbeddedSPARouter() chi.Router {
	r := chi.NewRouter()
	r.Use(SecurityHeaders)
	setupSPAFallback(r, web.Dist)
	return r
}

func TestShippedDistRootFilesRevalidate(t *testing.T) {
	dist := embeddedDist(t)
	r := newEmbeddedSPARouter()

	entries, err := fs.ReadDir(dist, ".")
	if err != nil {
		t.Fatalf("read the embedded dist root: %v", err)
	}

	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			// /static/* is the only subtree the build emits and every name in
			// it is content-hashed: pinned immutable by the test below.
			continue
		}
		if name == "index.html" {
			// Not registered as a root file — the SPA fallback answers it, and
			// TestSpaFallbackAndAPIPathsKeepTheirOwnHeaders pins that header.
			continue
		}
		checked++

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/"+name, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("GET /%s = %d, want 200: a dist root file must be served as itself, not swallowed by the SPA fallback", name, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); strings.Contains(ct, "html") {
			t.Errorf("GET /%s content-type = %q: the SPA fallback answered for a root file", name, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("GET /%s cache-control = %q, want no-cache: no dist root name is content-hashed, so an immutable header would pin this file for a year", name, cc)
		}
	}

	if checked == 0 {
		t.Fatal("the embedded dist root holds no files to check: this gate must not pass vacuously")
	}
	t.Logf("%d dist root file(s) all revalidate", checked)
}

func TestSpaFallbackAndAPIPathsKeepTheirOwnHeaders(t *testing.T) {
	// embeddedDist skips the job when the checkout carries the CI embed
	// placeholder instead of a real build.
	embeddedDist(t)
	r := newEmbeddedSPARouter()

	// The hashed subtree is the one thing that may be cached for a year.
	asset := findDistAsset(t, "dist/static/js", ".js")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/js/"+asset, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/js/%s = %d, want 200", asset, rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
		t.Errorf("GET /static/js/%s cache-control = %q, want immutable: this name is content-hashed", asset, cc)
	}

	// Everything the SPA answers — the document itself and any client-side
	// route — revalidates, so a deploy reaches an already-open tab.
	for _, path := range []string{"/index.html", "/sites", "/settings/general"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200 from the SPA fallback", path, rec.Code)
			continue
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
			t.Errorf("GET %s content-type = %q, want text/html", path, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("GET %s cache-control = %q, want no-cache", path, cc)
		}
	}

	// The same fallback answers the API surfaces with a JSON 404: an unknown
	// route must never hand a client the SPA document.
	for _, path := range []string{"/api/not-a-route", "/v1/not-a-route"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Errorf("GET %s content-type = %q, want application/json", path, ct)
		}
	}
}
