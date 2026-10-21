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

func (s *GatewayUserStore) DeleteOwnKey(ctx context.Context, userID, keyID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM downstream_api_keys WHERE id=? AND key_owner_type='user' AND owner_user_id=?`), keyID, userID)
	return authChanged(result, err)
}

type UserRequest struct {
	ID             string  `db:"id" json:"id"`
	ModelRequested *string `db:"model_requested" json:"modelRequested"`
	TerminalStatus string  `db:"terminal_status" json:"terminalStatus"`
	LatencyMs      int64   `db:"latency_ms" json:"latencyMs"`
	StartedAt      string  `db:"started_at" json:"startedAt"`
	EndedAt        string  `db:"ended_at" json:"endedAt"`
	AttemptCount   int     `db:"attempt_count" json:"attemptCount"`
}

const userRequestColumns = `id,model_requested,terminal_status,latency_ms,started_at,ended_at,attempt_count`

func (s *GatewayUserStore) ListOwnRequests(ctx context.Context, userID int64, page, pageSize int) ([]UserRequest, int64, error) {
	if err := s.available(); err != nil {
		return nil, 0, err
	}
	// Total and page use the same explicit immutable ownership predicate.
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var total int64
	if err := tx.GetContext(ctx, &total, s.db.Rebind(`SELECT COUNT(*) FROM gateway_requests WHERE owner_user_id=?`), userID); err != nil {
		return nil, 0, err
	}
	items := []UserRequest{}
	if err := tx.SelectContext(ctx, &items, s.db.Rebind(`SELECT `+userRequestColumns+` FROM gateway_requests WHERE owner_user_id=? ORDER BY started_at DESC,id DESC LIMIT ? OFFSET ?`), userID, pageSize, (page-1)*pageSize); err != nil {
		return nil, 0, err
	}
	return items, total, tx.Commit()
}
func (s *GatewayUserStore) OwnRequest(ctx context.Context, userID int64, id string) (UserRequest, error) {
	var item UserRequest
	if err := s.available(); err != nil {
		return item, err
	}
	err := s.db.GetContext(ctx, &item, `SELECT `+userRequestColumns+` FROM gateway_requests WHERE id=? AND owner_user_id=?`, id, userID)
	return item, err
}

type UserUsage struct {
	UsedCost     float64 `db:"used_cost" json:"usedCost"`
	UsedRequests int64   `db:"used_requests" json:"usedRequests"`
	KeyCount     int64   `db:"key_count" json:"keyCount"`
}

func (s *GatewayUserStore) OwnUsage(ctx context.Context, userID int64) (UserUsage, error) {
	var item UserUsage
	if err := s.available(); err != nil {
		return item, err
	}
	err := s.db.GetContext(ctx, &item, `SELECT COALESCE(SUM(used_cost),0) AS used_cost,COALESCE(SUM(used_requests),0) AS used_requests,COUNT(*) AS key_count FROM downstream_api_keys WHERE key_owner_type='user' AND owner_user_id=?`, userID)
	return item, err
}
