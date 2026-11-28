package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// sourceMappingQueryer is satisfied by both *sqlx.DB and *sqlx.Tx, so the mapping read
// runs either on its own or inside the import transaction.
type sourceMappingQueryer interface {
	Queryx(query string, args ...any) (*sqlx.Rows, error)
}

// mappedSourceIDs returns, per entity type, the source ids a previous
// import of this origin owns.
func mappedSourceIDs(q sourceMappingQueryer, db *store.DB, originKey string) (map[string]map[int64]int64, error) {
	rows, err := q.Queryx(db.Rebind(`SELECT entity_type, source_id, target_id FROM external_source_ids WHERE origin_key = ?`), originKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[int64]int64{}
	for rows.Next() {
		var entity string
		var sourceID, targetID int64
		if err := rows.Scan(&entity, &sourceID, &targetID); err != nil {
			return nil, err
		}
		if out[entity] == nil {
			out[entity] = map[int64]int64{}
		}
		out[entity][sourceID] = targetID
	}
	return out, rows.Err()
}

// axonHubPlanSourceIDs is the set of source ids the compiled plan owns, keyed by
// entity type. It must stay in lockstep with assignAxonHubSourceIDs.
func axonHubPlanSourceIDs(plan *axonHubPlan) map[string]map[int64]bool {
	ids := map[string]map[int64]bool{}
	mark := func(entity string, sourceID int64) {
		if ids[entity] == nil {
			ids[entity] = map[int64]bool{}
		}
		ids[entity][sourceID] = true
	}
	for _, key := range plan.keys {
		mark("downstream_api_keys", int64(key.SourceID))
	}
	for _, channel := range plan.channels {
		mark("upstream_channels", int64(channel.SourceID))
	}
	for _, credential := range plan.credentials {
		mark("upstream_credentials", axonHubCredentialSourceID(credential))
	}
	for _, model := range plan.models {
		mark("upstream_models", axonHubModelSourceID(model))
	}
	for _, grant := range plan.grants {
		mark("upstream_grants", axonHubGrantSourceID(grant))
	}
	for _, route := range plan.routes {
		mark("token_routes", int64(route.SourceModelID))
		mark("upstream_groups", axonHubGroupSourceID(route))
		for _, item := range route.Items {
			mark("upstream_group_items", axonHubItemSourceID(route, item))
		}
	}
	return ids
}

// AxonHub backups carry no primary key for credentials, per-channel models,
// grants, groups, or items: they are identified by the channel/model/key tuple.
// The importer therefore derives a deterministic BIGINT from that tuple, so an
// unchanged backup re-imports onto the same rows (preserving health counters)
// instead of duplicating them. The high base keeps the derived ids clear of
// AxonHub's serial source ids.
const derivedSourceIDBase int64 = 1 << 40

func sourceTupleID(parts ...string) int64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	hash := uint64(offset64)
	for _, part := range parts {
		for i := 0; i < len(part); i++ {
			hash ^= uint64(part[i])
			hash *= prime64
		}
		hash ^= 0
		hash *= prime64
	}
	return derivedSourceIDBase + int64(hash%(1<<40))
}

func axonHubCredentialSourceID(credential axonHubPlanCredential) int64 {
	return sourceTupleID("credential", strconv.Itoa(credential.ChannelSourceID), credential.Name)
}

func axonHubModelSourceID(model axonHubPlanModel) int64 {
	return sourceTupleID("model", strconv.Itoa(model.ChannelSourceID), model.Name)
}

func axonHubGrantSourceID(grant axonHubPlanGrant) int64 {
	return sourceTupleID("grant", strconv.Itoa(grant.ChannelSourceID), grant.ModelName, grant.CredentialName)
}

func axonHubGroupSourceID(route axonHubPlanRoute) int64 {
	return sourceTupleID("group", strconv.Itoa(route.SourceModelID), route.GroupKey)
}

func axonHubItemSourceID(route axonHubPlanRoute, item axonHubPlanRouteItem) int64 {
	return sourceTupleID("item", strconv.Itoa(route.SourceModelID), item.Key)
}

type importedRoute struct {
	Origin          string
	SourceID        int64
	Pattern         string
	DisplayName     string
	Strategy        string
	TraceStickyMode string
	Enabled         bool
	SortOrder       int
	Context         *int
	// KeepPresentation leaves display metadata alone on re-import, which is
	// the AxonHub contract: its snapshot has no such fields to update.
	KeepPresentation bool
}

// upsertImportedRoute claims the source model id as an exact Metapi route. A
// route already owned by a different origin is refused instead of stolen.
func upsertImportedRoute(db *store.DB, tx *sqlx.Tx, route importedRoute) (int64, error) {
	var routeID int64
	err := tx.Get(&routeID, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type=? AND source_id=?`), route.Origin, "token_routes", route.SourceID)
	if err == nil {
		if err = upstream.ValidateSourceReferencesTx(context.Background(), tx, route.Origin, []upstream.Reference{{Kind: upstream.KindRoute, ID: routeID}}); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrImportedEntityConflict, err)
		}
		columns := []string{"model_pattern=?", "routing_strategy=?", "trace_sticky_mode=?", "enabled=?"}
		values := []any{route.Pattern, route.Strategy, route.TraceStickyMode, route.Enabled}
		if !route.KeepPresentation {
			columns = append(columns, "display_name=?", "context_length=?", "sort_order=?")
			values = append(values, route.DisplayName, route.Context, route.SortOrder)
		}
		values = append(values, routeID)
		result, updateErr := tx.Exec(db.Rebind(`UPDATE token_routes SET `+strings.Join(columns, ", ")+`, updated_at=CURRENT_TIMESTAMP WHERE id=?`), values...)
		if updateErr != nil {
			return 0, updateErr
		}
		changed, countErr := result.RowsAffected()
		if countErr != nil {
			return 0, countErr
		}
		if changed != 1 {
			return 0, ErrImportedEntityConflict
		}
		return routeID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("lookup imported model route mapping: %w", err)
	}
	var conflicts int
	if err = tx.Get(&conflicts, db.Rebind(`SELECT COUNT(*) FROM token_routes WHERE model_pattern=?`), route.Pattern); err != nil {
		return 0, err
	}
	if conflicts > 0 {
		return 0, fmt.Errorf("model route %q already exists and cannot be claimed by an imported source", sanitizeIdentifier(route.Pattern))
	}
	if err = tx.Get(&routeID, db.Rebind(`INSERT INTO token_routes (model_pattern,display_name,route_mode,routing_strategy,trace_sticky_mode,context_length,sort_order,enabled,created_at,updated_at) VALUES (?,?,'pattern',?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`),
		route.Pattern, route.DisplayName, route.Strategy, route.TraceStickyMode, route.Context, route.SortOrder, route.Enabled); err != nil {
		return 0, fmt.Errorf("create imported model route: %w", err)
	}
	if _, err = tx.Exec(db.Rebind(`INSERT INTO external_source_ids (origin_key,entity_type,source_id,target_id) VALUES (?,?,?,?)`), route.Origin, "token_routes", route.SourceID, routeID); err != nil {
		return 0, fmt.Errorf("save imported model route mapping: %w", err)
	}
	return routeID, nil
}
