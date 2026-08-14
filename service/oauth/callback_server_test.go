package oauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- HTML helpers ----

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"ampersand", "a&b", "a&amp;b"},
		{"less than", "a<b", "a&lt;b"},
		{"greater than", "a>b", "a&gt;b"},
		{"double quote", `a"b`, "a&quot;b"},
		{"single quote", "a'b", "a&#39;b"},
		{"all special chars", `<script>alert("x")&'y'</script>`,
			"&lt;script&gt;alert(&quot;x&quot;)&amp;&#39;y&#39;&lt;/script&gt;"},
		{"no special chars", "plain text", "plain text"},
		{"empty string", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := escapeHTML(tc.input)
			if got != tc.want {
				t.Errorf("escapeHTML(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestRenderCompletionPage_ContainsMessage(t *testing.T) {
	page := renderCompletionPage("Authorization succeeded")
	if !strings.Contains(page, "Authorization succeeded") {
		t.Errorf("page should contain the message, got %q", page)
	}
	if !strings.Contains(page, "<script>window.close();</script>") {
		t.Error("page should include window.close() script")
	}
	if !strings.Contains(page, "<!doctype html>") {
		t.Error("page should be a full HTML document")
	}
}

func TestRenderCompletionPage_EscapesMessage(t *testing.T) {
	// A malicious message with HTML must be escaped, not injected.
	page := renderCompletionPage("<script>alert(1)</script>")
	if strings.Contains(page, "<script>alert(1)</script>") {
		t.Error("raw script tag should be escaped, not embedded verbatim")
	}
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Error("page should contain escaped script tag")
	}
}

func TestRespondHTML_SetsHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	respondHTML(recorder, http.StatusOK, "hello")
	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if ct := recorder.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if cc := recorder.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	if !strings.Contains(recorder.Body.String(), "hello") {
		t.Errorf("body should contain message, got %q", recorder.Body.String())
	}
}

// ---- normalizeOrigin ----

func TestNormalizeOrigin_LoopbackVariants(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		port     int
		want     string
	}{
		{"empty host → localhost", "", 8080, "http://localhost:8080"},
		{"wildcard IPv4 → localhost", "0.0.0.0", 8080, "http://localhost:8080"},
		{"wildcard IPv6 → localhost", "::", 8080, "http://localhost:8080"},
		{"explicit 127.0.0.1", "127.0.0.1", 8080, "http://127.0.0.1:8080"},
		{"explicit localhost", "localhost", 8080, "http://localhost:8080"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeOrigin(tc.host, tc.port)
			if got != tc.want {
				t.Errorf("normalizeOrigin(%q, %d) = %q, want %q", tc.host, tc.port, got, tc.want)
			}
		})
	}
}

func TestNormalizeOrigin_IPv6Bracketed(t *testing.T) {
	// IPv6 addresses contain colons and must be bracketed in the origin URL.
	got := normalizeOrigin("::1", 8080)
	if got != "http://[::1]:8080" {
		t.Errorf("normalizeOrigin(::1, 8080) = %q, want http://[::1]:8080", got)
	}
}

func TestNormalizeOrigin_IPv6WithExistingBrackets(t *testing.T) {
	// A host that already starts with [ should not be double-bracketed.
	got := normalizeOrigin("[::1]", 8080)
	if got != "http://[::1]:8080" {
		t.Errorf("normalizeOrigin([::1], 8080) = %q, want http://[::1]:8080", got)
	}
}

// ---- GetLoopbackCallbackServerStates ----

func TestGetLoopbackCallbackServerStates_ReturnsAllProviders(t *testing.T) {
	// Ensure a clean slate so this test doesn't observe state mutated by
	// StartLoopbackCallbackServer tests in the same package.
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	states := GetLoopbackCallbackServerStates()
	if len(states) == 0 {
		t.Fatal("expected at least one provider state")
	}
	// Every registered provider should appear exactly once.
	if len(states) != len(ListProviderDefinitions()) {
		t.Errorf("state count = %d, want %d (one per provider)",
			len(states), len(ListProviderDefinitions()))
	}
	seen := make(map[string]bool)
	for _, state := range states {
		if state.Provider == "" {
			t.Error("state should have a non-empty Provider")
		}
		if seen[state.Provider] {
			t.Errorf("provider %q appeared more than once", state.Provider)
		}
		seen[state.Provider] = true
	}
}

func TestGetLoopbackCallbackServerState_UnknownProvider(t *testing.T) {
	// Unknown provider returns nil (createDefaultState returns nil for
	// unregistered providers).
	state := GetLoopbackCallbackServerState("definitely-not-a-real-provider")
	if state != nil {
		t.Errorf("unknown provider should yield nil state, got %+v", state)
	}
}

func TestGetLoopbackCallbackServerState_CodexDefaults(t *testing.T) {
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	state := GetLoopbackCallbackServerState("codex")
	if state == nil {
		t.Fatal("expected non-nil state for codex")
	}
	if state.Provider != "codex" {
		t.Errorf("Provider = %q", state.Provider)
	}
	if state.Attempted {
		t.Error("unstarted state should not be Attempted")
	}
	if state.Ready {
		t.Error("unstarted state should not be Ready")
	}
	if state.Port != 1455 {
		t.Errorf("Port = %d, want 1455", state.Port)
	}
	if state.Path != "/auth/callback" {
		t.Errorf("Path = %q", state.Path)
	}
	if state.Origin == "" {
		t.Error("Origin should be normalized even for unstarted state")
	}
	if state.RedirectURI == "" {
		t.Error("RedirectURI should be populated from definition")
	}
}

// ---- StartLoopbackCallbackServer (using grok's port 0 for safety) ----

func TestStartLoopbackCallbackServer_GrokPortZeroBindsSuccessfully(t *testing.T) {
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	// Grok's loopback port is 0 (device OAuth has no callback), so the OS
	// assigns an ephemeral port — no risk of clashing with a real OAuth CLI
	// tool that has claimed 1455/54545/etc.
	state, err := StartLoopbackCallbackServer("grok")
	if err != nil {
		t.Fatalf("expected grok callback server to start on port 0: %v", err)
	}
	if state == nil || !state.Ready {
		t.Fatalf("expected Ready state, got %+v", state)
	}
	if !state.Attempted {
		t.Error("state should be Attempted after start")
	}
}

func TestStartLoopbackCallbackServer_IdempotentSecondCall(t *testing.T) {
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	state1, err := StartLoopbackCallbackServer("grok")
	if err != nil {
		t.Fatalf("first start failed: %v", err)
	}
	state2, err := StartLoopbackCallbackServer("grok")
	if err != nil {
		t.Fatalf("second (idempotent) start failed: %v", err)
	}
	if !state1.Ready || !state2.Ready {
		t.Error("both states should be Ready")
	}
}

func TestStartLoopbackCallbackServer_UnknownProvider(t *testing.T) {
	_, err := StartLoopbackCallbackServer("nonexistent-provider")
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(err.Error(), "unsupported oauth provider") {
		t.Errorf("error should mention unsupported provider, got %q", err.Error())
	}
}

func TestStartLoopbackCallbackServer_ConcurrentStartsWaitForPromise(t *testing.T) {
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	// Start two concurrent starts on the same provider — the second should
	// wait for the in-flight promise and observe the ready state.
	done := make(chan error, 2)
	go func() {
		_, err := StartLoopbackCallbackServer("grok")
		done <- err
	}()
	go func() {
		_, err := StartLoopbackCallbackServer("grok")
		done <- err
	}()
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent start %d failed: %v", i, err)
		}
	}
}

// ---- StartLoopbackCallbackServers (all providers) ----

func TestStartLoopbackCallbackServers_StartsAllProviders(t *testing.T) {
	StopLoopbackCallbackServers()
	t.Cleanup(StopLoopbackCallbackServers)

	states := StartLoopbackCallbackServers()
	if len(states) == 0 {
		t.Fatal("expected non-empty results")
	}
	if len(states) != len(ListProviderDefinitions()) {
		t.Errorf("result count = %d, want %d", len(states), len(ListProviderDefinitions()))
	}
	// Grok should succeed (port 0); the fixed-port providers may or may not
	// depending on whether the port is free on the host. We only assert grok
	// succeeds to keep the test deterministic across environments.
	grokReady := false
	for _, state := range states {
		if state.Provider == "grok" && state.Ready {
			grokReady = true
		}
	}
	if !grokReady {
		t.Error("grok (port 0) should always be ready")
	}
}

// ---- StopLoopbackCallbackServers ----

func TestStopLoopbackCallbackServers_ClearsState(t *testing.T) {
	StopLoopbackCallbackServers()
	_, _ = StartLoopbackCallbackServer("grok")
	StopLoopbackCallbackServers()

	// After stop, state should be reset to defaults (not Ready).
	state := GetLoopbackCallbackServerState("grok")
	if state == nil {
		t.Fatal("expected non-nil state after stop (defaults should be returned)")
	}
	if state.Ready {
		t.Error("state should not be Ready after stop")
	}
	if state.Attempted {
		t.Error("state should not be Attempted after stop")
	}
}

// ---- handleCallbackRequest (the actual HTTP handler) ----

// withFreshSessionStore swaps the global session store for a fresh in-memory
// one and restores the original on cleanup. Ensures each handler test starts
// from a clean slate.
func withFreshSessionStore(t *testing.T) {
	t.Helper()
	original := globalSessionStore
	SetSessionStore(NewMemoryOAuthSessionStore())
	t.Cleanup(func() { globalSessionStore = original })
}

func TestHandleCallbackRequest_ValidSessionDelegatesToHandleCallback(t *testing.T) {
	withFreshSessionStore(t)
	// A valid session + code reaches HandleCallback, which then tries the
	// exchange + DB persistence. Without a mocked token endpoint + DB the
	// downstream call fails, surfacing as 500. The point of this test is to
	// verify the handler delegates to HandleCallback (returns 500, NOT 404
	// or 405) when the path + method + state all look valid.
	session, err := CreateSession(CreateSessionInput{
		Provider:   "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?state="+session.State+"&code=auth-code-xyz", nil)
	handleCallbackRequest("codex", recorder, req)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (delegated to HandleCallback which fails without DB)",
			recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "failed") {
		t.Errorf("body should mention failure, got %q", recorder.Body.String())
	}
}

func TestHandleCallbackRequest_WrongPath_Returns404(t *testing.T) {
	withFreshSessionStore(t)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/wrong-path?state=abc&code=xyz", nil)
	handleCallbackRequest("codex", recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for wrong path", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Not found") {
		t.Errorf("body should mention Not found, got %q", recorder.Body.String())
	}
}

func TestHandleCallbackRequest_PostMethod_Returns405(t *testing.T) {
	withFreshSessionStore(t)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/callback?state=abc&code=xyz", nil)
	handleCallbackRequest("codex", recorder, req)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405 for POST", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET" {
		t.Errorf("Allow header = %q, want GET", got)
	}
}

func TestHandleCallbackRequest_UnknownProvider_Returns404(t *testing.T) {
	withFreshSessionStore(t)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	handleCallbackRequest("nonexistent-provider", recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unknown provider", recorder.Code)
	}
}

func TestHandleCallbackRequest_StateMismatch_Returns500(t *testing.T) {
	withFreshSessionStore(t)
	// No session exists for "bogus-state" → HandleCallback errors → 500.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?state=bogus-state&code=xyz", nil)
	handleCallbackRequest("codex", recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for state mismatch (origin spoofing protection)",
			recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "failed") {
		t.Errorf("body should mention failure, got %q", recorder.Body.String())
	}
}

func TestHandleCallbackRequest_ErrorParam_PropagatesTo500(t *testing.T) {
	withFreshSessionStore(t)
	session, err := CreateSession(CreateSessionInput{
		Provider:   "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?state="+session.State+"&error=access_denied", nil)
	handleCallbackRequest("codex", recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for OAuth error param", recorder.Code)
	}
}

func TestHandleCallbackRequest_MissingCodeAndError_Returns500(t *testing.T) {
	withFreshSessionStore(t)
	session, err := CreateSession(CreateSessionInput{
		Provider:   "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?state="+session.State, nil)
	handleCallbackRequest("codex", recorder, req)
	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 for missing code+error", recorder.Code)
	}
}

func TestHandleCallbackRequest_GrokAlwaysReturns404(t *testing.T) {
	withFreshSessionStore(t)
	// Grok is device OAuth: Loopback.Path is "" so every request path fails
	// the path check and returns 404. This is the intended behaviour — the
	// grok callback server is started for API uniformity but never receives
	// a real callback.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?state=abc&code=xyz", nil)
	handleCallbackRequest("grok", recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (grok has no callback path)", recorder.Code)
	}
}

func TestHandleCallbackRequest_SetsNoStoreCacheHeader(t *testing.T) {
	withFreshSessionStore(t)
	session, err := CreateSession(CreateSessionInput{
		Provider:   "codex",
		RedirectURI: "http://localhost:1455/auth/callback",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// Even on the error path (state delegated to HandleCallback which fails
	// without a DB), the handler must set Cache-Control: no-store to prevent
	// any intermediate proxy from caching an OAuth callback response.
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/auth/callback?state="+session.State+"&code=xyz", nil)
	handleCallbackRequest("codex", recorder, req)
	if cc := recorder.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}
