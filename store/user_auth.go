package store

import (
	"context"
	"database/sql"
	"time"
)

// UserAuthStore persists opaque credential digests, never plaintext passwords,
// activation codes or session tokens. No method returns these to an HTTP DTO.
type UserAuthStore struct{ db *DB }

func NewUserAuthStore(db *DB) *UserAuthStore { return &UserAuthStore{db: db} }

type UserCredential struct {
	UserID       int64  `db:"user_id"`
	PasswordHash string `db:"password_hash"`
	Version      string `db:"version"`
}

type UserSession struct {
	TokenHash string `db:"token_hash"`
	UserID    int64  `db:"user_id"`
	ExpiresAt string `db:"expires_at"`
}

func userAuthTime(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000000000Z") }

func (s *UserAuthStore) available() error {
	if s == nil || s.db == nil {
		return ErrGatewayUserUnavailable
	}
	return nil
}

// IssueActivation also resets an existing login. Its transaction serializes
// with password changes and invalidates old login attempts by version.
func (s *UserAuthStore) IssueActivation(ctx context.Context, userID int64, hash, version string, now, expires time.Time) error {
	if err := s.available(); err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id int64
	if err := tx.GetContext(ctx, &id, s.db.Rebind(`SELECT u.id FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id WHERE u.id=? AND u.enabled=TRUE AND g.enabled=TRUE`), userID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO gateway_user_credentials (user_id,password_hash,version,activation_hash,activation_expires_at,updated_at)
		VALUES (?,'',?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET password_hash='',version=excluded.version,
		activation_hash=excluded.activation_hash,activation_expires_at=excluded.activation_expires_at,updated_at=excluded.updated_at`), userID, version, hash, userAuthTime(expires), userAuthTime(now))
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM gateway_user_sessions WHERE user_id=?`), userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserAuthStore) Credential(ctx context.Context, username string) (UserCredential, error) {
	var row UserCredential
	if err := s.available(); err != nil {
		return row, err
	}
	err := s.db.GetContext(ctx, &row, `SELECT c.user_id,c.password_hash,c.version FROM gateway_user_credentials c
		JOIN gateway_users u ON u.id=c.user_id JOIN gateway_user_groups g ON g.id=u.group_id
		WHERE u.username=? AND u.enabled=TRUE AND g.enabled=TRUE`, username)
	return row, err
}

func (s *UserAuthStore) Activate(ctx context.Context, username, codeHash, passwordHash, version string, now time.Time) error {
	if err := s.available(); err != nil {
		return err
	}
	// UPDATE consumes the code atomically; concurrent/replayed activation loses.
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE gateway_user_credentials SET password_hash=?,version=?,activation_hash=NULL,activation_expires_at=NULL,updated_at=?
		WHERE activation_hash=? AND activation_expires_at>? AND user_id IN
		(SELECT u.id FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id WHERE u.username=? AND u.enabled=TRUE AND g.enabled=TRUE)`), passwordHash, version, userAuthTime(now), codeHash, userAuthTime(now), username)
	return authChanged(result, err)
}

func authChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *UserAuthStore) ChangePassword(ctx context.Context, userID int64, oldVersion, passwordHash, version string, now time.Time) error {
	if err := s.available(); err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, s.db.Rebind(`UPDATE gateway_user_credentials SET password_hash=?,version=?,activation_hash=NULL,activation_expires_at=NULL,updated_at=? WHERE user_id=? AND version=?`), passwordHash, version, userAuthTime(now), userID, oldVersion)
	if err := authChanged(result, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.db.Rebind(`DELETE FROM gateway_user_sessions WHERE user_id=?`), userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *UserAuthStore) CreateSession(ctx context.Context, credential UserCredential, hash string, now, expires, idle time.Time) error {
	if err := s.available(); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM gateway_user_sessions WHERE expires_at<=? OR idle_expires_at<=?`), userAuthTime(now), userAuthTime(now)); err != nil {
		return err
	}
	// A single conditional INSERT refuses login verified against credentials
	// concurrently changed/reset, including after expensive password comparison.
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO gateway_user_sessions(token_hash,user_id,credential_version,created_at,last_seen_at,expires_at,idle_expires_at)
		SELECT ?,c.user_id,c.version,?,?,?,? FROM gateway_user_credentials c
		JOIN gateway_users u ON u.id=c.user_id JOIN gateway_user_groups g ON g.id=u.group_id
		WHERE c.user_id=? AND c.version=? AND c.password_hash<>'' AND u.enabled=TRUE AND g.enabled=TRUE`), hash, userAuthTime(now), userAuthTime(now), userAuthTime(expires), userAuthTime(idle), credential.UserID, credential.Version)
	return authChanged(result, err)
}

func (s *UserAuthStore) ValidateSession(ctx context.Context, hash string, now time.Time, idleTTL time.Duration) (UserSession, error) {
	var row UserSession
	if err := s.available(); err != nil {
		return row, err
	}
	err := s.db.GetContext(ctx, &row, `UPDATE gateway_user_sessions SET last_seen_at=?,idle_expires_at=?
		WHERE token_hash=? AND expires_at>? AND idle_expires_at>? AND EXISTS
		(SELECT 1 FROM gateway_user_credentials c JOIN gateway_users u ON u.id=c.user_id JOIN gateway_user_groups g ON g.id=u.group_id
		WHERE c.user_id=gateway_user_sessions.user_id AND c.version=gateway_user_sessions.credential_version AND u.enabled=TRUE AND g.enabled=TRUE)
		RETURNING token_hash,user_id,expires_at`, userAuthTime(now), userAuthTime(now.Add(idleTTL)), hash, userAuthTime(now), userAuthTime(now))
	return row, err
}

func (s *UserAuthStore) RevokeSession(ctx context.Context, hash string) error {
	if err := s.available(); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM gateway_user_sessions WHERE token_hash=?`), hash)
	return err
}
