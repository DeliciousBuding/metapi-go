package service

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func seedProbeResult(t *testing.T, db *store.DB, accountID, siteID int64, modelName, status string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO model_probe_results
			(channel_id, account_id, site_id, model_name, status, created_at)
		 VALUES (NULL, ?, ?, ?, ?, ?)`,
		accountID, siteID, modelName, status, now,
	); err != nil {
		t.Fatalf("insert probe result: %v", err)
	}
}

func seedPatternRoute(t *testing.T, db *store.DB, pattern string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at)
		 VALUES (?, 'pattern', 'weighted', TRUE, ?, ?)`, pattern, now, now,
	)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestRebuild_ProbeFailureExcludesChannel(t *testing.T) {
	db := setupRouteRebuildDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, accountID, tokenID := seedSiteAccountToken(t, db, "probe-fail")
	siteID := siteIDForAccount(t, db, accountID)
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("avail: %v", err)
	}
	routeID := seedPatternRoute(t, db, "gpt-*")
	seedProbeResult(t, db, accountID, siteID, "gpt-4o", "failure")

	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{
		ProbeFilter: ProbeFilterConfig{Enabled: true},
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if stats.ChannelsInserted != 0 {
		t.Fatalf("channelsInserted = %d, want 0 (probe-failed channel filtered)", stats.ChannelsInserted)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("route channels = %d, want 0", count)
	}
}

func TestRebuild_ProbeFilterDisabledKeepsProbeFailedChannel(t *testing.T) {
	db := setupRouteRebuildDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, accountID, tokenID := seedSiteAccountToken(t, db, "legacy")
	siteID := siteIDForAccount(t, db, accountID)
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("avail: %v", err)
	}
	seedPatternRoute(t, db, "gpt-*")
	seedProbeResult(t, db, accountID, siteID, "gpt-4o", "failure")

	stats, err := RebuildTokenRoutesFromAvailability(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if stats.ChannelsInserted < 1 {
		t.Fatalf("channelsInserted = %d, want >= 1 (legacy path ignores probe)", stats.ChannelsInserted)
	}
}

func TestRebuild_ProbeLatestSuccessWinsOverEarlierFailure(t *testing.T) {
	db := setupRouteRebuildDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, accountID, tokenID := seedSiteAccountToken(t, db, "recovers")
	siteID := siteIDForAccount(t, db, accountID)
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("avail: %v", err)
	}
	seedPatternRoute(t, db, "gpt-*")
	seedProbeResult(t, db, accountID, siteID, "gpt-4o", "failure")
	seedProbeResult(t, db, accountID, siteID, "gpt-4o", "success")

	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{
		ProbeFilter: ProbeFilterConfig{Enabled: true},
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	if stats.ChannelsInserted < 1 {
		t.Fatalf("channelsInserted = %d, want >= 1 (latest success keeps channel)", stats.ChannelsInserted)
	}
}

func TestRebuild_ExcludeModelListFiltersChannel(t *testing.T) {
	db := setupRouteRebuildDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _, tokenID := seedSiteAccountToken(t, db, "exclude")
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'claude-3', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("avail: %v", err)
	}
	routeID := seedPatternRoute(t, db, "*")
	stats, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, RebuildOptions{
		ProbeFilter: ProbeFilterConfig{Enabled: true, ExcludeModels: []string{"claude-3"}},
	})
	if err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM route_channels WHERE route_id = ?`, routeID); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("route channels = %d, want 0 (excluded model)", count)
	}
	if stats.ChannelsInserted != 0 {
		t.Fatalf("channelsInserted = %d, want 0", stats.ChannelsInserted)
	}
}

func TestRebuild_NoChangeShortCircuitSkipsCacheInvalidation(t *testing.T) {
	db := setupRouteRebuildDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	_, _, tokenID := seedSiteAccountToken(t, db, "short")
	if _, err := db.Exec(
		`INSERT INTO token_model_availability (token_id, model_name, available, checked_at)
		 VALUES (?, 'gpt-4o', TRUE, ?)`, tokenID, now); err != nil {
		t.Fatalf("avail: %v", err)
	}
	seedPatternRoute(t, db, "gpt-*")

	opts := RebuildOptions{ProbeFilter: ProbeFilterConfig{Enabled: true}}
	first, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, opts)
	if err != nil {
		t.Fatalf("first rebuild: %v", err)
	}
	if first.ChannelsInserted < 1 || !first.Changed {
		t.Fatalf("first rebuild changed=%v inserted=%d, want changed", first.Changed, first.ChannelsInserted)
	}

	second, err := RebuildTokenRoutesFromAvailabilityWithOptions(context.Background(), db.DB, opts)
	if err != nil {
		t.Fatalf("second rebuild: %v", err)
	}
	if second.ChannelsInserted != 0 || second.ChannelsRemoved != 0 {
		t.Fatalf("second rebuild inserted=%d removed=%d, want 0/0", second.ChannelsInserted, second.ChannelsRemoved)
	}
	if second.Changed {
		t.Fatalf("second rebuild Changed=true, want false (no model-set change)")
	}
}

func siteIDForAccount(t *testing.T, db *store.DB, accountID int64) int64 {
	t.Helper()
	var siteID int64
	if err := db.Get(&siteID, `SELECT site_id FROM accounts WHERE id = ?`, accountID); err != nil {
		t.Fatalf("load site id: %v", err)
	}
	return siteID
}
