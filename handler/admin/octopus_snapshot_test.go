package admin

import (
	"encoding/json"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOctopusReplacementRequiresReviewedRemovalConfirmation(t *testing.T) {
	db := setupBackupTestDB(t)
	if _, err := backupsvc.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "replacement-fixture"); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	RegisterBackupRoutes(mux, db.DB)
	snapshot, err := backupsvc.ParseOctopusV5([]byte(backupsvcFixtureV5))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Channels = snapshot.Channels[:1]
	snapshot.Credentials = snapshot.Credentials[:1]
	snapshot.Models = snapshot.Models[:1]
	snapshot.Grants = snapshot.Grants[:1]
	snapshot.Groups = snapshot.Groups[:0]
	snapshot.GroupItems = snapshot.GroupItems[:0]
	rawBytes, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(rawBytes)
	post := func(path string, replace bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(raw))
		req.Header.Set("X-External-Origin-Key", "replacement-fixture")
		if replace {
			req.Header.Set("X-Octopus-Replace-Origin", "true")
		}
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, req)
		return out
	}
	preview := post("/api/settings/backup/import/preview", false)
	if preview.Code != 200 || !strings.Contains(preview.Body.String(), `"removals"`) || strings.Contains(preview.Body.String(), "fixture-key-octopus") {
		t.Fatalf("unsafe removal preview: %s", preview.Body.String())
	}
	if result := post("/api/settings/backup/import", false); result.Code != 409 {
		t.Fatalf("unreviewed deletion accepted: %d %s", result.Code, result.Body.String())
	}
	var remaining int
	if err := db.Get(&remaining, `SELECT COUNT(*) FROM upstream_channels`); err != nil || remaining != 2 {
		t.Fatalf("denied replacement mutated graph: %d %v", remaining, err)
	}
	if result := post("/api/settings/backup/import", true); result.Code != 200 {
		t.Fatalf("reviewed replacement rejected: %s", result.Body.String())
	}
	if err := db.Get(&remaining, `SELECT COUNT(*) FROM upstream_channels`); err != nil || remaining != 1 {
		t.Fatalf("obsolete channels remained active: %d %v", remaining, err)
	}
}
