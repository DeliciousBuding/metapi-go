package admin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestRefreshFamilyPortableRestoreRetainsUncertainty(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			source := catalogTestDB(t, dialect, ":memory:")
			scope := `["codex","synthetic-client"]`
			fingerprint := store.OAuthRefreshFingerprint(scope, "synthetic-restore-token")
			family, err := store.AcquireCredentialRefreshFamily(context.Background(), source.DB, scope, fingerprint, store.CredentialRefreshConsumable)
			if err != nil {
				t.Fatal(err)
			}
			if err := family.Begin(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := family.Fail(context.Background(), false); err != nil {
				t.Fatal(err)
			}
			if err := family.Release(); err != nil {
				t.Fatal(err)
			}
			for _, exportType := range []string{"all", "accounts"} {
				payload, err := backupsvc.BuildPayload(source.DB, exportType)
				if err != nil {
					t.Fatal(err)
				}
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), "synthetic-restore-token") {
					t.Fatal("coordination journal leaked token material")
				}
				tables, err := decodeBackupImportBodyFrom(raw)
				if err != nil {
					t.Fatal(err)
				}
				if tables["credential_refresh_families"] == nil || tables["credential_refresh_aliases"] == nil {
					t.Fatal("irreversible facts were excluded from credential export")
				}
				// Test both dialect directions on independent targets. Resetting
				// the PostgreSQL fixture never clears the SQLite source snapshot.
				targetDialect := store.DialectSQLite
				if dialect == store.DialectSQLite {
					targetDialect = store.DialectPostgres
				}
				target := catalogTestDB(t, targetDialect, ":memory:")
				if _, err := importBackupTables(target.DB, tables, true); err != nil {
					t.Fatal(err)
				}
				if _, err := store.AcquireCredentialRefreshFamily(context.Background(), target.DB, scope, fingerprint, store.CredentialRefreshConsumable); !errors.Is(err, store.ErrRefreshUncertain) {
					t.Fatalf("restore lost uncertainty: %v", err)
				}
			}
		})
	}
}

func TestRefreshFamilyImportCannotDowngradeTargetFacts(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			scope := `["codex","synthetic-client"]`
			oldFingerprint := store.OAuthRefreshFingerprint(scope, "synthetic-old-token")
			nextFingerprint := store.OAuthRefreshFingerprint(scope, "synthetic-next-token")
			family, err := store.AcquireCredentialRefreshFamily(context.Background(), db.DB, scope, oldFingerprint, store.CredentialRefreshConsumable)
			if err != nil {
				t.Fatal(err)
			}
			if err := family.Begin(context.Background()); err != nil {
				t.Fatal(err)
			}
			tx, err := family.BeginPublication(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if err := family.FinishTx(context.Background(), tx, nextFingerprint, "direct", 1); err != nil {
				_ = tx.Rollback()
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := family.Release(); err != nil {
				t.Fatal(err)
			}
			next, err := store.AcquireCredentialRefreshFamily(context.Background(), db.DB, scope, nextFingerprint, store.CredentialRefreshConsumable)
			if err != nil {
				t.Fatal(err)
			}
			if err := next.Begin(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := next.Fail(context.Background(), false); err != nil {
				t.Fatal(err)
			}
			if err := next.Release(); err != nil {
				t.Fatal(err)
			}
			families, _ := json.Marshal([]map[string]any{{"id": oldFingerprint, "provider": scope, "current_fingerprint": oldFingerprint, "generation": 0, "state": "active", "attempt_id": "", "result_scope": "", "result_id": 0}})
			aliases, _ := json.Marshal([]map[string]any{{"fingerprint": oldFingerprint, "family_id": oldFingerprint, "generation": 0, "retired": false}})
			if _, err := importBackupTables(db.DB, map[string]json.RawMessage{"credential_refresh_families": families, "credential_refresh_aliases": aliases}, true); err != nil {
				t.Fatal(err)
			}
			var state, current string
			var generation int64
			if err := db.QueryRow(`SELECT state,current_fingerprint,generation FROM credential_refresh_families`).Scan(&state, &current, &generation); err != nil {
				t.Fatal(err)
			}
			var retired bool
			if err := db.Get(&retired, `SELECT retired FROM credential_refresh_aliases WHERE fingerprint=?`, oldFingerprint); err != nil {
				t.Fatal(err)
			}
			if state != "uncertain" || current != nextFingerprint || generation != 1 || !retired {
				t.Fatal("semi-trusted import downgraded target consumption facts")
			}
		})
	}
}

func TestRefreshFamilyImportRejectsMalformedJournal(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			scope := `["codex","synthetic-client"]`
			fp := store.OAuthRefreshFingerprint(scope, "synthetic-token")
			for _, change := range []map[string]any{{"generation": -1}, {"generation": 0.5}, {"provider": "not-a-client-scope"}, {"state": "future"}, {"id": nil}, {"attempt_id": "not-empty"}, {"result_scope": "future"}} {
				row := map[string]any{"id": fp, "provider": scope, "current_fingerprint": fp, "generation": 0, "state": "active", "attempt_id": "", "result_scope": "", "result_id": 0}
				for key, value := range change {
					row[key] = value
				}
				raw, _ := json.Marshal([]map[string]any{row})
				if _, err := importBackupTables(db.DB, map[string]json.RawMessage{"credential_refresh_families": raw}, true); err == nil {
					t.Fatal("malformed journal silently succeeded")
				}
			}
		})
	}
}
