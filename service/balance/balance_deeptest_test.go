package balance

// Deep-testing probe: Sub2API balance refresh must not zero stored balance
// (or re-activate expired accounts) when the upstream /api/v1/auth/me call
// fails transiently.
//
// platform.Sub2ApiAdapter.GetBalance swallows the auth/me fetch error and
// returns (&BalanceInfo{}, nil). RefreshBalance treats a nil error as a
// successful read and writes balance=0 / quota=0 / balanceUsed=0 into
// accounts — and its status updater even flips "expired" accounts back to
// "active". A network blip or a 5xx from the Sub2API site therefore silently
// corrupts stored balance data instead of surfacing an error.

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestProbe_RefreshBalance_Sub2ApiUpstreamFailureMustNotZeroBalance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a transient upstream failure on the balance endpoint.
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	siteRes, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, 'sub2api', 'active', ?, ?)",
		"Sub2API failing upstream", server.URL, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	siteID, err := siteRes.LastInsertId()
	if err != nil {
		t.Fatalf("site LastInsertId: %v", err)
	}
	// Account with a known stored balance and an expired status — both must
	// survive a failed refresh untouched.
	accountRes, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, balance, quota, status, checkin_enabled, created_at, updated_at) VALUES (?, ?, ?, 88.5, 100.0, 'expired', 0, ?, ?)",
		siteID, "sub2api-user", "session-token", now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	accountID, err := accountRes.LastInsertId()
	if err != nil {
		t.Fatalf("account LastInsertId: %v", err)
	}

	_, refreshErr := RefreshBalance(&config.Config{}, db.DB, accountID)

	var storedBalance float64
	var storedQuota float64
	var storedStatus string
	if err := db.Get(&storedBalance, "SELECT balance FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("reload balance: %v", err)
	}
	if err := db.Get(&storedQuota, "SELECT quota FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("reload quota: %v", err)
	}
	if err := db.Get(&storedStatus, "SELECT status FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("reload status: %v", err)
	}

	if storedBalance != 88.5 || storedQuota != 100.0 {
		t.Errorf("failed upstream balance refresh corrupted stored values: balance=%v quota=%v (want 88.5 / 100.0); refreshErr=%v — GetBalance swallowed the fetch error and RefreshBalance wrote the empty BalanceInfo",
			storedBalance, storedQuota, refreshErr)
	}
	if storedStatus != "expired" {
		t.Errorf("failed upstream balance refresh changed status from expired to %q (refresh-based reactivation must require a real successful read)", storedStatus)
	}
	if refreshErr == nil {
		t.Errorf("RefreshBalance returned nil error even though the upstream /api/v1/auth/me call failed with HTTP 500")
	}
}
