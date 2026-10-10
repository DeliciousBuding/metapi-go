package admin

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUpstreamAnonymousCredentialLifecycle(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			r := catalogMux(db)
			endpoints := store.DirectEndpoints{
				Ollama:   &store.DirectEndpoint{URL: "http://localhost:11434/api/chat", Auth: "none", Profile: "ollama"},
				Messages: &store.DirectEndpoint{URL: "http://localhost:11434/v1/messages", Auth: "bearer", Profile: "ollama-messages"},
			}
			input := catalogChannelInput("Local Ollama", "http://localhost:11434")
			input["provider"], input["endpointConfig"] = "ollama", endpoints
			channelID := catalogID(catalogCall(t, r, "POST", catalogPrefix, input, 201))
			base := fmt.Sprintf("%s/%d", catalogPrefix, channelID)
			for _, invalid := range []map[string]any{
				{"kind": "none", "apiKey": "fixture-stale-secret"},
				{"kind": "none", "oauth": map[string]any{"accessToken": "fixture-stale-secret"}},
				{"kind": "api_key"}, {"kind": "invalid"}, {},
			} {
				invalid["name"] = "Invalid"
				catalogCall(t, r, "POST", base+"/credentials", invalid, 400)
			}
			credentialID := catalogID(catalogCall(t, r, "POST", base+"/credentials", map[string]any{"name": "Local access", "kind": "none", "enabled": true}, 201))
			credentialPath := fmt.Sprintf("%s/credentials/%d", catalogPrefix, credentialID)
			modelID := catalogCreateModel(t, r, channelID, "local-model")
			grant := map[string]any{"modelId": modelID, "credentialId": credentialID, "protocols": []int{store.DirectProtocolMessages}}
			catalogCall(t, r, "POST", catalogPrefix+"/grants", grant, 400)
			grant["protocols"] = []int{store.DirectProtocolOllama}
			grantID := catalogID(catalogCall(t, r, "POST", catalogPrefix+"/grants", grant, 201))
			grantPath := fmt.Sprintf("%s/grants/%d", catalogPrefix, grantID)
			catalogCall(t, r, "PATCH", grantPath, map[string]any{"protocols": []int{store.DirectProtocolMessages}}, 400)
			resolved, err := oauth.ResolveDirectCredential(context.Background(), db.DB, credentialID, nil, false)
			if err != nil || resolved.Kind != "none" || resolved.AccessToken != "" {
				t.Fatalf("anonymous credential unavailable: %v", err)
			}
			// An endpoint edit cannot invalidate an existing anonymous grant.
			endpoints.Ollama.Auth = "bearer"
			catalogCall(t, r, "PATCH", base, map[string]any{"endpointConfig": endpoints}, 400)
			endpoints.Ollama.Auth = "none"
			// Credential replacement and grant changes share the catalog lock.
			catalogCall(t, r, "PATCH", credentialPath, map[string]any{"kind": "api_key", "apiKey": "fixture-replacement"}, 200)
			catalogCall(t, r, "PATCH", grantPath, map[string]any{"protocols": []int{store.DirectProtocolOllama, store.DirectProtocolMessages}}, 200)
			catalogCall(t, r, "PATCH", credentialPath, map[string]any{"kind": "none"}, 400)
			var kind, secret string
			if err := db.QueryRow(`SELECT kind,secret FROM upstream_credentials WHERE id=?`, credentialID).Scan(&kind, &secret); err != nil || kind != "api_key" || secret != "fixture-replacement" {
				t.Fatal("rejected replacement changed stored credential")
			}
			catalogCall(t, r, "PATCH", grantPath, map[string]any{"protocols": []int{store.DirectProtocolOllama}}, 200)
			catalogCall(t, r, "PATCH", credentialPath, map[string]any{"kind": "none", "name": "Local access renamed"}, 200)
			catalogCall(t, r, "PATCH", credentialPath, map[string]any{"enabled": false}, 200)
			if _, err = oauth.ResolveDirectCredential(context.Background(), db.DB, credentialID, nil, false); err == nil {
				t.Fatal("disabled anonymous credential resolved")
			}
			out := catalogRequest(r, "GET", base+"/credentials", nil)
			if out.Code != 200 || strings.Contains(out.Body.String(), "fixture-") || !strings.Contains(out.Body.String(), `"kind":"none"`) {
				t.Fatalf("invalid credential projection: %s", out.Body.String())
			}
			input["name"], input["provider"] = "Wrong provider", "openai"
			otherID := catalogID(catalogCall(t, r, "POST", catalogPrefix, input, 201))
			catalogCall(t, r, "POST", fmt.Sprintf("%s/%d/credentials", catalogPrefix, otherID), map[string]any{"kind": "none", "name": "Local"}, 400)
		})
	}
}
