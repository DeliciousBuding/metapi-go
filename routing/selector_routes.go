package routing

import (
	"context"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// loadEnabledRoutes returns enabled routes, serving them from the route cache
// while it is fresh (IsRoutesFresh/GetRoutes) and falling back to the DB only
// on a miss, then repopulating the cache (SetRoutes). Every route-mutation
// path (route/channel CRUD, rebuild, account/site changes, failure-state
// recovery) funnels through routing.InvalidateCache -> InvalidateAll, which
// clears the cached list; worst-case staleness is therefore bounded by TTL.
// The returned slice is shared across goroutines and must be treated as
// read-only (findRoute only copies elements out of it).
func (s *ChannelSelector) loadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	if s.cache != nil && s.cache.IsRoutesFresh() {
		if routes := s.cache.GetRoutes(); routes != nil {
			return routes, nil
		}
	}
	routes, err := s.db.LoadEnabledRoutes(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		if routes == nil {
			// Cache empty results too (as a non-nil slice): GetRoutes uses
			// nil to signal miss/stale, and a deployment with zero enabled
			// routes must not re-query on every request.
			routes = []store.TokenRoute{}
		}
		s.cache.SetRoutes(routes)
	}
	return routes, nil
}

func (s *ChannelSelector) findRoute(ctx context.Context, model string, policy DownstreamRoutingPolicy) (*RouteMatch, error) {
	routes, err := s.loadEnabledRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("findRoute: load routes: %w", err)
	}

	// Apply allowedRouteIds filter
	matchedSupportedPattern := matchesSupportedModelPolicy(model, policy.SupportedModels)
	if len(policy.AllowedRouteIDs) > 0 && (!matchedSupportedPattern || policy.UserRouteIDs != nil) {
		allowSet := make(map[int64]bool)
		for _, id := range policy.AllowedRouteIDs {
			allowSet[id] = true
		}
		filtered := make([]store.TokenRoute, 0, len(routes))
		for _, r := range routes {
			if allowSet[r.ID] {
				filtered = append(filtered, r)
			}
		}
		routes = filtered
	}
	// Filter a fresh slice: loadEnabledRoutes may return the shared cache.
	if policy.UserRouteIDs != nil {
		filtered := make([]store.TokenRoute, 0, len(routes))
		for _, route := range routes {
			if userRouteAllowed(route.ID, policy) {
				filtered = append(filtered, route)
			}
		}
		routes = filtered
	}

	// Match priority: 1) explicit_group displayName exact, 2) exact model pattern,
	// 3) displayName exact, 4) wildcard. Within the first non-empty bucket,
	// multi-tier context may re-rank when RequestedContextTokens > 0.
	// Buckets 1-3 share a single pass over routes (cheap predicates); the
	// wildcard/regex bucket is scanned only when all three cheap buckets came
	// back empty, preserving the original priority order and early exit while
	// cutting up to four full scans down to at most two.
	var explicitGroupBucket, exactPatternBucket, displayNameBucket []store.TokenRoute
	for i := range routes {
		r := &routes[i]
		if IsExplicitGroupRoute(r.RouteMode) {
			if IsRouteDisplayNameMatch(model, r.DisplayName) {
				explicitGroupBucket = append(explicitGroupBucket, *r)
			}
			continue
		}
		if IsExactRouteModelPattern(r.ModelPattern) && strings.TrimSpace(r.ModelPattern) == model {
			exactPatternBucket = append(exactPatternBucket, *r)
		}
		if IsRouteDisplayNameMatch(model, r.DisplayName) {
			displayNameBucket = append(displayNameBucket, *r)
		}
	}

	var bucket []store.TokenRoute
	switch {
	case len(explicitGroupBucket) > 0:
		bucket = explicitGroupBucket
	case len(exactPatternBucket) > 0:
		bucket = exactPatternBucket
	case len(displayNameBucket) > 0:
		bucket = displayNameBucket
	default:
		for i := range routes {
			r := &routes[i]
			if !IsExplicitGroupRoute(r.RouteMode) && MatchesModelPattern(model, r.ModelPattern) {
				bucket = append(bucket, *r)
			}
		}
	}

	if len(bucket) == 0 {
		return nil, nil
	}

	matched := PickContextTierRoute(bucket, policy.RequestedContextTokens)
	if matched == nil {
		return nil, nil
	}
	return s.loadRouteMatch(ctx, *matched)
}

func (s *ChannelSelector) loadRouteMatch(ctx context.Context, route store.TokenRoute) (*RouteMatch, error) {
	// Check cache
	if cached := s.cache.GetMatch(route.ID); cached != nil {
		return cached, nil
	}

	// Load channels for this route
	routeIDs := []int64{route.ID}
	if IsExplicitGroupRoute(route.RouteMode) {
		// Load source route IDs
		sourceIDs, err := s.db.LoadRouteGroupSources(ctx, []int64{route.ID})
		if err != nil {
			return nil, err
		}
		if ids, ok := sourceIDs[route.ID]; ok {
			routeIDs = ids
		}
	}

	// Load channels
	joined, err := s.db.LoadRouteChannels(ctx, routeIDs)
	if err != nil {
		return nil, err
	}

	// Collect OAuth route unit IDs
	unitIDsMap := make(map[int64]bool)
	for _, j := range joined {
		if j.Channel.OAuthRouteUnitID != nil && *j.Channel.OAuthRouteUnitID > 0 {
			unitIDsMap[*j.Channel.OAuthRouteUnitID] = true
		}
	}
	var unitIDs []int64
	for id := range unitIDsMap {
		unitIDs = append(unitIDs, id)
	}

	var unitSummaries map[int64]OAuthRouteUnitSummary
	var unitMembers map[int64][]OAuthRouteUnitMemberCandidate
	if len(unitIDs) > 0 {
		unitSummaries, err = s.db.LoadOAuthRouteUnitSummaries(ctx, unitIDs)
		if err != nil {
			return nil, err
		}
		unitMembers, err = s.db.LoadOAuthRouteUnitMembers(ctx, unitIDs)
		if err != nil {
			return nil, err
		}
	}

	// Resolve source-model fallback from each channel's route model_pattern.
	// Group routes expand to source route IDs; blank channel SourceModel
	// must inherit that source route's pattern for eligibility/resolveModel.
	fallbackSourceModelByRouteID, err := s.loadFallbackSourceModelByRouteID(ctx, route, routeIDs)
	if err != nil {
		return nil, err
	}

	candidates := make([]RouteChannelCandidate, 0, len(joined))
	for _, j := range joined {
		channel := j.Channel
		resolvedSourceModel := NormalizeChannelSourceModel(channel.SourceModel)
		if resolvedSourceModel == "" {
			if fb, ok := fallbackSourceModelByRouteID[channel.RouteID]; ok && fb != "" {
				resolvedSourceModel = fb
			}
		}
		if resolvedSourceModel != "" {
			sm := resolvedSourceModel
			channel.SourceModel = &sm
		} else {
			channel.SourceModel = nil
		}

		candidate := RouteChannelCandidate{
			Channel: channel,
			Account: j.Account,
			Site:    j.Site,
			Token:   j.Token,
			Direct:  channel.Direct,
		}

		if j.Channel.OAuthRouteUnitID != nil && *j.Channel.OAuthRouteUnitID > 0 {
			unitID := *j.Channel.OAuthRouteUnitID
			if summary, ok := unitSummaries[unitID]; ok {
				candidate.RouteUnit = &summary
			}
			if members, ok := unitMembers[unitID]; ok {
				candidate.RouteUnitMembers = members
			}
		}

		candidates = append(candidates, candidate)
	}

	match := &RouteMatch{
		Route:    route,
		Channels: candidates,
	}
	s.cache.SetMatch(route.ID, match)
	return match, nil
}

// loadFallbackSourceModelByRouteID maps route IDs to model_pattern values used when
// a channel's SourceModel is nil/empty. For group routes this is each source route's
// pattern; for normal routes it is the matched route itself.
func (s *ChannelSelector) loadFallbackSourceModelByRouteID(ctx context.Context, route store.TokenRoute, routeIDs []int64) (map[int64]string, error) {
	fallback := make(map[int64]string, len(routeIDs))
	if len(routeIDs) == 0 {
		return fallback, nil
	}

	// Fast path: non-group routes only expand to themselves.
	if len(routeIDs) == 1 && routeIDs[0] == route.ID {
		pattern := strings.TrimSpace(route.ModelPattern)
		if pattern != "" {
			fallback[route.ID] = pattern
		}
		return fallback, nil
	}

	// Load enabled routes and map model patterns for every source route ID.
	routes, err := s.loadEnabledRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("loadRouteMatch: load source route patterns: %w", err)
	}
	byID := make(map[int64]store.TokenRoute, len(routes)+1)
	for _, r := range routes {
		byID[r.ID] = r
	}
	// Always include the matched route in case it is a group or not in the enabled list shape used above.
	byID[route.ID] = route

	for _, rID := range routeIDs {
		if r, ok := byID[rID]; ok {
			pattern := strings.TrimSpace(r.ModelPattern)
			if pattern != "" {
				fallback[rID] = pattern
			}
		}
	}
	return fallback, nil
}
