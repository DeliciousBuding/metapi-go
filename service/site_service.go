package service

import (
	"fmt"
	"math"
	"strings"
	"time"

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
		"browserUa":                           "browser_ua",
		"cfClearance":                         "cf_clearance",
		"resinEnabled":                        "resin_enabled",
		"useUtls":                             "use_utls",
	}
	return mapping[key]
}
