package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
)

// TestResponsesWebsocketEchoesInsecureAPIKeySubprotocol verifies the browser
// WebSocket auth alternative documented for audit L2: the client offers the key
// as a subprotocol and the server selects it during the upgrade.
func TestResponsesWebsocketEchoesInsecureAPIKeySubprotocol(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		HandleResponsesGet426(w, r)
	})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{Token: "test-global-token", Source: "global", KeyName: "global"}
		r = r.WithContext(auth.WithProxyAuth(r.Context(), pac))
		mux.ServeHTTP(w, r)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/responses"
	const protocol = "openai-insecure-api-key.test-key"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{"unrelated", protocol},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	if got := conn.Subprotocol(); got != protocol {
		t.Fatalf("negotiated subprotocol = %q, want %q", got, protocol)
	}
}
