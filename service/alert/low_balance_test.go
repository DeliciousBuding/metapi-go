package alert

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func setupAlertTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return db
}

func TestReportLowBalance_NoOpAboveThreshold(t *testing.T) {
	db := setupAlertTestDB(t)
	rt := &config.RuntimeSettings{AuthToken: "a", ProxyToken: "p"}
	ReportLowBalance(rt, db.DB, LowBalanceParams{
		AccountID: 1, Balance: 5.0, Threshold: 1.0,
	})
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM events WHERE type = 'balance'`); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 low-balance events above threshold, got %d", n)
	}
}

func TestReportLowBalance_FiresOnceThenDedups(t *testing.T) {
	db := setupAlertTestDB(t)
	rt := &config.RuntimeSettings{AuthToken: "a", ProxyToken: "p"}
	uname := "alice"
	site := "acme"

	call := func() {
		ReportLowBalance(rt, db.DB, LowBalanceParams{
			AccountID: 7, Username: &uname, SiteName: &site,
			Balance: 0.42, Threshold: 1.0,
		})
	}

	call()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM events WHERE type = 'balance' AND related_id = 7`); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 event after first fire, got %d", n)
	}

	// Second call within the 24h window must dedup — no new event.
	call()
	if err := db.Get(&n, `SELECT COUNT(*) FROM events WHERE type = 'balance' AND related_id = 7`); err != nil {
		t.Fatalf("count2: %v", err)
	}
	if n != 1 {
		t.Fatalf("dedup failed: expected 1 event, got %d", n)
	}
}
