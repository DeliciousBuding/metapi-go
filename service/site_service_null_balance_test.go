package service

import (
	"testing"
)

// TestListSites_NullBalanceDoesNotError guards the /api/sites 500 regression:
// an account with a NULL balance (migrated from the TS version, or never
// refreshed) must not abort ListSites. A site whose accounts all have
// unknown (NULL) balances reports totalBalance=nil (JSON null) — the
// frontend renders "—", never "$0.00".
func TestListSites_NullBalanceDoesNotError(t *testing.T) {
	db := openTestDB(t)

	siteID := createTestSite(t, db, "Null Balance Site", "https://api.example.com", "openai")
	accountID := createTestAccount(t, db, siteID, strPtr("null-balance-user"), "sk-test")

	if _, err := db.Exec("UPDATE accounts SET balance = NULL WHERE id = ?", accountID); err != nil {
		t.Fatalf("set balance NULL: %v", err)
	}

	sites, err := ListSites(db.DB)
	if err != nil {
		t.Fatalf("ListSites with NULL balance account: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}

	if sites[0]["totalBalance"] != nil {
		t.Fatalf("totalBalance = %v (%T), want nil: unknown balance must not render as $0", sites[0]["totalBalance"], sites[0]["totalBalance"])
	}
}

// TestListSites_NullBalanceWithSiblingSumsCorrectly ensures a NULL balance
// contributes nothing while a sibling account's balance still counts.
func TestListSites_NullBalanceWithSiblingSumsCorrectly(t *testing.T) {
	db := openTestDB(t)

	siteID := createTestSite(t, db, "Mixed Balance Site", "https://api.example.com", "openai")
	nullAccount := createTestAccount(t, db, siteID, strPtr("null-balance-user"), "sk-a")
	normalAccount := createTestAccount(t, db, siteID, strPtr("normal-user"), "sk-b")

	if _, err := db.Exec("UPDATE accounts SET balance = NULL WHERE id = ?", nullAccount); err != nil {
		t.Fatalf("set NULL account balance NULL: %v", err)
	}
	if _, err := db.Exec("UPDATE accounts SET balance = 12.5 WHERE id = ?", normalAccount); err != nil {
		t.Fatalf("set normal account balance: %v", err)
	}

	sites, err := ListSites(db.DB)
	if err != nil {
		t.Fatalf("ListSites with mixed balances: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("expected 1 site, got %d", len(sites))
	}

	tb, ok := sites[0]["totalBalance"].(float64)
	if !ok {
		t.Fatalf("totalBalance type = %T, want float64", sites[0]["totalBalance"])
	}
	if tb != 12.5 {
		t.Errorf("totalBalance = %v, want 12.5 (NULL contributes nothing, sibling counted)", tb)
	}
}
