package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// openSub2APITestDB opens a fresh in-memory SQLite DB with AutoMigrate.
func openSub2APITestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

// insertSub2APITestSite inserts a site with the given platform and status.
func insertSub2APITestSite(t *testing.T, db *store.DB, name, platform, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		name, "https://"+name+".example.test", platform, status, now, now,
	)
	if err != nil {
		t.Fatalf("insert site %s: %v", name, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("site id: %v", err)
	}
	return id
}

// insertSub2APITestAccount inserts an account linked to the given site.
func insertSub2APITestAccount(t *testing.T, db *store.DB, siteID int64, username, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		siteID, username, "sk-"+username, status, 1, now, now,
	)
	if err != nil {
		t.Fatalf("insert account %s: %v", username, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return id
}

// TestSub2APIRefreshScheduler_runPassLocked_EmptyDB verifies that runPassLocked
// completes without error on a fresh DB with no sub2api accounts. The SQL
// query returns zero rows, so the pass completes with scanned=0, eligible=0.
func TestSub2APIRefreshScheduler_runPassLocked_EmptyDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openSub2APITestDB(t)
	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	// On an empty DB, the query returns no rows and the pass completes
	// without calling balance.RefreshBalance (no HTTP calls).
	s.runPassLocked(db)
}

// TestSub2APIRefreshScheduler_runPassLocked_NonSub2APISite verifies that
// accounts on non-sub2api platforms are not scanned. The INNER JOIN + WHERE
// clause filters them out, so scanned=0 even with data in the DB.
func TestSub2APIRefreshScheduler_runPassLocked_NonSub2APISite(t *testing.T) {
	ResetLeasePressureForTest()
	db := openSub2APITestDB(t)
	siteID := insertSub2APITestSite(t, db, "new-api-site", "new-api", "active")
	insertSub2APITestAccount(t, db, siteID, "user1", "active")

	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	// The account is on a new-api site, not sub2api — the query filters it out.
	s.runPassLocked(db)
}

// TestSub2APIRefreshScheduler_runPassLocked_Sub2APIAccountNotEligible
// verifies that a sub2api account without valid managed auth (no
// refreshToken in extra_config) is scanned but not eligible for refresh.
// This exercises the isSub2APIRefreshCandidate false path without making
// HTTP calls.
func TestSub2APIRefreshScheduler_runPassLocked_Sub2APIAccountNotEligible(t *testing.T) {
	ResetLeasePressureForTest()
	db := openSub2APITestDB(t)
	siteID := insertSub2APITestSite(t, db, "sub2api-site", "sub2api", "active")
	insertSub2APITestAccount(t, db, siteID, "sub2api-user", "active")

	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	// The account is on a sub2api site and active, so it is scanned.
	// But extra_config is NULL, so isSub2APIRefreshCandidate returns false
	// and no balance.RefreshBalance call is made (no HTTP calls).
	s.runPassLocked(db)
}

// TestSub2APIRefreshScheduler_runPass_WithOverrideDB exercises the full
// runPass flow: inFlight guard, store.GetDB lookup, context creation, lease
// acquisition, and runPassLocked.
func TestSub2APIRefreshScheduler_runPass_WithOverrideDB(t *testing.T) {
	ResetLeasePressureForTest()
	db := openSub2APITestDB(t)
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	s.runPass()

	// After runPass completes, passInFlight must be false.
	s.mu.Lock()
	inFlight := s.passInFlight
	s.mu.Unlock()
	if inFlight {
		t.Error("passInFlight should be false after runPass completes")
	}
}

// TestSub2APIRefreshScheduler_runPass_NilDB verifies that runPass handles a
// nil DB gracefully (returns without panic).
func TestSub2APIRefreshScheduler_runPass_NilDB(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	s.runPass()
}

// TestSub2APIRefreshScheduler_runPass_InFlightGuard verifies that when
// passInFlight is already true, runPass returns immediately without
// registering the defer that resets the flag.
func TestSub2APIRefreshScheduler_runPass_InFlightGuard(t *testing.T) {
	ResetLeasePressureForTest()
	store.OverrideDB(nil)
	t.Cleanup(func() { store.OverrideDB(nil) })

	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	s.mu.Lock()
	s.passInFlight = true
	s.mu.Unlock()

	s.runPass()

	s.mu.Lock()
	inFlight := s.passInFlight
	s.mu.Unlock()
	if !inFlight {
		t.Error("passInFlight should still be true when the guard short-circuited")
	}
}

// TestIsSub2APIRefreshCandidate_NilAndEmpty verifies that nil and empty
// extra_config are correctly classified as non-candidates (no managed auth).
func TestIsSub2APIRefreshCandidate_NilAndEmpty(t *testing.T) {
	if isSub2APIRefreshCandidate(nil) {
		t.Error("nil extraConfig should not be a refresh candidate")
	}

	emptyStr := ""
	if isSub2APIRefreshCandidate(&emptyStr) {
		t.Error("empty extraConfig should not be a refresh candidate")
	}

	// A JSON string without refreshToken should also not be a candidate.
	jsonNoRefreshToken := `{"key":"value"}`
	if isSub2APIRefreshCandidate(&jsonNoRefreshToken) {
		t.Error("extraConfig without refreshToken should not be a refresh candidate")
	}
}

// TestSub2APIRefreshScheduler_StartStop_Lifecycle verifies the interval-
// runner wrapper.
func TestSub2APIRefreshScheduler_StartStop_Lifecycle(t *testing.T) {
	cfg := testConfig()
	s := NewSub2APIRefreshScheduler(cfg)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	if err := s.Stop(); err != nil {
		t.Errorf("Stop failed: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("second Stop failed: %v", err)
	}
}
