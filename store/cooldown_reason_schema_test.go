package store

import (
	"strings"
	"testing"
)

// =============================================================================
// P0-3 — structured cooldown reason columns must exist in BOTH dialects.
// SQLite runs for real below; PostgreSQL is asserted statically on the DDL
// string (CI has no PG server in this lane; PG_TEST_DSN runs cover it when
// set).
// =============================================================================

func TestCooldownReasonDDLContainsColumnsBothDialects(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		channelDDL := buildRouteChannelsDDL(dialect)
		memberDDL := buildOAuthRouteUnitMembersDDL(dialect)
		for _, col := range []string{"cooldown_reason_code TEXT", "cooldown_reason TEXT", "cooldown_reason_at TEXT"} {
			if !strings.Contains(channelDDL, col) {
				t.Errorf("route_channels DDL (%s) missing %q", dialect, col)
			}
			if !strings.Contains(memberDDL, col) {
				t.Errorf("oauth_route_unit_members DDL (%s) missing %q", dialect, col)
			}
		}
	}
}

func TestCooldownReasonColumnsRoundTripSQLite(t *testing.T) {
	db := openTestSQLite(t)

	for _, table := range []string{"route_channels", "oauth_route_unit_members"} {
		for _, col := range []string{"cooldown_reason_code", "cooldown_reason", "cooldown_reason_at"} {
			ok, err := columnExists(db, table, col)
			if err != nil {
				t.Fatalf("columnExists(%s.%s): %v", table, col, err)
			}
			if !ok {
				t.Fatalf("%s.%s missing after AutoMigrate", table, col)
			}
		}
	}

	// Round-trip: write a reason, read it back, then clear it the same way
	// a channel failure reset does.
	if _, err := db.Exec(`INSERT INTO sites (id, name, url, platform, status) VALUES (1, 's', 'https://example.com', 'openai', 'active')`); err != nil {
		t.Fatalf("seed site: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (id, site_id, username, access_token, status) VALUES (1, 1, 'u', 'tok', 'active')`); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO token_routes (id, model_pattern, route_mode, enabled) VALUES (1, 'gpt-*', 'pattern', TRUE)`); err != nil {
		t.Fatalf("seed route: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO route_channels
		(id, route_id, account_id, enabled, cooldown_until, cooldown_reason_code, cooldown_reason, cooldown_reason_at)
		VALUES (1, 1, 1, TRUE, '2026-08-28T10:00:00Z', 'upstream_error', 'boom', '2026-08-28T09:00:00Z')`); err != nil {
		t.Fatalf("insert channel with reason: %v", err)
	}

	var code, reason, at *string
	if err := db.QueryRow(`SELECT cooldown_reason_code, cooldown_reason, cooldown_reason_at FROM route_channels WHERE id = 1`).Scan(&code, &reason, &at); err != nil {
		t.Fatalf("select reason: %v", err)
	}
	if code == nil || *code != "upstream_error" || reason == nil || *reason != "boom" || at == nil || *at != "2026-08-28T09:00:00Z" {
		t.Fatalf("round-trip mismatch: code=%v reason=%v at=%v", code, reason, at)
	}

	if _, err := db.Exec(`UPDATE route_channels SET cooldown_until = NULL, cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL WHERE id = 1`); err != nil {
		t.Fatalf("clear reason: %v", err)
	}
	if err := db.QueryRow(`SELECT cooldown_reason_code, cooldown_reason, cooldown_reason_at FROM route_channels WHERE id = 1`).Scan(&code, &reason, &at); err != nil {
		t.Fatalf("select cleared reason: %v", err)
	}
	if code != nil || reason != nil || at != nil {
		t.Fatalf("cleared reason should be NULL, got code=%v reason=%v at=%v", code, reason, at)
	}
}
