package proxyhandler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/google/uuid"
)

func testKeyID(id int64) *int64 { return &id }

// TestDownstreamSessionNamespaceIsKeyScopedAndOpaque pins audit L28: the
// namespace must be stable per key, distinct across keys, and never contain the
// raw key or key id.
func TestDownstreamSessionNamespaceIsKeyScopedAndOpaque(t *testing.T) {
	nsA := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(1), Token: "sk-secret-a"})
	nsB := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(2), Token: "sk-secret-b"})
	if nsA == "" || nsB == "" {
		t.Fatal("managed keys must derive a non-empty namespace")
	}
	if nsA == nsB {
		t.Fatal("different keys share a session namespace")
	}
	if repeat := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(1), Token: "sk-secret-a"}); repeat != nsA {
		t.Fatal("namespace is not stable for the same key")
	}
	for _, secret := range []string{"sk-secret", "key:1", "key:2"} {
		if strings.Contains(nsA+nsB, secret) {
			t.Fatalf("namespace leaks %q: %q %q", secret, nsA, nsB)
		}
	}

	globalA := downstreamSessionNamespace(&auth.ProxyAuthContext{Token: "global-secret-a"})
	globalB := downstreamSessionNamespace(&auth.ProxyAuthContext{Token: "global-secret-b"})
	if globalA == "" || globalA == globalB {
		t.Fatal("token-sourced namespaces must differ and be non-empty")
	}
	if strings.Contains(globalA+globalB, "global-secret") {
		t.Fatal("token-sourced namespace leaks the token")
	}
	if got := downstreamSessionNamespace(nil); got != "" {
		t.Fatalf("nil auth namespace = %q, want empty", got)
	}
}

func TestDirectCodexSessionHeaderIsStableAndCrossKeyDistinct(t *testing.T) {
	nsA := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(11)})
	nsB := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(12)})
	downstream := http.Header{}
	downstream.Set("Session-Id", "shared-client-session")
	downstream.Set("Thread-Id", "shared-client-session")
	downstream.Set("X-Codex-Window-Id", "shared-client-session:2")
	downstream.Set("X-Codex-Turn-Metadata", `{"session_id":"shared-client-session","thread_id":"shared-client-session","window_id":"shared-client-session:2","turn_id":"turn-1"}`)

	build := func(namespace string) http.Header {
		headers := http.Header{}
		buildDirectCodexHeaders(headers, downstream, nil, "https://chatgpt.com/backend-api/codex/responses", namespace)
		return headers
	}

	first := build(nsA)
	second := build(nsA)
	other := build(nsB)

	session := first.Get("Session-Id")
	if session == "" || session == "shared-client-session" {
		t.Fatalf("Session-Id = %q, want a namespaced value", session)
	}
	if _, err := uuid.Parse(session); err != nil {
		t.Fatalf("Session-Id %q is not UUID-shaped: %v", session, err)
	}
	if first.Get("Conversation_id") != session {
		t.Fatalf("Conversation_id = %q, want Session-Id %q", first.Get("Conversation_id"), session)
	}
	if first.Get("Thread-Id") != session {
		t.Fatalf("Thread-Id = %q, want namespaced session %q", first.Get("Thread-Id"), session)
	}
	if got := first.Get("X-Codex-Window-Id"); got != session+":2" {
		t.Fatalf("X-Codex-Window-Id = %q, want %q", got, session+":2")
	}
	if second.Get("Session-Id") != session {
		t.Fatal("same key and session produced an unstable Session-Id")
	}
	if other.Get("Session-Id") == session {
		t.Fatal("different keys collided on the same Session-Id")
	}

	// The copied turn metadata must not carry the raw session id either.
	metadata := first.Get("X-Codex-Turn-Metadata")
	if strings.Contains(metadata, "shared-client-session") {
		t.Fatalf("turn metadata leaked the raw session id: %s", metadata)
	}
	for _, want := range []string{`"session_id":"` + session + `"`, `"thread_id":"` + session + `"`, `"window_id":"` + session + `:2"`} {
		if !strings.Contains(metadata, want) {
			t.Fatalf("turn metadata missing %s: %s", want, metadata)
		}
	}
}

// TestDirectCodexIndependentThreadIDIsPreserved proves the namespacing only
// rewrites values that actually carry the client session id.
func TestDirectCodexIndependentThreadIDIsPreserved(t *testing.T) {
	namespace := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(13)})
	downstream := http.Header{}
	downstream.Set("Session-Id", "shared-client-session")
	downstream.Set("Thread-Id", "independent-thread")
	headers := http.Header{}
	buildDirectCodexHeaders(headers, downstream, nil, "https://chatgpt.com/backend-api/codex/responses", namespace)
	if got := headers.Get("Thread-Id"); got != "independent-thread" {
		t.Fatalf("independent Thread-Id = %q, want passthrough", got)
	}
}

func TestDirectClaudeSessionHeaderIsNamespaced(t *testing.T) {
	nsA := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(21)})
	nsB := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(22)})
	downstream := http.Header{}
	downstream.Set("X-Claude-Code-Session-Id", "shared-client-session")

	headersA := http.Header{}
	buildDirectClaudeHeaders(headersA, downstream, true, nsA)
	headersB := http.Header{}
	buildDirectClaudeHeaders(headersB, downstream, true, nsB)

	sessionA := headersA.Get("X-Claude-Code-Session-Id")
	if sessionA == "" || sessionA == "shared-client-session" {
		t.Fatalf("X-Claude-Code-Session-Id = %q, want a namespaced value", sessionA)
	}
	if headersB.Get("X-Claude-Code-Session-Id") == sessionA {
		t.Fatal("different keys collided on the same Claude session header")
	}
}

func TestDirectOpenCodeSessionHeaderIsNamespaced(t *testing.T) {
	nsA := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(31)})
	nsB := downstreamSessionNamespace(&auth.ProxyAuthContext{KeyID: testKeyID(32)})
	downstream := http.Header{}
	downstream.Set("X-Opencode-Session", "shared-client-session")
	endpoint := &store.DirectEndpoint{URL: "https://opencode.example/v1/chat/completions"}
	body := []byte(`{"model":"gpt-fixture","input":"hello"}`)

	wireA, err := prepareDirectOpenCodeWire(endpoint, "", body, downstream, "", nsA)
	if err != nil {
		t.Fatalf("wire A: %v", err)
	}
	wireARepeat, err := prepareDirectOpenCodeWire(endpoint, "", body, downstream, "", nsA)
	if err != nil {
		t.Fatalf("wire A repeat: %v", err)
	}
	wireB, err := prepareDirectOpenCodeWire(endpoint, "", body, downstream, "", nsB)
	if err != nil {
		t.Fatalf("wire B: %v", err)
	}

	sessionA := wireA.Headers.Get("X-Opencode-Session")
	if sessionA == "" || sessionA == "shared-client-session" {
		t.Fatalf("X-Opencode-Session = %q, want a namespaced value", sessionA)
	}
	if wireARepeat.Headers.Get("X-Opencode-Session") != sessionA {
		t.Fatal("same key and session produced an unstable OpenCode session header")
	}
	if wireB.Headers.Get("X-Opencode-Session") == sessionA {
		t.Fatal("different keys collided on the same OpenCode session header")
	}
}
