package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

// createUncoveredModelRoutes runs under routeRebuildMu. It materializes missing
// literal model routes from the two availability sources, never from another
// route's cached channels. No schema ownership flag is needed: route definitions
// are retained on delisting, while rebuild already owns their automatic channels.
func createUncoveredModelRoutes(ctx context.Context, db *sqlx.DB) (created, skipped int, err error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("begin model route creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var patterns []string
	// Disabled routes count too: automatic creation must not bypass a manual
	// disable or take precedence over an operator's wildcard / group policy.
	if err := tx.SelectContext(ctx, &patterns, "SELECT model_pattern FROM token_routes"); err != nil {
		return 0, 0, fmt.Errorf("load existing route patterns: %w", err)
	}
	var models []string
	if err := tx.SelectContext(ctx, &models, tx.Rebind(`
        SELECT tma.model_name
        FROM token_model_availability tma
        JOIN account_tokens tok ON tok.id = tma.token_id
        JOIN accounts a ON a.id = tok.account_id
        JOIN sites s ON s.id = a.site_id
        WHERE tma.available = ? AND tok.enabled = ? AND a.status = ? AND s.status = ?
          AND COALESCE(tok.value_status, 'ready') = 'ready' AND TRIM(COALESCE(tok.token, '')) <> ''
        UNION
        SELECT ma.model_name
        FROM model_availability ma
        JOIN accounts a ON a.id = ma.account_id
        JOIN sites s ON s.id = a.site_id
        WHERE ma.available = ? AND a.status = ? AND s.status = ?
          AND (TRIM(COALESCE(a.api_token, '')) <> '' OR
               (TRIM(COALESCE(a.oauth_provider, '')) <> '' AND TRIM(COALESCE(a.access_token, '')) <> ''))
        ORDER BY model_name`), true, true, "active", "active", true, "active", "active"); err != nil {
		return 0, 0, fmt.Errorf("load models for automatic routes: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, rawModel := range models {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		model := strings.TrimSpace(rawModel)
		// An upstream name containing glob/regex syntax cannot be turned into
		// a literal route by the existing matcher; never silently widen it.
		if !routing.IsExactRouteModelPattern(model) {
			skipped++
			continue
		}
		covered := false
		for _, pattern := range patterns {
			if routing.MatchesModelPattern(model, pattern) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		result, err := tx.ExecContext(ctx, tx.Rebind(`
            INSERT INTO token_routes (model_pattern, display_name, route_mode, routing_strategy, enabled, created_at, updated_at)
            SELECT ?, ?, 'pattern', 'weighted', ?, ?, ?
            WHERE NOT EXISTS (SELECT 1 FROM token_routes WHERE model_pattern = ?)`), model, model, true, now, now, model)
		if err != nil {
			return 0, 0, fmt.Errorf("create model route: %w", err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, 0, fmt.Errorf("count created model routes: %w", err)
		}
		created += int(count)
		patterns = append(patterns, model)
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit model routes: %w", err)
	}
	return created, skipped, nil
}
