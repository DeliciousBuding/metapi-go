package service

import (
	"context"
	"fmt"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

type channelIdentity struct {
	AccountID   int64
	TokenID     int64 // 0 means NULL
	SourceModel string
}

type desiredChannel struct {
	AccountID   int64
	TokenID     *int64
	SourceModel *string
	Priority    int64
	Weight      int64
	Enabled     bool
}

func channelKey(accountID int64, tokenID *int64, sourceModel *string) channelIdentity {
	var tid int64
	if tokenID != nil {
		tid = *tokenID
	}
	sm := ""
	if sourceModel != nil {
		sm = *sourceModel
	}
	return channelIdentity{AccountID: accountID, TokenID: tid, SourceModel: sm}
}

func collectDesiredChannels(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string) (map[channelIdentity]desiredChannel, error) {
	desired := make(map[channelIdentity]desiredChannel)
	pattern := modelPattern

	// Source 1: manual attachments on matching exact routes. Automatic channels
	// are outputs of availability, not another source: copying them between
	// routes would keep delisted models alive indefinitely.
	type exactRoute struct {
		ID           int64  `db:"id"`
		ModelPattern string `db:"model_pattern"`
	}
	var exactRoutes []exactRoute
	if err := db.SelectContext(ctx, &exactRoutes,
		db.Rebind(`SELECT id, model_pattern FROM token_routes
		 WHERE enabled = ? AND route_mode != ?`), true, "explicit_group"); err != nil {
		return nil, fmt.Errorf("load exact routes: %w", err)
	}
	for _, er := range exactRoutes {
		if er.ID == routeID {
			continue
		}
		if !routing.IsExactRouteModelPattern(er.ModelPattern) {
			continue
		}
		if !routing.MatchesModelPattern(er.ModelPattern, pattern) {
			continue
		}
		type chRow struct {
			AccountID   int64   `db:"account_id"`
			TokenID     *int64  `db:"token_id"`
			SourceModel *string `db:"source_model"`
			Priority    *int64  `db:"priority"`
			Weight      *int64  `db:"weight"`
			Enabled     bool    `db:"enabled"`
		}
		var channels []chRow
		if err := db.SelectContext(ctx, &channels,
			db.Rebind(`SELECT account_id, token_id, source_model, priority, weight, enabled
			 FROM route_channels WHERE route_id = ? AND enabled = ? AND manual_override = ?`), er.ID, true, true); err != nil {
			return nil, fmt.Errorf("load channels for exact route %d: %w", er.ID, err)
		}
		for _, ch := range channels {
			sm := ch.SourceModel
			if sm == nil || *sm == "" {
				// Fall back to the exact route's model name.
				mp := er.ModelPattern
				sm = &mp
			}
			key := channelKey(ch.AccountID, ch.TokenID, sm)
			if _, exists := desired[key]; exists {
				continue // first occurrence wins (exact-route channels first)
			}
			weight := int64(0)
			if ch.Weight != nil {
				weight = *ch.Weight
			}
			if weight <= 0 {
				weight = 10
			}
			priority := int64(0)
			if ch.Priority != nil {
				priority = *ch.Priority
			}
			desired[key] = desiredChannel{
				AccountID:   ch.AccountID,
				TokenID:     ch.TokenID,
				SourceModel: sm,
				Priority:    priority,
				Weight:      weight,
				Enabled:     true,
			}
		}
	}

	// Source 2a: token_model_availability (token-scoped models).
	type tokenAvail struct {
		TokenID   int64  `db:"token_id"`
		AccountID int64  `db:"account_id"`
		ModelName string `db:"model_name"`
	}
	var tokenRows []tokenAvail
	if err := db.SelectContext(ctx, &tokenRows,
		db.Rebind(`SELECT tma.token_id, at.account_id, tma.model_name
		 FROM token_model_availability tma
		 JOIN account_tokens at ON at.id = tma.token_id
		 JOIN accounts a ON a.id = at.account_id
		 WHERE tma.available = ?
		   AND at.enabled = ?
		   AND a.status = ?
		   AND (at.value_status IS NULL OR at.value_status != ?)`),
		true, true, "active", "expired"); err != nil {
		return nil, fmt.Errorf("load token_model_availability: %w", err)
	}
	for _, row := range tokenRows {
		if !routing.MatchesModelPattern(row.ModelName, pattern) {
			continue
		}
		tid := row.TokenID
		sm := row.ModelName
		key := channelKey(row.AccountID, &tid, &sm)
		if _, exists := desired[key]; exists {
			continue
		}
		desired[key] = desiredChannel{
			AccountID:   row.AccountID,
			TokenID:     &tid,
			SourceModel: &sm,
			Priority:    0,
			Weight:      10,
			Enabled:     true,
		}
	}

	// Source 2b: model_availability (account-scoped models, no token).
	type modelAvail struct {
		AccountID int64  `db:"account_id"`
		ModelName string `db:"model_name"`
	}
	var modelRows []modelAvail
	if err := db.SelectContext(ctx, &modelRows,
		db.Rebind(`SELECT ma.account_id, ma.model_name
		 FROM model_availability ma
		 JOIN accounts a ON a.id = ma.account_id
		 WHERE ma.available = ?
		   AND a.status = ?`), true, "active"); err != nil {
		return nil, fmt.Errorf("load model_availability: %w", err)
	}
	for _, row := range modelRows {
		if !routing.MatchesModelPattern(row.ModelName, pattern) {
			continue
		}
		sm := row.ModelName
		key := channelKey(row.AccountID, nil, &sm)
		if _, exists := desired[key]; exists {
			continue
		}
		desired[key] = desiredChannel{
			AccountID:   row.AccountID,
			TokenID:     nil,
			SourceModel: &sm,
			Priority:    0,
			Weight:      10,
			Enabled:     true,
		}
	}

	return desired, nil
}
