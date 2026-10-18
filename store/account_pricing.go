package store

import (
	"context"
	"github.com/jmoiron/sqlx"
)

// AccountPricingSnapshot contains only normalized public pricing, never credentials.
type AccountPricingSnapshot struct {
	AccountID   int64  `db:"account_id" json:"accountId"`
	Payload     string `db:"payload" json:"-"`
	FetchedAt   string `db:"fetched_at" json:"fetchedAt"`
	AttemptedAt string `db:"attempted_at" json:"attemptedAt"`
	LastError   string `db:"last_error" json:"lastError"`
}

func buildAccountPricingDDL(dialect string) string {
	return `CREATE TABLE IF NOT EXISTS account_pricing (
		account_id BIGINT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
		payload TEXT NOT NULL DEFAULT '',
		fetched_at TEXT NOT NULL DEFAULT '',
		attempted_at TEXT NOT NULL DEFAULT '',
		last_error TEXT NOT NULL DEFAULT ''
	)`
}

func SaveAccountPricing(ctx context.Context, db *sqlx.DB, row AccountPricingSnapshot) error {
	_, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO account_pricing (account_id,payload,fetched_at,attempted_at,last_error) VALUES (?,?,?,?,?)
	ON CONFLICT(account_id) DO UPDATE SET payload=excluded.payload,fetched_at=excluded.fetched_at,attempted_at=excluded.attempted_at,last_error=excluded.last_error`), row.AccountID, row.Payload, row.FetchedAt, row.AttemptedAt, row.LastError)
	return err
}

func RecordAccountPricingFailure(ctx context.Context, db *sqlx.DB, accountID int64, at, code string) error {
	_, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO account_pricing (account_id,attempted_at,last_error) VALUES (?,?,?)
	ON CONFLICT(account_id) DO UPDATE SET attempted_at=excluded.attempted_at,last_error=excluded.last_error`), accountID, at, code)
	return err
}
