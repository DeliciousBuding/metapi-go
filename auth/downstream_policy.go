package auth

import (
	"strings"
)

// toPolicyFromView derives a DownstreamRoutingPolicy from a managed key view.
// Managed keys default to DenyAllWhenEmpty=true (deny all models when no
// patterns/routes are configured).
func toPolicyFromView(v *managedKeyView) DownstreamRoutingPolicy {
	return DownstreamRoutingPolicy{
		UserRouteIDs:           v.UserRouteIDs,
		AccessPolicy:           v.AccessPolicy,
		SupportedModels:        normalizeStringSlice(v.SupportedModels),
		AllowedRouteIDs:        normalizeInt64Slice(v.AllowedRouteIDs),
		SiteWeightMultipliers:  normalizeSiteWeightMap(v.SiteWeightMultipliers),
		KeyWeight:              v.KeyWeight,
		ExcludedSiteIDs:        normalizeInt64Slice(v.ExcludedSiteIDs),
		ExcludedCredentialRefs: normalizeExcludedRefs(v.ExcludedCredentialRefs),
		AllowedSiteIDs:         normalizeInt64Slice(v.AllowedSiteIDs),
		AllowedCredentialRefs:  normalizeExcludedRefs(v.AllowedCredentialRefs),
		DenyAllWhenEmpty:       true, // managed keys default to deny-all
	}
}

// splitIPList splits a TEXT blob of newline/comma-separated IP/CIDR entries
// into a clean []string (empty/whitespace entries dropped). Mirrors the format
// the admin UI stores; parseAllowlist then validates each into exact/CIDR.
func splitIPList(raw *string) []string {
	if raw == nil {
		return nil
	}
	s := strings.TrimSpace(*raw)
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r", "\n")
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		// Allow comma-separated within a single line too.
		for _, entry := range strings.Split(p, ",") {
			entry = strings.TrimSpace(entry)
			if entry != "" {
				out = append(out, entry)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// CheckDownstreamKeyIP reports whether a managed key permits a client IP.
// Returns (allowed, denyReason). Empty clientIP (e.g. internal test calls)
// is treated as "no restriction": the caller is responsible for passing a
// real RemoteAddr-derived IP at the proxy edge.

// Semantics (matches admin IP allowlist + New API token IP rules):
// - Blocklist wins: any match → deny ("ip_blocked").
// - Allowlist non-empty: must match → else deny ("ip_not_allowed").
// - Both empty: unrestricted → allow.
func CheckDownstreamKeyIP(key *managedKeyView, clientIP string) (bool, string) {
	if key == nil {
		return true, ""
	}
	ip := strings.TrimSpace(clientIP)
	if ip == "" {
		if key.AccessPolicy != nil && len(splitIPList(key.IPAllowlist)) > 0 {
			return false, "ip_not_allowed"
		}
		return true, "" // Preserve legacy callers; imported restrictions fail closed.
	}
	blockEntries := splitIPList(key.IPBlocklist)
	if len(blockEntries) > 0 {
		parsed := parseAllowlist(blockEntries)
		if isIPAllowed(ip, parsed) {
			return false, "ip_blocked"
		}
	}
	allowEntries := splitIPList(key.IPAllowlist)
	if len(allowEntries) > 0 {
		parsed := parseAllowlist(allowEntries)
		if !isIPAllowed(ip, parsed) {
			return false, "ip_not_allowed"
		}
	}
	return true, ""
}

func normalizeStringSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func normalizeInt64Slice(s []int64) []int64 {
	if s == nil {
		return []int64{}
	}
	return s
}

func normalizeSiteWeightMap(m map[int64]float64) map[int64]float64 {
	if m == nil {
		return map[int64]float64{}
	}
	return m
}

func normalizeExcludedRefs(r []ExcludedCredentialRef) []ExcludedCredentialRef {
	if r == nil {
		return []ExcludedCredentialRef{}
	}
	return r
}
