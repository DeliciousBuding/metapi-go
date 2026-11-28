package admin

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

type channelTestTarget struct {
	OAuthRouteUnitID *int64
	ChannelID        int64
	RouteID          int64
	SourceModel      string
	TokenValue       string
	Account          store.Account
	Site             store.Site
}

type harnessAccountRow struct {
	ID            int64   `db:"id"`
	SiteID        int64   `db:"site_id"`
	AccessToken   string  `db:"access_token"`
	APIToken      *string `db:"api_token"`
	OAuthProvider *string `db:"oauth_provider"`
	ExtraConfig   *string `db:"extra_config"`
	Status        string  `db:"status"`
}

type harnessSiteRow struct {
	ID                                  int64   `db:"id"`
	Name                                string  `db:"name"`
	URL                                 string  `db:"url"`
	Platform                            string  `db:"platform"`
	ProxyURL                            *string `db:"proxy_url"`
	UseSystemProxy                      bool    `db:"use_system_proxy"`
	CustomHeaders                       *string `db:"custom_headers"`
	CustomHeadersOverrideRequestHeaders bool    `db:"custom_headers_override_request_headers"`
	Status                              string  `db:"status"`
}

type harnessChannelRow struct {
	OAuthRouteUnitID *int64  `db:"oauth_route_unit_id"`
	ID               int64   `db:"id"`
	RouteID          int64   `db:"route_id"`
	AccountID        int64   `db:"account_id"`
	TokenID          *int64  `db:"token_id"`
	SourceModel      *string `db:"source_model"`
	Enabled          bool    `db:"enabled"`
}

type harnessTokenRow struct {
	ID      int64  `db:"id"`
	Token   string `db:"token"`
	Enabled bool   `db:"enabled"`
}

func (h *channelTestHandler) resolveTarget(ctx context.Context, channelID, siteID *int64, modelHint string) (*channelTestTarget, error) {
	if channelID != nil && *channelID > 0 {
		return h.loadByChannelID(ctx, *channelID)
	}
	return h.loadBySiteID(ctx, *siteID, modelHint)
}

func (h *channelTestHandler) loadByChannelID(ctx context.Context, channelID int64) (*channelTestTarget, error) {
	var ch harnessChannelRow
	err := h.db.GetContext(ctx, &ch, `
		SELECT id, route_id, account_id, token_id, oauth_route_unit_id, source_model, enabled
		FROM route_channels WHERE id = ?`, channelID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("channel not found")
	}
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
	}
	return h.assembleTarget(ctx, ch)
}

func (h *channelTestHandler) loadBySiteID(ctx context.Context, siteID int64, modelHint string) (*channelTestTarget, error) {
	// Prefer enabled channel whose source_model matches modelHint, else any enabled for the site.
	var ch harnessChannelRow
	modelHint = strings.TrimSpace(modelHint)
	var err error
	if modelHint != "" {
		err = h.db.GetContext(ctx, &ch, `
			SELECT rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id, rc.source_model, rc.enabled
			FROM route_channels rc
			JOIN accounts a ON a.id = rc.account_id
			WHERE a.site_id = ? AND rc.enabled = ?
			  AND LOWER(COALESCE(rc.source_model, '')) = LOWER(?)
			ORDER BY rc.priority DESC, rc.id ASC
			LIMIT 1`, siteID, true, modelHint)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("load channel by site/model: %w", err)
		}
	}
	if ch.ID == 0 {
		err = h.db.GetContext(ctx, &ch, `
			SELECT rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id, rc.source_model, rc.enabled
			FROM route_channels rc
			JOIN accounts a ON a.id = rc.account_id
			WHERE a.site_id = ? AND rc.enabled = ?
			ORDER BY rc.priority DESC, rc.id ASC
			LIMIT 1`, siteID, true)
		if err == sql.ErrNoRows {
			// Fall back to any channel on site even if disabled.
			err = h.db.GetContext(ctx, &ch, `
				SELECT rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id, rc.source_model, rc.enabled
				FROM route_channels rc
				JOIN accounts a ON a.id = rc.account_id
				WHERE a.site_id = ?
				ORDER BY rc.priority DESC, rc.id ASC
				LIMIT 1`, siteID)
		}
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no channel found for site")
		}
		if err != nil {
			return nil, fmt.Errorf("load channel by site: %w", err)
		}
	}
	return h.assembleTarget(ctx, ch)
}

func (h *channelTestHandler) assembleTarget(ctx context.Context, ch harnessChannelRow) (*channelTestTarget, error) {
	if ch.OAuthRouteUnitID != nil {
		return h.loadRouteUnitTarget(ctx, ch)
	}
	var acc harnessAccountRow
	if err := h.db.GetContext(ctx, &acc, `
		SELECT id, site_id, access_token, api_token, oauth_provider, extra_config, status
		FROM accounts WHERE id = ?`, ch.AccountID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("account not found")
		}
		return nil, fmt.Errorf("load account: %w", err)
	}

	var site harnessSiteRow
	if err := h.db.GetContext(ctx, &site, `
		SELECT id, name, url, platform, proxy_url, use_system_proxy, custom_headers, custom_headers_override_request_headers, status
		FROM sites WHERE id = ?`, acc.SiteID); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("site not found")
		}
		return nil, fmt.Errorf("load site: %w", err)
	}

	var token *harnessTokenRow
	if ch.TokenID != nil && *ch.TokenID > 0 {
		var tr harnessTokenRow
		if err := h.db.GetContext(ctx, &tr, `
			SELECT id, token, enabled FROM account_tokens WHERE id = ?`, *ch.TokenID); err == nil {
			token = &tr
		}
	}

	sourceModel := ""
	if ch.SourceModel != nil {
		sourceModel = strings.TrimSpace(*ch.SourceModel)
	}

	account := store.Account{
		ID:            acc.ID,
		SiteID:        acc.SiteID,
		AccessToken:   acc.AccessToken,
		APIToken:      acc.APIToken,
		OAuthProvider: acc.OAuthProvider,
		ExtraConfig:   acc.ExtraConfig,
		Status:        acc.Status,
	}
	siteModel := store.Site{
		ID:             site.ID,
		Name:           site.Name,
		URL:            site.URL,
		Platform:       site.Platform,
		ProxyURL:       site.ProxyURL,
		UseSystemProxy: site.UseSystemProxy,
		CustomHeaders:  site.CustomHeaders,
		Status:         site.Status,
	}

	return &channelTestTarget{
		ChannelID:   ch.ID,
		RouteID:     ch.RouteID,
		SourceModel: sourceModel,
		TokenValue:  resolveHarnessToken(ch.TokenID, token, acc),
		Account:     account,
		Site:        siteModel,
	}, nil
}

// A forced probe observes the same eligible member as routing without updating
// last_selected_at, cooldowns or the outer channel's representative account.
func (h *channelTestHandler) loadRouteUnitTarget(ctx context.Context, ch harnessChannelRow) (*channelTestTarget, error) {
	id := *ch.OAuthRouteUnitID
	target := &channelTestTarget{ChannelID: ch.ID, RouteID: ch.RouteID, OAuthRouteUnitID: ch.OAuthRouteUnitID}
	if ch.SourceModel != nil {
		target.SourceModel = strings.TrimSpace(*ch.SourceModel)
	}
	routes := service.NewProxyRoutingStore(h.db)
	ids := []int64{id}
	units, err := routes.LoadOAuthRouteUnitSummaries(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load OAuth route unit: %w", err)
	}
	unit := units[id]
	if !unit.Enabled {
		return target, nil
	}
	target.Site.ID = unit.SiteID
	members, err := routes.LoadOAuthRouteUnitMembers(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("load OAuth route unit members: %w", err)
	}
	now := time.Now().UTC()
	candidate := routing.RouteChannelCandidate{RouteUnit: &unit, RouteUnitMembers: members[id]}
	maxCooldown := config.TokenRouterFailureCooldownMaxSecCeiling
	if rt := config.RuntimeSafe(); rt != nil {
		maxCooldown = rt.TokenRouterFailureCooldownMaxSec
	}
	member := routing.SelectRouteUnitMember(candidate, target.SourceModel, now.Format(time.RFC3339), now.UnixMilli(), maxCooldown, nil)
	if member != nil {
		target.Account, target.Site = member.Account, member.Site
		target.TokenValue = routing.ResolveRouteUnitMemberTokenValue(member.Account)
	}
	return target, nil
}

// resolveHarnessToken mirrors routing.ChannelSelector.resolveChannelTokenValue
// without importing selector internals. Harness additionally falls back to
// access_token for session-style accounts so operators can still probe.
func resolveHarnessToken(tokenID *int64, token *harnessTokenRow, acc harnessAccountRow) string {
	if tokenID != nil && *tokenID > 0 {
		if token == nil || token.Token == "" || !token.Enabled {
			return ""
		}
		return token.Token
	}
	if acc.OAuthProvider != nil && strings.TrimSpace(*acc.OAuthProvider) != "" {
		return strings.TrimSpace(acc.AccessToken)
	}
	if acc.APIToken != nil && strings.TrimSpace(*acc.APIToken) != "" {
		return strings.TrimSpace(*acc.APIToken)
	}
	return strings.TrimSpace(acc.AccessToken)
}
