package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// TokenRouter is the main routing engine. It composes all sub-modules.
type TokenRouter struct {
	db                  ChannelSelectorDB
	cache               *RouteCache
	selector            *ChannelSelector
	cfg                 *config.Config
	routingWeights      RoutingWeightsConfig
	configuredMaxSec    int
	fallbackUnitCost    float64
	pricingFn           CatalogPricingResolver
	channelLoadProvider ChannelLoadSnapshotProvider
}

// NewTokenRouter creates a new TokenRouter.
func NewTokenRouter(
	db ChannelSelectorDB,
	cfg *config.Config,
	pricingFn CatalogPricingResolver,
	channelLoadProvider ChannelLoadSnapshotProvider,
) *TokenRouter {
	cacheTTLMs := int64(cfg.TokenRouterCacheTtlMs)
	cache := NewRouteCache(cacheTTLMs)
	SetGlobalCache(cache)

	// Tunables below are snapshotted at construction time (same semantics as
	// pre-C1: they are baked into the router and a settings change applies on
	// the next router rebuild / restart).
	rt := config.Runtime()
	configuredMaxSec := rt.TokenRouterFailureCooldownMaxSec
	if configuredMaxSec <= 0 {
		configuredMaxSec = TokenRouterFailureCooldownMaxSecCeiling
	}

	fallbackUnitCost := rt.RoutingFallbackUnitCost
	if fallbackUnitCost <= 0 {
		fallbackUnitCost = 1
	}

	routingWeights := RoutingWeightsConfig{
		BaseWeightFactor: rt.RoutingWeights.BaseWeightFactor,
		ValueScoreFactor: rt.RoutingWeights.ValueScoreFactor,
		CostWeight:       rt.RoutingWeights.CostWeight,
		BalanceWeight:    rt.RoutingWeights.BalanceWeight,
		UsageWeight:      rt.RoutingWeights.UsageWeight,
	}

	selector := NewChannelSelector(db, cache, configuredMaxSec, routingWeights, pricingFn, fallbackUnitCost, channelLoadProvider)

	return &TokenRouter{
		db:                  db,
		cache:               cache,
		selector:            selector,
		cfg:                 cfg,
		routingWeights:      routingWeights,
		configuredMaxSec:    configuredMaxSec,
		fallbackUnitCost:    fallbackUnitCost,
		pricingFn:           pricingFn,
		channelLoadProvider: channelLoadProvider,
	}
}

// SelectChannel finds a matching route and selects a channel.
func (tr *TokenRouter) SelectChannel(ctx context.Context, requestedModel string, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	return tr.selector.SelectChannel(ctx, requestedModel, policy)
}

// SelectNextChannel selects the next channel excluding already-tried ones.
func (tr *TokenRouter) SelectNextChannel(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	return tr.selector.SelectNextChannel(ctx, requestedModel, excludeChannelIDs, policy)
}

// SelectPreferredChannel selects a specific preferred channel.
func (tr *TokenRouter) SelectPreferredChannel(ctx context.Context, requestedModel string, preferredChannelID int64, policy DownstreamRoutingPolicy, excludeChannelIDs []int64) (*SelectedChannel, error) {
	return tr.selector.SelectPreferredChannel(ctx, requestedModel, preferredChannelID, policy, excludeChannelIDs)
}

// GetAvailableModels returns all exposed model names from enabled routes.
func (tr *TokenRouter) GetAvailableModels(ctx context.Context) ([]string, error) {
	routes, err := tr.db.FindAllEnabledRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getAvailableModels: %w", err)
	}

	exposed := buildVisibleEnabledRoutes(routes)
	names := make(map[string]bool)
	for _, route := range exposed {
		name := GetExposedModelNameForRoute(route.DisplayName, route.ModelPattern)
		if name != "" {
			names[name] = true
		}
	}
	var result []string
	for name := range names {
		result = append(result, name)
	}
	return result, nil
}

// GetAvailableModelContextLengths returns positive token_routes.context_length values
// keyed by the same exposed model id used in GetAvailableModels.

// Rules:
// - only non-null positive context_length values are included
// - when multiple visible routes expose the same id, the max value wins
// - NULL / non-positive lengths are omitted (caller keeps heuristics)

// Metadata only — no proxy max-token enforcement is implied by this map.
func (tr *TokenRouter) GetAvailableModelContextLengths(ctx context.Context) (map[string]int64, error) {
	routes, err := tr.db.FindAllEnabledRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("getAvailableModelContextLengths: %w", err)
	}
	return buildAvailableModelContextLengths(routes), nil
}

// buildAvailableModelContextLengths maps exposed model ids to the max positive
// context_length among visible enabled routes. Pure helper for unit tests.
func buildAvailableModelContextLengths(routes []store.TokenRoute) map[string]int64 {
	exposed := buildVisibleEnabledRoutes(routes)
	out := make(map[string]int64)
	for _, route := range exposed {
		if !route.Enabled {
			continue
		}
		name := GetExposedModelNameForRoute(route.DisplayName, route.ModelPattern)
		if name == "" || route.ContextLength == nil || *route.ContextLength <= 0 {
			continue
		}
		if prev, ok := out[name]; !ok || *route.ContextLength > prev {
			out[name] = *route.ContextLength
		}
	}
	return out
}

// buildVisibleEnabledRoutes filters out routes covered by wildcard display names.
func buildVisibleEnabledRoutes(routes []store.TokenRoute) []store.TokenRoute {
	exactModelNames := make(map[string]bool)
	for _, r := range routes {
		if !IsExplicitGroupRoute(r.RouteMode) && IsExactRouteModelPattern(r.ModelPattern) {
			name := strings.TrimSpace(r.ModelPattern)
			if name != "" {
				exactModelNames[name] = true
			}
		}
	}

	var coveringRoutes []store.TokenRoute
	for _, r := range routes {
		if !r.Enabled {
			continue
		}
		if IsExplicitGroupRoute(r.RouteMode) {
			continue
		}
		if !IsExactRouteModelPattern(r.ModelPattern) && HasCustomDisplayName(r.ModelPattern, r.DisplayName) {
			coveringRoutes = append(coveringRoutes, r)
		}
	}

	if len(coveringRoutes) == 0 {
		return routes
	}

	var result []store.TokenRoute
	for _, r := range routes {
		if IsExplicitGroupRoute(r.RouteMode) {
			dn := NormalizeRouteDisplayName(r.DisplayName)
			if dn != "" {
				result = append(result, r)
			}
			continue
		}

		if !IsExactRouteModelPattern(r.ModelPattern) {
			result = append(result, r)
			continue
		}
		if HasCustomDisplayName(r.ModelPattern, r.DisplayName) {
			result = append(result, r)
			continue
		}

		exactModel := strings.TrimSpace(r.ModelPattern)
		if exactModel == "" {
			result = append(result, r)
			continue
		}

		// Check if covered by any covering wildcard route
		covered := false
		for _, cr := range coveringRoutes {
			if cr.ID == r.ID {
				continue
			}
			groupDisplayName := NormalizeRouteDisplayName(cr.DisplayName)
			if groupDisplayName == "" || exactModelNames[groupDisplayName] {
				continue
			}
			if MatchesModelPattern(exactModel, cr.ModelPattern) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, r)
		}
	}
	return result
}


