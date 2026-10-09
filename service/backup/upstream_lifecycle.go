package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

var ErrImportedEntityConflict = errors.New("imported entity conflicts with existing ownership or mapping")

func confirmSourceDeletion(plan *upstream.DeletionPlan, origin string, confirmed bool, revisions []string, replacementError error) error {
	revision := ""
	if len(revisions) > 0 {
		revision = revisions[0]
	}
	if plan.IncludesOtherOrigins(origin) && (!confirmed || revision == "") {
		return fmt.Errorf("%w: deletion includes local or other-source dependents; confirm removalImpact.revision", replacementError)
	}
	if revision != "" && revision != plan.Preview.Revision {
		return fmt.Errorf("%w: deletion impact changed; review a fresh preview", ErrImportedEntityConflict)
	}
	return nil
}

func importedUpsertError(entity string, err error) error {
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "unique constraint") || strings.Contains(text, "duplicate key") {
		return fmt.Errorf("%w: %s identity, name or relationship already exists", ErrImportedEntityConflict, entity)
	}
	return fmt.Errorf("write imported %s: %w", entity, err)
}

func sourceDeletionPlan(tx *sqlx.Tx, origin string, roots []upstream.Reference) (*upstream.DeletionPlan, error) {
	ctx := context.Background()
	if err := upstream.ValidateSourceReferencesTx(ctx, tx, origin, roots); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrImportedEntityConflict, err)
	}
	plan, err := upstream.PlanDeletionTx(ctx, tx, roots)
	if err != nil {
		return nil, err
	}
	plan.Preview.Kind = "source"
	plan.Preview.ID = 0
	return plan, nil
}

func octopusDeletionPlan(tx *sqlx.Tx, db *store.DB, d *OctopusV5Dump, origin string) (*upstream.DeletionPlan, error) {
	removed, err := octopusRemovals(tx, db, d, origin)
	if err != nil {
		return nil, err
	}
	roots := make([]upstream.Reference, 0, len(removed))
	for _, row := range removed {
		kind, ok := upstream.KindForSourceEntity(row.entity.kind)
		if !ok {
			return nil, ErrImportedEntityConflict
		}
		roots = append(roots, upstream.Reference{Kind: kind, ID: row.mapping.TargetID})
	}
	return sourceDeletionPlan(tx, origin, roots)
}

func octopusRemovalCounts(p *upstream.DeletionPlan) map[string]int {
	counts := map[string]int{}
	for source, label := range map[string]string{"channels": "channels", "models": "channelModels", "credentials": "channelKeys", "grants": "channelGrants", "groups": "groups", "members": "groupItems", "routes": "routes"} {
		if count := p.Preview.Counts[source]; count > 0 {
			counts[label] = count
		}
	}
	return counts
}

func axonHubDeletionPlan(tx *sqlx.Tx, origin string, keep map[string]map[int64]bool, owned map[string]map[int64]int64) (*upstream.DeletionPlan, error) {
	var roots []upstream.Reference
	for entity, sources := range owned {
		kind, ok := upstream.KindForSourceEntity(entity)
		if !ok {
			continue
		}
		for source, id := range sources {
			if !keep[entity][source] {
				roots = append(roots, upstream.Reference{Kind: kind, ID: id})
			}
		}
	}
	plan, err := sourceDeletionPlan(tx, origin, roots)
	if err != nil {
		return nil, err
	}
	// Source API keys are not graph nodes, but their deletion still belongs to
	// the same reviewed replacement. Bind their identities without reading keys.
	var keys [][2]int64
	for source, id := range owned["downstream_api_keys"] {
		if !keep["downstream_api_keys"][source] {
			var count int
			if err := tx.Get(&count, tx.Rebind(`SELECT COUNT(*) FROM downstream_api_keys WHERE id=?`), id); err != nil {
				return nil, err
			}
			if count != 1 {
				return nil, fmt.Errorf("%w: stale source API key mapping", ErrImportedEntityConflict)
			}
			keys = append(keys, [2]int64{source, id})
		}
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i][0] < keys[j][0] })
	encoded, err := json.Marshal(struct {
		Origin, Graph string
		Keys          [][2]int64
	}{origin, plan.Preview.Revision, keys})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	plan.Preview.Revision = hex.EncodeToString(sum[:])
	return plan, nil
}

func axonHubRemovalCounts(p *upstream.DeletionPlan, keep map[string]map[int64]bool, owned map[string]map[int64]int64) map[string]int {
	counts := map[string]int{}
	for label, entity := range map[string]string{"channels": "upstream_channels", "models": "upstream_models", "credentials": "upstream_credentials", "grants": "upstream_grants", "groups": "upstream_groups", "members": "upstream_group_items", "routes": "token_routes"} {
		if n := p.Preview.Counts[label]; n > 0 {
			counts[entity] = n
		}
	}
	for source := range owned["downstream_api_keys"] {
		if !keep["downstream_api_keys"][source] {
			counts["downstream_api_keys"]++
		}
	}
	return counts
}

func deleteAxonHubSourceKeys(tx *sqlx.Tx, origin string, keep map[string]map[int64]bool, owned map[string]map[int64]int64) error {
	for source, id := range owned["downstream_api_keys"] {
		if keep["downstream_api_keys"][source] {
			continue
		}
		if _, err := tx.Exec(tx.Rebind(`DELETE FROM downstream_api_keys WHERE id=?`), id); err != nil {
			return err
		}
		if _, err := tx.Exec(tx.Rebind(`DELETE FROM external_source_ids WHERE origin_key=? AND entity_type='downstream_api_keys' AND source_id=? AND target_id=?`), origin, source, id); err != nil {
			return err
		}
	}
	return nil
}
