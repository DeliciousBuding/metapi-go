package admin

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterMonitorHealthRoute mounts the read-only observability health
// projection. It never hard-disables anything — the response only aggregates
// the in-memory runtime breaker state and the DB-backed channel cooldown /
// site+account health counters.
func RegisterMonitorHealthRoute(r chi.Router, db *store.DB) {
	h := &monitorHealthHandler{db: db}
	r.Get("/api/monitor/health", h.health)
}

type monitorHealthHandler struct {
	db *store.DB
}

// health is the read-only projection endpoint backing the observability
// Health section (site/account health + breaker/cooldown aggregation).
func (h *monitorHealthHandler) health(w http.ResponseWriter, r *http.Request) {
	runtimeHealth, err := h.namedRuntimeHealth(routing.SnapshotRuntimeHealth())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load breaker site names")
		return
	}

	cooldown, err := h.aggregateCooldown()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to aggregate cooldown")
		return
	}
	sites, err := h.db.SiteStatusCounts()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load status counts")
		return
	}
	accounts, err := h.db.AccountStatusCounts()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load status counts")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"runtimeHealth": runtimeHealth,
		"cooldown":      cooldown,
		"sites":         sites,
		"accounts":      accounts,
	})
}

type namedBreaker struct {
	routing.RuntimeHealthBreaker
	SiteName string `json:"siteName"`
}

type namedRuntimeHealth struct {
	routing.RuntimeHealthSnapshot
	OpenBreakers []namedBreaker `json:"openBreakers"`
}

func (h *monitorHealthHandler) namedRuntimeHealth(snapshot routing.RuntimeHealthSnapshot) (namedRuntimeHealth, error) {
	result := namedRuntimeHealth{RuntimeHealthSnapshot: snapshot, OpenBreakers: make([]namedBreaker, 0, len(snapshot.OpenBreakers))}
	if len(snapshot.OpenBreakers) == 0 {
		return result, nil
	}
	seen := make(map[int64]bool)
	var ids []int64
	for _, breaker := range snapshot.OpenBreakers {
		if !seen[breaker.SiteID] {
			ids = append(ids, breaker.SiteID)
			seen[breaker.SiteID] = true
		}
	}
	names, err := h.db.SiteNames(ids)
	if err != nil {
		return result, err
	}
	for _, breaker := range snapshot.OpenBreakers {
		result.OpenBreakers = append(result.OpenBreakers, namedBreaker{breaker, names[breaker.SiteID]})
	}
	return result, nil
}

// aggregateCooldown projects route_channels cooldown/failure state without
// writing anything. Cooling means cooldownUntil is still in the future;
// recently-failed means failCount>0 with a lastFailAt inside the configured
// Fibonacci backoff window.
func (h *monitorHealthHandler) aggregateCooldown() (map[string]any, error) {
	// Cooldown window cap is runtime-mutable (settings hot-update); read one
	// snapshot for the whole aggregation so every row uses the same value.
	configuredMaxSec := config.Runtime().TokenRouterFailureCooldownMaxSec
	rows, err := h.db.CooldownChannels()
	if err != nil {
		return nil, err
	}

	nowISO := time.Now().UTC().Format(time.RFC3339)
	nowMs := time.Now().UnixMilli()

	channelsCooling := 0
	channelsWithFailures := 0
	channelsRecentlyFailed := 0
	cooling := make([]map[string]any, 0)

	for _, row := range rows {
		failCount := row.FailCount
		cooldownUntil := row.CooldownUntil
		lastFailAt := row.LastFailAt

		if failCount > 0 {
			channelsWithFailures++
		}
		if routing.IsChannelRecentlyFailed(&failCount, lastFailAt, nowMs, configuredMaxSec) {
			channelsRecentlyFailed++
		}
		if routing.IsCooldownActive(cooldownUntil, nowISO) {
			channelsCooling++
			cooling = append(cooling, map[string]any{
				"cooldownReasonCode": row.ReasonCode,
				"cooldownReason":     row.Reason,
				"channelId":          row.ID,
				"accountId":          row.AccountID,
				"siteId":             row.SiteID,
				"siteName":           row.SiteName,
				"failCount":          failCount,
				"cooldownUntil":      *cooldownUntil,
			})
		}
	}

	return map[string]any{
		"channelsCooling":        channelsCooling,
		"channelsWithFailures":   channelsWithFailures,
		"channelsRecentlyFailed": channelsRecentlyFailed,
		"cooling":                cooling,
	}, nil
}
