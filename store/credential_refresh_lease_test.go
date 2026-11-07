package store

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
)

func TestCredentialRefreshLeaseFencesTakeoverAndCancellation(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, err := Open(dialect, dsn, false)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if dialect == DialectPostgres {
				pgtest.Reset(t, db.DB)
			}
			if err := AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			first, err := AcquireCredentialRefreshLease(context.Background(), db.DB, "direct", 1)
			if err != nil {
				t.Fatal(err)
			}
			defer first.Release()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := AcquireCredentialRefreshLease(ctx, db.DB, "direct", 1); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled waiter did not preserve context cancellation: %v", err)
			}
			if _, err := db.Exec(`UPDATE credential_refresh_leases SET expires_at=0 WHERE lease_key='direct:1'`); err != nil {
				t.Fatal(err)
			}
			second, err := AcquireCredentialRefreshLease(context.Background(), db.DB, "direct", 1)
			if err != nil {
				t.Fatal(err)
			}
			defer second.Release()
			if tx, err := first.BeginFencedTx(context.Background()); !errors.Is(err, ErrCredentialRefreshLeaseLost) {
				if tx != nil {
					_ = tx.Rollback()
				}
				t.Fatal("expired owner retained publication authority")
			}
			first.Release()
			tx, err := second.BeginFencedTx(context.Background())
			if err != nil {
				t.Fatal("old owner removed the new lease")
			}
			_ = tx.Rollback()
			for _, invalid := range []string{"", "future", "direct:1"} {
				if _, err := AcquireCredentialRefreshLease(context.Background(), db.DB, invalid, 1); err == nil {
					t.Fatal("invalid lease namespace accepted")
				}
			}
		})
	}
}

func TestCredentialRefreshLeasesAreTransientOwnership(t *testing.T) {
	const table = "credential_refresh_leases"
	if !slices.Contains(SchemaTableNames(), table) || !slices.Contains(FactoryResetTableNames(), table) {
		t.Fatal("refresh ownership must be created and cleared by the schema owner")
	}
	if slices.Contains(AllTableNames(), table) || slices.Contains(BackupTableNames(), table) {
		t.Fatal("active refresh owners must not be migrated or imported")
	}
	if migrationExcludedTables[table] == "" || backupExcludedTables[table] == "" {
		t.Fatal("refresh ownership exclusion needs a recorded restore boundary")
	}
}

func TestCredentialRefreshLeaseReleaseReportsStorageFailure(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(buildCredentialRefreshLeasesDDL(DialectSQLite)); err != nil {
		t.Fatal(err)
	}
	lease, err := AcquireCredentialRefreshLease(context.Background(), db.DB, "account", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE credential_refresh_leases`); err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); !errors.Is(err, ErrCredentialRefreshLeaseLost) {
		t.Fatal("release failure was discarded")
	}
}
