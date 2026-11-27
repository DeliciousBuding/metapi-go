package scheduler

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestChannelRecoveryOnlyProbesCoolingChannels(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			if dialect == store.DialectSQLite {
				db = sqlitetest.Open(t)
			} else {
				dsn := os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				var err error
				db, err = store.Open(dialect, dsn, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				pgtest.Reset(t, db.DB)
				if err := store.AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			// No network probe owner: a sweep records exactly which targets it
			// would send to the existing ModelProbeScheduler.
			prev := GetGlobalModelProbeScheduler()
			SetGlobalModelProbeScheduler(nil)
			t.Cleanup(func() { SetGlobalModelProbeScheduler(prev) })
			future := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)
			past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
			cooling := seedRecoveryChannel(t, db, "cooling", "fixture-model", true, &future)
			seedRecoveryChannel(t, db, "healthy", "healthy-model", true, nil)
			seedRecoveryChannel(t, db, "disabled", "off-model", false, &future)
			seedRecoveryChannel(t, db, "expired", "expired-model", true, &past)
			seedRecoveryChannel(t, db, "no-model", "", true, &future)
			inactive := seedRecoveryChannel(t, db, "inactive", "inactive-model", true, &future)
			if _, err := db.Exec("UPDATE accounts SET status = ? WHERE id = (SELECT account_id FROM route_channels WHERE id = ?)", "disabled", inactive); err != nil {
				t.Fatal(err)
			}
			s := NewChannelRecoveryScheduler(db, testConfig())
			candidates := s.loadCoolingCandidates(db)
			if len(candidates) != 1 || candidates[0].channelID != cooling || candidates[0].modelName != "fixture-model" {
				t.Fatalf("cooling candidates = %+v", candidates)
			}
			s.runSweepLocked(db)
			if len(s.lastStartedAtByKey) != 1 {
				t.Fatalf("sweep probed healthy/ineligible channels: %+v", s.lastStartedAtByKey)
			}
			if _, ok := s.lastStartedAtByKey[fmt.Sprintf("%d:fixture-model", cooling)]; !ok {
				t.Fatal("cooling channel was not probed")
			}
			if _, err := db.Exec("UPDATE route_channels SET cooldown_until = NULL WHERE id = ?", cooling); err != nil {
				t.Fatal(err)
			}
			s = NewChannelRecoveryScheduler(db, testConfig())
			s.runSweepLocked(db)
			if len(s.lastStartedAtByKey) != 0 {
				t.Fatalf("recovery started a healthy-channel scan: %+v", s.lastStartedAtByKey)
			}
		})
	}
}

func seedRecoveryChannel(t *testing.T, db *store.DB, suffix, model string, enabled bool, cooldownUntil *string) int64 {
	t.Helper()
	insert := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRow(query+" RETURNING id", args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if id <= 0 {
			t.Fatalf("invalid fixture id %d", id)
		}
		return id
	}
	now := time.Now().UTC().Format(time.RFC3339)
	siteID := insert(
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"site-"+suffix, "https://example.invalid/"+suffix, "anyrouter", "active", now, now)
	accountID := insert(
		`INSERT INTO accounts (site_id, access_token, api_token, status, balance, quota, value_score, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		siteID, "tok-"+suffix, "tok-"+suffix, "active", 10.0, 100.0, 1.0, now, now)
	routeID := insert(
		`INSERT INTO token_routes (model_pattern, route_mode, routing_strategy, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"route-"+suffix, "pattern", "weighted", true, now, now)
	return insert(
		`INSERT INTO route_channels (route_id, account_id, source_model, priority, weight, enabled, cooldown_until)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		routeID, accountID, model, 0, 10, enabled, cooldownUntil)
}
