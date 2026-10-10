package admin

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUpstreamMediaPresetVersionAndValidation(t *testing.T) {
	r := presetRouter()
	for _, base := range []string{"https://relay.example", "https://relay.example/v1"} {
		_, endpoints := resolvePreset(t, r, "new-api-connection", base)
		if endpoints.Chat.URL != "https://relay.example/v1/chat/completions" || endpoints.Gemini.URL != "https://relay.example/v1beta/models" || endpoints.Video.URL != "https://relay.example/v1/videos" {
			t.Fatalf("versioned gateway endpoints drifted: %+v", endpoints)
		}
	}
	_, deepseek := resolvePreset(t, r, "deepseek-openai", "https://api.deepseek.com/v1")
	if deepseek.Completions.URL != "https://api.deepseek.com/beta/completions" {
		t.Fatalf("legacy FIM URL = %s", deepseek.Completions.URL)
	}
	for _, preset := range service.ListUpstreamPresets() {
		_, endpoints := resolvePreset(t, r, preset.ID, "https://relay.example")
		raw, err := endpoints.Value()
		if err != nil {
			t.Fatal(err)
		}
		var loaded store.DirectEndpoints
		if err := loaded.Scan(raw); err != nil {
			t.Fatalf("%s resolved an invalid contract: %v", preset.ID, err)
		}
		if !reflect.DeepEqual(endpoints, loaded) {
			t.Fatalf("%s lost endpoint fields", preset.ID)
		}
	}
}

func TestUpstreamMediaCatalogPersistenceAndRestriction(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			r := catalogMux(db)
			// All 18 fields coexist, including two different embedding and image
			// wire formats. Each is independently addressable by a persisted bit.
			endpoints := service.UpstreamPresetEndpointPaths("new-api-connection", "new-api")
			endpoints.ImageVariation = &store.DirectEndpoint{URL: "/v1/images/variations", Auth: store.DirectAuthBearer}
			endpoints.GeminiEmbeddings = &store.DirectEndpoint{URL: "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}
			endpoints.JinaEmbeddings = &store.DirectEndpoint{URL: "/jina/embeddings", Auth: store.DirectAuthBearer, Profile: "jina-embeddings"}
			endpoints.ModelScopeImageGeneration = &store.DirectEndpoint{URL: "/modelscope/images/generations", Auth: store.DirectAuthBearer, Profile: "modelscope-image"}
			var bits []int
			for _, entry := range endpoints.Entries() {
				if entry.Endpoint == nil {
					continue
				}
				entry.Endpoint.URL = "https://relay.example" + entry.Endpoint.URL
				bits = append(bits, entry.Protocol)
			}
			input := catalogChannelInput("Media", "https://relay.example")
			input["endpointConfig"] = endpoints
			channelID := catalogID(catalogCall(t, r, "POST", catalogPrefix, input, 201))
			base := fmt.Sprintf(catalogPrefix+"/%d", channelID)
			credID := catalogID(catalogCall(t, r, "POST", base+"/credentials", map[string]any{"name": "Media key", "apiKey": "fixture-media"}, 201))
			modelID := catalogCreateModel(t, r, channelID, "media-model")
			grantID := catalogID(catalogCall(t, r, "POST", catalogPrefix+"/grants", map[string]any{"modelId": modelID, "credentialId": credID, "protocols": bits}, 201))
			order := []int{store.DirectProtocolJinaEmbeddings, store.DirectProtocolEmbeddings, store.DirectProtocolModelScopeImageGeneration, store.DirectProtocolImageGeneration}
			group := catalogCall(t, r, "POST", catalogPrefix+"/groups", map[string]any{"name": "Media group", "route": map[string]any{"modelPattern": "media-alias"}, "members": []any{map[string]any{"grantId": grantID, "protocolOrder": order}}}, 201)
			memberID := catalogID(group["members"].([]any)[0].(map[string]any))
			var loaded store.DirectEndpoints
			if err := db.Get(&loaded, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), channelID); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(loaded, endpoints) {
				t.Fatal("persisted endpoint fields diverged")
			}
			var grantMask int
			if err := db.Get(&grantMask, db.Rebind(`SELECT protocols FROM upstream_grants WHERE id=?`), grantID); err != nil || grantMask != (1<<19)-2 {
				t.Fatalf("grant mask = %d, err=%v", grantMask, err)
			}
			// Shrinking configuration or a grant cannot silently change an
			// existing member's allowed wire formats.
			endpoints.JinaEmbeddings = nil
			catalogCall(t, r, "PATCH", base, map[string]any{"endpointConfig": endpoints}, 400)
			grantPath := fmt.Sprintf(catalogPrefix+"/grants/%d", grantID)
			catalogCall(t, r, "PATCH", grantPath, map[string]any{"protocols": []int{store.DirectProtocolEmbeddings}}, 409)
			catalogCall(t, r, "PATCH", fmt.Sprintf(catalogPrefix+"/members/%d", memberID), map[string]any{"protocolOrder": []int{store.DirectProtocolEmbeddings}}, 200)
			catalogCall(t, r, "PATCH", grantPath, map[string]any{"protocols": []int{store.DirectProtocolEmbeddings}}, 200)
			catalogCall(t, r, "PATCH", base, map[string]any{"endpointConfig": endpoints}, 200)
			out := catalogRequest(r, "GET", base+"/models", nil)
			var listing struct {
				Items []struct {
					Grants []struct {
						Protocols int `json:"protocols"`
					} `json:"grants"`
				} `json:"items"`
			}
			if err := json.Unmarshal(out.Body.Bytes(), &listing); err != nil || len(listing.Items) != 1 || len(listing.Items[0].Grants) != 1 || listing.Items[0].Grants[0].Protocols != store.DirectProtocolEmbeddings {
				t.Fatalf("media grant projection lost: %s", out.Body.String())
			}
		})
	}
}
