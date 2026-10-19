package proxyhandler

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

// setDirectAccountMaxConcurrencyForTest publishes a config snapshot with the
// per-direct-account cap under test while preserving every other baseline
// field.
func setDirectAccountMaxConcurrencyForTest(t *testing.T, limit int) {
	t.Helper()
	prev := config.GetSafe()
	next := &config.Config{ProxyMaxChannelAttempts: 10}
	if prev != nil {
		copyCfg := *prev
		next = &copyCfg
	}
	next.ProxyDirectAccountMaxConcurrency = limit
	config.Set(next)
	t.Cleanup(func() { config.Set(prev) })
}

func directChatCandidate(credentialID, channelID int64, baseURL string) *store.DirectUpstreamCandidate {
	return &store.DirectUpstreamCandidate{
		CredentialID:   credentialID,
		ChannelID:      channelID,
		CredentialKind: store.DirectCredentialAPIKey,
		Protocols:      store.DirectProtocolChat,
		Endpoints: store.DirectEndpoints{
			Chat: &store.DirectEndpoint{URL: baseURL, Auth: store.DirectAuthBearer},
		},
	}
}

func directAccountSelected(credentialID, channelID int64, baseURL string) *routing.SelectedChannel {
	return &routing.SelectedChannel{
		Channel:     store.RouteChannel{ID: channelID, Enabled: true},
		TokenValue:  "direct-token",
		ActualModel: "gpt-4o-upstream",
		Direct:      directChatCandidate(credentialID, channelID, baseURL),
	}
}

// directSequenceRouter returns the first candidate whose channel is not yet
// excluded, mirroring the production router's exclusion contract.
type directSequenceRouter struct {
	candidates []*routing.SelectedChannel
	failures   []int64
	successes  []int64
}

func (r *directSequenceRouter) pick(exclude []int64) (*routing.SelectedChannel, error) {
	for _, candidate := range r.candidates {
		excluded := false
		for _, id := range exclude {
			if id == candidate.Channel.ID {
				excluded = true
				break
			}
		}
		if !excluded {
			return candidate, nil
		}
	}
	return nil, nil
}

func (r *directSequenceRouter) SelectChannel(_ context.Context, _ string, _ routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	return r.pick(nil)
}

func (r *directSequenceRouter) SelectNextChannel(_ context.Context, _ string, exclude []int64, _ routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error) {
	return r.pick(exclude)
}

func (r *directSequenceRouter) SelectPreferredChannel(_ context.Context, _ string, _ int64, _ routing.DownstreamRoutingPolicy, _ []int64) (*routing.SelectedChannel, error) {
	return nil, nil
}

func (r *directSequenceRouter) RecordSuccess(_ context.Context, channelID int64, _ float64, _ float64, _ *string, _ *int64) error {
	r.successes = append(r.successes, channelID)
	return nil
}

func (r *directSequenceRouter) RecordFailure(_ context.Context, channelID int64, _ routing.SiteRuntimeFailureContext, _ *int64) error {
	r.failures = append(r.failures, channelID)
	return nil
}

func dispatchDirectAccountTestRequest(t *testing.T, cfg *UpstreamConfig, maxRetries int) *httptest.ResponseRecorder {
	t.Helper()
	SetUpstreamConfig(cfg)
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := makeProxyReq("POST", "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	dispatchUpstream(rec, req, &Ctx{
		RequestedModel: "gpt-4o",
		DownstreamPath: "/v1/chat/completions",
		RawBody:        []byte(body),
		MaxRetries:     maxRetries,
	})
	return rec
}

// A saturated account can own several grants (models). Skipping them must not
// spend the request's real upstream retry budget, or a healthy alternative
// channel is never reached when the retry count is small.
func TestDirectAccountSaturationDoesNotConsumeAttemptBudget(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 1)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	limiter := proxy.NewSiteConcurrencyLimiter()
	held, ok := limiter.TryAcquire(55, 1)
	if !ok {
		t.Fatal("pre-hold direct account 55 failed")
	}
	t.Cleanup(held.Release)

	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
		directAccountSelected(55, 101, upstream.URL),
		directAccountSelected(55, 102, upstream.URL),
		directAccountSelected(55, 103, upstream.URL),
		directAccountSelected(66, 204, upstream.URL),
	}}
	rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
		Router:               router,
		DirectAccountLimiter: limiter,
	}, 1)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s (saturated grants consumed the retry budget)", rec.Code, rec.Body.String())
	}
	if len(router.failures) != 0 {
		t.Fatalf("saturation must not record channel failures, got %v", router.failures)
	}
	if len(router.successes) != 1 || router.successes[0] != 204 {
		t.Fatalf("expected healthy credential 66 channel 204 success, got %v", router.successes)
	}
	if got := limiter.ActiveCount(55); got != 1 {
		t.Fatalf("pre-held direct account slot changed: ActiveCount=%d, want 1", got)
	}
	if got := limiter.ActiveCount(66); got != 0 {
		t.Fatalf("healthy direct account slot leaked: ActiveCount=%d, want 0", got)
	}
}

// All candidates belong to a saturated direct account: the request ends as a
// no-route rejection with a distinguishable reason, no upstream attempt, and
// no channel failure/cooldown cascade.
func TestDirectAccountSaturationLifecycleRejectedNoRoute(t *testing.T) {
	db := setupAllFailedAlertDB(t)
	setDirectAccountMaxConcurrencyForTest(t, 1)

	limiter := proxy.NewSiteConcurrencyLimiter()
	held, ok := limiter.TryAcquire(55, 1)
	if !ok {
		t.Fatal("pre-hold direct account 55 failed")
	}
	t.Cleanup(held.Release)

	// Two grants of the same credential: both must be skipped.
	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
		directAccountSelected(55, 101, "http://127.0.0.1:9"),
		directAccountSelected(55, 102, "http://127.0.0.1:9"),
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router, DirectAccountLimiter: limiter})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	// Alert dedup is process-global per model. A unique model keeps repeated
	// runs (and -count) independent without weakening the production cooldown.
	model := fmt.Sprintf("gpt-direct-saturated-%d", time.Now().UnixNano())
	req := makeProxyReq("POST", "/v1/chat/completions",
		fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]}`, model))
	rec := httptest.NewRecorder()
	RequestLifecycle(http.HandlerFunc(HandleChatCompletions)).ServeHTTP(rec, req)

	var gateway store.GatewayRequest
	if err := db.Get(&gateway, "SELECT * FROM gateway_requests"); err != nil {
		t.Fatal(err)
	}
	if gateway.TerminalStatus != "rejected_no_route" || gateway.ReasonCode == nil ||
		*gateway.ReasonCode != "direct_account_concurrency_saturated" ||
		gateway.AttemptCount != 0 || gateway.HTTPStatus == nil || *gateway.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("saturated direct request result: %+v", gateway)
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "No available channels") {
		t.Fatalf("body = %q, want No available channels", rec.Body.String())
	}
	if len(router.failures) != 0 {
		t.Fatalf("saturation must not record channel failures, got %v", router.failures)
	}
	if n := countAllFailedProxyEvents(t, db); n != 1 {
		t.Fatalf("expected 1 proxy event, got %d", n)
	}
	msg := lastAllFailedProxyEventMessage(t, db)
	if !strings.Contains(msg, model) || !strings.Contains(msg, "direct_account_concurrency_saturated") {
		t.Fatalf("event message must distinguish direct saturation: %q", msg)
	}
	var attemptCount int
	if err := db.Get(&attemptCount, "SELECT COUNT(*) FROM proxy_logs"); err != nil {
		t.Fatal(err)
	}
	if attemptCount != 0 {
		t.Fatalf("saturated request dispatched %d attempts, want 0", attemptCount)
	}
}

// 0 (the default) must leave direct dispatch unlimited, and a cap on one
// credential must not affect another credential.
func TestDirectAccountConcurrencyUnlimitedByDefaultAndIndependentPerCredential(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	t.Run("zero is unlimited", func(t *testing.T) {
		setDirectAccountMaxConcurrencyForTest(t, 0)
		limiter := proxy.NewSiteConcurrencyLimiter()
		held, ok := limiter.TryAcquire(55, 1)
		if !ok {
			t.Fatal("pre-hold direct account 55 failed")
		}
		t.Cleanup(held.Release)

		router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
			directAccountSelected(55, 101, upstream.URL),
		}}
		rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
			Router:               router,
			DirectAccountLimiter: limiter,
		}, 0)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if len(router.successes) != 1 || router.successes[0] != 101 {
			t.Fatalf("expected channel 101 success, got %v", router.successes)
		}
		if got := limiter.ActiveCount(55); got != 1 {
			t.Fatalf("unlimited dispatch must not change tracked slots: ActiveCount=%d, want 1", got)
		}
	})

	t.Run("different credential has its own budget", func(t *testing.T) {
		setDirectAccountMaxConcurrencyForTest(t, 1)
		limiter := proxy.NewSiteConcurrencyLimiter()
		held, ok := limiter.TryAcquire(55, 1)
		if !ok {
			t.Fatal("pre-hold direct account 55 failed")
		}
		t.Cleanup(held.Release)

		router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
			directAccountSelected(66, 204, upstream.URL),
		}}
		rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
			Router:               router,
			DirectAccountLimiter: limiter,
		}, 0)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if len(router.successes) != 1 || router.successes[0] != 204 {
			t.Fatalf("expected channel 204 success, got %v", router.successes)
		}
		if got := limiter.ActiveCount(66); got != 0 {
			t.Fatalf("slot leaked for independent credential: ActiveCount=%d, want 0", got)
		}
	})
}

func waitForDirectActiveCount(t *testing.T, limiter *proxy.SiteConcurrencyLimiter, credentialID int64, want int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if limiter.ActiveCount(credentialID) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("ActiveCount(%d) = %d, want %d", credentialID, limiter.ActiveCount(credentialID), want)
}

// The slot must be held for the whole attempt and released on both success and
// a real upstream error.
func TestDirectAccountSlotReleasedAfterSuccessAndUpstreamError(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 1)

	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(okServer.Close)
	errServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	t.Cleanup(errServer.Close)

	t.Run("success", func(t *testing.T) {
		limiter := proxy.NewSiteConcurrencyLimiter()
		router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
			directAccountSelected(55, 101, okServer.URL),
		}}
		rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
			Router:               router,
			DirectAccountLimiter: limiter,
		}, 0)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if got := limiter.ActiveCount(55); got != 0 {
			t.Fatalf("slot leaked after success: ActiveCount=%d", got)
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		limiter := proxy.NewSiteConcurrencyLimiter()
		router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
			directAccountSelected(55, 101, errServer.URL),
		}}
		rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
			Router:               router,
			DirectAccountLimiter: limiter,
		}, 0)
		if rec.Code < 500 {
			t.Fatalf("status=%d body=%s, want upstream failure", rec.Code, rec.Body.String())
		}
		if len(router.failures) == 0 {
			t.Fatal("real upstream error should still record a channel failure")
		}
		if got := limiter.ActiveCount(55); got != 0 {
			t.Fatalf("slot leaked after upstream error: ActiveCount=%d", got)
		}
	})
}

// A panic inside the attempt must still release the credential slot.
func TestDirectAccountSlotReleasedOnPanic(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 1)

	limiter := proxy.NewSiteConcurrencyLimiter()
	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
		directAccountSelected(55, 101, "http://127.0.0.1:9"),
	}}
	SetUpstreamConfig(&UpstreamConfig{
		Router:               router,
		DirectAccountLimiter: limiter,
		ResolveDirectCredential: func(context.Context, int64, *string, bool) (*oauth.DirectCredentialResult, error) {
			panic("credential resolver panic")
		},
	})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`
	req := makeProxyReq("POST", "/v1/chat/completions", body)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected the resolver panic to propagate")
			}
		}()
		dispatchUpstream(httptest.NewRecorder(), req, &Ctx{
			RequestedModel: "gpt-4o",
			DownstreamPath: "/v1/chat/completions",
			RawBody:        []byte(body),
			MaxRetries:     0,
		})
	}()
	if got := limiter.ActiveCount(55); got != 0 {
		t.Fatalf("slot leaked after panic: ActiveCount=%d", got)
	}
}

// A streaming attempt holds the slot until the client disconnects, then
// releases it.
func TestDirectAccountSlotHeldThroughStreamAndReleasedOnClientCancel(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 1)

	upstreamStarted := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"1\",\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
		close(upstreamStarted)
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	limiter := proxy.NewSiteConcurrencyLimiter()
	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
		directAccountSelected(55, 101, upstream.URL),
	}}
	SetUpstreamConfig(&UpstreamConfig{Router: router, DirectAccountLimiter: limiter})
	t.Cleanup(func() { SetUpstreamConfig(nil) })

	downstream := httptest.NewServer(injectAuth(http.HandlerFunc(HandleChatCompletions)))
	t.Cleanup(downstream.Close)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	select {
	case <-upstreamStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream stream did not start")
	}
	reader := bufio.NewReader(resp.Body)
	if line, err := reader.ReadString('\n'); err != nil || !strings.Contains(line, "data:") {
		t.Fatalf("first SSE line = %q, err=%v", line, err)
	}

	waitForDirectActiveCount(t, limiter, 55, 1)
	cancel()
	resp.Body.Close()
	waitForDirectActiveCount(t, limiter, 55, 0)
}

// Site saturation keeps its historical max_concurrency diagnostic field.
func TestSiteSaturationLogKeepsMaxConcurrency(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 0)

	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{
		{
			Channel:     store.RouteChannel{ID: 101, Enabled: true},
			Account:     store.Account{ID: 7, Status: "active"},
			Site:        store.Site{ID: 5, URL: "http://127.0.0.1:9", Status: "active", MaxConcurrency: 1},
			TokenValue:  "site-token",
			ActualModel: "gpt-4o-upstream",
		},
	}}
	limiter := proxy.NewSiteConcurrencyLimiter()
	held, ok := limiter.TryAcquire(5, 1)
	if !ok {
		t.Fatal("pre-hold site 5 failed")
	}
	t.Cleanup(held.Release)

	rec := dispatchDirectAccountTestRequest(t, &UpstreamConfig{
		Router:      router,
		SiteLimiter: limiter,
	}, 0)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	output := logs.String()
	if !strings.Contains(output, "reason=site_concurrency_saturated") || !strings.Contains(output, "max_concurrency=1") {
		t.Fatalf("site saturation log lost its diagnostic fields: %s", output)
	}
}
