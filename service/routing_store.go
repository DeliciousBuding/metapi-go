// Package service hosts the SQL-backed routing store that feeds the token
// router (moved from app/proxy_upstream.go so the app layer stays pure
// wiring). The channel/account/site join columns used by several loaders are
// kept in the channelAccountSiteSelect fragment so a store schema change is a
// one-line edit here instead of a silent drift across three queries.
package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// Compile-time assertions: the routing store satisfies the interfaces the
// token router / route decision service consume.
var (
	_ routing.ChannelSelectorDB = (*ProxyRoutingStore)(nil)
	_ routing.DecisionDB        = (*ProxyRoutingStore)(nil)
	_ routing.SnapshotDB        = (*ProxyRoutingStore)(nil)
)

type ProxyRoutingStore struct {
	db *store.DB
}

func NewProxyRoutingStore(db *store.DB) *ProxyRoutingStore {
	return &ProxyRoutingStore{db: db}
}

func (s *ProxyRoutingStore) LoadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	var routes []store.TokenRoute
	err := s.selectContext(ctx, &routes, `
		SELECT id, model_pattern, display_name, display_icon, route_mode, model_mapping,
		       decision_snapshot, decision_refreshed_at, routing_strategy, trace_sticky_mode, context_length,
		       sort_order, enabled, created_at, updated_at
		FROM token_routes
		WHERE enabled = ?
		ORDER BY sort_order ASC, id ASC`, true)
	return routes, err
}

func (s *ProxyRoutingStore) FindAllEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	return s.LoadEnabledRoutes(ctx)
}

func (s *ProxyRoutingStore) LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	if len(groupRouteIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT group_route_id, source_route_id
		FROM route_group_sources
		WHERE group_route_id IN (?)
		ORDER BY group_route_id ASC, source_route_id ASC`, groupRouteIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, sourceID int64
		if err := rows.Scan(&groupID, &sourceID); err != nil {
			return nil, err
		}
		result[groupID] = append(result[groupID], sourceID)
	}
	return result, rows.Err()
}

// UpdateRouteDecisionSnapshot persists a route decision explanation snapshot.
func (s *ProxyRoutingStore) UpdateRouteDecisionSnapshot(ctx context.Context, routeID int64, snapshot string, refreshedAt string) error {
	if strings.TrimSpace(refreshedAt) == "" {
		refreshedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = ?, decision_refreshed_at = ?, updated_at = ?
		WHERE id = ?`, snapshot, refreshedAt, refreshedAt, routeID)
	return err
}

// ClearRouteDecisionSnapshot clears one route decision snapshot.
func (s *ProxyRoutingStore) ClearRouteDecisionSnapshot(ctx context.Context, routeID int64) error {
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE id = ?`, routeID)
	return err
}

// ClearRouteDecisionSnapshots clears decision snapshots for the given route IDs.
func (s *ProxyRoutingStore) ClearRouteDecisionSnapshots(ctx context.Context, routeIDs []int64) error {
	if len(routeIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE id IN (?)`, routeIDs)
	if err != nil {
		return err
	}
	_, err = s.execContext(ctx, query, args...)
	return err
}

// ClearAllRouteDecisionSnapshots clears every route decision snapshot.
func (s *ProxyRoutingStore) ClearAllRouteDecisionSnapshots(ctx context.Context) error {
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE decision_snapshot IS NOT NULL OR decision_refreshed_at IS NOT NULL`)
	return err
}

// queryxContext/selectContext/execContext delegate to store.DB's Context
// helpers, which rebind ? to $N for PostgreSQL internally. Keeping the dialect
// branch out of the routing store means callers never branch on dialect here.

func (s *ProxyRoutingStore) queryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	return s.db.QueryxContext(ctx, query, args...)
}

func (s *ProxyRoutingStore) selectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return s.db.SelectContext(ctx, dest, query, args...)
}

func (s *ProxyRoutingStore) execContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}
