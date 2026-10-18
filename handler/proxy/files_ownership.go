package proxyhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

// Files API ownership.
//
// The Files surface forwards to a channel's upstream credential, and that
// credential is shared by every downstream key routed to the same account.
// Upstream therefore answers "list files" with every file the account holds,
// and accepts any file id for metadata, content and deletion. Without a local
// ownership record the Files API has no horizontal isolation at all: any key
// could enumerate, download, overwrite or delete another key's files (audit
// H5). proxy_file_objects records the downstream owner at upload time and the
// three id-addressed handlers reject every request whose id is not mapped to
// the calling key.
//
// The owner string is the same identity direct video tasks already use
// (store.DirectVideoTaskOwner): a managed key is pinned to its key ID, so
// rotating the key's token does not lose the files. Rows without an owner are
// not possible for this table (owner is NOT NULL), but a missing row is
// deliberately indistinguishable from someone else's file: both are 404.
//
// Response processing is fail-closed as well. An upload whose response cannot
// be parsed creates no ownership record, so the file is unusable rather than
// shared. A list response that cannot be parsed is answered with 502 instead
// of being relayed, because relaying it would disclose the account-wide file
// list. Deletion removes the mapping only after the upstream delete succeeded.

// proxyFileOwner is the durable owner identity of the authenticated key.
// Empty means the request cannot own files (unauthenticated or malformed auth).
func proxyFileOwner(pac *auth.ProxyAuthContext) string {
	return auth.DirectVideoTaskOwner(pac)
}

var (
	errProxyFilesUnavailable = errors.New("file ownership storage is unavailable")
	errProxyFilesListShape   = errors.New("upstream file list response has an unsupported shape")
)

// recordProxyFileObject remembers that fileID belongs to owner. An id that is
// already mapped is never reassigned: file ids are unique upstream, so a
// collision would mean either a replay or two upstreams returning the same id,
// and silently taking ownership away from the first key would be worse than
// leaving the second upload inaccessible.
func recordProxyFileObject(pac *auth.ProxyAuthContext, fileID string) {
	fileID = strings.TrimSpace(fileID)
	owner := proxyFileOwner(pac)
	if fileID == "" || owner == "" {
		slog.Warn("proxy files: upload response has no usable file id for ownership",
			"has_id", fileID != "", "has_owner", owner != "")
		return
	}
	db := store.GetDB()
	if db == nil {
		slog.Warn("proxy files: ownership storage unavailable; uploaded file will not be readable", "file_id", fileID)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`INSERT INTO proxy_file_objects (file_id, owner, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(file_id) DO NOTHING`, fileID, owner, now, now)
	if err != nil {
		slog.Warn("proxy files: recording file ownership failed; uploaded file will not be readable",
			"file_id", fileID, "error", err)
		return
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		var existing string
		if err := db.QueryRow(`SELECT owner FROM proxy_file_objects WHERE file_id = ?`, fileID).Scan(&existing); err != nil {
			slog.Warn("proxy files: could not read existing file ownership", "file_id", fileID, "error", err)
			return
		}
		if existing != owner {
			slog.Warn("proxy files: upstream returned a file id already owned by another key; ownership was not reassigned",
				"file_id", fileID)
		}
	}
}

// proxyFileOwnedBy reports whether fileID belongs to the authenticated key.
// Storage failures and missing rows both deny: an ownership check that cannot
// be answered must not become a pass-through.
func proxyFileOwnedBy(pac *auth.ProxyAuthContext, fileID string) bool {
	fileID = strings.TrimSpace(fileID)
	owner := proxyFileOwner(pac)
	if fileID == "" || owner == "" {
		return false
	}
	db := store.GetDB()
	if db == nil {
		return false
	}
	var stored string
	err := db.QueryRow(`SELECT owner FROM proxy_file_objects WHERE file_id = ? LIMIT 1`, fileID).Scan(&stored)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Warn("proxy files: ownership lookup failed; denying access", "file_id", fileID, "error", err)
		}
		return false
	}
	return stored == owner
}

// ownedProxyFileIDs loads every file id owned by owner for list filtering.
func ownedProxyFileIDs(owner string) (map[string]bool, error) {
	if owner == "" {
		return nil, errProxyFilesUnavailable
	}
	db := store.GetDB()
	if db == nil {
		return nil, errProxyFilesUnavailable
	}
	rows, err := db.Query(`SELECT file_id FROM proxy_file_objects WHERE owner = ?`, owner)
	if err != nil {
		return nil, fmt.Errorf("load owned files: %w", err)
	}
	defer rows.Close()
	owned := make(map[string]bool)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan owned file: %w", err)
		}
		owned[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate owned files: %w", err)
	}
	return owned, nil
}

// filterProxyFileList rewrites an upstream GET /v1/files body so only the
// calling key's objects remain. The rest of the object is preserved; when the
// body carries first_id/last_id they are rewritten to match the filtered data
// so they cannot leak another key's file ids. Any shape this build does not
// understand is an error, never a pass-through: the caller turns it into 502.
func filterProxyFileList(body []byte, owner string) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, errProxyFilesListShape
	}
	rawData, ok := payload["data"]
	if !ok {
		return nil, errProxyFilesListShape
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawData, &items); err != nil || items == nil {
		return nil, errProxyFilesListShape
	}

	owned, err := ownedProxyFileIDs(owner)
	if err != nil {
		return nil, err
	}

	kept := make([]json.RawMessage, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		var obj struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(item, &obj); err != nil || strings.TrimSpace(obj.ID) == "" {
			return nil, errProxyFilesListShape
		}
		if owned[obj.ID] {
			kept = append(kept, item)
			ids = append(ids, obj.ID)
		}
	}

	encoded, err := json.Marshal(kept)
	if err != nil {
		return nil, errProxyFilesListShape
	}
	payload["data"] = encoded
	if _, present := payload["first_id"]; present {
		payload["first_id"] = firstOrNil(ids)
	}
	if _, present := payload["last_id"]; present {
		payload["last_id"] = lastOrNil(ids)
	}
	return json.Marshal(payload)
}

func firstOrNil(ids []string) json.RawMessage {
	if len(ids) == 0 {
		return json.RawMessage("null")
	}
	encoded, _ := json.Marshal(ids[0])
	return encoded
}

func lastOrNil(ids []string) json.RawMessage {
	if len(ids) == 0 {
		return json.RawMessage("null")
	}
	encoded, _ := json.Marshal(ids[len(ids)-1])
	return encoded
}

// deleteProxyFileObject drops the ownership record after a successful upstream
// deletion. A failed delete keeps the mapping so the owner can retry.
func deleteProxyFileObject(fileID string) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return
	}
	db := store.GetDB()
	if db == nil {
		return
	}
	if _, err := db.Exec(`DELETE FROM proxy_file_objects WHERE file_id = ?`, fileID); err != nil {
		slog.Warn("proxy files: deleting file ownership failed", "file_id", fileID, "error", err)
	}
}

// proxyFileIDFromResponse extracts the upstream file id from an upload
// response. Anything else is reported so the caller can log it.
func proxyFileIDFromResponse(body []byte) (string, bool) {
	var payload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", false
	}
	id := strings.TrimSpace(payload.ID)
	return id, id != ""
}
