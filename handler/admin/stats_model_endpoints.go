package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/go-chi/chi/v5"
)

func (h *statsHandler) marketplace(w http.ResponseWriter, r *http.Request) {
	refreshRequested := parseTruthyQuery(r.URL.Query().Get("refresh"))
	includePricing := parseTruthyQuery(r.URL.Query().Get("includePricing"))

	query := parseMarketplaceQuery(r)

	aggregation, err := h.buildMarketplaceAggregation()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build marketplace models")
		return
	}

	catalogSnapshot := appCatalogSnapshot()

	// Query contract: search, filters, sort, totals, and facets run on the
	// lightweight index projection of the whole marketplace, so a page-1
	// search also finds page-2 models and every count shares one filter
	// context. Only the selected page is hydrated into the full row shape.
	indexRows := buildMarketplaceIndex(aggregation, catalogSnapshot)
	filtered := filterMarketplaceIndex(indexRows, query)
	sortMarketplaceIndex(filtered, query)

	// Pagination keeps the same page-gated envelope as /api/accounts,
	// /api/channels and /api/routes. When ?page is absent the response keeps
	// the legacy {models, meta} surface; filters and sort still apply.
	pageStr := strings.TrimSpace(r.URL.Query().Get("page"))
	total := len(filtered)
	var pageRows []marketplaceIndexRow
	page, pageSize := 1, 50
	if pageStr != "" {
		page = config.ClampInt(getQueryInt(r, "page", 1), 1, 1_000_000)
		pageSize = config.ClampInt(getQueryInt(r, "pageSize", 50), 1, 200)
		offset := (page - 1) * pageSize
		if offset >= total {
			pageRows = []marketplaceIndexRow{}
		} else {
			end := offset + pageSize
			if end > total {
				end = total
			}
			pageRows = filtered[offset:end]
		}
	} else {
		pageRows = filtered
	}
	respModels := make([]map[string]any, 0, len(pageRows))
	for _, row := range pageRows {
		respModels = append(respModels, hydrateMarketplaceRow(aggregation, row.name, catalogSnapshot))
	}
	meta := map[string]any{
		"refreshRequested": refreshRequested,
		// No background pricing/catalog job in this surface — DB-derived only.
		"refreshQueued":  false,
		"refreshReused":  false,
		"refreshRunning": false,
		"refreshJobId":   nil,
		"includePricing": includePricing,
		"source":         "db_availability",
		// Echo the normalized query context so a shared link never needs its
		// raw parameters re-parsed to be understood.
		"query": query.metaQueryEcho(),
	}
	// Catalog observability: which sources built the merged snapshot, when,
	// and how many entries it carries.
	if catalogMeta, ok := catalogSnapshotMeta(); ok {
		meta["catalogSource"] = catalogMeta.Source
		meta["catalogFetchedAt"] = catalogMeta.FetchedAt
		meta["catalogModels"] = catalogMeta.Models
	} else {
		meta["catalogSource"] = "disabled"
		meta["catalogFetchedAt"] = nil
		meta["catalogModels"] = 0
	}
	if includePricing {
		if _, ok := catalogSnapshotMeta(); ok {
			// Catalog list prices hydrate pricingSources, but marketplace
			// sites are third-party relays — the official list price is an
			// estimate, never a real payment price.
			meta["pricingStatus"] = "catalog_estimate"
			meta["pricingNote"] = "pricingSources carry the official catalog list price as an estimate for relay sites; use /api/models/price-compare for effective observed rates."
		} else {
			meta["pricingStatus"] = "unavailable"
			meta["pricingNote"] = "Remote marketplace pricing catalog is not hydrated; use /api/models/price-compare for effective rates. pricingSources intentionally empty."
		}
	}
	if refreshRequested {
		meta["refreshNote"] = "refresh=true acknowledged but no remote marketplace scrape is performed; response is rebuilt from local model_availability / token_model_availability / token_routes."
	}

	if pageStr != "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":      normalizeSlice(respModels),
			"total":      total,
			"page":       page,
			"pageSize":   pageSize,
			"facets":     buildMarketplaceFacets(indexRows, query),
			"facetScope": marketplaceFacetScope,
			"meta":       meta,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"models": respModels,
		"meta":   meta,
	})
}

// marketplaceDetail serves one fully hydrated row by exact model name,
// independent of pagination: a ?model=<name> deep link resolves even when the
// model is not on the page the browser has loaded. 404 means the name has no
// supply and no exact route in the current aggregation.
func (h *statsHandler) marketplaceDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	aggregation, err := h.buildMarketplaceAggregation()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build marketplace models")
		return
	}
	if _, ok := aggregation.byModel[name]; !ok {
		writeError(w, http.StatusNotFound, "model not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model": hydrateMarketplaceRow(aggregation, name, appCatalogSnapshot()),
	})
}

// catalogSnapshotMeta reports the merged catalog snapshot's provenance for
// the marketplace meta block. ok=false when the catalog is disabled.
func catalogSnapshotMeta() (meta struct {
	Source    string
	FetchedAt *time.Time
	Models    int
}, ok bool) {
	manager := app.CatalogManager()
	if manager == nil {
		return meta, false
	}
	snapshot := manager.Snapshot()
	if snapshot == nil {
		return meta, false
	}
	meta.Source = snapshot.Source
	meta.Models = snapshot.Len()
	if !snapshot.FetchedAt.IsZero() {
		t := snapshot.FetchedAt
		meta.FetchedAt = &t
	}
	return meta, true
}

// ---- Token Candidates ----
// GET /api/models/token-candidates

// Structured maps for route configuration:
// - models: token-scoped availability (token_model_availability)
// - modelsWithoutToken: account-level availability with no matching token coverage
// - modelsMissingTokenGroups: accounts whose tokens lack group labels when groups are required
// - endpointTypesByModel: inferred endpoint types from site platforms
func (h *statsHandler) tokenCandidates(w http.ResponseWriter, r *http.Request) {
	allowed := h.loadGlobalAllowedModels()
	models, err := h.buildTokenCandidateModels(allowed)
	if err != nil {
		slog.Error("token-candidates: buildTokenCandidateModels failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load token candidates")
		return
	}
	withoutToken, err := h.buildModelsWithoutToken(allowed)
	if err != nil {
		slog.Error("token-candidates: buildModelsWithoutToken failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load models without token")
		return
	}
	missingGroups, err := h.buildModelsMissingTokenGroups(allowed)
	if err != nil {
		slog.Error("token-candidates: buildModelsMissingTokenGroups failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load missing token groups")
		return
	}
	endpointTypes, err := h.buildEndpointTypesByModel(allowed)
	if err != nil {
		slog.Error("token-candidates: buildEndpointTypesByModel failed", "error", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load endpoint types")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"models":                   models,
		"modelsWithoutToken":       withoutToken,
		"modelsMissingTokenGroups": missingGroups,
		"endpointTypesByModel":     endpointTypes,
	})
}

// ---- Model Check ----
// POST /api/models/check/:accountId

// Real availability refresh for one account via platform.GetModels, then
// best-effort route rebuild. Never returns fake success when refresh fails.
func (h *statsHandler) modelCheck(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": false,
			"error":   "Invalid account id",
		})
		return
	}

	result := accountModelRefresher(r.Context(), h.db.DB, id, true)
	writeJSON(w, http.StatusOK, result)
}

// ---- Model Probe ----
// POST /api/models/probe
func (h *statsHandler) modelProbe(w http.ResponseWriter, r *http.Request) {
	sched := scheduler.GetGlobalModelProbeScheduler()
	if sched == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, ErrorCodeResourceDisabled, "model probe scheduler is not running (enable MODEL_AVAILABILITY_PROBE_ENABLED or start schedulers)")
		return
	}
	jobID := fmt.Sprintf("probe-%d", time.Now().UTC().UnixNano())
	go func() {
		sched.TriggerNow(true)
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  false,
		"jobId":   jobID,
		"status":  "pending",
		"message": "model availability probe started; check the task list or LastRunSummary later",
	})
}
