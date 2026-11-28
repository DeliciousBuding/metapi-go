package auth

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// managedKeyView is an internal struct mirroring the downstream_api_keys row
// with parsed JSON columns. Used only within the auth package for authorization.
type managedKeyView struct {
	KeyOwnerType        string
	OwnerUserID         *int64
	UserGroupID         *int64
	UserRouteIDs        *[]int64
	AccessPolicy        *store.DownstreamAccessPolicy
	InvalidAccessPolicy bool
	ID                  int64
	Name                string
	Enabled             bool
	ExpiresAt           *string
	MaxCost             *float64
	UsedCost            float64
	MaxRequests         *int64
	UsedRequests        int64
	// ProxyURL is the optional per-key egress proxy (downstream_api_keys.proxy_url).
	// nil/empty means inherit site/account/system proxy.
	ProxyURL *string
	// MaxRPM / MaxTPM optional soft rate windows.
	MaxRPM *int64
	MaxTPM *int64
	// Policy fields — parsed from JSON TEXT columns
	SupportedModels        []string
	AllowedRouteIDs        []int64
	SiteWeightMultipliers  map[int64]float64
	KeyWeight              float64
	ExcludedSiteIDs        []int64
	ExcludedCredentialRefs []ExcludedCredentialRef
	AllowedSiteIDs         []int64
	AllowedCredentialRefs  []ExcludedCredentialRef
	// IPAllowlist / IPBlocklist raw TEXT. nil/empty = unrestricted.
	// Parsed lazily at enforcement time via parseAllowlist (shared with admin IP
	// guard). Blocklist wins over allowlist.
	IPAllowlist *string
	IPBlocklist *string
}

// getManagedKeyByToken queries the downstream_api_keys table for a row matching
// the given token (key column). Returns nil if no row matches.
func getManagedKeyByToken(db *store.DB, token string) (*managedKeyView, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	return loadManagedKeyFrom(db.DB, "key", token)
}

// loadManagedKeyFrom queries downstream_api_keys by one indexed column ("key"
// or "id") on the given handle and parses the JSON columns. Returns
// (nil, nil) when no row matches. The explicit handle keeps admin read paths
// on their scoped/test database instead of the process-global singleton.
func loadManagedKeyFrom(db *sqlx.DB, column string, value any) (*managedKeyView, error) {
	if column != "key" && column != "id" {
		return nil, fmt.Errorf("unsupported managed key lookup column %q", column)
	}
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	row := db.QueryRowx(db.Rebind(
		`SELECT id, name, enabled, expires_at, max_cost, used_cost,
		        max_requests, used_requests, proxy_url, max_rpm, max_tpm,
		        supported_models, allowed_route_ids,
		        site_weight_multipliers, key_weight, excluded_site_ids,
		        excluded_credential_refs, allowed_site_ids, allowed_credential_refs,
		        ip_allowlist, ip_blocklist, access_policy, key_owner_type, owner_user_id
		 FROM downstream_api_keys
		 WHERE `+column+` = ?`), value,
	)

	var v managedKeyView
	var proxyURL, accessPolicyJSON *string
	var keyWeight *float64
	var supportedModelsJSON, allowedRouteIDsJSON *string
	var siteWeightMultiJSON, excludedSiteIDsJSON *string
	var excludedCredRefsJSON *string
	var allowedSiteIDsJSON, allowedCredRefsJSON *string

	err := row.Scan(
		&v.ID, &v.Name, &v.Enabled, &v.ExpiresAt, &v.MaxCost, &v.UsedCost,
		&v.MaxRequests, &v.UsedRequests, &proxyURL, &v.MaxRPM, &v.MaxTPM,
		&supportedModelsJSON, &allowedRouteIDsJSON,
		&siteWeightMultiJSON, &keyWeight, &excludedSiteIDsJSON,
		&excludedCredRefsJSON, &allowedSiteIDsJSON, &allowedCredRefsJSON,
		&v.IPAllowlist, &v.IPBlocklist, &accessPolicyJSON, &v.KeyOwnerType, &v.OwnerUserID,
	)
	if err != nil {
		// sql.ErrNoRows → return nil, nil (not found)
		if isNoRows(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("query downstream_api_keys: %w", err)
	}

	if accessPolicyJSON != nil {
		var parseErr error
		v.AccessPolicy, parseErr = store.ParseDownstreamAccessPolicy(*accessPolicyJSON)
		v.InvalidAccessPolicy = parseErr != nil
	}
	// Normalize proxy_url: whitespace-only → nil (inherit site/system).
	if proxyURL != nil {
		trimmed := strings.TrimSpace(*proxyURL)
		if trimmed != "" {
			v.ProxyURL = &trimmed
		}
	}

	if keyWeight != nil && *keyWeight > 0 && !math.IsNaN(*keyWeight) && !math.IsInf(*keyWeight, 0) {
		v.KeyWeight = *keyWeight
	}

	// Parse JSON columns into typed slices/maps
	v.SupportedModels = parseStringArray(supportedModelsJSON)
	v.AllowedRouteIDs = parseInt64Array(allowedRouteIDsJSON)
	v.SiteWeightMultipliers = parseSiteWeightMultipliers(siteWeightMultiJSON)
	v.ExcludedSiteIDs = parseInt64Array(excludedSiteIDsJSON)
	v.ExcludedCredentialRefs = parseExcludedCredentialRefs(excludedCredRefsJSON)
	v.AllowedSiteIDs = parseInt64Array(allowedSiteIDsJSON)
	v.AllowedCredentialRefs = parseExcludedCredentialRefs(allowedCredRefsJSON)

	return &v, nil
}

// parseStringArray parses a JSON TEXT column containing an array of strings.
// Returns nil on empty, null, or parse errors.
func parseStringArray(raw *string) []string {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var arr []string
	if err := json.Unmarshal([]byte(*raw), &arr); err != nil {
		return nil
	}
	return arr
}

// parseInt64Array parses a JSON TEXT column containing an array of numbers.
// Returns an empty slice on nil, empty, or parse errors.
func parseInt64Array(raw *string) []int64 {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var arr []json.Number
	if err := json.Unmarshal([]byte(*raw), &arr); err != nil {
		// Try parsing as []int64 directly (PostgreSQL may store differently)
		var intArr []int64
		if err2 := json.Unmarshal([]byte(*raw), &intArr); err2 != nil {
			return nil
		}
		return intArr
	}
	result := make([]int64, 0, len(arr))
	for _, n := range arr {
		v, err := n.Int64()
		if err != nil {
			continue
		}
		result = append(result, v)
	}
	return result
}

// parseSiteWeightMultipliers parses a JSON TEXT column containing a
// {"site_id": multiplier} object.
func parseSiteWeightMultipliers(raw *string) map[int64]float64 {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var rawMap map[string]json.Number
	if err := json.Unmarshal([]byte(*raw), &rawMap); err != nil {
		// Try direct float map (PostgreSQL)
		var floatMap map[int64]float64
		if err2 := json.Unmarshal([]byte(*raw), &floatMap); err2 != nil {
			return nil
		}
		return floatMap
	}
	result := make(map[int64]float64, len(rawMap))
	for k, n := range rawMap {
		siteID, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			continue
		}
		multiplier, err := n.Float64()
		if err != nil {
			continue
		}
		result[siteID] = multiplier
	}
	return result
}

// parseExcludedCredentialRefs parses a JSON TEXT column containing an array of
// ExcludedCredentialRef objects.
func parseExcludedCredentialRefs(raw *string) []ExcludedCredentialRef {
	if raw == nil || *raw == "" || *raw == "null" {
		return nil
	}
	var refs []ExcludedCredentialRef
	if err := json.Unmarshal([]byte(*raw), &refs); err != nil {
		return nil
	}
	return refs
}

// isNoRows checks whether the error is a "no rows in result set" error.
func isNoRows(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no rows") ||
		strings.Contains(msg, "sql: no rows")
}
