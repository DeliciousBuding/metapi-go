package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestResponsesWebsocket_PrewarmTurn verifies C1 upgrade + generate=false prewarm
// frames without inventing completions for a real upstream turn.
func TestResponsesWebsocket_PrewarmTurn(t *testing.T) {
	// Enable proxy stub so any accidental bridge call would still return 200,
	// but prewarm must short-circuit before bridgeHTTP.
	t.Setenv("METAPI_ENABLE_PROXY_STUB", "1")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/responses", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			HandleResponsesGet426(w, r)
			return
		}
		http.Error(w, "unexpected method", 405)
	})

	// Wrap with synthetic ProxyAuth for upgrade path.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := &auth.ProxyAuthContext{
			Token:   "test-global-token",
			Source:  "global",
			KeyName: "global",
		}
		r = r.WithContext(auth.WithProxyAuth(r.Context(), pac))
		mux.ServeHTTP(w, r)
	})

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/responses"
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer test-global-token"},
		},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	create := map[string]any{
		"type":     "response.create",
		"model":    "gpt-4o",
		"generate": false,
		"input":    []any{},
	}
	raw, _ := json.Marshal(create)
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write create: %v", err)
	}

	// Expect response.created then response.completed.
	types := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		var frame map[string]any
		if err := json.Unmarshal(data, &frame); err != nil {
			t.Fatalf("json: %v body=%s", err, string(data))
		}
		typ, _ := frame["type"].(string)
		types = append(types, typ)
	}
	if types[0] != "response.created" || types[1] != "response.completed" {
		t.Fatalf("frame types = %v, want created+completed", types)
	}
}

// TestResponsesWebsocket_DialWithoutAuthRejected ensures the HTTP upgrade path
// refuses unauthenticated clients before a socket is established.
func TestResponsesWebsocket_DialWithoutAuthRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/responses", HandleResponsesGet426)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/v1/responses"
	_, resp, err := websocket.Dial(ctx, wsURL, nil)
	if err == nil {
		t.Fatal("expected dial failure without auth")
	}
	if resp == nil {
		// Some failures may not surface HTTP response; accept err-only.
		return
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		// Accept may surface other non-101 statuses depending on stack.
		if resp.StatusCode == http.StatusSwitchingProtocols {
			t.Fatalf("must not switch protocols without auth, status=%d", resp.StatusCode)
		}
	}
}

// TestResponsesWebsocketBridgeQueuedRevalidationPreservesClientIP verifies that
// the WS → HTTP fallback bridge inherits the real client RemoteAddr instead of
// a synthetic test address: an IP-restricted managed key passes queued
// revalidation from an allowed client IP, and is rejected if its IP allowlist
// is narrowed/revoked while waiting for a channel slot.
func TestResponsesWebsocketBridgeQueuedRevalidationPreservesClientIP(t *testing.T) {
	for _, tc := range []struct {
		name              string
		revokeWhileWait   bool
		wantTerminal      string
		wantUpstreamCalls int32
		wantErrorStatus   int
	}{
		{name: "allowed_ip_succeeds_after_queue_wait", revokeWhileWait: false, wantTerminal: "response.completed", wantUpstreamCalls: 1},
		{name: "revoked_ip_rejected_after_queue_wait", revokeWhileWait: true, wantTerminal: "error", wantUpstreamCalls: 0, wantErrorStatus: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var upstreamCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upstreamCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-queued\",\"status\":\"completed\",\"model\":\"provider-model\",\"output\":[],\"usage\":{\"input_tokens\":2,\"output_tokens\":1}}}\n\n")
			}))
			t.Cleanup(upstream.Close)

			db := installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{
				Responses: &store.DirectEndpoint{URL: upstream.URL + "/v1/responses", Auth: store.DirectAuthBearer},
			}, store.DirectProtocolResponses, "")
			prevDB := store.GetDB()
			store.OverrideDB(db)
			t.Cleanup(func() { store.OverrideDB(prevDB) })

			var channelID int64
			if err := db.QueryRow(`SELECT id FROM upstream_channels LIMIT 1`).Scan(&channelID); err != nil {
				t.Fatalf("query channel id: %v", err)
			}
			limits := channellimit.Config{MaxConcurrent: 1, QueueSize: 4, QueueTimeoutMs: 5000}
			limitsJSON, err := json.Marshal(limits)
			if err != nil {
				t.Fatalf("marshal limits: %v", err)
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET rate_limit=? WHERE id=?`), string(limitsJSON), channelID); err != nil {
				t.Fatalf("set channel rate_limit: %v", err)
			}

			keyToken := "sk-ws-ip-" + tc.name
			keyID := insertWSAdmissionKey(t, db, keyToken, nil, 0, nil, 0)
			if _, err := db.Exec(db.Rebind(`UPDATE downstream_api_keys SET ip_allowlist='127.0.0.1,::1' WHERE id=?`), keyID); err != nil {
				t.Fatalf("set ip_allowlist: %v", err)
			}

			mgr := channellimit.NewManager()
			cfg := *getUpstreamConfig()
			cfg.ChannelLimits = mgr
			SetUpstreamConfig(&cfg)

			// Occupy the single concurrency slot so the WS turn must queue and
			// trigger revalidateQueuedChannel → auth.RevalidateQueuedRequest.
			blocker, err := mgr.Acquire(t.Context(), channelID, limits)
			if err != nil {
				t.Fatalf("acquire blocker slot: %v", err)
			}
			var releaseOnce sync.Once
			releaseBlocker := func() { releaseOnce.Do(blocker.Release) }
			t.Cleanup(releaseBlocker)

			server := httptest.NewServer(auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket)))
			t.Cleanup(server.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &websocket.DialOptions{
				HTTPHeader: http.Header{"Authorization": {"Bearer " + keyToken}},
			})
			if err != nil {
				t.Fatalf("dial responses WS: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "done")

			turn := `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"hi"}]}`
			if err := conn.Write(ctx, websocket.MessageText, []byte(turn)); err != nil {
				t.Fatalf("write turn: %v", err)
			}

			// Wait until the WS turn is queued behind the blocker slot.
			deadline := time.Now().Add(3 * time.Second)
			for mgr.Snapshot(channelID, limits).Waiting == 0 {
				if time.Now().After(deadline) {
					t.Fatal("WS turn did not enter channel queue")
				}
				time.Sleep(10 * time.Millisecond)
			}

			if tc.revokeWhileWait {
				if _, err := db.Exec(db.Rebind(`UPDATE downstream_api_keys SET ip_allowlist='203.0.113.1' WHERE id=?`), keyID); err != nil {
					t.Fatalf("narrow ip_allowlist: %v", err)
				}
			}
			releaseBlocker()

			var terminal map[string]any
			for range 10 {
				frame := readWSFrame(t, ctx, conn)
				kind, _ := frame["type"].(string)
				if kind == "error" || kind == "response.completed" || kind == "response.failed" {
					terminal = frame
					break
				}
			}
			if terminal == nil {
				t.Fatal("missing terminal WS frame")
			}
			got, _ := terminal["type"].(string)
			if got != tc.wantTerminal {
				t.Fatalf("terminal frame = %q, want %q (frame=%v)", got, tc.wantTerminal, terminal)
			}
			if calls := upstreamCalls.Load(); calls != tc.wantUpstreamCalls {
				t.Fatalf("upstream calls = %d, want %d", calls, tc.wantUpstreamCalls)
			}
			if tc.wantErrorStatus != 0 {
				if status := fmtInt(terminal["status"]); status != tc.wantErrorStatus {
					t.Fatalf("error frame status = %d, want %d (frame=%v)", status, tc.wantErrorStatus, terminal)
				}
			}
		})
	}
}
