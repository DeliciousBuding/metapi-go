package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// geminiTraversalEnv wires the same double-hop harness as
// TestConfigureProxyUpstreamWiresRealSQLiteRouter, but mounts the non-/v1
// proxy surface (Gemini native paths) under proxy auth. The upstream recorder
// captures every path it receives so tests can prove which paths escape the
// site API prefix.
type geminiTraversalEnv struct {
	router        chi.Router
	cfg           *config.Config
	upstreamHits  atomic.Int64
	upstreamPaths chan string
}

func newGeminiTraversalEnv(t *testing.T) *geminiTraversalEnv {
	t.Helper()
	_ = store.CloseDatabase()

	env := &geminiTraversalEnv{
		upstreamPaths: make(chan string, 16),
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		env.upstreamHits.Add(1)
		env.upstreamPaths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"totalTokenCount":1}}`))
	}))
	t.Cleanup(upstream.Close)

	cfg := testProxyConfig(t)
	t.Cleanup(func() {
		proxyhandler.SetUpstreamConfig(nil)
		scheduler.SetActiveChannelIDsProvider(nil)
		_ = store.CloseDatabase()
	})
	config.Set(cfg)
	if err := store.EnsureRuntimeDatabase(cfg); err != nil {
		t.Fatalf("EnsureRuntimeDatabase: %v", err)
	}
	// Model requested in the body; the route pattern makes channel selection
	// succeed independently of the (attacker-controlled) path shape.
	seedProxyRoute(t, store.GetDB(), upstream.URL, "gemini-trav", "upstream-token")

	if err := ConfigureProxyUpstream(cfg); err != nil {
		t.Fatalf("ConfigureProxyUpstream: %v", err)
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(auth.ProxyAuth(cfg))
		proxyhandler.RegisterNonV1ProxyRoutes(r)
	})
	env.router = r
	env.cfg = cfg
	return env
}

func (env *geminiTraversalEnv) postGemini(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{"model":"gemini-trav","contents":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.cfg.ProxyToken)
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	return rec
}

// TestGeminiWildcardPathTraversalNeverReachesUpstream is the Wave 4 S-line T1
// regression: a legitimate downstream key holder must not be able to escape
// the site API prefix by smuggling ".." segments through the Gemini wildcard
// route (POST /v1beta/models/*). net/http has already percent-decoded the
// target into r.URL.Path by the time chi matches, so the %2e%2e variant
// arrives identically to the literal form.
func TestGeminiWildcardPathTraversalNeverReachesUpstream(t *testing.T) {
	env := newGeminiTraversalEnv(t)

	targets := []struct {
		name   string
		target string
	}{
		{"literal dot-dot segments", "/v1beta/models/../../admin:probe"},
		{"percent-encoded dot-dot segments", "/v1beta/models/%2e%2e/%2e%2e/admin:probe"},
		{"deep traversal to host root", "/v1beta/models/../../../../etc/passwd"},
	}
	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.postGemini(t, tc.target)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400 (traversal must be rejected before dispatch)",
					rec.Code, rec.Body.String())
			}
			if hits := env.upstreamHits.Load(); hits != 0 {
				t.Fatalf("upstream was hit %d time(s); the \"..\" path escaped the site prefix", hits)
			}
		})
	}
}

// TestGeminiWildcardNormalPathStillForwardedVerbatim pins the forwarding
// contract for paths WITHOUT ".." segments: the hardening must not clean,
// rewrite, or otherwise alter legitimate proxy requests.
func TestGeminiWildcardNormalPathStillForwardedVerbatim(t *testing.T) {
	env := newGeminiTraversalEnv(t)

	rec := env.postGemini(t, "/v1beta/models/gemini-trav:generateContent")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 (normal gemini path must still be proxied)",
			rec.Code, rec.Body.String())
	}
	if hits := env.upstreamHits.Load(); hits != 1 {
		t.Fatalf("upstream hits = %d, want exactly 1; downstream status=%d body=%s",
			hits, rec.Code, rec.Body.String())
	}
	select {
	case upstreamPath := <-env.upstreamPaths:
		if upstreamPath != "/v1beta/models/gemini-trav:generateContent" {
			t.Fatalf("upstream path = %q, want the verbatim downstream path", upstreamPath)
		}
	default:
		t.Fatal("no upstream path recorded")
	}
}
