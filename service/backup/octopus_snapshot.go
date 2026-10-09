package backup

import (
	"context"
	"database/sql"
	"errors"

	"github.com/deliciousbuding/metapi-go/store"
)

var ErrOctopusReplacementRequired = errors.New("source snapshot removes imported entries; review the preview and explicitly confirm origin replacement")

type octopusSourceEntity struct {
	kind string
	ids  map[int64]bool
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
		{kind: "token_routes"},
		{kind: "group_items"},
		{kind: "groups"},
		{kind: "channel_grants"},
		{kind: "channel_models"},
		{kind: "channel_keys"},
		{kind: "channels"},
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

// Preview follows the same relationship closure that replacement removes.
func addOctopusRemovalPreview(db *store.DB, d *OctopusV5Dump, origin string, preview *OctopusV5Preview) error {
	if db == nil {
		return nil
	}
	tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	plan, err := octopusDeletionPlan(tx, db, d, origin)
	if err != nil {
		return err
	}
	preview.Removals = octopusRemovalCounts(plan)
	if len(preview.Removals) > 0 {
		preview.RemovalImpact = &plan.Preview
	}
	return nil
}
