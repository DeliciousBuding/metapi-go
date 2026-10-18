package store

import (
	"context"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"os"
	"testing"
)

func TestAccountPricingSnapshotBothDialects(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := ":memory:"
			if dialect == DialectPostgres {
				if os.Getenv("PG_TEST_DSN") == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				dsn = os.Getenv("PG_TEST_DSN")
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
			for _, q := range []string{`INSERT INTO sites (id,name,url,platform) VALUES (100,'Synthetic','https://synthetic.example','newapi')`, `INSERT INTO accounts (id,site_id,access_token) VALUES (100,100,'synthetic-only')`} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			if err := SaveAccountPricing(ctx, db.DB, AccountPricingSnapshot{AccountID: 100, Payload: `{"models":[]}`, FetchedAt: "2026-10-18T01:00:00Z", AttemptedAt: "2026-10-18T01:00:00Z"}); err != nil {
				t.Fatal(err)
			}
			if err := RecordAccountPricingFailure(ctx, db.DB, 100, "2026-10-18T02:00:00Z", "upstream_error"); err != nil {
				t.Fatal(err)
			}
			var got AccountPricingSnapshot
			if err := db.Get(&got, `SELECT * FROM account_pricing WHERE account_id=100`); err != nil {
				t.Fatal(err)
			}
			if got.Payload != `{"models":[]}` || got.FetchedAt != "2026-10-18T01:00:00Z" || got.LastError != "upstream_error" {
				t.Fatal(got)
			}
			if _, err := db.Exec(`DELETE FROM accounts WHERE id=100`); err != nil {
				t.Fatal(err)
			}
			var count int
			if err := db.Get(&count, `SELECT COUNT(*) FROM account_pricing`); err != nil || count != 0 {
				t.Fatalf("cascade count=%d err=%v", count, err)
			}
		})
	}
}
