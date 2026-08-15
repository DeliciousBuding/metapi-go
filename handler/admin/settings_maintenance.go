package admin

import (
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
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
	clearCacheTaskTitle = "清理缓存并重建路由"
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("读取 model_availability 失败：%v", err))
		return
	}
	if err := h.db.Get(&routeCh, h.db.Rebind("SELECT COUNT(*) FROM route_channels")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("读取 route_channels 失败：%v", err))
		return
	}
	if err := h.db.Get(&tokenRoutes, h.db.Rebind("SELECT COUNT(*) FROM token_routes")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("读取 token_routes 失败：%v", err))
		return
	}

	// Delete all (shared DB — multi-instance safe for durable state)
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM model_availability")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 model_availability 失败：%v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM route_channels")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 route_channels 失败：%v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM token_routes")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 token_routes 失败：%v", err))
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
		"message":                  "缓存已清理，重建路由已开始执行（本进程内存缓存已失效；多实例需各自失效）",
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
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("读取 proxy_logs 失败：%v", err))
		return
	}
	if _, err := h.db.Exec(h.db.Rebind("DELETE FROM proxy_logs")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 proxy_logs 失败：%v", err))
		return
	}

	// Reset route channel stats
	if _, err := h.db.Exec(h.db.Rebind(`UPDATE route_channels SET
		success_count = 0, fail_count = 0, total_latency_ms = 0, total_cost = 0,
		last_used_at = NULL, last_selected_at = NULL, last_fail_at = NULL,
		consecutive_fail_count = 0, cooldown_level = 0, cooldown_until = NULL`)); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 route_channels 统计失败：%v", err))
		return
	}

	// Reset account balanceUsed
	if _, err := h.db.Exec(h.db.Rebind("UPDATE accounts SET balance_used = 0")); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("清理 accounts 余额占用失败：%v", err))
		return
	}

	// Accounts list snapshot may still show old balanceUsed until cleared.
	if globalAccountsCache != nil {
		globalAccountsCache.clear()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"message":          "占用统计已清理",
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
		writeError(w, http.StatusBadRequest, "工厂重置需要确认。请在请求体中设置 confirm: true")
		return
	}

	tx, err := h.db.Beginx()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("开启事务失败：%v", err))
		return
	}
	defer tx.Rollback()

	deleted := map[string]int64{}

	// Delete all 27 tables in reverse FK order (children before parents).
	for _, table := range reverseAllTables {
		// Count before deletion
		var count int64
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := tx.Get(&count, countQuery); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("工厂重置失败：无法读取表 %s：%v", table, err))
			return
		}

		deleteQuery := fmt.Sprintf("DELETE FROM %s", table)
		if _, err := tx.Exec(deleteQuery); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("工厂重置失败：无法清空表 %s：%v", table, err))
			return
		}

		deleted[table] = count
	}

	// Reset auto-increment sequences.
	// SQLite: DELETE FROM sqlite_sequence removes all autoincrement tracking.
	// PostgreSQL: ALTER SEQUENCE... RESTART for each table with a serial column.
	// Try SQLite first (silently fails on non-SQLite); then try PG.
	driverName := h.db.DriverName()
	switch driverName {
	case "sqlite", "sqlite3":
		if _, err := tx.Exec("DELETE FROM sqlite_sequence"); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("工厂重置失败：无法重置自增序列：%v", err))
			return
		}
	case "pgx", "postgres":
		for _, table := range allTables {
			seqName := table + "_id_seq"
			if _, err := tx.Exec(fmt.Sprintf("ALTER SEQUENCE IF EXISTS %s RESTART WITH 1", seqName)); err != nil {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("工厂重置失败：无法重置序列 %s：%v", seqName, err))
				return
			}
		}
	}
	// For other drivers, skip sequence reset (no-op).

	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("提交事务失败：%v", err))
		return
	}

	invalidateLocalProcessCaches()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "工厂重置完成",
		"deleted": deleted,
	})
}
