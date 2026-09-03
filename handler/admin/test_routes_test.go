package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func setupTestRoutes(t *testing.T) (*store.DB, *channelTestHandler, chi.Router) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{}
	config.SetRuntime(&config.RuntimeSettings{AuthToken: "test-routes-token"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	channel := &channelTestHandler{db: db.DB, cfg: cfg}
	r := chi.NewRouter()
	h := &testHandler{channel: channel}
	r.Post("/api/test/chat", h.chatTest)
	return db, channel, r
}

func TestTestChat_ChannelIDAliasHarness(t *testing.T) {
	db, channel, r := setupTestRoutes(t)
	_, _, _, channelID := insertHarnessFixtures(t, db)

	channel.transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Request:    req,
		}, nil
	})

	resp := doPostJSON(t, r, "/api/test/chat", map[string]any{
		"channelId": channelID,
		"model":     "gpt-4o-mini",
		"messages": []map[string]string{
			{"role": "user", "content": "ping chat"},
		},
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	if out["success"] != true {
		t.Fatalf("out=%v", out)
	}
	if out["mode"] != "chat" {
		t.Fatalf("mode=%v", out["mode"])
	}
}

func TestTestChat_SiteIDAndValidation(t *testing.T) {
	db, channel, r := setupTestRoutes(t)
	siteID, _, _, channelID := insertHarnessFixtures(t, db)

	channel.transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	// Invalid JSON
	req := httptest.NewRequest(http.MethodPost, "/api/test/chat", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid json status=%d", rec.Code)
	}

	// siteId resolution
	resp := doPostJSON(t, r, "/api/test/chat", map[string]any{
		"siteId": siteID,
		"model":  "gpt-4o-mini",
		"prompt": "from site",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
	var out map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	if out["channelId"].(float64) != float64(channelID) {
		t.Fatalf("channelId=%v want %d", out["channelId"], channelID)
	}

	// unknown channel
	resp = doPostJSON(t, r, "/api/test/chat", map[string]any{
		"channelId": 999999,
		"model":     "gpt-4o-mini",
	})
	if resp.Code != http.StatusNotFound {
		t.Fatalf("missing channel status=%d body=%s", resp.Code, resp.Body.String())
	}
}

func TestMapFlexibleToChannelTest_ExtractsNestedFields(t *testing.T) {
	ch := int64(12)
	body := flexibleTestBody{
		ForcedChannelID: &ch,
		Path:            "/v1/chat/completions",
		JSONBody:        json.RawMessage(`{"model":"gpt-4o","messages":[{"role":"user","content":"nested hello"}]}`),
	}
	req, ok := mapFlexibleToChannelTest(body)
	if !ok {
		t.Fatal("expected mapping ok")
	}
	if req.ChannelID == nil || *req.ChannelID != 12 {
		t.Fatalf("channelId=%v", req.ChannelID)
	}
	if req.Model != "gpt-4o" {
		t.Fatalf("model=%q", req.Model)
	}
	if req.Prompt != "nested hello" {
		t.Fatalf("prompt=%q", req.Prompt)
	}
	if req.Mode != channelTestModeChat {
		t.Fatalf("mode=%q", req.Mode)
	}

	// models path detection
	site := int64(3)
	body2 := flexibleTestBody{SiteID: &site, Path: "/v1/models"}
	req2, ok := mapFlexibleToChannelTest(body2)
	if !ok {
		t.Fatal("expected site mapping")
	}
	if req2.Mode != channelTestModeModels {
		t.Fatalf("mode=%q want models", req2.Mode)
	}
}
