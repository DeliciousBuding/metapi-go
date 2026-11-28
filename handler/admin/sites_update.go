package admin

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Update Site ----

func (h *sitesHandler) updateSite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var existing store.Site
	err := h.db.Get(&existing, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", id)
	if err == sql.ErrNoRows {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "Site not found")
		return
	} else if err != nil {
		slog.Error("Failed to load site for update", "err", err, "site_id", id)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load site")
		return
	}

	var body payloads.SiteUpdatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid site payload: "+err.Error())
		return
	}

	updates := map[string]any{}

	// Validate each field
	if body.Name != nil {
		if strings.TrimSpace(*body.Name) == "" {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid name. Expected non-empty string.")
			return
		}
		updates["name"] = *body.Name
	}
	if body.URL != nil {
		if strings.TrimSpace(*body.URL) == "" {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected non-empty string.")
			return
		}
		if service.IsForbiddenSiteTargetURL(*body.URL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Cloud metadata / link-local targets are not allowed.")
			return
		}
		if !service.IsValidHTTPURL(*body.URL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected a valid http(s) URL.")
			return
		}
		updates["url"] = service.CanonicalizeSiteURL(*body.URL)
	}
	if body.Platform != nil {
		p := strings.TrimSpace(strings.ToLower(*body.Platform))
		if p == "" {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid platform. Expected non-empty string.")
			return
		}
		updates["platform"] = p
	}
	if body.ProxyURL != nil {
		normalizedProxyURL, valid := normalizeProxyURLInput(body.ProxyURL)
		if !valid {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		if requireProxyURLReauth(w, r, normalizedProxyURL) {
			return
		}
		updates["proxyUrl"] = service.NormalizeNullable(&normalizedProxyURL)
	}
	if body.UseSystemProxy != nil {
		updates["useSystemProxy"] = *body.UseSystemProxy
	}
	if body.CustomHeaders != nil {
		updates["customHeaders"] = *body.CustomHeaders
	}
	if body.CustomHeadersOverrideRequestHeaders != nil {
		updates["customHeadersOverrideRequestHeaders"] = *body.CustomHeadersOverrideRequestHeaders
	}
	if body.ExternalCheckinURL != nil {
		if *body.ExternalCheckinURL != "" && !service.IsValidHTTPURL(*body.ExternalCheckinURL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid externalCheckinUrl. Expected a valid http(s) URL.")
			return
		}
		updates["externalCheckinUrl"] = service.NormalizeNullable(body.ExternalCheckinURL)
	}
	if body.Status != nil {
		ns := normalizeSiteStatusString(body.Status)
		if ns == "" {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid site status. Expected active or disabled.")
			return
		}
		updates["status"] = ns
	}
	if body.IsPinned != nil {
		updates["isPinned"] = *body.IsPinned
	}
	if body.SortOrder != nil {
		so := service.NormalizeSortOrder(body.SortOrder)
		if so == nil {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid sortOrder value. Expected non-negative integer.")
			return
		}
		updates["sortOrder"] = int64(*so)
	}
	if body.GlobalWeight != nil {
		gw := service.NormalizeGlobalWeight(body.GlobalWeight)
		if gw == nil {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid globalWeight value. Expected a positive number.")
			return
		}
		updates["globalWeight"] = *gw
	}
	if body.MaxConcurrency != nil {
		if *body.MaxConcurrency < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid maxConcurrency value. Expected non-negative integer (0 = unlimited)."})
			return
		}
		updates["maxConcurrency"] = *body.MaxConcurrency
	}
	if body.PostRefreshProbeEnabled != nil {
		updates["postRefreshProbeEnabled"] = *body.PostRefreshProbeEnabled
	}
	if body.PostRefreshProbeModel != nil {
		updates["postRefreshProbeModel"] = strings.TrimSpace(*body.PostRefreshProbeModel)
	}
	if body.PostRefreshProbeScope != nil {
		scope := strings.TrimSpace(strings.ToLower(*body.PostRefreshProbeScope))
		if scope != "all" {
			scope = "single"
		}
		updates["postRefreshProbeScope"] = scope
	}
	if body.PostRefreshProbeLatencyThresholdMs != nil {
		ms := int64(*body.PostRefreshProbeLatencyThresholdMs)
		if ms < 0 {
			ms = 0
		}
		updates["postRefreshProbeLatencyThresholdMs"] = ms
	}
	// Per-site nullable overrides (resin / uTLS): true/false stores the
	// override, null clears it (inherit global flag), absent leaves it alone.
	if body.ResinEnabled.Present {
		if body.ResinEnabled.Null {
			updates["resinEnabled"] = nil
		} else {
			updates["resinEnabled"] = body.ResinEnabled.Value
		}
	}
	if body.UseUTLS.Present {
		if body.UseUTLS.Null {
			updates["useUtls"] = nil
		} else {
			updates["useUtls"] = body.UseUTLS.Value
		}
	}

	// Handle API endpoints
	if body.APIEndpoints != nil {
		eps, errMsg := normalizeAPIEndpointsInput(body.APIEndpoints)
		if errMsg != "" {
			writeErrorWithRequest(w, r, http.StatusBadRequest, errMsg)
			return
		}
		now := time.Now().UTC().Format(time.RFC3339)
		updates["apiEndpoints"] = siteEndpointRecords(eps, now)
	}

	// Check for unique conflict on (platform, url) change
	nextURL := existing.URL
	if v, ok := updates["url"]; ok {
		nextURL = v.(string)
	}
	nextPlatform := existing.Platform
	if v, ok := updates["platform"]; ok {
		nextPlatform = v.(string)
	}
	if nextURL != existing.URL || nextPlatform != existing.Platform {
		var conflictCount int
		h.db.Get(&conflictCount, "SELECT COUNT(*) FROM sites WHERE platform = ? AND url = ? AND id != ?", nextPlatform, nextURL, id)
		if conflictCount > 0 {
			writeErrorWithRequest(w, r, http.StatusConflict, fmt.Sprintf("A %s site with URL %s already exists.", nextPlatform, nextURL))
			return
		}
	}

	if err := service.UpdateSite(h.db.DB, id, updates); err != nil {
		if isUniqueConstraintError(err) {
			writeErrorWithRequest(w, r, http.StatusConflict, fmt.Sprintf("A %s site with URL %s already exists.", nextPlatform, nextURL))
			return
		}
		slog.Error("Failed to update site", "err", err, "site_id", id)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to update site")
		return
	}

	// Apply status side effects
	if newStatus, ok := updates["status"].(string); ok {
		service.ApplySiteStatusSideEffects(h.db.DB, id, existing.Name, newStatus)
		service.InvalidateSiteCaches()
	}

	result, _ := service.LoadSiteWithEndpoints(h.db.DB, id)
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, result)
}
