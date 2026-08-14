package admin

import (
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Stats handler helper functions extracted from stats.go for readability.
// These are pure value-coercion / query / time helpers shared across the
// stats handlers and the marketplace/token-candidate builders. Moving them
// here is behavior-neutral (same package, same exported surface).
//

func modelAllowed(model string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
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

func roundMicro(v float64) float64 {
	return float64(int64(v*1_000_000)) / 1_000_000
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
