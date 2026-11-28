package admin

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
)

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

// normalizeDownstreamProxyURL trims proxyUrl for create/update.
// Empty / whitespace / null -> nil (inherit site/account/system proxy).
// Non-empty values share the same scheme/host validation as sites and
// accounts, so a key cannot store an egress target the transport cannot dial.
func normalizeDownstreamProxyURL(input *string) (*string, string) {
	if input == nil {
		return nil, ""
	}
	v := strings.TrimSpace(*input)
	if v == "" {
		return nil, ""
	}
	if err := service.ValidateProxyURL(v); err != nil {
		return nil, invalidProxyURLMessage
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
