package store

import (
	"strconv"
	"strings"
)

type CheckinLogFilter struct {
	AccountID, Status, From, To, Search string
	Reasons, Sites                      []string
}

func (db *DB) CheckinLogs(f CheckinLogFilter, limit, offset int) ([]map[string]any, int, error) {
	var conditions []string
	var args []any

	if f.AccountID != "" {
		if aid, err := strconv.ParseInt(f.AccountID, 10, 64); err == nil && aid > 0 {
			conditions = append(conditions, "cl.account_id = ?")
			args = append(args, aid)
		}
	}
	if f.Status == "success" || f.Status == "failed" || f.Status == "skipped" {
		conditions = append(conditions, "cl.status = ?")
		args = append(args, f.Status)
	}
	// created_at is stored as a naive UTC RFC3339 string, so lexicographic
	// comparison against UTC bounds is correct and dialect-agnostic.
	if f.From != "" {
		conditions = append(conditions, "cl.created_at >= ?")
		args = append(args, f.From)
	}
	if f.To != "" {
		conditions = append(conditions, "cl.created_at <= ?")
		args = append(args, f.To)
	}
	if len(f.Reasons) > 0 {
		// failure_reason is compact JSON from encoding/json, so a substring
		// match on "category":"<value>" hits cleanly. OR across the selected
		// categories so the multi-select behaves like the legacy client filter.
		reasonClauses := make([]string, 0, len(f.Reasons))
		for _, category := range f.Reasons {
			reasonClauses = append(reasonClauses, "LOWER(COALESCE(cl.failure_reason, '')) LIKE ?")
			args = append(args, "%\"category\":\""+strings.ToLower(category)+"\"%")
		}
		conditions = append(conditions, "("+strings.Join(reasonClauses, " OR ")+")")
	}
	if len(f.Sites) > 0 {
		placeHolders := make([]string, 0, len(f.Sites))
		for _, name := range f.Sites {
			placeHolders = append(placeHolders, "?")
			args = append(args, name)
		}
		conditions = append(conditions, "s.name IN ("+strings.Join(placeHolders, ",")+")")
	}
	if f.Search != "" {
		conditions = append(conditions, "(LOWER(COALESCE(a.username, '')) LIKE ? OR LOWER(COALESCE(s.name, '')) LIKE ? OR LOWER(COALESCE(cl.message, '')) LIKE ? OR LOWER(COALESCE(cl.reward, '')) LIKE ?)")
		like := "%" + f.Search + "%"
		args = append(args, like, like, like, like)
	}

	var where string
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	const joined = ` FROM checkin_logs cl
 JOIN accounts a ON cl.account_id = a.id
 JOIN sites s ON a.site_id = s.id`
	rows, err := SelectMaps(db.DB, `SELECT cl.id, cl.account_id, cl.status, cl.message, cl.reward, cl.created_at, cl.failure_reason,
 a.username AS account_username, s.name AS site_name, s.url AS site_url`+joined+where+` ORDER BY cl.created_at DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	var total int
	// Preserve the legacy best-effort count after a successful page read.
	_ = db.Get(&total, "SELECT COUNT(*)"+joined+where, args...)
	return rows, total, nil
}
