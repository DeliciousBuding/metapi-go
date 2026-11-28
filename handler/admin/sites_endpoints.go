package admin

import (
	"fmt"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func normalizeAPIEndpointsInput(input []payloads.SiteAPIEndpointInput) ([]payloads.SiteAPIEndpointInput, string) {
	if input == nil {
		return nil, ""
	}

	seen := map[string]bool{}
	var result []payloads.SiteAPIEndpointInput

	for i, ep := range input {
		normalizedURL := service.NormalizeSiteAPIEndpointBaseUrl(ep.URL)
		if normalizedURL == "" {
			return nil, "Invalid apiEndpoints url. Expected a valid http(s) URL."
		}
		// Prefer the specific metadata/link-local 400 before the generic
		// scheme check. IsValidAPIEndpointURL also rejects these targets,
		// so order only affects the error message (double-safe).
		if service.IsForbiddenSiteTargetURL(normalizedURL) {
			return nil, "Invalid apiEndpoints url. Cloud metadata / link-local targets are not allowed."
		}
		if !service.IsValidAPIEndpointURL(normalizedURL) {
			return nil, "Invalid apiEndpoints url. Expected a valid http(s) URL."
		}
		if seen[normalizedURL] {
			return nil, fmt.Sprintf("Duplicate apiEndpoints url: %s", normalizedURL)
		}
		seen[normalizedURL] = true
		sortOrder := ep.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		result = append(result, payloads.SiteAPIEndpointInput{
			URL: normalizedURL, Enabled: ep.Enabled, SortOrder: sortOrder,
		})
	}
	return result, ""
}

func siteEndpointRecords(eps []payloads.SiteAPIEndpointInput, now string) []store.SiteAPIEndpoint {
	var storeEps []store.SiteAPIEndpoint
	for _, ep := range eps {
		storeEps = append(storeEps, store.SiteAPIEndpoint{
			URL: ep.URL, Enabled: ep.Enabled, SortOrder: int64(ep.SortOrder),
			CreatedAt: now, UpdatedAt: now,
		})
	}
	return storeEps
}
