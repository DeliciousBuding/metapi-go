package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// UserKey and UserRequest are explicit safe projections. Do not replace these
// with SELECT * or the administrator's credential-bearing records.
type UserKey struct {
	ID              int64    `db:"id" json:"id"`
	Name            string   `db:"name" json:"name"`
	KeyMasked       string   `db:"key_masked" json:"keyMasked"`
	Enabled         bool     `db:"enabled" json:"enabled"`
	ExpiresAt       *string  `db:"expires_at" json:"expiresAt"`
	SupportedModels []string `db:"-" json:"supportedModels"`
	MaxCost         *float64 `db:"max_cost" json:"maxCost"`
	MaxRequests     *int64   `db:"max_requests" json:"maxRequests"`
	UsedCost        float64  `db:"used_cost" json:"usedCost"`
	UsedRequests    int64    `db:"used_requests" json:"usedRequests"`
	CreatedAt       string   `db:"created_at" json:"createdAt"`
}

const userKeyColumns = `id,name,enabled,expires_at,max_cost,max_requests,COALESCE(used_cost,0),COALESCE(used_requests,0),COALESCE(created_at,''),key,COALESCE(supported_models,'[]')`

type rowScanner interface{ Scan(...any) error }

func scanUserKey(row rowScanner) (UserKey, error) {
	var item UserKey
	var key, models string
	err := row.Scan(&item.ID, &item.Name, &item.Enabled, &item.ExpiresAt, &item.MaxCost, &item.MaxRequests, &item.UsedCost, &item.UsedRequests, &item.CreatedAt, &key, &models)
	if err != nil {
		return item, err
	}
	if len(key) >= 12 {
		item.KeyMasked = key[:6] + "****" + key[len(key)-4:]
	} else {
		item.KeyMasked = "****"
	}
	item.SupportedModels = []string{}
	if err := json.Unmarshal([]byte(models), &item.SupportedModels); err != nil {
		return item, errors.New("invalid stored key model policy")
	}
	if item.SupportedModels == nil {
		item.SupportedModels = []string{}
	}
	return item, nil
}

func (s *GatewayUserStore) ListOwnKeys(ctx context.Context, userID int64) ([]UserKey, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryxContext(ctx, s.db.Rebind(`SELECT `+userKeyColumns+` FROM downstream_api_keys WHERE key_owner_type='user' AND owner_user_id=? ORDER BY id`), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []UserKey{}
	for rows.Next() {
		item, err := scanUserKey(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *GatewayUserStore) CreateOwnKey(ctx context.Context, userID int64, name, key string, models []string, expires *string, maxCost *float64, maxRequests *int64) (UserKey, error) {
	if err := s.available(); err != nil {
		return UserKey{}, err
	}
	encoded, err := json.Marshal(models)
	if err != nil {
		return UserKey{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Ownership and current account validity are checked inside the INSERT.
	return scanUserKey(s.db.QueryRowxContext(ctx, s.db.Rebind(`INSERT INTO downstream_api_keys
		(key_owner_type,owner_user_id,name,key,supported_models,expires_at,max_cost,max_requests,enabled,created_at,updated_at)
		SELECT 'user',u.id,?,?,?,?,?,?,TRUE,?,? FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id
		WHERE u.id=? AND u.enabled=TRUE AND g.enabled=TRUE RETURNING `+userKeyColumns), name, key, string(encoded), expires, maxCost, maxRequests, now, now, userID))
}

// RotateOwnKey replaces only the secret of an existing caller-owned Key. Its
// identity, restrictions, counters and request history stay intact. Once the
// transaction commits, the old secret cannot authorize another request/WS turn.
func (s *GatewayUserStore) RotateOwnKey(ctx context.Context, userID, keyID int64, key string) (UserKey, error) {
	if err := s.available(); err != nil {
		return UserKey{}, err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return UserKey{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Recheck current ownership/account state inside the write, rather than
	// relying solely on the earlier session read. A disabled Key stays disabled.
	item, err := scanUserKey(tx.QueryRowxContext(ctx, s.db.Rebind(`UPDATE downstream_api_keys
		SET key=?,updated_at=?
		WHERE id=? AND key_owner_type='user' AND owner_user_id=?
		AND EXISTS (SELECT 1 FROM gateway_users u JOIN gateway_user_groups g ON g.id=u.group_id
			WHERE u.id=downstream_api_keys.owner_user_id AND u.enabled=TRUE AND g.enabled=TRUE)
		RETURNING `+userKeyColumns), key, now, keyID, userID))
	if err != nil {
		return UserKey{}, err
	}
	// A malformed stored policy must not rotate the secret and then fail while
	// building the safe response: in that case the caller receives no new key.
	if err := tx.Commit(); err != nil {
		return UserKey{}, err
	}
	return item, nil
}

func (s *GatewayUserStore) DeleteOwnKey(ctx context.Context, userID, keyID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM downstream_api_keys WHERE id=? AND key_owner_type='user' AND owner_user_id=?`), keyID, userID)
	return authChanged(result, err)
}

type UserRequest struct {
	ID             string            `db:"id" json:"id"`
	ModelRequested *string           `db:"model_requested" json:"modelRequested"`
	TerminalStatus string            `db:"terminal_status" json:"terminalStatus"`
	LatencyMs      int64             `db:"latency_ms" json:"latencyMs"`
	StartedAt      string            `db:"started_at" json:"startedAt"`
	EndedAt        string            `db:"ended_at" json:"endedAt"`
	AttemptCount   int               `db:"attempt_count" json:"attemptCount"`
	Quota          *UserRequestQuota `db:"-" json:"quota"`
}

func (s *GatewayUserStore) ListOwnRequests(ctx context.Context, userID int64, page, pageSize int) ([]UserRequest, int64, error) {
	return s.ListOwnRequestsFiltered(ctx, userID, page, pageSize, UserRequestFilter{})
}

func (s *GatewayUserStore) ListOwnRequestsFiltered(ctx context.Context, userID int64, page, pageSize int, filter UserRequestFilter) ([]UserRequest, int64, error) {
	if err := s.available(); err != nil {
		return nil, 0, err
	}
	// Total and page use the same explicit immutable ownership predicate.
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	where, args, err := filter.where(userID)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := tx.GetContext(ctx, &total, s.db.Rebind(`SELECT COUNT(*) FROM gateway_requests r LEFT JOIN gateway_user_budget_ledger q ON q.request_id=r.id AND q.user_id=r.owner_user_id`+where), args...); err != nil {
		return nil, 0, err
	}
	items := []UserRequest{}
	rows, err := tx.QueryxContext(ctx, s.db.Rebind(userRequestSelect+where+` ORDER BY r.started_at DESC,r.id DESC LIMIT ? OFFSET ?`), append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanUserRequest(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if err := rows.Close(); err != nil {
		return nil, 0, err
	}
	return items, total, tx.Commit()
}
func (s *GatewayUserStore) OwnRequest(ctx context.Context, userID int64, id string) (UserRequest, error) {
	var item UserRequest
	if err := s.available(); err != nil {
		return item, err
	}
	return scanUserRequest(s.db.QueryRowxContext(ctx, s.db.Rebind(userRequestSelect+` WHERE r.id=? AND r.owner_user_id=?`), id, userID))
}

type UserUsage struct {
	UsedCost         float64 `db:"used_cost" json:"usedCost"`
	UsedRequests     int64   `db:"used_requests" json:"usedRequests"`
	KeyCount         int64   `db:"key_count" json:"keyCount"`
	RecordedRequests int64   `db:"recorded_requests" json:"recordedRequests"`
}

func (s *GatewayUserStore) OwnUsage(ctx context.Context, userID int64) (UserUsage, error) {
	var item UserUsage
	if err := s.available(); err != nil {
		return item, err
	}
	// Retained request records are independent of current Key counters and survive
	// Key deletion/rotation. Log retention may still remove these records.
	err := s.db.GetContext(ctx, &item, s.db.Rebind(`SELECT COALESCE(SUM(used_cost),0) AS used_cost,COALESCE(SUM(used_requests),0) AS used_requests,COUNT(*) AS key_count,
		(SELECT COUNT(*) FROM gateway_requests WHERE owner_user_id=?) AS recorded_requests
		FROM downstream_api_keys WHERE key_owner_type='user' AND owner_user_id=?`), userID, userID)
	return item, err
}
