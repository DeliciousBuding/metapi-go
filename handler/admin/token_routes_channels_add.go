package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
)

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
		`SELECT COUNT(*) FROM route_channels
		 WHERE route_id = ? AND account_id = ?
		 AND (token_id = ? OR (token_id IS NULL AND CAST(? AS BIGINT) IS NULL))
		 AND (source_model = ? OR (source_model IS NULL AND CAST(? AS TEXT) IS NULL))`,
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
			`SELECT COUNT(*) FROM route_channels
			 WHERE route_id = ? AND account_id = ?
			 AND (token_id = ? OR (token_id IS NULL AND CAST(? AS BIGINT) IS NULL))
			 AND (source_model = ? OR (source_model IS NULL AND CAST(? AS TEXT) IS NULL))`,
			routeID, ch.AccountID, ch.TokenID, ch.TokenID, ch.SourceModel, ch.SourceModel); err != nil {
			errors = append(errors, "failed to check existing channel")
			continue
		}
		if dupCount > 0 {
			skipped++
			continue
		}

		_, err := h.db.Exec(
			"INSERT INTO route_channels (route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
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
	err := h.db.Get(&ownerID, "SELECT account_id FROM account_tokens WHERE id = ?", *tokenID)
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
