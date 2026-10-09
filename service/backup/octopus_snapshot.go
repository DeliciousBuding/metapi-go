package backup

import (
	"errors"
	"fmt"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrOctopusReplacementRequired = errors.New("source snapshot removes imported entries; review the preview and explicitly confirm origin replacement")

type octopusSourceEntity struct {
	kind, table, label string
	ids                map[int64]bool
}
type octopusRemovedMapping struct {
	SourceID int64 `db:"source_id"`
	TargetID int64 `db:"target_id"`
}
type octopusRemoval struct {
	entity  octopusSourceEntity
	mapping octopusRemovedMapping
}

func octopusSourceEntities(d *OctopusV5Dump) []octopusSourceEntity {
	entities := []octopusSourceEntity{
		{kind: "token_routes", table: "token_routes", label: "routes"},
		{kind: "group_items", table: "upstream_group_items", label: "groupItems"},
		{kind: "groups", table: "upstream_groups", label: "groups"},
		{kind: "channel_grants", table: "upstream_grants", label: "channelGrants"},
		{kind: "channel_models", table: "upstream_models", label: "channelModels"},
		{kind: "channel_keys", table: "upstream_credentials", label: "channelKeys"},
		{kind: "channels", table: "upstream_channels", label: "channels"},
	}
	for i := range entities {
		entities[i].ids = map[int64]bool{}
	}
	for _, row := range d.Groups {
		entities[0].ids[row.ID] = true
		entities[2].ids[row.ID] = true
	}
	for _, row := range d.GroupItems {
		entities[1].ids[row.ID] = true
	}
	for _, row := range d.Grants {
		entities[3].ids[row.ID] = true
	}
	for _, row := range d.Models {
		entities[4].ids[row.ID] = true
	}
	for _, row := range d.Credentials {
		entities[5].ids[row.ID] = true
	}
	for _, row := range d.Channels {
		entities[6].ids[row.ID] = true
	}
	return entities
}

// Both preview (DB) and commit (transaction) use the same source-ID comparison.
func octopusRemovals(reader interface {
	Select(any, string, ...any) error
}, db *store.DB, d *OctopusV5Dump, origin string) ([]octopusRemoval, error) {
	var removed []octopusRemoval
	for _, entity := range octopusSourceEntities(d) {
		var mappings []octopusRemovedMapping
		if err := reader.Select(&mappings, db.Rebind(`SELECT source_id,target_id FROM external_source_ids WHERE origin_key=? AND entity_type=? ORDER BY source_id`), origin, entity.kind); err != nil {
			return nil, err
		}
		for _, mapping := range mappings {
			if !entity.ids[mapping.SourceID] {
				removed = append(removed, octopusRemoval{entity: entity, mapping: mapping})
			}
		}
	}
	return removed, nil
}

// Source ownership is explicit. Re-import removes only mappings absent from
// this source snapshot; native rows and other origins are never targets.
func pruneOctopusSourceSnapshot(db *store.DB, tx *sqlx.Tx, d *OctopusV5Dump, origin string) (map[string]int64, error) {
	removed, err := octopusRemovals(tx, db, d, origin)
	if err != nil {
		return nil, err
	}
	counts := map[string]int64{}
	for _, row := range removed {
		if row.entity.kind == "token_routes" {
			var owned int
			if err := tx.Get(&owned, db.Rebind(`SELECT COUNT(*) FROM upstream_route_groups rg JOIN upstream_groups g ON g.id=rg.group_id WHERE rg.route_id=? AND g.origin_key=?`), row.mapping.TargetID, origin); err != nil {
				return nil, err
			}
			if owned != 1 {
				return nil, fmt.Errorf("imported route ownership changed; source replacement requires review")
			}
			if _, err := tx.Exec(db.Rebind(`DELETE FROM token_routes WHERE id=?`), row.mapping.TargetID); err != nil {
				return nil, err
			}
		} else {
			if _, err := tx.Exec(db.Rebind("DELETE FROM "+row.entity.table+" WHERE id=? AND origin_key=?"), row.mapping.TargetID, origin); err != nil {
				return nil, err
			}
		}
		if _, err := tx.Exec(db.Rebind(`DELETE FROM external_source_ids WHERE origin_key=? AND entity_type=? AND source_id=? AND target_id=?`), origin, row.entity.kind, row.mapping.SourceID, row.mapping.TargetID); err != nil {
			return nil, err
		}
		counts[row.entity.label]++
	}
	return counts, nil
}

func addOctopusRemovalPreview(db *store.DB, d *OctopusV5Dump, origin string, preview *OctopusV5Preview) error {
	if db == nil {
		return nil
	}
	removed, err := octopusRemovals(db, db, d, origin)
	if err != nil {
		return err
	}
	if len(removed) > 0 {
		preview.Removals = map[string]int{}
	}
	for _, row := range removed {
		preview.Removals[row.entity.label]++
	}
	return nil
}
