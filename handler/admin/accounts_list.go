package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- List Accounts ----

func (h *accountsHandler) listAccounts(w http.ResponseWriter, r *http.Request) {
	refresh := strings.TrimSpace(r.URL.Query().Get("refresh"))
	forceRefresh := refresh == "true" || refresh == "1"

	// Defensive pagination (#719/#711 parity). When ?page is absent the handler
	// returns the full 30s-TTL cached snapshot exactly as before (backward
	// compat for the frontend that does not paginate). When ?page is supplied,
	// bypass the snapshot cache and run a bounded LIMIT/OFFSET query so the
	// response size stays capped as the account fleet grows.
	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	if pageStr != "" {
		h.listAccountsPaginated(w, r)
		return
	}

	// Snapshot cache: a hit short-circuits; on a miss the single-flight group
	// deduplicates concurrent computes so N admin sessions polling an expired
	// snapshot share one ListAccountsWithSites + per-account metrics run
	// instead of running it N× (thundering herd). ?refresh=true clears the
	// cache first so a force request always recomputes and repopulates.
	if forceRefresh {
		globalAccountsCache.clear()
	}
	data, cacheHit, err := globalAccountsCache.getOrCompute(func() ([]byte, error) {
		return h.computeAccountsSnapshot()
	})
	if err != nil {
		slog.Error("Failed to load accounts", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load accounts", "errorCode": "resourceLoadFailed"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if cacheHit {
		w.Header().Set("x-accounts-snapshot-cache", "hit")
	} else {
		w.Header().Set("x-accounts-snapshot-cache", "miss")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// computeAccountsSnapshot builds the full accounts+sites snapshot payload (the
// cache-miss path). Extracted from listAccounts so the single-flight group can
// deduplicate concurrent misses. Returns the marshaled JSON bytes; the caller
// (getOrCompute) stores them in the cache. Today-metrics failure degrades to
// "no metrics" and is logged — the account list itself must not fail because
// an auxiliary aggregation query broke.
func (h *accountsHandler) computeAccountsSnapshot() ([]byte, error) {
	accounts, err := service.ListAccountsWithSites(h.db)
	if err != nil {
		return nil, err
	}

	// Per-account today truth. Failure degrades to "no metrics" (frontend shows
	// — instead of fake zeros) and is logged; the account list itself must not
	// fail because an auxiliary aggregation query broke.
	todayMetrics, metricsErr := dailyservice.CollectPerAccountTodayMetrics(h.db, time.Now())
	if metricsErr != nil {
		slog.Error("Failed to load per-account today metrics", "err", metricsErr)
	} else {
		applyAccountTodayMetrics(accounts, todayMetrics)
	}

	// Also fetch sites for the response
	var sites []store.Site
	h.db.Select(&sites, "SELECT "+service.SiteSelectColumns+" FROM sites ORDER BY sort_order, id")

	resp := map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"accounts":    normalizeSlice(accounts),
		"sites":       sitesOrEmpty(sites),
	}
	return json.Marshal(resp)
}

// sitesOrEmpty keeps the snapshot contract array-shaped on an empty database:
// a nil []store.Site marshals to JSON null, which trips the admin frontend's
// Array.isArray guards (useAccounts) into treating an empty fleet as a broken
// payload. normalizeSlice covers the []map[string]any accounts slice; sites is
// a typed slice so it gets its own one-liner instead of widening the helper.
func sitesOrEmpty(sites []store.Site) []store.Site {
	if sites == nil {
		return []store.Site{}
	}
	return sites
}

// listAccountsPaginated serves GET /api/accounts?page=&pageSize=&status=&site=
// with a bounded LIMIT/OFFSET query. The snapshot cache is intentionally
// bypassed: a paged request is an explicit opt-out of the snapshot, and
// caching every page combo would multiply the cache surface. `status` and
// `site` are optional comma-separated server-side filters (issue #1108):
// they filter the whole fleet, so rows on other pages are matched — unlike
// the previous client-side filters over the returned page only. Today
// metrics are still enriched for the accounts on the current page. Response
// shape mirrors /api/channels: {items, total, page, pageSize, generatedAt, sites}.
func (h *accountsHandler) listAccountsPaginated(w http.ResponseWriter, r *http.Request) {
	page := config.ClampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
	pageSize := config.ClampInt(getQueryInt(r, "pageSize", 50), 1, 200)
	offset := (page - 1) * pageSize

	filter, err := parseAccountListFilter(r)
	if err != nil {
		// Invalid filter params only come from hand-edited URLs — no client
		// branches on this rejection, so it stays a plain (unregistered) 400.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	accounts, total, err := service.ListAccountsWithSitesPaginated(h.db, pageSize, offset, filter)
	if err != nil {
		slog.Error("Failed to load paginated accounts", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load accounts", "errorCode": "resourceLoadFailed"})
		return
	}

	todayMetrics, metricsErr := dailyservice.CollectPerAccountTodayMetrics(h.db, time.Now())
	if metricsErr != nil {
		slog.Error("Failed to load per-account today metrics", "err", metricsErr)
	} else {
		applyAccountTodayMetrics(accounts, todayMetrics)
	}

	var sites []store.Site
	h.db.Select(&sites, "SELECT "+service.SiteSelectColumns+" FROM sites ORDER BY sort_order, id")

	writeJSON(w, http.StatusOK, map[string]any{
		"items":       normalizeSlice(accounts),
		"total":       total,
		"page":        page,
		"pageSize":    pageSize,
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"sites":       sitesOrEmpty(sites),
	})
}

// applyAccountTodayMetrics merges per-account today aggregates onto each
// account map. Accounts with no metrics for the local day get explicit zeros
// with a "complete" status so the UI shows real zeros rather than placeholders.
// Extracted from listAccounts so both the cached and paginated paths share it.
func applyAccountTodayMetrics(accounts []map[string]any, todayMetrics map[int64]*dailyservice.AccountTodayMetrics) {
	for _, account := range accounts {
		accountID := coerceInt64(account["id"])
		if accountID <= 0 {
			continue
		}
		if m, exists := todayMetrics[accountID]; exists {
			account["todayReward"] = m.Reward
			account["todayRewardStatus"] = m.RewardStatus
			account["todayRewardReason"] = m.RewardReason
			account["todaySpend"] = m.Spend
			account["todaySpendStatus"] = m.SpendStatus
			account["todaySpendReason"] = m.SpendReason
			account["todayTokens"] = m.Tokens
			account["todayProxy"] = map[string]any{
				"total":   m.ProxyTotal,
				"success": m.ProxySuccess,
				"failed":  m.ProxyFailed,
				"unknown": m.ProxyUnknown,
			}
		} else {
			// Real zero, not missing: account had no rows within the local day.
			account["todayReward"] = 0.0
			account["todayRewardStatus"] = "complete"
			account["todaySpend"] = 0.0
			account["todaySpendStatus"] = "complete"
			account["todayTokens"] = int64(0)
			account["todayProxy"] = map[string]any{
				"total":   0,
				"success": 0,
				"failed":  0,
				"unknown": 0,
			}
		}
	}
}

// ---- Account Models ----

// validAccountStatuses whitelists the `status` filter values accepted by the
// paginated accounts list (the same domain the UI filter exposes: active /
// disabled / expired).
var validAccountStatuses = map[string]struct{}{
	"active":   {},
	"disabled": {},
	"expired":  {},
}

// parseAccountListFilter reads the optional `q`, comma-separated `status` and
// comma-separated `site` query params into a service.AccountListFilter.
// Invalid status/site values are rejected explicitly (mirroring
// parseChannelStatusFilter) instead of being silently dropped.
func parseAccountListFilter(r *http.Request) (service.AccountListFilter, error) {
	var filter service.AccountListFilter

	filter.Query = strings.TrimSpace(r.URL.Query().Get("q"))

	rawStatus := strings.TrimSpace(r.URL.Query().Get("status"))
	if rawStatus != "" {
		seen := make(map[string]struct{}, 4)
		for _, part := range strings.Split(rawStatus, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return filter, fmt.Errorf("invalid account status filter")
			}
			if _, ok := validAccountStatuses[part]; !ok {
				return filter, fmt.Errorf("invalid account status filter: %q", part)
			}
			if _, ok := seen[part]; ok {
				continue
			}
			seen[part] = struct{}{}
			filter.Statuses = append(filter.Statuses, part)
		}
	}

	rawSite := strings.TrimSpace(r.URL.Query().Get("site"))
	if rawSite != "" {
		seen := make(map[int64]struct{}, 4)
		for _, part := range strings.Split(rawSite, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				return filter, fmt.Errorf("invalid account site filter")
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return filter, fmt.Errorf("invalid account site filter: %q", part)
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			filter.SiteIDs = append(filter.SiteIDs, id)
		}
	}

	return filter, nil
}
