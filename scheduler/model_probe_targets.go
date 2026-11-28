package scheduler

import (
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// loadProbeTargets selects a budgeted set of active route channels for probing.
// Prefers channels that are currently cooling down, then other enabled channels.
// Caps the batch so default-on budgets stay small (out of scope: every model).
func (s *ModelProbeScheduler) loadProbeTargets(dbw *store.DB) ([]ProbeTarget, error) {
	const maxBatch = 16
	nowISO := time.Now().UTC().Format(time.RFC3339)

	// Cooling channels first (proactive recovery signal).
	cooling, err := queryProbeTargets(dbw, `
		SELECT rc.id, rc.account_id, a.site_id, COALESCE(rc.source_model, '') AS source_model
		FROM route_channels rc
		INNER JOIN accounts a ON rc.account_id = a.id
		INNER JOIN sites st ON a.site_id = st.id
		WHERE rc.enabled = TRUE
		  AND a.status = 'active'
		  AND st.status = 'active'
		  AND rc.cooldown_until IS NOT NULL
		  AND rc.cooldown_until > ?
		  AND COALESCE(rc.source_model, '') <> ''
		ORDER BY rc.cooldown_until ASC
		LIMIT ?
	`, nowISO, maxBatch)
	if err != nil {
		return nil, err
	}

	if len(cooling) >= maxBatch {
		return cooling, nil
	}

	// Fill remaining budget with active (non-cooling) channels.
	remaining := maxBatch - len(cooling)
	active, err := queryProbeTargets(dbw, `
		SELECT rc.id, rc.account_id, a.site_id, COALESCE(rc.source_model, '') AS source_model
		FROM route_channels rc
		INNER JOIN accounts a ON rc.account_id = a.id
		INNER JOIN sites st ON a.site_id = st.id
		WHERE rc.enabled = TRUE
		  AND a.status = 'active'
		  AND st.status = 'active'
		  AND rc.cooldown_until IS NULL
		  AND COALESCE(rc.source_model, '') <> ''
		ORDER BY COALESCE(rc.last_used_at, '') ASC, rc.id ASC
		LIMIT ?
	`, remaining)
	if err != nil {
		return cooling, err
	}

	// De-dupe by channel id.
	seen := make(map[int64]struct{}, len(cooling)+len(active))
	out := make([]ProbeTarget, 0, len(cooling)+len(active))
	for _, t := range cooling {
		if _, ok := seen[t.ChannelID]; ok {
			continue
		}
		seen[t.ChannelID] = struct{}{}
		out = append(out, t)
	}
	for _, t := range active {
		if _, ok := seen[t.ChannelID]; ok {
			continue
		}
		seen[t.ChannelID] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

func queryProbeTargets(dbw *store.DB, query string, args ...any) ([]ProbeTarget, error) {
	rows, err := dbw.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProbeTarget
	var scanErr error
	for rows.Next() {
		var t ProbeTarget
		if err := rows.Scan(&t.ChannelID, &t.AccountID, &t.SiteID, &t.ModelName); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		if t.ChannelID <= 0 || t.ModelName == "" {
			continue
		}
		out = append(out, t)
	}
	return out, scanErr
}
