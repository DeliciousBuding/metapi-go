package backup

import (
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/store"
)

func compileAxonHubKeyRouting(profile *AxonHubAccessProfile, policy *store.DownstreamAccessPolicy, residuals *[]string) {
	if profile.LoadBalanceStrategy != nil && !store.DefaultRoutingOverride(*profile.LoadBalanceStrategy) {
		switch *profile.LoadBalanceStrategy {
		case "round-robin":
			policy.RoutingStrategy = "round_robin"
			*residuals = append(*residuals, "source_round_robin_replaced_by_gateway_rotation")
		case "adaptive", "failover", "circuit-breaker":
			policy.BlockReason = "source_key_strategy_not_equivalent:" + *profile.LoadBalanceStrategy
		default:
			policy.BlockReason = "source_key_strategy_unknown"
		}
	}
	if profile.TraceStickyMode != nil && !store.DefaultRoutingOverride(*profile.TraceStickyMode) {
		if !store.ValidTraceStickyMode(*profile.TraceStickyMode) {
			policy.BlockReason = "source_key_trace_sticky_unknown"
		} else {
			policy.TraceStickyMode = *profile.TraceStickyMode
		}
	}
	if policy.TraceStickyMode == "prefer_previous_channel" {
		*residuals = append(*residuals, "source_trace_affinity_replaced_by_success_only_key_scope")
		*residuals = append(*residuals, "source_custom_trace_extraction_not_imported")
	}
}

// Source deployment policy is flattened into each imported route's defaults,
// never into global gateway settings. Explicit model and Key overrides remain
// higher precedence.
func axonHubSystemRouting(src *AxonHubSource) (strategy, sticky string, residuals []string) {
	strategy, sticky = "adaptive", "prefer_previous_channel"
	for _, row := range src.SystemConfigs {
		if row.Key != "retry_policy" {
			continue
		}
		var policy struct {
			Strategy string `json:"load_balancer_strategy"`
			Sticky   string `json:"trace_sticky_mode"`
		}
		if json.Unmarshal([]byte(row.Value), &policy) != nil {
			residuals = append(residuals, "source_system_routing_policy_invalid")
			continue
		}
		if !store.DefaultRoutingOverride(policy.Strategy) {
			strategy = policy.Strategy
		}
		if !store.DefaultRoutingOverride(policy.Sticky) {
			sticky = policy.Sticky
		}
	}
	return
}

func axonHubModelRouting(src *AxonHubSource, model AxonHubSourceModel, residuals *[]string, pattern string) (strategy, sticky string) {
	systemStrategy, systemSticky, notes := axonHubSystemRouting(src)
	*residuals = append(*residuals, notes...)
	strategy = model.Settings.LoadBalancerStrategy
	if store.DefaultRoutingOverride(strategy) {
		strategy = systemStrategy
	}
	strategy = axonHubRouteStrategy(strategy, residuals, pattern)
	sticky = model.Settings.TraceStickyMode
	if store.DefaultRoutingOverride(sticky) {
		sticky = systemSticky
	}
	if !store.ValidTraceStickyMode(sticky) {
		*residuals = append(*residuals, "model_trace_sticky_mode_unsupported:"+sanitizeIdentifier(pattern))
		sticky = "disabled"
	}
	if sticky == "prefer_previous_channel" {
		*residuals = append(*residuals, "source_trace_affinity_replaced_by_success_only_key_scope")
		*residuals = append(*residuals, "source_custom_trace_extraction_not_imported")
	}
	return
}
