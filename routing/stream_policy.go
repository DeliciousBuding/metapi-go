package routing

import "github.com/deliciousbuding/metapi-go/store"

func streamPolicyBlocker(candidate RouteChannelCandidate, policy DownstreamRoutingPolicy) CandidateBlocker {
	if candidate.Direct == nil {
		return CandidateBlocker{}
	}
	streamPolicy := candidate.Direct.StreamPolicy
	if !store.ValidDirectStreamPolicy(streamPolicy) {
		return CandidateBlocker{"stream_policy_invalid", "direct upstream stream policy is invalid"}
	}
	if streamPolicy == "" || streamPolicy == store.DirectStreamUnlimited {
		return CandidateBlocker{}
	}
	if policy.RequestStream == nil {
		return CandidateBlocker{"stream_context_missing", "request streaming mode is required to evaluate channel policy"}
	}
	if *policy.RequestStream && streamPolicy == store.DirectStreamForbid {
		return CandidateBlocker{"stream_forbidden", "direct upstream forbids streaming requests"}
	}
	if !*policy.RequestStream && streamPolicy == store.DirectStreamRequire && !policy.AllowStreamAggregation {
		return CandidateBlocker{"stream_aggregation_unsupported", "request protocol cannot aggregate a required upstream stream"}
	}
	return CandidateBlocker{}
}

// Preference is applied only after hard eligibility, across priority layers.
// A disallowed native candidate must not hide an authorized streaming fallback.
func streamPolicyPrefersNative(candidates []RouteChannelCandidate, policy DownstreamRoutingPolicy) bool {
	if policy.RequestStream == nil || *policy.RequestStream {
		return false
	}
	for _, candidate := range candidates {
		if candidate.Direct == nil || candidate.Direct.StreamPolicy != store.DirectStreamRequire {
			return true
		}
	}
	return false
}

func filterStreamPolicyCandidates(candidates []RouteChannelCandidate, policy DownstreamRoutingPolicy) []RouteChannelCandidate {
	if !streamPolicyPrefersNative(candidates, policy) {
		return candidates
	}
	filtered := make([]RouteChannelCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Direct == nil || candidate.Direct.StreamPolicy != store.DirectStreamRequire {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}
