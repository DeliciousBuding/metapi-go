package proxy

import (
	"maps"
	"slices"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
)

// RoutingPolicyFromAuth converts the auth-layer downstream policy into the
// routing-layer policy. Single owner of the field mapping; both the proxy
// admission path and admin explain/pricing surfaces consume it without
// introducing handler/admin -> handler/proxy coupling. Mutable maps and slices
// are cloned so downstream callers cannot mutate the auth context's policy.
func RoutingPolicyFromAuth(policy auth.DownstreamRoutingPolicy) routing.DownstreamRoutingPolicy {
	refs := mapAuthCredentialRefs(policy.ExcludedCredentialRefs)
	allowedRefs := mapAuthCredentialRefs(policy.AllowedCredentialRefs)

	multipliers := map[int64]float64{}
	if len(policy.SiteWeightMultipliers) > 0 {
		multipliers = maps.Clone(policy.SiteWeightMultipliers)
	}

	var userRouteIDs *[]int64
	if policy.UserRouteIDs != nil {
		cloned := slices.Clone(*policy.UserRouteIDs)
		userRouteIDs = &cloned
	}

	return routing.DownstreamRoutingPolicy{
		UserRouteIDs:           userRouteIDs,
		AccessPolicy:           policy.AccessPolicy,
		SupportedModels:        slices.Clone(policy.SupportedModels),
		AllowedRouteIDs:        slices.Clone(policy.AllowedRouteIDs),
		SiteWeightMultipliers:  multipliers,
		KeyWeight:              policy.KeyWeight,
		ExcludedSiteIDs:        slices.Clone(policy.ExcludedSiteIDs),
		ExcludedCredentialRefs: refs,
		AllowedSiteIDs:         slices.Clone(policy.AllowedSiteIDs),
		AllowedCredentialRefs:  allowedRefs,
		DenyAllWhenEmpty:       policy.DenyAllWhenEmpty,
	}
}

func mapAuthCredentialRefs(in []auth.ExcludedCredentialRef) []routing.CredentialRef {
	refs := make([]routing.CredentialRef, 0, len(in))
	for _, ref := range in {
		tokenID := int64(0)
		if ref.TokenID != nil {
			tokenID = *ref.TokenID
		}
		refs = append(refs, routing.CredentialRef{
			Kind:      string(ref.Kind),
			SiteID:    ref.SiteID,
			AccountID: ref.AccountID,
			TokenID:   tokenID,
			GrantID:   ref.GrantID,
		})
	}
	return refs
}
