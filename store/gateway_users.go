package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// GatewayUserDirectoryLimit caps the initial administrator directories until
// they have a paginated API. Route grants are not truncated authorization data.
const GatewayUserDirectoryLimit = 1000

var (
	ErrGatewayUserInvalid     = errors.New("invalid gateway user input")
	ErrGatewayUserUnavailable = errors.New("gateway user store unavailable")
)

// GatewayUserGroup is an administrator-managed access group for this gateway.
// It is not an independent organization or an upstream provider pool.
type GatewayUserGroup struct {
	ID        int64  `db:"id" json:"id"`
	Slug      string `db:"slug" json:"slug"`
	Name      string `db:"name" json:"name"`
	Enabled   bool   `db:"enabled" json:"enabled"`
	CreatedAt string `db:"created_at" json:"createdAt"`
	UpdatedAt string `db:"updated_at" json:"updatedAt"`
}

// GatewayUser belongs to exactly one current group. This identity foundation
// contains no login credential, deployment-administrator role or issued Key.
type GatewayUser struct {
	ID          int64  `db:"id" json:"id"`
	Username    string `db:"username" json:"username"`
	DisplayName string `db:"display_name" json:"displayName"`
	GroupID     int64  `db:"group_id" json:"groupId"`
	Enabled     bool   `db:"enabled" json:"enabled"`
	CreatedAt   string `db:"created_at" json:"createdAt"`
	UpdatedAt   string `db:"updated_at" json:"updatedAt"`
}

type GatewayUserGroupRouteGrant struct {
	GroupID   int64  `db:"group_id" json:"groupId"`
	RouteID   int64  `db:"route_id" json:"routeId"`
	CreatedAt string `db:"created_at" json:"createdAt"`
}

// GatewayUserStore owns persistence, not management authorization. Callers
// authorize administrator writes separately; client-supplied IDs are selectors.
type GatewayUserStore struct{ db *DB }

func NewGatewayUserStore(db *DB) *GatewayUserStore { return &GatewayUserStore{db: db} }

func (s *GatewayUserStore) available() error {
	if s == nil || s.db == nil {
		return ErrGatewayUserUnavailable
	}
	return nil
}

func (s *GatewayUserStore) CreateUserGroup(ctx context.Context, slug, name string, enabled bool) (GatewayUserGroup, error) {
	if err := s.available(); err != nil {
		return GatewayUserGroup{}, err
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	name, err := gatewayUserName(name, 100, false)
	if err != nil || !gatewayUserIdentifier(slug) {
		return GatewayUserGroup{}, ErrGatewayUserInvalid
	}
	now := gatewayUserNow()
	var row GatewayUserGroup
	err = s.db.GetContext(ctx, &row, `INSERT INTO gateway_user_groups (slug, name, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING *`, slug, name, enabled, now, now)
	return row, err
}

func (s *GatewayUserStore) GetUserGroup(ctx context.Context, groupID int64) (GatewayUserGroup, error) {
	if err := s.available(); err != nil {
		return GatewayUserGroup{}, err
	}
	if groupID <= 0 {
		return GatewayUserGroup{}, ErrGatewayUserInvalid
	}
	var row GatewayUserGroup
	err := s.db.GetContext(ctx, &row, `SELECT * FROM gateway_user_groups WHERE id = ?`, groupID)
	return row, err
}

func (s *GatewayUserStore) ListUserGroups(ctx context.Context) ([]GatewayUserGroup, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []GatewayUserGroup{}
	err := s.db.SelectContext(ctx, &rows, `SELECT * FROM gateway_user_groups ORDER BY id LIMIT ?`, GatewayUserDirectoryLimit)
	return rows, err
}

func (s *GatewayUserStore) UpdateUserGroup(ctx context.Context, groupID int64, name string, enabled bool) error {
	if err := s.available(); err != nil {
		return err
	}
	name, err := gatewayUserName(name, 100, false)
	if err != nil || groupID <= 0 {
		return ErrGatewayUserInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE gateway_user_groups SET name = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		name, enabled, gatewayUserNow(), groupID)
	return gatewayUserChanged(result, err)
}

// CreateUser uses the default group when groupID is zero. Creating that group
// and its first user is atomic, and an existing default group's administrator
// configuration is never overwritten.
func (s *GatewayUserStore) CreateUser(ctx context.Context, username, displayName string, groupID int64, enabled bool) (GatewayUser, error) {
	if err := s.available(); err != nil {
		return GatewayUser{}, err
	}
	username = strings.ToLower(strings.TrimSpace(username))
	displayName, err := gatewayUserName(displayName, 100, true)
	if err != nil || !gatewayUserIdentifier(username) || groupID < 0 {
		return GatewayUser{}, ErrGatewayUserInvalid
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return GatewayUser{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := gatewayUserNow()
	if groupID == 0 {
		if _, err := tx.ExecContext(ctx, s.db.Rebind(`INSERT INTO gateway_user_groups (slug, name, enabled, created_at, updated_at)
			VALUES ('default', '默认分组', TRUE, ?, ?) ON CONFLICT (slug) DO NOTHING`), now, now); err != nil {
			return GatewayUser{}, err
		}
		if err := tx.GetContext(ctx, &groupID, `SELECT id FROM gateway_user_groups WHERE slug = 'default'`); err != nil {
			return GatewayUser{}, err
		}
	}
	var row GatewayUser
	if err := tx.GetContext(ctx, &row, s.db.Rebind(`INSERT INTO gateway_users (username, display_name, group_id, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) RETURNING *`), username, displayName, groupID, enabled, now, now); err != nil {
		return GatewayUser{}, err
	}
	if err := tx.Commit(); err != nil {
		return GatewayUser{}, err
	}
	return row, nil
}

func (s *GatewayUserStore) GetUser(ctx context.Context, userID int64) (GatewayUser, error) {
	if err := s.available(); err != nil {
		return GatewayUser{}, err
	}
	if userID <= 0 {
		return GatewayUser{}, ErrGatewayUserInvalid
	}
	var row GatewayUser
	err := s.db.GetContext(ctx, &row, `SELECT * FROM gateway_users WHERE id = ?`, userID)
	return row, err
}

func (s *GatewayUserStore) ListUsers(ctx context.Context) ([]GatewayUser, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	rows := []GatewayUser{}
	err := s.db.SelectContext(ctx, &rows, `SELECT * FROM gateway_users ORDER BY id LIMIT ?`, GatewayUserDirectoryLimit)
	return rows, err
}

// UpdateUser changes the current group; subsequent active lookups follow that
// group immediately. No historical group union or per-user grant is retained.
func (s *GatewayUserStore) UpdateUser(ctx context.Context, userID int64, displayName string, groupID int64, enabled bool) error {
	if err := s.available(); err != nil {
		return err
	}
	displayName, err := gatewayUserName(displayName, 100, true)
	if err != nil || userID <= 0 || groupID <= 0 {
		return ErrGatewayUserInvalid
	}
	result, err := s.db.ExecContext(ctx, `UPDATE gateway_users SET display_name = ?, group_id = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		displayName, groupID, enabled, gatewayUserNow(), userID)
	return gatewayUserChanged(result, err)
}

// GetActiveGatewayUser reloads both identity and its current group. Possession
// of a user ID is not authentication and callers must establish the principal.
func (s *GatewayUserStore) GetActiveGatewayUser(ctx context.Context, userID int64) (GatewayUser, error) {
	if err := s.available(); err != nil {
		return GatewayUser{}, err
	}
	if userID <= 0 {
		return GatewayUser{}, ErrGatewayUserInvalid
	}
	var row GatewayUser
	err := s.db.GetContext(ctx, &row, `SELECT u.* FROM gateway_users u
		JOIN gateway_user_groups g ON g.id = u.group_id
		WHERE u.id = ? AND u.enabled = TRUE AND g.enabled = TRUE`, userID)
	return row, err
}

func (s *GatewayUserStore) GrantUserGroupRoute(ctx context.Context, groupID, routeID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	if groupID <= 0 || routeID <= 0 {
		return ErrGatewayUserInvalid
	}
	// Disabled groups/routes can be configured; enabled state is a live gate.
	result, err := s.db.ExecContext(ctx, `INSERT INTO gateway_user_group_route_grants (group_id, route_id, created_at)
		SELECT g.id, r.id, ? FROM gateway_user_groups g JOIN token_routes r ON r.id = ?
		WHERE g.id = ?`, gatewayUserNow(), routeID, groupID)
	return gatewayUserChanged(result, err)
}

func (s *GatewayUserStore) RevokeUserGroupRoute(ctx context.Context, groupID, routeID int64) error {
	if err := s.available(); err != nil {
		return err
	}
	if groupID <= 0 || routeID <= 0 {
		return ErrGatewayUserInvalid
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM gateway_user_group_route_grants WHERE group_id = ? AND route_id = ?`, groupID, routeID)
	return gatewayUserChanged(result, err)
}

func (s *GatewayUserStore) ListUserGroupRouteGrants(ctx context.Context, groupID int64) ([]GatewayUserGroupRouteGrant, error) {
	if _, err := s.GetUserGroup(ctx, groupID); err != nil {
		return nil, err
	}
	rows := []GatewayUserGroupRouteGrant{}
	err := s.db.SelectContext(ctx, &rows, `SELECT group_id, route_id, created_at FROM gateway_user_group_route_grants WHERE group_id = ? ORDER BY route_id`, groupID)
	return rows, err
}

// ListActiveGatewayUserRouteIDs reads the user's current group and all enabled
// states in the same query that returns route grants. Empty means deny-all,
// never unrestricted; this hard layer is separate from legacy Key policy ORs.
func (s *GatewayUserStore) ListActiveGatewayUserRouteIDs(ctx context.Context, userID int64) ([]int64, error) {
	if err := s.available(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrGatewayUserInvalid
	}
	rows := []int64{}
	err := s.db.SelectContext(ctx, &rows, `SELECT gr.route_id FROM gateway_users u
		JOIN gateway_user_groups g ON g.id = u.group_id
		JOIN gateway_user_group_route_grants gr ON gr.group_id = g.id
		JOIN token_routes r ON r.id = gr.route_id
		WHERE u.id = ? AND u.enabled = TRUE AND g.enabled = TRUE AND r.enabled = TRUE
		ORDER BY gr.route_id`, userID)
	return rows, err
}

func gatewayUserIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func gatewayUserName(value string, maxLength int, optional bool) (string, error) {
	value = strings.TrimSpace(value)
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxLength || (!optional && value == "") {
		return "", ErrGatewayUserInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", ErrGatewayUserInvalid
		}
	}
	return value, nil
}

func gatewayUserChanged(result sql.Result, err error) error {
	if err != nil {
		return fmt.Errorf("store: change gateway user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func gatewayUserNow() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }
