package proxyhandler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// TestUserBudgetFailoverAfterAuthRejectionSettlesNormally verifies that a
// definitive pre-generation rejection (e.g. 401/429 with no usage payload) on
// attempt 0 does not poison user budget settlement into a permanent unknown
// freeze when a subsequent failover attempt succeeds with valid usage.
func TestUserBudgetFailoverAfterAuthRejectionSettlesNormally(t *testing.T) {
	db, users, user, keys := newBudgetProxyUser(t)
	ctx := context.Background()
	if _, err := users.SetUserBudget(ctx, user.ID, 1, false); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	upstreamFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer upstreamFail.Close()

	upstreamOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":10,"total_tokens":15}}`))
	}))
	defer upstreamOK.Close()

	videoExec(t, db, `INSERT INTO sites(id,name,url,platform,status) VALUES(1,'fail-site',?,'openai','active')`, upstreamFail.URL)
	videoExec(t, db, `INSERT INTO sites(id,name,url,platform,status) VALUES(2,'ok-site',?,'openai','active')`, upstreamOK.URL)
	videoExec(t, db, `INSERT INTO accounts(id,site_id,access_token,api_token,status) VALUES(1,1,'','bad-key','active')`)
	videoExec(t, db, `INSERT INTO accounts(id,site_id,access_token,api_token,status) VALUES(2,2,'','good-key','active')`)
	videoExec(t, db, `INSERT INTO route_channels(route_id,account_id,source_model,priority,weight,enabled) VALUES(1,1,'fixture',0,10,TRUE)`)
	videoExec(t, db, `INSERT INTO route_channels(route_id,account_id,source_model,priority,weight,enabled) VALUES(1,2,'fixture',1,10,TRUE)`)
	for _, table := range []string{"sites", "accounts", "token_routes"} {
		videoExec(t, db, `UPDATE `+table+` SET created_at='now',updated_at='now'`)
	}

	priorCfg, priorUpstream, priorDB := config.GetSafe(), getUpstreamConfig(), store.GetDB()
	store.OverrideDB(db)
	config.Set(&config.Config{ProxyMaxChannelAttempts: 2})
	t.Cleanup(func() {
		store.OverrideDB(priorDB)
		config.Set(priorCfg)
		SetUpstreamConfig(priorUpstream)
		routing.SetGlobalCache(nil)
	})
	SetUpstreamConfig(&UpstreamConfig{
		Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil),
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
	})

	h := requestLifecycleMiddleware(auth.ProxyAuth()(http.HandlerFunc(HandleChatCompletions)), func(context.Context, store.GatewayRequest) error { return nil })
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"fixture","messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set("Authorization", "Bearer "+keys[0])
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 after failover", rec.Code, rec.Body.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("upstream calls = %d, want 2", calls.Load())
	}
	budget, err := users.GetUserBudget(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Frozen || budget.UnknownRequests != 0 || budget.Reserved != 0 || budget.Used <= 0 {
		t.Fatalf("expected clean settled budget after 401->200 failover, got %+v", budget)
	}
}

// TestClientCancelDuringHeaderWaitSkipsFallbackAndFailureRecord verifies that
// cancelling a /v1/messages request while waiting for upstream headers stops
// immediately with 499, does not run cross-protocol fallback, and does not
// record an upstream channel failure.
func TestClientCancelDuringHeaderWaitSkipsFallbackAndFailureRecord(t *testing.T) {
	started := make(chan struct{}, 1)
	releaseUpstream := make(chan struct{})
	var messagesHits, chatHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/messages":
			messagesHits.Add(1)
			select {
			case started <- struct{}{}:
			default:
			}
			select {
			case <-r.Context().Done():
			case <-releaseUpstream:
			}
		case "/v1/chat/completions":
			chatHits.Add(1)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer upstream.Close()
	defer close(releaseUpstream)

	var recordedFailures atomic.Int32
	var recordedLogs []proxy.ProxyLogEntry
	router := &retryBudgetMockRouter{
		channels: []*routing.SelectedChannel{
			{
				Channel:     store.RouteChannel{ID: 101},
				Site:        store.Site{ID: 1, Name: "slow-site", URL: upstream.URL, Platform: "openai"},
				Account:     store.Account{ID: 1, SiteID: 1},
				ActualModel: "claude-sonnet",
				TokenValue:  "tok",
			},
		},
		onFailure: func() { recordedFailures.Add(1) },
	}

	priorCfg, priorUpstream, priorRT := config.GetSafe(), getUpstreamConfig(), config.RuntimeSafe()
	config.Set(&config.Config{ProxyMaxChannelAttempts: 1})
	config.SetRuntime(&config.RuntimeSettings{ProxyFirstByteTimeoutSec: 5})
	t.Cleanup(func() {
		config.Set(priorCfg)
		config.SetRuntime(priorRT)
		SetUpstreamConfig(priorUpstream)
	})
	SetUpstreamConfig(&UpstreamConfig{
		Router: router,
		LogProxy: func(_ context.Context, entry proxy.ProxyLogEntry) error {
			recordedLogs = append(recordedLogs, entry)
			return nil
		},
	})

	reqCtx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"model":"claude-sonnet","max_tokens":16,"messages":[{"role":"user","content":"hi"}]}`))).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Token: "tok", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy}))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		HandleClaudeMessages(rec, req)
	}()

	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream /v1/messages request did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HandleClaudeMessages did not return after client cancel")
	}

	if rec.Code != 499 {
		t.Fatalf("status = %d, want 499", rec.Code)
	}
	if messagesHits.Load() != 1 || chatHits.Load() != 0 {
		t.Fatalf("hits = (messages=%d, chat=%d), want (1, 0) (no fallback after cancel)", messagesHits.Load(), chatHits.Load())
	}
	if recordedFailures.Load() != 0 {
		t.Fatalf("recorded upstream failures = %d, want 0 on client cancel", recordedFailures.Load())
	}
	if len(recordedLogs) != 1 || recordedLogs[0].HTTPStatus != 499 {
		t.Fatalf("recorded logs = %+v, want 1 log with HTTPStatus=499", recordedLogs)
	}
}

type retryBudgetMockRouter struct {
	channels  []*routing.SelectedChannel
	idx       int
	onFailure func()
}

func (m *retryBudgetMockRouter) SelectChannel(context.Context, string, routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if len(m.channels) == 0 {
		return nil, nil
	}
	ch := m.channels[0]
	m.idx = 1
	return ch, nil
}

func (m *retryBudgetMockRouter) SelectNextChannel(context.Context, string, []int64, routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	if m.idx >= len(m.channels) {
		return nil, nil
	}
	ch := m.channels[m.idx]
	m.idx++
	return ch, nil
}

func (m *retryBudgetMockRouter) SelectPreferredChannel(ctx context.Context, model string, _ int64, policy routing.DownstreamRoutingPolicy, _ []int64) (*routing.SelectedChannel, error) {
	return m.SelectChannel(ctx, model, policy)
}

func (m *retryBudgetMockRouter) RecordSuccess(context.Context, int64, float64, float64, *string, *int64) error {
	return nil
}

func (m *retryBudgetMockRouter) RecordFailure(context.Context, int64, routing.SiteRuntimeFailureContext, *int64) error {
	if m.onFailure != nil {
		m.onFailure()
	}
	return nil
}
