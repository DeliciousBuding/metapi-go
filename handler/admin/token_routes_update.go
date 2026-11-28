package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Update Route ----
// PUT /api/routes/:id
func (h *tokenRoutesHandler) updateRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load route")
		return
	}
	if existing == nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeRouteNotFound, "route not found")
		return
	}

	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate the resulting mode configuration before writing anything —
	// mirrors the createRoute invariants (pattern → modelPattern required,
	// explicit_group → displayName required) so a mode switch can never
	// persist an inconsistent route.
	nextMode, _ := existing["routeMode"].(string)
	if v, ok := body["routeMode"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextMode = strings.TrimSpace(s)
		}
	}
	nextPattern, _ := existing["modelPattern"].(string)
	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextPattern = strings.TrimSpace(s)
		}
	}
	nextDisplayName, _ := existing["displayName"].(string)
	if v, ok := body["displayName"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextDisplayName = strings.TrimSpace(s)
		}
	}
	if nextMode != "explicit_group" && nextPattern == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "model match pattern is required")
		return
	}
	if nextMode == "explicit_group" && nextDisplayName == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "explicit groups must specify an external model name")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := h.db.Beginx()
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to begin transaction")
		return
	}
	defer tx.Rollback()

	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET model_pattern = ?, updated_at = ? WHERE id = ?"), strings.TrimSpace(s), now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["displayName"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET display_name = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["displayIcon"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET display_icon = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["enabled"]; ok {
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET enabled = ?, updated_at = ? WHERE id = ?"), toBool(v), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}
	if v, ok := body["routeMode"]; ok {
		if s, ok2 := v.(string); ok2 {
			mode := strings.TrimSpace(s)
			if mode != "pattern" && mode != "explicit_group" {
				writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid routeMode. Expected pattern or explicit_group.")
				return
			}
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET route_mode = ?, updated_at = ? WHERE id = ?"), mode, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["routingStrategy"]; ok {
		if s, ok2 := v.(string); ok2 {
			if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET routing_strategy = ?, updated_at = ? WHERE id = ?"), s, now, id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
				return
			}
		}
	}
	if v, ok := body["traceStickyMode"]; ok {
		mode, valid := v.(string)
		if !valid || !store.ValidTraceStickyMode(mode) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid traceStickyMode")
			return
		}
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET trace_sticky_mode=?,updated_at=? WHERE id=?"), mode, now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}
	if v, ok := body["modelMapping"]; ok {
		mappingJSON, _ := json.Marshal(v)
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET model_mapping = ?, updated_at = ? WHERE id = ?"), string(mappingJSON), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}
	// contextLength: present key updates (including explicit null/0 clear → NULL).
	// Metadata only — no proxy max-token enforcement is wired from this field yet.
	if v, ok := body["contextLength"]; ok {
		if _, err := tx.Exec(tx.Rebind("UPDATE token_routes SET context_length = ?, updated_at = ? WHERE id = ?"), normalizeContextLengthOrNull(v), now, id); err != nil {
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route")
			return
		}
	}

	// Update source route IDs for explicit_group
	if v, ok := body["sourceRouteIds"]; ok {
		if ids, ok2 := v.([]any); ok2 {
			if _, err := tx.Exec(tx.Rebind("DELETE FROM route_group_sources WHERE group_route_id = ?"), id); err != nil {
				writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route group source")
				return
			}
			for _, rawID := range ids {
				switch rid := rawID.(type) {
				case float64:
					if _, err := tx.Exec(tx.Rebind("INSERT INTO route_group_sources (group_route_id, source_route_id) VALUES (?, ?)"), id, int64(rid)); err != nil {
						writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update route group source")
						return
					}
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to commit transaction")
		return
	}

	// Pattern change: recompose automatic channels while preserving manual overrides.
	// Unrelated field updates (displayName, routingStrategy, modelMapping, enabled)
	// must not wipe intentional in-route channel configuration.
	if v, ok := body["modelPattern"]; ok {
		if s, ok2 := v.(string); ok2 {
			nextPattern := strings.TrimSpace(s)
			prevPattern, _ := existing["modelPattern"].(string)
			mode, _ := existing["routeMode"].(string)
			if nextPattern != prevPattern && !routing.IsExplicitGroupRoute(mode) {
				if _, err := service.RebuildTokenRoutesFromAvailability(r.Context(), h.db.DB); err != nil {
					slog.Warn("route update: rebuild after modelPattern change failed", "routeId", id, "error", err)
				}
			}
		}
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	updated, err := queryRow(h.db, "SELECT * FROM token_routes WHERE id = ?", id)
	if err != nil || updated == nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "route updated but failed to load result")
		return
	}
	var srcIDs []int64
	if err := h.db.Select(&srcIDs, "SELECT source_route_id FROM route_group_sources WHERE group_route_id = ?", id); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "route updated but failed to load sources")
		return
	}
	updated["sourceRouteIds"] = srcIDs
	writeJSON(w, http.StatusOK, updated)
}

// ---- Delete Route ----
// DELETE /api/routes/:id
func (h *tokenRoutesHandler) deleteRoute(w http.ResponseWriter, r *http.Request) {
	deleteLifecycleEntity(w, r, h.db, upstream.KindRoute, true)
}
