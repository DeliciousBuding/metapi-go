package store

import (
	"errors"
	"fmt"
	"strings"
)

var ErrAuditCount = errors.New("count audit logs")

func (db *DB) AppendAuditLog(entry AdminAuditLog) error {
	_, err := db.Exec(`INSERT INTO admin_audit_logs (actor, method, path, status, request_id, remote_ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.Actor, entry.Method, entry.Path, entry.Status, entry.RequestID, entry.RemoteIP, entry.CreatedAt)
	return err
}

func (db *DB) ListAuditLogs(method, pathFilter string, limit, offset int) ([]AdminAuditLog, int, error) {
	where := []string{"1 = 1"}
	args := []any{}
	if method != "" {
		where = append(where, "method = ?")
		args = append(args, method)
	}
	if pathFilter != "" {
		// LOWER() on both sides: SQLite LIKE is ASCII case-insensitive but
		// Postgres LIKE is not — keep the filter parity across dialects.
		where = append(where, "LOWER(path) LIKE ?")
		args = append(args, "%"+strings.ToLower(pathFilter)+"%")
	}

	predicate := strings.Join(where, " AND ")
	var total int
	if err := db.Get(&total, "SELECT COUNT(*) FROM admin_audit_logs WHERE "+predicate, args...); err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrAuditCount, err)
	}
	items := []AdminAuditLog{}
	err := db.Select(&items, `SELECT id, actor, method, path, status,
 COALESCE(request_id, '') AS request_id, COALESCE(remote_ip, '') AS remote_ip, created_at FROM admin_audit_logs WHERE `+predicate+` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	return items, total, err
}
