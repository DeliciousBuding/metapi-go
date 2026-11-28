package admin

import (
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

// downstreamPricingScope is the visibility filter derived from the managed
// key's routing policy. It deliberately mirrors routing's allow/exclude
// semantics (see routing.ChannelSelector.resolveDownstreamExclusionBlocker)
// instead of inventing a second policy language for pricing.
type downstreamPricingScope struct {
	policy routing.DownstreamRoutingPolicy

	allowedSites  map[int64]struct{}
	excludedSites map[int64]struct{}

	// accountID -> siteID. Credential refs that cannot be mapped to an account
	// (direct grants) contribute nothing: their pricing rows cannot be
	// attributed reliably, so they are never returned.
	allowedAccountSites  map[int64]int64
	excludedAccountSites map[int64]int64
	credentialRestricted bool

	// accountPricingVisible mirrors the routing check for account-backed
	// candidates (non-direct candidates carry upstream channel ID 0). A policy
	// restricted to specific imported direct channels has no account-backed
	// pricing rows to expose.
	accountPricingVisible bool
}

func newDownstreamPricingScope(policy auth.DownstreamRoutingPolicy) *downstreamPricingScope {
	routePolicy := proxy.RoutingPolicyFromAuth(policy)
	scope := &downstreamPricingScope{
		policy:                routePolicy,
		allowedSites:          map[int64]struct{}{},
		excludedSites:         map[int64]struct{}{},
		allowedAccountSites:   map[int64]int64{},
		excludedAccountSites:  map[int64]int64{},
		credentialRestricted:  len(routePolicy.AllowedCredentialRefs) > 0,
		accountPricingVisible: routePolicy.AccessPolicy.AllowsUpstreamChannel(0),
	}
	for _, id := range routePolicy.AllowedSiteIDs {
		if id > 0 {
			scope.allowedSites[id] = struct{}{}
		}
	}
	for _, id := range routePolicy.ExcludedSiteIDs {
		if id > 0 {
			scope.excludedSites[id] = struct{}{}
		}
	}
	for _, ref := range routePolicy.AllowedCredentialRefs {
		if siteID, accountID, ok := credentialRefAccount(ref); ok {
			scope.allowedAccountSites[accountID] = siteID
		}
	}
	for _, ref := range routePolicy.ExcludedCredentialRefs {
		if siteID, accountID, ok := credentialRefAccount(ref); ok {
			scope.excludedAccountSites[accountID] = siteID
		}
	}
	return scope
}

// credentialRefAccount maps a credential reference onto the account whose
// aggregate pricing row it selects. Only account-level refs are mappable;
// direct grants are a different upstream dimension and map to nothing.
func credentialRefAccount(ref routing.CredentialRef) (siteID, accountID int64, ok bool) {
	switch ref.Kind {
	case string(auth.CredentialRefAccountToken), string(auth.CredentialRefDefaultApiKey), "":
		if ref.SiteID <= 0 || ref.AccountID <= 0 {
			return 0, 0, false
		}
		return ref.SiteID, ref.AccountID, true
	default:
		return 0, 0, false
	}
}

func (s *downstreamPricingScope) siteVisible(siteID int64) bool {
	if siteID <= 0 {
		return false
	}
	if len(s.allowedSites) > 0 {
		if _, ok := s.allowedSites[siteID]; !ok {
			return false
		}
	}
	_, excluded := s.excludedSites[siteID]
	return !excluded
}

func (s *downstreamPricingScope) accountVisible(siteID, accountID int64) bool {
	if !s.siteVisible(siteID) {
		return false
	}
	if refSite, ok := s.excludedAccountSites[accountID]; ok && refSite == siteID {
		return false
	}
	if s.credentialRestricted {
		refSite, ok := s.allowedAccountSites[accountID]
		if !ok || refSite != siteID {
			return false
		}
	}
	return true
}

// modelVisible reuses the routing policy's model visibility (AccessPolicy
// model list + supported-model patterns/routes). The pricing surface is
// upstream-model oriented, so a rename target that is not itself allowed by
// the policy is withheld.
func (s *downstreamPricingScope) modelVisible(model string) bool {
	return routing.IsModelAllowedByDownstreamPolicy(model, s.policy)
}

// accountFilters returns " AND ..." clauses for an accounts/sites join. Column
// aliases are supplied by the caller so the same policy is pushed into every
// aggregation query instead of being applied only after loading all rows.
func (s *downstreamPricingScope) accountFilters(siteCol, accountCol string) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)

	if len(s.allowedSites) > 0 {
		ids := sortedInt64Keys(s.allowedSites)
		clauses = append(clauses, siteCol+" IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(s.excludedSites) > 0 {
		ids := sortedInt64Keys(s.excludedSites)
		clauses = append(clauses, siteCol+" NOT IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if s.credentialRestricted {
		if len(s.allowedAccountSites) == 0 {
			// An allow-list made only of direct grants authorizes no account
			// pricing row.
			clauses = append(clauses, "1 = 0")
		} else {
			ids := sortedInt64MapKeys(s.allowedAccountSites)
			clauses = append(clauses, accountCol+" IN ("+sqlPlaceholders(len(ids))+")")
			for _, id := range ids {
				args = append(args, id)
			}
		}
	}
	if len(s.excludedAccountSites) > 0 {
		ids := sortedInt64MapKeys(s.excludedAccountSites)
		clauses = append(clauses, accountCol+" NOT IN ("+sqlPlaceholders(len(ids))+")")
		for _, id := range ids {
			args = append(args, id)
		}
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func sortedInt64Keys(set map[int64]struct{}) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedInt64MapKeys(set map[int64]int64) []int64 {
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sqlPlaceholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
