package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func openImportTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestImportSites_CreatesIdempotently(t *testing.T) {
	db := openImportTestDB(t)

	items := []ImportSiteInput{
		{Name: "OpenAI", URL: "https://api.openai.com/v1"},
		{Name: "Anthropic", URL: "https://api.anthropic.com/v1"},
	}

	res, err := ImportSites(db.DB, items, ImportDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportSites: %v", err)
	}
	if res.Imported != 2 || res.Skipped != 0 || res.Failed != 0 {
		t.Fatalf("first run imported=%d skipped=%d failed=%d, want 2/0/0", res.Imported, res.Skipped, res.Failed)
	}

	// Re-run the same payload: everything is a duplicate no-op.
	res, err = ImportSites(db.DB, items, ImportDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportSites rerun: %v", err)
	}
	if res.Imported != 0 || res.Skipped != 2 || res.Failed != 0 {
		t.Fatalf("rerun imported=%d skipped=%d failed=%d, want 0/2/0", res.Imported, res.Skipped, res.Failed)
	}
}

func TestImportSites_DuplicateMergeAttachesAccounts(t *testing.T) {
	db := openImportTestDB(t)

	items := []ImportSiteInput{
		{Name: "OpenAI", URL: "https://api.openai.com/v1", Accounts: []ImportAccountInput{
			{Username: strPtr("ops"), AccessToken: "sk-import-1"},
		}},
	}
	res, err := ImportSites(db.DB, items, ImportDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportSites: %v", err)
	}
	if res.Imported != 1 {
		t.Fatalf("imported=%d want 1", res.Imported)
	}

	// Merge another account into the existing site.
	merge := []ImportSiteInput{
		{Name: "OpenAI", URL: "https://api.openai.com/v1", Accounts: []ImportAccountInput{
			{Username: strPtr("ops2"), AccessToken: "sk-import-2"},
		}},
	}
	res, err = ImportSites(db.DB, merge, ImportDuplicateMerge)
	if err != nil {
		t.Fatalf("ImportSites merge: %v", err)
	}
	if res.Imported != 1 || res.Results[0].Status != "merged" {
		t.Fatalf("merge imported=%d status=%q, want imported=1 merged", res.Imported, res.Results[0].Status)
	}

	var accountCount int
	if err := db.DB.Get(&accountCount, "SELECT COUNT(*) FROM accounts"); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if accountCount != 2 {
		t.Fatalf("accounts=%d want 2", accountCount)
	}
}

func TestImportSites_FailsOnUnknownPlatform(t *testing.T) {
	db := openImportTestDB(t)

	items := []ImportSiteInput{
		{Name: "Unknown", URL: "https://totally-unknown-host.invalid/v1"},
	}
	res, err := ImportSites(db.DB, items, ImportDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportSites: %v", err)
	}
	if res.Failed != 1 {
		t.Fatalf("failed=%d want 1 (results=%+v)", res.Failed, res.Results)
	}
}
