package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ---- Normalization helpers (mirrors TS sites.ts normalize functions) ----

func NormalizeSortOrder(input *int) *int {
	if input == nil {
		return nil
	}
	v := *input
	if v < 0 {
		v = 0
	}
	return &v
}

func NormalizeGlobalWeight(input *float64) *float64 {
	if input == nil {
		return nil
	}
	v := *input
	if math.IsInf(v, 0) || math.IsNaN(v) || v <= 0 {
		return nil
	}
	clamped := math.Max(0.01, math.Min(100, v))
	rounded := math.Round(clamped*1000) / 1000
	return &rounded
}

// NormalizeNullable returns nil for empty string, the value otherwise.
func NormalizeNullable(s *string) *string {
	if s == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// accountAgg is used for aggregating account balance/subscription data.
type accountAgg struct {
	SiteID      int64   `db:"site_id"`
	Balance     float64 `db:"balance"`
	ExtraConfig *string `db:"extra_config"`
}

// ---- API Endpoint management ----

// UpsertSiteAPIEndpoints replaces all apiEndpoints for a site within a transaction.
func UpsertSiteAPIEndpoints(tx *sqlx.Tx, siteID int64, endpoints []store.SiteAPIEndpoint) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Delete existing endpoints
	_, err := tx.Exec(tx.Rebind("DELETE FROM site_api_endpoints WHERE site_id = ?"), siteID)
	if err != nil {
		return fmt.Errorf("delete site_api_endpoints: %w", err)
	}

	// Insert new endpoints
	for i := range endpoints {
		ep := &endpoints[i]
		normalizedURL := NormalizeSiteAPIEndpointBaseUrl(ep.URL)
		if normalizedURL == "" {
			continue
		}
		if IsForbiddenSiteTargetURL(normalizedURL) {
			return fmt.Errorf("site api endpoint url rejects cloud metadata / link-local targets")
		}
		enabled := true
		if !ep.Enabled {
			enabled = false
		}
		sortOrder := ep.SortOrder
		if sortOrder == 0 && i > 0 {
			sortOrder = int64(i)
		}
		_, err := tx.Exec(
			tx.Rebind(`INSERT INTO site_api_endpoints (site_id, url, enabled, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`),
			siteID, normalizedURL, enabled, sortOrder, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert site_api_endpoint: %w", err)
		}
	}
	return nil
}

// LoadSiteAPIEndpoints loads all api endpoints for a list of site IDs.
func LoadSiteAPIEndpoints(db *sqlx.DB, siteIDs []int64) (map[int64][]store.SiteAPIEndpoint, error) {
	result := make(map[int64][]store.SiteAPIEndpoint)
	if len(siteIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, site_id, url, enabled, sort_order, cooldown_until,
		        last_selected_at, last_failed_at, last_failure_reason, created_at, updated_at
		 FROM site_api_endpoints
		 WHERE site_id IN (?)
		 ORDER BY site_id, sort_order, id`,
		siteIDs,
	)
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)

	var rows []store.SiteAPIEndpoint
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.SiteID] = append(result[row.SiteID], row)
	}
	return result, nil
}

// SiteSelectColumns lists known sites columns for SELECT (never SELECT *).
// Shared PG CI DBs may have leftover probe columns from schema experiments;
// SELECT * would fail struct scan with "missing destination name...".
const SiteSelectColumns = `id, name, url, external_checkin_url, platform, proxy_url, use_system_proxy,
	custom_headers, custom_headers_override_request_headers, status, is_pinned, sort_order, global_weight, api_key, max_concurrency,
	post_refresh_probe_enabled, post_refresh_probe_model, post_refresh_probe_scope,
	post_refresh_probe_latency_threshold_ms, tags, cf_clearance, browser_ua, resin_enabled, use_utls, created_at, updated_at`

// LoadSiteWithEndpoints loads a single site with its apiEndpoints attached.
func LoadSiteWithEndpoints(db *sqlx.DB, siteID int64) (map[string]any, error) {
	var site store.Site
	err := db.Get(&site, db.Rebind("SELECT "+SiteSelectColumns+" FROM sites WHERE id = ?"), siteID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	endpoints, err := LoadSiteAPIEndpoints(db, []int64{siteID})
	if err != nil {
		return nil, err
	}

	return siteToMap(site, endpoints[siteID]), nil
}

func siteToMap(site store.Site, endpoints []store.SiteAPIEndpoint) map[string]any {
	return map[string]any{
		"id":                                  site.ID,
		"name":                                site.Name,
		"url":                                 site.URL,
		"externalCheckinUrl":                  site.ExternalCheckinURL,
		"platform":                            site.Platform,
		"proxyUrl":                            site.ProxyURL,
		"useSystemProxy":                      site.UseSystemProxy,
		"customHeaders":                       site.CustomHeaders,
		"customHeadersOverrideRequestHeaders": site.CustomHeadersOverrideRequestHeaders,
		"status":                              site.Status,
		"isPinned":                            site.IsPinned,
		"sortOrder":                           site.SortOrder,
		"globalWeight":                        site.GlobalWeight,
		"apiKey":                              site.APIKey,
		"maxConcurrency":                      site.MaxConcurrency,
		"postRefreshProbeEnabled":             site.PostRefreshProbeEnabled,
		"postRefreshProbeModel":               site.PostRefreshProbeModel,
		"postRefreshProbeScope":               site.PostRefreshProbeScope,
		"postRefreshProbeLatencyThresholdMs":  site.PostRefreshProbeLatencyThresholdMs,
		"browserUa":                           site.BrowserUA,
		"cfClearance":                         site.CfClearance,
		"resinEnabled":                        site.ResinEnabled,
		"useUtls":                             site.UseUTLS,
		"createdAt":                           site.CreatedAt,
		"updatedAt":                           site.UpdatedAt,
		"apiEndpoints":                        endpoints,
	}
}

// ---- Site CRUD ----

// ListSites returns all sites with apiEndpoints, totalBalance, and subscriptionSummary.
func ListSites(db *sqlx.DB) ([]map[string]any, error) {
	var sites []store.Site
	if err := db.Select(&sites, "SELECT "+SiteSelectColumns+" FROM sites ORDER BY sort_order, id"); err != nil {
		return nil, err
	}

	siteIDs := make([]int64, len(sites))
	for i, s := range sites {
		siteIDs[i] = s.ID
	}
	endpointsBySite, err := LoadSiteAPIEndpoints(db, siteIDs)
	if err != nil {
		return nil, err
	}

	// Aggregate totalBalance and subscriptionSummary per site.
	//
	// The accounts query pulls balance + extra_config for every account once.
	// We then build both the per-site balance sum and the per-site sub2api
	// subscription summary in a SINGLE pass over the accounts slice (O(accounts))
	// before the site loop, so the site loop does O(1) map lookups instead of
	// rescanning the whole accounts slice per site (the previous behavior was
	// O(sites × accounts) — 50 sites × 1000 accounts = 50,000 iterations).
	//
	// A LIKE filter on extra_config was considered to skip non-sub2api accounts
	// at the DB layer, but balanceBySite must sum across ALL accounts (sub2api
	// or not), so filtering the shared query would change totalBalance. We keep
	// the unfiltered query and let buildSubscriptionSummariesBySite skip
	// non-sub2api accounts in-memory (GetSub2ApiAuthFromExtraConfig returns nil
	// for them, so they never allocate a map entry).
	var accounts []accountAgg
	// COALESCE(balance, 0): accounts migrated from the TS version (or never
	// refreshed) may carry a NULL balance, which sqlx cannot scan into float64
	// and would 500 the whole /api/sites response. Aggregation treats an
	// unknown balance as 0, matching the stats endpoints' existing convention.
	if err := db.Select(&accounts, "SELECT site_id, COALESCE(balance, 0) AS balance, extra_config FROM accounts"); err != nil {
		return nil, err
	}

	balanceBySite := make(map[int64]float64)
	summariesBySite := buildSubscriptionSummariesBySite(accounts)
	for _, a := range accounts {
		balanceBySite[a.SiteID] += a.Balance
	}

	result := make([]map[string]any, len(sites))
	for i, s := range sites {
		siteMap := siteToMap(s, endpointsBySite[s.ID])
		totalBalance := math.Round(balanceBySite[s.ID]*1_000_000) / 1_000_000
		siteMap["totalBalance"] = totalBalance
		siteMap["subscriptionSummary"] = summariesBySite[s.ID]
		result[i] = siteMap
	}

	return result, nil
}

// subscriptionSummary is the JSON-serializable summary of a site's sub2api
// subscription state, aggregated from each account's extra_config.sub2apiAuth.
// Mirrors the TS sites.ts buildSubscriptionSummary shape consumed by the
// frontend (web/src/features/sites/components/sites-columns.tsx uses
// activeCount as an accountCount fallback).
type subscriptionSummary struct {
	Group       string     `json:"group,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	Active      bool       `json:"active"`
	ActiveCount int        `json:"activeCount"`
}

// buildSubscriptionSummariesBySite aggregates sub2api subscription info for
// every site in a single pass over the accounts slice, returning a map keyed
// by site ID. Sites whose accounts carry no sub2apiAuth block are absent from
// the map (callers treat a missing key as nil — the pre-existing behavior, so
// non-sub2api sites stay unaffected).
//
// This is the O(accounts) replacement for the previous per-site
// buildSubscriptionSummary(accounts, siteID) call inside the site loop, which
// rescanned the whole accounts slice per site (O(sites × accounts)).
func buildSubscriptionSummariesBySite(accounts []accountAgg) map[int64]*subscriptionSummary {
	result := make(map[int64]*subscriptionSummary)
	now := time.Now().UTC()
	for _, acc := range accounts {
		auth := GetSub2ApiAuthFromExtraConfig(acc.ExtraConfig)
		if auth == nil {
			continue
		}
		summary := result[acc.SiteID]
		if summary == nil {
			summary = &subscriptionSummary{}
			result[acc.SiteID] = summary
		}

		// Subscription group/plan name: prefer "group", fall back to "planName".
		// First-seen wins so the caller-visible group is stable across accounts.
		if summary.Group == "" {
			if g, ok := auth["group"].(string); ok && g != "" {
				summary.Group = g
			} else if p, ok := auth["planName"].(string); ok && p != "" {
				summary.Group = p
			}
		}

		// tokenExpiresAt is epoch seconds (see NormalizeManagedTokenExpiresAt).
		if exp, ok := NormalizeManagedTokenExpiresAt(auth["tokenExpiresAt"]); ok && exp > 0 {
			expiry := time.Unix(exp, 0).UTC()
			if summary.ExpiresAt == nil || expiry.After(*summary.ExpiresAt) {
				summary.ExpiresAt = &expiry
			}
			if expiry.After(now) {
				summary.ActiveCount++
			}
		} else {
			// No usable expiry — treat as active so accountCount fallback still works.
			summary.ActiveCount++
		}
	}
	// Derive the Active flag once after all accounts are folded in, since
	// ActiveCount is incremented across multiple accounts per site.
	for _, summary := range result {
		summary.Active = summary.ActiveCount > 0
	}
	return result
}

// buildSubscriptionSummary aggregates sub2api subscription info for a single
// site by scanning every account's extra_config for a sub2apiAuth object.
//
// Kept for backward compatibility with existing tests and any caller that
// needs a single-site summary. It delegates to buildSubscriptionSummariesBySite
// (single pass) and looks up the requested site. Returns an untyped nil when
// no account for the site carries a sub2apiAuth block (matching the original
// behavior; indexing a missing map key would yield a typed-nil *subscriptionSummary
// which compares != nil, so we guard with the comma-ok form).
func buildSubscriptionSummary(accounts []accountAgg, siteID int64) any {
	if s, ok := buildSubscriptionSummariesBySite(accounts)[siteID]; ok {
		return s
	}
	return nil
}

// CreateSite creates a new site with apiEndpoints in a transaction.
func CreateSite(db *sqlx.DB, siteData map[string]any) (int64, error) {
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	// Load existing max sort order
	var maxSort int64
	tx.Get(&maxSort, tx.Rebind("SELECT COALESCE(MAX(sort_order), -1) FROM sites"))

	sortOrder := maxSort + 1
	if so, ok := siteData["sortOrder"].(int64); ok {
		sortOrder = so
	}

	name := siteData["name"].(string)
	urlStr := CanonicalizeSiteURL(siteData["url"].(string))
	platform := siteData["platform"].(string)

	maxConcurrency := int64(0)
	if v, ok := siteData["maxConcurrency"]; ok && v != nil {
		switch n := v.(type) {
		case int64:
			maxConcurrency = n
		case int:
			maxConcurrency = int64(n)
		case float64:
			maxConcurrency = int64(n)
		}
		if maxConcurrency < 0 {
			maxConcurrency = 0
		}
	}

	useSystemProxy, _ := siteData["useSystemProxy"].(bool)
	customHeadersOverrideRequestHeaders, _ := siteData["customHeadersOverrideRequestHeaders"].(bool)
	isPinned, _ := siteData["isPinned"].(bool)
	postRefreshProbeEnabled, _ := siteData["postRefreshProbeEnabled"].(bool)
	postRefreshProbeModel, _ := siteData["postRefreshProbeModel"].(string)
	postRefreshProbeScope, _ := siteData["postRefreshProbeScope"].(string)
	if postRefreshProbeScope == "" {
		postRefreshProbeScope = "single"
	}
	postRefreshProbeLatencyThresholdMs := int64(0)
	switch v := siteData["postRefreshProbeLatencyThresholdMs"].(type) {
	case int64:
		postRefreshProbeLatencyThresholdMs = v
	case int:
		postRefreshProbeLatencyThresholdMs = int64(v)
	case float64:
		postRefreshProbeLatencyThresholdMs = int64(v)
	}
	status, _ := siteData["status"].(string)
	if status == "" {
		status = "active"
	}
	globalWeight := 1.0
	switch v := siteData["globalWeight"].(type) {
	case float64:
		globalWeight = v
	case float32:
		globalWeight = float64(v)
	case int:
		globalWeight = float64(v)
	case int64:
		globalWeight = float64(v)
	}

	// Use RETURNING so PostgreSQL (no LastInsertId) and SQLite both get a real id
	// inside the open transaction before apiEndpoints FK inserts.
	//
	// resin_enabled / use_utls are nullable *bool columns: nil = inherit the
	// global RESIN_ENABLED / UTLS_ENABLED flag, true/false = per-site override.
	// They are passed through directly from siteData (same nullable pattern as
	// proxyUrl / customHeaders) so a missing key or JSON null stores NULL.
	var siteID int64
	err = tx.QueryRowx(
		tx.Rebind(`INSERT INTO sites (name, url, platform, proxy_url, use_system_proxy, custom_headers,
		 custom_headers_override_request_headers,
		 external_checkin_url, status, is_pinned, sort_order, global_weight, max_concurrency,
		 post_refresh_probe_enabled, post_refresh_probe_model, post_refresh_probe_scope,
		 post_refresh_probe_latency_threshold_ms, browser_ua, cf_clearance, resin_enabled, use_utls, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`),
		name, urlStr, platform,
		siteData["proxyUrl"], useSystemProxy, siteData["customHeaders"],
		customHeadersOverrideRequestHeaders,
		siteData["externalCheckinUrl"], status, isPinned,
		sortOrder, globalWeight, maxConcurrency,
		postRefreshProbeEnabled, postRefreshProbeModel,
		postRefreshProbeScope, postRefreshProbeLatencyThresholdMs,
		siteData["browserUa"], siteData["cfClearance"],
		siteData["resinEnabled"], siteData["useUtls"],
		now, now,
	).Scan(&siteID)
	if err != nil {
		return 0, err
	}
	if siteID <= 0 {
		return 0, fmt.Errorf("create site: invalid id %d", siteID)
	}

	// Insert apiEndpoints if present
	if endpoints, ok := siteData["apiEndpoints"].([]store.SiteAPIEndpoint); ok && len(endpoints) > 0 {
		for i := range endpoints {
			endpoints[i].SiteID = siteID
		}
		if err := UpsertSiteAPIEndpoints(tx, siteID, endpoints); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return siteID, nil
}

// UpdateSite updates a site and its apiEndpoints in a transaction.
func UpdateSite(db *sqlx.DB, siteID int64, updates map[string]any) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)

	// Build UPDATE SET clause dynamically
	setClauses := []string{"updated_at = ?"}
	args := []any{now}

	for key, val := range updates {
		colName := jsonKeyToColumn(key)
		if colName == "" {
			continue
		}
		setClauses = append(setClauses, colName+" = ?")
		args = append(args, val)
	}
	args = append(args, siteID)

	query := fmt.Sprintf("UPDATE sites SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	if _, err := tx.Exec(tx.Rebind(query), args...); err != nil {
		return err
	}

	// Handle apiEndpoints full-replace
	if endpoints, ok := updates["apiEndpoints"]; ok {
		if eps, ok := endpoints.([]store.SiteAPIEndpoint); ok {
			if err := UpsertSiteAPIEndpoints(tx, siteID, eps); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// DeleteSite deletes a site by ID (cascade via DB foreign keys).
func DeleteSite(db *sqlx.DB, siteID int64) error {
	_, err := db.Exec(db.Rebind("DELETE FROM sites WHERE id = ?"), siteID)
	return err
}

// siteProxyCacheInvalidators are optional hooks (e.g. admin accounts snapshot cache).
// Registered from other packages to avoid import cycles.
var (
	siteProxyCacheInvalidatorsMu sync.RWMutex
	siteProxyCacheInvalidators   []func()
)

// RegisterSiteProxyCacheInvalidator appends a hook invoked by InvalidateSiteProxyCache.
// Safe for concurrent registration; hooks should be idempotent and non-blocking.
func RegisterSiteProxyCacheInvalidator(fn func()) {
	if fn == nil {
		return
	}
	siteProxyCacheInvalidatorsMu.Lock()
	siteProxyCacheInvalidators = append(siteProxyCacheInvalidators, fn)
	siteProxyCacheInvalidatorsMu.Unlock()
}

// InvalidateSiteProxyCache refreshes process-local caches that depend on site/proxy config.
// Always invalidates the token-router route cache; then runs registered hooks
// (admin accounts snapshot, future proxy config caches).
func InvalidateSiteProxyCache() {
	routing.InvalidateCache()
	siteProxyCacheInvalidatorsMu.RLock()
	hooks := append([]func(){}, siteProxyCacheInvalidators...)
	siteProxyCacheInvalidatorsMu.RUnlock()
	for _, fn := range hooks {
		func() {
			defer func() {
				if r := recover(); r != nil {
					// never let a hook panic abort site mutations
					_ = r
				}
			}()
			fn()
		}()
	}
}

// InvalidateTokenRouterCache signals that cached token-router state should be invalidated.
// Implemented in route_rebuild.go (delegates to routing.InvalidateCache).
func InvalidateTokenRouterCache() {
	routing.InvalidateCache()
}

// InvalidateSiteCaches invalidates both site proxy and token router caches.
func InvalidateSiteCaches() {
	InvalidateSiteProxyCache()
	InvalidateTokenRouterCache()
}

// RebuildRoutesBestEffort is implemented in route_rebuild.go.

// ApplySiteStatusSideEffects handles status change side effects for sites.
func ApplySiteStatusSideEffects(db *sqlx.DB, siteID int64, siteName string, newStatus string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	if newStatus == "disabled" {
		// Disable all accounts under this site
		db.Exec(db.Rebind("UPDATE accounts SET status = 'disabled', updated_at = ? WHERE site_id = ?"), now, siteID)

		// Create event
		msg := fmt.Sprintf("%s disabled; all linked accounts disabled", siteName)
		db.Exec(
			db.Rebind(`INSERT INTO events (type, title, message, level, related_id, related_type, created_at)
			 VALUES ('status', 'Site disabled', ?, 'warning', ?, 'site', ?)`),
			msg, siteID, now,
		)
	} else {
		// Enable only previously-disabled accounts
		db.Exec(
			db.Rebind("UPDATE accounts SET status = 'active', updated_at = ? WHERE site_id = ? AND status = 'disabled'"),
			now, siteID,
		)

		msg := fmt.Sprintf("%s enabled; disabled linked accounts restored to active", siteName)
		db.Exec(
			db.Rebind(`INSERT INTO events (type, title, message, level, related_id, related_type, created_at)
			 VALUES ('status', 'Site enabled', ?, 'info', ?, 'site', ?)`),
			msg, siteID, now,
		)
	}
	return nil
}

// jsonKeyToColumn maps JSON field names to DB column names.
func jsonKeyToColumn(key string) string {
	mapping := map[string]string{
		"name":                                "name",
		"url":                                 "url",
		"platform":                            "platform",
		"proxyUrl":                            "proxy_url",
		"useSystemProxy":                      "use_system_proxy",
		"customHeaders":                       "custom_headers",
		"customHeadersOverrideRequestHeaders": "custom_headers_override_request_headers",
		"externalCheckinUrl":                  "external_checkin_url",
		"status":                              "status",
		"isPinned":                            "is_pinned",
		"sortOrder":                           "sort_order",
		"globalWeight":                        "global_weight",
		"apiKey":                              "api_key",
		"maxConcurrency":                      "max_concurrency",
		"postRefreshProbeEnabled":             "post_refresh_probe_enabled",
		"postRefreshProbeModel":               "post_refresh_probe_model",
		"postRefreshProbeScope":               "post_refresh_probe_scope",
		"postRefreshProbeLatencyThresholdMs":  "post_refresh_probe_latency_threshold_ms",
		"browserUa":                          "browser_ua",
		"cfClearance":                        "cf_clearance",
		"resinEnabled":                       "resin_enabled",
		"useUtls":                            "use_utls",
	}
	return mapping[key]
}

// ---- JSON helpers ----

// ParseExtraConfig parses an extraConfig field (JSON string) as a map.
func ParseExtraConfig(raw *string) map[string]any {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(*raw), &result); err != nil {
		return nil
	}
	return result
}

// MarshalExtraConfig marshals a map to a JSON string.
func MarshalExtraConfig(config map[string]any) *string {
	if len(config) == 0 {
		return nil
	}
	b, err := json.Marshal(config)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}
