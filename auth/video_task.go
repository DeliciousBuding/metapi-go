package auth

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// DirectVideoTaskOwner stays stable across rotation of the same managed key.
func DirectVideoTaskOwner(pac *ProxyAuthContext) string {
	if pac == nil {
		return ""
	}
	owner := store.DirectVideoTaskOwner(pac.Source, pac.Token, pac.KeyID)
	if pac.OwnerUserID != nil && owner != "" {
		return store.UserKeyResourceOwnerPrefix + owner
	}
	return owner
}

func directVideoReadID(r *http.Request) string {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/videos/"), "/")
	if !strings.HasPrefix(r.URL.Path, "/v1/videos/") || len(parts) < 1 || !strings.HasPrefix(parts[0], "video_direct_") {
		return ""
	}
	if len(parts) != 1 && !(len(parts) == 2 && parts[1] == "content" && r.Method == http.MethodGet) {
		return ""
	}
	return parts[0]
}

// Budget exemptions are only granted after a durable ownership check. They do
// not replace the handler's TTL, model, route and current credential checks.
func ownsDirectVideoTask(publicID string, pac *ProxyAuthContext) bool {
	if publicID == "" || store.GetDB() == nil {
		return false
	}
	var raw string
	if err := store.GetDB().QueryRow(`SELECT direct_identity FROM proxy_video_tasks WHERE public_id=? AND direct_identity IS NOT NULL`, publicID).Scan(&raw); err != nil {
		return false
	}
	var identity struct {
		Version int    `json:"version"`
		Owner   string `json:"owner"`
	}
	return json.Unmarshal([]byte(raw), &identity) == nil && (identity.Version == 1 || identity.Version == 2) &&
		identity.Owner != "" && identity.Owner == DirectVideoTaskOwner(pac)
}
