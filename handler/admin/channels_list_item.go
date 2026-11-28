package admin

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

// channelListRow is the flattened read-only projection for GET /api/channels.
// It aggregates route_channels across the account/token/oauth_unit dimensions
// and joins only the display fields the channels page needs (no credentials).
type channelListRow struct {
	ID               int64   `db:"id"`
	RouteID          int64   `db:"route_id"`
	AccountID        int64   `db:"account_id"`
	TokenID          *int64  `db:"token_id"`
	OAuthRouteUnitID *int64  `db:"oauth_route_unit_id"`
	SourceModel      *string `db:"source_model"`
	// Priority/Weight/SuccessCount/TotalLatencyMs are nullable in route_channels
	// (DEFAULT without NOT NULL): NULL must scan to nil, not crash the list
	// with "converting NULL to int64" (#933 family).
	Priority       *int64  `db:"priority"`
	Weight         *int64  `db:"weight"`
	Enabled        bool    `db:"enabled"`
	ManualOverride bool    `db:"manual_override"`
	SuccessCount   *int64  `db:"success_count"`
	TotalLatencyMs *int64  `db:"total_latency_ms"`
	CooldownUntil  *string `db:"cooldown_until"`
	// Structured cooldown reason: why the channel cooled down. NULL on
	// rows cooled before the reason columns existed.
	CooldownReasonCode *string `db:"cooldown_reason_code"`
	CooldownReason     *string `db:"cooldown_reason"`
	CooldownReasonAt   *string `db:"cooldown_reason_at"`
	Username           string  `db:"username"`
	SiteID             int64   `db:"site_id"`
	SiteName           string  `db:"site_name"`
	SiteStatus         string  `db:"site_status"`
	AccountStatus      string  `db:"account_status"`
	RouteModelPattern  string  `db:"route_model_pattern"`
	OAuthUnitName      string  `db:"oauth_unit_name"`
	TokenName          string  `db:"token_name"`
	// TotalCount is the COUNT(*) OVER () window result: the total number of
	// route_channels rows matching the FROM/JOINs ignoring LIMIT/OFFSET, so the
	// pager can show the true fleet size on every page. Identical across all
	// rows in a page; read from the first row (or a fallback COUNT when empty).
	TotalCount int64 `db:"total_count"`
}

var validChannelStatuses = map[string]struct{}{
	routing.ChannelStatusEnabled:          {},
	routing.ChannelStatusCooldown:         {},
	routing.ChannelStatusBreakerOpen:      {},
	routing.ChannelStatusManuallyDisabled: {},
}

func parseChannelStatusFilter(raw string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	statuses := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("invalid channel status filter")
		}
		if _, ok := validChannelStatuses[part]; !ok {
			return nil, fmt.Errorf("invalid channel status filter: %q", part)
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		statuses = append(statuses, part)
	}
	return statuses, nil
}

func channelStatusForRow(row channelListRow) string {
	sourceModel := ""
	if row.SourceModel != nil {
		sourceModel = *row.SourceModel
	}
	return routing.ChannelRuntimeStatus(row.SiteID, sourceModel, channelEffectivelyEnabled(row.Enabled, row.SiteStatus, row.AccountStatus), row.CooldownUntil)
}

// Keep configured enabled separate from inherited availability. Disabling a
// site must not rewrite shared routes or a channel's explicit configuration.
func channelEffectivelyEnabled(enabled bool, siteStatus, accountStatus string) bool {
	return enabled && siteStatus != "disabled" && accountStatus != "disabled"
}

func channelStatusMatches(row channelListRow, statusFilter []string) bool {
	status := channelStatusForRow(row)
	for _, wanted := range statusFilter {
		if status == wanted {
			return true
		}
	}
	return false
}

func channelListItem(row channelListRow) map[string]any {
	sourceModel := ""
	if row.SourceModel != nil {
		sourceModel = *row.SourceModel
	}
	models := sourceModel
	if models == "" {
		models = row.RouteModelPattern
	}

	name := ""
	chType := "account"
	switch {
	case row.OAuthRouteUnitID != nil && *row.OAuthRouteUnitID > 0:
		chType = "oauth_unit"
		name = row.OAuthUnitName
	case row.TokenID != nil && *row.TokenID > 0:
		chType = "token"
		name = row.TokenName
	default:
		name = row.Username
	}
	if name == "" {
		name = sourceModel
	}
	if name == "" {
		name = row.RouteModelPattern
	}

	var responseMs *int64
	if row.SuccessCount != nil && *row.SuccessCount > 0 && row.TotalLatencyMs != nil && *row.TotalLatencyMs > 0 {
		ms := *row.TotalLatencyMs / *row.SuccessCount
		responseMs = &ms
	}

	return map[string]any{
		"id":                 row.ID,
		"routeId":            row.RouteID,
		"name":               name,
		"site":               map[string]any{"id": row.SiteID, "name": row.SiteName, "status": row.SiteStatus},
		"type":               chType,
		"status":             channelStatusForRow(row),
		"models":             models,
		"priority":           row.Priority,
		"weight":             row.Weight,
		"responseMs":         responseMs,
		"cooldownUntil":      row.CooldownUntil,
		"cooldownReasonCode": row.CooldownReasonCode,
		"cooldownReason":     row.CooldownReason,
		"cooldownReasonAt":   row.CooldownReasonAt,
		"enabled":            row.Enabled,
		"manualOverride":     row.ManualOverride,
	}
}
