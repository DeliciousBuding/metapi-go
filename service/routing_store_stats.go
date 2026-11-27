package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
)

func (s *ProxyRoutingStore) UpdateChannelLastSelectedAt(ctx context.Context, channelID int64, lastSelectedAt string) error {
	if channelID < 0 {
		_, err := s.execContext(ctx, `UPDATE upstream_group_items SET last_selected_at=? WHERE id=?`, lastSelectedAt, -channelID)
		return err
	}
	_, err := s.execContext(ctx, `UPDATE route_channels SET last_selected_at = ? WHERE id = ?`, lastSelectedAt, channelID)
	return err
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberLastSelectedAt(ctx context.Context, unitID, accountID int64, lastSelectedAt string) error {
	_, err := s.execContext(ctx, `UPDATE oauth_route_unit_members SET last_selected_at = ?, updated_at = ? WHERE unit_id = ? AND account_id = ?`, lastSelectedAt, lastSelectedAt, unitID, accountID)
	return err
}

func (s *ProxyRoutingStore) UpdateChannelCooldownFields(ctx context.Context, channelIDs []int64, updates map[string]interface{}) error {
	if len(channelIDs) == 0 || len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeChannelUpdateColumns)
	if err != nil {
		return err
	}
	query, inArgs, err := sqlx.In(`UPDATE route_channels SET `+setClause+` WHERE id IN (?)`, append(args, channelIDs)...)
	if err != nil {
		return err
	}
	_, err = s.execContext(ctx, query, inArgs...)
	return err
}

func (s *ProxyRoutingStore) UpdateChannelSuccessFields(ctx context.Context, channelID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeChannelUpdateColumns)
	if err != nil {
		return err
	}
	args = append(args, channelID)
	_, err = s.execContext(ctx, `UPDATE route_channels SET `+setClause+` WHERE id = ?`, args...)
	return err
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberCooldownFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return s.updateRouteUnitMemberFields(ctx, memberID, updates)
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberSuccessFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return s.updateRouteUnitMemberFields(ctx, memberID, updates)
}

func (s *ProxyRoutingStore) updateRouteUnitMemberFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeUnitMemberUpdateColumns)
	if err != nil {
		return err
	}
	args = append(args, memberID)
	_, err = s.execContext(ctx, `UPDATE oauth_route_unit_members SET `+setClause+` WHERE id = ?`, args...)
	return err
}

func (s *ProxyRoutingStore) LoadRuntimeHealthChannelRows(ctx context.Context, channelIDs []int64) ([]struct {
	SiteID            int64
	SourceModel       *string
	RouteModelPattern string
}, error) {
	var result []struct {
		SiteID            int64
		SourceModel       *string
		RouteModelPattern string
	}
	if len(channelIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT a.site_id, rc.source_model, tr.model_pattern
		FROM route_channels rc
		JOIN accounts a ON a.id = rc.account_id
		JOIN token_routes tr ON tr.id = rc.route_id
		WHERE rc.id IN (?)`, channelIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row struct {
			SiteID            int64
			SourceModel       *string
			RouteModelPattern string
		}
		if err := rows.Scan(&row.SiteID, &row.SourceModel, &row.RouteModelPattern); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

var routeChannelUpdateColumns = map[string]string{
	"successCount":         "success_count",
	"failCount":            "fail_count",
	"totalLatencyMs":       "total_latency_ms",
	"totalCost":            "total_cost",
	"lastUsedAt":           "last_used_at",
	"lastFailAt":           "last_fail_at",
	"consecutiveFailCount": "consecutive_fail_count",
	"cooldownLevel":        "cooldown_level",
	"cooldownUntil":        "cooldown_until",
	"cooldownReasonCode":   "cooldown_reason_code",
	"cooldownReason":       "cooldown_reason",
	"cooldownReasonAt":     "cooldown_reason_at",
	// The failure-recording path may disable a channel outright when
	// its failing status falls in the operator-configured disable ranges.
	"enabled":        "enabled",
	"manualOverride": "manual_override",
}

var routeUnitMemberUpdateColumns = map[string]string{
	"successCount":         "success_count",
	"failCount":            "fail_count",
	"totalLatencyMs":       "total_latency_ms",
	"totalCost":            "total_cost",
	"lastUsedAt":           "last_used_at",
	"lastFailAt":           "last_fail_at",
	"lastSelectedAt":       "last_selected_at",
	"consecutiveFailCount": "consecutive_fail_count",
	"cooldownLevel":        "cooldown_level",
	"cooldownUntil":        "cooldown_until",
	"cooldownReasonCode":   "cooldown_reason_code",
	"cooldownReason":       "cooldown_reason",
	"cooldownReasonAt":     "cooldown_reason_at",
	"updatedAt":            "updated_at",
}

func buildAllowedUpdateSet(updates map[string]interface{}, allowed map[string]string) (string, []interface{}, error) {
	parts := make([]string, 0, len(updates))
	args := make([]interface{}, 0, len(updates))
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := updates[key]
		column, ok := allowed[key]
		if !ok {
			return "", nil, fmt.Errorf("unsupported routing update field %q", key)
		}
		parts = append(parts, column+" = ?")
		args = append(args, value)
	}
	return strings.Join(parts, ", "), args, nil
}
