package proxyhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// insertWSAdmissionKey inserts a managed key that accepts every model, with the
// given budget columns, and returns its ID.
func insertWSAdmissionKey(t *testing.T, db *store.DB, key string, maxCost *float64, usedCost float64, maxRequests *int64, usedRequests int64) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO downstream_api_keys
		 (name, key, enabled, max_cost, used_cost, max_requests, used_requests,
		  supported_models, allowed_route_ids, site_weight_multipliers, excluded_site_ids, excluded_credential_refs,
		  created_at, updated_at)
		 VALUES (?, ?, TRUE, ?, ?, ?, ?, '["*"]', '[]', '{}', '[]', '[]', ?, ?)`,
		"ws-admission-"+key, key, maxCost, usedCost, maxRequests, usedRequests, now, now,
	)
	if err != nil {
		t.Fatalf("INSERT ws admission key: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return id
}

func readWSFrame(t *testing.T, ctx context.Context, conn *websocket.Conn) map[string]any {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read WS frame: %v", err)
	}
	var frame map[string]any
	if err := json.Unmarshal(data, &frame); err != nil {
		t.Fatalf("decode WS frame: %v", err)
	}
	return frame
}

func readWSRequestCount(t *testing.T, db *store.DB, id int64) int64 {
	t.Helper()
	var used int64
	if err := db.QueryRow(`SELECT COALESCE(used_requests, 0) FROM downstream_api_keys WHERE id = ?`, id).Scan(&used); err != nil {
		t.Fatalf("read used_requests: %v", err)
	}
	return used
}

// dialResponsesWS starts a ProxyAuth-gated Responses WS server and returns a
// connected client. Cleanup closes the connection and server.
func dialResponsesWS(t *testing.T, key string) *websocket.Conn {
	t.Helper()
	previous := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(previous) })

	handler := auth.ProxyAuth()(http.HandlerFunc(HandleResponsesWebsocket))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", &websocket.DialOptions{
		HTTPHeader: http.Header{"Authorization": {"Bearer " + key}},
	})
	if err != nil {
		t.Fatalf("dial responses WS: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(websocket.StatusNormalClosure, "test done") })
	return conn
}

func writeWSMessage(t *testing.T, ctx context.Context, conn *websocket.Conn, payload string) {
	t.Helper()
	if err := conn.Write(ctx, websocket.MessageText, []byte(payload)); err != nil {
		t.Fatalf("write WS message: %v", err)
	}
}

const wsPrewarmTurn = `{"type":"response.create","model":"model-a","generate":false,"input":[]}`

// TestResponsesWebsocketTurnRejectsDisabledKey verifies H9/L3: disabling a key
// mid-session stops the next turn, the handshake itself is not billed, and the
// first turn is billed exactly once.
func TestResponsesWebsocketTurnRejectsDisabledKey(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	maxRequests := int64(100)
	id := insertWSAdmissionKey(t, db, "sk-ws-disabled", nil, 0, &maxRequests, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dialResponsesWS(t, "sk-ws-disabled")

	// Local prewarm turn: synthetically completes without an upstream.
	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	created := readWSFrame(t, ctx, conn)
	if created["type"] != "response.created" {
		t.Fatalf("first frame type = %v, want response.created", created["type"])
	}
	completed := readWSFrame(t, ctx, conn)
	if completed["type"] != "response.completed" {
		t.Fatalf("second frame type = %v, want response.completed", completed["type"])
	}
	if got := readWSRequestCount(t, db, id); got != 1 {
		t.Fatalf("used_requests after first turn = %d, want 1 (handshake must not be billed)", got)
	}

	if _, err := db.Exec(`UPDATE downstream_api_keys SET enabled = FALSE WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	frame := readWSFrame(t, ctx, conn)
	if frame["type"] != "error" {
		t.Fatalf("frame after disabling key = %v, want error", frame)
	}
	errDetail, _ := frame["error"].(map[string]any)
	message, _ := errDetail["message"].(string)
	if !strings.Contains(strings.ToLower(message), "disabled") {
		t.Fatalf("error message = %q, want key disabled", message)
	}
	if status, _ := frame["status"].(float64); status != http.StatusForbidden {
		t.Fatalf("error status = %v, want 403", frame["status"])
	}
	if got := readWSRequestCount(t, db, id); got != 1 {
		t.Fatalf("used_requests after rejected turn = %d, want 1", got)
	}

	// The session must end after a terminal key-state failure.
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("expected session to close after disabled-key rejection")
	}
}

// TestResponsesWebsocketTurnRejectsOverCostKey verifies H9: max_cost is
// re-checked on every turn instead of only at handshake.
func TestResponsesWebsocketTurnRejectsOverCostKey(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	maxCost := 1.0
	maxRequests := int64(100)
	id := insertWSAdmissionKey(t, db, "sk-ws-overcost", &maxCost, 0, &maxRequests, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dialResponsesWS(t, "sk-ws-overcost")

	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.created" {
		t.Fatalf("first frame type = %v, want response.created", frame["type"])
	}
	if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
		t.Fatalf("second frame type = %v, want response.completed", frame["type"])
	}
	if got := readWSRequestCount(t, db, id); got != 1 {
		t.Fatalf("used_requests after first turn = %d, want 1", got)
	}

	if _, err := db.Exec(`UPDATE downstream_api_keys SET used_cost = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	frame := readWSFrame(t, ctx, conn)
	if frame["type"] != "error" {
		t.Fatalf("frame after exhausting max_cost = %v, want error", frame)
	}
	errDetail, _ := frame["error"].(map[string]any)
	message, _ := errDetail["message"].(string)
	if !strings.Contains(strings.ToLower(message), "max cost") {
		t.Fatalf("error message = %q, want max cost exceeded", message)
	}
	if status, _ := frame["status"].(float64); status != http.StatusTooManyRequests {
		t.Fatalf("error status = %v, want 429", frame["status"])
	}
	if got := readWSRequestCount(t, db, id); got != 1 {
		t.Fatalf("used_requests after over-cost rejection = %d, want 1", got)
	}
}

// TestResponsesWebsocketTurnPolicyReloaded verifies a tightened accessPolicy
// takes effect on the next turn (the session no longer runs on the handshake
// snapshot).
func TestResponsesWebsocketTurnPolicyReloaded(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	maxRequests := int64(100)
	id := insertWSAdmissionKey(t, db, "sk-ws-policy", nil, 0, &maxRequests, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn := dialResponsesWS(t, "sk-ws-policy")

	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	readWSFrame(t, ctx, conn) // response.created
	readWSFrame(t, ctx, conn) // response.completed

	if _, err := db.Exec(`UPDATE downstream_api_keys SET supported_models = '["model-b"]' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	writeWSMessage(t, ctx, conn, wsPrewarmTurn)
	frame := readWSFrame(t, ctx, conn)
	if frame["type"] != "error" {
		t.Fatalf("frame after policy tightening = %v, want error", frame)
	}
	errDetail, _ := frame["error"].(map[string]any)
	message, _ := errDetail["message"].(string)
	if !strings.Contains(strings.ToLower(message), "not allowed") {
		t.Fatalf("error message = %q, want model not allowed", message)
	}
}
