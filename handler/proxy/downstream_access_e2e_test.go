package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/go-chi/chi/v5"
)

func TestDownstreamAccessPolicyImportedHTTPRelayAndQuota(t *testing.T) {
	db := setupManagedKeyCostTestDB(t)
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["model"] != "provider-model" {
			t.Errorf("upstream model=%v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`))
	}))
	defer upstream.Close()
	raw := []byte(fmt.Sprintf(`{"version":"1.4","timestamp":"2026-10-09T00:00:00Z","projects":[{"id":1,"name":"fixture","status":"active"}],"channels":[{"id":1,"type":"openai","name":"fixture","status":"enabled","base_url":%q,"credentials":{"apiKey":"fixture-provider-key"},"supported_models":["provider-model","hidden-model"],"endpoints":[{"api_format":"openai/chat_completions","path":"/v1/chat/completions"}]}],"models":[{"id":1,"model_id":"provider-model","status":"enabled","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider-model"}}]}},{"id":2,"model_id":"hidden-model","status":"enabled","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"hidden-model"}}]}}],"api_keys":[{"id":1,"project_id":1,"key":"sk-relay-access","name":"fixture","type":"user","status":"enabled","scopes":["write_requests"],"profiles":{"activeProfile":"active","profiles":[{"name":"active","modelIDs":["provider-model"],"modelMappings":[{"from":"client-alias","to":"provider-model"}],"quota":{"requests":1,"period":{"type":"all_time"}}}]}}],"usage_logs":[]}`, upstream.URL))
	if _, err := backupsvc.ImportAxonHubV14(db, raw, "relay-fixture", false); err != nil {
		t.Fatal(err)
	}
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
	t.Cleanup(func() { SetUpstreamConfig(nil) })
	r := chi.NewRouter()
	r.Use(auth.ProxyAuth())
	r.Route("/v1", RegisterProxyRoutes)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-relay-access")
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		return out
	}
	listed := request("GET", "/v1/models", "")
	if listed.Code != 200 || !strings.Contains(listed.Body.String(), "client-alias") || strings.Contains(listed.Body.String(), "hidden-model") {
		t.Fatalf("model listing crossed boundary: %d %s", listed.Code, listed.Body.String())
	}
	denied := request("POST", "/v1/chat/completions", `{"model":"hidden-model","messages":[{"role":"user","content":"hello"}]}`)
	if denied.Code < 400 || calls.Load() != 0 {
		t.Fatal("denied model reached upstream")
	}
	allowed := request("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`)
	if allowed.Code != 200 || calls.Load() != 1 {
		t.Fatalf("allowed relay failed: %d %s calls=%d", allowed.Code, allowed.Body.String(), calls.Load())
	}
	var response map[string]any
	if err := json.Unmarshal(allowed.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["model"] != "client-alias" {
		t.Fatalf("response model did not preserve client alias: %v", response["model"])
	}
	var count, tokens int64
	if err := db.QueryRowx(`SELECT COUNT(*),COALESCE(SUM(total_tokens),0) FROM downstream_quota_usage`).Scan(&count, &tokens); err != nil || count != 1 || tokens != 7 {
		t.Fatalf("relay quota accounting=%d/%d err=%v", count, tokens, err)
	}
	exhausted := request("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"again"}]}`)
	if exhausted.Code != 429 || calls.Load() != 1 {
		t.Fatalf("exhausted quota reached upstream: %d calls=%d", exhausted.Code, calls.Load())
	}
}
