package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestBackupImportLeavesMissingOwnershipDenied pins the cross-version restore
// contract for the ownership fix: a backup written before proxy_file_objects
// and proxy_video_tasks.owner existed must still import, and the rows it does
// carry stay ownerless. Ownerless rows are denied by the proxy (fail closed),
// so a restore can never resurrect files/videos as shared objects.
func TestBackupImportLeavesMissingOwnershipDenied(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}

	payload := `{
		"metadata": {"exported_at": "2026-01-01T00:00:00Z", "version": "1.0"},
		"type": "all",
		"tables": {
			"proxy_video_tasks": [
				{"public_id": "video_legacy_ownerless", "upstream_video_id": "upstream-legacy", "site_url": "", "token_value": ""}
			]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(payload))
	rec := httptest.NewRecorder()
	handler.importBackup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	imported, _ := body["imported"].(map[string]any)
	if got := imported["proxy_video_tasks"]; got != float64(1) {
		t.Fatalf("imported.proxy_video_tasks = %v, want 1; body = %s", got, rec.Body.String())
	}
	if _, present := imported["proxy_file_objects"]; present {
		t.Fatalf("import reports proxy_file_objects from a payload that does not carry it: %v", imported)
	}

	var owner *string
	if err := db.QueryRow(`SELECT owner FROM proxy_video_tasks WHERE public_id = 'video_legacy_ownerless'`).Scan(&owner); err != nil {
		t.Fatalf("read imported owner: %v", err)
	}
	if owner != nil {
		t.Fatalf("imported owner = %q, want NULL (fail closed)", *owner)
	}
}
