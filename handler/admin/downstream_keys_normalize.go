package admin

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

func redactDownstreamKeySecret(row map[string]any) {
	if row == nil {
		return
	}
	if key, ok := row["key"].(string); ok {
		row["keyMasked"] = maskSecret(key)
	}
	delete(row, "key")
}

func normalizeRange(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "24h", "7d", "all":
		return v
	default:
		return "24h"
	}
}

func normalizeStatus(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "enabled", "disabled":
		return v
	default:
		return "all"
	}
}

// --- helpers for extracting existing values from DB row maps ---

func rowValue(row map[string]any, key string) (any, bool) {
	if v, ok := row[key]; ok {
		return v, true
	}
	camelKey := snakeToCamel(key)
	if camelKey != key {
		v, ok := row[camelKey]
		return v, ok
	}
	return nil, false
}

func existingString(row map[string]any, key string) string {
	if v, ok := rowValue(row, key); ok {
		if s, ok2 := v.(string); ok2 {
			return s
		}
	}
	return ""
}

func existingStringPtr(row map[string]any, key string) *string {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	s, ok2 := v.(string)
	if !ok2 || s == "" {
		return nil
	}
	return &s
}

func existingBool(row map[string]any, key string) bool {
	v, ok := rowValue(row, key)
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val == "1" || strings.EqualFold(val, "true")
	}
	return false
}

func existingFloat64Ptr(row map[string]any, key string) *float64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case float64:
		return &val
	case int64:
		f := float64(val)
		return &f
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			return nil
		}
		return &f
	}
	return nil
}

func existingInt64Ptr(row map[string]any, key string) *int64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case int64:
		return &val
	case float64:
		i := int64(val)
		return &i
	case json.Number:
		i, err := val.Int64()
		if err != nil {
			return nil
		}
		return &i
	}
	return nil
}

// parseJsonField unmarshals a DB column value (string or already-parsed) into target.
func parseJsonField(row map[string]any, key string, target any) {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return
	}
	switch val := v.(type) {
	case string:
		if val == "" {
			return
		}
		json.Unmarshal([]byte(val), target)
	case []byte:
		if len(val) == 0 {
			return
		}
		json.Unmarshal(val, target)
	default:
		// Already parsed by sqlx (e.g. JSON column in PostgreSQL)
		data, _ := json.Marshal(val)
		json.Unmarshal(data, target)
	}
}

func parseStringArrayFromDB(row map[string]any, key string) []string {
	var arr []string
	parseJsonField(row, key, &arr)
	return arr
}

func parseIntArrayFromDB(row map[string]any, key string) []int64 {
	var arr []int64
	parseJsonField(row, key, &arr)
	return arr
}

func parseMapFromDB(row map[string]any, key string) map[string]float64 {
	v, ok := rowValue(row, key)
	if !ok || v == nil {
		return nil
	}
	switch val := v.(type) {
	case string:
		if val == "" || val == "{}" {
			return nil
		}
		var raw map[string]float64
		if err := json.Unmarshal([]byte(val), &raw); err != nil {
			return nil
		}
		return raw
	case []byte:
		if len(val) == 0 || string(val) == "{}" {
			return nil
		}
		var raw map[string]float64
		if err := json.Unmarshal(val, &raw); err != nil {
			return nil
		}
		return raw
	default:
		data, _ := json.Marshal(val)
		var raw map[string]float64
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil
		}
		return raw
	}
}

func parseAnyArrayFromDB(row map[string]any, key string) []any {
	var arr []any
	parseJsonField(row, key, &arr)
	return arr
}

// --- normalization helpers (mirrors TS normalizeDownstreamApiKeyPayload) ---

func normalizeGroupNameInput(input *string) *string {
	if input == nil {
		return nil
	}
	v := strings.TrimSpace(*input)
	if v == "" {
		return nil
	}
	if len(v) > 64 {
		v = v[:64]
	}
	return &v
}

func normalizeTagsInput(input []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(input))
	for _, raw := range input {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if len(v) > 32 {
			v = v[:32]
		}
		lower := strings.ToLower(v)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		result = append(result, v)
		if len(result) >= 20 {
			break
		}
	}
	return result
}

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

func normalizeExcludedCredentialRefsInput(input []any) []any {
	seen := make(map[string]bool)
	result := make([]any, 0, len(input))
	for _, item := range input {
		obj, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := obj["kind"].(string)
		kind = strings.TrimSpace(kind)
		siteId := coerceInt64(obj["siteId"])
		accountId := coerceInt64(obj["accountId"])
		if siteId <= 0 || accountId <= 0 {
			continue
		}

		var dedupeKey string
		if kind == "account_token" {
			tokenId := coerceInt64(obj["tokenId"])
			if tokenId <= 0 {
				continue
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
		}
		// Unknown kinds are silently skipped
		if len(result) >= 1000 {
			break
		}
	}
	return result
}

// normalizeDownstreamProxyURL trims proxyUrl for create/update.
// Empty / whitespace / null -> nil (inherit site/account/system proxy).
// Non-empty values must include a supported scheme (http/https/socks*).
func normalizeDownstreamProxyURL(input *string) (*string, string) {
	if input == nil {
		return nil, ""
	}
	v := strings.TrimSpace(*input)
	if v == "" {
		return nil, ""
	}
	lower := strings.ToLower(v)
	supported := false
	for _, scheme := range []string{"http://", "https://", "socks://", "socks4://", "socks4a://", "socks5://", "socks5h://"} {
		if strings.HasPrefix(lower, scheme) {
			supported = true
			break
		}
	}
	if !supported {
		return nil, "proxyUrl 必须以 http://、https:// 或 socks 代理 scheme 开头"
	}
	return &v, ""
}

// normalizeIPListPtr trims an IP allowlist/blocklist TEXT input. Returns nil
// for nil/empty/whitespace (NULL = unrestricted). Invalid entries are not
// validated here — auth.parseAllowlist silently skips them at enforcement time
// (matches the admin IP-allowlist behavior in auth/admin.go).
func normalizeIPListPtr(input *string) *string {
	if input == nil {
		return nil
	}
	v := strings.TrimSpace(*input)
	if v == "" {
		return nil
	}
	return &v
}

func normalizeExpiresAt(input *string) *string {
	if input == nil {
		return nil
	}
	v := strings.TrimSpace(*input)
	if v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		// Try other common formats.
		t, err = time.Parse("2006-01-02T15:04:05Z", v)
		if err != nil {
			t, err = time.Parse("2006-01-02T15:04:05.000Z", v)
			if err != nil {
				// If we can't parse it, keep the original string.
				// TS throws an error; we are lenient and just store the trimmed value.
				return &v
			}
		}
	}
	iso := t.UTC().Format(time.RFC3339)
	return &iso
}

// normalizeQuotaFloatOrNull implements maxCost clear/set semantics for create/update.
// Contract:
// - omitted on update → caller keeps existing (do not call this)
// - null / "" / missing any → NULL (unlimited)
// - 0 / negative / NaN / Inf → NULL (clear)
// - positive number or numeric string → stored value
func normalizeQuotaFloatOrNull(input any) *float64 {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case float64:
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		return &v
	case float32:
		f := float64(v)
		if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		return &f
	case int:
		if v <= 0 {
			return nil
		}
		f := float64(v)
		return &f
	case int64:
		if v <= 0 {
			return nil
		}
		f := float64(v)
		return &f
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
	case *float64:
		return normalizeQuotaFloatOrNull(derefAnyFloat(v))
	default:
		return nil
	}
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

// normalizeQuotaIntOrNull implements maxRequests clear/set semantics for create/update.
// Same contract as normalizeQuotaFloatOrNull: null/0/"" clear to unlimited.
func normalizeQuotaIntOrNull(input any) *int64 {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case float64:
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil
		}
		i := int64(v)
		return &i
	case float32:
		f := float64(v)
		if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil
		}
		i := int64(f)
		return &i
	case int:
		if v <= 0 {
			return nil
		}
		i := int64(v)
		return &i
	case int64:
		if v <= 0 {
			return nil
		}
		return &v
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			f, ferr := v.Float64()
			if ferr != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
				return nil
			}
			n := int64(f)
			return &n
		}
		if i <= 0 {
			return nil
		}
		return &i
	case string:
		s := strings.TrimSpace(v)
		if s == "" {
			return nil
		}
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			f, ferr := strconv.ParseFloat(s, 64)
			if ferr != nil || f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
				return nil
			}
			n := int64(f)
			return &n
		}
		if i <= 0 {
			return nil
		}
		return &i
	case *int64:
		if v == nil {
			return nil
		}
		return normalizeQuotaIntOrNull(*v)
	default:
		return nil
	}
}

func derefAnyFloat(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func toPersistenceJSON(v any) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case []string:
		if len(val) == 0 {
			return nil
		}
	case []int64:
		if len(val) == 0 {
			return nil
		}
	case []any:
		if len(val) == 0 {
			return nil
		}
	case map[string]float64:
		if len(val) == 0 {
			return nil
		}
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(data)
}

// --- policy reference validation (mirrors TS validatePolicyReferences) ---
