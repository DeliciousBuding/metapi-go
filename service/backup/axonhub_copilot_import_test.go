package backup

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func copilotFixturePayload(credentials map[string]any, endpoints []any) []byte {
	channel := map[string]any{
		"id": 1, "type": "github_copilot", "name": "copilot",
		"credentials":      credentials,
		"supported_models": []string{"gpt-5.4"},
		"settings": map[string]any{
			"modelProtocols": []any{map[string]any{
				"model": "copilot", "apiFormats": []string{"openai/chat_completions"},
			}},
		},
	}
	if endpoints != nil {
		channel["endpoints"] = endpoints
	}
	models := []map[string]any{
		{"id": 1, "model_id": "gpt-5.4", "type": "chat", "settings": map[string]any{
			"associations": []any{map[string]any{
				"type":         "channel_model",
				"channelModel": map[string]any{"channelId": 1, "modelId": "gpt-5.4"},
			}},
		}},
	}
	raw, _ := json.Marshal(map[string]any{
		"version": "1.4", "channels": []map[string]any{channel}, "models": models,
	})
	return raw
}

func TestAxonHubCopilotImport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := copilotFixturePayload(map[string]any{
		"oauth": map[string]any{
			"access_token": "fixture-github-access",
			"token_type":   "bearer",
		},
	}, nil)

	preview, err := PreviewAxonHubV14(db, raw, "copilot")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 {
		t.Fatalf("copilot preview: %+v", preview)
	}
	if _, err := ImportAxonHubV14(db, raw, "copilot", false); err != nil {
		t.Fatal(err)
	}

	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name='copilot'`)); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat == nil ||
		endpoints.Chat.URL != "https://api.githubcopilot.com/chat/completions" ||
		endpoints.Chat.Profile != "github-copilot" ||
		endpoints.Chat.Auth != store.DirectAuthBearer {
		t.Fatalf("copilot endpoint: %+v", endpoints.Chat)
	}

	var kind, secret, oauthState string
	if err := db.QueryRow(db.Rebind(`SELECT kind, secret, oauth_state FROM upstream_credentials`)).Scan(&kind, &secret, &oauthState); err != nil {
		t.Fatal(err)
	}
	if kind != store.DirectCredentialOAuth || secret != "" || !strings.Contains(oauthState, "fixture-github-access") {
		t.Fatalf("credential: kind=%s secret=%q state=%s", kind, secret, oauthState)
	}
}

func TestAxonHubCopilotImportNegativeCases(t *testing.T) {
	db := openAxonHubTestDB(t)

	raw := copilotFixturePayload(map[string]any{"apiKey": "fixture-static-key"}, nil)
	preview, err := PreviewAxonHubV14(db, raw, "copilot-static")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 1 || !slices.Contains(preview.SkippedChannels[0].Reasons, "copilot_oauth_credential_required") {
		t.Fatalf("static Copilot key must be refused: %+v", preview.SkippedChannels)
	}

	raw = copilotFixturePayload(map[string]any{
		"oauth": map[string]any{"access_token": "fixture-github-access", "token_type": "bearer"},
	}, []any{map[string]any{"api_format": "openai/chat_completions", "path": "/custom"}})
	preview, err = PreviewAxonHubV14(db, raw, "copilot-custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.SkippedChannels) != 1 || !slices.Contains(preview.SkippedChannels[0].Reasons, "oauth_custom_endpoint_unsupported") {
		t.Fatalf("custom Copilot endpoint must be refused: %+v", preview.SkippedChannels)
	}
}
