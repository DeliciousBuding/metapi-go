// Package upstream manages the direct-upstream catalog independently of imports.
package upstream

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

const NativeOrigin = "native:local"

func Ownership(origin string) string {
	if origin == NativeOrigin {
		return "native"
	}
	return "imported"
}

type Error struct {
	Status    int
	Message   string
	MemberIDs []int64
}

func (e *Error) Error() string      { return e.Message }
func invalid(message string) error  { return &Error{Status: 400, Message: message} }
func conflict(message string) error { return &Error{Status: 409, Message: message} }

func normalizeError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return &Error{Status: 404, Message: "Upstream resource not found"}
	}
	if err != nil {
		s := strings.ToLower(err.Error())
		if strings.Contains(s, "unique constraint") || strings.Contains(s, "duplicate key") || strings.Contains(s, "database is locked") || strings.Contains(s, "serialization") || strings.Contains(s, "deadlock detected") {
			return conflict("Upstream catalog conflicts with another update")
		}
	}
	return err
}

// Write uses the graph lifecycle lock before reads, serializing mutations with
// deletion previews and preventing SQLite read-snapshot upgrade races.
// Invalidating only after commit keeps failed mutations invisible to routing.
func Write(ctx context.Context, db *sqlx.DB, apply func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return normalizeError(err)
	}
	defer tx.Rollback()
	if err = LockLifecycleTx(ctx, tx); err != nil {
		return normalizeError(err)
	}
	if err = apply(tx); err != nil {
		return normalizeError(err)
	}
	if err = tx.Commit(); err != nil {
		return normalizeError(err)
	}
	routing.InvalidateCache()
	return nil
}

// InsertNative accepts only catalog entities and their fixed column lists.
// The database owns identity allocation; the staging source ID never escapes
// the transaction. Multi-entity callers insert channel, credential, model,
// grant, group, member in that order, avoiding inverted unique-key waits.
func InsertNative(ctx context.Context, tx *sqlx.Tx, entity string, values ...any) (int64, error) {
	var columns string
	switch entity {
	case "channels":
		columns = "name,dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex"
	case "credentials":
		columns = "channel_id,name,secret,enabled,kind,oauth_state"
	case "models":
		columns = "channel_id,name,enabled"
	case "grants":
		columns = "model_id,credential_id,protocols,enabled"
	case "groups":
		columns = "name,mode,active_item_id,relay_config,enabled"
	case "group_items":
		columns = "group_id,grant_id,priority,weight,protocol_order"
	default:
		return 0, invalid("Unknown native catalog entity")
	}
	if len(values) != len(strings.Split(columns, ",")) {
		return 0, fmt.Errorf("invalid native insert arity")
	}
	args := append([]any{NativeOrigin, int64(0)}, values...)
	query := `INSERT INTO upstream_` + entity + ` (origin_key,source_id,` + columns + `) VALUES (` + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + `) RETURNING id`
	var id int64
	if err := tx.GetContext(ctx, &id, tx.Rebind(query), args...); err != nil {
		return 0, err
	}
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE upstream_`+entity+` SET source_id=? WHERE id=?`), -id, id)
	return id, err
}

func lockQuery(tx *sqlx.Tx, query string) string {
	if tx.DriverName() == "pgx" {
		return query + " FOR UPDATE"
	}
	return query
}
func validName(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 200 && !strings.ContainsAny(value, "\r\n\x00")
}
func boolDefault(value *bool, fallback bool) bool {
	if value != nil {
		return *value
	}
	return fallback
}
