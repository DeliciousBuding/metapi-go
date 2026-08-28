package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- B2: live ops WebSocket (#1034: one-time ticket auth) ----

// setupOpsWSTest builds the ops WS endpoint backed by a real (in-memory
// SQLite) SessionManager so ticket mint/redemption runs the production path.
func setupOpsWSTest(t *testing.T) (*chi.Mux, *auth.SessionManager) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	sessions := auth.NewSessionManager(db, time.Minute)
	cfg := &config.Config{AuthToken: "admin-secret-token"}
	r := chi.NewRouter()
	RegisterOpsWSRoutes(r, cfg, sessions)
	return r, sessions
}

func TestOpsWS_RequiresValidTicket(t *testing.T) {
	r, sessions := setupOpsWSTest(t)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws"

	// No ticket → 403.
	resp, err := http.Get("http" + srv.URL[4:] + "/api/admin/ops/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no-ticket status = %d, want 403", resp.StatusCode)
	}

	// Legacy ?token=<master token> path is gone: presenting the master token
	// as a query parameter must NOT authenticate (#1034 acceptance: the
	// master token never rides a URL).
	req, err := http.NewRequest(http.MethodGet, "http"+srv.URL[4:]+"/api/admin/ops/ws?token=admin-secret-token", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("legacy-token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("legacy-token status = %d, want 403", resp.StatusCode)
	}

	// Wrong ticket → 403 (HTTP-level, no upgrade).
	req, err = http.NewRequest(http.MethodGet, "http"+srv.URL[4:]+"/api/admin/ops/ws?ticket=wrong", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("wrong-ticket request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-ticket status = %d, want 403", resp.StatusCode)
	}

	// Correct ticket → upgrades and pushes frames.
	shared.ResetRealtimeForTest()
	t.Cleanup(shared.ResetRealtimeForTest)
	shared.RecordRealtimeOutcome(true)

	ticket, _ := sessions.IssueWSTicket()
	if ticket == "" {
		t.Fatal("IssueWSTicket returned an empty ticket")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, httpResp, err := websocket.Dial(ctx, wsURL+"?ticket="+ticket, nil)
	if err != nil {
		status := 0
		if httpResp != nil {
			status = httpResp.StatusCode
		}
		t.Fatalf("dial with ticket: %v (http %d)", err, status)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame struct {
		Lifetime int64 `json:"lifetime"`
		Points   []struct {
			Ts      int64 `json:"ts"`
			Total   int64 `json:"total"`
			Success int64 `json:"success"`
		} `json:"points"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal frame: %v (%s)", err, string(data))
	}
	if frame.Lifetime < 1 {
		t.Fatalf("lifetime = %d, want >= 1", frame.Lifetime)
	}
	if len(frame.Points) != 300 {
		t.Fatalf("points = %d, want 300", len(frame.Points))
	}
	last := frame.Points[len(frame.Points)-1]
	if last.Total < 1 || last.Success < 1 {
		t.Fatalf("last point = %+v, want recorded traffic", last)
	}

	// Single-use: replaying the same ticket after consumption must fail.
	req, err = http.NewRequest(http.MethodGet, "http"+srv.URL[4:]+"/api/admin/ops/ws?ticket="+ticket, nil)
	if err != nil {
		t.Fatalf("build replay request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("replay request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("replayed ticket status = %d, want 403", resp.StatusCode)
	}
}

// TestOpsWSOriginPatterns covers the origin-restriction helper:
//   - nil/empty config -> nil (same-origin only, the safe default)
//   - configured origins -> configured + localhost/127.0.0.1 (dev convenience)
func TestOpsWSOriginPatterns(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want []string
	}{
		{name: "nil config", cfg: nil, want: nil},
		{name: "empty origins", cfg: &config.Config{}, want: nil},
		{
			name: "single origin",
			cfg:  &config.Config{AdminCorsAllowedOrigins: []string{"admin.example.test"}},
			want: []string{"admin.example.test", "localhost", "127.0.0.1"},
		},
		{
			name: "multiple origins",
			cfg:  &config.Config{AdminCorsAllowedOrigins: []string{"a.example", "b.example"}},
			want: []string{"a.example", "b.example", "localhost", "127.0.0.1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := opsWSOriginPatterns(tc.cfg)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestOpsWS_RejectsCrossOriginWhenUnconfigured verifies that with no
// AdminCorsAllowedOrigins set, a cross-origin WebSocket upgrade (Origin host
// != request Host) is rejected with 403 — the safe same-origin default. The
// ticket is valid; the rejection is purely on the Origin gate.
func TestOpsWS_RejectsCrossOriginWhenUnconfigured(t *testing.T) {
	r, sessions := setupOpsWSTest(t) // no AdminCorsAllowedOrigins
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	ticket, _ := sessions.IssueWSTicket()
	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws?ticket=" + ticket

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, httpResp, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}},
	})
	if err == nil {
		t.Fatal("cross-origin dial should fail when origins are unconfigured")
	}
	if httpResp == nil {
		t.Fatal("dial response is nil; want 403 response")
	}
	if httpResp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want 403", httpResp.StatusCode)
	}
}

// TestOpsWS_AllowsConfiguredCrossOrigin verifies that when an origin is
// present in AdminCorsAllowedOrigins, the cross-origin upgrade succeeds and
// the server pushes frames.
func TestOpsWS_AllowsConfiguredCrossOrigin(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	sessions := auth.NewSessionManager(db, time.Minute)
	cfg := &config.Config{
		AuthToken:               "admin-secret-token",
		AdminCorsAllowedOrigins: []string{"evil.example"},
	}
	r := chi.NewRouter()
	RegisterOpsWSRoutes(r, cfg, sessions)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	ticket, _ := sessions.IssueWSTicket()
	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws?ticket=" + ticket

	shared.ResetRealtimeForTest()
	t.Cleanup(shared.ResetRealtimeForTest)
	shared.RecordRealtimeOutcome(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": []string{"http://evil.example"}},
	})
	if err != nil {
		t.Fatalf("configured cross-origin dial failed: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "done")

	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame struct {
		Lifetime int64 `json:"lifetime"`
	}
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("unmarshal frame: %v (%s)", err, string(data))
	}
	if frame.Lifetime < 1 {
		t.Fatalf("lifetime = %d, want >= 1", frame.Lifetime)
	}
}
