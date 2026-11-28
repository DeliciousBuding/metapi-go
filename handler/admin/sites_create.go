package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// ---- Create Site ----

func (h *sitesHandler) createSite(w http.ResponseWriter, r *http.Request) {
	var body payloads.SiteCreatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid site payload: "+err.Error())
		return
	}

	// Validate name and url
	if strings.TrimSpace(body.Name) == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid name. Expected non-empty string.")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected non-empty string.")
		return
	}
	if service.IsForbiddenSiteTargetURL(body.URL) {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Cloud metadata / link-local targets are not allowed.")
		return
	}
	if !service.IsValidHTTPURL(body.URL) {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected a valid http(s) URL.")
		return
	}

	// Normalize values
	normalizedStatus := normalizeSiteStatusString(body.Status)
	if body.Status != nil && normalizedStatus == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid site status. Expected active or disabled.")
		return
	}
	if body.UseSystemProxy != nil {
		if !boolPtrValid(body.UseSystemProxy) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid useSystemProxy value. Expected boolean.")
			return
		}
	}
	if body.ProxyURL != nil {
		normalizedProxyURL, valid := normalizeProxyURLInput(body.ProxyURL)
		if !valid {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		// A per-site proxy silently redirects stored upstream credentials, so
		// setting one requires the master token; clearing it does not.
		if requireProxyURLReauth(w, r, normalizedProxyURL) {
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}
	if body.ExternalCheckinURL != nil && *body.ExternalCheckinURL != "" && !service.IsValidHTTPURL(*body.ExternalCheckinURL) {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid externalCheckinUrl. Expected a valid http(s) URL.")
		return
	}
	normalizedPinned := body.IsPinned
	if body.IsPinned != nil && !boolPtrValid(body.IsPinned) {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid isPinned value. Expected boolean.")
		return
	}
	normalizedSortOrder := service.NormalizeSortOrder(body.SortOrder)
	if body.SortOrder != nil && normalizedSortOrder == nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid sortOrder value. Expected non-negative integer.")
		return
	}
	normalizedWeight := service.NormalizeGlobalWeight(body.GlobalWeight)
	if body.GlobalWeight != nil && normalizedWeight == nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid globalWeight value. Expected a positive number.")
		return
	}
	if body.MaxConcurrency != nil && *body.MaxConcurrency < 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid maxConcurrency value. Expected non-negative integer (0 = unlimited)."})
		return
	}
	if body.CustomHeaders != nil && *body.CustomHeaders != "" {
		if !json.Valid([]byte(*body.CustomHeaders)) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid customHeaders.")
			return
		}
	}

	// Normalize API endpoints
	eps, err := normalizeAPIEndpointsInput(body.APIEndpoints)
	if err != "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, err)
		return
	}

	// Canonicalize URL and detect platform
	canonicalURL := service.CanonicalizeSiteURL(body.URL)
	platform := ""
	if body.Platform != nil {
		platform = strings.TrimSpace(strings.ToLower(*body.Platform))
	}
	if platform == "" {
		detected := service.DetectSite(canonicalURL)
		if detected != nil {
			platform = detected.Platform
		}
	}
	if platform == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Could not detect platform. Please specify manually.")
		return
	}

	// Validate initializationPresetId against registry (platform + URL match rules).
	var initializationPresetID string
	if body.InitializationPresetID != nil {
		initializationPresetID = strings.TrimSpace(*body.InitializationPresetID)
	}
	if initializationPresetID != "" {
		if err := service.ValidateSiteInitializationPreset(initializationPresetID, platform, body.URL); err != nil {
			writeErrorWithRequest(w, r, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		// Auto-detect preset when client omitted it but URL is a known vendor entry.
		if preset := service.DetectSiteInitializationPreset(body.URL, platform); preset != nil {
			initializationPresetID = preset.ID
		} else if preset := service.DetectSiteInitializationPreset(canonicalURL, platform); preset != nil {
			initializationPresetID = preset.ID
		}
	}

	// Check for duplicate (platform, url)
	var existingCount int
	h.db.Get(&existingCount, "SELECT COUNT(*) FROM sites WHERE platform = ? AND url = ?", platform, canonicalURL)
	if existingCount > 0 {
		writeErrorWithRequest(w, r, http.StatusConflict, fmt.Sprintf("A %s site with URL %s already exists.", platform, canonicalURL))
		return
	}

	// Build siteData map
	now := time.Now().UTC().Format(time.RFC3339)
	siteData := map[string]any{
		"name":                               body.Name,
		"url":                                canonicalURL,
		"platform":                           platform,
		"status":                             coalesce(normalizedStatus, "active"),
		"isPinned":                           coalescePtr(normalizedPinned, false),
		"globalWeight":                       coalescePtr(normalizedWeight, 1.0),
		"maxConcurrency":                     coalescePtr(body.MaxConcurrency, 0),
		"postRefreshProbeEnabled":            false,
		"postRefreshProbeModel":              "",
		"postRefreshProbeScope":              "single",
		"postRefreshProbeLatencyThresholdMs": 0,
	}

	if body.ProxyURL != nil {
		siteData["proxyUrl"] = service.NormalizeNullable(body.ProxyURL)
	} else {
		siteData["proxyUrl"] = nil
	}
	if body.UseSystemProxy != nil {
		siteData["useSystemProxy"] = *body.UseSystemProxy
	} else {
		siteData["useSystemProxy"] = false
	}
	if body.CustomHeaders != nil {
		siteData["customHeaders"] = *body.CustomHeaders
	} else {
		siteData["customHeaders"] = nil
	}
	if body.CustomHeadersOverrideRequestHeaders != nil {
		siteData["customHeadersOverrideRequestHeaders"] = *body.CustomHeadersOverrideRequestHeaders
	} else {
		siteData["customHeadersOverrideRequestHeaders"] = false
	}
	if body.ExternalCheckinURL != nil {
		siteData["externalCheckinUrl"] = service.NormalizeNullable(body.ExternalCheckinURL)
	} else {
		siteData["externalCheckinUrl"] = nil
	}
	// Per-site nullable overrides (resin / uTLS): explicit true/false stores
	// the override, explicit null clears it back to inheriting the global
	// flag, and an absent key leaves the column NULL (inherit) — matching
	// the tri-state the frontend form sends.
	if body.ResinEnabled.Present {
		if body.ResinEnabled.Null {
			siteData["resinEnabled"] = nil
		} else {
			siteData["resinEnabled"] = body.ResinEnabled.Value
		}
	}
	if body.UseUTLS.Present {
		if body.UseUTLS.Null {
			siteData["useUtls"] = nil
		} else {
			siteData["useUtls"] = body.UseUTLS.Value
		}
	}

	// Convert apiEndpoints
	if eps != nil {
		siteData["apiEndpoints"] = siteEndpointRecords(eps, now)
	}

	createdID, createErr := service.CreateSite(h.db.DB, siteData)
	if createErr != nil {
		// Check for unique constraint violation
		if isUniqueConstraintError(createErr) {
			writeErrorWithRequest(w, r, http.StatusConflict, fmt.Sprintf("A %s site with URL %s already exists.", platform, canonicalURL))
			return
		}
		slog.Error("CreateSite failed", "err", createErr, "platform", platform, "url", canonicalURL)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Create site failed")
		return
	}

	result, loadErr := service.LoadSiteWithEndpoints(h.db.DB, createdID)
	if loadErr != nil {
		slog.Error("LoadSiteWithEndpoints after create failed", "err", loadErr, "site_id", createdID)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Create site failed")
		return
	}
	if result == nil {
		slog.Error("LoadSiteWithEndpoints returned nil after create", "site_id", createdID)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Create site failed")
		return
	}
	if initializationPresetID != "" {
		result["initializationPresetId"] = initializationPresetID
	}
	// Match update/delete: clear the process-local accounts snapshot cache (via
	// the registered site-cache invalidator) so a freshly created site shows up
	// on the accounts page immediately instead of waiting out the snapshot TTL.
	service.InvalidateSiteCaches()
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, result)
}

func boolPtrValid(b *bool) bool {
	return b != nil
}
