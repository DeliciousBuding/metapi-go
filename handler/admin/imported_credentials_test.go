package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedCredentialManagementDoesNotExposeSecrets(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "fixture"); err != nil {
		t.Fatal(err)
	}
	var id, channelID int64
	if err := db.QueryRow(`SELECT id,channel_id FROM upstream_credentials ORDER BY id LIMIT 1`).Scan(&id, &channelID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='claudecode' WHERE id=?`, channelID); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	RegisterImportedUpstreamRoutes(mux, db.DB)
	patch := func(body string) *httptest.ResponseRecorder {
		out := httptest.NewRecorder()
		mux.ServeHTTP(out, httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/imported-upstreams/credentials/%d", id), strings.NewReader(body)))
		return out
	}
	for _, body := range []string{`{}`, `{"apiKey":"x","oauth":{"accessToken":"y"}}`, `{"oauth":{"refreshToken":"rt"}}`, `{"apiKey":"x","unknown":"y"}`} {
		if out := patch(body); out.Code != 400 {
			t.Fatalf("invalid replacement accepted: %s", out.Body.String())
		}
	}
	out := patch(`{"oauth":{"accessToken":"private-access-fixture","refreshToken":"private-refresh-fixture","clientId":"source-client","expiresAt":1}}`)
	if out.Code != 200 || strings.Contains(out.Body.String(), "private-") {
		t.Fatalf("replacement failed/leaked: %s", out.Body.String())
	}
	list := doGet(t, mux, fmt.Sprintf("/api/imported-upstreams/%d/credentials", channelID))
	if list.Code != 200 || strings.Contains(list.Body.String(), "private-") || !strings.Contains(list.Body.String(), `"kind":"oauth"`) || !strings.Contains(list.Body.String(), `"canRefresh":true`) {
		t.Fatalf("bad sanitized credential list: %s", list.Body.String())
	}
	inventory := doGet(t, mux, "/api/imported-upstreams")
	if strings.Contains(inventory.Body.String(), "private-") || strings.Contains(inventory.Body.String(), "oauthState") {
		t.Fatal("inventory exposed credential state")
	}
	if out := patch(`{"enabled":false}`); out.Code != 200 {
		t.Fatalf("disable failed: %s", out.Body.String())
	}
	if out := patch(`{"enabled":true,"apiKey":"replacement-static"}`); out.Code != 200 {
		t.Fatalf("replacement of disabled credential failed: %s", out.Body.String())
	}
	var kind, raw string
	if err := db.QueryRow(`SELECT kind,oauth_state FROM upstream_credentials WHERE id=?`, id).Scan(&kind, &raw); err != nil {
		t.Fatal(err)
	}
	if kind != "api_key" || raw != "{}" {
		t.Fatal("static credential retained stale OAuth refresh material")
	}
}
