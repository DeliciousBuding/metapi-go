package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func (h *tokenRoutesHandler) getRouteChannels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	channelRows, err := queryRowsErr(h.db,
		`SELECT rc.*, a.username, `+
			store.CredentialFragmentsSelect(h.db.DB, "a.access_token", "access_token")+`, `+
			store.CredentialFragmentsSelect(h.db.DB, "a.api_token", "api_token")+`,
		        a.balance, a.status as account_status,
		        s.id as site_id, s.name as site_name, s.url as site_url, s.platform as site_platform, s.status as site_status,
		        t.name as token_name, t.enabled as token_enabled, t.is_default as token_is_default
		 FROM route_channels rc
		 LEFT JOIN accounts a ON rc.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id
		 LEFT JOIN account_tokens t ON rc.token_id = t.id
		 WHERE rc.route_id = ?`, id)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load route channels")
		return
	}
	var enrichedChans []map[string]any
	for _, ch := range channelRows {
		enriched := map[string]any{
			"id":               ch["id"],
			"routeId":          ch["routeId"],
			"accountId":        ch["accountId"],
			"tokenId":          ch["tokenId"],
			"token":            routeChannelTokenPublic(ch),
			"oauthRouteUnitId": ch["oauthRouteUnitId"],
			"sourceModel":      ch["sourceModel"],
			"priority":         ch["priority"],
			"weight":           ch["weight"],
			"enabled":          ch["enabled"],
			"manualOverride":   ch["manualOverride"],
			// Runtime counters the route detail sheet renders (channel truth
			// list): hit counts and persisted cooldown must reach the wire.
			"successCount":  ch["successCount"],
			"failCount":     ch["failCount"],
			"cooldownUntil": ch["cooldownUntil"],
			"account":       routeChannelAccountPublic(ch),
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

// PUT /api/channels/:channelId
func (h *tokenRoutesHandler) updateChannel(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "channelId")
	channelID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || channelID <= 0 {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}

	var body map[string]any
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Any intentional channel edit marks manual_override so rebuild cannot
	// wipe operator-tuned priority/weight/enabled/sourceModel.
	if v, ok := body["priority"]; ok {
		h.db.Exec("UPDATE route_channels SET priority = ?, manual_override = ? WHERE id = ?", coerceFloat(v), true, channelID)
	}
	if v, ok := body["weight"]; ok {
		h.db.Exec("UPDATE route_channels SET weight = ?, manual_override = ? WHERE id = ?", coerceFloat(v), true, channelID)
	}
	if v, ok := body["enabled"]; ok {
		h.db.Exec("UPDATE route_channels SET enabled = ?, manual_override = ? WHERE id = ?", toBool(v), true, channelID)
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
		h.db.Exec("UPDATE route_channels SET source_model = ?, manual_override = ? WHERE id = ?", sourceModel, true, channelID)
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	updated, err := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", channelID)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "channel updated but failed to load result")
		return
	}
	if updated == nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}
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

	h.db.Exec("DELETE FROM route_channels WHERE id = ?", channelID)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---- Batch Update Channels ----
// PUT /api/channels/batch
// Body: { "updates": [ { "id": 12, "enabled": false, "priority": 5, "weight": 20 } ] }
//
// Applies only the fields present on each item — the same partial-update
// semantics as PUT /api/channels/:channelId, including manual_override=true
// on any intentional edit. The response reports per-item truth: successIds
// for applied rows and failedItems (id + message) for rejected or missing
// ones, so a partial failure is never swallowed. `success` is true only when
// the batch is non-empty and every item applied. `channels` echoes the
// updated rows. The model-tester batch comparison's "disable failed
// channels" action uses this endpoint with `enabled:false` items; the admin
// AuditMiddleware records each call.
func (h *tokenRoutesHandler) batchUpdateChannels(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Updates []struct {
			ID       int64  `json:"id"`
			Priority *int64 `json:"priority"`
			Weight   *int64 `json:"weight"`
			Enabled  *bool  `json:"enabled"`
		} `json:"updates"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if len(body.Updates) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "updates is required")
		return
	}
	if len(body.Updates) > 1000 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "too many items (max 1000)")
		return
	}

	successIDs := []int64{}
	failedItems := []map[string]any{}
	seen := map[int64]bool{}

	for _, update := range body.Updates {
		if update.ID <= 0 {
			failedItems = append(failedItems, map[string]any{"id": update.ID, "message": "invalid id"})
			continue
		}

		// Shape validation runs before the duplicate check: an item carrying
		// no updatable fields is rejected on shape alone and must not consume
		// the id, so a well-formed item for the same id can still apply.
		sets := []string{}
		args := []any{}
		if update.Priority != nil {
			sets = append(sets, "priority = ?")
			args = append(args, *update.Priority)
		}
		if update.Weight != nil {
			sets = append(sets, "weight = ?")
			args = append(args, *update.Weight)
		}
		if update.Enabled != nil {
			sets = append(sets, "enabled = ?")
			args = append(args, *update.Enabled)
		}
		if len(sets) == 0 {
			failedItems = append(failedItems, map[string]any{"id": update.ID, "message": "no updatable fields (priority, weight or enabled required)"})
			continue
		}
		if seen[update.ID] {
			failedItems = append(failedItems, map[string]any{"id": update.ID, "message": "duplicate id in payload"})
			continue
		}
		seen[update.ID] = true
		// Any intentional channel edit marks manual_override so a route
		// rebuild cannot wipe operator tuning (mirrors the single-channel
		// update path).
		sets = append(sets, "manual_override = ?")
		args = append(args, true)
		args = append(args, update.ID)

		res, err := h.db.Exec("UPDATE route_channels SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": update.ID, "message": err.Error()})
			continue
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			failedItems = append(failedItems, map[string]any{"id": update.ID, "message": "channel not found"})
			continue
		}
		successIDs = append(successIDs, update.ID)
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	updatedChannels := []map[string]any{}
	for _, cid := range successIDs {
		ch, err := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", cid)
		if err != nil || ch == nil {
			// Writes are already committed; retain their per-item outcome even
			// when the response projection cannot be loaded.
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"error":       "channels updated but failed to load results",
				"errorCode":   ErrorCodeResourceLoadFailed,
				"success":     false,
				"successIds":  successIDs,
				"failedItems": failedItems,
			})
			return
		}
		updatedChannels = append(updatedChannels, ch)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     len(failedItems) == 0,
		"successIds":  successIDs,
		"failedItems": failedItems,
		"channels":    normalizeSlice(updatedChannels),
	})
}
