package store

type ResinSiteOverride struct {
	SiteID   int64       `db:"id" json:"siteId"`
	Name     string      `db:"name" json:"name"`
	Platform string      `db:"platform" json:"platform"`
	Enabled  scannedBool `db:"resin_enabled" json:"resinEnabled"`
}

// ResinSiteOverrides lists explicit opt-ins and opt-outs in admin display order.
func (db *DB) ResinSiteOverrides() ([]ResinSiteOverride, error) {
	rows := []ResinSiteOverride{}
	if db == nil {
		return rows, nil
	}
	err := db.Select(&rows, `
		SELECT id, name, platform, resin_enabled
		FROM sites
		WHERE resin_enabled IS NOT NULL
		ORDER BY sort_order, id
	`)
	return rows, err
}
