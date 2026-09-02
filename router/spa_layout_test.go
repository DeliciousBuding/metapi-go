package router

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/deliciousbuding/metapi-go/web"
)

// A released binary carries exactly one frontend: //go:embed dist bakes in the
// build produced from this same commit. The contract between that build's
// output layout and the router's mounts is therefore fixed at compile time and
// nothing re-checks it at runtime -- which is how it broke once already. When
// the frontend moved from Vite to Rsbuild the bundles relocated from
// dist/assets to dist/static, the router was still mounting /assets/*, and
// every script and stylesheet request was answered by the SPA fallback with
// 200 text/html. nosniff browsers then refused to execute them and the
// embedded UI rendered blank behind a status code that looked fine.
//
// These tests pin the contract from the side that matters: every root-relative
// asset the built index.html references must be served as that asset, never as
// the fallback HTML, and mounting the dist this commit actually ships must not
// warn. They run against the real embedded dist -- the jobs that execute Go
// tests download the web-dist artifact the frontend job built -- so a rename,
// a new output directory or a dropped root file fails here instead of in a
// browser.

// assetRefPattern picks every src/href target out of the built index.html.
var assetRefPattern = regexp.MustCompile(`(?:src|href)="([^"]+)"`)

// embeddedDist returns the embedded frontend subtree rooted at web/dist.
//
// The CI embed placeholder (web/dist/placeholder.txt, created so //go:embed
// type-checks on a fresh checkout) has no index.html and is skipped with an
// explicit reason. Anything else that lacks index.html is a real problem and
// fails: silently skipping would let this gate pass without ever looking at a
// SPA, which is the failure mode it exists to prevent.
func embeddedDist(t *testing.T) fs.FS {
	t.Helper()

	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		t.Fatalf("web.Dist has no dist subtree: %v", err)
	}
	if _, err := fs.ReadFile(sub, "index.html"); err != nil {
		if _, placeholderErr := fs.ReadFile(sub, "placeholder.txt"); placeholderErr == nil {
			t.Skip("embedded dist is the CI embed placeholder (placeholder.txt, no index.html); the jobs that run Go tests download the real web-dist artifact, so there is no SPA to check here")
		}
		t.Fatalf("embedded dist has no index.html and is not the CI placeholder: %v", err)
	}
	return sub
}

func TestEmbeddedSpaReferencesAreServedAsAssets(t *testing.T) {
	dist := embeddedDist(t)

	indexHTML, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}

	var refs []string
	seen := make(map[string]bool)
	for _, m := range assetRefPattern.FindAllStringSubmatch(string(indexHTML), -1) {
		ref := m[1]
		// Only root-relative references are this router's business. Absolute
		// URLs, protocol-relative URLs, data: URIs and in-page anchors are
		// somebody else's contract.
		if !strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, "//") {
			continue
		}
		if seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}

	if len(refs) == 0 {
		t.Fatal("the embedded index.html references no root-relative assets: either the frontend build changed shape or the embedded dist is not the real one. This gate must not pass vacuously.")
	}

	r := chi.NewRouter()
	r.Use(SecurityHeaders)
	setupSPAFallback(r, web.Dist)

	for _, ref := range refs {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, ref, nil))

		if rec.Code != http.StatusOK {
			t.Errorf("%s: got %d, want 200 — the embedded SPA references an asset this router does not serve", ref, rec.Code)
			continue
		}
		// 200 alone is not the contract. The SPA fallback answers 200
		// text/html for every unmounted path, which is precisely how the
		// Vite→Rsbuild relocation hid a blank UI behind a green status code.
		if ct := rec.Header().Get("Content-Type"); strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: served as %s — the SPA fallback answered instead of the asset; browsers with X-Content-Type-Options: nosniff refuse to execute it and the UI renders blank", ref, ct)
			continue
		}
		if rec.Body.Len() == 0 {
			t.Errorf("%s: served 200 %s with an empty body", ref, rec.Header().Get("Content-Type"))
		}
	}

	t.Logf("%d root-relative asset references in the embedded index.html all resolve to real assets", len(refs))
}

// warnRecorder collects WARN-and-above messages so a test can assert on what
// startup logging claims.
type warnRecorder struct{ messages []string }

func (w *warnRecorder) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (w *warnRecorder) Handle(_ context.Context, r slog.Record) error {
	if r.Level >= slog.LevelWarn {
		w.messages = append(w.messages, r.Message)
	}
	return nil
}

func (w *warnRecorder) WithAttrs([]slog.Attr) slog.Handler { return w }
func (w *warnRecorder) WithGroup(string) slog.Handler      { return w }

// Mounting the dist this commit ships must be silent. The removed /assets/*
// mount logged "embedded web/dist subtree not readable, serving disabled" on
// every startup of every deployment, because the embedded tree has not
// contained an assets/ directory since the Rsbuild migration. An unconditional
// WARN is worse than none: it teaches operators to ignore the level, and it
// sits on the same line a genuine /static failure would use.
func TestSetupSPAFallbackIsQuietAboutTheShippedDist(t *testing.T) {
	dist := embeddedDist(t)
	if _, err := fs.ReadDir(dist, "static"); err != nil {
		t.Fatalf("the embedded dist has no static/ subtree, so the router has nothing to mount: %v", err)
	}

	recorder := &warnRecorder{}
	previous := slog.Default()
	slog.SetDefault(slog.New(recorder))
	defer slog.SetDefault(previous)

	r := chi.NewRouter()
	setupSPAFallback(r, web.Dist)

	for _, msg := range recorder.messages {
		t.Errorf("setupSPAFallback warned while mounting the dist this commit ships: %q", msg)
	}
}
