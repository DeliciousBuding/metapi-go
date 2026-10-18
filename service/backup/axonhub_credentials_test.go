package backup

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubOAuthCredentialsAreTypedAndPrivate(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "structured", true: "legacy"}[legacy], func(t *testing.T) {
			credentials := map[string]any{"access_token": "fixture-access-secret", "refresh_token": "fixture-refresh-secret", "client_id": "source-client", "expires_at": "2026-01-01T00:00:00Z", "token_type": "bearer"}
			var credentialField map[string]any
			if legacy {
				raw, _ := json.Marshal(credentials)
				credentialField = map[string]any{"apiKey": string(raw)}
			} else {
				credentialField = map[string]any{"oauth": credentials}
			}
			body, _ := json.Marshal(map[string]any{"version": "1.4", "channels": []any{map[string]any{"id": 1, "type": "claudecode", "name": "fixture", "base_url": "https://api.example.invalid", "credentials": credentialField, "supported_models": []string{"model"}}}, "models": []any{map[string]any{"id": 1, "model_id": "model", "settings": map[string]any{"associations": []any{map[string]any{"type": "channel_model", "channelModel": map[string]any{"channelId": 1, "modelId": "model"}}}}}}})
			src, err := ParseAxonHubSource(body)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := CompileAxonHubPlan(src)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.channels) != 1 || len(plan.credentials) != 1 {
				t.Fatalf("missing OAuth channel: %+v", plan.skipped)
			}
			credential := plan.credentials[0]
			if credential.Kind != store.DirectCredentialOAuth || credential.Secret != "fixture-access-secret" || credential.OAuthState.RefreshToken != "fixture-refresh-secret" {
				t.Fatal("OAuth credential was not typed")
			}
			if plan.channels[0].Endpoints.Messages.Profile != "claudecode" || plan.channels[0].Endpoints.Messages.Auth != store.DirectAuthBearer {
				t.Fatal("missing Claude wire contract")
			}
			db := openAxonHubTestDB(t)
			preview, err := PreviewAxonHubV14(db, body, "fixture-oauth")
			if err != nil {
				t.Fatal(err)
			}
			previewJSON, _ := json.Marshal(preview)
			if strings.Contains(string(previewJSON), "fixture-access-secret") || strings.Contains(string(previewJSON), "fixture-refresh-secret") {
				t.Fatal("preview leaked OAuth material")
			}
			if _, err := ImportAxonHubV14(db, body, "fixture-oauth", false); err != nil {
				t.Fatal(err)
			}
			var kind, secret string
			var state store.DirectOAuthState
			if err := db.QueryRow(`SELECT kind,secret,oauth_state FROM upstream_credentials`).Scan(&kind, &secret, &state); err != nil {
				t.Fatal(err)
			}
			if kind != "oauth" || strings.HasPrefix(secret, "{") || state.ClientID != "source-client" {
				t.Fatal("stored JSON as bearer or lost source client")
			}
		})
	}
}

func TestAxonHubManagementAPIKeyDoesNotDropChannel(t *testing.T) {
	// managementApiKey only feeds provider quota polling (itself a residual)
	// and is never attached to inference requests, so the channel stays
	// importable and the omission is reported instead of skipping it.
	body, _ := json.Marshal(map[string]any{
		"version": "1.4",
		"channels": []any{map[string]any{
			"id": 1, "type": "openai", "name": "management",
			"base_url": "https://api.example.invalid/v1",
			"credentials": map[string]any{
				"apiKey":           "fixture-inference-key",
				"managementApiKey": "fixture-console-key",
			},
			"supported_models": []string{"model"},
		}},
		"models": []any{map[string]any{
			"id": 1, "model_id": "model", "type": "chat",
			"settings": map[string]any{"associations": []any{map[string]any{
				"type":         "channel_model",
				"channelModel": map[string]any{"channelId": 1, "modelId": "model"},
			}}},
		}},
	})
	src, err := ParseAxonHubSource(body)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.channels) != 1 || len(plan.skipped) != 0 {
		t.Fatalf("management key dropped the channel: %+v", plan.skipped)
	}
	if !containsString(plan.residuals, "channel-1:management_api_key_not_imported") {
		t.Fatalf("management key omission not reported: %v", plan.residuals)
	}
	if len(plan.credentials) != 1 || plan.credentials[0].Secret != "fixture-inference-key" {
		t.Fatalf("inference credential changed: %+v", plan.credentials)
	}
}

func TestAxonHubCodexAndFennoWireProfile(t *testing.T) {
	for _, provider := range []string{"codex", "fenno"} {
		channel := AxonHubSourceChannel{ID: 1, Type: provider, Credentials: AxonHubSourceCredentials{APIKey: "fixture-static-key"}, SupportedModels: []string{"model"}}
		compiled, reasons, _ := compileAxonHubChannel(channel)
		if len(reasons) > 0 || compiled == nil || compiled.Endpoints.Responses.Profile != "codex" || compiled.Endpoints.Responses.URL != "https://chatgpt.com/backend-api/codex/responses" {
			t.Fatalf("%s: reasons=%v", provider, reasons)
		}
	}
}
