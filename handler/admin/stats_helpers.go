package admin

import (
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// Stats handler helper functions extracted from stats.go for readability.
// These are pure value-coercion / query / time helpers shared across the
// stats handlers and the marketplace/token-candidate builders. Moving them
// here is behavior-neutral (same package, same exported surface).
//

func modelAllowed(model string, allowed map[string]struct{}) bool {
	if allowed == nil || len(allowed) == 0 {
		return true
	}
	_, ok := allowed[strings.ToLower(strings.TrimSpace(model))]
	return ok
}

func parseTruthyQuery(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func queryRow(db *sqlx.DB, query string, args ...any) map[string]any {
	rows, err := db.Queryx(rebindAdminQuery(db, query), args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	if rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil
		}
		return mapKeysToCamel(row)
	}
	return nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func roundMicro(v float64) float64 {
	return float64(int64(v*1_000_000)) / 1_000_000
}

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

func coerceString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		if v == nil {
			return ""
		}
		return strings.TrimSpace(strconv.FormatInt(coerceInt64(v), 10))
	}
}
