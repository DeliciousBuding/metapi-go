package admin

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
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

// Both store-backed handlers and legacy route handlers use the same row projection.
func queryRow[R interface{ MapScan(map[string]any) error }](db interface {
	Rebind(string) string
	QueryRowx(string, ...any) R
}, query string, args ...any) (map[string]any, error) {
	row := make(map[string]any)
	err := db.QueryRowx(db.Rebind(query), args...).MapScan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return mapKeysToCamel(row), nil
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
