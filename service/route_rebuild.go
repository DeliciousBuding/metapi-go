package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// RouteRebuildStats summarizes a synchronous rebuild pass.
// explicit_group routes do not materialize their own channels; they expand
// source routes at selection time via route_group_sources.
type RouteRebuildStats struct {
	RoutesConsidered int `json:"routesConsidered"`
	PatternRoutes    int `json:"patternRoutes"`
	GroupRoutes      int `json:"groupRoutes"`
	ChannelsInserted int `json:"channelsInserted"`
	ChannelsRemoved  int `json:"channelsRemoved"`
	ChannelsKept     int `json:"channelsKept"`
	// Changed is true when at least one automatic channel was inserted or removed.
	Changed bool `json:"changed"`
}

// RebuildOptions configures optional rebuild behavior. The zero value keeps the
// legacy non-probe path byte-for-byte identical.
type RebuildOptions struct {
	ProbeFilter ProbeFilterConfig
}

// ProbeFilterConfig gates the route-rebuild probe filter (#625).
// IncludeModels/ExcludeModels are optional exact source-model names; empty means
// no-op. When Enabled, desired channels whose latest connectivity probe failed
// are dropped before any route_channels diff is written.
type ProbeFilterConfig struct {
	Enabled       bool
	IncludeModels []string
	ExcludeModels []string
}

var (
	routeRebuildMu sync.Mutex
	// optional override for tests / injected SQL handles without store.GetDB
	routeRebuildDBMu sync.RWMutex
	routeRebuildDB   *sqlx.DB
)

// SetRouteRebuildDB sets the SQL handle used by RebuildRoutesBestEffort when
// store.GetDB() is not initialized (tests). Pass nil to clear.
func SetRouteRebuildDB(db *sqlx.DB) {
	routeRebuildDBMu.Lock()
	defer routeRebuildDBMu.Unlock()
	routeRebuildDB = db
}

// RebuildRoutesBestEffort rebuilds pattern-route channels from current model
// availability and invalidates the token-router cache. Failures are logged and
// swallowed so account/site mutation paths stay best-effort.
func RebuildRoutesBestEffort() {
	db := resolveRebuildDB()
	if db == nil {
		slog.Debug("route rebuild skipped: database not available")
		return
	}
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db, rebuildOptionsFromConfig())
	if err != nil {
		slog.Warn("route rebuild best-effort failed", "error", err)
		return
	}
	slog.Info("route rebuild best-effort completed",
		"routesConsidered", stats.RoutesConsidered,
		"patternRoutes", stats.PatternRoutes,
		"groupRoutes", stats.GroupRoutes,
		"channelsInserted", stats.ChannelsInserted,
		"channelsRemoved", stats.ChannelsRemoved,
	)
}

// RebuildTokenRoutesFromAvailability is the legacy (non-probe) rebuild entrypoint.
// It keeps byte-for-byte identical semantics: the probe filter is off and the
// in-process cache is always invalidated after the pass.
func RebuildTokenRoutesFromAvailability(ctx context.Context, db *sqlx.DB) (RouteRebuildStats, error) {
	return RebuildTokenRoutesFromAvailabilityWithOptions(ctx, db, RebuildOptions{})
}

// RebuildTokenRoutesFromAvailabilityWithOptions repopulates automatic
// (non-manual) channels for every pattern/exact route from dual sources:
// 1. channels on exact-model routes whose model_pattern matches the target pattern
// 2. token_model_availability + model_availability rows whose model_name matches
//
// explicit_group routes are counted but not rewritten: their membership is
// route_group_sources, and channel expansion happens at select time. Manual
// override channels on pattern routes are never deleted.
//
// When opts.ProbeFilter.Enabled is true, desired channels are filtered by
// include/exclude model lists plus the latest connectivity probe result before
// any diff is written, and the cache is invalidated only when the model set
// actually changes. The zero-value RebuildOptions{} preserves legacy behavior.
//
// Concurrent rebuilds serialize on a process-wide mutex.
func RebuildTokenRoutesFromAvailabilityWithOptions(ctx context.Context, db *sqlx.DB, opts RebuildOptions) (RouteRebuildStats, error) {
	var stats RouteRebuildStats
	if db == nil {
		return stats, fmt.Errorf("route rebuild: db is nil")
	}
	if err := ctx.Err(); err != nil {
		return stats, err
	}

	routeRebuildMu.Lock()
	defer routeRebuildMu.Unlock()

	type routeRow struct {
		ID           int64  `db:"id"`
		ModelPattern string `db:"model_pattern"`
		RouteMode    string `db:"route_mode"`
		Enabled      bool   `db:"enabled"`
	}
	var routes []routeRow
	if err := db.SelectContext(ctx, &routes,
		`SELECT id, model_pattern, route_mode, enabled FROM token_routes ORDER BY id ASC`); err != nil {
		return stats, fmt.Errorf("load token_routes: %w", err)
	}

	stats.RoutesConsidered = len(routes)
	for _, route := range routes {
		if routing.IsExplicitGroupRoute(route.RouteMode) {
			stats.GroupRoutes++
			continue
		}
		stats.PatternRoutes++
		inserted, removed, kept, err := rebuildAutomaticRouteChannelsByModelPattern(ctx, db, route.ID, route.ModelPattern, opts)
		if err != nil {
			return stats, fmt.Errorf("rebuild route %d (%s): %w", route.ID, route.ModelPattern, err)
		}
		stats.ChannelsInserted += inserted
		stats.ChannelsRemoved += removed
		stats.ChannelsKept += kept
	}

	changed := stats.ChannelsInserted + stats.ChannelsRemoved
	stats.Changed = changed > 0
	if opts.ProbeFilter.Enabled {
		if changed > 0 {
			routing.InvalidateCache()
		} else {
			slog.Info("route rebuild: model set unchanged, skipping cache invalidation")
		}
	} else {
		// Legacy non-probe path: always invalidate (identical to prior behavior).
		routing.InvalidateCache()
	}
	return stats, nil
}

// PopulateRouteChannelsByModelPattern is the create-path helper: insert automatic
// channels for a newly created pattern route without wiping existing rows.
func PopulateRouteChannelsByModelPattern(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string) (inserted int, err error) {
	if db == nil {
		return 0, fmt.Errorf("populate channels: db is nil")
	}
	routeRebuildMu.Lock()
	defer routeRebuildMu.Unlock()

	desired, err := collectDesiredChannels(ctx, db, routeID, modelPattern)
	if err != nil {
		return 0, err
	}
	existing, err := loadRouteChannelKeys(ctx, db, routeID)
	if err != nil {
		return 0, err
	}
	for key, cand := range desired {
		if _, ok := existing[key]; ok {
			continue
		}
		if err := insertAutoChannel(ctx, db, routeID, cand); err != nil {
			return inserted, err
		}
		inserted++
	}
	routing.InvalidateCache()
	return inserted, nil
}

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

func rebuildAutomaticRouteChannelsByModelPattern(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string, opts RebuildOptions) (inserted, removed, kept int, err error) {
	desired, err := collectDesiredChannels(ctx, db, routeID, modelPattern)
	if err != nil {
		return 0, 0, 0, err
	}
	if err := applyProbeFilter(ctx, db, desired, opts); err != nil {
		return 0, 0, 0, err
	}

	type existingRow struct {
		ID             int64   `db:"id"`
		AccountID      int64   `db:"account_id"`
		TokenID        *int64  `db:"token_id"`
		SourceModel    *string `db:"source_model"`
		ManualOverride bool    `db:"manual_override"`
	}
	var rows []existingRow
	if err := db.SelectContext(ctx, &rows,
		db.Rebind(`SELECT id, account_id, token_id, source_model, manual_override
		 FROM route_channels WHERE route_id = ?`), routeID); err != nil {
		return 0, 0, 0, fmt.Errorf("load route_channels: %w", err)
	}

	// No-change short-circuit: when the automatic channel key set already equals
	// the desired key set, skip every write. The caller uses the zero-diff result
	// to skip cache invalidation on the probe-filter path.
	autoExisting := make(map[channelIdentity]struct{}, len(rows))
	manualCount := 0
	for _, row := range rows {
		if row.ManualOverride {
			manualCount++
			continue
		}
		autoExisting[channelKey(row.AccountID, row.TokenID, row.SourceModel)] = struct{}{}
	}
	if sameChannelKeySet(desired, autoExisting) {
		return 0, 0, manualCount + len(autoExisting), nil
	}

	present := make(map[channelIdentity]struct{}, len(rows))
	for _, row := range rows {
		key := channelKey(row.AccountID, row.TokenID, row.SourceModel)
		if row.ManualOverride {
			// Manual attachments are never deleted by rebuild.
			present[key] = struct{}{}
			kept++
			continue
		}
		if _, ok := desired[key]; ok {
			present[key] = struct{}{}
			kept++
			continue
		}
		if _, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM route_channels WHERE id = ?`), row.ID); err != nil {
			return inserted, removed, kept, fmt.Errorf("delete stale channel %d: %w", row.ID, err)
		}
		removed++
	}

	for key, cand := range desired {
		if _, ok := present[key]; ok {
			continue
		}
		if err := insertAutoChannel(ctx, db, routeID, cand); err != nil {
			return inserted, removed, kept, err
		}
		inserted++
	}
	return inserted, removed, kept, nil
}

func collectDesiredChannels(ctx context.Context, db *sqlx.DB, routeID int64, modelPattern string) (map[channelIdentity]desiredChannel, error) {
	desired := make(map[channelIdentity]desiredChannel)
	pattern := modelPattern

	// Source 1: channels from exact-model routes whose pattern matches the target.
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
			Priority    int64   `db:"priority"`
			Weight      int64   `db:"weight"`
			Enabled     bool    `db:"enabled"`
		}
		var channels []chRow
		if err := db.SelectContext(ctx, &channels,
			db.Rebind(`SELECT account_id, token_id, source_model, priority, weight, enabled
			 FROM route_channels WHERE route_id = ? AND enabled = ?`), er.ID, true); err != nil {
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
			weight := ch.Weight
			if weight <= 0 {
				weight = 10
			}
			desired[key] = desiredChannel{
				AccountID:   ch.AccountID,
				TokenID:     ch.TokenID,
				SourceModel: sm,
				Priority:    ch.Priority,
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

func loadRouteChannelKeys(ctx context.Context, db *sqlx.DB, routeID int64) (map[channelIdentity]struct{}, error) {
	type row struct {
		AccountID   int64   `db:"account_id"`
		TokenID     *int64  `db:"token_id"`
		SourceModel *string `db:"source_model"`
	}
	var rows []row
	if err := db.SelectContext(ctx, &rows,
		db.Rebind(`SELECT account_id, token_id, source_model FROM route_channels WHERE route_id = ?`), routeID); err != nil {
		return nil, err
	}
	out := make(map[channelIdentity]struct{}, len(rows))
	for _, r := range rows {
		out[channelKey(r.AccountID, r.TokenID, r.SourceModel)] = struct{}{}
	}
	return out, nil
}

func insertAutoChannel(ctx context.Context, db *sqlx.DB, routeID int64, cand desiredChannel) error {
	_, err := db.ExecContext(ctx,
		db.Rebind(`INSERT INTO route_channels
			(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		routeID, cand.AccountID, cand.TokenID, cand.SourceModel,
		cand.Priority, cand.Weight, cand.Enabled, false,
	)
	if err != nil {
		return fmt.Errorf("insert auto channel route=%d account=%d: %w", routeID, cand.AccountID, err)
	}
	return nil
}

func resolveRebuildDB() *sqlx.DB {
	routeRebuildDBMu.RLock()
	override := routeRebuildDB
	routeRebuildDBMu.RUnlock()
	if override != nil {
		return override
	}
	if dbw := store.GetDB(); dbw != nil {
		return dbw.DB
	}
	return nil
}

func sameChannelKeySet(desired map[channelIdentity]desiredChannel, existing map[channelIdentity]struct{}) bool {
	if len(desired) != len(existing) {
		return false
	}
	for k := range desired {
		if _, ok := existing[k]; !ok {
			return false
		}
	}
	return true
}

func toStringSet(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

type probeFailureKey struct {
	AccountID int64
	Model     string
}

// applyProbeFilter removes desired channels that the probe filter rejects.
// It is intentionally a pure in-memory filter over the desired set; the caller
// has not yet written any route_channels diff, so a rejected channel is simply
// not materialised (soft isolation) rather than hard-disabled.
func applyProbeFilter(ctx context.Context, db *sqlx.DB, desired map[channelIdentity]desiredChannel, opts RebuildOptions) error {
	if !opts.ProbeFilter.Enabled {
		return nil
	}
	include := toStringSet(opts.ProbeFilter.IncludeModels)
	exclude := toStringSet(opts.ProbeFilter.ExcludeModels)
	failed, err := loadLatestProbeFailures(ctx, db)
	if err != nil {
		return err
	}
	for key := range desired {
		model := key.SourceModel
		if len(include) > 0 && !include[model] {
			delete(desired, key)
			continue
		}
		if exclude[model] {
			delete(desired, key)
			continue
		}
		if failed[probeFailureKey{AccountID: key.AccountID, Model: model}] {
			delete(desired, key)
		}
	}
	return nil
}

// loadLatestProbeFailures returns the latest model_probe_results row per
// (account_id, model_name), marking only rows whose latest status is "failure".
// MAX(id) is monotonically increasing with time (SERIAL/AUTOINCREMENT), so this
// works on both SQLite and PostgreSQL without dialect-specific window syntax.
func loadLatestProbeFailures(ctx context.Context, db *sqlx.DB) (map[probeFailureKey]bool, error) {
	type row struct {
		AccountID int64  `db:"account_id"`
		ModelName string `db:"model_name"`
		Status    string `db:"status"`
	}
	var rows []row
	if err := db.SelectContext(ctx, &rows, db.Rebind(`
		SELECT account_id, model_name, status
		FROM model_probe_results
		WHERE id IN (SELECT MAX(id) FROM model_probe_results GROUP BY account_id, model_name)`)); err != nil {
		return nil, fmt.Errorf("load latest probe results: %w", err)
	}
	out := make(map[probeFailureKey]bool, len(rows))
	for _, r := range rows {
		if r.Status == "failure" {
			out[probeFailureKey{AccountID: r.AccountID, Model: r.ModelName}] = true
		}
	}
	return out, nil
}

// rebuildOptionsFromConfig resolves the probe-filter configuration from the
// global config singleton. A nil config yields the zero-value (legacy) options.
func rebuildOptionsFromConfig() RebuildOptions {
	cfg := config.GetSafe()
	if cfg == nil {
		return RebuildOptions{}
	}
	return RebuildOptions{
		ProbeFilter: ProbeFilterConfig{
			Enabled:       cfg.RouteRebuildProbeFilterEnabled,
			IncludeModels: cfg.RouteRebuildProbeFilterIncludeModels,
			ExcludeModels: cfg.RouteRebuildProbeFilterExcludeModels,
		},
	}
}
