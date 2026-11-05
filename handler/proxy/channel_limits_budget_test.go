package proxyhandler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestChannelRateRejectionReleasesOrdinaryUserBudget(t *testing.T) {
	db, users, user, keys := newBudgetProxyUser(t)
	ctx := t.Context()
	videoExec(t, db, `UPDATE token_routes SET created_at='2026-11-05T00:00:00Z',updated_at='2026-11-05T00:00:00Z'`)
	if _, err := users.SetUserBudget(ctx, user.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer upstream.Close()
	raw := []byte(fmt.Sprintf(`{"version":"1.4","channels":[{"id":1,"type":"qiniu","name":"limited","base_url":%q,"credentials":{"apiKey":"synthetic-upstream"},"supported_models":["provider"],"settings":{"rateLimit":{"rpm":1}}}],"models":[{"id":1,"model_id":"limited-model","type":"chat","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider"}}]}}]}`, upstream.URL))
	if _, err := backup.ImportAxonHubV14(db, raw, "user-limits", false); err != nil {
		t.Fatal(err)
	}
	var routeID, channelID int64
	if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='limited-model'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&channelID, `SELECT id FROM upstream_channels`); err != nil {
		t.Fatal(err)
	}
	if err := users.GrantUserGroupRoute(ctx, user.GroupID, routeID); err != nil {
		t.Fatal(err)
	}
	manager := channellimit.NewManager()
	if err := manager.TryRequest(ctx, channelID, channellimit.Config{RPM: 1}); err != nil {
		t.Fatal(err)
	}
	priorConfig, priorUpstream := config.GetSafe(), getUpstreamConfig()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	t.Cleanup(func() { config.Set(priorConfig); SetUpstreamConfig(priorUpstream); routing.SetGlobalCache(nil) })
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	router.SetChannelLimits(manager)
	SetUpstreamConfig(&UpstreamConfig{
		Router: router, ChannelLimits: manager,
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
		ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
			return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
		},
	})
	handler := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleChatCompletions)), func(context.Context, store.GatewayRequest) error { return nil })
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"limited-model","messages":[{"role":"user","content":"hello"}],"max_tokens":16}`))
	request.Header.Set("Authorization", "Bearer "+keys[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 503 || calls.Load() != 0 {
		t.Fatalf("rate rejection status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
	}
	budget, err := users.GetUserBudget(ctx, user.ID)
	if err != nil || budget.Reserved != 0 || budget.Used != 0 {
		t.Fatalf("zero-dispatch rate rejection retained or settled a user hold: %+v %v", budget, err)
	}
}
