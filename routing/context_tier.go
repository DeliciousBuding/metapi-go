package routing

import (
	"github.com/deliciousbuding/metapi-go/internal/tokenestimate"
	"github.com/deliciousbuding/metapi-go/store"
)

// Multi-tier context routing (cita-777/metapi).
//
// Product: multiple token_routes may share the same model pattern with different
// context_length ceilings (e.g. 32k token-bill route vs 128k req-bill route).
// When the inbound request's estimated context is known, pick the tightest
// positive ceiling that still fits; fall back to unsized (NULL/≤0) catch-alls.
//
// When estimate is 0 (unknown), callers keep first-match behavior.

// EstimateRequestContextTokens returns a bounded estimate for context-tier selection.
func EstimateRequestContextTokens(body map[string]any) int64 {
	return tokenestimate.Request(body, 2_000_000)
}

// PickContextTierRoute selects among same-priority model matches by context
// ceiling. requestedTokens must be >0; otherwise the first route is returned
// unchanged (first-match honesty).
//
// Rules:
// 1. Among routes with positive context_length >= requestedTokens, pick the
// **smallest** ceiling (tightest fit).
// 2. Else if any positive context_length exists, pick the **largest** ceiling
// (best available still short of estimate).
// 3. Else prefer unsized (NULL/≤0) catch-all routes (first by id).
// 4. Empty input → nil.
func PickContextTierRoute(routes []store.TokenRoute, requestedTokens int64) *store.TokenRoute {
	if len(routes) == 0 {
		return nil
	}
	if requestedTokens <= 0 || len(routes) == 1 {
		r := routes[0]
		return &r
	}

	var (
		bestFit      *store.TokenRoute
		bestFitCeil  int64
		largest      *store.TokenRoute
		largestCeil  int64
		firstUnsized *store.TokenRoute
	)

	for i := range routes {
		r := &routes[i]
		ceil, ok := positiveContextLength(r.ContextLength)
		if !ok {
			if firstUnsized == nil {
				cp := *r
				firstUnsized = &cp
			}
			continue
		}
		if largest == nil || ceil > largestCeil {
			cp := *r
			largest = &cp
			largestCeil = ceil
		}
		if ceil >= requestedTokens {
			if bestFit == nil || ceil < bestFitCeil {
				cp := *r
				bestFit = &cp
				bestFitCeil = ceil
			}
		}
	}

	if bestFit != nil {
		return bestFit
	}
	// Prefer catch-all when nothing sized is large enough? Product intent:
	// oversized requests should use long-context / coding-plan tiers if present
	// (largest), else unsized.
	if largest != nil {
		return largest
	}
	if firstUnsized != nil {
		return firstUnsized
	}
	r := routes[0]
	return &r
}

func positiveContextLength(v *int64) (int64, bool) {
	if v == nil || *v <= 0 {
		return 0, false
	}
	return *v, true
}
