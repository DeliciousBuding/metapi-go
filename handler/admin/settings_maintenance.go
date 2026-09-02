package admin

import (
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterMaintenanceRoutes registers all /api/settings/maintenance routes.
func RegisterMaintenanceRoutes(r chi.Router, db *sqlx.DB) {
	handler := &maintenanceHandler{db: db}

	r.Post("/api/settings/maintenance/clear-cache", handler.clearCache)
	r.Post("/api/settings/maintenance/clear-usage", handler.clearUsage)
	r.Post("/api/settings/maintenance/factory-reset", handler.factoryReset)
}

type maintenanceHandler struct {
	db *sqlx.DB
}

const (
	clearCacheTaskType  = "maintenance-clear-cache"
	clearCacheTaskTitle = "Clear cache and rebuild routes"
	clearCacheDedupeKey = "maintenance-clear-cache"
)

// POST /api/settings/maintenance/clear-cache
// Real local ops:
// 1. Delete model_availability / route_channels / token_routes rows
// 2. Invalidate in-process caches (routing + accounts snapshot)
// 3. Queue a real background rebuild task (no fake stub job ids)
//
// Multi-instance known limitation: only this process's in-memory caches are cleared;
// peer instances retain their own caches until TTL/local invalidation.
func (h *maintenanceHandler) clearCache(w http.ResponseWriter, r *http.Request) {
	// Count before deletion
	var modelAvail, routeCh, tokenRoutes int64
	if err := h.db.Get(&modelAvail, h.db.Rebind("SELECT COUNT(*) FROM model_availability")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read model_availability: %v", err))
		return
	}
	if err := h.db.Get(&routeCh, h.db.Rebind("SELECT COUNT(*) FROM route_channels")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read route_channels: %v", err))
		return
	}
	if err := h.db.Get(&tokenRoutes, h.db.Rebind("SELECT COUNT(*) FROM token_routes")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read token_routes: %v", err))
		return
	}

	// Delete all (shared DB — multi-instance safe for durable state)
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM model_availability")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear model_availability: %v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM route_channels")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear route_channels: %v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM token_routes")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear token_routes: %v", err))
		return
	}

	// Local in-process invalidation (this instance only).
	invalidateLocalProcessCaches()

	// Ensure rebuild path uses this request's DB handle (tests / non-global DB).
	service.SetRouteRebuildDB(h.db)

	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      clearCacheTaskType,
		Title:     clearCacheTaskTitle,
		DedupeKey: clearCacheDedupeKey,
	}, func() (any, error) {
		// Rebuild is best-effort: after wiping availability tables there may be
		// little to rebuild until models are re-probed; still honest work.
		service.RebuildRoutesBestEffort()
		// Re-invalidate after rebuild so stale route matches cannot linger.
		invalidateLocalProcessCaches()
		return map[string]any{
			"deletedModelAvailability": modelAvail,
			"deletedRouteChannels":     routeCh,
			"deletedTokenRoutes":       tokenRoutes,
			"scope":                    "local-process-cache + shared-db-rows",
		}, nil
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success":                  true,
		"queued":                   true,
		"reused":                   reused,
		"jobId":                    task.ID,
		"taskId":                   task.ID,
		"status":                   string(task.Status),
		"message":                  "cache cleared; route rebuild started (this process in-memory cache is stale; multi-instance deployments must invalidate each instance)",
		"deletedModelAvailability": modelAvail,
		"deletedRouteChannels":     routeCh,
		"deletedTokenRoutes":       tokenRoutes,
	})
}

// invalidateLocalProcessCaches clears known process-local caches.
// Safe no-ops when caches are uninitialized.
func invalidateLocalProcessCaches() {
	routing.InvalidateCache()
	if globalAccountsCache != nil {
		globalAccountsCache.clear()
	}
}

// POST /api/settings/maintenance/clear-usage
func (h *maintenanceHandler) clearUsage(w http.ResponseWriter, r *http.Request) {
	var deletedProxyLogs int64
	if err := h.db.Get(&deletedProxyLogs, h.db.Rebind("SELECT COUNT(*) FROM proxy_logs")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read proxy_logs: %v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM proxy_logs")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear proxy_logs: %v", err))
		return
	}

	// Reset route channel stats
	if _, err := h.db.Exec(h.db.Rebind(`UPDATE route_channels SET
		success_count = 0, fail_count = 0, total_latency_ms = 0, total_cost = 0,
		last_used_at = NULL, last_selected_at = NULL, last_fail_at = NULL,
		consecutive_fail_count = 0, cooldown_level = 0, cooldown_until = NULL,
		cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL`)); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear route_channels stats: %v", err))
		return
	}

	// Reset account balanceUsed
	if _, err := h.db.Exec(h.db.Rebind("UPDATE accounts SET balance_used = 0")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to clear account balance usage: %v", err))
		return
	}

	// Accounts list snapshot may still show old balanceUsed until cleared.
	if globalAccountsCache != nil {
		globalAccountsCache.clear()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"message":          "usage stats cleared",
		"deletedProxyLogs": deletedProxyLogs,
	})
}

// POST /api/settings/maintenance/factory-reset
func (h *maintenanceHandler) factoryReset(w http.ResponseWriter, r *http.Request) {
	// Require confirmation token to prevent accidental invocation.
	var body struct {
		Confirm bool `json:"confirm"`
	}
	// Decode body; if confirm is not true, reject.
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !body.Confirm {
		writeError(w, http.StatusBadRequest, "factory reset requires confirmation. Set confirm: true in the request body")
		return
	}

	// One transaction over the schema registry's table set. This endpoint used
	// to walk a hand-copied 28-name list that had drifted from the schema, so a
	// "factory reset" silently left 9 tables standing — admin_sessions among
	// them, which meant every cookie issued before the reset stayed valid
	// against an otherwise empty database (session validation reads the table
	// per request, so emptying it is what actually revokes them). Wiping
	// admin_audit_logs, model_probe_results and the rest is the intended
	// semantics of restoring a clean install; store.FactoryReset owns the
	// FK-safe order, the sequence restart and the per-table counts.
	deleted, err := store.FactoryReset(h.db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("factory reset failed: %v", err))
		return
	}

	invalidateLocalProcessCaches()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "factory reset completed",
		"deleted": deleted,
	})
}
