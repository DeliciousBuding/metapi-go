package backup

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// AxonHubV14Origin names the origin key an AxonHub v1.4 import claims by
// default. Callers may override it, but every origin is independent.
const AxonHubV14Origin = "axonhub-v1.4"

// ErrAxonHubReplacementRequired is returned when a re-import would remove
// entities a previous import of the same origin created and the caller has not
// explicitly confirmed replacement.
var ErrAxonHubReplacementRequired = errors.New("AxonHub source snapshot removes imported entries; confirm origin replacement")

// AxonHubImportPreview is the safe, non-secret plan an operator reviews before
// committing. It never carries credentials, source URLs, channel names, or any
// other operator text.
type AxonHubImportPreview struct {
	Source          string                  `json:"source"`
	OriginKey       string                  `json:"originKey"`
	Version         string                  `json:"version"`
	Sections        map[string]int          `json:"sections"`
	Routable        map[string]int          `json:"routable"`
	NotImported     map[string]int          `json:"notImported,omitempty"`
	SkippedChannels []AxonHubSkippedChannel `json:"skippedChannels,omitempty"`
	Residuals       []string                `json:"residuals,omitempty"`
	Removals        map[string]int          `json:"removals,omitempty"`
	Blocking        []string                `json:"blocking,omitempty"`
}

// IsAxonHubV14Payload reports whether the payload is an AxonHub v1.4 backup.
// Detection is by envelope only; ParseAxonHubSource does the real work.
func IsAxonHubV14Payload(raw []byte) bool {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || top == nil {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(top["version"]), []byte(`"1.4"`)) && len(top["timestamp"]) > 0 &&
		len(top["channels"]) > 0 && len(top["models"]) > 0
}

// PreviewAxonHubV14 parses, compiles, and compares the result with what a
// previous import of the same origin created. It writes nothing.
func PreviewAxonHubV14(db *store.DB, raw []byte, originKey string) (*AxonHubImportPreview, error) {
	if err := validateOriginKey(originKey); err != nil {
		return nil, err
	}
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		return nil, err
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		return nil, err
	}
	preview := &AxonHubImportPreview{
		Source:      AxonHubV14Origin,
		OriginKey:   originKey,
		Version:     src.Version,
		Sections:    axonHubSourceSections(src),
		Routable:    axonHubPlanCounts(plan),
		NotImported: plan.notImported,
		Residuals:   plan.residuals,
		Blocking:    plan.blocking,
	}
	if len(plan.skipped) > 0 {
		preview.SkippedChannels = plan.skipped
	}
	if db != nil {
		removals, err := axonHubRemovals(db, plan, originKey)
		if err != nil {
			return nil, fmt.Errorf("failed to compare imported source snapshot: %w", err)
		}
		if len(removals) > 0 {
			preview.Removals = removals
		}
	}
	return preview, nil
}

func axonHubSourceSections(src *AxonHubSource) map[string]int {
	sections := map[string]int{
		"channels":      len(src.Channels),
		"models":        len(src.Models),
		"projects":      len(src.Projects),
		"apiKeys":       len(src.APIKeys),
		"modelPrices":   len(src.ChannelModelPrices),
		"systemConfigs": len(src.SystemConfigs),
		"usageRequests": src.UsageRequests,
		"usageLogs":     src.UsageLogs,
	}
	for section, count := range src.UnknownSections {
		sections["section:"+section] = count
	}
	return sections
}

func axonHubPlanCounts(plan *axonHubPlan) map[string]int {
	return map[string]int{
		"channels":    len(plan.channels),
		"credentials": len(plan.credentials),
		"models":      len(plan.models),
		"grants":      len(plan.grants),
		"routes":      len(plan.routes),
	}
}

// axonHubEntityTables maps each mapped entity type to its owning table and the
// deletion order used when a re-import drops a source entity.
var axonHubEntityDeletionOrder = []struct {
	entity string
	table  string
}{
	{"token_routes", "token_routes"},
	{"upstream_group_items", "upstream_group_items"},
	{"upstream_grants", "upstream_grants"},
	{"upstream_models", "upstream_models"},
	{"upstream_credentials", "upstream_credentials"},
	{"upstream_groups", "upstream_groups"},
	{"upstream_channels", "upstream_channels"},
}

// axonHubQueryer is satisfied by both *sqlx.DB and *sqlx.Tx, so the mapping read
// runs either on its own or inside the import transaction.
type axonHubQueryer interface {
	Queryx(query string, args ...any) (*sqlx.Rows, error)
}

// axonHubMappedSources returns, per entity type, the source ids a previous
// import of this origin owns.
func axonHubMappedSources(q axonHubQueryer, db *store.DB, originKey string) (map[string]map[int64]int64, error) {
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

func axonHubRemovals(db *store.DB, plan *axonHubPlan, originKey string) (map[string]int, error) {
	owned, err := axonHubMappedSources(db, db, originKey)
	if err != nil {
		return nil, err
	}
	keep := axonHubPlanSourceIDs(plan)
	removals := map[string]int{}
	for entity, sources := range owned {
		for sourceID := range sources {
			if !keep[entity][sourceID] {
				removals[entity]++
			}
		}
	}
	return removals, nil
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
const axonHubDerivedIDBase int64 = 1 << 40

func axonHubTupleID(parts ...string) int64 {
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
	return axonHubDerivedIDBase + int64(hash%(1<<40))
}

func axonHubCredentialSourceID(credential axonHubPlanCredential) int64 {
	return axonHubTupleID("credential", strconv.Itoa(credential.ChannelSourceID), credential.Name)
}

func axonHubModelSourceID(model axonHubPlanModel) int64 {
	return axonHubTupleID("model", strconv.Itoa(model.ChannelSourceID), model.Name)
}

func axonHubGrantSourceID(grant axonHubPlanGrant) int64 {
	return axonHubTupleID("grant", strconv.Itoa(grant.ChannelSourceID), grant.ModelName, grant.CredentialName)
}

func axonHubGroupSourceID(route axonHubPlanRoute) int64 {
	return axonHubTupleID("group", strconv.Itoa(route.SourceModelID), route.GroupKey)
}

func axonHubItemSourceID(route axonHubPlanRoute, item axonHubPlanRouteItem) int64 {
	return axonHubTupleID("item", strconv.Itoa(route.SourceModelID), item.Key)
}

// ImportAxonHubV14 writes the compiled channel graph in a single transaction.
// Re-imports update the same rows through origin-scoped source-id mappings and
// drop only entities this origin previously owned.
func ImportAxonHubV14(db *store.DB, raw []byte, originKey string, allowReplacement bool) (map[string]int64, error) {
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		return nil, err
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		return nil, err
	}
	if len(plan.blocking) > 0 {
		return nil, errors.New("AxonHub backup has no channel that a direct upstream grant can serve")
	}
	if err = validateOriginKey(originKey); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("database is unavailable")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("begin AxonHub import: %w", err)
	}
	defer tx.Rollback()

	// Recheck inside the transaction, not only in the HTTP preview: a caller
	// that did not confirm replacement must never remove source-owned entries.
	owned, err := axonHubMappedSources(tx, db, originKey)
	if err != nil {
		return nil, err
	}
	keep := axonHubPlanSourceIDs(plan)
	removals := map[string]int{}
	for entity, sources := range owned {
		for sourceID := range sources {
			if !keep[entity][sourceID] {
				removals[entity]++
			}
		}
	}
	if len(removals) > 0 && !allowReplacement {
		return nil, ErrAxonHubReplacementRequired
	}

	counts := map[string]int64{}
	channelIDs := map[int]int64{}
	for _, channel := range plan.channels {
		id, err := upsertMapped(db, tx, originKey, "upstream_channels", int64(channel.SourceID), "upstream_channels",
			[]string{"name", "dialect", "enabled", "base_url", "openai_chat_completion_path", "openai_response_path", "anthropic_message_path", "proxy", "channel_proxy", "custom_header", "param_override", "match_regex"},
			[]any{channel.Name, "generic", channel.Enabled, channel.BaseURL, channel.ChatPath, channel.ResponsesPath, channel.MessagesPath, false, channel.ChannelProxy, "[]", channel.ParamOverride, ""})
		if err != nil {
			return nil, err
		}
		channelIDs[channel.SourceID] = id
		counts["channels"]++
	}
	credentialIDs := map[int64]int64{}
	for _, credential := range plan.credentials {
		sourceID := axonHubCredentialSourceID(credential)
		id, err := upsertMapped(db, tx, originKey, "upstream_credentials", sourceID, "upstream_credentials",
			[]string{"channel_id", "name", "secret", "enabled"},
			[]any{channelIDs[credential.ChannelSourceID], credential.Name, credential.Secret, credential.Enabled})
		if err != nil {
			return nil, err
		}
		credentialIDs[sourceID] = id
		counts["credentials"]++
	}
	modelIDs := map[int64]int64{}
	for _, model := range plan.models {
		sourceID := axonHubModelSourceID(model)
		id, err := upsertMapped(db, tx, originKey, "upstream_models", sourceID, "upstream_models",
			[]string{"channel_id", "name", "enabled"},
			[]any{channelIDs[model.ChannelSourceID], model.Name, model.Enabled})
		if err != nil {
			return nil, err
		}
		modelIDs[sourceID] = id
		counts["models"]++
	}
	grantIDs := map[int64]int64{}
	for _, grant := range plan.grants {
		sourceID := axonHubGrantSourceID(grant)
		modelSourceID := axonHubModelSourceID(axonHubPlanModel{ChannelSourceID: grant.ChannelSourceID, Name: grant.ModelName})
		credentialSourceID := axonHubTupleID("credential", strconv.Itoa(grant.ChannelSourceID), grant.CredentialName)
		id, err := upsertMapped(db, tx, originKey, "upstream_grants", sourceID, "upstream_grants",
			[]string{"model_id", "credential_id", "protocols", "enabled"},
			[]any{modelIDs[modelSourceID], credentialIDs[credentialSourceID], grant.Protocols, grant.Enabled})
		if err != nil {
			return nil, err
		}
		grantIDs[sourceID] = id
		counts["grants"]++
	}
	for _, route := range plan.routes {
		groupSourceID := axonHubGroupSourceID(route)
		groupID, err := upsertMapped(db, tx, originKey, "upstream_groups", groupSourceID, "upstream_groups",
			[]string{"name", "mode", "active_item_id", "relay_config", "enabled"},
			[]any{route.GroupKey, "failover", int64(0), "{}", true})
		if err != nil {
			return nil, err
		}
		counts["groups"]++
		routeID, err := upsertAxonHubRoute(db, tx, originKey, int64(route.SourceModelID), route.Pattern, route.Strategy, route.Enabled)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(db.Rebind(`INSERT INTO upstream_route_groups (route_id, group_id) VALUES (?, ?) ON CONFLICT(route_id) DO UPDATE SET group_id=excluded.group_id`), routeID, groupID); err != nil {
			return nil, fmt.Errorf("link imported group to model route: %w", err)
		}
		counts["routes"]++
		for _, item := range route.Items {
			grantSourceID := axonHubTupleID("grant", strconv.Itoa(item.ChannelSourceID), item.ModelName, item.CredentialName)
			if grantIDs[grantSourceID] == 0 {
				return nil, fmt.Errorf("AxonHub route item references a grant that was not compiled")
			}
			if _, err := upsertMapped(db, tx, originKey, "upstream_group_items", axonHubItemSourceID(route, item), "upstream_group_items",
				[]string{"group_id", "grant_id", "priority", "weight"},
				[]any{groupID, grantIDs[grantSourceID], int64(item.Priority), int64(1)}); err != nil {
				return nil, err
			}
			counts["groupItems"]++
		}
	}
	if err := pruneAxonHubSource(db, tx, originKey, keep, owned); err != nil {
		return nil, err
	}
	for label, count := range removals {
		counts["removed"+strings.ToUpper(label[:1])+label[1:]] = int64(count)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit AxonHub import: %w", err)
	}
	return counts, nil
}

// upsertAxonHubRoute claims the source model id as an exact Metapi route. A
// route already owned by a different origin is refused instead of stolen.
func upsertAxonHubRoute(db *store.DB, tx *sqlx.Tx, origin string, sourceID int64, pattern, strategy string, enabled bool) (int64, error) {
	var routeID int64
	err := tx.Get(&routeID, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type=? AND source_id=?`), origin, "token_routes", sourceID)
	if err == nil {
		if _, err := tx.Exec(db.Rebind(`UPDATE token_routes SET model_pattern=?, routing_strategy=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`), pattern, strategy, enabled, routeID); err != nil {
			return 0, fmt.Errorf("update imported model route: %w", err)
		}
		return routeID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("lookup imported model route mapping: %w", err)
	}
	var conflicts int
	if err = tx.Get(&conflicts, db.Rebind(`SELECT COUNT(*) FROM token_routes WHERE model_pattern=?`), pattern); err != nil {
		return 0, err
	}
	if conflicts > 0 {
		return 0, fmt.Errorf("model route %q already exists and cannot be claimed by an AxonHub import", sanitizeIdentifier(pattern))
	}
	if err = tx.Get(&routeID, db.Rebind(`INSERT INTO token_routes (model_pattern,route_mode,routing_strategy,enabled,created_at,updated_at) VALUES (?, 'pattern', ?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`), pattern, strategy, enabled); err != nil {
		return 0, fmt.Errorf("create imported model route: %w", err)
	}
	if _, err = tx.Exec(db.Rebind(`INSERT INTO external_source_ids (origin_key,entity_type,source_id,target_id) VALUES (?,?,?,?)`), origin, "token_routes", sourceID, routeID); err != nil {
		return 0, fmt.Errorf("save imported model route mapping: %w", err)
	}
	return routeID, nil
}

// pruneAxonHubSource removes only the entities this origin previously owned and
// the current source no longer contains.
func pruneAxonHubSource(db *store.DB, tx *sqlx.Tx, origin string, keep map[string]map[int64]bool, owned map[string]map[int64]int64) error {
	for _, entry := range axonHubEntityDeletionOrder {
		sources := owned[entry.entity]
		if len(sources) == 0 {
			continue
		}
		var staleSources []any
		var staleTargets []any
		for sourceID, targetID := range sources {
			if keep[entry.entity][sourceID] {
				continue
			}
			staleSources = append(staleSources, sourceID)
			staleTargets = append(staleTargets, targetID)
		}
		if len(staleSources) == 0 {
			continue
		}
		for start := 0; start < len(staleTargets); start += 400 {
			end := start + 400
			if end > len(staleTargets) {
				end = len(staleTargets)
			}
			targetChunk := staleTargets[start:end]
			sourceChunk := staleSources[start:end]
			targetQuery, targetArgs, err := sqlx.In("DELETE FROM "+entry.table+" WHERE id IN (?)", targetChunk)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(db.Rebind(targetQuery), targetArgs...); err != nil {
				return fmt.Errorf("remove stale imported %s: %w", entry.entity, err)
			}
			mappingQuery, mappingArgs, err := sqlx.In(`DELETE FROM external_source_ids WHERE origin_key = ? AND entity_type = ? AND source_id IN (?)`, origin, entry.entity, sourceChunk)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(db.Rebind(mappingQuery), mappingArgs...); err != nil {
				return fmt.Errorf("remove stale imported %s mapping: %w", entry.entity, err)
			}
		}
	}
	return nil
}
