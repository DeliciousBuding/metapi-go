package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestIsPublicAPIRoute_LegitimatePaths pins the two intended public surfaces:
// the desktop health probe and single-segment OAuth callbacks.
func TestIsPublicAPIRoute_LegitimatePaths(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/desktop/health", true},
		{"/api/oauth/callback/claude", true},
		{"/api/oauth/callback/codex", true},
		{"/api/oauth/callback/gemini-cli", true},
		// Multi-segment callback tails stay public (pinned parity with
		// TestIsPublicAPIRoute_OAuthCallbackWithPath); chi only registers the
		// single-segment route, so deeper tails 404 before auth runs.
		{"/api/oauth/callback/gemini/code", true},
		{"/api/sites", false},
		{"/api/desktop/health/detail", false},
		{"/api/desktop", false},
		{"/api/oauth/callback", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isPublicAPIRoute(tc.path); got != tc.want {
			t.Errorf("isPublicAPIRoute(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

// TestIsPublicAPIRoute_TraversalVariantsRejected asserts that ".." segments
// never widen the public bypass. chi does not clean paths, so today these
// strings never reach a registered handler — but the middleware predicate
// itself must not hand out a bypass for a path that merely STARTS with a
// public prefix. If a future refactor normalizes paths before (or without)
// chi routing, a loose prefix match would become a full admin auth bypass.
func TestIsPublicAPIRoute_TraversalVariantsRejected(t *testing.T) {
	cases := []string{
		"/api/desktop/health/../secrets",
		"/api/desktop/health/../../api/sites",
		"/api/oauth/callback/../sites",
		"/api/oauth/callback/../../api/settings/backup/export",
		"/api/oauth/callback/claude/../codex",
		"/api/oauth/callback/claude/../../api/sites",
		"/api/oauth/callback/..",
	}
	for _, path := range cases {
		if isPublicAPIRoute(path) {
			t.Errorf("isPublicAPIRoute(%q) = true, want false (traversal variant must not bypass auth)", path)
		}
	}
}

// TestAdminAuth_PublicBypassDoesNotLeakAdminContext verifies that a request
// admitted through the public bypass carries no admin auth marker, so a
// downstream handler can never mistake it for an authenticated admin call.
func TestAdminAuth_PublicBypassDoesNotLeakAdminContext(t *testing.T) {
	cfg := &config.Config{AuthToken: "unit-admin-token"}
	var sawAdmin bool
	handler := AdminAuth(cfg, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAdmin = IsAdmin(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/desktop/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("public route status = %d, want 200", rec.Code)
	}
	if sawAdmin {
		t.Fatal("public bypass request must not carry the admin auth context marker")
	}
}
