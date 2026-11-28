package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// DeleteOauthRouteUnit deletes a route unit with snapshot + rebuild + rollback.
func DeleteOauthRouteUnit(db *store.DB, routeUnitID int64) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	var unit store.OAuthRouteUnit
	if err := db.Get(&unit, "SELECT * FROM oauth_route_units WHERE id = ?", routeUnitID); err != nil {
		return fmt.Errorf("oauth route unit not found")
	}

	// Step 2: Snapshot current members + channels for rollback.
	var snapshotMembers []store.OAuthRouteUnitMember
	if err := db.Select(&snapshotMembers, "SELECT * FROM oauth_route_unit_members WHERE unit_id = ?", routeUnitID); err != nil {
		return fmt.Errorf("snapshot oauth route unit members: %w", err)
	}

	var snapshotChannels []store.RouteChannel
	if err := db.Select(&snapshotChannels, "SELECT * FROM route_channels WHERE oauth_route_unit_id = ?", routeUnitID); err != nil {
		return fmt.Errorf("snapshot oauth route unit channels: %w", err)
	}

	// Step 3: Delete channels -> members -> unit.
	_, err := db.Exec("DELETE FROM route_channels WHERE oauth_route_unit_id = ?", routeUnitID)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM oauth_route_unit_members WHERE unit_id = ?", routeUnitID)
	if err != nil {
		// Rollback: re-insert channels.
		for _, ch := range snapshotChannels {
			rollbackInsertChannel(db, &ch)
		}
		return err
	}
	_, err = db.Exec("DELETE FROM oauth_route_units WHERE id = ?", routeUnitID)
	if err != nil {
		// Rollback: re-insert members + channels.
		for _, m := range snapshotMembers {
			rollbackInsertMember(db, &m)
		}
		for _, ch := range snapshotChannels {
			rollbackInsertChannel(db, &ch)
		}
		return err
	}

	// Step 4: Rebuild routes.
	hooks := getWorkflowHooks()
	if hooks != nil {
		if err := hooks.RebuildRoutesOnly(context.Background()); err != nil {
			// Step 5: On rebuild failure: full rollback (re-insert unit + members + channels).
			slog.Warn("route rebuild failed after deleting route unit, rolling back", "routeUnitID", routeUnitID, "error", err)
			rbErr := rollbackDeleteRouteUnit(db, &unit, snapshotMembers, snapshotChannels)
			if rbErr != nil {
				slog.Warn("rollback route unit deletion failed", "error", rbErr)
			}
			// Best-effort retry rebuild after rollback.
			_ = hooks.RebuildRoutesOnly(context.Background())
			hooks.InvalidateTokenRouterCache()
			return fmt.Errorf("route rebuild failed: %w", err)
		}
		hooks.InvalidateTokenRouterCache()
	}

	return nil
}

// rollbackDeleteRouteUnit re-inserts a route unit, its members, and channels on rebuild failure.
func rollbackDeleteRouteUnit(db *store.DB, unit *store.OAuthRouteUnit, members []store.OAuthRouteUnitMember, channels []store.RouteChannel) error {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO oauth_route_units (id, site_id, provider, name, strategy, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		unit.ID, unit.SiteID, unit.Provider, unit.Name, unit.Strategy, unit.Enabled, unit.CreatedAt, now)
	if err != nil {
		return err
	}
	for _, m := range members {
		if err := rollbackInsertMember(db, &m); err != nil {
			return err
		}
	}
	for _, ch := range channels {
		if err := rollbackInsertChannel(db, &ch); err != nil {
			return err
		}
	}
	return nil
}

func rollbackInsertMember(db *store.DB, m *store.OAuthRouteUnitMember) error {
	now := time.Now().Format(time.RFC3339)
	createdAt := now
	updatedAt := now
	if m.CreatedAt != "" {
		createdAt = m.CreatedAt
	}
	if m.UpdatedAt != "" {
		updatedAt = m.UpdatedAt
	}
	_, err := db.Exec(
		`INSERT INTO oauth_route_unit_members (unit_id, account_id, sort_order, success_count, fail_count,
		 total_latency_ms, total_cost, last_used_at, last_selected_at, last_fail_at,
		 consecutive_fail_count, cooldown_level, cooldown_until,
		 cooldown_reason_code, cooldown_reason, cooldown_reason_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.UnitID, m.AccountID, m.SortOrder, m.SuccessCount, m.FailCount,
		m.TotalLatencyMs, m.TotalCost, m.LastUsedAt, m.LastSelectedAt, m.LastFailAt,
		m.ConsecutiveFailCount, m.CooldownLevel, m.CooldownUntil,
		m.CooldownReasonCode, m.CooldownReason, m.CooldownReasonAt,
		createdAt, updatedAt)
	return err
}

func rollbackInsertChannel(db *store.DB, ch *store.RouteChannel) error {
	_, err := db.Exec(
		`INSERT INTO route_channels (id, route_id, account_id, token_id, oauth_route_unit_id,
		 source_model, priority, weight, enabled, manual_override,
		 success_count, fail_count, total_latency_ms, total_cost,
		 last_used_at, last_selected_at, last_fail_at,
		 consecutive_fail_count, cooldown_level, cooldown_until,
		 cooldown_reason_code, cooldown_reason, cooldown_reason_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ch.ID, ch.RouteID, ch.AccountID, ch.TokenID, ch.OAuthRouteUnitID,
		ch.SourceModel, ch.Priority, ch.Weight, ch.Enabled, ch.ManualOverride,
		ch.SuccessCount, ch.FailCount, ch.TotalLatencyMs, ch.TotalCost,
		ch.LastUsedAt, ch.LastSelectedAt, ch.LastFailAt,
		ch.ConsecutiveFailCount, ch.CooldownLevel, ch.CooldownUntil,
		ch.CooldownReasonCode, ch.CooldownReason, ch.CooldownReasonAt)
	return err
}
