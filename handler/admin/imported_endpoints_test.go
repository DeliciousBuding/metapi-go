package admin

import (
	"encoding/json"
	"strings"
	"testing"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestImportedUpstreamsReturnsTypedEndpointConfig(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := backupsvc.ImportAxonHubV14(db, []byte(`{"version":"1.4","channels":[{"id":1,"type":"xai","name":"fixture","base_url":"https://provider.invalid/api","credentials":{"apiKey":"fixture-secret-must-not-leak"},"supported_models":["model"]}],"models":[{"id":1,"model_id":"model","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"model"}}]}}]}`), "endpoint-api", false); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	RegisterImportedUpstreamRoutes(mux, db.DB)
	response := doGet(t, mux, "/api/imported-upstreams")
	if response.Code != 200 || strings.Contains(response.Body.String(), "fixture-secret") {
		t.Fatalf("unsafe inventory: %s", response.Body.String())
	}
	var body struct {
		Items []struct {
			Endpoints store.DirectEndpoints `json:"endpointConfig"`
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 || body.Items[0].Endpoints.Chat == nil || body.Items[0].Endpoints.Responses == nil {
		t.Fatalf("missing endpoint config: %s", response.Body.String())
	}
	if body.Items[0].Endpoints.Responses.URL != "https://provider.invalid/api/v1/responses" || body.Items[0].Endpoints.Responses.Auth != store.DirectAuthBearer {
		t.Fatalf("bad endpoint config: %+v", body.Items[0].Endpoints.Responses)
	}
}
