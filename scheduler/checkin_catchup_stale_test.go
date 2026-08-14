package scheduler

import (
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- issue #669: restart catch-up (per-account stale enqueue) ----
//
// selectStaleCheckinAccountIDs must return only enabled, active accounts on
// non-disabled sites whose last_checkin_at is nil/empty or older than the
// 24h cutoff. This complements the global "ranToday" catch-up for cases where
// some accounts ran today but others are stale (newly added, recovered after a
// transient failure, missed by an aborted prior run).

func setupStaleCatchUpDB(t *testing.T) *store.DB {
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

func insertStaleCatchUpSite(t *testing.T, db *store.DB, name, status string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'new-api', ?, ?, ?)",
		name, "https://"+name+".example.test", status, now, now,
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

func insertStaleCatchUpAccount(t *testing.T, db *store.DB, siteID int64, username, status, lastCheckinAt string, enabled bool) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	enabledArg := any(1)
	if !enabled {
		enabledArg = 0
	}
	res, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, last_checkin_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		siteID, username, "sk-"+username, status, enabledArg, nullableStr(lastCheckinAt), now, now,
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

// nullableStr returns the empty string as a SQL NULL (nil) and otherwise the
// literal value — sqlite stores '' and NULL distinctly, and the stale query
// treats both as "no checkin yet".
func nullableStr(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func TestSelectStaleCheckinAccountIDs_PicksNilAndOldSkipsRecentAndDisabled(t *testing.T) {
	db := setupStaleCatchUpDB(t)
	activeSite := insertStaleCatchUpSite(t, db, "active-site", "active")

	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	recent := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	veryOld := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)

	// Stale: nil last_checkin_at → must be picked up.
	nilID := insertStaleCatchUpAccount(t, db, activeSite, "nil-account", "active", "", true)
	// Stale: older than 24h → must be picked up.
	oldID := insertStaleCatchUpAccount(t, db, activeSite, "old-account", "active", veryOld, true)
	// Not stale: checked in recently → must be skipped.
	insertStaleCatchUpAccount(t, db, activeSite, "recent-account", "active", recent, true)
	// Not eligible: account disabled → must be skipped even though stale.
	insertStaleCatchUpAccount(t, db, activeSite, "disabled-account", "disabled", "", true)
	// Not eligible: checkin disabled → must be skipped even though stale.
	insertStaleCatchUpAccount(t, db, activeSite, "checkin-off-account", "active", "", false)
	// Stale: very old → must be picked up.
	staleID := insertStaleCatchUpAccount(t, db, activeSite, "very-old-account", "active", veryOld, true)

	// Disabled site: account must be skipped even if otherwise stale.
	disabledSite := insertStaleCatchUpSite(t, db, "disabled-site", "disabled")
	insertStaleCatchUpAccount(t, db, disabledSite, "disabled-site-account", "active", "", true)

	ids, err := selectStaleCheckinAccountIDs(db, cutoff)
	if err != nil {
		t.Fatalf("selectStaleCheckinAccountIDs: %v", err)
	}

	want := map[int64]bool{nilID: true, oldID: true, staleID: true}
	if len(ids) != len(want) {
		t.Fatalf("got %d ids %v, want %d %v", len(ids), ids, len(want), keysOf(want))
	}
	for _, id := range ids {
		if !want[id] {
			t.Errorf("unexpected stale id %d (want %v)", id, keysOf(want))
		}
	}
}

func TestSelectStaleCheckinAccountIDs_EmptyWhenAllRecent(t *testing.T) {
	db := setupStaleCatchUpDB(t)
	siteID := insertStaleCatchUpSite(t, db, "all-recent-site", "active")
	recent := time.Now().Add(-30 * time.Minute).UTC().Format(time.RFC3339)
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	insertStaleCatchUpAccount(t, db, siteID, "a", "active", recent, true)
	insertStaleCatchUpAccount(t, db, siteID, "b", "active", recent, true)

	ids, err := selectStaleCheckinAccountIDs(db, cutoff)
	if err != nil {
		t.Fatalf("selectStaleCheckinAccountIDs: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("got %d stale ids %v, want none (all recent)", len(ids), ids)
	}
}

func keysOf(m map[int64]bool) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
