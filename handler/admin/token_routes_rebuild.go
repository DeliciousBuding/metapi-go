package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/service"
)

const routesRebuildWorkBudget = 30 * time.Minute
const routesRebuildTaskType = "routes-rebuild"

// rebuildRoutes preserves the synchronous response when wait is true or absent.
// Explicit wait:false returns a real task immediately; /api/tasks carries its
// eventual result, so a browser disconnect or navigation does not own the work.
func (h *tokenRoutesHandler) rebuildRoutes(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshModels *bool `json:"refreshModels"`
		Wait          *bool `json:"wait"`
	}
	if r.Body != nil {
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if readErr == nil && len(bytes.TrimSpace(body)) > 0 {
			if err := json.Unmarshal(body, &req); err != nil {
				slog.Warn("routes rebuild: malformed body, using defaults", "error", err)
			}
		}
	}
	refreshModels := req.RefreshModels == nil || *req.RefreshModels
	parent := context.WithoutCancel(r.Context())
	run := func() (map[string]any, error) {
		ctx, cancel := context.WithTimeout(parent, routesRebuildWorkBudget)
		defer cancel()
		return h.runRoutesRebuild(ctx, refreshModels)
	}
	if req.Wait != nil && !*req.Wait {
		task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
			Type:  routesRebuildTaskType,
			Title: "Rebuild routes",
			// A local-only recomposition must not swallow a requested upstream
			// refresh: only identical work is eligible for deduplication.
			DedupeKey: fmt.Sprintf("routes-rebuild:refresh-models=%t", refreshModels),
		}, func() (any, error) {
			result, err := run()
			if err != nil {
				slog.Error("routes rebuild failed", "error", err)
				// Task errors are an admin API response too, not a raw SQL or
				// upstream-error log sink.
				return nil, errors.New("route rebuild failed")
			}
			return result, nil
		})
		writeJSON(w, http.StatusAccepted, map[string]any{
			"success": true, "queued": true, "reused": reused,
			"jobId": task.ID, "taskId": task.ID, "status": string(task.Status),
			"message": "route rebuild started; observe the task for its result",
		})
		return
	}
	result, err := run()
	if err != nil {
		writeRebuildFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// runRoutesRebuild is the single owner of synchronous and queued rebuild work.
func (h *tokenRoutesHandler) runRoutesRebuild(ctx context.Context, refreshModels bool) (map[string]any, error) {
	var stats service.RouteRebuildStats
	var modelRefresh map[string]any
	if refreshModels {
		summary := service.SyncAllAccountModels(ctx, h.db)
		if summary.RebuildErr != nil {
			return nil, summary.RebuildErr
		}
		stats = summary.Rebuild
		modelRefresh = map[string]any{
			"total": summary.Total, "success": summary.Success, "failed": summary.Failed,
			"notProcessed": summary.Total - summary.Success - summary.Failed,
		}
	} else {
		var err error
		stats, err = service.RebuildTokenRoutesFromAvailability(ctx, h.db)
		if err != nil {
			return nil, err
		}
	}
	shared.RecordRouteRebuildCompleted()
	invalidateChannelsSnapshotCache()
	slog.Info("routes rebuild completed", "routesConsidered", stats.RoutesConsidered,
		"patternRoutes", stats.PatternRoutes, "groupRoutes", stats.GroupRoutes,
		"channelsInserted", stats.ChannelsInserted, "channelsRemoved", stats.ChannelsRemoved)
	result := map[string]any{
		"success": true, "queued": false, "reused": false, "status": "completed",
		"message":          "route channels rebuilt and cache refreshed",
		"routesConsidered": stats.RoutesConsidered, "patternRoutes": stats.PatternRoutes,
		"routesCreated": stats.RoutesCreated, "unsafeModelsSkipped": stats.UnsafeModelsSkipped,
		"groupRoutes": stats.GroupRoutes, "channelsInserted": stats.ChannelsInserted,
		"channelsRemoved": stats.ChannelsRemoved, "channelsKept": stats.ChannelsKept,
		"changed": stats.Changed,
	}
	if modelRefresh != nil {
		result["modelRefresh"] = modelRefresh
	}
	return result, nil
}

func writeRebuildFailure(w http.ResponseWriter, err error) {
	slog.Error("routes rebuild failed", "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{
		"success": false, "queued": false, "reused": false,
		"status": "failed", "message": "route rebuild failed",
	})
}
