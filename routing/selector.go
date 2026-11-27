package routing

import (
	"context"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/store"
)

// ChannelSelector implements selectChannel, selectNextChannel, selectPreferredChannel.
type ChannelSelector struct {
	db                  ChannelSelectorDB
	cache               *RouteCache
	configuredMaxSec    int
	routingWeights      RoutingWeightsConfig
	pricingFn           CatalogPricingResolver
	fallbackUnitCost    float64
	channelLoadProvider ChannelLoadSnapshotProvider
	channelLimits       *channellimit.Manager
	affinity            *traceAffinity
}

// NewChannelSelector creates a new ChannelSelector.
func NewChannelSelector(
	db ChannelSelectorDB,
	cache *RouteCache,
	configuredMaxSec int,
	routingWeights RoutingWeightsConfig,
	pricingFn CatalogPricingResolver,
	fallbackUnitCost float64,
	channelLoadProvider ChannelLoadSnapshotProvider,
) *ChannelSelector {
	return &ChannelSelector{
		db:                  db,
		cache:               cache,
		configuredMaxSec:    configuredMaxSec,
		routingWeights:      routingWeights,
		pricingFn:           pricingFn,
		fallbackUnitCost:    fallbackUnitCost,
		channelLoadProvider: channelLoadProvider,
		affinity:            newTraceAffinity(),
	}
}

// SelectChannel finds a matching route and selects the best channel.
func (s *ChannelSelector) SelectChannel(ctx context.Context, requestedModel string, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	return s.selectChannel(ctx, requestedModel, nil, nil, policy)
}

// SelectNextChannel selects a channel excluding already-tried channels.
func (s *ChannelSelector) SelectNextChannel(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	return s.selectChannel(ctx, requestedModel, nil, excludeChannelIDs, policy)
}

// SelectPreferredChannel selects a specific preferred channel if eligible.
func (s *ChannelSelector) SelectPreferredChannel(
	ctx context.Context,
	requestedModel string,
	preferredChannelID int64,
	policy DownstreamRoutingPolicy,
	excludeChannelIDs []int64,
) (*SelectedChannel, error) {
	return s.selectChannel(ctx, requestedModel, &preferredChannelID, excludeChannelIDs, policy)
}

func (s *ChannelSelector) selectChannel(ctx context.Context, requestedModel string, preferredID *int64, excludeChannelIDs []int64, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	if !IsModelAllowedByDownstreamPolicy(requestedModel, policy) || (preferredID != nil && *preferredID == 0) {
		return nil, nil
	}
	if err := EnsureSiteRuntimeHealthStateLoaded(); err != nil {
		return nil, err
	}

	requestedModel = policy.AccessPolicy.MapModel(requestedModel)
	match, err := s.findRoute(ctx, requestedModel, policy)
	if err != nil || match == nil {
		return nil, err
	}
	if preferredID != nil {
		return s.selectPreferredFromMatch(ctx, match, requestedModel, *preferredID, policy, excludeChannelIDs)
	}
	return s.selectFromMatch(ctx, match, requestedModel, policy, excludeChannelIDs, true)
}

func (s *ChannelSelector) selectPreferredFromMatch(ctx context.Context, match *RouteMatch, requestedModel string, preferredChannelID int64, policy DownstreamRoutingPolicy, excludeChannelIDs []int64) (*SelectedChannel, error) {
	mappedModel := ResolveMappedModel(requestedModel, match.Route.ModelMapping)
	requestedByDisplayName := IsRouteDisplayNameMatch(requestedModel, match.Route.DisplayName)
	strategy := effectiveStrategy(match.Route, policy)

	nowISO := time.Now().UTC().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()

	available := s.eligibleChannels(match, requestedModel, requestedByDisplayName, excludeChannelIDs, nowISO, policy)
	var preferred *RouteChannelCandidate
	for i := range available {
		if available[i].Channel.ID == preferredChannelID {
			preferred = &available[i]
			break
		}
	}
	if preferred == nil {
		return nil, nil
	}
	if snapshot := s.limitSnapshot(*preferred); snapshot != nil && snapshot.ExhaustedBy != "" {
		return nil, nil
	}

	// Check site/model breaker on the preferred candidate alone.
	// FilterSiteRuntimeBrokenCandidatesByModel intentionally keeps a single-candidate
	// set unchanged (multi-candidate starvation fallback). Preferred/sticky must not
	// reuse that short-circuit: an open breaker should decline preferred so the
	// caller can fall through to normal SelectChannel, which still applies the
	// multi-candidate empty-filter fallback when every sibling is also broken.
	details := GetSiteRuntimeHealthDetails(preferred.Site.ID, candidateRuntimeModel(*preferred, mappedModel, requestedByDisplayName))
	if details.GlobalBreakerOpen || details.ModelBreakerOpen ||
		details.GlobalHalfOpen || details.ModelHalfOpen {
		return nil, nil
	}

	// Round robin and OAuth units handle recent failures during rotation.
	if strategy != StrategyRoundRobin && !IsOAuthRouteUnitCandidate(*preferred) {
		if IsChannelRecentlyFailed(preferred.Channel.FailCount, preferred.Channel.LastFailAt, nowMs, s.configuredMaxSec) {
			return nil, nil
		}
	}

	recordSelection := strategy == StrategyRoundRobin || strategy == StrategyStableFirst
	rotationKey := BuildStableFirstRotationKey(match.Route.ID, requestedModel)
	return s.finalizeDispatch(ctx, preferred, match, requestedModel, mappedModel, policy,
		recordSelection, rotationKey, false, excludeChannelIDs, nowISO, nowMs)
}

func (s *ChannelSelector) selectFromMatch(
	ctx context.Context,
	match *RouteMatch,
	requestedModel string,
	policy DownstreamRoutingPolicy,
	excludeChannelIDs []int64,
	recordSelection bool,
) (*SelectedChannel, error) {
	mappedModel := ResolveMappedModel(requestedModel, match.Route.ModelMapping)
	requestedByDisplayName := IsRouteDisplayNameMatch(requestedModel, match.Route.DisplayName)
	strategy := effectiveStrategy(match.Route, policy)

	nowISO := time.Now().UTC().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()

	available := s.eligibleChannels(match, requestedModel, requestedByDisplayName, excludeChannelIDs, nowISO, policy)
	if len(available) == 0 {
		return nil, nil
	}
	available = s.preferChannelCapacity(available)
	if scope := affinityScope(match.Route, requestedModel, policy); scope != "" && s.affinity != nil {
		if previous := s.affinity.get(scope, time.Now()); previous.channelID != 0 {
			preferredID := previous.channelID
			if previous.directID != 0 {
				preferredID = 0
				var preferred *RouteChannelCandidate
				for i := range available {
					candidate := &available[i]
					if candidate.Direct != nil && candidate.Direct.ChannelID == previous.directID &&
						(preferred == nil || candidate.Channel.PriorityOrZero() < preferred.Channel.PriorityOrZero()) {
						preferred = candidate
					}
				}
				if preferred != nil {
					preferredID = preferred.Channel.ID
				}
			}
			if sticky, err := s.selectPreferredFromMatch(ctx, &RouteMatch{Route: match.Route, Channels: available}, requestedModel, preferredID, policy, excludeChannelIDs); err != nil || sticky != nil {
				if sticky != nil {
					sticky.AffinityPreferred = true
				}
				return sticky, err
			}
		}
	}

	resolveModel := func(c RouteChannelCandidate) string {
		return candidateRuntimeModel(c, mappedModel, requestedByDisplayName)
	}
	selected, rotationKey, usedObservation := s.pickCandidate(available, strategy, resolveModel, policy, match.Route.ID, requestedModel, nowMs, recordSelection)
	if selected == nil {
		return nil, nil
	}
	return s.finalizeDispatch(ctx, selected, match, requestedModel, mappedModel, policy,
		recordSelection, rotationKey, usedObservation, excludeChannelIDs, nowISO, nowMs)
}

func candidateRuntimeModel(candidate RouteChannelCandidate, mappedModel string, byDisplayName bool) string {
	if byDisplayName {
		if source := NormalizeChannelSourceModel(candidate.Channel.SourceModel); source != "" {
			return source
		}
	}
	return mappedModel
}

func (s *ChannelSelector) finalizeDispatch(
	ctx context.Context,
	selected *RouteChannelCandidate,
	match *RouteMatch,
	requestedModel string,
	mappedModel string,
	policy DownstreamRoutingPolicy,
	recordSelection bool,
	stableFirstRotationKey string,
	usedObservation bool,
	excludeChannelIDs []int64,
	nowISO string,
	nowMs int64,
) (*SelectedChannel, error) {
	dispatchCandidate := *selected
	var resolvedRouteUnitMemberTokenValue string

	if IsOAuthRouteUnitCandidate(*selected) {
		allowed := s.filterRouteUnitMembersByPolicy(*selected, policy)
		member := SelectRouteUnitMember(allowed, requestedModel, nowISO, nowMs, s.configuredMaxSec, excludeChannelIDs)
		if member == nil || selected.RouteUnit == nil {
			return nil, nil
		}
		resolvedRouteUnitMemberTokenValue = ResolveRouteUnitMemberTokenValue(member.Account)
		dispatchCandidate.Account = member.Account
		dispatchCandidate.Site = member.Site
		dispatchCandidate.Token = nil

		if recordSelection {
			if err := s.db.UpdateRouteUnitMemberLastSelectedAt(ctx, selected.RouteUnit.ID, member.Account.ID, nowISO); err != nil {
				return nil, err
			}
			// Invalidate caches for routes using this unit
			routeIDs, _ := s.db.FindRouteIDsByOAuthRouteUnitID(ctx, selected.RouteUnit.ID)
			for _, rid := range routeIDs {
				s.cache.InvalidateRouteScopedCache(rid)
			}
		}
	}

	// Half-open admission gate (three-state breaker, mirrors octopus circuit.go):
	// hard-open breakers and in-flight recovery probes are rejected; after a
	// breaker cooldown expires, exactly one request is admitted as the recovery
	// probe while the rest keep being rejected until the probe outcome is
	// recorded. Open now means no traffic — the pre-existing fail-open
	// starvation fallback no longer dispatches against hard-open sites.
	runtimeModelKey := candidateRuntimeModel(*selected, mappedModel, IsRouteDisplayNameMatch(requestedModel, match.Route.DisplayName))
	if dispatchCandidate.Direct == nil && !TryAdmitSiteModelRuntimeRequest(dispatchCandidate.Site.ID, runtimeModelKey) {
		return nil, nil
	}

	tokenValue := resolvedRouteUnitMemberTokenValue
	if dispatchCandidate.Direct != nil {
		tokenValue = dispatchCandidate.Direct.Credential
	}
	if tokenValue == "" && dispatchCandidate.Direct == nil {
		tokenValue = s.resolveChannelTokenValue(dispatchCandidate)
	}
	if tokenValue == "" && !dispatchCandidate.Direct.HasAnonymousCredential() &&
		!(dispatchCandidate.Direct != nil && (dispatchCandidate.Direct.CredentialKind == store.DirectCredentialGCPServiceAccount ||
			dispatchCandidate.Direct.CredentialKind == store.DirectCredentialOAuth && dispatchCandidate.Direct.Provider == "antigravity")) {
		// gcp_service_account holds no stored token until the first exchange;
		// the dispatch path resolves it through the credential owner.
		return nil, nil
	}

	if recordSelection {
		if stableFirstRotationKey != "" && dispatchCandidate.Direct == nil {
			targetKey := stableFirstRotationKey
			if usedObservation {
				targetKey += ":observe"
			}
			rememberStableFirstSiteSelectionForKey(targetKey, dispatchCandidate.Site.ID)
			UpdateStableFirstObservationProgress(stableFirstRotationKey, usedObservation, dispatchCandidate.Site.ID)
		}
		lastSelected := nowISO
		if dispatchCandidate.Direct != nil {
			lastSelected = time.Now().UTC().Format(time.RFC3339Nano)
		}
		if err := s.db.UpdateChannelLastSelectedAt(ctx, selected.Channel.ID, lastSelected); err != nil {
			return nil, err
		}
		s.cache.PatchCachedChannel(selected.Channel.ID, func(ch *store.RouteChannel) {
			ch.LastSelectedAt = &lastSelected
		})
	}

	actualModel := ResolveActualModelForSelectedChannel(requestedModel, match.Route.DisplayName, mappedModel, selected.Channel.SourceModel, selected.Channel.AccountID)
	if dispatchCandidate.Direct != nil {
		actualModel = dispatchCandidate.Direct.ModelName
	}

	tokenName := "default"
	if dispatchCandidate.Token != nil {
		tokenName = dispatchCandidate.Token.Name
	}

	return &SelectedChannel{
		Channel:       selected.Channel,
		Account:       dispatchCandidate.Account,
		Site:          dispatchCandidate.Site,
		Token:         dispatchCandidate.Token,
		TokenValue:    tokenValue,
		TokenName:     tokenName,
		ActualModel:   actualModel,
		ContextLength: match.Route.ContextLength,
		Direct:        dispatchCandidate.Direct,
		AffinityScope: affinityScope(match.Route, requestedModel, policy),
	}, nil
}
