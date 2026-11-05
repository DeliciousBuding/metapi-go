package backup

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func antigravityImportFixture(material, base string) []byte {
	raw, _ := json.Marshal(map[string]any{
		"version": "1.4",
		"channels": []any{map[string]any{
			"id": 1, "type": "antigravity", "name": "synthetic",
			"base_url": base, "credentials": map[string]any{"apiKey": material},
			"supported_models": []string{"gemini-3-pro"},
		}},
		"models": []any{map[string]any{
			"id": 1, "model_id": "public-model", "settings": map[string]any{
				"associations": []any{map[string]any{"type": "channel_model",
					"channelModel": map[string]any{"channelId": 1, "modelId": "gemini-3-pro"}}},
			},
		}},
	})
	return raw
}

func TestAxonHubAntigravityImportPreviewReimportAndResidualBoundaries(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := antigravityImportFixture("synthetic-refresh|synthetic-project", "")
	preview, err := PreviewAxonHubV14(db, raw, "antigravity-fixture")
	if err != nil || len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 {
		t.Fatalf("preview failed: %v", err)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "synthetic-refresh") || strings.Contains(string(encoded), "synthetic-project") {
		t.Fatal("preview exposed private credential state")
	}
	for i := 0; i < 2; i++ {
		if _, err := ImportAxonHubV14(db, raw, "antigravity-fixture", false); err != nil {
			t.Fatal(err)
		}
		for _, table := range []string{"upstream_channels", "upstream_credentials", "upstream_grants", "upstream_group_items"} {
			if got := countRows(t, db, "SELECT COUNT(*) FROM "+table); got != 1 {
				t.Fatalf("%s has %d rows after reimport", table, got)
			}
		}
	}
	var state store.DirectOAuthState
	var kind, secret string
	if err := db.QueryRow(`SELECT kind,secret,oauth_state FROM upstream_credentials`).Scan(&kind, &secret, &state); err != nil {
		t.Fatal(err)
	}
	if kind != store.DirectCredentialOAuth || secret != "" || state.ProjectID != "synthetic-project" || state.RefreshToken != "synthetic-refresh" {
		t.Fatal("persistent credential changed its meaning")
	}
	for _, tc := range []struct {
		name, material, base, reason string
	}{
		{"no project", "synthetic-refresh", "", "antigravity_refresh_token_project_required"},
		{"foreign origin", "synthetic-refresh|synthetic-project", "https://foreign.invalid", "antigravity_origin_unsupported"},
		{"nested path", "synthetic-refresh|synthetic-project", "https://cloudcode-pa.googleapis.com/other", "antigravity_origin_unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := PreviewAxonHubV14(db, antigravityImportFixture(tc.material, tc.base), "other-origin")
			if err != nil || len(p.SkippedChannels) != 1 || !slices.Contains(p.SkippedChannels[0].Reasons, tc.reason) {
				t.Fatalf("invalid source not refused: %+v err=%v", p, err)
			}
		})
	}
}

func TestAxonHubTestExecutorsRemainExplicitAndDoNotBlockRealChannels(t *testing.T) {
	var source map[string]any
	_ = json.Unmarshal(antigravityImportFixture("synthetic-refresh|synthetic-project", ""), &source)
	channels := source["channels"].([]any)
	for i, provider := range []string{"openai_fake", "anthropic_fake"} {
		channels = append(channels, map[string]any{"id": i + 2, "type": provider,
			"name": "synthetic test executor", "base_url": "https://test.invalid",
			"credentials": map[string]any{"apiKey": "synthetic-not-supply"}, "supported_models": []string{"fixture"}})
	}
	source["channels"] = channels
	raw, _ := json.Marshal(source)
	db := openAxonHubTestDB(t)
	for i := 0; i < 2; i++ {
		preview, err := PreviewAxonHubV14(db, raw, "mixed-fixture")
		if err != nil || len(preview.SkippedChannels) != 2 {
			t.Fatalf("wrong mixed preview: %+v err=%v", preview, err)
		}
		for _, skipped := range preview.SkippedChannels {
			if !slices.Contains(skipped.Reasons, "provider_test_executor_not_imported") {
				t.Errorf("test executor missing explicit reason: %+v", skipped)
			}
		}
		if _, err := ImportAxonHubV14(db, raw, "mixed-fixture", false); err != nil {
			t.Fatal(err)
		}
		if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants`) != 1 ||
			countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE provider LIKE '%fake'`) != 0 {
			t.Fatal("fake executor became a supply grant")
		}
	}
}

func TestAxonHubAntigravityImportPreservesProjectAndLazyOAuth(t *testing.T) {
	src, err := ParseAxonHubSource(antigravityImportFixture("synthetic-refresh|synthetic-project", ""))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil || len(plan.channels) != 1 || len(plan.credentials) != 1 || len(plan.grants) != 1 {
		t.Fatalf("Antigravity did not compile: plan=%+v err=%v", plan, err)
	}
	credential := plan.credentials[0]
	if credential.Kind != store.DirectCredentialOAuth || credential.Secret != "" ||
		credential.OAuthState.RefreshToken != "synthetic-refresh" || credential.OAuthState.ProjectID != "synthetic-project" {
		t.Fatalf("durable material was not separated from access token")
	}
	ep := plan.channels[0].Endpoints.Gemini
	if ep == nil || ep.Profile != "antigravity" || ep.Auth != store.DirectAuthBearer || ep.URL != "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal" {
		t.Fatalf("wrong selected Antigravity endpoint: %+v", ep)
	}
	if !strings.Contains(strings.Join(plan.residuals, "\n"), "source_endpoint_fallback_not_imported") {
		t.Fatal("source multi-origin fallback must remain explicit")
	}
}
