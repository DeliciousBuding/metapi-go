package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
)

// parseStringListSetting decodes a settings-table value into a trimmed,
// de-duplicated string list. It accepts:
// - JSON arrays: ["a","b"]
// - JSON null → empty list
// - legacy double-encoded JSON arrays: "[\"a\",\"b\"]"
// - comma-separated plain strings: "a, b"

// ok=false means the value is present but unusable; callers must not overwrite
// the in-memory setting with an empty default in that case.
func parseStringListSetting(raw string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}

	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		// Non-JSON legacy plain string (comma-separated).
		return normalizeStringList(raw), true
	}

	switch v := decoded.(type) {
	case nil:
		return []string{}, true
	case []any:
		return normalizeStringListAny(v), true
	case []string:
		return normalizeStringList(v...), true
	case string:
		// Double-encoded JSON array/string, or comma-separated plain string.
		inner := strings.TrimSpace(v)
		if inner == "" {
			return []string{}, true
		}
		var nested any
		if err := json.Unmarshal([]byte(inner), &nested); err == nil {
			switch nv := nested.(type) {
			case nil:
				return []string{}, true
			case []any:
				return normalizeStringListAny(nv), true
			case []string:
				return normalizeStringList(nv...), true
			case string:
				return normalizeStringList(nv), true
			default:
				return nil, false
			}
		}
		return normalizeStringList(inner), true
	default:
		return nil, false
	}
}

func normalizeStringListAny(items []any) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func normalizeStringList(parts ...string) []string {
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		for _, item := range strings.Split(part, ",") {
			s := strings.TrimSpace(item)
			if s == "" {
				continue
			}
			if _, exists := seen[s]; exists {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// parseNumberSetting decodes a JSON number setting. The admin write path
// marshals ints and floats with encoding/json, so values arrive as "30" or
// "0.25". Unlike parseFloatSetting it keeps negative numbers, letting each
// case apply the exact clamp config.Load uses for the matching env var instead
// of silently substituting the fallback. Unparseable or non-finite values keep
// the fallback.
func parseNumberSetting(raw string, fallback float64) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return fallback
	}
	return v
}

// parseIntSetting mirrors the "max(lo, trunc(n))" shape config.Load uses for
// its integer settings, so a number persisted through the admin API is clamped
// identically to the same number supplied through env.
func parseIntSetting(raw string, fallback float64, lo int) int {
	return config.MaxInt(lo, int(math.Trunc(parseNumberSetting(raw, fallback))))
}

// parseBoolSetting parses a boolean setting value.
func parseBoolSetting(value string, fallback bool) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return fallback
	}
	return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
}

// parseInt parses an integer setting value, returning fallback on failure.
func parseInt(value string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return v
}

// parseFloatSetting parses a non-negative float setting; returns fallback on
// parse error, NaN, Inf, or negative. 0 is preserved (free cache read).
func parseFloatSetting(value string, fallback float64) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || !(v >= 0) || isNaNOrInf(v) {
		return fallback
	}
	return v
}

func isNaNOrInf(v float64) bool {
	return v != v || v > 1e308 || v < -1e308
}

func parseJSONSettingString(value string) string {
	value = strings.TrimSpace(value)
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.TrimSpace(value)
}

// parseJSONValueSetting decodes a settings row that holds arbitrary JSON. An
// empty cell, or one encoding/json cannot read, means the persisted intent is
// unrecoverable: the caller must keep the value it already resolved instead of
// assigning the failure result, because config.ParseJsonValue encodes that
// failure as nil and a direct assignment turned one unreadable row into an
// empty rule set. A readable JSON null IS intent ("no rules") and comes back as
// (nil, nil).
func parseJSONValueSetting(raw string) (any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("value is empty")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	return decoded, nil
}

// parseNotifyTaskTogglesSetting decodes the per-alert-type mute map. Only a
// JSON object of booleans — or null, meaning "no per-type overrides" — carries
// a readable intent; an array, a bare scalar or a non-boolean flag is a row the
// caller must refuse to apply.
func parseNotifyTaskTogglesSetting(raw string) (map[string]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("value is empty")
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return nil, fmt.Errorf("not valid JSON: %v", err)
	}
	switch v := decoded.(type) {
	case nil:
		return map[string]bool{}, nil
	case map[string]any:
		toggles := make(map[string]bool, len(v))
		for name, flag := range v {
			enabled, ok := flag.(bool)
			if !ok {
				return nil, fmt.Errorf("toggle %q is %s, want a boolean",
					name, describeJSONSettingValue(flag))
			}
			toggles[name] = enabled
		}
		return toggles, nil
	default:
		return nil, fmt.Errorf("JSON %s, want an object of booleans", describeJSONSettingValue(decoded))
	}
}

// warnUnreadableSettingRow logs the one line an operator needs when hydration
// refuses a persisted row: which key, why, and which value the process keeps.
// A refusal the log does not mention leaves the settings table and
// GET /api/settings/runtime disagreeing with nothing to explain the gap.
func warnUnreadableSettingRow(key, reason string, kept any) {
	slog.Warn("settings: persisted value not applied, keeping the resolved value",
		"key", key, "reason", reason, "kept", kept)
}

// describeJSONSettingValue names the shape of a resolved JSON setting so a
// warning can say what the process keeps without echoing the whole blob.
func describeJSONSettingValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return fmt.Sprintf("object with %d keys", len(value))
	case []any:
		return fmt.Sprintf("array with %d items", len(value))
	case string:
		return "string"
	case bool:
		return fmt.Sprintf("bool %t", value)
	case float64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// validatedCronSetting checks the expression before replacing a resolved schedule.
func validatedCronSetting(key, value string) (string, bool) {
	if config.ValidateCronExpr(value) {
		return value, true
	}
	slog.Warn("settings: ignoring invalid " + key + " value")
	return "", false
}
