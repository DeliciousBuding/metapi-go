package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func normalizeSupportedModelsInput(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(input))
	for _, raw := range input {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

func normalizeAllowedRouteIdsInput(input []int64) []int64 {
	seen := make(map[int64]bool)
	result := make([]int64, 0, len(input))
	for _, raw := range input {
		if raw <= 0 {
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		result = append(result, raw)
		if len(result) >= 500 {
			break
		}
	}
	return result
}

func normalizeSiteWeightMultipliersInput(input any) map[string]float64 {
	if input == nil {
		return nil
	}
	// Accept both map[string]float64 (from struct) and raw JSON object.
	var raw map[string]any
	switch val := input.(type) {
	case map[string]float64:
		if len(val) == 0 {
			return nil
		}
		return val
	case map[string]any:
		raw = val
	default:
		data, err := json.Marshal(input)
		if err != nil {
			return nil
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil
		}
	}
	if len(raw) == 0 {
		return nil
	}
	result := make(map[string]float64, len(raw))
	for k, v := range raw {
		siteId, _ := strconv.ParseFloat(k, 64)
		if siteId <= 0 {
			continue
		}
		var multiplier float64
		switch mv := v.(type) {
		case float64:
			multiplier = mv
		case json.Number:
			multiplier, _ = mv.Float64()
		case int64:
			multiplier = float64(mv)
		default:
			continue
		}
		if multiplier <= 0 {
			continue
		}
		result[fmt.Sprintf("%.0f", siteId)] = multiplier
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func normalizeInt64Set(input []int64) []int64 {
	seen := make(map[int64]bool)
	result := make([]int64, 0, len(input))
	for _, raw := range input {
		if raw <= 0 {
			continue
		}
		if seen[raw] {
			continue
		}
		seen[raw] = true
		result = append(result, raw)
		if len(result) >= 500 {
			break
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

// normalizeExcludedCredentialRefsInput dedupes and canonicalizes credential
// refs shared by excludedCredentialRefs and allowedCredentialRefs. Malformed
// entries are rejected (non-empty reason) instead of silently dropped:
// dropping an allow-list entry would silently widen access (fail-open), and
// dropping an exclude entry would hide the operator's intent.
// Accepted shapes:
//   - {"kind":"account_token","siteId":>0,"accountId":>0,"tokenId":>0}
//   - {"kind":"default_api_key","siteId":>0,"accountId":>0}
//   - {"kind":"direct_grant","grantId":>0}
func normalizeExcludedCredentialRefsInput(input []any) ([]any, string) {
	seen := make(map[string]bool)
	result := make([]any, 0, len(input))
	for i, item := range input {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Sprintf("credentialRefs[%d] must be an object with kind/siteId/accountId", i)
		}
		kind, _ := obj["kind"].(string)
		kind = strings.TrimSpace(kind)
		if kind == "direct_grant" {
			grantId := coerceInt64(obj["grantId"])
			if grantId <= 0 {
				return nil, fmt.Sprintf("credentialRefs[%d] (direct_grant) requires a positive grantId", i)
			}
			dedupeKey := fmt.Sprintf("direct_grant:%d", grantId)
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			result = append(result, map[string]any{"kind": "direct_grant", "grantId": grantId})
			if len(result) >= 1000 {
				break
			}
			continue
		}
		siteId := coerceInt64(obj["siteId"])
		accountId := coerceInt64(obj["accountId"])
		if siteId <= 0 || accountId <= 0 {
			return nil, fmt.Sprintf("credentialRefs[%d] requires positive siteId and accountId", i)
		}

		var dedupeKey string
		if kind == "account_token" {
			tokenId := coerceInt64(obj["tokenId"])
			if tokenId <= 0 {
				return nil, fmt.Sprintf("credentialRefs[%d] (account_token) requires a positive tokenId", i)
			}
			dedupeKey = fmt.Sprintf("account_token:%d:%d:%d", siteId, accountId, tokenId)
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			result = append(result, map[string]any{
				"kind":      "account_token",
				"siteId":    siteId,
				"accountId": accountId,
				"tokenId":   tokenId,
			})
		} else if kind == "default_api_key" {
			dedupeKey = fmt.Sprintf("default_api_key:%d:%d", siteId, accountId)
			if seen[dedupeKey] {
				continue
			}
			seen[dedupeKey] = true
			result = append(result, map[string]any{
				"kind":      "default_api_key",
				"siteId":    siteId,
				"accountId": accountId,
			})
		} else {
			return nil, fmt.Sprintf("credentialRefs[%d] has unknown kind %q (expected account_token, default_api_key, or direct_grant)", i, kind)
		}
		if len(result) >= 1000 {
			break
		}
	}
	return result, ""
}

// normalizeKeyWeightInput accepts number|string|null for per-key weight.
// null / omitted / 0 / "" / non-positive → NULL (routing treats as 1.0).
func normalizeKeyWeightInput(input any) *float64 {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case float64:
		if v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			return &v
		}
		return nil
	case int:
		if v > 0 {
			f := float64(v)
			return &f
		}
		return nil
	case int64:
		if v > 0 {
			f := float64(v)
			return &f
		}
		return nil
	case json.Number:
		f, err := v.Float64()
		if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return &f
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return &f
	default:
		return nil
	}
}
