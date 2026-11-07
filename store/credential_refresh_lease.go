package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrCredentialRefreshLeaseLost = errors.New("credential refresh lease lost")

func buildCredentialRefreshLeasesDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS credential_refresh_leases (
		lease_key TEXT PRIMARY KEY,
		owner TEXT NOT NULL,
		expires_at BIGINT NOT NULL
	)`
}

// CredentialRefreshLease coordinates one persisted credential record across
// database handles/processes. It is not a refresh-token-family identity.
type CredentialRefreshLease struct {
	db          *sqlx.DB
	key, owner  string
	databaseNow string
}

func AcquireCredentialRefreshLease(ctx context.Context, db *sqlx.DB, scope string, id int64) (*CredentialRefreshLease, error) {
	if db == nil || id <= 0 || scope != "account" && scope != "direct" {
		return nil, ErrCredentialRefreshLeaseLost
	}
	clock := ""
	switch db.DriverName() {
	case "sqlite":
		clock = `CAST((julianday('now')-2440587.5)*86400000 AS BIGINT)`
	case "pgx":
		clock = `CAST(EXTRACT(EPOCH FROM clock_timestamp())*1000 AS BIGINT)`
	default:
		return nil, ErrCredentialRefreshLeaseLost
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, ErrCredentialRefreshLeaseLost
	}
	lease := &CredentialRefreshLease{db: db, key: fmt.Sprintf("%s:%d", scope, id), owner: hex.EncodeToString(nonce), databaseNow: clock}
	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		query := `INSERT INTO credential_refresh_leases(lease_key,owner,expires_at) VALUES(?,?,` + clock + `+180000)
			ON CONFLICT(lease_key) DO UPDATE SET owner=excluded.owner,expires_at=excluded.expires_at
			WHERE credential_refresh_leases.expires_at<=` + clock
		result, err := db.ExecContext(waitCtx, db.Rebind(query), lease.key, lease.owner)
		if err != nil {
			if waitCtx.Err() != nil {
				return nil, waitCtx.Err()
			}
			return nil, ErrCredentialRefreshLeaseLost
		}
		count, err := result.RowsAffected()
		if err != nil {
			return nil, ErrCredentialRefreshLeaseLost
		}
		if count == 1 {
			return lease, nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			return nil, waitCtx.Err()
		case <-timer.C:
		}
	}
}

// BeginFencedTx locks the still-owned lease row until the credential CAS commits
// or rolls back. A late owner cannot publish after expiry or takeover.
func (l *CredentialRefreshLease) BeginFencedTx(ctx context.Context) (*sqlx.Tx, error) {
	tx, err := l.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrCredentialRefreshLeaseLost
	}
	query := `UPDATE credential_refresh_leases SET owner=owner WHERE lease_key=? AND owner=? AND expires_at>` + l.databaseNow
	result, err := tx.ExecContext(ctx, l.db.Rebind(query), l.key, l.owner)
	if err != nil {
		_ = tx.Rollback()
		return nil, ErrCredentialRefreshLeaseLost
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		_ = tx.Rollback()
		return nil, ErrCredentialRefreshLeaseLost
	}
	return tx, nil
}

func (l *CredentialRefreshLease) Release() error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := l.db.ExecContext(ctx, l.db.Rebind(`DELETE FROM credential_refresh_leases WHERE lease_key=? AND owner=?`), l.key, l.owner)
	if err != nil {
		return ErrCredentialRefreshLeaseLost
	}
	return nil
}
