package backup_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	proxyhandler "github.com/deliciousbuding/metapi-go/handler/proxy"
	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func limitedAxonHubHTTPFixture(t *testing.T, upstream string, limits channellimit.Config) (http.Handler, *channellimit.Manager, *proxy.SiteConcurrencyLimiter, int64, []int64) {
	t.Helper()
	handler := installAxonHubHTTPFixture(t, "openai", upstream, "", "", "chat", func(channel map[string]any) {
		channel["credentials"] = map[string]any{"apiKeys": []string{"synthetic-first", "synthetic-second"}}
		channel["settings"] = map[string]any{"rateLimit": limits}
	})
	db := store.GetDB()
	manager := channellimit.NewManager()
	accounts := proxy.NewSiteConcurrencyLimiter()
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), config.Get(), nil, nil)
	router.SetChannelLimits(manager)
	proxyhandler.SetUpstreamConfig(&proxyhandler.UpstreamConfig{
		Router: router, ChannelLimits: manager, DirectAccountLimiter: accounts,
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
		ResolveDirectCredential: func(ctx context.Context, id int64, proxyURL *string, force bool) (*oauth.DirectCredentialResult, error) {
			return oauth.ResolveDirectCredential(ctx, db.DB, id, proxyURL, force)
		},
	})
	var channelID int64
	var grants []int64
	if err := db.Get(&channelID, `SELECT id FROM upstream_channels`); err != nil {
		t.Fatal(err)
	}
	if err := db.Select(&grants, `SELECT id FROM upstream_grants ORDER BY id`); err != nil || len(grants) != 2 {
		t.Fatalf("credential grants=%v err=%v", grants, err)
	}
	return handler, manager, accounts, channelID, grants
}

func requestLimitedAxonHub(ctx context.Context, handler http.Handler, grant int64, stream bool) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{"model": "client-alias", "stream": stream, "messages": []any{map[string]any{"role": "user", "content": "hello"}}})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	policy := auth.DownstreamRoutingPolicy{
		AllowedCredentialRefs: []auth.ExcludedCredentialRef{{Kind: auth.CredentialRefDirectGrant, GrantID: grant}},
	}
	req = req.WithContext(auth.WithProxyAuth(req.Context(), &auth.ProxyAuthContext{Source: "managed", Policy: policy}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func waitChannelState(t *testing.T, manager *channellimit.Manager, id int64, limits channellimit.Config, wantActive, wantWaiting int64) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := manager.Snapshot(id, limits)
		if snapshot.Active == wantActive && snapshot.Waiting == wantWaiting {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("channel state=%+v want active=%d waiting=%d", snapshot, wantActive, wantWaiting)
		case <-ticker.C:
		}
	}
}

func TestAxonHubChannelLimitsSharedCredentialHTTP(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "json", true: "sse"}[stream], func(t *testing.T) {
			var calls atomic.Int32
			entered := make(chan string, 2)
			release := make(chan struct{}, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				entered <- r.Header.Get("Authorization")
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\ndata: [DONE]\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
				}
			}))
			defer upstream.Close()
			limits := channellimit.Config{RPM: 2, TPM: 4, MaxConcurrent: 1, QueueSize: 1}
			handler, manager, accounts, channelID, grants := limitedAxonHubHTTPFixture(t, upstream.URL, limits)
			// The independent credential guard remains enabled. A waiter for the
			// second credential must not reserve that credential while queued.
			cfgCopy := *config.Get()
			cfgCopy.ProxyDirectAccountMaxConcurrency = 1
			config.Set(&cfgCopy)
			results := []chan *httptest.ResponseRecorder{make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)}
			go func() { results[0] <- requestLimitedAxonHub(t.Context(), handler, grants[0], stream) }()
			if got := <-entered; got != "Bearer synthetic-first" {
				t.Fatalf("first credential=%q", got)
			}
			go func() { results[1] <- requestLimitedAxonHub(t.Context(), handler, grants[1], stream) }()
			waitChannelState(t, manager, channelID, limits, 1, 1)
			var credentialID int64
			if err := store.GetDB().Get(&credentialID, store.GetDB().Rebind(`SELECT credential_id FROM upstream_grants WHERE id=?`), grants[1]); err != nil {
				t.Fatal(err)
			}
			if accounts.ActiveCount(credentialID) != 0 {
				t.Fatal("queued channel held the independent credential permit")
			}
			rejected := requestLimitedAxonHub(t.Context(), handler, grants[1], stream)
			if rejected.Code != 503 || calls.Load() != 1 || manager.Snapshot(channelID, limits).Requests != 1 {
				t.Fatalf("queue rejection dispatched or spent RPM: status=%d calls=%d snapshot=%+v", rejected.Code, calls.Load(), manager.Snapshot(channelID, limits))
			}
			release <- struct{}{}
			if response := <-results[0]; response.Code != 200 {
				t.Fatalf("first response=%d %s", response.Code, response.Body.String())
			}
			if got := <-entered; got != "Bearer synthetic-second" {
				t.Fatalf("second credential=%q", got)
			}
			release <- struct{}{}
			if response := <-results[1]; response.Code != 200 {
				t.Fatalf("second response=%d %s", response.Code, response.Body.String())
			}
			snapshot := manager.Snapshot(channelID, limits)
			if snapshot.Active != 0 || snapshot.Waiting != 0 || snapshot.Requests != 2 || snapshot.Tokens != 6 || snapshot.UnknownUsage != 0 {
				t.Fatalf("shared channel accounting=%+v", snapshot)
			}
			exhausted := requestLimitedAxonHub(t.Context(), handler, grants[0], stream)
			if exhausted.Code != 503 || calls.Load() != 2 {
				t.Fatalf("RPM was per-credential or bypassed: status=%d calls=%d", exhausted.Code, calls.Load())
			}
			var failures int64
			if err := store.GetDB().Get(&failures, `SELECT COALESCE(SUM(fail_count),0) FROM upstream_grants`); err != nil || failures != 0 {
				t.Fatalf("local rejection entered provider cooldown: failures=%d err=%v", failures, err)
			}
		})
	}
}

func TestAxonHubChannelLimitsCancelQueueHTTP(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}, 1), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		entered <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer upstream.Close()
	limits := channellimit.Config{RPM: 4, TPM: 10, MaxConcurrent: 1}
	handler, manager, _, channelID, grants := limitedAxonHubHTTPFixture(t, upstream.URL, limits)
	first := make(chan *httptest.ResponseRecorder, 1)
	go func() { first <- requestLimitedAxonHub(t.Context(), handler, grants[0], false) }()
	<-entered
	queuedCtx, cancel := context.WithCancel(t.Context())
	queued := make(chan *httptest.ResponseRecorder, 1)
	go func() { queued <- requestLimitedAxonHub(queuedCtx, handler, grants[1], false) }()
	waitChannelState(t, manager, channelID, limits, 1, 1)
	cancel()
	if response := <-queued; response.Code != 499 {
		t.Fatalf("queue cancel status=%d body=%s", response.Code, response.Body.String())
	}
	waitChannelState(t, manager, channelID, limits, 1, 0)
	if calls.Load() != 1 || manager.Snapshot(channelID, limits).Requests != 1 {
		t.Fatal("canceled waiter consumed RPM or dispatched")
	}
	close(release)
	if response := <-first; response.Code != 200 {
		t.Fatalf("active request status=%d", response.Code)
	}
	if snapshot := manager.Snapshot(channelID, limits); snapshot.Active != 0 || snapshot.Tokens != 0 || snapshot.UnknownUsage != 1 {
		t.Fatalf("unknown usage became measured zero or leaked slot: %+v", snapshot)
	}
}

func TestAxonHubChannelLimitsPartialStreamUsageHTTP(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		// The upstream ends before a completion event. Usage is still real,
		// even though it is a failed attempt rather than a successful answer.
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n")
	}))
	defer upstream.Close()
	limits := channellimit.Config{RPM: 3, TPM: 5, MaxConcurrent: 1}
	handler, manager, _, channelID, grants := limitedAxonHubHTTPFixture(t, upstream.URL, limits)
	out := requestLimitedAxonHub(t.Context(), handler, grants[0], true)
	if calls.Load() != 1 || !strings.Contains(out.Body.String(), "upstream stream interrupted") {
		t.Fatalf("partial stream was retried or fabricated success: calls=%d body=%s", calls.Load(), out.Body.String())
	}
	snapshot := manager.Snapshot(channelID, limits)
	if snapshot.Active != 0 || snapshot.Requests != 1 || snapshot.Tokens != 3 || snapshot.UnknownUsage != 0 {
		t.Fatalf("partial stream accounting=%+v", snapshot)
	}
}

func TestAxonHubChannelLimitsRevalidateQueuedAuthorizationHTTP(t *testing.T) {
	for _, change := range []string{"grant", "memberProtocol", "key", "model"} {
		t.Run(change, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}],"usage":{"total_tokens":1}}`)
			}))
			defer upstream.Close()
			limits := channellimit.Config{RPM: 10, MaxConcurrent: 1, QueueSize: 2}
			handler, manager, _, channelID, grants := limitedAxonHubHTTPFixture(t, upstream.URL, limits)
			db := store.GetDB()
			held, err := manager.Acquire(t.Context(), channelID, limits)
			if err != nil {
				t.Fatal(err)
			}
			defer held.Release()
			var keyID int64
			token := "synthetic-queued-managed-key"
			if change == "key" {
				if err := db.Get(&keyID, db.Rebind(`INSERT INTO downstream_api_keys(name,key,enabled,supported_models,created_at,updated_at) VALUES('queued',?,?,'["*"]',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`), token, true); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-alias","messages":[{"role":"user","content":"hello"}]}`))
				pac := &auth.ProxyAuthContext{Source: "managed", Policy: auth.DownstreamRoutingPolicy{
					AllowedCredentialRefs: []auth.ExcludedCredentialRef{{Kind: auth.CredentialRefDirectGrant, GrantID: grants[0]}},
				}}
				if change == "key" {
					pac.KeyID, pac.Token = &keyID, token
				}
				req = req.WithContext(auth.WithProxyAuth(t.Context(), pac))
				out := httptest.NewRecorder()
				handler.ServeHTTP(out, req)
				done <- out
			}()
			waitChannelState(t, manager, channelID, limits, 1, 1)
			var mutateErr error
			switch change {
			case "grant":
				_, mutateErr = db.Exec(db.Rebind(`UPDATE upstream_grants SET enabled=? WHERE id=?`), false, grants[0])
			case "memberProtocol":
				_, mutateErr = db.Exec(db.Rebind(`UPDATE upstream_group_items SET protocol_order='[4]' WHERE grant_id=?`), grants[0])
			case "model":
				_, mutateErr = db.Exec(db.Rebind(`UPDATE upstream_models SET enabled=? WHERE channel_id=?`), false, channelID)
			case "key":
				_, mutateErr = db.Exec(db.Rebind(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`), false, keyID)
			}
			if mutateErr != nil {
				t.Fatal(mutateErr)
			}
			// Intentionally leave the normal route cache intact. A queued
			// request must read fresh authorization rather than rely on TTL.
			held.Release()
			response := <-done
			want := 503
			if change == "key" {
				want = 403
			}
			if response.Code != want || calls.Load() != 0 {
				t.Fatalf("revoked queued request dispatched: status=%d calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
			}
			if snapshot := manager.Snapshot(channelID, limits); snapshot.Active != 0 || snapshot.Requests != 0 || snapshot.UnknownUsage != 0 {
				t.Fatalf("revoked request leaked or consumed admission: %+v", snapshot)
			}
		})
	}
}
