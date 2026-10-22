package router

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserKeyRotationSecurity(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			users := store.NewGatewayUserStore(f.db)
			key, err := users.CreateOwnKey(context.Background(), f.user.ID, "Rotate", "fixture-security-original", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			path := fmt.Sprintf("/api/user/keys/%d/rotate", key.ID)
			for _, tc := range []struct {
				name    string
				path    string
				cookie  *http.Cookie
				headers map[string]string
				status  int
			}{
				{"missing session", path, nil, nil, 401},
				{"downstream key", path, nil, map[string]string{"Authorization": "Bearer fixture-security-original"}, 401},
				{"admin bearer", path, nil, map[string]string{"Authorization": "Bearer " + portalAdminToken}, 401},
				{"admin cookie", path, &http.Cookie{Name: auth.SessionCookieName, Value: cookie.Value}, nil, 401},
				{"cross origin", path, cookie, map[string]string{"Origin": "https://evil.example"}, 403},
				{"other scheme", path, cookie, map[string]string{"Origin": "https://example.com"}, 403},
				{"same site fetch", path, cookie, map[string]string{"Sec-Fetch-Site": "same-site"}, 403},
				{"invalid id", "/api/user/keys/invalid/rotate", cookie, nil, 400},
				{"nonpositive id", "/api/user/keys/0/rotate", cookie, nil, 400},
				{"missing key", "/api/user/keys/999999/rotate", cookie, nil, 404},
			} {
				t.Run(tc.name, func(t *testing.T) {
					w := f.call("POST", tc.path, "", tc.cookie, tc.headers)
					body := portalJSON(t, w, tc.status)
					if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") || body["key"] != nil || body["item"] != nil || strings.Contains(w.Body.String(), "fixture-security-original") {
						t.Fatal("failed rotation is cacheable or exposes a key")
					}
				})
			}
			for _, target := range []struct {
				table string
				id    int64
			}{{"gateway_users", f.user.ID}, {"gateway_user_groups", f.group.ID}} {
				if _, err := f.db.Exec("UPDATE "+target.table+" SET enabled=FALSE WHERE id=?", target.id); err != nil {
					t.Fatal(err)
				}
				portalJSON(t, f.call("POST", path, "", cookie, nil), 401)
				if _, err := f.db.Exec("UPDATE "+target.table+" SET enabled=TRUE WHERE id=?", target.id); err != nil {
					t.Fatal(err)
				}
			}
			// A response projection failure stays generic and leaves the old
			// credential intact, even though UPDATE RETURNING found the row.
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET supported_models='private malformed policy' WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			w := f.call("POST", path, "", cookie, nil)
			body := portalJSON(t, w, 503)
			if body["error"] != "User service unavailable" || len(body) != 1 || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("rotation failure exposed internal state or is cacheable")
			}
			var stored string
			if err := f.db.Get(&stored, `SELECT key FROM downstream_api_keys WHERE id=?`, key.ID); err != nil || stored != "fixture-security-original" {
				t.Fatalf("denied or failed rotation changed the secret: %v", err)
			}
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET supported_models='["*"]',enabled=FALSE WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			w = f.call("POST", path, "", cookie, map[string]string{"Origin": "http://example.com"})
			body = portalJSON(t, w, 200)
			secret, ok := body["key"].(string)
			if !ok || !strings.HasPrefix(secret, "sk-") || len(secret) != 67 || secret == stored || !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
				t.Fatal("invalid one-time rotation credential or cache policy")
			}
			if decoded, err := hex.DecodeString(secret[3:]); err != nil || len(decoded) != 32 {
				t.Fatal("rotation credential does not contain 256 random bits")
			}
			item := body["item"].(map[string]any)
			if item["enabled"] != false || item["id"] != float64(key.ID) || item["key"] != nil || len(item) != 11 {
				t.Fatal("rotation changed disabled state or exposed private fields")
			}
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer " + secret}), 403)
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer fixture-security-original"}), 401)
		})
	}
}

func TestUserKeyRotationHTTPAndExistingWebsocket(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			var routeID int64
			if err := f.db.Get(&routeID, `INSERT INTO token_routes (model_pattern,enabled,created_at,updated_at) VALUES ('model-a',TRUE,'now','now') RETURNING id`); err != nil {
				t.Fatal(err)
			}
			if err := store.NewGatewayUserStore(f.db).GrantUserGroupRoute(context.Background(), f.group.ID, routeID); err != nil {
				t.Fatal(err)
			}
			created := portalJSON(t, f.call("POST", "/api/user/keys", `{"name":"Client","maxRequests":10}`, cookie, nil), 201)
			key := created["key"].(string)
			keyID := int64(created["item"].(map[string]any)["id"].(float64))
			server := httptest.NewServer(f.router)
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			dial := func(secret string) *websocket.Conn {
				t.Helper()
				conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + secret}}})
				if err != nil {
					t.Fatalf("dial: %v", err)
				}
				return conn
			}
			read := func(conn *websocket.Conn) map[string]any {
				t.Helper()
				_, raw, err := conn.Read(ctx)
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				var frame map[string]any
				if err := json.Unmarshal(raw, &frame); err != nil {
					t.Fatal(err)
				}
				return frame
			}
			write := func(conn *websocket.Conn) {
				t.Helper()
				if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"response.create","model":"model-a","generate":false,"input":[]}`)); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			conn := dial(key)
			defer conn.CloseNow()
			write(conn)
			if frame := read(conn); frame["type"] != "response.created" {
				t.Fatalf("original Key did not start prewarm turn: %+v", frame)
			}
			if frame := read(conn); frame["type"] != "response.completed" {
				t.Fatalf("original Key did not complete prewarm turn: %+v", frame)
			}
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer " + key}), 200)
			rotated := portalJSON(t, f.call("POST", fmt.Sprintf("/api/user/keys/%d/rotate", keyID), "", cookie, nil), 200)
			replacement := rotated["key"].(string)
			if rotated["item"].(map[string]any)["usedRequests"] != float64(2) {
				t.Fatal("rotation reset usage or billed WebSocket handshake")
			}
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer " + key}), 401)
			write(conn)
			frame := read(conn)
			if frame["type"] != "error" || frame["status"] != float64(401) {
				t.Fatalf("old WebSocket survived rotation: %+v", frame)
			}
			if _, _, err := conn.Read(ctx); err == nil || websocket.CloseStatus(err) != websocket.StatusNormalClosure {
				t.Fatalf("revoked WebSocket did not close normally: %v", err)
			}
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer " + replacement}), 200)
			next := dial(replacement)
			defer next.CloseNow()
			write(next)
			if read(next)["type"] != "response.created" || read(next)["type"] != "response.completed" {
				t.Fatal("replacement Key did not complete new WebSocket turn")
			}
			listed := f.call("GET", "/api/user/keys", "", cookie, nil)
			items := portalJSON(t, listed, 200)["items"].([]any)
			if len(items) != 1 || items[0].(map[string]any)["usedRequests"] != float64(4) || strings.Contains(listed.Body.String(), key) || strings.Contains(listed.Body.String(), replacement) {
				t.Fatal("rejected credentials consumed requests or listing exposed a secret")
			}
			// Rotation cannot refill an exhausted request allowance.
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET max_requests=4 WHERE id=?`, keyID); err != nil {
				t.Fatal(err)
			}
			rotatedAgain := portalJSON(t, f.call("POST", fmt.Sprintf("/api/user/keys/%d/rotate", keyID), "", cookie, nil), 200)
			portalJSON(t, f.call("GET", "/v1/models", "", nil, map[string]string{"Authorization": "Bearer " + rotatedAgain["key"].(string)}), 429)
		})
	}
}

func TestUserKeyRotationRateLimit(t *testing.T) {
	f := newPortalFixture(t, store.DialectSQLite)
	cookie := f.activate(f.issue())
	// Invalid IDs still consume the rotation bucket; no secret needs changing.
	for i := range 6 {
		req := httptest.NewRequest("POST", "/api/user/keys/invalid/rotate", nil)
		req.AddCookie(cookie)
		req.RemoteAddr = "198.51.100.42:1234"
		w := httptest.NewRecorder()
		f.router.ServeHTTP(w, req)
		want := 400
		if i == 5 {
			want = 429
		}
		portalJSON(t, w, want)
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("rate-limited response is cacheable")
		}
	}
}
