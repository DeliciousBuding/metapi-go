package backup

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

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
