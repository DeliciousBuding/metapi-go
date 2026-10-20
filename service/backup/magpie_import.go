package backup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ErrMagpieReplacementRequired is returned when a re-import would remove rows
// a previous import of the same origin created without explicit confirmation.
var ErrMagpieReplacementRequired = errors.New("Magpie source snapshot removes imported entries; confirm origin replacement")

// MagpieImportPreview is the safe plan shown before an import. It contains no
// provider keys, downstream keys, URLs, names, or other operator text.
type MagpieImportPreview struct {
	Source           string                    `json:"source"`
	OriginKey        string                    `json:"originKey"`
	Version          string                    `json:"version"`
	Sections         map[string]int            `json:"sections"`
	Routable         map[string]int            `json:"routable"`
	NotImported      map[string]int            `json:"notImported,omitempty"`
	SkippedProviders []MagpieSkippedProvider   `json:"skippedProviders,omitempty"`
	SkippedGroups    []MagpieSkippedGroup      `json:"skippedGroups,omitempty"`
	Residuals        []string                  `json:"residuals,omitempty"`
	Removals         map[string]int            `json:"removals,omitempty"`
	RemovalImpact    *upstream.DeletionPreview `json:"removalImpact,omitempty"`
	Blocking         []string                  `json:"blocking,omitempty"`
}

// PreviewMagpieBackup decrypts, parses and compiles a Magpie backup, then
// compares it with the rows a previous import of the same origin owns.
func PreviewMagpieBackup(db *store.DB, raw []byte, passphrase, originKey string) (*MagpieImportPreview, error) {
	if err := validateOriginKey(originKey); err != nil {
		return nil, err
	}
	src, err := ParseMagpieBackup(raw, passphrase)
	if err != nil {
		return nil, err
	}
	plan, err := compileMagpiePlan(src)
	if err != nil {
		return nil, err
	}
	preview := &MagpieImportPreview{
		Source: MagpieBackupOrigin, OriginKey: originKey,
		Version:  fmt.Sprintf("bundle-%d", src.Version),
		Sections: magpieSourceSections(src), Routable: magpiePlanCounts(plan),
		NotImported: plan.notImported, SkippedProviders: plan.skippedProviders,
		SkippedGroups: plan.skippedGroups, Residuals: plan.residuals, Blocking: plan.blocking,
	}
	if len(preview.SkippedProviders) == 0 {
		preview.SkippedProviders = nil
	}
	if len(preview.SkippedGroups) == 0 {
		preview.SkippedGroups = nil
	}
	if len(preview.Residuals) == 0 {
		preview.Residuals = nil
	}
	if len(preview.Blocking) == 0 {
		preview.Blocking = nil
	}
	if db != nil {
		tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		owned, err := mappedSourceIDs(tx, db, originKey)
		if err != nil {
			return nil, err
		}
		removalPlan, err := mappedSourceDeletionPlan(tx, originKey, magpiePlanSourceIDs(plan), owned)
		if err != nil {
			return nil, err
		}
		preview.Removals = mappedSourceRemovalCounts(removalPlan, magpiePlanSourceIDs(plan), owned)
		if len(preview.Removals) > 0 {
			preview.RemovalImpact = &removalPlan.Preview
		}
	}
	return preview, nil
}

// ImportMagpieBackup writes the compiled Magpie graph in one transaction.
// Re-imports update the same rows through origin-scoped mappings.
func ImportMagpieBackup(db *store.DB, raw []byte, passphrase, originKey string, allowReplacement bool, expectedRevision ...string) (map[string]int64, error) {
	if err := validateOriginKey(originKey); err != nil {
		return nil, err
	}
	if db == nil {
		return nil, errors.New("database is unavailable")
	}
	src, err := ParseMagpieBackup(raw, passphrase)
	if err != nil {
		return nil, err
	}
	plan, err := compileMagpiePlan(src)
	if err != nil {
		return nil, err
	}
	if len(plan.blocking) > 0 {
		return nil, errors.New("Magpie backup has no provider a direct upstream grant can serve")
	}
	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("begin Magpie import: %w", err)
	}
	defer tx.Rollback()
	if err = upstream.LockLifecycleTx(context.Background(), tx); err != nil {
		return nil, err
	}
	keep := magpiePlanSourceIDs(plan)
	owned, err := mappedSourceIDs(tx, db, originKey)
	if err != nil {
		return nil, err
	}
	removalPlan, err := mappedSourceDeletionPlan(tx, originKey, keep, owned)
	if err != nil {
		return nil, err
	}
	removals := mappedSourceRemovalCounts(removalPlan, keep, owned)
	if len(removals) > 0 && !allowReplacement {
		return nil, ErrMagpieReplacementRequired
	}
	if err = confirmSourceDeletion(removalPlan, originKey, allowReplacement, expectedRevision, ErrMagpieReplacementRequired); err != nil {
		return nil, err
	}
	if _, err = upstream.ApplyDeletionTx(context.Background(), tx, removalPlan); err != nil {
		return nil, err
	}
	if err = deleteMappedSourceKeys(tx, originKey, keep, owned); err != nil {
		return nil, err
	}

	counts := map[string]int64{}
	channelIDs := map[int64]int64{}
	for _, provider := range plan.providers {
		id, err := upsertMapped(db, tx, originKey, "upstream_channels", provider.SourceID, "upstream_channels",
			[]string{"name", "dialect", "provider", "enabled", "base_url", "endpoint_config", "openai_chat_completion_path", "openai_response_path", "anthropic_message_path", "proxy", "channel_proxy", "custom_header", "param_override", "match_regex"},
			[]any{provider.Name, "generic", "magpie", provider.Enabled, strings.TrimRight(provider.BaseURL, "/"), provider.Endpoints, "/v1/chat/completions", "/v1/responses", "/v1/messages", provider.UseProxy, provider.ChannelProxy, provider.CustomHeader, "", ""})
		if err != nil {
			return nil, err
		}
		channelIDs[provider.SourceID] = id
		counts["channels"]++
	}
	credentialIDs := map[int64]int64{}
	for _, credential := range plan.credentials {
		channelID := sourceProviderForCredential(plan, credential.ProviderID)
		if channelID == 0 {
			return nil, errors.New("Magpie credential references a channel that was not compiled")
		}
		id, err := upsertMapped(db, tx, originKey, "upstream_credentials", credential.SourceID, "upstream_credentials",
			[]string{"channel_id", "name", "secret", "kind", "oauth_state", "enabled"},
			[]any{channelIDs[channelID], credential.Name, credential.Secret, store.DirectCredentialAPIKey, store.DirectOAuthState{}, credential.Enabled})
		if err != nil {
			return nil, err
		}
		credentialIDs[credential.SourceID] = id
		counts["credentials"]++
	}
	modelIDs := map[int64]int64{}
	for _, model := range plan.models {
		channelID := sourceProviderForModel(plan, model.ProviderID)
		if channelID == 0 {
			return nil, errors.New("Magpie model references a channel that was not compiled")
		}
		id, err := upsertMapped(db, tx, originKey, "upstream_models", model.SourceID, "upstream_models",
			[]string{"channel_id", "name", "enabled"}, []any{channelIDs[channelID], model.Name, model.Enabled})
		if err != nil {
			return nil, err
		}
		modelIDs[model.SourceID] = id
		counts["models"]++
	}
	grantIDs := map[int64]int64{}
	for _, grant := range plan.grants {
		modelSource := sourceTupleID("magpie-model", grant.ProviderID, grant.ModelName)
		credentialName := grant.Credential
		credentialSource := sourceTupleID("magpie-credential", grant.ProviderID, credentialName)
		if modelIDs[modelSource] == 0 || credentialIDs[credentialSource] == 0 {
			return nil, errors.New("Magpie grant references a model or credential that was not compiled")
		}
		id, err := upsertMapped(db, tx, originKey, "upstream_grants", grant.SourceID, "upstream_grants",
			[]string{"model_id", "credential_id", "protocols", "enabled"},
			[]any{modelIDs[modelSource], credentialIDs[credentialSource], grant.Protocols, grant.Enabled})
		if err != nil {
			return nil, err
		}
		grantIDs[grant.SourceID] = id
		counts["grants"]++
	}
	for index, route := range plan.routes {
		routeID, err := upsertImportedRoute(db, tx, importedRoute{
			Origin: originKey, SourceID: route.SourceID, Pattern: route.Pattern,
			DisplayName: route.DisplayName, Strategy: route.Strategy, Enabled: route.Enabled,
			SortOrder: index, Context: route.Context,
		})
		if err != nil {
			return nil, err
		}
		groupID, err := upsertMapped(db, tx, originKey, "upstream_groups", route.Group.SourceID, "upstream_groups",
			[]string{"name", "mode", "active_item_id", "relay_config", "enabled"},
			[]any{route.Group.Name, route.Group.Mode, int64(0), "{}", route.Group.Enabled})
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(db.Rebind(`INSERT INTO upstream_route_groups (route_id, group_id) VALUES (?, ?) ON CONFLICT(route_id) DO UPDATE SET group_id=excluded.group_id`), routeID, groupID); err != nil {
			return nil, fmt.Errorf("link imported Magpie group to route: %w", err)
		}
		counts["routes"]++
		counts["groups"]++
		itemIDs := map[int64]int64{}
		for _, item := range route.Group.Items {
			grantID := grantIDs[item.GrantSource]
			if grantID == 0 {
				return nil, errors.New("Magpie group item references a grant that was not compiled")
			}
			itemID, err := upsertMapped(db, tx, originKey, "upstream_group_items", item.SourceID, "upstream_group_items",
				[]string{"group_id", "grant_id", "priority", "weight", "protocol_order"},
				[]any{groupID, grantID, int64(item.Priority), int64(item.Weight), store.DirectProtocolOrder{}})
			if err != nil {
				return nil, err
			}
			itemIDs[item.SourceID] = itemID
			counts["groupItems"]++
		}
		if route.Group.ActiveItemID != 0 {
			active := itemIDs[route.Group.ActiveItemID]
			if active == 0 {
				return nil, errors.New("Magpie manual group active item was not compiled")
			}
			if _, err := tx.Exec(db.Rebind(`UPDATE upstream_groups SET active_item_id=? WHERE id=?`), active, groupID); err != nil {
				return nil, fmt.Errorf("set imported Magpie group active item: %w", err)
			}
		}
	}
	for _, key := range plan.keys {
		if err := importMagpieAccessKey(db, tx, originKey, key); err != nil {
			return nil, err
		}
		counts["gatewayKeys"]++
	}
	for label, count := range removals {
		counts["removed"+strings.ToUpper(label[:1])+label[1:]] = int64(count)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit Magpie import: %w", err)
	}
	return counts, nil
}

func sourceProviderForCredential(plan *magpiePlan, providerID string) int64 {
	for _, provider := range plan.providers {
		if provider.ID == providerID {
			return provider.SourceID
		}
	}
	return 0
}

func sourceProviderForModel(plan *magpiePlan, providerID string) int64 {
	return sourceProviderForCredential(plan, providerID)
}

func importMagpieAccessKey(db *store.DB, tx *sqlx.Tx, origin string, key magpiePlanKey) error {
	policy := key.Policy
	encoded, err := json.Marshal(policy)
	if err != nil {
		return errors.New("cannot encode imported Magpie key policy")
	}
	var id, collision int64
	err = tx.Get(&id, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type='downstream_api_keys' AND source_id=?`), origin, key.SourceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.New("cannot read imported Magpie key identity")
	}
	if err = tx.Get(&collision, db.Rebind(`SELECT id FROM downstream_api_keys WHERE key=?`), key.Secret); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.New("cannot check imported Magpie key collision")
	}
	if collision != 0 && collision != id {
		return errors.New("imported Magpie key collides with an existing key from another origin")
	}
	if id != 0 {
		_, err = tx.Exec(db.Rebind(`UPDATE downstream_api_keys SET name=?,key=?,enabled=?,supported_models='["*"]',access_policy=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`), key.Name, key.Secret, key.Enabled, string(encoded), id)
	} else {
		err = tx.QueryRowx(db.Rebind(`INSERT INTO downstream_api_keys (name,key,enabled,supported_models,access_policy,created_at,updated_at) VALUES (?,?,?,'["*"]',?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`), key.Name, key.Secret, key.Enabled, string(encoded)).Scan(&id)
		if err == nil {
			_, err = tx.Exec(db.Rebind(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) VALUES (?,'downstream_api_keys',?,?)`), origin, key.SourceID, id)
		}
	}
	if err != nil {
		return importedUpsertError("Magpie gateway key", err)
	}
	return nil
}
