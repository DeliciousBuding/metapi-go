package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/service/catalogsync"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// ---- Model Catalog Data Sources ----
//
// The DB-persisted registry (catalog_sources) drives the merged model
// catalog: sources are fetched in list order, earlier sources override later
// ones, and per-source sync status (last success / error / entry count) is
// recorded. Manual sync (all or one source) + status query + auto-sync toggle
// live under /api/models/catalog-sync; CRUD under /api/models/catalog-sources.

// RegisterCatalogSourceRoutes mounts the catalog registry + sync endpoints.
func RegisterCatalogSourceRoutes(r chi.Router, db *sqlx.DB) {
	h := &catalogSourceHandler{db: db}
	r.Get("/api/models/catalog-sources", h.list)
	r.Post("/api/models/catalog-sources", h.create)
	r.Put("/api/models/catalog-sources/{id}", h.update)
	r.Delete("/api/models/catalog-sources/{id}", h.remove)
	r.Post("/api/models/catalog-sync", h.sync)
	r.Get("/api/models/catalog-sync", h.status)
	r.Put("/api/models/catalog-sync/config", h.updateConfig)
}

type catalogSourceHandler struct {
	db *sqlx.DB
}

// managerOrFallback returns the runtime manager (nil when the pricing
// catalog is disabled) plus a standalone store so CRUD still works while the
// catalog feature is off.
func (h *catalogSourceHandler) manager() *catalogsync.Manager {
	return app.CatalogManager()
}

func (h *catalogSourceHandler) store() *catalogsync.Store {
	return catalogsync.NewStore(h.db)
}

// GET /api/models/catalog-sources
func (h *catalogSourceHandler) list(w http.ResponseWriter, r *http.Request) {
	sources, err := h.store().ListSources(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list catalog sources")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources})
}

// POST /api/models/catalog-sources — body {name, url, type?, enabled?}
func (h *catalogSourceHandler) create(w http.ResponseWriter, r *http.Request) {
	var body catalogsync.SourceInput
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	source, err := h.store().CreateSource(r.Context(), body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	if manager := h.manager(); manager != nil {
		if err := manager.ReloadSources(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reload catalog sources")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source})
}

// PUT /api/models/catalog-sources/{id} — partial update {name?, url?, type?, enabled?, sortOrder?}
func (h *catalogSourceHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid source id"})
		return
	}
	var body catalogsync.SourceInput
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	source, err := h.store().UpdateSource(r.Context(), id, body)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "source not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": err.Error()})
		return
	}
	if manager := h.manager(); manager != nil {
		if err := manager.ReloadSources(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reload catalog sources")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source})
}

// DELETE /api/models/catalog-sources/{id}
func (h *catalogSourceHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid source id"})
		return
	}
	if err := h.store().DeleteSource(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "source not found"})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete catalog source")
		return
	}
	if manager := h.manager(); manager != nil {
		if err := manager.ReloadSources(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reload catalog sources")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

// POST /api/models/catalog-sync — body {sourceId?: number} (omitted = all)
func (h *catalogSourceHandler) sync(w http.ResponseWriter, r *http.Request) {
	manager := h.manager()
	if manager == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "pricing catalog is disabled (PRICING_CATALOG_ENABLED=false)"})
		return
	}
	var body struct {
		SourceID *int64 `json:"sourceId"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	sourceID := int64(0)
	if body.SourceID != nil {
		sourceID = *body.SourceID
	}

	// Manual sync can legitimately take tens of seconds across sources.
	ctx := r.Context()
	status, err := manager.SyncNow(ctx, sourceID)
	if err != nil {
		if errors.Is(err, catalogsync.ErrSyncBusy) {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "sync already running"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "catalog sync failed: " + err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// GET /api/models/catalog-sync — status (auto-sync toggle, snapshot meta,
// per-source last sync time / count / error).
func (h *catalogSourceHandler) status(w http.ResponseWriter, r *http.Request) {
	manager := h.manager()
	if manager == nil {
		writeJSON(w, http.StatusConflict, map[string]any{
			"message":  "pricing catalog is disabled (PRICING_CATALOG_ENABLED=false)",
			"autoSync": false,
		})
		return
	}
	writeJSON(w, http.StatusOK, manager.Status(r.Context()))
}

// PUT /api/models/catalog-sync/config — body {autoSync: bool}
func (h *catalogSourceHandler) updateConfig(w http.ResponseWriter, r *http.Request) {
	manager := h.manager()
	if manager == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"message": "pricing catalog is disabled (PRICING_CATALOG_ENABLED=false)"})
		return
	}
	var body struct {
		AutoSync *bool `json:"autoSync"`
	}
	if err := decodeJSONRequest(r, &body); err != nil || body.AutoSync == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body: autoSync required"})
		return
	}
	if err := manager.SetAutoSyncEnabled(r.Context(), *body.AutoSync); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update auto-sync config")
		return
	}
	writeJSON(w, http.StatusOK, manager.Status(r.Context()))
}
