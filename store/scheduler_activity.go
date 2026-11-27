package store

// SchedulerActivity preserves the dashboard's best-effort signals. An empty
// history or failed individual read leaves just that signal at its zero value.
type SchedulerActivity struct {
	Latest  string `db:"created_at"`
	Status  string `db:"status"`
	Runs    int64
	Success int64
}

func (db *DB) CheckinActivity(since string) SchedulerActivity {
	var a SchedulerActivity
	_ = db.Get(&a, `SELECT created_at, status FROM checkin_logs ORDER BY created_at DESC LIMIT 1`)
	_ = db.Get(&a.Runs, `SELECT COUNT(*) FROM checkin_logs WHERE created_at >= ?`, since)
	_ = db.Get(&a.Success, `SELECT COUNT(*) FROM checkin_logs WHERE created_at >= ? AND status = 'success'`, since)
	return a
}

func (db *DB) BalanceRefreshActivity(since string) (SchedulerActivity, int64) {
	var a SchedulerActivity
	var active int64
	_ = db.Get(&a.Latest, `SELECT MAX(last_balance_refresh) FROM accounts WHERE last_balance_refresh IS NOT NULL`)
	_ = db.Get(&a.Runs, `SELECT COUNT(*) FROM accounts WHERE last_balance_refresh >= ?`, since)
	_ = db.Get(&active, `SELECT COUNT(*) FROM accounts WHERE status = 'active'`)
	return a, active
}

func (db *DB) LatestSiteAnnouncementTime() string {
	var latest string
	_ = db.Get(&latest, `SELECT MAX(last_seen_at) FROM site_announcements`)
	return latest
}

func (db *DB) EventActivity(eventType, since string) SchedulerActivity {
	var a SchedulerActivity
	_ = db.Get(&a.Latest, `SELECT created_at FROM events WHERE type = ? ORDER BY created_at DESC LIMIT 1`, eventType)
	_ = db.Get(&a.Runs, `SELECT COUNT(*) FROM events WHERE type = ? AND created_at >= ?`, eventType, since)
	return a
}
