package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

type magpiePlan struct {
	providers   []magpiePlanProvider
	credentials []magpiePlanCredential
	models      []magpiePlanModel
	grants      []magpiePlanGrant
	routes      []magpiePlanRoute
	keys        []magpiePlanKey

	notImported      map[string]int
	residuals        []string
	skippedProviders []MagpieSkippedProvider
	skippedGroups    []MagpieSkippedGroup
	blocking         []string
}

type magpiePlanProvider struct {
	SourceID      int64
	ID            string
	Name          string
	Enabled       bool
	BaseURL       string
	Endpoints     store.DirectEndpoints
	Protocols     int
	Routing       string
	AccountModels map[string][]string
	UseProxy      bool
	ChannelProxy  string
	CustomHeader  string
	Credentials   []magpiePlanCredential
	Models        []string
}

type magpiePlanCredential struct {
	SourceID   int64
	ProviderID string
	Name       string
	Secret     string
	Protocols  int
	Weight     int
	Enabled    bool
}

type magpiePlanModel struct {
	SourceID      int64
	ProviderID    string
	Name          string
	Enabled       bool
	ContextLength *int
}

type magpiePlanGrant struct {
	SourceID   int64
	ProviderID string
	ModelName  string
	Credential string
	Protocols  int
	Enabled    bool
}

type magpiePlanRoute struct {
	SourceID    int64
	Pattern     string
	DisplayName string
	Strategy    string
	Enabled     bool
	Context     *int
	Group       magpiePlanGroup
}

type magpiePlanGroup struct {
	SourceID     int64
	Name         string
	Mode         string
	Enabled      bool
	ActiveItemID int64
	Items        []magpiePlanGroupItem
}

type magpiePlanGroupItem struct {
	SourceID    int64
	GrantSource int64
	Priority    int
	Weight      int
}

type magpiePlanKey struct {
	SourceID int64
	Name     string
	Secret   string
	Enabled  bool
	Policy   store.DownstreamAccessPolicy
}

// MagpieSkippedProvider names a provider this importer will not write. IDs and
// reasons are source identifiers only; names and URLs stay out of the preview.
type MagpieSkippedProvider struct {
	SourceID string   `json:"sourceId"`
	Type     string   `json:"type"`
	Reasons  []string `json:"reasons"`
}

type MagpieSkippedGroup struct {
	SourceID string   `json:"sourceId"`
	Reasons  []string `json:"reasons"`
}

type magpieGrantRef struct {
	ProviderID string
	Model      string
	Credential string
}

func compileMagpiePlan(src *MagpieSource) (*magpiePlan, error) {
	if src == nil {
		return nil, errors.New("Magpie source is missing")
	}
	plan := &magpiePlan{notImported: map[string]int{}}
	aliases := magpieProviderAliases(src)
	groupModels := magpieGroupReferencedModels(src, aliases)
	usedNames := map[string]int{}
	modelsByProvider := map[string]map[string]bool{}
	grantsByRef := map[magpieGrantRef]magpiePlanGrant{}

	for _, sourceProvider := range src.Providers {
		extraModels := groupModels[sourceProvider.ID]
		provider, reasons, residuals := compileMagpieProvider(sourceProvider, usedNames, extraModels)
		if len(extraModels) > 0 && provider != nil {
			plan.residuals = append(plan.residuals, "provider_models_completed_from_groups")
		}
		plan.residuals = append(plan.residuals, residuals...)
		if len(reasons) > 0 {
			plan.skippedProviders = append(plan.skippedProviders, MagpieSkippedProvider{
				SourceID: sanitizeIdentifier(sourceProvider.ID), Type: "provider", Reasons: uniquifyReasons(reasons),
			})
			continue
		}
		plan.providers = append(plan.providers, *provider)
		modelsByProvider[provider.ID] = map[string]bool{}
		for _, model := range provider.Models {
			modelsByProvider[provider.ID][model] = true
		}
		plan.credentials = append(plan.credentials, provider.Credentials...)
		for _, model := range provider.Models {
			plan.models = append(plan.models, magpiePlanModel{
				SourceID: sourceTupleID("magpie-model", provider.ID, model), ProviderID: provider.ID,
				Name: model, Enabled: provider.Enabled, ContextLength: magpieContext(sourceProvider, model),
			})
			for _, credential := range provider.Credentials {
				if !magpieCredentialServes(credential, model, provider.AccountModels) {
					continue
				}
				grant := magpiePlanGrant{
					SourceID:   sourceTupleID("magpie-grant", provider.ID, model, credential.Name),
					ProviderID: provider.ID, ModelName: model, Credential: credential.Name,
					Protocols: credential.Protocols, Enabled: provider.Enabled && credential.Enabled,
				}
				plan.grants = append(plan.grants, grant)
				grantsByRef[magpieGrantRef{provider.ID, model, credential.Name}] = grant
			}
		}
	}

	if len(plan.providers) == 0 {
		plan.blocking = append(plan.blocking, "no_servable_providers")
	}
	if len(src.Providers) > 0 && !src.KeysIncluded {
		plan.residuals = append(plan.residuals, "provider_keys_not_in_backup")
	}

	// Every provider/model stays addressable by the same "provider/model" id
	// Magpie exposed. Each public route owns one internal group.
	for i := range plan.providers {
		provider := &plan.providers[i]
		if len(provider.Models) == 0 {
			plan.notImported["providersWithoutExplicitModels"]++
			plan.residuals = append(plan.residuals, "provider_models_not_explicit")
			continue
		}
		for _, model := range provider.Models {
			grants, names := magpieGrantsForModel(plan, grantsByRef, provider.ID, model)
			if len(grants) == 0 {
				plan.notImported["modelsWithoutCredentials"]++
				continue
			}
			pattern := provider.ID + "/" + model
			groupSourceID := sourceTupleID("magpie-model-group", provider.ID, model)
			group := magpiePlanGroup{
				SourceID: groupSourceID, Name: uniqueGroupName("model:"+pattern, model, usedNames),
				Mode: "failover", Enabled: provider.Enabled,
			}
			strategy := "weighted"
			switch provider.Routing {
			case "rotate":
				strategy = "round_robin"
			case "", "order", "usage", "pace", "weight":
			}
			for index, grant := range grants {
				credential := names[index]
				priority := 0
				if provider.Routing == "order" {
					priority = index
				}
				item := magpiePlanGroupItem{
					SourceID:    sourceTupleID("magpie-model-item", provider.ID, model, credential.Name),
					GrantSource: grant.SourceID, Priority: priority, Weight: max(1, credential.Weight),
				}
				group.Items = append(group.Items, item)
			}
			plan.routes = append(plan.routes, magpiePlanRoute{
				SourceID: sourceTupleID("magpie-route", pattern), Pattern: pattern,
				DisplayName: model, Strategy: strategy, Enabled: provider.Enabled,
				Context: magpieModelContext(plan, provider.ID, model), Group: group,
			})
		}
	}

	residuals := compileMagpieGroups(src, plan, aliases, modelsByProvider, grantsByRef, usedNames)
	plan.residuals = append(plan.residuals, residuals...)
	if err := compileMagpieGatewayKeys(src, plan); err != nil {
		return nil, err
	}

	plan.notImported["settings"] += boolCount(src.Sections["settings"] > 0)
	plan.notImported["profiles"] += src.Sections["profiles"]
	plan.notImported["agents"] += src.Sections["agents"]
	plan.notImported["library"] += boolCount(src.Sections["library"] > 0)
	plan.notImported["searches"] += src.Sections["searches"]
	plan.notImported["icons"] += src.Sections["icons"]
	plan.notImported["providerOrder"] += boolCount(src.Sections["order"] > 0)
	plan.notImported["groupOrder"] += boolCount(src.Sections["groupOrder"] > 0)
	for section, count := range src.Unknown {
		plan.notImported["section:"+section] += count
	}
	dropZeroCounts(plan.notImported)
	plan.residuals = uniqueSortedStrings(plan.residuals)
	sort.Slice(plan.skippedGroups, func(i, j int) bool { return plan.skippedGroups[i].SourceID < plan.skippedGroups[j].SourceID })
	return plan, nil
}

func compileMagpieProvider(src MagpieSourceProvider, usedNames map[string]int, groupModels []string) (*magpiePlanProvider, []string, []string) {
	var reasons, residuals []string
	if !magpieIdentifier(src.ID) {
		return nil, []string{"provider_id_invalid"}, nil
	}
	if src.HasVertex {
		return nil, []string{"vertex_credentials_unsupported"}, nil
	}
	switch src.Routing {
	case "", "order", "rotate", "usage", "pace", "weight":
	default:
		return nil, []string{"provider_routing_unsupported"}, nil
	}
	switch src.Routing {
	case "":
		residuals = append(residuals, "provider_routing_smart_approximated_as_weighted")
	case "usage":
		residuals = append(residuals, "provider_routing_usage_approximated_as_weighted")
	case "pace":
		residuals = append(residuals, "provider_routing_pace_approximated_as_weighted")
	}
	if affinity := strings.ToLower(strings.TrimSpace(src.Affinity)); affinity != "off" {
		residuals = append(residuals, "provider_affinity_not_imported")
	}
	if src.Sink {
		residuals = append(residuals, "provider_rate_limit_sink_not_imported")
	}
	if src.Decide != "" {
		residuals = append(residuals, "decision_api_not_imported")
	}
	if len(src.AccountProxies) > 0 {
		residuals = append(residuals, "per_key_proxies_not_imported")
	}
	if src.HasControlSecrets {
		residuals = append(residuals, "provider_control_credentials_not_imported")
	}
	if src.HasAccountControls {
		residuals = append(residuals, "subscription_account_controls_not_imported")
	}
	for _, context := range src.Contexts {
		if context == -1 {
			residuals = append(residuals, "provider_smallest_context_not_imported")
			break
		}
	}
	if len(src.UnsupportedFields) > 0 {
		residuals = append(residuals, "provider_fields_not_imported")
	}
	endpoints, protocols, reasons, endpointResiduals := magpieEndpoints(src)
	residuals = append(residuals, endpointResiduals...)
	if len(reasons) > 0 {
		return nil, reasons, residuals
	}
	base := magpieChannelBase(endpoints)
	if base == "" {
		return nil, []string{"provider_endpoint_missing"}, residuals
	}
	useProxy, channelProxy, proxyReason := magpieProxy(src)
	if proxyReason != "" {
		return nil, []string{proxyReason}, residuals
	}
	customHeader, headerResiduals := magpieCustomHeaders(src.Headers)
	residuals = append(residuals, headerResiduals...)

	keys := magpieProviderKeys(src)
	if len(keys) == 0 {
		residuals = append(residuals, "provider_has_no_importable_keys")
	}
	provider := &magpiePlanProvider{
		SourceID: sourceTupleID("magpie-provider", src.ID), ID: src.ID,
		Name: uniqueSourceName(src.Name, src.ID, usedNames), Enabled: !src.Off,
		BaseURL: base, Endpoints: endpoints, Protocols: protocols, Routing: src.Routing,
		AccountModels: src.AccountModels,
		UseProxy:      useProxy, ChannelProxy: channelProxy, CustomHeader: customHeader,
		Models: magpieServedModels(src.Models, groupModels),
	}
	usedCredentialNames := map[string]int{}
	for index, key := range keys {
		protocolMask, ok := magpieKeyProtocol(key.Protocol, protocols)
		if !ok {
			residuals = append(residuals, "provider_key_protocol_unsupported")
			continue
		}
		name := strings.TrimSpace(key.Name)
		if name == "" {
			if index == 0 {
				name = "default"
			} else {
				name = fmt.Sprintf("key-%d", index+1)
			}
		}
		name = uniqueCredentialName(name, index, usedCredentialNames)
		provider.Credentials = append(provider.Credentials, magpiePlanCredential{
			SourceID: sourceTupleID("magpie-credential", src.ID, name), ProviderID: src.ID,
			Name: name, Secret: key.Key, Protocols: protocolMask, Weight: key.Weight,
			Enabled: !src.Off && !key.Off,
		})
	}
	if len(src.AccountModels) > 0 {
		residuals = append(residuals, "per_key_model_scopes_applied")
	}
	return provider, nil, residuals
}

func magpieEndpoints(p MagpieSourceProvider) (store.DirectEndpoints, int, []string, []string) {
	var out store.DirectEndpoints
	mask := 0
	var reasons, residuals []string
	add := func(base, suffix, auth string, modelPath bool, protocol int) {
		if base == "" {
			return
		}
		endpoint, err := buildMagpieEndpoint(base, suffix, auth, modelPath)
		if err != nil {
			reasons = append(reasons, "endpoint_url_invalid")
			return
		}
		switch protocol {
		case store.DirectProtocolChat:
			out.Chat = endpoint
		case store.DirectProtocolResponses:
			out.Responses = endpoint
		case store.DirectProtocolMessages:
			out.Messages = endpoint
		case store.DirectProtocolGemini:
			out.Gemini = endpoint
		}
		mask |= protocol
	}
	add(strings.TrimRight(p.Chat, "/"), "/chat/completions", store.DirectAuthBearer, false, store.DirectProtocolChat)
	add(strings.TrimRight(p.Responses, "/"), "/responses", store.DirectAuthBearer, false, store.DirectProtocolResponses)
	add(magpieAnthropicBase(p.Anthropic), "/v1/messages", store.DirectAuthAPIKey, false, store.DirectProtocolMessages)
	add(magpieGeminiBase(p.Gemini), "", store.DirectAuthGoogle, true, store.DirectProtocolGemini)
	if mask == 0 {
		reasons = append(reasons, "provider_endpoint_missing")
	}
	if p.Anthropic != "" && !strings.HasSuffix(strings.ToLower(hostOf(p.Anthropic)), "anthropic.com") {
		residuals = append(residuals, "anthropic_relay_bearer_header_not_imported")
	}
	var checked store.DirectEndpoints
	encoded, _ := json.Marshal(out)
	if err := checked.Scan(encoded); err != nil {
		return store.DirectEndpoints{}, 0, []string{"endpoint_configuration_invalid"}, residuals
	}
	return checked, mask, uniquifyReasons(reasons), residuals
}

func buildMagpieEndpoint(base, suffix, auth string, modelPath bool) (*store.DirectEndpoint, error) {
	full := strings.TrimRight(strings.TrimSpace(base), "/") + suffix
	u, err := url.Parse(full)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("invalid endpoint URL")
	}
	if service.IsForbiddenSiteTargetURL(full) {
		return nil, errors.New("forbidden endpoint target")
	}
	return &store.DirectEndpoint{URL: full, Auth: auth, ModelPath: modelPath}, nil
}

func magpieChannelBase(endpoints store.DirectEndpoints) string {
	for _, endpoint := range []*store.DirectEndpoint{endpoints.Chat, endpoints.Responses, endpoints.Messages, endpoints.Gemini} {
		if endpoint == nil || endpoint.URL == "" {
			continue
		}
		base := endpoint.URL
		for _, suffix := range []string{"/chat/completions", "/responses", "/v1/messages"} {
			if cut, ok := strings.CutSuffix(base, suffix); ok {
				base = cut
				break
			}
		}
		return strings.TrimRight(base, "/")
	}
	return ""
}

func magpieAnthropicBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{"/v1/messages", "/v1"} {
		if cut, ok := strings.CutSuffix(base, suffix); ok && strings.Contains(cut, "://") && len(cut) > len("https://") {
			return cut
		}
	}
	return base
}

func magpieGeminiBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	if u, err := url.Parse(base); err == nil && u.Host != "" && strings.Trim(u.Path, "/") == "" {
		return base + "/v1beta"
	}
	return base
}

func magpieProxy(p MagpieSourceProvider) (bool, string, string) {
	proxy := strings.TrimSpace(p.Proxy)
	if proxy == "" {
		return true, "", ""
	}
	if proxy == "direct" {
		return false, "", ""
	}
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Host == "" {
		return false, "", "proxy_url_invalid"
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return false, "", "proxy_url_invalid"
	}
	return false, proxy, ""
}

func magpieCustomHeaders(headers map[string]string) (string, []string) {
	type header struct {
		Key   string `json:"header_key"`
		Value string `json:"header_value"`
	}
	var out []header
	var residuals []string
	for _, key := range sortedStringKeys(headers) {
		value := headers[key]
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "authorization", "x-api-key", "x-goog-api-key":
			residuals = append(residuals, "auth_override_headers_not_imported")
			continue
		}
		if !validHTTPHeaderName(key) || strings.ContainsAny(value, "\r\n") {
			residuals = append(residuals, "invalid_custom_headers_not_imported")
			continue
		}
		out = append(out, header{Key: key, Value: value})
	}
	if len(out) == 0 {
		return "[]", residuals
	}
	encoded, _ := json.Marshal(out)
	return string(encoded), residuals
}

func magpieProviderKeys(p MagpieSourceProvider) []MagpieSourceKey {
	var out []MagpieSourceKey
	if p.Key != "" {
		out = append(out, MagpieSourceKey{Name: p.KeyName, Key: p.Key, Protocol: p.KeyProtocol, Weight: p.KeyWeight})
	}
	for _, key := range p.Keys {
		if key.Key != "" {
			out = append(out, key)
		}
	}
	return out
}

func magpieKeyProtocol(protocol string, channelMask int) (int, bool) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return channelMask, true
	}
	bit := map[string]int{
		"chat": store.DirectProtocolChat, "responses": store.DirectProtocolResponses,
		"anthropic": store.DirectProtocolMessages, "gemini": store.DirectProtocolGemini,
	}[protocol]
	if bit == 0 || channelMask&bit == 0 {
		return 0, false
	}
	return bit, true
}

func magpieGrantsForModel(plan *magpiePlan, grants map[magpieGrantRef]magpiePlanGrant, providerID, model string) ([]magpiePlanGrant, []magpiePlanCredential) {
	provider := magpieProviderByID(plan, providerID)
	if provider == nil {
		return nil, nil
	}
	var out []magpiePlanGrant
	var credentials []magpiePlanCredential
	for _, credential := range provider.Credentials {
		grant, ok := grants[magpieGrantRef{providerID, model, credential.Name}]
		if !ok {
			continue
		}
		out = append(out, grant)
		credentials = append(credentials, credential)
	}
	return out, credentials
}

func magpieCredentialServes(credential magpiePlanCredential, model string, accountModels map[string][]string) bool {
	allowed := accountModels[magpieKeyFingerprint(credential.Secret)]
	if len(allowed) == 0 {
		return true
	}
	for _, pattern := range allowed {
		if pattern == model || magpieGlobMatch(pattern, model) {
			return true
		}
	}
	return false
}

func magpieContext(p MagpieSourceProvider, model string) *int {
	if p.Contexts == nil {
		return nil
	}
	if value := p.Contexts[model]; value > 0 {
		return &value
	}
	if value := p.Contexts["*"]; value > 0 {
		return &value
	}
	return nil
}

func magpieModelContext(plan *magpiePlan, providerID, model string) *int {
	for _, planned := range plan.models {
		if planned.ProviderID == providerID && planned.Name == model {
			return planned.ContextLength
		}
	}
	return nil
}

func compileMagpieGroups(src *MagpieSource, plan *magpiePlan, aliases map[string]string, models map[string]map[string]bool, grants map[magpieGrantRef]magpiePlanGrant, usedNames map[string]int) []string {
	var residuals []string
	groups := make(map[string]MagpieSourceGroup, len(src.Groups))
	for _, group := range src.Groups {
		groups[group.ID] = group
	}
	for _, group := range src.Groups {
		if group.Hidden {
			plan.notImported["hiddenGroups"]++
			continue
		}
		if !magpieIdentifier(group.ID) {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_id_invalid"}})
			continue
		}
		switch group.Routing {
		case "", "manual", "order", "rotate", "usage", "pace", "weight":
		default:
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_routing_unsupported"}})
			continue
		}
		members, memberResiduals := magpieGroupMembers(group, groups, aliases, models, map[string]bool{group.ID: true})
		residuals = append(residuals, memberResiduals...)
		if len(group.Matched) > 0 {
			residuals = append(residuals, "group_matched_snapshot_ignored")
		}
		if len(group.Match) > 0 {
			residuals = append(residuals, "group_match_expanded_to_snapshot")
		}
		if group.HasRules || group.Classifier != "" || group.Effort != "" {
			residuals = append(residuals, "group_rules_and_classifier_not_imported")
		}
		if affinity := strings.ToLower(strings.TrimSpace(group.Affinity)); affinity != "off" {
			residuals = append(residuals, "group_affinity_not_imported")
		}
		if group.Sink {
			residuals = append(residuals, "group_rate_limit_sink_not_imported")
		}
		if group.FirstToken != 0 || group.Patience != 0 {
			residuals = append(residuals, "group_timeouts_not_imported")
		}
		if len(group.Levels) > 0 || len(group.Fast) > 0 {
			residuals = append(residuals, "group_reasoning_controls_not_imported")
		}
		if group.Family != "" {
			residuals = append(residuals, "group_family_not_imported")
		}
		if group.Auto {
			residuals = append(residuals, "group_auto_flag_not_imported")
		}
		if len(group.UnsupportedFields) > 0 {
			residuals = append(residuals, "group_fields_not_imported")
		}
		if group.Context == -1 {
			residuals = append(residuals, "group_smallest_context_not_imported")
		}
		if group.Pick != "" && group.Routing != "manual" {
			residuals = append(residuals, "group_manual_pick_not_imported")
		}
		if len(group.Off) > 0 {
			residuals = append(residuals, "disabled_group_members_omitted")
		}
		if len(members) == 0 {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_has_no_servable_members"}})
			continue
		}
		pattern := "group/" + group.ID
		groupPlan := magpiePlanGroup{
			SourceID: sourceTupleID("magpie-group", group.ID),
			Name:     uniqueGroupName(group.Name, group.ID, usedNames), Mode: "failover",
			Enabled: !group.Disabled,
		}
		strategy := "weighted"
		switch group.Routing {
		case "rotate":
			strategy = "round_robin"
		case "manual":
			groupPlan.Mode = "manual"
		case "":
			residuals = append(residuals, "group_routing_smart_approximated_as_weighted")
		case "usage":
			residuals = append(residuals, "group_routing_usage_approximated_as_weighted")
		case "pace":
			residuals = append(residuals, "group_routing_pace_approximated_as_weighted")
		}
		seenGrants := map[int64]bool{}
		pickProvider, pickModel := "", ""
		if group.Pick != "" {
			if providerID, model, _, ok := resolveMagpieMember(group.Pick, aliases, models); ok {
				pickProvider, pickModel = providerID, model
			}
		}
		for memberIndex, member := range members {
			ref := magpieGrantRef{member.ProviderID, member.Model, ""}
			provider := magpieProviderByID(plan, member.ProviderID)
			if provider == nil {
				continue
			}
			for _, credential := range provider.Credentials {
				grant, ok := grants[magpieGrantRef{ref.ProviderID, ref.Model, credential.Name}]
				if !ok || seenGrants[grant.SourceID] {
					continue
				}
				seenGrants[grant.SourceID] = true
				priority := 0
				if group.Routing == "order" {
					priority = memberIndex
				}
				item := magpiePlanGroupItem{
					SourceID:    sourceTupleID("magpie-group-item", group.ID, fmt.Sprint(memberIndex), credential.Name),
					GrantSource: grant.SourceID, Priority: priority, Weight: max(1, credential.Weight),
				}
				groupPlan.Items = append(groupPlan.Items, item)
				if group.Routing == "manual" && member.ProviderID == pickProvider && member.Model == pickModel && groupPlan.ActiveItemID == 0 {
					groupPlan.ActiveItemID = item.SourceID
				}
			}
		}
		if len(groupPlan.Items) == 0 {
			plan.skippedGroups = append(plan.skippedGroups, MagpieSkippedGroup{SourceID: sanitizeIdentifier(group.ID), Reasons: []string{"group_has_no_importable_grants"}})
			continue
		}
		if groupPlan.Mode == "manual" && groupPlan.ActiveItemID == 0 {
			groupPlan.ActiveItemID = groupPlan.Items[0].SourceID
			residuals = append(residuals, "manual_group_pick_defaulted_to_first_member")
		}
		var context *int
		if group.Context > 0 {
			value := group.Context
			context = &value
		}
		plan.routes = append(plan.routes, magpiePlanRoute{
			SourceID: sourceTupleID("magpie-route", pattern), Pattern: pattern,
			DisplayName: groupPlan.Name, Strategy: strategy, Enabled: groupPlan.Enabled,
			Context: context, Group: groupPlan,
		})
	}
	return residuals
}

type magpieResolvedMember struct {
	ProviderID string
	Model      string
	Ref        string
}

func magpieGroupMembers(group MagpieSourceGroup, groups map[string]MagpieSourceGroup, aliases map[string]string, models map[string]map[string]bool, visiting map[string]bool) ([]magpieResolvedMember, []string) {
	var out []magpieResolvedMember
	var residuals []string
	seen := map[string]bool{}
	off := map[string]bool{}
	for _, member := range group.Off {
		if providerID, model, _, ok := resolveMagpieMember(member, aliases, models); ok {
			off[providerID+"/"+model] = true
		}
	}
	var add func(ref string) bool = func(ref string) bool {
		ref = strings.TrimSpace(ref)
		if groupID, ok := strings.CutPrefix(ref, "group/"); ok {
			nested, exists := groups[groupID]
			if !exists || visiting[groupID] || nested.Hidden || nested.Disabled {
				residuals = append(residuals, "nested_group_not_imported")
				return false
			}
			visiting[groupID] = true
			nestedMembers, nestedResiduals := magpieGroupMembers(nested, groups, aliases, models, visiting)
			delete(visiting, groupID)
			residuals = append(residuals, nestedResiduals...)
			residuals = append(residuals, "nested_group_flattened")
			for _, member := range nestedMembers {
				if !seen[member.Ref] {
					seen[member.Ref] = true
					out = append(out, member)
				}
			}
			return len(nestedMembers) > 0
		}
		providerID, model, effort, ok := resolveMagpieMember(ref, aliases, models)
		if !ok {
			residuals = append(residuals, "group_member_not_servable")
			return false
		}
		if effort != "" {
			residuals = append(residuals, "group_member_effort_not_imported")
			ref = providerID + "/" + model
		}
		if off[providerID+"/"+model] {
			residuals = append(residuals, "disabled_group_members_omitted")
			return false
		}
		if seen[ref] {
			residuals = append(residuals, "duplicate_group_member_ignored")
			return false
		}
		seen[ref] = true
		out = append(out, magpieResolvedMember{ProviderID: providerID, Model: model, Ref: ref})
		return true
	}
	for _, member := range group.Members {
		add(member)
	}
	if len(group.Match) > 0 {
		var patterns []*regexp.Regexp
		for _, pattern := range group.Match {
			if re, err := compileMagpiePattern(pattern); err == nil {
				patterns = append(patterns, re)
			} else {
				residuals = append(residuals, "group_match_pattern_invalid")
			}
		}
		providerIDs := make([]string, 0, len(models))
		for providerID := range models {
			providerIDs = append(providerIDs, providerID)
		}
		sort.Strings(providerIDs)
		for _, providerID := range providerIDs {
			modelNames := make([]string, 0, len(models[providerID]))
			for model := range models[providerID] {
				modelNames = append(modelNames, model)
			}
			sort.Strings(modelNames)
			for _, model := range modelNames {
				id := providerID + "/" + model
				for _, pattern := range patterns {
					if pattern.MatchString(id) {
						add(id)
						break
					}
				}
			}
		}
	}
	return out, residuals
}

func resolveMagpieMember(ref string, aliases map[string]string, models map[string]map[string]bool) (string, string, string, bool) {
	ref = strings.TrimSpace(ref)
	if _, _, ok := strings.Cut(ref, "/"); !ok {
		return "", "", "", false
	}
	base, effort := trimMagpieEffort(ref)
	for _, candidate := range []string{ref, base} {
		providerID, model, ok := strings.Cut(candidate, "/")
		if !ok {
			continue
		}
		if mapped := aliases[strings.ToLower(providerID)]; mapped != "" {
			providerID = mapped
		}
		if models[providerID][model] {
			if candidate == ref && effort != "" {
				effort = ""
			}
			return providerID, model, effort, true
		}
	}
	return "", "", "", false
}

var magpieEfforts = map[string]bool{"none": true, "minimal": true, "low": true, "medium": true, "high": true, "xhigh": true, "max": true}

func trimMagpieEffort(ref string) (string, string) {
	index := strings.LastIndex(ref, ":")
	if index <= 0 {
		return ref, ""
	}
	effort := strings.ToLower(ref[index+1:])
	if !magpieEfforts[effort] {
		return ref, ""
	}
	return ref[:index], effort
}

func compileMagpiePattern(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(pattern)
	if re, ok := strings.CutPrefix(pattern, "re:"); ok {
		if strings.TrimSpace(re) == "" {
			return nil, errors.New("empty regex")
		}
		return regexp.Compile("^(?:" + re + ")$")
	}
	parts := strings.Split(pattern, "*")
	for i, part := range parts {
		parts[i] = regexp.QuoteMeta(part)
	}
	return regexp.Compile("(?i)^" + strings.Join(parts, ".*") + "$")
}

func compileMagpieGatewayKeys(src *MagpieSource, plan *magpiePlan) error {
	routePatterns := map[string]bool{}
	for _, route := range plan.routes {
		routePatterns[route.Pattern] = true
	}
	for _, key := range src.GatewayKeys {
		if key.Secret == "" {
			plan.notImported["gatewayKeysWithoutSecret"]++
			continue
		}
		name := key.Name
		if name == "" {
			name = "Magpie"
		}
		compiled := magpiePlanKey{
			SourceID: sourceTupleID("magpie-gateway-key", key.ID),
			Name:     name, Secret: key.Secret, Enabled: !key.Off,
		}
		if len(key.Accounts) > 0 {
			compiled.Policy.BlockReason = "source_key_account_scope_unsupported"
		}
		if len(key.UnsupportedFields) > 0 {
			plan.residuals = append(plan.residuals, "gateway_key_fields_not_imported")
		}
		if key.LAN {
			plan.residuals = append(plan.residuals, "gateway_key_lan_role_not_imported")
		}
		if len(key.Models) > 0 {
			models, unresolved := expandMagpieKeyModels(key.Models, routePatterns)
			if unresolved > 0 {
				compiled.Policy.BlockReason = "source_key_model_scope_unresolved"
				plan.residuals = append(plan.residuals, "gateway_key_model_scope_unresolved")
			} else {
				compiled.Policy.ModelIDs = models
			}
		}
		if key.Limit != nil {
			quota, residuals := magpieQuota(key.Limit, src.Created)
			compiled.Policy.Quota = quota
			plan.residuals = append(plan.residuals, residuals...)
			if quota == nil {
				compiled.Policy.BlockReason = "source_key_quota_unsupported"
			}
		}
		if err := compiled.Policy.Validate(); err != nil {
			compiled.Policy.ModelIDs = nil
			compiled.Policy.Quota = nil
			compiled.Policy.BlockReason = "source_key_policy_unsupported"
			plan.residuals = append(plan.residuals, "gateway_key_policy_blocked")
		}
		if compiled.Policy.BlockReason != "" {
			plan.residuals = append(plan.residuals, compiled.Policy.BlockReason)
		}
		plan.keys = append(plan.keys, compiled)
	}
	return nil
}

func expandMagpieKeyModels(patterns []string, routes map[string]bool) ([]string, int) {
	seen := map[string]bool{}
	var out []string
	unresolved := 0
	for _, pattern := range patterns {
		matches := 0
		for route := range routes {
			if route == pattern || magpieGlobMatch(pattern, route) {
				if !seen[route] {
					seen[route] = true
					out = append(out, route)
				}
				matches++
			}
		}
		if matches == 0 {
			unresolved++
		}
	}
	sort.Strings(out)
	return out, unresolved
}

func magpieQuota(limit *MagpieSourceLimit, created time.Time) (*store.DownstreamQuota, []string) {
	if limit == nil || (limit.Tokens <= 0 && limit.Cost <= 0) {
		return nil, nil
	}
	q := &store.DownstreamQuota{Period: store.DownstreamQuotaPeriod{Type: "calendar_duration"}}
	if limit.Tokens > 0 {
		v := limit.Tokens
		q.TotalTokens = &v
	}
	if limit.Cost > 0 {
		v := limit.Cost
		q.Cost = &v
	}
	var residuals []string
	switch limit.Period {
	case "", "day":
		q.Period.CalendarDuration = &store.DownstreamQuotaDuration{Value: 1, Unit: "day"}
	case "month":
		q.Period.CalendarDuration = &store.DownstreamQuotaDuration{Value: 1, Unit: "month"}
	case "week":
		q.Period = store.DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &store.DownstreamQuotaDuration{Value: 7, Unit: "day"}}
		residuals = append(residuals, "gateway_key_week_quota_approximated_as_rolling_7_days")
	case "days":
		if limit.Days <= 0 {
			residuals = append(residuals, "gateway_key_quota_period_unsupported")
			return nil, residuals
		}
		q.Period = store.DownstreamQuotaPeriod{Type: "past_duration", PastDuration: &store.DownstreamQuotaDuration{Value: int64(limit.Days), Unit: "day"}}
		residuals = append(residuals, "gateway_key_cycle_quota_approximated_as_rolling")
	default:
		residuals = append(residuals, "gateway_key_quota_period_unsupported")
		return nil, residuals
	}
	if limit.CacheReads {
		residuals = append(residuals, "gateway_key_cache_read_accounting_not_imported")
	}
	if limit.HasSince {
		residuals = append(residuals, "gateway_key_quota_cycle_anchor_not_imported")
	}
	if !created.IsZero() {
		ms := created.UnixMilli()
		q.HistoryMissingBefore = &ms
		residuals = append(residuals, "gateway_key_quota_history_not_imported")
	}
	return q, residuals
}

func magpieProviderAliases(src *MagpieSource) map[string]string {
	aliases := map[string]string{}
	for _, provider := range src.Providers {
		aliases[provider.ID] = provider.ID
	}
	for _, provider := range src.Providers {
		for _, alias := range provider.Was {
			alias = strings.ToLower(strings.TrimSpace(alias))
			if alias != "" {
				aliases[alias] = provider.ID
			}
		}
	}
	return aliases
}

// magpieGroupReferencedModels collects every "provider/model" a routing group
// names explicitly. Magpie serves those models even when the provider left
// them out of its exposed Models list (an unlisted provider serves only
// through its groups), so dropping them would silently empty the group.
func magpieGroupReferencedModels(src *MagpieSource, aliases map[string]string) map[string][]string {
	known := make(map[string]string, len(src.Providers))
	for _, provider := range src.Providers {
		known[provider.ID] = provider.ID
		lower := strings.ToLower(provider.ID)
		if _, exists := known[lower]; !exists {
			known[lower] = provider.ID
		}
	}
	referenced := map[string]map[string]bool{}
	add := func(ref string) {
		ref = strings.TrimSpace(ref)
		if ref == "" || strings.HasPrefix(ref, "group/") {
			return
		}
		base, _ := trimMagpieEffort(ref)
		providerID, model, ok := strings.Cut(base, "/")
		if !ok {
			return
		}
		providerID, model = strings.TrimSpace(providerID), strings.TrimSpace(model)
		if providerID == "" || model == "" {
			return
		}
		if mapped := aliases[strings.ToLower(providerID)]; mapped != "" {
			providerID = mapped
		}
		canonical, exists := known[providerID]
		if !exists {
			canonical, exists = known[strings.ToLower(providerID)]
		}
		if !exists {
			return
		}
		if referenced[canonical] == nil {
			referenced[canonical] = map[string]bool{}
		}
		referenced[canonical][model] = true
	}
	for _, group := range src.Groups {
		if group.Hidden {
			continue
		}
		for _, member := range group.Members {
			add(member)
		}
		for _, member := range group.Off {
			add(member)
		}
		add(group.Pick)
	}
	models := make(map[string][]string, len(referenced))
	for providerID, set := range referenced {
		list := make([]string, 0, len(set))
		for model := range set {
			list = append(list, model)
		}
		sort.Strings(list)
		models[providerID] = list
	}
	return models
}

// magpieServedModels keeps the provider's exposed models in their declared
// order and appends the models its groups name that the list left out.
func magpieServedModels(exposed, fromGroups []string) []string {
	seen := make(map[string]bool, len(exposed)+len(fromGroups))
	models := make([]string, 0, len(exposed)+len(fromGroups))
	for _, model := range exposed {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	for _, model := range fromGroups {
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	return models
}

func magpieProviderByID(plan *magpiePlan, id string) *magpiePlanProvider {
	for i := range plan.providers {
		if plan.providers[i].ID == id {
			return &plan.providers[i]
		}
	}
	return nil
}

func uniqueCredentialName(name string, index int, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("key-%d", index+1)
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	for suffix := index + 1; ; suffix++ {
		candidate := fmt.Sprintf("%s#%d", name, suffix)
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate
		}
	}
}

func uniqueGroupName(name, fallback string, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fallback
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s#%s", name, fallback)
		if suffix > 2 {
			candidate = fmt.Sprintf("%s#%s-%d", name, fallback, suffix)
		}
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate
		}
	}
}

func uniqueSourceName(name, fallback string, used map[string]int) string {
	if strings.TrimSpace(fallback) == "" {
		fallback = "source"
	}
	return uniqueGroupName(name, fallback, used)
}

func magpieIdentifier(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

func magpieGlobMatch(pattern, value string) bool {
	re, err := compileMagpiePattern(pattern)
	return err == nil && re.MatchString(value)
}

func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func sortedStringKeys(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func magpieKeyFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:5])
}

func magpiePlanSourceIDs(plan *magpiePlan) map[string]map[int64]bool {
	ids := map[string]map[int64]bool{}
	mark := func(entity string, sourceID int64) {
		if ids[entity] == nil {
			ids[entity] = map[int64]bool{}
		}
		ids[entity][sourceID] = true
	}
	for _, provider := range plan.providers {
		mark("upstream_channels", provider.SourceID)
	}
	for _, credential := range plan.credentials {
		mark("upstream_credentials", credential.SourceID)
	}
	for _, model := range plan.models {
		mark("upstream_models", model.SourceID)
	}
	for _, grant := range plan.grants {
		mark("upstream_grants", grant.SourceID)
	}
	for _, route := range plan.routes {
		mark("token_routes", route.SourceID)
		mark("upstream_groups", route.Group.SourceID)
		for _, item := range route.Group.Items {
			mark("upstream_group_items", item.SourceID)
		}
	}
	for _, key := range plan.keys {
		mark("downstream_api_keys", key.SourceID)
	}
	return ids
}

func magpiePlanCounts(plan *magpiePlan) map[string]int {
	return map[string]int{
		"channels": len(plan.providers), "credentials": len(plan.credentials),
		"models": len(plan.models), "grants": len(plan.grants),
		"routes": len(plan.routes), "groups": len(plan.routes), "gatewayKeys": len(plan.keys),
	}
}

func magpieSourceSections(src *MagpieSource) map[string]int {
	out := map[string]int{}
	for key, value := range src.Sections {
		out[key] = value
	}
	for key, value := range src.Unknown {
		out["section:"+key] = value
	}
	return out
}
