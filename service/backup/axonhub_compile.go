package backup

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// axonHubPlan is the single compiled result shared by the preview and the
// transaction executor. Both paths render exactly what this produced, so a
// preview can never claim a channel the import will not write.
type axonHubPlan struct {
	keys        []axonHubPlanAPIKey
	channels    []axonHubPlanChannel
	credentials []axonHubPlanCredential
	models      []axonHubPlanModel
	grants      []axonHubPlanGrant
	routes      []axonHubPlanRoute

	notImported map[string]int
	skipped     []AxonHubSkippedChannel
	residuals   []string
	// blocking holds reasons this backup cannot be imported at all. Skip
	// reasons stay per channel so a partial import is still reviewable.
	blocking []string
}

type axonHubPlanCredential struct {
	ChannelSourceID int
	Name            string
	Secret          string
	Kind            string
	OAuthState      store.DirectOAuthState
	// GCPMaterial is the service-account JSON for gcp_service_account
	// credentials; Secret stays empty until the first token exchange.
	GCPMaterial string
	Enabled     bool
}

type axonHubPlanModel struct {
	ChannelSourceID int
	Name            string
	Enabled         bool
}

type axonHubPlanGrant struct {
	ChannelSourceID int
	ModelName       string
	CredentialName  string
	Protocols       int
	Enabled         bool
}

type axonHubPlanRoute struct {
	SourceModelID   int
	Pattern         string
	GroupKey        string
	Enabled         bool
	Strategy        string
	TraceStickyMode string
	Items           []axonHubPlanRouteItem
}

type axonHubPlanRouteItem struct {
	AssociationConditions store.DirectAssociationConditions
	ProtocolOrder         store.DirectProtocolOrder
	Key                   string
	ChannelSourceID       int
	ModelName             string
	CredentialName        string
	Priority              int
}

// CompileAxonHubPlan turns a parsed backup into the exact channel graph the
// executor will persist. It never guesses: a feature it cannot reproduce
// excludes the channel that carries it and says why.
func CompileAxonHubPlan(src *AxonHubSource) (*axonHubPlan, error) {
	if src == nil {
		return nil, axonHubErr("AxonHub source is missing")
	}
	plan := &axonHubPlan{
		notImported: map[string]int{},
	}
	channels := make([]*axonHubPlanChannel, 0, len(src.Channels))
	bySourceID := make(map[int]*axonHubPlanChannel, len(src.Channels))
	usedChannelNames := map[string]int{}

	for _, channel := range src.Channels {
		if channel.DeletedAt != 0 {
			plan.notImported["deletedChannels"]++
			continue
		}
		compiled, reasons, protocolResiduals := compileAxonHubChannel(channel)
		plan.residuals = append(plan.residuals, protocolResiduals...)
		if len(reasons) > 0 {
			plan.skipped = append(plan.skipped, AxonHubSkippedChannel{
				SourceID: channel.ID,
				Type:     sanitizeIdentifier(channel.Type),
				Reasons:  reasons,
			})
			continue
		}
		compiled.Name = uniqueChannelName(compiled.Name, channel.ID, usedChannelNames)
		channels = append(channels, compiled)
		bySourceID[channel.ID] = compiled
	}
	if len(channels) == 0 {
		plan.blocking = append(plan.blocking, "no_servable_channels")
	}

	plan.channels = derefChannels(channels)
	for _, channel := range plan.channels {
		if channel.GCPMaterial != "" {
			plan.credentials = append(plan.credentials, axonHubPlanCredential{
				ChannelSourceID: channel.SourceID,
				Name:            credentialName(0, 1),
				Kind:            store.DirectCredentialGCPServiceAccount,
				GCPMaterial:     channel.GCPMaterial,
				Enabled:         true,
			})
			continue
		}
		for i, secret := range channel.CredentialKeys {
			kind := store.DirectCredentialAPIKey
			if secret == "" {
				kind = store.DirectCredentialNone
			}
			state := store.DirectOAuthState{}
			if channel.OAuthState != nil {
				kind = store.DirectCredentialOAuth
				state = *channel.OAuthState
			}
			plan.credentials = append(plan.credentials, axonHubPlanCredential{
				ChannelSourceID: channel.SourceID,
				Name:            credentialName(i, len(channel.CredentialKeys)),
				Secret:          secret,
				Kind:            kind,
				OAuthState:      state,
				Enabled:         !channel.DisabledKeys[secret],
			})
		}
	}

	routes, residuals := compileAxonHubRoutes(src, channels, bySourceID)
	plan.routes = routes
	plan.residuals = append(plan.residuals, residuals...)
	sort.Strings(plan.residuals)

	usedModels := map[string]bool{}
	// A grant is one (channel, model, credential) triple: upstream_grants is
	// unique on (model_id, credential_id), so two models on one channel each
	// need their own row even when they share a key.
	usedGrants := map[string]bool{}
	for _, route := range plan.routes {
		if !route.Enabled {
			plan.notImported["disabledModelRoutes"]++
		}
		for _, item := range route.Items {
			modelKey := fmt.Sprintf("%d\x00%s", item.ChannelSourceID, item.ModelName)
			if !usedModels[modelKey] {
				usedModels[modelKey] = true
				channel := bySourceID[item.ChannelSourceID]
				plan.models = append(plan.models, axonHubPlanModel{
					ChannelSourceID: item.ChannelSourceID,
					Name:            item.ModelName,
					Enabled:         channel.Enabled,
				})
			}
			grantKey := fmt.Sprintf("%d\x00%s\x00%s", item.ChannelSourceID, item.ModelName, item.CredentialName)
			if usedGrants[grantKey] {
				continue
			}
			usedGrants[grantKey] = true
			channel := bySourceID[item.ChannelSourceID]
			plan.grants = append(plan.grants, axonHubPlanGrant{
				ChannelSourceID: item.ChannelSourceID,
				ModelName:       item.ModelName,
				CredentialName:  item.CredentialName,
				Protocols:       channel.Protocols,
				Enabled:         true,
			})
		}
	}

	if err := compileAxonHubAccess(src, plan); err != nil {
		return nil, err
	}
	plan.notImported["modelPrices"] = len(src.ChannelModelPrices)
	plan.notImported["systemConfigs"] = len(src.SystemConfigs)
	plan.notImported["usageRequests"] = src.UsageRequests
	plan.notImported["usageLogs"] = src.UsageLogs
	for section, count := range src.UnknownSections {
		plan.notImported["section:"+section] += count
	}
	dropZeroCounts(plan.notImported)
	return plan, nil
}

func derefChannels(channels []*axonHubPlanChannel) []axonHubPlanChannel {
	out := make([]axonHubPlanChannel, 0, len(channels))
	for _, channel := range channels {
		out = append(out, *channel)
	}
	return out
}

func credentialName(index, total int) string {
	if total <= 1 {
		return "default"
	}
	return fmt.Sprintf("key-%d", index+1)
}

// compileAxonHubRoutes resolves every model's associations into concrete
// channel/model candidates and emits one exact route per requested model name.
func compileAxonHubRoutes(src *AxonHubSource, channels []*axonHubPlanChannel, bySourceID map[int]*axonHubPlanChannel) ([]axonHubPlanRoute, []string) {
	var routes []axonHubPlanRoute
	var residuals []string
	for _, model := range src.Models {
		if model.DeletedAt != 0 {
			continue
		}
		pattern := strings.TrimSpace(model.ModelID)
		if pattern == "" {
			continue
		}
		if !routing.IsExactRouteModelPattern(pattern) {
			residuals = append(residuals, "model_pattern_not_exact:"+sanitizeIdentifier(pattern))
			continue
		}
		if len(model.Settings.UnsupportedFields) > 0 {
			residuals = append(residuals, "model_settings_unsupported:"+sanitizeIdentifier(pattern))
			continue
		}
		switch model.Type {
		case "", "chat", "embedding", "rerank", "image_generation", "video_generation":
		default:
			residuals = append(residuals, "model_type_not_servable:"+sanitizeIdentifier(model.Type))
			continue
		}

		items := map[string]axonHubPlanRouteItem{}
		var order []string
		for _, assoc := range axonHubEffectiveAssociations(src, model) {
			if assoc.Disabled {
				continue
			}
			if assoc.UnsupportedField != "" {
				residuals = append(residuals, "model_association_field_unsupported:"+assoc.UnsupportedField)
				continue
			}
			connections, ok := matchAxonHubAssociation(assoc, channels, bySourceID)
			if !ok {
				residuals = append(residuals, "model_association_invalid:"+sanitizeIdentifier(pattern))
				continue
			}
			for _, connection := range connections {
				channel := bySourceID[connection.channelSourceID]
				if channel == nil {
					continue
				}
				protocolOrder, servable := axonHubModelProtocolOrder(channel, pattern, connection.requestModel, connection.modelName)
				if servable && !axonHubMediaModelHasEndpoint(model.Type, protocolOrder) {
					servable = false
				}
				if !servable {
					residuals = append(residuals, "model_protocol_not_servable:"+sanitizeIdentifier(pattern))
					continue
				}
				for i := range channel.CredentialKeys {
					name := credentialName(i, len(channel.CredentialKeys))
					if channel.DisabledKeys[channel.CredentialKeys[i]] {
						continue
					}
					key := fmt.Sprintf("%d\x00%s\x00%s", connection.channelSourceID, name, connection.modelName)
					rule := store.DirectAssociationRule{Priority: int64(assoc.Priority), ProtocolOrder: protocolOrder, Condition: assoc.Condition}
					if previous, exists := items[key]; exists {
						previous.AssociationConditions = append(previous.AssociationConditions, rule)
						if previous.Priority > assoc.Priority {
							previous.Priority = assoc.Priority
						}
						// The stored order is the maximum allowed item scope. Each
						// request narrows it to its first matching association.
						for _, bit := range protocolOrder {
							if !slices.Contains(previous.ProtocolOrder, bit) {
								previous.ProtocolOrder = append(previous.ProtocolOrder, bit)
							}
						}
						items[key] = previous
						continue
					} else {
						order = append(order, key)
					}
					items[key] = axonHubPlanRouteItem{
						AssociationConditions: store.DirectAssociationConditions{rule},
						ProtocolOrder:         protocolOrder,
						Key:                   key,
						ChannelSourceID:       connection.channelSourceID,
						ModelName:             connection.modelName,
						CredentialName:        name,
						Priority:              assoc.Priority,
					}
				}
			}
		}
		if len(items) == 0 {
			residuals = append(residuals, "model_has_no_importable_channel:"+sanitizeIdentifier(pattern))
			continue
		}
		strategy, sticky := axonHubModelRouting(src, model, &residuals, pattern)
		route := axonHubPlanRoute{
			SourceModelID:   model.ID,
			Pattern:         pattern,
			GroupKey:        pattern,
			Enabled:         model.Status == "" || model.Status == "enabled",
			Strategy:        strategy,
			TraceStickyMode: sticky,
		}
		sort.Strings(order)
		conditionsValid := true
		for _, key := range order {
			item := items[key]
			sort.SliceStable(item.AssociationConditions, func(i, j int) bool {
				return item.AssociationConditions[i].Priority < item.AssociationConditions[j].Priority
			})
			hasCondition := false
			for _, rule := range item.AssociationConditions {
				hasCondition = hasCondition || rule.Condition != nil
			}
			if !hasCondition {
				item.ProtocolOrder = item.AssociationConditions[0].ProtocolOrder
				item.AssociationConditions = nil
			}
			if item.AssociationConditions.Validate() != nil {
				residuals = append(residuals, "model_association_conditions_unsupported:"+sanitizeIdentifier(pattern))
				conditionsValid = false
				break
			}
			route.Items = append(route.Items, item)
		}
		if !conditionsValid {
			continue
		}
		routes = append(routes, route)
	}
	return routes, residuals
}

func axonHubRouteStrategy(strategy string, residuals *[]string, pattern string) string {
	switch strategy {
	case "", "default", "adaptive":
		if strategy == "adaptive" {
			*residuals = append(*residuals, "model_strategy_approximated:adaptive:"+sanitizeIdentifier(pattern))
		}
		return "weighted"
	case "round-robin":
		*residuals = append(*residuals, "source_round_robin_replaced_by_gateway_rotation")
		return "round_robin"
	default:
		*residuals = append(*residuals, "model_strategy_approximated:"+sanitizeIdentifier(strategy)+":"+sanitizeIdentifier(pattern))
		return "weighted"
	}
}

type axonHubConnection struct {
	channelSourceID int
	modelName       string
	requestModel    string
}

// matchAxonHubAssociation is a faithful port of AxonHub's six association
// matchers. The second result reports a structural error (an unknown type or an
// unusable regex) rather than an empty match.
func matchAxonHubAssociation(assoc AxonHubSourceAssociation, channels []*axonHubPlanChannel, bySourceID map[int]*axonHubPlanChannel) ([]axonHubConnection, bool) {
	switch assoc.Type {
	case "channel_model":
		if assoc.ChannelModel == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelModel.ChannelID]
		if channel == nil {
			return nil, true
		}
		if _, ok := channel.Entries[assoc.ChannelModel.ModelID]; !ok {
			return nil, true
		}
		return []axonHubConnection{{assoc.ChannelModel.ChannelID, channel.Entries[assoc.ChannelModel.ModelID], assoc.ChannelModel.ModelID}}, true
	case "channel_regex":
		if assoc.ChannelRegex == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelRegex.ChannelID]
		if channel == nil {
			return nil, true
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelRegex.Pattern)
		if err != nil {
			return nil, false
		}
		return matchEntries(channel.SourceID, channel.Entries, matcher), true
	case "regex":
		if assoc.Regex == nil {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.Regex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.Regex.Exclude) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	case "model":
		if assoc.ModelID == nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.ModelID.Exclude) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ModelID.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual, assoc.ModelID.ModelID})
			}
		}
		return out, true
	case "channel_tags_model":
		if assoc.ChannelTagsModel == nil || len(assoc.ChannelTagsModel.ChannelTags) == 0 {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsModel.ChannelTags) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ChannelTagsModel.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual, assoc.ChannelTagsModel.ModelID})
			}
		}
		return out, true
	case "channel_tags_regex":
		if assoc.ChannelTagsRegex == nil || len(assoc.ChannelTagsRegex.ChannelTags) == 0 {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelTagsRegex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsRegex.ChannelTags) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	default:
		return nil, false
	}
}

func matchEntries(channelSourceID int, entries map[string]string, matcher func(string) bool) []axonHubConnection {
	names := make([]string, 0, len(entries))
	for requestModel := range entries {
		if matcher(requestModel) {
			names = append(names, requestModel)
		}
	}
	sort.Strings(names)
	out := make([]axonHubConnection, 0, len(names))
	for _, name := range names {
		out = append(out, axonHubConnection{channelSourceID, entries[name], name})
	}
	return out
}

// compileAxonHubPattern uses Go's regexp engine. AxonHub's xregexp wrapper is
// RE2-compatible, so a pattern that fails to compile here is refused rather
// than approximated.
func compileAxonHubPattern(pattern string) (func(string) bool, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	expr, err := regexp.Compile(trimmed)
	if err != nil {
		return nil, err
	}
	return expr.MatchString, nil
}

func channelHasAnyTag(channel *axonHubPlanChannel, tags []string) bool {
	for _, tag := range tags {
		for _, candidate := range channel.Tags {
			if candidate == tag {
				return true
			}
		}
	}
	return false
}

func axonHubExcluded(channel *axonHubPlanChannel, excludes []AxonHubSourceExclude) bool {
	for _, exclude := range excludes {
		if exclude.ChannelNamePattern != "" {
			if matcher, err := compileAxonHubPattern(exclude.ChannelNamePattern); err == nil && matcher(channel.Name) {
				return true
			}
		}
		for _, id := range exclude.ChannelIDs {
			if id == channel.SourceID {
				return true
			}
		}
		for _, tag := range exclude.ChannelTags {
			for _, candidate := range channel.Tags {
				if candidate == tag {
					return true
				}
			}
		}
	}
	return false
}
