package routing

import (
	"context"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
)

// RevalidateQueuedChannel uses fresh storage and the same eligibility owner
// after a potentially long queue wait. It neither re-ranks another channel
// under the old permit nor spends a second selection/admission.
func (tr *TokenRouter) RevalidateQueuedChannel(ctx context.Context, selected *SelectedChannel, requestedModel string, policy DownstreamRoutingPolicy) (*SelectedChannel, error) {
	if selected == nil || selected.Direct == nil || !IsModelAllowedByDownstreamPolicy(requestedModel, policy) {
		return nil, nil
	}
	selector := *tr.selector
	selector.cache = NewRouteCache(0)
	requestedModel = policy.AccessPolicy.MapModel(requestedModel)
	match, err := selector.findRoute(ctx, requestedModel, policy)
	if err != nil || match == nil || match.Route.ID != selected.Channel.RouteID {
		return nil, err
	}
	now := time.Now()
	mappedModel := ResolveMappedModel(requestedModel, match.Route.ModelMapping)
	byDisplay := IsRouteDisplayNameMatch(requestedModel, match.Route.DisplayName)
	var eligible []RouteChannelCandidate
	for _, candidate := range match.Channels {
		var associationBlocker CandidateBlocker
		candidate, associationBlocker = resolveAssociationCandidate(candidate, policy)
		if associationBlocker.Code != "" {
			continue
		}
		if len(selector.getCandidateBlockers(candidate, requestedModel, byDisplay, nil, now.UTC().Format(time.RFC3339), policy)) != 0 {
			continue
		}
		eligible = append(eligible, candidate)
	}
	// Queue waiting can outlive configuration changes on a different channel.
	// Re-evaluate native preference over the fresh authorized pool, not just
	// the previously selected row.
	for _, candidate := range filterStreamPolicyCandidates(eligible, policy) {
		if candidate.Channel.ID != selected.Channel.ID || candidate.Direct == nil ||
			candidate.Direct.ChannelID != selected.Direct.ChannelID || candidate.Direct.CredentialID != selected.Direct.CredentialID {
			continue
		}
		// A changed concurrency policy requires a new queue decision; don't
		// dispatch under a permit acquired for a different policy snapshot.
		if candidate.Direct.RateLimit != selected.Direct.RateLimit {
			return nil, nil
		}
		return selector.finalizeDispatch(ctx, &candidate, match, requestedModel, mappedModel, policy,
			false, "", "", false, nil, now.UTC().Format(time.RFC3339), now.UnixMilli())
	}
	return nil, nil
}

// SetChannelLimits injects the same admission owner as the dispatcher before
// serving requests. Nil keeps the process-local default.
func (tr *TokenRouter) SetChannelLimits(limits *channellimit.Manager) {
	tr.selector.channelLimits = limits
}

func (s *ChannelSelector) limitSnapshot(candidate RouteChannelCandidate) *channellimit.Snapshot {
	if candidate.Direct == nil || !candidate.Direct.RateLimit.Enabled() {
		return nil
	}
	manager := s.channelLimits
	if manager == nil {
		manager = channellimit.Default
	}
	snapshot := manager.Snapshot(candidate.Direct.ChannelID, candidate.Direct.RateLimit.Config)
	return &snapshot
}

// Rate-limit exhaustion is a ranking signal, not a new permission. If every
// candidate is exhausted they remain fallback candidates: strict RPM and FIFO
// admission run atomically at dispatch; TPM never becomes an invented hard cap.
func (s *ChannelSelector) preferChannelCapacity(candidates []RouteChannelCandidate) []RouteChannelCandidate {
	result := make([]RouteChannelCandidate, 0, len(candidates))
	fallback := make([]RouteChannelCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.RateLimitLoad = s.limitSnapshot(candidate)
		fallback = append(fallback, candidate)
		if candidate.RateLimitLoad == nil || candidate.RateLimitLoad.ExhaustedBy == "" {
			result = append(result, candidate)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func channelCapacityMultiplier(candidate RouteChannelCandidate) float64 {
	if candidate.RateLimitLoad == nil {
		return 1
	}
	// Keep an exhausted-only pool selectable, with admission owning refusal.
	return max(candidate.RateLimitLoad.Score, 0.0001)
}
