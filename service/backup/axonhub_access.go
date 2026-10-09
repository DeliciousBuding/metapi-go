package backup

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

type AxonHubAccessProfiles struct {
	ActiveProfile string                 `json:"activeProfile"`
	Profiles      []AxonHubAccessProfile `json:"profiles"`
}

type AxonHubAccessProfile struct {
	Name                 string                         `json:"name"`
	TemplateID           *int                           `json:"templateID,omitempty"`
	TemplateName         string                         `json:"templateName,omitempty"`
	ChannelIDs           []int                          `json:"channelIDs,omitempty"`
	ChannelTags          []string                       `json:"channelTags,omitempty"`
	ChannelTagsMatchMode string                         `json:"channelTagsMatchMode,omitempty"`
	ModelIDs             []string                       `json:"modelIDs,omitempty"`
	ModelMappings        []store.DownstreamModelMapping `json:"modelMappings,omitempty"`
	Quota                json.RawMessage                `json:"quota,omitempty"`
	LoadBalanceStrategy  *string                        `json:"loadBalanceStrategy,omitempty"`
	TraceStickyMode      *string                        `json:"traceStickyMode,omitempty"`
}

func decodeAxonHubAccessProfiles(raw json.RawMessage, project bool) (*AxonHubAccessProfiles, error) {
	if !present(raw) {
		return nil, nil
	}
	var p AxonHubAccessProfiles
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&p) != nil {
		return nil, axonHubErr("invalid AxonHub access profiles")
	}
	seen := map[string]bool{}
	for _, profile := range p.Profiles {
		if profile.Name == "" || seen[profile.Name] {
			return nil, axonHubErr("invalid AxonHub access profile name")
		}
		seen[profile.Name] = true
		switch profile.ChannelTagsMatchMode {
		case "", "any", "all", "none":
		default:
			return nil, axonHubErr("invalid AxonHub channel tags match mode")
		}
		for _, id := range profile.ChannelIDs {
			if id <= 0 {
				return nil, axonHubErr("invalid AxonHub profile channel ID")
			}
		}
		if project && (len(profile.ModelIDs) > 0 || len(profile.ModelMappings) > 0 || present(profile.Quota)) {
			return nil, axonHubErr("unsupported AxonHub project profile field")
		}
	}
	if p.ActiveProfile != "" && !seen[p.ActiveProfile] {
		return nil, axonHubErr("AxonHub active access profile does not exist")
	}
	return &p, nil
}

func (p *AxonHubAccessProfiles) active() *AxonHubAccessProfile {
	if p == nil || p.ActiveProfile == "" {
		return nil
	}
	for i := range p.Profiles {
		if p.Profiles[i].Name == p.ActiveProfile {
			return &p.Profiles[i]
		}
	}
	return nil
}

func (p *AxonHubAccessProfile) allows(ch AxonHubSourceChannel) bool {
	if p == nil {
		return true
	}
	if len(p.ChannelIDs) > 0 && !slices.Contains(p.ChannelIDs, ch.ID) {
		return false
	}
	if len(p.ChannelTags) == 0 {
		return true
	}
	matched := 0
	for _, tag := range p.ChannelTags {
		if slices.Contains(ch.Tags, tag) {
			matched++
		}
	}
	switch p.ChannelTagsMatchMode {
	case "all":
		return matched == len(p.ChannelTags)
	case "none":
		return matched == 0
	default:
		return matched > 0
	}
}

// Only quota-bearing numeric history is retained. The source key is used for
// association inside the transaction and never rendered in errors or previews.
type AxonHubQuotaUsage struct {
	ID          int64     `json:"id"`
	Key         string    `json:"api_key_key"`
	CreatedAt   time.Time `json:"created_at"`
	TotalTokens int64     `json:"total_tokens"`
	Cost        *float64  `json:"total_cost"`
}

func decodeAxonHubQuotaUsage(raw json.RawMessage) ([]AxonHubQuotaUsage, bool, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, false, nil
	}
	var rows []AxonHubQuotaUsage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, false, axonHubErr("invalid AxonHub quota usage")
	}
	seen := map[int64]bool{}
	for _, u := range rows {
		if u.ID <= 0 || seen[u.ID] || u.CreatedAt.IsZero() || u.TotalTokens < 0 || (u.Cost != nil && (*u.Cost < 0 || math.IsNaN(*u.Cost) || math.IsInf(*u.Cost, 0))) {
			return nil, false, axonHubErr("invalid AxonHub quota usage identity or amount")
		}
		seen[u.ID] = true
	}
	return rows, true, nil
}

type axonHubPlanAPIKey struct {
	SourceID                            int
	Name, Key, ProjectName, IPAllowlist string
	Enabled                             bool
	ChannelSourceIDs                    []int
	Policy                              store.DownstreamAccessPolicy
	Usage                               []AxonHubQuotaUsage
}

func compileAxonHubAccess(src *AxonHubSource, plan *axonHubPlan) error {
	projects := map[int]AxonHubSourceProject{}
	for _, p := range src.Projects {
		if p.ID <= 0 {
			return axonHubErr("invalid AxonHub project ID")
		}
		if _, ok := projects[p.ID]; ok {
			return axonHubErr("duplicate AxonHub project ID")
		}
		projects[p.ID] = p
	}
	channelIDs := map[int]bool{}
	for _, ch := range plan.channels {
		channelIDs[ch.SourceID] = true
	}
	seenID := map[int]bool{}
	seenKey := map[string]bool{}
	for _, key := range src.APIKeys {
		if key.ID <= 0 || seenID[key.ID] {
			return axonHubErr("invalid or duplicate AxonHub API key ID")
		}
		seenID[key.ID] = true
		if key.DeletedAt != 0 {
			plan.notImported["deletedApiKeys"]++
			continue
		}
		if strings.TrimSpace(key.Key) == "" {
			plan.notImported["apiKeysWithoutSecret"]++
			continue
		}
		if seenKey[key.Key] {
			return axonHubErr("duplicate AxonHub API key value")
		}
		seenKey[key.Key] = true
		p, hasProject := projects[key.ProjectID]
		compiled := axonHubPlanAPIKey{SourceID: key.ID, Name: key.Name, Key: key.Key, ProjectName: p.Name, Enabled: key.Status == "enabled" || key.Status == ""}
		if key.Type == "noauth" || (key.Type != "" && key.Type != "user" && key.Type != "personal" && key.Type != "service_account") {
			compiled.Policy.BlockReason = "source_key_type_not_proxy"
		}
		if !slices.Contains(key.Scopes, "write_requests") {
			compiled.Policy.BlockReason = "source_key_missing_write_requests"
		}
		if !hasProject || p.DeletedAt != 0 || (p.Status != "" && p.Status != "active") {
			compiled.Policy.BlockReason = "source_project_unavailable"
		}
		profile := key.Profiles.active()
		for _, ch := range src.Channels {
			if channelIDs[ch.ID] && p.Profiles.active().allows(ch) && profile.allows(ch) {
				compiled.ChannelSourceIDs = append(compiled.ChannelSourceIDs, ch.ID)
			}
		}
		for _, ip := range key.IPAllowlist {
			if _, err := netip.ParsePrefix(ip); err != nil {
				if addr, err := netip.ParseAddr(ip); err != nil || addr.Zone() != "" {
					return axonHubErr("invalid AxonHub API key allowed IP")
				}
			}
		}
		compiled.IPAllowlist = strings.Join(key.IPAllowlist, "\n")
		if profile != nil {
			compiled.Policy.ModelIDs = profile.ModelIDs
			compiled.Policy.ModelMappings = profile.ModelMappings
			for _, v := range []*string{profile.LoadBalanceStrategy, profile.TraceStickyMode} {
				if v != nil && *v != "" && *v != "default" && *v != "system_default" {
					compiled.Policy.BlockReason = "source_key_routing_override_unsupported"
				}
			}
			if present(profile.Quota) {
				q, err := compileAxonHubQuota(profile.Quota, src)
				if err != nil {
					return err
				}
				compiled.Policy.Quota = q
				if !src.QuotaHistoryPresent {
					plan.residuals = append(plan.residuals, "api_key_quota_history_missing")
				}
			}
		}
		if err := compiled.Policy.Validate(); err != nil {
			compiled.Policy.ModelMappings = nil
			compiled.Policy.BlockReason = "source_key_model_mapping_unsupported"
		}
		if compiled.Policy.BlockReason != "" {
			plan.residuals = append(plan.residuals, compiled.Policy.BlockReason)
		}
		for _, usage := range src.QuotaUsage {
			if usage.Key == key.Key {
				compiled.Usage = append(compiled.Usage, usage)
			}
		}
		plan.keys = append(plan.keys, compiled)
	}
	if len(src.Projects) > 0 {
		plan.residuals = append(plan.residuals, "project_active_profiles_flattened_into_key_channel_boundaries")
	}
	return nil
}

func compileAxonHubQuota(raw json.RawMessage, src *AxonHubSource) (*store.DownstreamQuota, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil, axonHubErr("invalid AxonHub quota")
	}
	if v := obj["cost"]; len(v) > 0 && len(bytes.TrimSpace(v)) > 0 && bytes.TrimSpace(v)[0] == '"' {
		var amount string
		_ = json.Unmarshal(v, &amount)
		n, err := strconv.ParseFloat(amount, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, axonHubErr("invalid AxonHub quota cost")
		}
		obj["cost"], _ = json.Marshal(n)
	}
	b, _ := json.Marshal(obj)
	var q store.DownstreamQuota
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil {
		return nil, axonHubErr("invalid AxonHub quota fields")
	}
	q.Timezone = "UTC"
	for _, config := range src.SystemConfigs {
		if config.Key == "system_general_settings" {
			var s struct {
				Timezone string `json:"timezone"`
			}
			if json.Unmarshal([]byte(config.Value), &s) != nil {
				return nil, axonHubErr("invalid AxonHub quota timezone settings")
			}
			if s.Timezone != "" {
				q.Timezone = s.Timezone
			}
		}
	}
	if !src.QuotaHistoryPresent {
		timestamp, err := time.Parse(time.RFC3339Nano, src.Timestamp)
		if err != nil {
			return nil, axonHubErr("AxonHub quota history is missing and backup timestamp is invalid")
		}
		ms := timestamp.UnixMilli()
		q.HistoryMissingBefore = &ms
	}
	if err := (&store.DownstreamAccessPolicy{Quota: &q}).Validate(); err != nil {
		return nil, axonHubErr("invalid AxonHub quota definition")
	}
	return &q, nil
}

func importAxonHubAccess(db *store.DB, tx *sqlx.Tx, origin string, plan *axonHubPlan, channelIDs map[int]int64, counts map[string]int64) error {
	for _, key := range plan.keys {
		ids := make([]int64, 0, len(key.ChannelSourceIDs))
		for _, sourceID := range key.ChannelSourceIDs {
			id := channelIDs[sourceID]
			if id <= 0 {
				return axonHubErr("imported API key channel mapping is missing")
			}
			ids = append(ids, id)
		}
		policy := key.Policy
		policy.AllowedUpstreamChannelIDs = &ids
		encoded, err := json.Marshal(policy)
		if err != nil {
			return axonHubErr("cannot encode imported key policy")
		}
		var id, collision int64
		err = tx.Get(&id, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type='downstream_api_keys' AND source_id=?`), origin, key.SourceID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return axonHubErr("cannot read imported API key identity")
		}
		if err = tx.Get(&collision, db.Rebind(`SELECT id FROM downstream_api_keys WHERE key=?`), key.Key); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return axonHubErr("cannot check imported API key collision")
		}
		if collision != 0 && collision != id {
			return axonHubErr("imported API key collides with an existing key from another origin")
		}
		if id != 0 {
			_, err = tx.Exec(db.Rebind(`UPDATE downstream_api_keys SET name=?,key=?,group_name=?,enabled=?,supported_models='["*"]',access_policy=?,ip_allowlist=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`), key.Name, key.Key, key.ProjectName, key.Enabled, string(encoded), key.IPAllowlist, id)
		} else {
			err = tx.QueryRowx(db.Rebind(`INSERT INTO downstream_api_keys (name,key,group_name,enabled,supported_models,access_policy,ip_allowlist,created_at,updated_at) VALUES (?,?,?,?, '["*"]',?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP) RETURNING id`), key.Name, key.Key, key.ProjectName, key.Enabled, string(encoded), key.IPAllowlist).Scan(&id)
			if err == nil {
				_, err = tx.Exec(db.Rebind(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) VALUES (?,'downstream_api_keys',?,?)`), origin, key.SourceID, id)
			}
		}
		if err != nil {
			return axonHubErr("failed to save imported API key")
		}
		for _, usage := range key.Usage {
			// Source NULL total_cost is SUM-ignored (equivalent to zero), unlike
			// an unobserved native usage event where accounting is unknown.
			cost := 0.0
			if usage.Cost != nil {
				cost = *usage.Cost
			}
			_, err := tx.Exec(db.Rebind(`INSERT INTO downstream_quota_usage(key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,?,?,1,?,?) ON CONFLICT(key_id,event_key) DO NOTHING`), id, fmt.Sprintf("source:%s:%d", origin, usage.ID), usage.CreatedAt.UnixMilli(), usage.TotalTokens, cost)
			if err != nil {
				return axonHubErr("failed to save imported quota usage")
			}
		}
		counts["apiKeys"]++
	}
	return nil
}
