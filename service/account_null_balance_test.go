package service

import "testing"

// TestGetAccountByID_NullBalanceDoesNotError guards the NULL-scan crash on the
// single-account load path: an account whose balance/balance_used/quota/
// value_score columns are NULL (migrated from TS, or never refreshed) must scan
// into nil pointers, not fail with "converting NULL to float64".
func TestGetAccountByID_NullBalanceDoesNotError(t *testing.T) {
	db := openTestDB(t)

	siteID := createTestSite(t, db, "Null Balance Site", "https://api.example.com", "openai")
	accountID := createTestAccount(t, db, siteID, strPtr("null-balance-user"), "sk-test")

	if _, err := db.Exec(`
		UPDATE accounts SET balance = NULL, balance_used = NULL, quota = NULL, value_score = NULL
		WHERE id = ?`, accountID); err != nil {
		t.Fatalf("null the numeric columns: %v", err)
	}

	account, err := GetAccountByID(db.DB, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID with NULL numeric columns: %v", err)
	}
	if account.Balance != nil {
		t.Errorf("Balance = %v, want nil (NULL preserved)", *account.Balance)
	}
	if account.BalanceUsed != nil {
		t.Errorf("BalanceUsed = %v, want nil", *account.BalanceUsed)
	}
	if account.Quota != nil {
		t.Errorf("Quota = %v, want nil", *account.Quota)
	}
	if account.ValueScore != nil {
		t.Errorf("ValueScore = %v, want nil", *account.ValueScore)
	}
	// OrZero helpers coerce the unknown values to 0 for numeric callers.
	if account.BalanceOrZero() != 0 || account.QuotaOrZero() != 0 || account.ValueScoreOrZero() != 0 {
		t.Errorf("OrZero helpers should return 0 for nil fields")
	}
}

// TestGetAccountWithSiteByID_NullBalanceDoesNotError covers the balance-refresh
// load path (the JOIN + inline-struct scan) with NULL numeric columns.
func TestGetAccountWithSiteByID_NullBalanceDoesNotError(t *testing.T) {
	db := openTestDB(t)

	siteID := createTestSite(t, db, "Null Balance Site", "https://api.example.com", "openai")
	accountID := createTestAccount(t, db, siteID, strPtr("null-balance-user"), "sk-test")

	if _, err := db.Exec(`
		UPDATE accounts SET balance = NULL, balance_used = NULL, quota = NULL, value_score = NULL
		WHERE id = ?`, accountID); err != nil {
		t.Fatalf("null the numeric columns: %v", err)
	}

	aws, err := GetAccountWithSiteByID(db.DB, accountID)
	if err != nil {
		t.Fatalf("GetAccountWithSiteByID with NULL numeric columns: %v", err)
	}
	if aws.Account.Balance != nil {
		t.Errorf("Balance = %v, want nil", *aws.Account.Balance)
	}
	if aws.Account.ValueScore != nil {
		t.Errorf("ValueScore = %v, want nil", *aws.Account.ValueScore)
	}
}
