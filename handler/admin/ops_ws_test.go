package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/go-chi/chi/v5"
)

// ---- B2: live ops WebSocket ----

func TestOpsWS_RequiresValidToken(t *testing.T) {
	cfg := &config.Config{AuthToken: "admin-secret-token"}
	r := chi.NewRouter()
	RegisterOpsWSRoutes(r, cfg)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws"

	// No token → 403.
	resp, err := http.Get("http" + srv.URL[4:] + "/api/admin/ops/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("no-token status = %d, want 403", resp.StatusCode)
	}

	// Wrong token → 403 (HTTP-level, no upgrade).
	req, err := http.NewRequest(http.MethodGet, "http"+srv.URL[4:]+"/api/admin/ops/ws?token=wrong", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("wrong-token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("wrong-token status = %d, want 403", resp.StatusCode)
	}

	// Correct token → upgrades and pushes frames.
	shared.ResetRealtimeForTest()
	t.Cleanup(shared.ResetRealtimeForTest)
	shared.RecordRealtimeOutcome(true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, httpResp, err := websocket.Dial(ctx, wsURL+"?token=admin-secret-token", nil)
	if err != nil {
		t.Fatalf("dial with token: %v (http %d)", err, httpResp.StatusCode)
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
// admin token is valid; the rejection is purely on the Origin gate.
func TestOpsWS_RejectsCrossOriginWhenUnconfigured(t *testing.T) {
	cfg := &config.Config{AuthToken: "admin-secret-token"} // no AdminCorsAllowedOrigins
	r := chi.NewRouter()
	RegisterOpsWSRoutes(r, cfg)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws?token=admin-secret-token"

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
	cfg := &config.Config{
		AuthToken:               "admin-secret-token",
		AdminCorsAllowedOrigins: []string{"evil.example"},
	}
	r := chi.NewRouter()
	RegisterOpsWSRoutes(r, cfg)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	wsURL := "ws" + srv.URL[4:] + "/api/admin/ops/ws?token=admin-secret-token"

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
