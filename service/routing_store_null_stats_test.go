package service

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// These tests guard the #933-family residual: route_channels / token_routes /
// sites / oauth_route_unit_members carry nullable numeric columns (DEFAULT
// without NOT NULL in the DDL), and the routing loaders scanned them into bare
// int64/float64/bool fields. A single NULL row then failed with "converting
// NULL to int64" and took down the whole routing load (LoadEnabledRoutes /
// LoadRouteChannels / LoadOAuthRouteUnitMembers). Mirrors the #933 fix
// pattern: pointer-typed columns + OrZero helpers for numeric callers.

func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func insertNullSortOrderRoute(t *testing.T, db *store.DB, pattern string) int64 {
	t.Helper()
	now := nowISO()
	res, err := db.Exec(
		`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at)
		 VALUES (?, 'pattern', 'weighted', 1, ?, ?)`, pattern, now, now)
	if err != nil {
		t.Fatalf("INSERT token_routes failed: %v", err)
	}
	routeID, _ := res.LastInsertId()
	if _, err := db.Exec(`UPDATE token_routes SET sort_order = NULL WHERE id = ?`, routeID); err != nil {
		t.Fatalf("null token_routes.sort_order: %v", err)
	}
	return routeID
}

func insertNullStatsRouteChannel(t *testing.T, db *store.DB, routeID, accountID int64) int64 {
	t.Helper()
	res, err := db.Exec(
		`INSERT INTO route_channels (route_id, account_id, enabled) VALUES (?, ?, 1)`,
		routeID, accountID)
	if err != nil {
		t.Fatalf("INSERT route_channels failed: %v", err)
	}
	channelID, _ := res.LastInsertId()
	if _, err := db.Exec(`
		UPDATE route_channels SET
			priority = NULL, weight = NULL, success_count = NULL,
			fail_count = NULL, total_latency_ms = NULL, total_cost = NULL
		WHERE id = ?`, channelID); err != nil {
		t.Fatalf("null route_channels stat columns: %v", err)
	}
	return channelID
}

func nullSiteStatColumns(t *testing.T, db *store.DB, siteID int64) {
	t.Helper()
	if _, err := db.Exec(`
		UPDATE sites SET is_pinned = NULL, sort_order = NULL, global_weight = NULL
		WHERE id = ?`, siteID); err != nil {
		t.Fatalf("null sites stat columns: %v", err)
	}
}

// TestLoadEnabledRoutes_NullSortOrderDoesNotError guards token_routes.sort_order
// (INTEGER DEFAULT 0, nullable): a NULL row must load, not crash the routing
// table scan.
func TestLoadEnabledRoutes_NullSortOrderDoesNotError(t *testing.T) {
	db := openTestDB(t)
	insertNullSortOrderRoute(t, db, "null-sort-*")

	routes, err := NewProxyRoutingStore(db).LoadEnabledRoutes(context.Background())
	if err != nil {
		t.Fatalf("LoadEnabledRoutes with NULL sort_order: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("LoadEnabledRoutes returned %d routes, want 1", len(routes))
	}
	if routes[0].SortOrder != nil {
		t.Errorf("SortOrder = %v, want nil (NULL preserved)", *routes[0].SortOrder)
	}
	if routes[0].SortOrderOrZero() != 0 {
		t.Errorf("SortOrderOrZero = %d, want 0", routes[0].SortOrderOrZero())
	}
}

// TestLoadRouteChannels_NullStatsDoNotError guards the route_channels stat
// columns (priority/weight/success_count/fail_count/total_latency_ms/
// total_cost, all DEFAULT without NOT NULL) plus the joined sites columns
// (is_pinned/sort_order/global_weight): NULL rows must load through the
// 5-table join, not fail with "converting NULL to int64".
func TestLoadRouteChannels_NullStatsDoNotError(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "Null Stats Site", "https://nullstats.example.com", "openai")
	accountID := createTestAccount(t, db, siteID, strPtr("null-stats-user"), "sk-null-stats")
	nullSiteStatColumns(t, db, siteID)
	routeID := insertNullSortOrderRoute(t, db, "null-stats-*")
	insertNullStatsRouteChannel(t, db, routeID, accountID)

	rows, err := NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), []int64{routeID})
	if err != nil {
		t.Fatalf("LoadRouteChannels with NULL stat columns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("LoadRouteChannels returned %d rows, want 1", len(rows))
	}

	channel := rows[0].Channel
	if channel.Priority != nil || channel.Weight != nil || channel.SuccessCount != nil ||
		channel.FailCount != nil || channel.TotalLatencyMs != nil || channel.TotalCost != nil {
		t.Errorf("NULL stat columns must scan to nil: priority=%v weight=%v success=%v fail=%v latency=%v cost=%v",
			channel.Priority, channel.Weight, channel.SuccessCount, channel.FailCount, channel.TotalLatencyMs, channel.TotalCost)
	}
	if channel.PriorityOrZero() != 0 || channel.WeightOrZero() != 0 || channel.SuccessCountOrZero() != 0 ||
		channel.FailCountOrZero() != 0 || channel.TotalLatencyMsOrZero() != 0 || channel.TotalCostOrZero() != 0 {
		t.Errorf("OrZero helpers must return 0 for nil stat columns")
	}

	site := rows[0].Site
	if site.IsPinned != nil || site.SortOrder != nil || site.GlobalWeight != nil {
		t.Errorf("NULL site columns must scan to nil: isPinned=%v sortOrder=%v globalWeight=%v",
			site.IsPinned, site.SortOrder, site.GlobalWeight)
	}
	if site.IsPinnedOrFalse() != false || site.SortOrderOrZero() != 0 || site.GlobalWeightOrZero() != 0 {
		t.Errorf("site OrZero helpers must return zero values for nil columns")
	}
}

// TestLoadOAuthRouteUnitMembers_NullStatsDoNotError guards the
// oauth_route_unit_members stat columns (sort_order/success_count/fail_count/
// total_latency_ms/total_cost): NULL member rows must load through the join.
func TestLoadOAuthRouteUnitMembers_NullStatsDoNotError(t *testing.T) {
	db := openTestDB(t)
	siteID := createTestSite(t, db, "Null Member Site", "https://nullmember.example.com", "openai")
	accountID := createTestAccount(t, db, siteID, strPtr("null-member-user"), "sk-null-member")
	nullSiteStatColumns(t, db, siteID)

	now := nowISO()
	res, err := db.Exec(
		`INSERT INTO oauth_route_units (site_id, provider, name, strategy, enabled, created_at, updated_at)
		 VALUES (?, 'google', 'null-member-unit', 'round_robin', 1, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("INSERT oauth_route_units failed: %v", err)
	}
	unitID, _ := res.LastInsertId()

	if _, err := db.Exec(
		`INSERT INTO oauth_route_unit_members (unit_id, account_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`, unitID, accountID, now, now); err != nil {
		t.Fatalf("INSERT oauth_route_unit_members failed: %v", err)
	}
	if _, err := db.Exec(`
		UPDATE oauth_route_unit_members SET
			sort_order = NULL, success_count = NULL, fail_count = NULL,
			total_latency_ms = NULL, total_cost = NULL
		WHERE unit_id = ?`, unitID); err != nil {
		t.Fatalf("null oauth_route_unit_members stat columns: %v", err)
	}

	members, err := NewProxyRoutingStore(db).LoadOAuthRouteUnitMembers(context.Background(), []int64{unitID})
	if err != nil {
		t.Fatalf("LoadOAuthRouteUnitMembers with NULL stat columns: %v", err)
	}
	if len(members[unitID]) != 1 {
		t.Fatalf("LoadOAuthRouteUnitMembers returned %d members, want 1", len(members[unitID]))
	}

	member := members[unitID][0].Member
	if member.SortOrder != nil || member.SuccessCount != nil || member.FailCount != nil ||
		member.TotalLatencyMs != nil || member.TotalCost != nil {
		t.Errorf("NULL member stat columns must scan to nil: sort=%v success=%v fail=%v latency=%v cost=%v",
			member.SortOrder, member.SuccessCount, member.FailCount, member.TotalLatencyMs, member.TotalCost)
	}
	if member.SortOrderOrZero() != 0 || member.SuccessCountOrZero() != 0 || member.FailCountOrZero() != 0 ||
		member.TotalLatencyMsOrZero() != 0 || member.TotalCostOrZero() != 0 {
		t.Errorf("member OrZero helpers must return 0 for nil stat columns")
	}
	if members[unitID][0].Site.GlobalWeightOrZero() != 0 {
		t.Errorf("joined site GlobalWeightOrZero must return 0 for NULL global_weight")
	}
}

// TestNullableStatColumnsOrZeroSemantics checks the OrZero helpers on both nil
// and populated values (nil -> zero, set -> value), mirroring the #933 Account
// helper tests.
func TestNullableStatColumnsOrZeroSemantics(t *testing.T) {
	channel := store.RouteChannel{}
	if channel.PriorityOrZero() != 0 || channel.WeightOrZero() != 0 || channel.SuccessCountOrZero() != 0 ||
		channel.FailCountOrZero() != 0 || channel.TotalLatencyMsOrZero() != 0 || channel.TotalCostOrZero() != 0 {
		t.Fatalf("RouteChannel OrZero helpers must return 0 for nil fields")
	}
	priority, weight, success, fail, latency, cost := int64(3), int64(15), int64(7), int64(2), int64(250), 1.25
	channel = store.RouteChannel{
		Priority: &priority, Weight: &weight, SuccessCount: &success,
		FailCount: &fail, TotalLatencyMs: &latency, TotalCost: &cost,
	}
	if channel.PriorityOrZero() != 3 || channel.WeightOrZero() != 15 || channel.SuccessCountOrZero() != 7 ||
		channel.FailCountOrZero() != 2 || channel.TotalLatencyMsOrZero() != 250 || channel.TotalCostOrZero() != 1.25 {
		t.Fatalf("RouteChannel OrZero helpers must return the stored value for non-nil fields")
	}

	member := store.OAuthRouteUnitMember{}
	if member.SortOrderOrZero() != 0 || member.SuccessCountOrZero() != 0 || member.FailCountOrZero() != 0 ||
		member.TotalLatencyMsOrZero() != 0 || member.TotalCostOrZero() != 0 {
		t.Fatalf("OAuthRouteUnitMember OrZero helpers must return 0 for nil fields")
	}

	site := store.Site{}
	if site.IsPinnedOrFalse() != false || site.SortOrderOrZero() != 0 || site.GlobalWeightOrZero() != 0 {
		t.Fatalf("Site OrZero helpers must return zero values for nil fields")
	}
	pinned, sortOrder, globalWeight := true, int64(4), 2.5
	site = store.Site{IsPinned: &pinned, SortOrder: &sortOrder, GlobalWeight: &globalWeight}
	if site.IsPinnedOrFalse() != true || site.SortOrderOrZero() != 4 || site.GlobalWeightOrZero() != 2.5 {
		t.Fatalf("Site OrZero helpers must return the stored value for non-nil fields")
	}

	route := store.TokenRoute{}
	if route.SortOrderOrZero() != 0 {
		t.Fatalf("TokenRoute.SortOrderOrZero must return 0 for nil sort_order")
	}
}
