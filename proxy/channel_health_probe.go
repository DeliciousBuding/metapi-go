package proxy

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ChannelProbeTarget identifies one channel/model pair for a lightweight health check.
type ChannelProbeTarget struct {
	ChannelID int64
	AccountID int64
	SiteID    int64
	ModelName string
}

// ChannelProbeOutcome is the result of a single lightweight probe attempt.
// Status is one of: success | failure | inconclusive | skipped.
type ChannelProbeOutcome struct {
	AccountID  int64
	SiteID     int64
	Status     string
	LatencyMs  float64
	HTTPStatus int
	ErrorText  string
}

// ChannelHealthProbeExecutor is a thin boot-wired probe that hits live upstream
// for ModelProbeScheduler / admin probe-now paths.
//
// It prefers a lightweight GET /v1/models check. Platforms without a models
// catalog endpoint may return non-2xx; those outcomes are recorded as failure
// (not credential expiry).
type ChannelHealthProbeExecutor struct {
	cfg *config.Config

	// transport is an optional override for tests (fake upstream).
	// When nil, production uses platform.DoWithProxy / default client.
	transport http.RoundTripper

	db *store.DB
}

// NewChannelHealthProbeExecutor builds the production probe executor.
func NewChannelHealthProbeExecutor(db *store.DB, cfg *config.Config) *ChannelHealthProbeExecutor {
	return &ChannelHealthProbeExecutor{cfg: cfg, db: db}
}

// ProbeChannel runs a single lightweight upstream probe for target.
func (p *ChannelHealthProbeExecutor) ProbeChannel(ctx context.Context, target ChannelProbeTarget) (ChannelProbeOutcome, error) {
	if p == nil {
		return ChannelProbeOutcome{Status: "inconclusive", ErrorText: "probe executor nil"}, nil
	}
	if target.ChannelID <= 0 {
		return ChannelProbeOutcome{Status: "skipped", ErrorText: "invalid channel id"}, nil
	}

	dbw := p.db
	if dbw == nil {
		return ChannelProbeOutcome{}, fmt.Errorf("database not initialized")
	}

	loaded, err := loadProbeChannel(ctx, dbw, target)
	if err != nil {
		// Missing channel is inconclusive (no health mutation).
		return ChannelProbeOutcome{Status: "inconclusive", ErrorText: err.Error()}, nil
	}
	if loaded.TokenValue == "" {
		return ChannelProbeOutcome{
			Status:    "inconclusive",
			ErrorText: "no usable token on channel/account",
		}, nil
	}

	model := strings.TrimSpace(target.ModelName)
	if model == "" {
		model = loaded.SourceModel
	}

	// Prefer lightweight models listing. Fall back to a tiny chat completion
	// when the operator/target model is known and models endpoint is absent.
	out, err := p.executeModelsProbe(ctx, loaded, model)
	out.AccountID, out.SiteID = loaded.AccountID, loaded.SiteID
	return out, err
}

type probeChannelTarget struct {
	ChannelID   int64
	AccountID   int64
	SiteID      int64
	SourceModel string
	SiteURL     string
	TokenValue  string
	Account     store.Account
	Site        store.Site
}

func loadProbeChannel(ctx context.Context, dbw *store.DB, target ChannelProbeTarget) (*probeChannelTarget, error) {
	var ch struct {
		ID          int64   `db:"id"`
		AccountID   int64   `db:"account_id"`
		UnitID      *int64  `db:"oauth_route_unit_id"`
		TokenID     *int64  `db:"token_id"`
		SourceModel *string `db:"source_model"`
		Enabled     bool    `db:"enabled"`
	}
	err := dbw.GetContext(ctx, &ch, dbw.Rebind(`
		SELECT id, account_id, oauth_route_unit_id, token_id, source_model, enabled
		FROM route_channels WHERE id = ?`), target.ChannelID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
	}

	accountID := ch.AccountID
	if target.AccountID > 0 {
		accountID = target.AccountID
	}
	accountQuery := `
		SELECT id, site_id, username, access_token, api_token, balance, balance_used,
		       quota, unit_cost, value_score, status, is_pinned, sort_order,
		       checkin_enabled, last_checkin_at, last_balance_refresh, oauth_provider,
		       oauth_account_key, oauth_project_id, extra_config, created_at, updated_at
		FROM accounts WHERE id = ?`
	args := []any{accountID}
	if ch.UnitID != nil && *ch.UnitID > 0 {
		accountQuery += ` AND EXISTS (SELECT 1 FROM oauth_route_unit_members m WHERE m.unit_id=? AND m.account_id=accounts.id)`
		args = append(args, *ch.UnitID)
		ch.TokenID = nil
	} else if accountID != ch.AccountID {
		return nil, fmt.Errorf("probe account %d is not bound to channel %d", accountID, ch.ID)
	}
	var acc store.Account
	if err := dbw.GetContext(ctx, &acc, dbw.Rebind(accountQuery), args...); err != nil {
		return nil, fmt.Errorf("load probe account %d for channel %d: %w", accountID, ch.ID, err)
	}
	if target.SiteID > 0 && target.SiteID != acc.SiteID {
		return nil, fmt.Errorf("probe account %d is not bound to site %d", accountID, target.SiteID)
	}

	var site store.Site
	if err := dbw.GetContext(ctx, &site, dbw.Rebind(`
		SELECT id, name, url, external_checkin_url, platform, proxy_url, use_system_proxy,
		       custom_headers, custom_headers_override_request_headers, status, is_pinned, sort_order, global_weight, api_key,
		       max_concurrency,
		       post_refresh_probe_enabled, post_refresh_probe_model, post_refresh_probe_scope,
		       post_refresh_probe_latency_threshold_ms, created_at, updated_at
		FROM sites WHERE id = ?`), acc.SiteID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("site not found")
		}
		return nil, fmt.Errorf("load site: %w", err)
	}

	var token *store.AccountToken
	if ch.TokenID != nil && *ch.TokenID > 0 {
		var tr store.AccountToken
		if err := dbw.GetContext(ctx, &tr, dbw.Rebind(`
			SELECT id, account_id, name, token, token_group, value_status, source,
			       enabled, is_default, created_at, updated_at
			FROM account_tokens WHERE id = ? AND account_id = ?`), *ch.TokenID, accountID); err != nil {
			return nil, fmt.Errorf("load probe token: %w", err)
		}
		token = &tr
	}

	sourceModel := ""
	if ch.SourceModel != nil {
		sourceModel = strings.TrimSpace(*ch.SourceModel)
	}

	tokenValue := resolveProbeToken(ch.TokenID, token, acc)
	if ch.UnitID != nil && *ch.UnitID > 0 {
		tokenValue = routing.ResolveRouteUnitMemberTokenValue(acc)
	}
	return &probeChannelTarget{
		ChannelID:   ch.ID,
		AccountID:   acc.ID,
		SiteID:      site.ID,
		SourceModel: sourceModel,
		SiteURL:     site.URL,
		TokenValue:  tokenValue,
		Account:     acc,
		Site:        site,
	}, nil
}

// resolveProbeToken mirrors harness/routing token resolution without importing
// selector internals. Falls back to access_token for session-style accounts.
func resolveProbeToken(tokenID *int64, token *store.AccountToken, acc store.Account) string {
	if tokenID != nil && *tokenID > 0 {
		if token == nil || strings.TrimSpace(token.Token) == "" || !token.Enabled {
			return ""
		}
		return strings.TrimSpace(token.Token)
	}
	if acc.OAuthProvider != nil && strings.TrimSpace(*acc.OAuthProvider) != "" {
		return strings.TrimSpace(acc.AccessToken)
	}
	if acc.APIToken != nil && strings.TrimSpace(*acc.APIToken) != "" {
		return strings.TrimSpace(*acc.APIToken)
	}
	return strings.TrimSpace(acc.AccessToken)
}

func (p *ChannelHealthProbeExecutor) executeModelsProbe(ctx context.Context, target *probeChannelTarget, model string) (ChannelProbeOutcome, error) {
	upstreamPath := "/v1/models"
	upstreamURL := BuildUpstreamURL(target.SiteURL, upstreamPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, upstreamURL, nil)
	if err != nil {
		return ChannelProbeOutcome{Status: "inconclusive", ErrorText: "build request: " + err.Error()}, nil
	}
	req.Header.Set("Authorization", "Bearer "+target.TokenValue)
	req.Header.Set("User-Agent", "metapi-go-model-probe/1.0")

	proxyCfg := service.BuildPlatformProxyConfig(p.cfg, &target.Account, &target.Site)
	if proxyCfg != nil {
		// Deny-list + override priority (probe path mirrors upstream apply).
		platform.ApplyCustomHeadersWithOptions(req, proxyCfg.CustomHeaders, platform.ApplyCustomHeadersOptions{
			OverrideRequest: proxyCfg.CustomHeadersOverrideRequest,
		})
	}

	started := time.Now()
	resp, err := p.doRequest(ctx, req, proxyCfg)
	latencyMs := float64(time.Since(started).Milliseconds())
	if err != nil {
		// Transport / timeout: treat as failure so recovery can cool flaky channels.
		msg := err.Error()
		if strings.Contains(msg, "context deadline exceeded") {
			msg = "timeout waiting for upstream"
		}
		return ChannelProbeOutcome{
			Status:     "failure",
			LatencyMs:  latencyMs,
			ErrorText:  msg,
			HTTPStatus: 0,
		}, nil
	}
	defer resp.Body.Close()
	// Drain a small amount so keep-alive connections can be reused; discard body.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ChannelProbeOutcome{
			Status:     "success",
			LatencyMs:  latencyMs,
			HTTPStatus: resp.StatusCode,
		}, nil
	}

	// Non-2xx models listing: some gateways lack /v1/models. Retry once with a
	// tiny chat completion when we have a model name (better signal, still cheap).
	if model != "" && (resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed) {
		return p.executeChatProbe(ctx, target, model)
	}

	return ChannelProbeOutcome{
		Status:     "failure",
		LatencyMs:  latencyMs,
		HTTPStatus: resp.StatusCode,
		ErrorText:  fmt.Sprintf("upstream status %d", resp.StatusCode),
	}, nil
}

func (p *ChannelHealthProbeExecutor) executeChatProbe(ctx context.Context, target *probeChannelTarget, model string) (ChannelProbeOutcome, error) {
	upstreamPath := "/v1/chat/completions"
	upstreamURL := BuildUpstreamURL(target.SiteURL, upstreamPath)
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
		"max_tokens": 1,
		"stream":     false,
	}
	bodyBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return ChannelProbeOutcome{Status: "inconclusive", ErrorText: "build chat request: " + err.Error()}, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+target.TokenValue)
	req.Header.Set("User-Agent", "metapi-go-model-probe/1.0")

	proxyCfg := service.BuildPlatformProxyConfig(p.cfg, &target.Account, &target.Site)
	if proxyCfg != nil {
		// Deny-list + override priority (probe path mirrors upstream apply).
		platform.ApplyCustomHeadersWithOptions(req, proxyCfg.CustomHeaders, platform.ApplyCustomHeadersOptions{
			OverrideRequest: proxyCfg.CustomHeadersOverrideRequest,
		})
	}

	started := time.Now()
	resp, err := p.doRequest(ctx, req, proxyCfg)
	latencyMs := float64(time.Since(started).Milliseconds())
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "context deadline exceeded") {
			msg = "timeout waiting for upstream"
		}
		return ChannelProbeOutcome{
			Status:     "failure",
			LatencyMs:  latencyMs,
			ErrorText:  msg,
			HTTPStatus: 0,
		}, nil
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8<<10))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return ChannelProbeOutcome{
			Status:     "success",
			LatencyMs:  latencyMs,
			HTTPStatus: resp.StatusCode,
		}, nil
	}
	return ChannelProbeOutcome{
		Status:     "failure",
		LatencyMs:  latencyMs,
		HTTPStatus: resp.StatusCode,
		ErrorText:  fmt.Sprintf("upstream status %d", resp.StatusCode),
	}, nil
}

// probeFallbackTransport bounds the dial/TLS/idle phases for probe requests
// that run without an injected transport or site proxy config. The deadline
// is owned by the caller context (probe timeout is operator-tunable with no
// fixed ceiling), so the response-header phase stays unbounded here.
// probeFallbackTransport dials operator-configured site URLs, so it carries the
// same data-plane SSRF dial guard as the proxy executor transports.
var probeFallbackTransport = httpclient.NewTransport(httpclient.Options{SiteDialGuard: true})

func (p *ChannelHealthProbeExecutor) doRequest(ctx context.Context, req *http.Request, proxyCfg *platform.ProxyConfig) (*http.Response, error) {
	if p.transport != nil {
		return p.transport.RoundTrip(req.WithContext(ctx))
	}
	if proxyCfg != nil && (proxyCfg.ProxyURL != "" || proxyCfg.InsecureSkipTLS) {
		return platform.DoWithProxy(ctx, req, proxyCfg)
	}
	// Context owns the deadline; refuse cross-origin redirects so a public
	// site URL cannot 302 into metadata/loopback (shared platform policy).
	client := &http.Client{
		Timeout:       0,
		Transport:     probeFallbackTransport,
		CheckRedirect: platform.RejectCrossOriginRedirect,
	}
	return client.Do(req.WithContext(ctx))
}
