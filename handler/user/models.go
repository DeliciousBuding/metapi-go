package user

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/pricing"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
)

type userModel struct {
	ID             string              `json:"id"`
	RouteID        int64               `json:"routeId"`
	Protocols      []string            `json:"protocols"`
	ReferencePrice *userReferencePrice `json:"referencePrice"`
}

// This projection deliberately excludes catalog source names, URLs, provider
// identities and resolved model IDs. Catalog prices are references, not the
// gateway's token-credit charges or an upstream account's private costs.
type userReferencePrice struct {
	Currency   string   `json:"currency"`
	Unit       string   `json:"unit"`
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cacheRead"`
	CacheWrite *float64 `json:"cacheWrite"`
	UpdatedAt  *string  `json:"updatedAt"`
}

type userModelQuota struct {
	Unit            string  `json:"unit"`
	TokensPerUnit   int64   `json:"tokensPerUnit"`
	GroupMultiplier float64 `json:"groupMultiplier"`
}

type userModelCatalog struct {
	Items []userModel    `json:"items"`
	Quota userModelQuota `json:"quota"`
}

func (h *handler) listModels(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	groupID, routes, err := h.users.ResolveUserKeyAccess(r.Context(), u.ID)
	if err != nil {
		resultError(w, err)
		return
	}
	groupPricing, err := h.users.GetUserGroupPricing(r.Context(), groupID)
	if err != nil {
		resultError(w, err)
		return
	}
	result := userModelCatalog{
		Items: []userModel{},
		Quota: userModelQuota{Unit: "token_credits", TokensPerUnit: 1_000_000, GroupMultiplier: groupPricing.Multiplier},
	}
	if len(routes) == 0 {
		writeJSON(w, http.StatusOK, result)
		return
	}
	if h.models == nil {
		fail(w, http.StatusServiceUnavailable, "Model catalog unavailable")
		return
	}
	catalog := h.models()
	if catalog == nil {
		fail(w, http.StatusServiceUnavailable, "Model catalog unavailable")
		return
	}
	names, err := catalog.GetAvailableModels(r.Context())
	if err != nil {
		resultError(w, err)
		return
	}
	// One immutable snapshot per response; listing never refreshes a source.
	var snapshot *pricingcatalog.CatalogSnapshot
	if manager := app.CatalogManager(); manager != nil {
		snapshot = manager.Snapshot()
	}
	sort.Strings(names)
	policy := routing.DownstreamRoutingPolicy{UserRouteIDs: &routes, SupportedModels: []string{"*"}, RequiredUpstreamProtocol: -1}
	for _, name := range names {
		// Wildcard/regex selectors are not actionable public model IDs.
		if name == "" || !routing.IsExactRouteModelPattern(name) {
			continue
		}
		decision, err := catalog.ExplainSelection(r.Context(), name, nil, policy)
		if err != nil {
			resultError(w, err)
			return
		}
		if !decision.Matched || decision.RouteID == nil {
			continue
		}
		eligible, direct := false, false
		for _, candidate := range decision.Candidates {
			if candidate.Eligible {
				eligible = true
				direct = direct || candidate.CandidateRef.Kind == "direct"
			}
		}
		if !eligible {
			continue
		}
		item := userModel{ID: name, RouteID: *decision.RouteID, Protocols: []string{}, ReferencePrice: publicReferencePrice(snapshot, name)}
		if direct {
			item.Protocols, err = modelExampleProtocols(r.Context(), catalog, name, policy)
			if err != nil {
				resultError(w, err)
				return
			}
		}
		result.Items = append(result.Items, item)
	}
	writeJSON(w, http.StatusOK, result)
}

// Protocols describe supported examples, not an exhaustive capability list or
// a health guarantee. Legacy account/OAuth candidates have no protocol filter
// in routing, so their eligibility alone is insufficient evidence for examples.
// Direct grants use the same protocol/conversion policy as proxy execution.
func modelExampleProtocols(ctx context.Context, catalog ModelCatalog, name string, policy routing.DownstreamRoutingPolicy) ([]string, error) {
	protocols := []string{}
	for _, protocol := range []string{"chat/completions", "responses", "messages", "embeddings"} {
		policy.RequiredUpstreamProtocol, policy.AllowUpstreamProtocolConversion = proxy.RoutingProtocolForPath("/v1/" + protocol)
		decision, err := catalog.ExplainSelection(ctx, name, nil, policy)
		if err != nil {
			return nil, err
		}
		if !decision.Matched {
			continue
		}
		for _, candidate := range decision.Candidates {
			if candidate.Eligible && candidate.CandidateRef.Kind == "direct" {
				protocols = append(protocols, protocol)
				break
			}
		}
	}
	return protocols, nil
}

func publicReferencePrice(snapshot *pricingcatalog.CatalogSnapshot, model string) *userReferencePrice {
	price := pricing.OfficialPriceFor(snapshot, model)
	if price == nil || !price.Known {
		return nil
	}
	result := &userReferencePrice{
		Currency: "USD", Unit: "per_million_tokens",
		Input: price.Rates.Input, Output: price.Rates.Output,
		CacheRead: price.Rates.CacheRead, CacheWrite: price.Rates.CacheWrite,
	}
	// LastUpdated originates in external metadata. Accept only dates, never
	// echo an arbitrary string that might contain private source information.
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if updated, err := time.Parse(layout, price.UpdatedAt); err == nil {
			value := updated.UTC().Format(layout)
			result.UpdatedAt = &value
			break
		}
	}
	return result
}
