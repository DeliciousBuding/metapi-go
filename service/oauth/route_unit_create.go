package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// CreateRouteUnitInput holds input for creating a route unit.
type CreateRouteUnitInput struct {
	AccountIDs []int64
	Name       string
	Strategy   OAuthRouteUnitStrategy
}

// CreateRouteUnitResult holds the result of creating a route unit.
type CreateRouteUnitResult struct {
	Success   bool                   `json:"success"`
	RouteUnit *OAuthRouteUnitSummary `json:"routeUnit"`
}

// CreateOauthRouteUnit creates a new OAuth route unit.
func CreateOauthRouteUnit(db *store.DB, input CreateRouteUnitInput) (*CreateRouteUnitResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	accountIDs := uniquePositiveIDs(input.AccountIDs)
	if len(accountIDs) < 2 {
		return nil, fmt.Errorf("oauth route unit requires at least 2 accounts")
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("oauth route unit name is required")
	}

	strategy := normalizeStrategy(string(input.Strategy))
	if strategy != RouteUnitStrategyRoundRobin && strategy != RouteUnitStrategyStickUntilUnavailable {
		return nil, fmt.Errorf("invalid oauth route unit strategy")
	}

	// Load accounts with site join.
	query, args, _ := sqlx.In(
		`SELECT a.id, a.site_id, a.oauth_provider FROM accounts a
		 INNER JOIN sites s ON a.site_id = s.id
		 WHERE a.id IN (?)`, accountIDs)
	query = db.Rebind(query)

	type accountRouteUnitRow struct {
		ID            int64   `db:"id"`
		SiteID        int64   `db:"site_id"`
		OAuthProvider *string `db:"oauth_provider"`
	}
	var rows []accountRouteUnitRow
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("oauth route unit accounts not found")
	}
	if len(rows) != len(accountIDs) {
		return nil, fmt.Errorf("oauth route unit accounts not found")
	}

	first := rows[0]
	expectedSiteID := first.SiteID
	expectedProvider := ""
	if first.OAuthProvider != nil {
		expectedProvider = strings.TrimSpace(*first.OAuthProvider)
	}
	if expectedProvider == "" {
		return nil, fmt.Errorf("oauth route unit only supports oauth accounts")
	}

	for _, row := range rows {
		if row.SiteID != expectedSiteID {
			return nil, fmt.Errorf("oauth route unit accounts must belong to the same site")
		}
		p := ""
		if row.OAuthProvider != nil {
			p = strings.TrimSpace(*row.OAuthProvider)
		}
		if p != expectedProvider {
			return nil, fmt.Errorf("oauth route unit accounts must share the same provider")
		}
	}

	// Check uniqueness — accounts already in route units.
	checkQuery, checkArgs, _ := sqlx.In(
		"SELECT account_id FROM oauth_route_unit_members WHERE account_id IN (?)", accountIDs)
	checkQuery = db.Rebind(checkQuery)

	var existingMembers []struct {
		AccountID int64 `db:"account_id"`
	}
	if err := db.Select(&existingMembers, checkQuery, checkArgs...); err == nil && len(existingMembers) > 0 {
		return nil, fmt.Errorf("oauth route unit accounts already grouped")
	}

	// Create in transaction.
	tx, err := db.Beginx()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().Format(time.RFC3339)
	unitID, err := execRouteUnitInsertID(db, tx,
		`INSERT INTO oauth_route_units (site_id, provider, name, strategy, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		expectedSiteID, expectedProvider, name, string(strategy), true, now, now)
	if err != nil {
		return nil, fmt.Errorf("oauth route unit creation failed: %w", err)
	}

	for i, accountID := range accountIDs {
		_, err := tx.Exec(tx.Rebind(
			`INSERT INTO oauth_route_unit_members (unit_id, account_id, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?)`),
			unitID, accountID, i, now, now)
		if err != nil {
			return nil, fmt.Errorf("oauth route unit accounts already grouped")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Step 5: Rebuild routes.
	hooks := getWorkflowHooks()
	if hooks != nil {
		if err := hooks.RebuildRoutesOnly(context.Background()); err != nil {
			// Step 6: On rebuild failure, rollback created route unit.
			slog.Warn("route rebuild failed after creating route unit, rolling back", "unitID", unitID, "error", err)
			rollbackErr := rollbackCreateRouteUnit(db, unitID, accountIDs)
			if rollbackErr != nil {
				slog.Warn("rollback route unit creation failed", "error", rollbackErr)
			}
			// Best-effort retry rebuild after rollback.
			_ = hooks.RebuildRoutesOnly(context.Background())
			return nil, fmt.Errorf("route rebuild failed: %w", err)
		}
		// Invalidate token router cache.
		hooks.InvalidateTokenRouterCache()
	}

	return &CreateRouteUnitResult{
		Success: true,
		RouteUnit: &OAuthRouteUnitSummary{
			ID:          unitID,
			SiteID:      expectedSiteID,
			Provider:    expectedProvider,
			Name:        name,
			Strategy:    strategy,
			Enabled:     true,
			MemberCount: int64(len(accountIDs)),
		},
	}, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	seen := make(map[int64]bool)
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result
}

func execRouteUnitInsertID(db *store.DB, tx *sqlx.Tx, query string, args ...any) (int64, error) {
	if db.Dialect == store.DialectPostgres {
		var id int64
		if err := tx.QueryRowx(tx.Rebind(query+" RETURNING id"), args...).Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	}

	result, err := tx.Exec(tx.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// rollbackCreateRouteUnit deletes a route unit and its members when route rebuild fails.
func rollbackCreateRouteUnit(db *store.DB, unitID int64, accountIDs []int64) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}
	_, err := db.Exec("DELETE FROM oauth_route_unit_members WHERE unit_id = ?", unitID)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM oauth_route_units WHERE id = ?", unitID)
	return err
}
