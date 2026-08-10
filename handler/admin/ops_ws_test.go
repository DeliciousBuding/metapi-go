package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/go-chi/chi/v5"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/shared"
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
