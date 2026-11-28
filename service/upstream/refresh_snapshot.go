package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/jmoiron/sqlx"
)

type catalogModelRow struct {
	ID      int64  `db:"id"`
	Name    string `db:"name"`
	Enabled bool   `db:"enabled"`
	Origin  string `db:"origin_key"`
}

func loadCatalogModels(ctx context.Context, db sqlx.ExtContext, channelID int64) ([]catalogModelRow, error) {
	rows := []catalogModelRow{}
	if err := sqlx.SelectContext(ctx, db, &rows, db.Rebind(`SELECT id,name,enabled,origin_key FROM upstream_models WHERE channel_id=? ORDER BY id`), channelID); err != nil {
		return nil, err
	}
	return rows, nil
}

// catalogBasisToken binds the catalog rows and the discovered set: applying a
// preview is only valid while both are unchanged.
func catalogBasisToken(rows []catalogModelRow, discovered []string, identity ...string) string {
	h := sha256.New()
	if len(identity) > 0 && identity[0] != "" {
		_, _ = h.Write([]byte("provider:" + identity[0] + "\x00"))
	}
	for _, row := range rows {
		_, _ = fmt.Fprintf(h, "%d|%s|%t|%s\n", row.ID, row.Name, row.Enabled, row.Origin)
	}
	h.Write([]byte{0})
	sorted := slices.Clone(discovered)
	slices.Sort(sorted)
	for _, name := range sorted {
		h.Write([]byte(name + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// staleImpact counts the grants and the routes referencing one model, so the
// operator sees what disabling it affects before confirming.
func staleImpact(ctx context.Context, db sqlx.ExtContext, modelID int64) (int, []int64, error) {
	var grantCount int
	if err := sqlx.GetContext(ctx, db, &grantCount, db.Rebind(`SELECT COUNT(*) FROM upstream_grants WHERE model_id=?`), modelID); err != nil {
		return 0, nil, err
	}
	routeIDs := []int64{}
	if err := sqlx.SelectContext(ctx, db, &routeIDs, db.Rebind(`SELECT DISTINCT rg.route_id FROM upstream_route_groups rg JOIN upstream_group_items i ON i.group_id=rg.group_id JOIN upstream_grants g ON g.id=i.grant_id WHERE g.model_id=? ORDER BY rg.route_id`), modelID); err != nil {
		return 0, nil, err
	}
	return grantCount, routeIDs, nil
}

func computeCatalogDiff(rows []catalogModelRow, discovered []string, discoveryOrigin string) (added, kept, manualOnly, manualKept, offline []string, stale []catalogModelRow) {
	catalogNames := map[string]bool{}
	upstreamSet := map[string]bool{}
	for _, name := range discovered {
		upstreamSet[name] = true
	}
	added, kept, manualOnly, manualKept, offline = []string{}, []string{}, []string{}, []string{}, []string{}
	stale = []catalogModelRow{}
	for _, row := range rows {
		catalogNames[row.Name] = true
		switch row.Origin {
		case discoveryOrigin:
			if upstreamSet[row.Name] {
				kept = append(kept, row.Name)
			} else if row.Enabled {
				stale = append(stale, row)
			} else {
				offline = append(offline, row.Name)
			}
		default:
			// Native rows are operator-owned: never refreshed away, reported
			// for visibility only.
			if upstreamSet[row.Name] {
				manualKept = append(manualKept, row.Name)
			} else {
				manualOnly = append(manualOnly, row.Name)
			}
		}
	}
	for _, name := range discovered {
		if !catalogNames[name] {
			added = append(added, name)
		}
	}
	return added, kept, manualOnly, manualKept, offline, stale
}
