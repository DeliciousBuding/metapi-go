package admin

import (
	"database/sql"
	"errors"
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

	channelRows, err := queryRowsErr(h.db,
		`SELECT rc.*, a.username, `+
			credentialFragmentsSelect(h.db, "a.access_token", "access_token")+`, `+
			credentialFragmentsSelect(h.db, "a.api_token", "api_token")+`,
		        a.balance, a.status as account_status,
		        s.id as site_id, s.name as site_name, s.url as site_url, s.platform as site_platform, s.status as site_status
		 FROM route_channels rc
		 LEFT JOIN accounts a ON rc.account_id = a.id
		 LEFT JOIN sites s ON a.site_id = s.id
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
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if reason, err := h.validateChannelToken(body.AccountID, body.TokenID); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load channel credential")
		return
	} else if reason != "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, reason)
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
	if err := h.db.Get(&dupCount,
		h.db.Rebind(`SELECT COUNT(*) FROM route_channels
		 WHERE route_id = ? AND account_id = ?
		 AND (token_id = ? OR (token_id IS NULL AND CAST(? AS BIGINT) IS NULL))
		 AND (source_model = ? OR (source_model IS NULL AND CAST(? AS TEXT) IS NULL))`),
		routeID, body.AccountID, body.TokenID, body.TokenID, body.SourceModel, body.SourceModel); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to check existing channel")
		return
	}
	if dupCount > 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "a channel for this source model already exists")
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
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to create channel")
		return
	}

	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	created, err := queryRow(h.db, "SELECT * FROM route_channels WHERE id = ?", id)
	if err != nil || created == nil {
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "channel created but failed to load result")
		return
	}
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
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	created := 0
	skipped := 0
	var errors []string

	for _, ch := range body.Channels {
		if reason, err := h.validateChannelToken(ch.AccountID, ch.TokenID); err != nil {
			errors = append(errors, "failed to load channel credential")
			continue
		} else if reason != "" {
			errors = append(errors, reason)
			continue
		}
		var dupCount int
		if err := h.db.Get(&dupCount,
			h.db.Rebind(`SELECT COUNT(*) FROM route_channels
			 WHERE route_id = ? AND account_id = ?
			 AND (token_id = ? OR (token_id IS NULL AND CAST(? AS BIGINT) IS NULL))
			 AND (source_model = ? OR (source_model IS NULL AND CAST(? AS TEXT) IS NULL))`),
			routeID, ch.AccountID, ch.TokenID, ch.TokenID, ch.SourceModel, ch.SourceModel); err != nil {
			errors = append(errors, "failed to check existing channel")
			continue
		}
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

// Account and token are independently valid foreign keys, but a credential
// from another account must never become a channel's relay identity. Status
// remains operator-owned: disabled credentials can still be configured.
func (h *tokenRoutesHandler) validateChannelToken(accountID int64, tokenID *int64) (string, error) {
	if tokenID == nil {
		return "", nil
	}
	if *tokenID <= 0 {
		return "tokenId must be a positive integer", nil
	}
	var ownerID int64
	err := h.db.Get(&ownerID, h.db.Rebind("SELECT account_id FROM account_tokens WHERE id = ?"), *tokenID)
	if errors.Is(err, sql.ErrNoRows) {
		return "account token not found", nil
	}
	if err != nil {
		return "", err
	}
	if ownerID != accountID {
		return "account token does not belong to this account", nil
	}
	return "", nil
}

// ---- Clear Cooldown ----
// POST /api/routes/:id/cooldown/clear
// clearChannelCooldown resets the cooldown of ONE route_channels row — the
// per-candidate recovery entry used by the route decision panel (the
// route-level clearCooldown above resets every channel of the route at once).
// Idempotent: a row that is not cooled is a successful no-op; unknown ids 404.
func (h *tokenRoutesHandler) clearChannelCooldown(w http.ResponseWriter, r *http.Request) {
	channelID, err := strconv.ParseInt(chi.URLParam(r, "channelId"), 10, 64)
	if err != nil || channelID <= 0 {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}
	res, err := h.db.Exec(h.db.Rebind(`UPDATE route_channels SET cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0, cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL WHERE id = ?`), channelID)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to clear channel cooldown: "+err.Error())
		return
	}
	affected, err := res.RowsAffected()
	if err != nil || affected == 0 {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeChannelNotFound, "channel not found")
		return
	}
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "id": channelID})
}

func (h *tokenRoutesHandler) clearCooldown(w http.ResponseWriter, r *http.Request) {
	routeID, ok := pathID(w, r)
	if !ok {
		return
	}

	_, err := h.db.ExecContext(r.Context(), h.db.Rebind(`UPDATE route_channels SET cooldown_until = NULL, consecutive_fail_count = 0, cooldown_level = 0, cooldown_reason_code = NULL, cooldown_reason = NULL, cooldown_reason_at = NULL WHERE route_id = ?`), routeID)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to clear route cooldown: "+err.Error())
		return
	}
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

		res, err := h.db.Exec(h.db.Rebind("UPDATE route_channels SET "+strings.Join(sets, ", ")+" WHERE id = ?"), args...)
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
		writeErrorWithRequest(w, r, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}
	if len(body.Items) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "items is required")
		return
	}
	if len(body.Items) > 1000 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "too many items (max 1000)")
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

	h.db.Exec(h.db.Rebind("DELETE FROM route_channels WHERE id = ?"), channelID)
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---- Route Decisions ----

// GET /api/routes/decision?model=
