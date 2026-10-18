package proxyhandler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// newPathTraversalHarness wires a mock upstream and the full proxy route table
// so the tests exercise the same chi matching (RawPath included) as production.
func newPathTraversalHarness(t *testing.T) (chi.Router, *atomic.Int32) {
	t.Helper()
	// Path validation and ownership are independent gates: these tests target
	// the former, so seed a valid owner for the ids whose "allowed shape" case
	// must reach the upstream.
	db := openProxyOwnershipDB(t)
	insertFileOwner(t, db, "file-abc_123", testOwner())
	SaveProxyVideoTask(&ProxyVideoTask{
		PublicID:        "video_abc-123",
		UpstreamVideoID: "video_abc-123",
		Owner:           testOwner(),
	})
	t.Cleanup(func() { DeleteProxyVideoTaskByPublicID("video_abc-123") })

	var hits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"upstream-object","object":"file","deleted":true,"data":[]}`))
	}))
	t.Cleanup(upstream.Close)

	SetUpstreamConfig(&UpstreamConfig{
		Router: &upstreamTestRouter{selected: routing.SelectedChannel{
			Channel:     store.RouteChannel{ID: 1, Enabled: true},
			Account:     store.Account{ID: 2, Status: "active"},
			Site:        store.Site{ID: 3, URL: upstream.URL, Status: "active"},
			TokenValue:  "upstream-token",
			ActualModel: "gpt-4o",
		}},
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	r := chi.NewRouter()
	r.Use(injectAuth)
	r.Route("/v1", func(r chi.Router) {
		RegisterProxyRoutes(r)
	})
	return r, &hits
}

// hostileResourceIDs are the bypass shapes from the audit plus their encoded
// variants. None may ever be concatenated into an upstream path.
func hostileResourceIDs() []struct {
	name string
	id   string
} {
	return []struct {
		name string
		id   string
	}{
		{"encoded slash traversal", "..%2f..%2fadmin"},
		{"encoded dot traversal", "%2e%2e%2fadmin"},
		{"double encoded traversal", "%252e%252e%252fadmin"},
		{"encoded backslash traversal", "..%5c..%5cadmin"},
		{"encoded NUL", "file%00name"},
		{"encoded CRLF", "file%0d%0aX"},
		{"encoded hyphen", "file%2Dabc"},
		{"overlong id", strings.Repeat("a", maxProxiedResourceIDLen+1)},
	}
}

func TestFilesResourceIDRejectsTraversal(t *testing.T) {
	r, hits := newPathTraversalHarness(t)

	routes := []struct {
		method string
		format string
	}{
		{http.MethodGet, "/v1/files/%s"},
		{http.MethodGet, "/v1/files/%s/content"},
		{http.MethodDelete, "/v1/files/%s"},
	}
	for _, route := range routes {
		for _, tc := range hostileResourceIDs() {
			t.Run(route.method+" "+tc.name, func(t *testing.T) {
				before := hits.Load()
				req := makeProxyReqNoBody(route.method, strings.Replace(route.format, "%s", tc.id, 1))
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
				}
				if got := hits.Load(); got != before {
					t.Fatalf("hostile id reached the upstream %d time(s)", got-before)
				}
			})
		}
	}
}

func TestFilesResourceIDAllowsUpstreamShape(t *testing.T) {
	r, hits := newPathTraversalHarness(t)

	req := makeProxyReqNoBody(http.MethodGet, "/v1/files/file-abc_123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1", got)
	}
}

func TestVideosResourceIDRejectsTraversal(t *testing.T) {
	r, hits := newPathTraversalHarness(t)

	routes := []struct {
		method string
		format string
	}{
		{http.MethodGet, "/v1/videos/%s"},
		{http.MethodDelete, "/v1/videos/%s"},
		{http.MethodGet, "/v1/videos/%s/content"},
		{http.MethodPost, "/v1/videos/%s/remix"},
	}
	for _, route := range routes {
		for _, tc := range hostileResourceIDs() {
			t.Run(route.method+" "+tc.name, func(t *testing.T) {
				before := hits.Load()
				req := makeProxyReqNoBody(route.method, strings.Replace(route.format, "%s", tc.id, 1))
				rec := httptest.NewRecorder()
				r.ServeHTTP(rec, req)

				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
				}
				if got := hits.Load(); got != before {
					t.Fatalf("hostile id reached the upstream %d time(s)", got-before)
				}
			})
		}
	}
}

func TestVideosResourceIDAllowsUpstreamShape(t *testing.T) {
	r, hits := newPathTraversalHarness(t)

	req := makeProxyReqNoBody(http.MethodGet, "/v1/videos/video_abc-123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1", got)
	}
}

func TestValidProxiedResourceID(t *testing.T) {
	cases := []struct {
		name string
		id   string
		want bool
	}{
		{"openai file shape", "file-abc_123", true},
		{"public video shape", "video_0123456789abcdef", true},
		{"max length", strings.Repeat("a", maxProxiedResourceIDLen), true},
		{"over max length", strings.Repeat("a", maxProxiedResourceIDLen+1), false},
		{"empty", "", false},
		{"dot", ".", false},
		{"dot-dot", "..", false},
		{"literal slash", "a/b", false},
		{"literal backslash", `a\b`, false},
		{"percent", "a%2fb", false},
		{"space", "a b", false},
		{"control", "a\x00b", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validProxiedResourceID(tc.id); got != tc.want {
				t.Fatalf("validProxiedResourceID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

func TestValidUpstreamDispatchPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"files list", "/v1/files", true},
		{"files info", "/v1/files/file-abc_123", true},
		{"files content", "/v1/files/file-abc_123/content", true},
		{"videos create", "/v1/videos", true},
		{"videos info", "/v1/videos/video_abc-123", true},
		{"videos remix", "/v1/videos/video_abc-123/remix", true},
		{"escaped native video id", "/v1/videos/task%2Fid%20%3F%23%25", true},
		{"gemini query untouched", "/v1beta/models/gemini-2.5-pro:generateContent?alt=sse", true},
		{"literal traversal", "/v1/files/../admin", false},
		{"encoded slash traversal", "/v1/files/..%2f..%2fadmin", false},
		{"encoded dot traversal", "/v1/files/%2e%2e%2fadmin", false},
		{"double encoded traversal", "/v1/files/%252e%252e%252fadmin", false},
		{"triple encoded traversal", "/v1/files/%25252e%25252e%25252fadmin", false},
		{"encoded backslash", "/v1/files/..%5c..%5cadmin", false},
		{"encoded NUL", "/v1/files/file%00name", false},
		{"encoded CRLF", "/v1/files/file%0d%0aX", false},
		{"literal backslash", `/v1/files/a\b`, false},
		{"files id with encoded slash", "/v1/files/a%2fb", false},
		{"videos escapes prefix", "/v1/videos/../../admin", false},
		{"files escapes prefix after clean", "/v1/files/../../v1/chat/completions", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validUpstreamDispatchPath(tc.path); got != tc.want {
				t.Fatalf("validUpstreamDispatchPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
