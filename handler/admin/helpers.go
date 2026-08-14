package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Shared admin handler helpers: secret masking, value coercers, coalesce,
// DB row/query utilities, query-param parsing, and URL path parsing.
// Extracted from search.go / stats_helpers.go / sites.go / accounts.go /
// downstream_keys.go / token_routes.go / account_tokens.go.

// ---- Secret masking ----

// maskSecret masks a secret/token for JSON responses: first 4 + **** + last 4.
// Short secrets (<=8 chars) collapse to "****"; empty stays empty.
func maskSecret(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

// IsMaskedTokenValue checks if a token value contains masking characters.
func IsMaskedTokenValue(token string) bool {
	value := strings.TrimSpace(token)
	return value != "" && (strings.Contains(value, "*") || strings.Contains(value, "•"))
}

// ---- Numeric coercers ----

func coerceFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case []byte:
		f, _ := strconv.ParseFloat(string(n), 64)
		return f
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func coerceInt(v any) int {
	return int(coerceInt64(v))
}

func coerceInt64(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case []byte:
		i, _ := strconv.ParseInt(string(n), 10, 64)
		return i
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

// asInt converts v to int, reporting whether v was a whole-number type.
func asInt(v any) (int, bool) {
	switch v.(type) {
	case int, int64, float64:
		return int(coerceInt64(v)), true
	default:
		return 0, false
	}
}

// parseFlexibleFloat converts v to float64, reporting whether v was a usable
// number. Blank strings are "absent" (false) rather than 0.
func parseFlexibleFloat(v any) (float64, bool) {
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		return f, err == nil
	}
	switch v.(type) {
	case float64, float32, int, int64:
		return coerceFloat(v), true
	default:
		return 0, false
	}
}

// ---- Coalesce ----

// coalesce returns fallback when v is empty.
func coalesce(v string, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// coalescePtr returns fallback when v is nil.
func coalescePtr[T any](v *T, fallback T) T {
	if v == nil {
		return fallback
	}
	return *v
}

// ---- DB row helpers (moved from search.go) ----

func queryRows(db *sqlx.DB, query string, args ...any) []map[string]any {
	result, _ := queryRowsErr(db, query, args...)
	return result
}

func queryRowsErr(db *sqlx.DB, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.Queryx(rebindAdminQuery(db, query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		result = append(result, mapKeysToCamel(row))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func rebindAdminQuery(db *sqlx.DB, query string) string {
	if db == nil {
		return query
	}
	return db.Rebind(query)
}

func normalizeSlice(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}

// snakeToCamel converts snake_case to camelCase.
// e.g. "model_pattern" -> "modelPattern", "id" -> "id"
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// mapKeysToCamel returns a new map with all keys converted from snake_case to camelCase.
func mapKeysToCamel(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[snakeToCamel(k)] = v
	}
	return result
}

// ---- Query param parsing ----

func getQueryInt(r *http.Request, key string, fallback int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return fallback
	}
	return n
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ---- URL path helpers ----

// pathID parses the chi "id" URL param as a positive int64 and writes a
// unified 400 error when invalid. Callers must return when ok is false.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	idStr := strings.TrimSpace(chi.URLParam(r, "id"))
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "无效的 ID")
		return 0, false
	}
	return id, true
}

// parseLimitOffset parses ?limit= and ?offset= query params, clamping limit to
// [1, maxLimit] and offset to >= 0.
func parseLimitOffset(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = clampInt(getQueryInt(r, "limit", defaultLimit), 1, maxLimit)
	offset = max(0, getQueryInt(r, "offset", 0))
	return limit, offset
}
