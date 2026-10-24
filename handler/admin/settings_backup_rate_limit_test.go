package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAxonHubRateLimitMalformedHTTP(t *testing.T) {
	db := setupBackupTestDB(t)
	handler := &backupHandler{db: db.DB}
	// The same malformed field must fail in preview and commit without echoing
	// the field value or partially persisting any of the otherwise valid graph.
	payload := strings.Replace(backupFixtureAxonHubV14, `"id":11,`, `"settings":{"rateLimit":{"rpm":"fixture-private-rate-limit"}},"id":11,`, 1)
	for _, path := range []string{"/api/settings/backup/import/preview", "/api/settings/backup/import"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(payload))
			req.Header.Set("X-External-Origin-Key", "rate-limit-http")
			response := httptest.NewRecorder()
			if strings.HasSuffix(path, "/preview") {
				handler.previewBackupImport(response, req)
			} else {
				handler.importBackup(response, req)
			}
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "rate limit has an invalid field type") {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "fixture-private-rate-limit") {
				t.Fatal("error echoed private rate-limit value")
			}
			var channels int
			if err := db.Get(&channels, `SELECT COUNT(*) FROM upstream_channels`); err != nil || channels != 0 {
				t.Fatalf("malformed backup partially imported: channels=%d err=%v", channels, err)
			}
		})
	}
}
