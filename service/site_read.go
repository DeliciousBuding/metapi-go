package service

import (
	"database/sql"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

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
	siteMap := map[string]any{
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
	RedactSiteCredentials(siteMap)
	return siteMap
}

// RedactSiteCredentials applies the admin site credential policy to a site map:
// apiKey becomes a masked projection plus hasApiKey, and cfClearance (a live
// Cloudflare clearance cookie) is removed. Both GET /api/sites and search rows
// (scanned as maps) answer through this helper, so the two surfaces cannot
// drift.
func RedactSiteCredentials(site map[string]any) {
	if site == nil {
		return
	}
	apiKey := secretValue(site["apiKey"])
	delete(site, "apiKey")
	delete(site, "cfClearance")
	site["hasApiKey"] = apiKey != ""
	if apiKey != "" {
		site["apiKeyMasked"] = maskSecretTail(apiKey)
	}
}

// SitePublicMap returns the admin-safe site projection without loading
// sub-resources. Account list responses also embed sites and must answer with
// the same credential policy as GET /api/sites instead of marshaling
// store.Site, whose JSON tags would expose apiKey and cfClearance.
func SitePublicMap(site store.Site) map[string]any {
	return siteToMap(site, []store.SiteAPIEndpoint{})
}

// accountAgg is used for aggregating account balance/subscription data.
type accountAgg struct {
	SiteID      int64           `db:"site_id"`
	Balance     sql.NullFloat64 `db:"balance"`
	ExtraConfig *string         `db:"extra_config"`
}

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
	// Balance is scanned as sql.NullFloat64: accounts migrated from the TS
	// version (or never refreshed) may carry a NULL balance, which sqlx cannot
	// scan into float64 and would 500 the whole /api/sites response. A NULL
	// balance means "unknown" — it must NOT be aggregated as zero, or the list
	// would show $0.00 for a site whose balance is simply not known.
	if err := db.Select(&accounts, "SELECT site_id, balance, extra_config FROM accounts"); err != nil {
		return nil, err
	}

	balanceBySite := make(map[int64]float64)
	knownBalanceBySite := make(map[int64]bool)
	accountCountBySite := make(map[int64]int)
	summariesBySite := buildSubscriptionSummariesBySite(accounts)
	for _, a := range accounts {
		accountCountBySite[a.SiteID]++
		if a.Balance.Valid {
			balanceBySite[a.SiteID] += a.Balance.Float64
			knownBalanceBySite[a.SiteID] = true
		}
	}

	result := make([]map[string]any, len(sites))
	for i, s := range sites {
		siteMap := siteToMap(s, endpointsBySite[s.ID])
		// Unknown-balance policy: a site with accounts but no known balance
		// reports nil (the frontend renders '—'); a site with no accounts at
		// all reports 0 ($0.00 — there is genuinely nothing to aggregate); a
		// mix reports the sum of the known balances.
		var totalBalance any
		if knownBalanceBySite[s.ID] || accountCountBySite[s.ID] == 0 {
			totalBalance = math.Round(balanceBySite[s.ID]*1_000_000) / 1_000_000
		}
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

		// tokenExpiresAt is normalized to epoch milliseconds.
		if exp, ok := NormalizeManagedTokenExpiresAt(auth["tokenExpiresAt"]); ok && exp > 0 {
			expiry := time.UnixMilli(exp).UTC()
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
