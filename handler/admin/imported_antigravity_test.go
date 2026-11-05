package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedAntigravityCredentialProjectIsWriteOnly(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"version":"1.4","channels":[{"id":1,"type":"antigravity","name":"project fixture","credentials":{"apiKey":"synthetic-old-refresh|synthetic-old-project"},"supported_models":["model"]}],"models":[{"id":1,"model_id":"model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"model"}}]}}]}`)
	if _, err := backup.ImportAxonHubV14(db, raw, "project-fixture", false); err != nil {
		t.Fatal(err)
	}
	var id, channelID int64
	if err := db.QueryRow(`SELECT id,channel_id FROM upstream_credentials`).Scan(&id, &channelID); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	RegisterImportedUpstreamRoutes(mux, db.DB)
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/imported-upstreams/credentials/%d", id),
		strings.NewReader(`{"oauth":{"accessToken":"","refreshToken":"synthetic-new-refresh","projectId":"synthetic-new-project"}}`)))
	if out.Code != 200 || strings.Contains(out.Body.String(), "synthetic-new-") {
		t.Fatalf("replacement failed or exposed material: %s", out.Body.String())
	}
	var state store.DirectOAuthState
	if err := db.Get(&state, `SELECT oauth_state FROM upstream_credentials`); err != nil {
		t.Fatal(err)
	}
	if state.ProjectID != "synthetic-new-project" || state.RefreshToken != "synthetic-new-refresh" || state.ClientID == "" {
		t.Fatal("replacement lost private project or issuing client")
	}
	for _, path := range []string{
		fmt.Sprintf("/api/imported-upstreams/%d/credentials", channelID),
		"/api/imported-upstreams",
		fmt.Sprintf("/api/imported-upstreams/%d", channelID),
	} {
		response := doGet(t, mux, path)
		if response.Code != 200 || strings.Contains(response.Body.String(), "synthetic-new-") || strings.Contains(response.Body.String(), `"projectId"`) {
			t.Fatalf("private identity exposed by %s: %s", path, response.Body.String())
		}
	}
	endpoint := store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: "https://cloudcode-pa.googleapis.com/v1internal", Auth: store.DirectAuthBearer, Profile: "antigravity"}}
	config := importedChannelConfig{Name: "fixture", Provider: "openai", BaseURL: "https://provider.invalid", Endpoints: endpoint}
	if err := validateImportedConfig(config); err == nil {
		t.Fatal("Antigravity profile accepted an unrelated provider")
	}
	config.Provider = "antigravity"
	if err := validateImportedConfig(config); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(state)
	if !strings.Contains(string(encoded), `"projectId"`) {
		t.Fatal("private JSON persistence lost project identity")
	}
}
