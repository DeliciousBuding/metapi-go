package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/go-chi/chi/v5"
)

func (h *tokenRoutesHandler) getRouteChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	channelRows := queryRows(h.db,
		`SELECT rc.*, a.username, `+
			credentialFragmentsSelect(h.db, "a.access_token", "access_token")+`, `+
			credentialFragmentsSelect(h.db, "a.api_token", "api_token")+`,
		        a.balance, a.status as account_status,
		        s.id as site_id, s.name as site_name, s.url as site_url, s.platform as site_platform, s.status as site_status
		 FROM route_channels rc
		 LEFT JOIN accounts a ON rc.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id
		 WHERE rc.route_id = ?`, id)
	var enrichedChans []map[string]any
	for _, ch := range channelRows {
		enriched := map[string]any{
			"id":               ch["id"],
			"routeId":          ch["routeId"],
			"accountId":        ch["accountId"],
			"tokenId":          ch["tokenId"],
			"oauthRouteUnitId": ch["oauthRouteUnitId"],
			"sourceModel":      ch["sourceModel"],
			"priority":         ch["priority"],
			"weight":           ch["weight"],
			"enabled":          ch["enabled"],
			"manualOverride":   ch["manualOverride"],
			"account":          routeChannelAccountPublic(ch),
			"site": map[string]any{
				"id":       ch["siteId"],
				"name":     ch["siteName"],
				"url":      ch["siteUrl"],
				"platform": ch["sitePlatform"],
				"status":   ch["siteStatus"],
			},
		}
		enrichedChans = append(enrichedChans, enriched)
	}
	writeJSON(w, http.StatusOK, enrichedChans)
}

// ---- Add Channel ----
// POST /api/routes/:id/channels
func (h *tokenRoutesHandler) addChannel(w http.ResponseWriter, r *http.Request) {
	routeID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body struct {
		AccountID   int64   `json:"accountId"`
		TokenID     *int64  `json:"tokenId"`
		SourceModel *string `json:"sourceModel"`
		Priority    *int64  `json:"priority"`
		Weight      *int64  `json:"weight"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	priority := int64(0)
	weight := int64(10)
	if body.Priority != nil {
		priority = *body.Priority
	}
	if body.Weight != nil {
		weight = *body.Weight
	}

	// Check for duplicates
	var dupCount int
	h.db.Get(&dupCount,
		h.db.Rebind(`SELECT COUNT(*) FROM route_channels
		 WHERE route_id = ? AND account_id = ?
		 AND (token_id = ? OR (token_id IS NULL AND ? IS NULL))
		 AND (source_model = ? OR (source_model IS NULL AND ? IS NULL))`),
		routeID, body.AccountID, body.TokenID, body.TokenID, body.SourceModel, body.SourceModel)
	if dupCount > 0 {
		writeError(w, http.StatusBadRequest, "该来源模型的通道已存在")
		return
	}

	// Operator-added channels are intentional configuration and must survive
	// RebuildTokenRoutesFromAvailability (manual_override stays true unless
	// the channel is explicitly deleted).
	id, err := execInsertID(h.db,
		"INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		routeID, body.AccountID, body.TokenID, body.SourceModel, priority, weight, true, true,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建通道失败")
		return
	}

	created := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", id)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, created)
}

// ---- Batch Add Channels ----
// POST /api/routes/:id/channels/batch
func (h *tokenRoutesHandler) batchAddChannels(w http.ResponseWriter, r *http.Request) {
	routeID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body struct {
		Channels []struct {
			AccountID   int64   `json:"accountId"`
			TokenID     *int64  `json:"tokenId"`
			SourceModel *string `json:"sourceModel"`
		} `json:"channels"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	created := 0
	skipped := 0
	var errors []string

	for _, ch := range body.Channels {
		var dupCount int
		h.db.Get(&dupCount,
			h.db.Rebind(`SELECT COUNT(*) FROM route_channels
			 WHERE route_id = ? AND account_id = ?
			 AND (token_id = ? OR (token_id IS NULL AND ? IS NULL))
			 AND (source_model = ? OR (source_model IS NULL AND ? IS NULL))`),
			routeID, ch.AccountID, ch.TokenID, ch.TokenID, ch.SourceModel, ch.SourceModel)
		if dupCount > 0 {
			skipped++
			continue
		}

		_, err := h.db.Exec(
			h.db.Rebind("INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override) VALUES (?, ?, ?, ?, ?, ?, ?, ?)"),
			routeID, ch.AccountID, ch.TokenID, ch.SourceModel, 0, 10, true, true,
		)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		created++
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"created": created,
		"skipped": skipped,
		"errors":  errors,
	})
}

// ---- Clear Cooldown ----
// POST /api/routes/:id/cooldown/clear
func (h *tokenRoutesHandler) clearCooldown(w http.ResponseWriter, r *http.Request) {
	routeID, ok := pathID(w, r)
	if !ok {
		return
	}

	h.db.Exec(h.db.Rebind(`UPDATE route_channels SET cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0 WHERE route_id = ?`), routeID)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---- Batch Update Channels ----
// PUT /api/channels/batch
func (h *tokenRoutesHandler) batchUpdateChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Updates []struct {
			ID       int64 `json:"id"`
			Priority int64 `json:"priority"`
		} `json:"updates"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var updatedIDs []int64
	for _, update := range body.Updates {
		h.db.Exec(h.db.Rebind("UPDATE route_channels SET priority = ?, manual_override = ? WHERE id = ?"), update.Priority, true, update.ID)
		updatedIDs = append(updatedIDs, update.ID)
	}

	var updatedChannels []map[string]any
	for _, cid := range updatedIDs {
		ch := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", cid)
		if ch != nil {
			updatedChannels = append(updatedChannels, ch)
		}
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"channels": normalizeSlice(updatedChannels),
	})
}

// ---- Update Channel ----
// PUT /api/channels/:channelId

// PUT /api/routes/reorder
// Body: { "items": [ { "id": 1, "sortOrder": 0 },... ] }
// Assigns explicit sort_order for admin drag-and-drop route lists.
func (h *tokenRoutesHandler) reorderRoutes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []struct {
			ID        int64 `json:"id"`
			SortOrder int64 `json:"sortOrder"`
		} `json:"items"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid payload")
		return
	}
	if len(body.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items is required")
		return
	}
	if len(body.Items) > 1000 {
		writeError(w, http.StatusBadRequest, "too many items (max 1000)")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var successIDs []int64
	var failedItems []map[string]any
	seen := map[int64]bool{}

	for _, item := range body.Items {
		if item.ID <= 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "invalid id"})
			continue
		}
		if item.SortOrder < 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "sortOrder must be >= 0"})
			continue
		}
		if seen[item.ID] {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "duplicate id in payload"})
			continue
		}
		seen[item.ID] = true

		res, err := h.db.Exec(h.db.Rebind(`
			UPDATE token_routes SET sort_order = ?, updated_at = ? WHERE id = ?
		`), item.SortOrder, now, item.ID)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": err.Error()})
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			failedItems = append(failedItems, map[string]any{"id": item.ID, "message": "route not found"})
			continue
		}
		successIDs = append(successIDs, item.ID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     len(failedItems) == 0,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}

func (h *tokenRoutesHandler) updateChannel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "channelId")
	channelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || channelID <= 0 {
		writeError(w, http.StatusNotFound, "通道不存在")
		return
	}

	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Any intentional channel edit marks manual_override so rebuild cannot
	// wipe operator-tuned priority/weight/enabled/sourceModel.
	if v, ok := body["priority"]; ok {
		h.db.Exec(h.db.Rebind("UPDATE route_channels SET priority = ?, manual_override = ? WHERE id = ?"), coerceFloat(v), true, channelID)
	}
	if v, ok := body["weight"]; ok {
		h.db.Exec(h.db.Rebind("UPDATE route_channels SET weight = ?, manual_override = ? WHERE id = ?"), coerceFloat(v), true, channelID)
	}
	if v, ok := body["enabled"]; ok {
		h.db.Exec(h.db.Rebind("UPDATE route_channels SET enabled = ?, manual_override = ? WHERE id = ?"), toBool(v), true, channelID)
	}
	if v, ok := body["sourceModel"]; ok {
		var sourceModel any
		switch s := v.(type) {
		case nil:
			sourceModel = nil
		case string:
			trimmed := strings.TrimSpace(s)
			if trimmed == "" {
				sourceModel = nil
			} else {
				sourceModel = trimmed
			}
		default:
			sourceModel = fmt.Sprint(v)
		}
		h.db.Exec(h.db.Rebind("UPDATE route_channels SET source_model = ?, manual_override = ? WHERE id = ?"), sourceModel, true, channelID)
	}

	updated := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", channelID)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, updated)
}

// ---- Delete Channel ----
// DELETE /api/channels/:channelId
func (h *tokenRoutesHandler) deleteChannel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "channelId")
	channelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || channelID <= 0 {
		writeJSON(w, http.StatusOK, map[string]any{"success": true})
		return
	}

	h.db.Exec(h.db.Rebind("DELETE FROM route_channels WHERE id = ?"), channelID)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---- Route Decisions ----

// GET /api/routes/decision?model=
