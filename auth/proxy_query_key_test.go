package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// TestExtractProxyToken_QueryKeyOnlyOnModelListRoutes pins audit L2: the ?key=
// fallback must not authenticate relays, otherwise the key lands in
// reverse-proxy logs and browser history.
func TestExtractProxyToken_QueryKeyOnlyOnModelListRoutes(t *testing.T) {
	allowed := []struct{ method, path string }{
		{http.MethodGet, "/v1/models"},
		{http.MethodGet, "/v1beta/models"},
		{http.MethodGet, "/gemini/v1beta/models"},
		{http.MethodGet, "/gemini/v1/models"},
	}
	for _, tc := range allowed {
		req := newProxyRequest(tc.method, tc.path, nil, "sk-query-key-123")
		if got := extractProxyToken(req); got != "sk-query-key-123" {
			t.Errorf("%s %s query key = %q, want accepted", tc.method, tc.path, got)
		}
	}

	denied := []struct{ method, path string }{
		{http.MethodPost, "/v1/chat/completions"},
		{http.MethodPost, "/v1/messages"},
		{http.MethodPost, "/v1/responses"},
		{http.MethodGet, "/v1/responses"},
		{http.MethodPost, "/v1/models"},
		{http.MethodGet, "/v1/models/gpt-4o"},
		{http.MethodPost, "/v1beta/models/gemini-pro:generateContent"},
		{http.MethodGet, "/gemini/v1beta/models/gemini-pro"},
		{http.MethodGet, "/v1/pricing"},
	}
	for _, tc := range denied {
		req := newProxyRequest(tc.method, tc.path, nil, "sk-query-key-123")
		if got := extractProxyToken(req); got != "" {
			t.Errorf("%s %s query key = %q, want rejected", tc.method, tc.path, got)
		}
	}
}

// TestExtractProxyToken_WebSocketSubprotocol covers the browser WebSocket
// alternative to a URL key. Only real upgrade requests may use it, and the
// Authorization header keeps its exclusive priority.
func TestExtractProxyToken_WebSocketSubprotocol(t *testing.T) {
	upgrade := func(protocol string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
		req.Header.Set("Upgrade", "websocket")
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Sec-WebSocket-Protocol", protocol)
		return req
	}

	if got := extractProxyToken(upgrade("openai-insecure-api-key.sk-ws-key")); got != "sk-ws-key" {
		t.Fatalf("subprotocol key = %q, want sk-ws-key", got)
	}
	if got := extractProxyToken(upgrade("chat, openai-insecure-api-key.sk-ws-key")); got != "sk-ws-key" {
		t.Fatalf("subprotocol key in list = %q, want sk-ws-key", got)
	}
	if got := extractProxyToken(upgrade("unrelated-protocol")); got != "" {
		t.Fatalf("unrelated subprotocol = %q, want empty", got)
	}

	plain := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	plain.Header.Set("Sec-WebSocket-Protocol", "openai-insecure-api-key.sk-ws-key")
	if got := extractProxyToken(plain); got != "" {
		t.Fatalf("non-upgrade subprotocol key = %q, want empty", got)
	}

	exclusive := upgrade("openai-insecure-api-key.sk-ws-key")
	exclusive.Header.Set("Authorization", "Bearer ")
	if got := extractProxyToken(exclusive); got != "" {
		t.Fatalf("empty Authorization must stay exclusive, got %q", got)
	}

	priority := upgrade("openai-insecure-api-key.sk-ws-key")
	priority.Header.Set("x-api-key", "sk-header-key")
	if got := extractProxyToken(priority); got != "sk-header-key" {
		t.Fatalf("x-api-key should win over subprotocol, got %q", got)
	}
}

// TestProxyAuthMiddleware_QueryKeyScopeMatrix checks the 401/allow matrix at
// the middleware boundary: model-list GETs authenticate from ?key=, every other
// route still requires a header.
func TestProxyAuthMiddleware_QueryKeyScopeMatrix(t *testing.T) {
	setupTestDB(t)
	config.SetRuntime(proxyCfg("synthetic-global-token"))
	t.Cleanup(func() { config.SetRuntime(nil) })
	insertTestKeyWithRPM(t, "sk-query-scope", nil, 0, nil, nil)

	handler := ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowed := serveProxyAuth(t, handler, http.MethodGet, "/v1/models?key=sk-query-scope", nil, "")
	if allowed.Code != http.StatusOK {
		t.Fatalf("GET /v1/models?key= status = %d, want 200: %s", allowed.Code, allowed.Body.String())
	}

	denied := serveProxyAuth(t, handler, http.MethodPost, "/v1/chat/completions?key=sk-query-scope", nil, `{"model":"m","messages":[]}`)
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("POST /v1/chat/completions?key= status = %d, want 401: %s", denied.Code, denied.Body.String())
	}
	if !strings.Contains(denied.Body.String(), "missing_api_key") {
		t.Fatalf("denied body should report missing_api_key: %s", denied.Body.String())
	}
}

func TestProxyAuthMiddleware_WebSocketSubprotocolAuthenticates(t *testing.T) {
	setupTestDB(t)
	config.SetRuntime(proxyCfg("synthetic-global-token"))
	t.Cleanup(func() { config.SetRuntime(nil) })
	insertTestKeyWithRPM(t, "sk-ws-protocol", nil, 0, nil, nil)

	handler := ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := serveProxyAuth(t, handler, http.MethodGet, "/v1/responses", map[string]string{
		"Upgrade":                "websocket",
		"Connection":             "Upgrade",
		"Sec-WebSocket-Protocol": "openai-insecure-api-key.sk-ws-protocol",
	}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("WS subprotocol handshake status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	plain := serveProxyAuth(t, handler, http.MethodPost, "/v1/chat/completions", map[string]string{
		"Sec-WebSocket-Protocol": "openai-insecure-api-key.sk-ws-protocol",
	}, `{"model":"m","messages":[]}`)
	if plain.Code != http.StatusUnauthorized {
		t.Fatalf("non-upgrade subprotocol status = %d, want 401: %s", plain.Code, plain.Body.String())
	}
}
