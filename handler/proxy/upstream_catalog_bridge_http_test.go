package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectResponsesChatHTTPFailureEvents(t *testing.T) {
	const prefix = `data: {"id":"c","choices":[{"index":0,"delta":{"role":"assistant","content":"partial"}}]}` + "\n\n"
	for _, tc := range []struct {
		name, tail, message string
		limit               int
	}{
		{"interrupted", "", "upstream stream interrupted", 0},
		{"input limit", ":" + strings.Repeat("x", 8192) + "\n\n", "upstream stream exceeded configured byte limit", 4096},
		{"converted limit", `data: {"id":"c","choices":[{"index":0,"delta":{"content":"` + strings.Repeat("x", 1550) + `"}}]}` + "\n\n", "stream response exceeded configured byte limit", 2048},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := config.GetSafe()
			cfg := *config.Get()
			cfg.ProxyMaxStreamResponseBytes = tc.limit
			config.Set(&cfg)
			t.Cleanup(func() { config.Set(old) })
			var server *httptest.Server
			if tc.tail == "" {
				server = abruptSSEUpstream(prefix)
			} else {
				server = sseChunkUpstream([]string{prefix, tc.tail}, 0)
			}
			t.Cleanup(server.Close)
			installDirectMediaRuntime(t, store.DialectSQLite, store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: server.URL, Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			rec := directFallbackHTTPRequest("responses", true)
			frames := strings.Split(strings.TrimSpace(rec.Body.String()), "\n\n")
			if rec.Code != 200 || len(frames) < 3 || !strings.Contains(rec.Body.String(), "partial") {
				t.Fatalf("status=%d SSE=%s", rec.Code, rec.Body.String())
			}
			var id string
			for i, frame := range frames {
				e := parseSseBlock(frame)
				var event struct {
					Type, Code, Message string
					Sequence            *int `json:"sequence_number"`
					Response            struct {
						ID, Status string
						Error      struct{ Code, Message string }
					}
				}
				if e == nil || json.Unmarshal([]byte(e.Data), &event) != nil || event.Type != e.Event || event.Sequence == nil || *event.Sequence != i {
					t.Fatalf("invalid SSE frame %d: %s", i, frame)
				}
				switch {
				case i == 0:
					if event.Type != "response.created" || event.Response.ID == "" {
						t.Fatalf("invalid first frame: %s", frame)
					}
					id = event.Response.ID
				case i == len(frames)-2:
					if event.Type != "error" || event.Code != "upstream_error" || event.Message != tc.message {
						t.Fatalf("invalid error: %s", frame)
					}
				case i == len(frames)-1:
					if event.Type != "response.failed" || event.Response.ID != id || event.Response.Status != "failed" || event.Response.Error.Code != "upstream_error" || event.Response.Error.Message != tc.message {
						t.Fatalf("invalid failure: %s", frame)
					}
				default:
					if event.Type == "error" || event.Type == "response.failed" || event.Type == "response.completed" {
						t.Fatalf("premature terminal: %s", frame)
					}
				}
			}
		})
	}
}

func TestOfficialCatalogHTTPSuccessCost(t *testing.T) {
	list, err := pricingcatalog.ParseCatalog([]byte(`{"vendor":{"models":{"client-alias":{"id":"client-alias","cost":{"input":5,"output":12}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	ratios, err := pricingcatalog.ParseNewAPIRatios([]byte(`{"success":true,"data":{"model_ratio":{"client-alias":2},"completion_ratio":{"client-alias":3}}}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := pricingcatalog.MergeSnapshots([]*pricingcatalog.CatalogSnapshot{list, ratios})
	oldCatalog := billingCatalog.Load()
	SetBillingCatalogProvider(func() *pricingcatalog.CatalogSnapshot { return snapshot })
	t.Cleanup(func() { billingCatalog.Store(oldCatalog) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "api.openai.com" || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream %s%s", r.Host, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, directFallbackReplies["chat"])
	}))
	t.Cleanup(server.Close)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}}
	t.Cleanup(transport.CloseIdleConnections)
	oldClient := defaultUpstreamClient
	defaultUpstreamClient = &http.Client{Transport: transport}
	t.Cleanup(func() { defaultUpstreamClient = oldClient })
	router := &upstreamTestRouter{selected: routing.SelectedChannel{
		Site:       store.Site{ID: 3, Status: "active", Platform: "openai", URL: "http://api.openai.com"},
		Channel:    store.RouteChannel{ID: 42, Enabled: true},
		Account:    store.Account{ID: 7, Status: "active"},
		TokenValue: "fixture-key", ActualModel: "provider-model",
	}}
	var logs []proxy.ProxyLogEntry
	prev := getUpstreamConfig()
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(_ context.Context, e proxy.ProxyLogEntry) error { logs = append(logs, e); return nil }})
	t.Cleanup(func() { SetUpstreamConfig(prev) })
	out := directFallbackHTTPRequest("chat", false)
	if out.Code != 200 || len(router.successes) != 1 || len(logs) != 1 || logs[0].Status != "success" {
		t.Fatalf("status=%d successes=%d logs=%+v body=%s", out.Code, len(router.successes), logs, out.Body.String())
	}
	cost := logs[0].EstimatedCost
	if math.Abs(cost-0.000017) > 1e-12 || router.successes[0].cost != cost {
		t.Fatalf("official 1 input + 1 output: routing=%g proxy_log=%g want=0.000017", router.successes[0].cost, cost)
	}
}
