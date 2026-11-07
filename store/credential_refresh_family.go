package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/jmoiron/sqlx"
)

var (
	ErrRefreshFamilyChanged  = errors.New("credential refresh family changed")
	ErrRefreshUncertain      = errors.New("credential refresh consumption is unconfirmed")
	ErrInvalidRefreshJournal = errors.New("invalid credential refresh journal")
)

func buildCredentialRefreshFamiliesDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS credential_refresh_families (
		id TEXT PRIMARY KEY, provider TEXT NOT NULL, current_fingerprint TEXT NOT NULL,
		generation BIGINT NOT NULL DEFAULT 0, state TEXT NOT NULL DEFAULT 'active',
		attempt_id TEXT NOT NULL DEFAULT '',
		result_scope TEXT NOT NULL DEFAULT '', result_id BIGINT NOT NULL DEFAULT 0,
		CHECK(generation >= 0), CHECK(state IN ('active','refreshing','uncertain')),
		CHECK(result_scope IN ('','account','direct')), CHECK(result_id >= 0)
	)`
}

// Portable backups are untrusted scalar maps. Validate the irreversible
// journal before SQLite's INSERT OR IGNORE could silently skip a CHECK failure.
// Existing rows remain authoritative through the importer's conflict policy.
func NormalizeCredentialRefreshJournalRow(table string, row map[string]any) error {
	var fingerprints []string
	switch table {
	case "credential_refresh_families":
		fingerprints = []string{"id", "current_fingerprint"}
	case "credential_refresh_aliases":
		fingerprints = []string{"fingerprint", "family_id"}
	default:
		return ErrInvalidRefreshJournal
	}
	for _, key := range fingerprints {
		value, ok := row[key].(string)
		if !ok || !validRefreshFingerprint(value) {
			return ErrInvalidRefreshJournal
		}
	}
	generation, ok := journalInteger(row["generation"])
	if !ok {
		return ErrInvalidRefreshJournal
	}
	if table == "credential_refresh_aliases" {
		switch value := row["retired"].(type) {
		case bool:
			return nil
		case float64:
			if value == 0 || value == 1 {
				// SQLite exports BOOLEAN columns as 0/1; pgx's binary
				// parameter encoder requires an actual Go bool.
				row["retired"] = value == 1
				return nil
			}
		}
		return ErrInvalidRefreshJournal
	}
	provider, ok := row["provider"].(string)
	var scope []string
	if !ok || json.Unmarshal([]byte(provider), &scope) != nil || len(scope) != 2 || scope[0] == "" || scope[1] == "" || len(provider) > 1024 {
		return ErrInvalidRefreshJournal
	}
	state, ok := row["state"].(string)
	if !ok || state != "active" && state != "refreshing" && state != "uncertain" {
		return ErrInvalidRefreshJournal
	}
	attempt, ok := row["attempt_id"].(string)
	if !ok || state != "active" && attempt == "" || generation == 0 && state == "active" && attempt != "" {
		return ErrInvalidRefreshJournal
	}
	if attempt != "" {
		data, err := hex.DecodeString(attempt)
		if err != nil || len(data) != 16 || len(attempt) != 32 {
			return ErrInvalidRefreshJournal
		}
	}
	resultID, ok := journalInteger(row["result_id"])
	resultScope, scopeOK := row["result_scope"].(string)
	if !ok || !scopeOK || resultScope != "" && resultScope != "account" && resultScope != "direct" {
		return ErrInvalidRefreshJournal
	}
	if generation == 0 && (resultID != 0 || resultScope != "") || generation > 0 && (resultID == 0 || resultScope == "") {
		return ErrInvalidRefreshJournal
	}
	return nil
}

func journalInteger(value any) (int64, bool) {
	n, ok := value.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 9007199254740991 || math.Trunc(n) != n {
		return 0, false
	}
	return int64(n), true
}

func buildCredentialRefreshAliasesDDL(string) string {
	return `CREATE TABLE IF NOT EXISTS credential_refresh_aliases (
		fingerprint TEXT PRIMARY KEY, family_id TEXT NOT NULL REFERENCES credential_refresh_families(id),
		generation BIGINT NOT NULL DEFAULT 0, retired BOOLEAN NOT NULL DEFAULT FALSE,
		CHECK(generation >= 0)
	)`
}

// Only irreversible, issuer-scoped identities enter the journal. Tokens stay
// in accounts/upstream_credentials, never in this coordination state.
func OAuthRefreshFingerprint(provider, token string) string {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(token) == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(strings.TrimSpace(provider) + "\x00" + strings.TrimSpace(token)))
	return hex.EncodeToString(hash[:])
}

func validRefreshFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

type CredentialRefreshFamily struct {
	db                 *sqlx.DB
	lease              *CredentialRefreshLease
	ID                 string `db:"id"`
	Provider           string `db:"provider"`
	Fingerprint        string `db:"-"`
	CurrentFingerprint string `db:"current_fingerprint"`
	Generation         int64  `db:"generation"`
	State              string `db:"state"`
	AttemptID          string `db:"attempt_id"`
	ResultScope        string `db:"result_scope"`
	ResultID           int64  `db:"result_id"`
	Retired            bool   `db:"retired"`
	started            bool
}

// The issuer owner, not channel settings or portable metadata, determines
// whether an exchange consumes its input. Reusable inputs can retry after an
// interrupted exchange; rotating OAuth refresh tokens must retain the intent.
type CredentialRefreshMode uint8

const (
	CredentialRefreshConsumable CredentialRefreshMode = iota
	CredentialRefreshReusable
)

func AcquireCredentialRefreshFamily(ctx context.Context, db *sqlx.DB, provider, fingerprint string, mode CredentialRefreshMode) (*CredentialRefreshFamily, error) {
	if db == nil || provider == "" || !validRefreshFingerprint(fingerprint) || mode != CredentialRefreshConsumable && mode != CredentialRefreshReusable {
		return nil, ErrRefreshFamilyChanged
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, ErrRefreshFamilyChanged
	}
	defer tx.Rollback()
	var familyID string
	// Write before reading: upgrading a stale SQLite read snapshot to a
	// writer can otherwise fail when two first-use records race to initialize.
	if _, err = tx.ExecContext(ctx, db.Rebind(`INSERT INTO credential_refresh_families(id,provider,current_fingerprint) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING`), fingerprint, provider, fingerprint); err != nil {
		return nil, ErrRefreshFamilyChanged
	}
	if _, err = tx.ExecContext(ctx, db.Rebind(`INSERT INTO credential_refresh_aliases(fingerprint,family_id) VALUES(?,?) ON CONFLICT(fingerprint) DO NOTHING`), fingerprint, fingerprint); err != nil {
		return nil, ErrRefreshFamilyChanged
	}
	if err = tx.GetContext(ctx, &familyID, db.Rebind(`SELECT family_id FROM credential_refresh_aliases WHERE fingerprint=?`), fingerprint); err != nil {
		return nil, ErrRefreshFamilyChanged
	}
	// A rotation may already own this alias. Remove only our temporary orphan.
	if familyID != fingerprint {
		if _, err = tx.ExecContext(ctx, db.Rebind(`DELETE FROM credential_refresh_families WHERE id=? AND NOT EXISTS(SELECT 1 FROM credential_refresh_aliases WHERE family_id=?)`), fingerprint, fingerprint); err != nil {
			return nil, ErrRefreshFamilyChanged
		}
	}
	if err := tx.Commit(); err != nil || !validRefreshFingerprint(familyID) {
		return nil, ErrRefreshFamilyChanged
	}
	lease, err := acquireCredentialRefreshLeaseKey(ctx, db, "family:"+familyID)
	if err != nil {
		return nil, err
	}
	f := &CredentialRefreshFamily{db: db, lease: lease, Fingerprint: fingerprint}
	if err := db.GetContext(ctx, f, db.Rebind(`SELECT f.id,f.provider,f.current_fingerprint,f.generation,f.state,f.attempt_id,f.result_scope,f.result_id,a.retired
		FROM credential_refresh_families f JOIN credential_refresh_aliases a ON a.family_id=f.id WHERE a.fingerprint=?`), fingerprint); err != nil ||
		f.Provider != provider || !validRefreshFingerprint(f.CurrentFingerprint) {
		_ = lease.Release()
		return nil, ErrRefreshFamilyChanged
	}
	if f.State != "active" && mode == CredentialRefreshReusable && !f.Retired && f.CurrentFingerprint == fingerprint {
		if err := f.resumeReusable(ctx); err != nil {
			_ = lease.Release()
			return nil, err
		}
	}
	if f.State != "active" {
		_ = lease.Release()
		// Expiry can transfer a lease, not undo a potentially consumed RT.
		return nil, ErrRefreshUncertain
	}
	if !f.Retired && f.CurrentFingerprint != fingerprint {
		_ = lease.Release()
		return nil, ErrRefreshFamilyChanged
	}
	return f, nil
}

func (f *CredentialRefreshFamily) resumeReusable(ctx context.Context) error {
	tx, err := f.currentTx(ctx, f.State)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET state='active',attempt_id='' WHERE id=?`), f.ID); err != nil {
		return ErrRefreshUncertain
	}
	if err := tx.Commit(); err != nil {
		return ErrRefreshUncertain
	}
	f.State, f.AttemptID = "active", ""
	return nil
}

func (f *CredentialRefreshFamily) Release() error { return f.lease.Release() }

func (f *CredentialRefreshFamily) currentTx(ctx context.Context, state string) (*sqlx.Tx, error) {
	tx, err := f.lease.BeginFencedTx(ctx)
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET generation=generation
		WHERE id=? AND generation=? AND current_fingerprint=? AND state=? AND attempt_id=?`),
		f.ID, f.Generation, f.CurrentFingerprint, state, f.AttemptID)
	if err != nil {
		_ = tx.Rollback()
		return nil, ErrRefreshFamilyChanged
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		_ = tx.Rollback()
		return nil, ErrRefreshFamilyChanged
	}
	return tx, nil
}

// Begin is committed before any token request. The intent survives a process
// crash and an expired lease, without storing any request or credential body.
func (f *CredentialRefreshFamily) Begin(ctx context.Context) error {
	if f.Retired || f.Fingerprint != f.CurrentFingerprint || f.started {
		return ErrRefreshFamilyChanged
	}
	tx, err := f.currentTx(ctx, "active")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET state='refreshing',attempt_id=? WHERE id=?`), f.lease.owner, f.ID); err != nil {
		return ErrRefreshFamilyChanged
	}
	if err := tx.Commit(); err != nil {
		return ErrRefreshUncertain
	}
	f.State, f.AttemptID, f.started = "refreshing", f.lease.owner, true
	return nil
}

func (f *CredentialRefreshFamily) BeginPublication(ctx context.Context) (*sqlx.Tx, error) {
	if !f.started {
		return nil, ErrRefreshFamilyChanged
	}
	return f.currentTx(ctx, "refreshing")
}

func (f *CredentialRefreshFamily) BeginReuse(ctx context.Context) (*sqlx.Tx, error) {
	return f.currentTx(ctx, "active")
}

func (f *CredentialRefreshFamily) PointResultTx(ctx context.Context, tx *sqlx.Tx, scope string, id int64) error {
	if (scope != "account" && scope != "direct") || id <= 0 {
		return ErrRefreshFamilyChanged
	}
	_, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET result_scope=?,result_id=?,attempt_id=? WHERE id=?`), scope, id, f.lease.owner, f.ID)
	if err != nil {
		return ErrRefreshFamilyChanged
	}
	return nil
}

// FinishTx publishes the new namespace in the same transaction as credentials.
// An already-owned/retired alias is never silently rebound to another family.
func (f *CredentialRefreshFamily) FinishTx(ctx context.Context, tx *sqlx.Tx, nextFingerprint, scope string, id int64) error {
	if !validRefreshFingerprint(nextFingerprint) || !f.started || (scope != "account" && scope != "direct") || id <= 0 {
		return ErrRefreshFamilyChanged
	}
	if nextFingerprint != f.Fingerprint {
		result, err := tx.ExecContext(ctx, f.db.Rebind(`INSERT INTO credential_refresh_aliases(fingerprint,family_id,generation,retired) VALUES(?,?,?,?) ON CONFLICT(fingerprint) DO NOTHING`), nextFingerprint, f.ID, f.Generation+1, false)
		if err != nil {
			return ErrRefreshFamilyChanged
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return ErrRefreshFamilyChanged
		}
		if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_aliases SET retired=? WHERE fingerprint=? AND family_id=?`), true, f.Fingerprint, f.ID); err != nil {
			return ErrRefreshFamilyChanged
		}
	} else {
		if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_aliases SET generation=? WHERE fingerprint=? AND family_id=?`), f.Generation+1, f.Fingerprint, f.ID); err != nil {
			return ErrRefreshFamilyChanged
		}
	}
	if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET current_fingerprint=?,generation=?,state='active',attempt_id=?,result_scope=?,result_id=? WHERE id=?`), nextFingerprint, f.Generation+1, f.lease.owner, scope, id, f.ID); err != nil {
		return ErrRefreshFamilyChanged
	}
	return nil
}

// active state's attempt_id retains the last publisher nonce, rather than an
// in-flight intent. It proves this exact owner's commit when an acknowledgement
// is lost; a new owner's Begin overwrites it before consuming another token.
func (f *CredentialRefreshFamily) PublicationMatches(ctx context.Context, nextFingerprint, scope string, id int64, reuse bool) (bool, error) {
	generation := f.Generation
	if !reuse {
		generation++
	}
	var count int
	err := f.db.GetContext(ctx, &count, f.db.Rebind(`SELECT COUNT(*) FROM credential_refresh_families
		WHERE id=? AND state='active' AND generation=? AND current_fingerprint=? AND attempt_id=? AND result_scope=? AND result_id=?`),
		f.ID, generation, nextFingerprint, f.lease.owner, scope, id)
	if err != nil {
		return false, ErrRefreshFamilyChanged
	}
	return count == 1, nil
}

// Only a proved rejection/not-sent result can release the durable intent.
func (f *CredentialRefreshFamily) Fail(ctx context.Context, definite bool) error {
	if !f.started {
		return nil
	}
	tx, err := f.currentTx(ctx, "refreshing")
	if err != nil {
		return err
	}
	defer tx.Rollback()
	state, attempt := "uncertain", f.AttemptID
	if definite {
		state, attempt = "active", ""
	}
	if _, err := tx.ExecContext(ctx, f.db.Rebind(`UPDATE credential_refresh_families SET state=?,attempt_id=? WHERE id=?`), state, attempt, f.ID); err != nil {
		return ErrRefreshUncertain
	}
	if err := tx.Commit(); err != nil {
		return ErrRefreshUncertain
	}
	f.State, f.AttemptID, f.started = state, attempt, !definite
	return nil
}
