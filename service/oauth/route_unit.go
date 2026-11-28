package oauth

import (
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// OAuthRouteUnitStrategy is the strategy for route unit member selection.
type OAuthRouteUnitStrategy string

const (
	RouteUnitStrategyRoundRobin            OAuthRouteUnitStrategy = "round_robin"
	RouteUnitStrategyStickUntilUnavailable OAuthRouteUnitStrategy = "stick_until_unavailable"
)

// OAuthRouteUnitSummary holds a summary of a route unit.
type OAuthRouteUnitSummary struct {
	ID          int64                  `json:"id"`
	SiteID      int64                  `json:"siteId"`
	Provider    string                 `json:"provider"`
	Name        string                 `json:"name"`
	Strategy    OAuthRouteUnitStrategy `json:"strategy"`
	Enabled     bool                   `json:"enabled"`
	MemberCount int64                  `json:"memberCount"`
}

// OAuthRouteUnitAccountParticipation represents route unit participation for an account.
type OAuthRouteUnitAccountParticipation struct {
	Kind        string                 `json:"kind"`
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Strategy    OAuthRouteUnitStrategy `json:"strategy"`
	MemberCount int64                  `json:"memberCount"`
}

// ListOauthRouteUnitsByAccountIDs returns route unit participation for a set of accounts.
func ListOauthRouteUnitsByAccountIDs(db *store.DB, accountIDs []int64) map[int64]*OAuthRouteUnitAccountParticipation {
	if len(accountIDs) == 0 {
		return nil
	}

	if db == nil {
		return nil
	}

	query, args, err := sqlx.In(
		`SELECT m.account_id, u.id as unit_id, u.name, u.strategy,
		        COUNT(*) OVER (PARTITION BY u.id) as member_count
		 FROM oauth_route_unit_members m
		 INNER JOIN oauth_route_units u ON m.unit_id = u.id
		 WHERE m.account_id IN (?)`, accountIDs)
	if err != nil {
		return nil
	}
	query = db.Rebind(query)

	type row struct {
		AccountID   int64  `db:"account_id"`
		UnitID      int64  `db:"unit_id"`
		Name        string `db:"name"`
		Strategy    string `db:"strategy"`
		MemberCount int64  `db:"member_count"`
	}

	var rows []row
	if err := db.Select(&rows, query, args...); err != nil {
		return nil
	}

	result := make(map[int64]*OAuthRouteUnitAccountParticipation)
	for _, r := range rows {
		strategy := normalizeStrategy(r.Strategy)
		result[r.AccountID] = &OAuthRouteUnitAccountParticipation{
			Kind:        "route_unit",
			ID:          r.UnitID,
			Name:        r.Name,
			Strategy:    strategy,
			MemberCount: max(r.MemberCount, 1),
		}
	}
	return result
}

// ListEnabledOauthRouteUnitsWithMembers returns all enabled route units with their members.
func ListEnabledOauthRouteUnitsWithMembers(db *store.DB) []struct {
	Unit    store.OAuthRouteUnit
	Members []store.OAuthRouteUnitMember
} {
	if db == nil {
		return nil
	}

	var units []store.OAuthRouteUnit
	if err := db.Select(&units, "SELECT * FROM oauth_route_units WHERE enabled = ?", true); err != nil {
		return nil
	}
	if len(units) == 0 {
		return nil
	}

	unitIDs := make([]int64, len(units))
	for i, u := range units {
		unitIDs[i] = u.ID
	}

	query, args, _ := sqlx.In(
		"SELECT * FROM oauth_route_unit_members WHERE unit_id IN (?) ORDER BY sort_order, id", unitIDs)
	query = db.Rebind(query)

	var allMembers []store.OAuthRouteUnitMember
	if err := db.Select(&allMembers, query, args...); err != nil {
		return nil
	}

	membersByUnit := make(map[int64][]store.OAuthRouteUnitMember)
	for _, m := range allMembers {
		membersByUnit[m.UnitID] = append(membersByUnit[m.UnitID], m)
	}

	result := make([]struct {
		Unit    store.OAuthRouteUnit
		Members []store.OAuthRouteUnitMember
	}, len(units))
	for i, u := range units {
		result[i].Unit = u
		result[i].Members = membersByUnit[u.ID]
	}
	return result
}

// UpdateOauthRouteUnit updates a route unit's name and/or strategy.
func UpdateOauthRouteUnit(db *store.DB, routeUnitID int64, name *string, strategy *OAuthRouteUnitStrategy) (*OAuthRouteUnitSummary, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var unit store.OAuthRouteUnit
	if err := db.Get(&unit, "SELECT * FROM oauth_route_units WHERE id = ?", routeUnitID); err != nil {
		return nil, fmt.Errorf("oauth route unit not found")
	}

	now := time.Now().Format(time.RFC3339)
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if trimmed == "" {
			return nil, fmt.Errorf("oauth route unit name is required")
		}
		unit.Name = trimmed
	}
	if strategy != nil {
		ns := normalizeStrategy(string(*strategy))
		if ns != RouteUnitStrategyRoundRobin && ns != RouteUnitStrategyStickUntilUnavailable {
			return nil, fmt.Errorf("invalid oauth route unit strategy")
		}
		unit.Strategy = string(ns)
	}

	_, err := db.Exec(
		"UPDATE oauth_route_units SET name = ?, strategy = ?, updated_at = ? WHERE id = ?",
		unit.Name, unit.Strategy, now, routeUnitID)
	if err != nil {
		return nil, err
	}

	var memberCount int64
	db.Get(&memberCount, "SELECT COUNT(*) FROM oauth_route_unit_members WHERE unit_id = ?", routeUnitID)

	// Step 3 (spec): Invalidate token router cache.
	hooks := getWorkflowHooks()
	if hooks != nil {
		hooks.InvalidateTokenRouterCache()
	}

	return &OAuthRouteUnitSummary{
		ID:          unit.ID,
		SiteID:      unit.SiteID,
		Provider:    unit.Provider,
		Name:        unit.Name,
		Strategy:    normalizeStrategy(unit.Strategy),
		Enabled:     unit.Enabled,
		MemberCount: memberCount,
	}, nil
}

func normalizeStrategy(s string) OAuthRouteUnitStrategy {
	normalized := strings.ToLower(strings.TrimSpace(s))
	switch normalized {
	case "round_robin":
		return RouteUnitStrategyRoundRobin
	case "stick_until_unavailable":
		return RouteUnitStrategyStickUntilUnavailable
	default:
		return RouteUnitStrategyRoundRobin
	}
}
