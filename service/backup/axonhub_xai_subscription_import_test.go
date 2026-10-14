package backup

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func xaiSubscriptionFixture(credentials map[string]any, baseURL string, endpoints []any) []byte {
	channel := map[string]any{
		"id": 1, "type": "xai_subscription", "name": "xAI subscription",
		"base_url": baseURL, "credentials": credentials,
		"supported_models": []string{"grok-4.6"},
	}
	if endpoints != nil {
		channel["endpoints"] = endpoints
	}
	raw, _ := json.Marshal(map[string]any{
		"version":  "1.4",
		"channels": []map[string]any{channel},
		"models": []map[string]any{{
			"id": 1, "model_id": "client-alias", "type": "chat", "status": "enabled",
			"settings": map[string]any{
				"associations": []any{map[string]any{
					"type":         "channel_model",
					"channelModel": map[string]any{"channelId": 1, "modelId": "grok-4.6"},
				}},
			},
		}},
	})
	return raw
}

func xaiSubscriptionOAuth() map[string]any {
	return map[string]any{"oauth": map[string]any{
		"access_token":  "fixture-xai-access",
		"refresh_token": "fixture-xai-refresh",
		"client_id":     "b1a00492-073a-47ea-816f-4c329264a828",
		"token_type":    "bearer",
	}}
}

func compileXaiSubscription(t *testing.T, raw []byte) (*axonHubPlan, error) {
	t.Helper()
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	return CompileAxonHubPlan(src)
}

func TestAxonHubXAISubscriptionImport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := xaiSubscriptionFixture(xaiSubscriptionOAuth(), "https://cli-chat-proxy.grok.com/v1", nil)
	preview, err := PreviewAxonHubV14(db, raw, "xai-subscription")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 {
		t.Fatalf("preview blocked or skipped: %+v", preview)
	}
	if _, err := ImportAxonHubV14(db, raw, "xai-subscription", false); err != nil {
		t.Fatal(err)
	}

	var provider string
	var endpoints store.DirectEndpoints
	if err := db.QueryRow(db.Rebind(`SELECT provider,endpoint_config FROM upstream_channels WHERE name=?`), "xAI subscription").Scan(&provider, &endpoints); err != nil {
		t.Fatal(err)
	}
	if provider != "xai_subscription" ||
		endpoints.Responses == nil ||
		endpoints.Responses.URL != "https://cli-chat-proxy.grok.com/v1/responses" ||
		endpoints.Responses.Profile != "xai-subscription" ||
		endpoints.Responses.Auth != store.DirectAuthBearer {
		t.Fatalf("xAI subscription endpoint: provider=%s endpoints=%+v", provider, endpoints)
	}

	var kind, secret string
	var state store.DirectOAuthState
	if err := db.QueryRow(db.Rebind(`SELECT kind,secret,oauth_state FROM upstream_credentials`)).Scan(&kind, &secret, &state); err != nil {
		t.Fatal(err)
	}
	if kind != store.DirectCredentialOAuth || secret != "fixture-xai-access" ||
		state.RefreshToken != "fixture-xai-refresh" || state.ClientID != "b1a00492-073a-47ea-816f-4c329264a828" {
		t.Fatalf("xAI subscription credential: kind=%s secret=%q state=%+v", kind, secret, state)
	}
}

func TestAxonHubXAISubscriptionRefusesUnrepresentableShapes(t *testing.T) {
	cases := []struct {
		name        string
		credentials map[string]any
		baseURL     string
		endpoints   []any
		want        string
	}{
		{
			name:        "static key",
			credentials: map[string]any{"apiKey": "fixture-static-key"},
			want:        "xai_subscription_oauth_credential_required",
		},
		{
			name:        "missing refresh",
			credentials: map[string]any{"oauth": map[string]any{"access_token": "fixture-xai-access"}},
			want:        "xai_subscription_refresh_token_required",
		},
		{
			name: "wrong client",
			credentials: map[string]any{"oauth": map[string]any{
				"access_token": "fixture-xai-access", "refresh_token": "fixture-refresh",
				"client_id": "source-client", "token_type": "bearer",
			}},
			want: "xai_subscription_client_id_unsupported",
		},
		{
			name:        "non bearer",
			credentials: map[string]any{"oauth": map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh", "token_type": "basic"}},
			want:        "oauth_token_type_unsupported",
		},
		{
			name:        "custom base",
			credentials: xaiSubscriptionOAuth(),
			baseURL:     "https://custom.invalid/v1",
			want:        "xai_subscription_custom_base_url_unsupported",
		},
		{
			name:        "custom endpoint",
			credentials: xaiSubscriptionOAuth(),
			baseURL:     "https://cli-chat-proxy.grok.com/v1",
			endpoints:   []any{map[string]any{"api_format": "openai/responses", "path": "/custom"}},
			want:        "oauth_custom_endpoint_unsupported",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := compileXaiSubscription(t, xaiSubscriptionFixture(tc.credentials, tc.baseURL, tc.endpoints))
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.skipped) != 1 || !slices.Contains(plan.skipped[0].Reasons, tc.want) {
				t.Fatalf("skip reasons=%v want %q", plan.skipped, tc.want)
			}
		})
	}
}
