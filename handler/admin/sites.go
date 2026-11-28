package admin

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterSitesRoutes registers all /api/sites routes.
func RegisterSitesRoutes(r chi.Router, db *store.DB) {
	handler := &sitesHandler{db: db}

	r.Get("/api/sites", handler.listSites)
	r.Get("/api/sites/initialization-presets", handler.listInitializationPresets)
	r.Post("/api/sites", handler.createSite)
	r.Put("/api/sites/{id}", handler.updateSite)
	r.Delete("/api/sites/{id}", handler.deleteSite)
	r.Post("/api/sites/batch", handler.batchSites)
	r.Post("/api/sites/detect", handler.detectSite)
	r.Post("/api/sites/import", handler.importSites)

	// Sub-resources
	r.Get("/api/sites/{id}/disabled-models", handler.getDisabledModels)
	r.Put("/api/sites/{id}/disabled-models", handler.updateDisabledModels)
	r.Get("/api/sites/{id}/available-models", handler.getAvailableModels)
	r.Post("/api/sites/{id}/probe-now", handler.probeNow)
	r.Get("/api/sites/{id}/probe-stream", handler.probeStream)
}

type sitesHandler struct {
	db *store.DB
}

// ---- List Sites ----

func (h *sitesHandler) listSites(w http.ResponseWriter, r *http.Request) {
	// Lists answer with masked site credentials and must not be cached by the
	// browser or an intermediary.
	setNoStore(w)
	sites, err := service.ListSites(h.db.DB)
	if err != nil {
		slog.Error("Failed to load sites", "err", err)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load sites")
		return
	}
	writeJSON(w, http.StatusOK, sites)
}

// ---- Delete Site ----

func (h *sitesHandler) deleteSite(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := service.DeleteSite(h.db.DB, id); err != nil {
		slog.Error("Failed to delete site", "err", err, "site_id", id)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to delete site")
		return
	}
	service.InvalidateSiteCaches()
	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Batch Sites ----

func (h *sitesHandler) batchSites(w http.ResponseWriter, r *http.Request) {
	var body payloads.SiteBatchPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid site payload: "+err.Error())
		return
	}

	if len(body.IDs) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "ids is required")
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{
		"enable": true, "disable": true, "delete": true,
		"enableSystemProxy": true, "disableSystemProxy": true,
	}
	if !validActions[action] {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid action")
		return
	}

	var successIDs []int64
	var failedItems []map[string]any

	for _, rawID := range body.IDs {
		id := int64(rawID)
		var existing store.Site
		err := h.db.Get(&existing, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Site not found"})
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		switch action {
		case "delete":
			h.db.Exec("DELETE FROM sites WHERE id = ?", id)
		case "enableSystemProxy":
			h.db.Exec("UPDATE sites SET use_system_proxy = true, updated_at = ? WHERE id = ?", now, id)
		case "disableSystemProxy":
			h.db.Exec("UPDATE sites SET use_system_proxy = false, updated_at = ? WHERE id = ?", now, id)
		case "enable":
			h.db.Exec("UPDATE sites SET status = 'active', updated_at = ? WHERE id = ?", now, id)
			service.ApplySiteStatusSideEffects(h.db.DB, id, existing.Name, "active")
		case "disable":
			h.db.Exec("UPDATE sites SET status = 'disabled', updated_at = ? WHERE id = ?", now, id)
			service.ApplySiteStatusSideEffects(h.db.DB, id, existing.Name, "disabled")
		}
		successIDs = append(successIDs, id)
	}

	// Invalidate caches when batch action mutates site state
	if action == "delete" || action == "enable" || action == "disable" ||
		action == "enableSystemProxy" || action == "disableSystemProxy" {
		service.InvalidateSiteCaches()
		routing.InvalidateCache()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, data any) {
	shared.WriteJSON(w, statusCode, data)
}

func normalizeSiteStatusString(s *string) string {
	if s == nil {
		return ""
	}
	status := strings.TrimSpace(strings.ToLower(*s))
	if status == "active" || status == "disabled" {
		return status
	}
	return ""
}

func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "duplicate entry") ||
		strings.Contains(msg, "unique") && strings.Contains(msg, "constraint")
}
