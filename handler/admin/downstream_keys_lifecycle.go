package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
)

// A changed bearer secret survives session revocation, so rotation requires reauthentication.
func (h *downstreamKeysHandler) allowKeyRotation(w http.ResponseWriter, r *http.Request, id int64, key string) bool {
	if !strings.HasPrefix(key, "sk-") || len(key) < 6 {
		writeError(w, http.StatusBadRequest, "key must start with sk- and be at least 6 characters long")
		return false
	}
	if !auth.ReauthConfirmed(r) {
		auth.WriteReauthRequired(w)
		return false
	}
	var dupCount int
	h.db.Get(&dupCount, "SELECT COUNT(*) FROM downstream_api_keys WHERE key = ? AND id != ?", key, id)
	if dupCount > 0 {
		writeError(w, http.StatusConflict, "API key already exists")
		return false
	}
	return true
}

// POST /api/downstream-keys/:id/reset-usage
func (h *downstreamKeysHandler) resetUsage(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
		writeError(w, http.StatusInternalServerError, "reset failed")
		return
	}

	updated, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "API key usage reset but failed to load result")
		return
	}
	// Reset-usage must not return full key; only keyMasked.
	redactDownstreamKeySecret(updated)
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"item":    updated,
	})
}

// DELETE /api/downstream-keys/:id
func (h *downstreamKeysHandler) deleteKey(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	if _, err := h.db.Exec("DELETE FROM downstream_api_keys WHERE id = ?", id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/downstream-keys/batch
func (h *downstreamKeysHandler) batchKeys(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs            []int64  `json:"ids"`
		Action         string   `json:"action"`
		GroupName      *string  `json:"groupName"`
		GroupOperation *string  `json:"groupOperation"`
		Tags           []string `json:"tags"`
		TagOperation   *string  `json:"tagOperation"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{
		"enable": true, "disable": true, "delete": true,
		"resetUsage": true, "updateMetadata": true,
	}
	if !validActions[action] {
		writeError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	var successIDs []int64
	var failedItems []map[string]any

	for _, id := range body.IDs {
		row, err := queryRow(h.db, "SELECT * FROM downstream_api_keys WHERE id = ?", id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "failed to load API key"})
			continue
		}
		if row == nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "API key not found"})
			continue
		}
		now := time.Now().UTC().Format(time.RFC3339)
		switch action {
		case "delete":
			if _, err := h.db.Exec("DELETE FROM downstream_api_keys WHERE id = ?", id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "delete failed"})
				continue
			}
		case "resetUsage":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET used_cost = 0, used_requests = 0, updated_at = ? WHERE id = ?", now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "reset failed"})
				continue
			}
		case "enable":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?", true, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "enable failed"})
				continue
			}
		case "disable":
			if _, err := h.db.Exec("UPDATE downstream_api_keys SET enabled = ?, updated_at = ? WHERE id = ?", false, now, id); err != nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "disable failed"})
				continue
			}
		case "updateMetadata":
			if body.GroupOperation != nil && *body.GroupOperation == "set" && body.GroupName != nil {
				if _, err := h.db.Exec("UPDATE downstream_api_keys SET group_name = ?, updated_at = ? WHERE id = ?", *body.GroupName, now, id); err != nil {
					failedItems = append(failedItems, map[string]any{"id": id, "message": "metadata update failed"})
					continue
				}
			} else if body.GroupOperation != nil && *body.GroupOperation == "clear" {
				if _, err := h.db.Exec("UPDATE downstream_api_keys SET group_name = NULL, updated_at = ? WHERE id = ?", now, id); err != nil {
					failedItems = append(failedItems, map[string]any{"id": id, "message": "metadata update failed"})
					continue
				}
			}
		}
		successIDs = append(successIDs, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}
