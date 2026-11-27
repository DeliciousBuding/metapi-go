package store

import "github.com/jmoiron/sqlx"

func (db *DB) SiteNames(ids []int64) (map[int64]string, error) {
	names := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	query, args, err := sqlx.In(`SELECT id, COALESCE(name,'') AS name FROM sites WHERE id IN (?)`, ids)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		ID   int64
		Name string
	}
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

type CooldownChannel struct {
	ID            int64   `db:"id"`
	AccountID     int64   `db:"account_id"`
	FailCount     int64   `db:"fail_count"`
	SiteID        int64   `db:"site_id"`
	SiteName      string  `db:"site_name"`
	LastFailAt    *string `db:"last_fail_at"`
	CooldownUntil *string `db:"cooldown_until"`
	ReasonCode    *string `db:"cooldown_reason_code"`
	Reason        *string `db:"cooldown_reason"`
}

func (db *DB) CooldownChannels() ([]CooldownChannel, error) {
	var rows []CooldownChannel
	err := db.Select(&rows, `
		SELECT
			rc.id AS id,
			COALESCE(rc.account_id,0) AS account_id,
			COALESCE(rc.fail_count,0) AS fail_count,
			NULLIF(rc.last_fail_at,'') AS last_fail_at,
			NULLIF(rc.cooldown_until,'') AS cooldown_until,
			rc.cooldown_reason_code, rc.cooldown_reason,
			COALESCE(s.id,0) AS site_id,
			COALESCE(s.name, '') AS site_name
		FROM route_channels rc
		LEFT JOIN accounts a ON a.id = rc.account_id
		LEFT JOIN sites s ON s.id = a.site_id
		WHERE rc.cooldown_until IS NOT NULL OR rc.fail_count > 0
		ORDER BY rc.cooldown_until DESC
		LIMIT 10000
	`)
	return rows, err
}

type HealthStatusCounts struct {
	Total    int64 `json:"total"`
	Active   int64 `json:"active"`
	Disabled int64 `json:"disabled"`
	Other    int64 `json:"other"`
}

func (db *DB) SiteStatusCounts() (HealthStatusCounts, error) { return db.statusCounts("sites") }

func (db *DB) AccountStatusCounts() (HealthStatusCounts, error) { return db.statusCounts("accounts") }

func (db *DB) statusCounts(table string) (HealthStatusCounts, error) {
	var counts HealthStatusCounts
	var rows []struct {
		Status *string
		Count  int64
	}
	if err := db.Select(&rows, "SELECT status, COUNT(*) AS count FROM "+table+" GROUP BY status"); err != nil {
		return counts, err
	}
	for _, row := range rows {
		counts.Total += row.Count
		switch {
		case row.Status != nil && *row.Status == "active":
			counts.Active += row.Count
		case row.Status != nil && *row.Status == "disabled":
			counts.Disabled += row.Count
		default:
			counts.Other += row.Count
		}
	}
	return counts, nil
}
