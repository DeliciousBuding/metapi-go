package service

import (
	"reflect"
	"slices"
	"testing"
)

func TestUpstreamPresetProductsAndLegacyAliases(t *testing.T) {
	aliases := map[string]string{
		"bailian-claude": "bailian", "codingplan-claude": "codingplan-openai",
		"deepseek-claude": "deepseek-openai", "moonshot-claude": "moonshot-openai", "minimax-claude": "minimax-openai",
		"zhipu-coding-plan-claude": "zhipu-coding-plan-openai", "zai-coding-plan-claude": "zai-coding-plan-openai",
		"doubao-coding-claude": "doubao-coding-openai", "kimi-coding-claude": "kimi-coding-openai", "qiniu-claude": "qiniu-openai",
		"opencode-go-messages": "opencode-go", "ollama-anthropic": "ollama-native", "seedance-video": "doubao-openai",
		"zenmux-video": "zenmux", "codex-alpha-search": "codex",
	}
	items := ListUpstreamPresets()
	seen := map[string]bool{}
	for _, preset := range items {
		if seen[preset.ID] || len(preset.Protocols) == 0 || preset.Name != preset.Label {
			t.Fatalf("duplicate, empty or protocol-labelled product: %+v", preset)
		}
		seen[preset.ID] = true
		if canonical, exists := aliases[preset.ID]; exists {
			t.Fatalf("legacy variant %s still appears alongside %s", preset.ID, canonical)
		}
	}
	for alias, canonical := range aliases {
		old, current := GetUpstreamPreset(alias), GetUpstreamPreset(canonical)
		if old == nil || current == nil || old.ID != canonical || !reflect.DeepEqual(old, current) {
			t.Fatalf("legacy preset %s lost its product: old=%+v current=%+v", alias, old, current)
		}
	}
	for _, pair := range [][2]string{{"bailian", "codingplan-openai"}, {"moonshot-openai", "kimi-coding-openai"}, {"zhipu-openai", "zhipu-coding-plan-openai"}, {"xiaomi-openai", "xiaomi-token-plan-claude"}} {
		if !seen[pair[0]] || !seen[pair[1]] || GetUpstreamPreset(pair[0]).DefaultURL == GetUpstreamPreset(pair[1]).DefaultURL {
			t.Fatalf("distinct API/plan products merged: %v", pair)
		}
	}
}

func TestUpstreamPresetSourceOwnershipAndCredentialModes(t *testing.T) {
	seen := map[string]string{}
	for _, product := range upstreamPresetProducts {
		for _, id := range product.sources {
			if previous := seen[id]; previous != "" {
				t.Fatalf("source %s belongs to both %s and %s", id, previous, product.id)
			}
			seen[id] = product.id
			source := upstreamPresetSource(id)
			if source == nil || !upstreamPresetVariantEndpoints(id, source.Platform).IsConfigured() {
				t.Fatalf("source %s has no executable configuration", id)
			}
			for _, model := range source.RecommendedModels {
				if !slices.Contains(GetUpstreamPreset(product.id).RecommendedModels, model) {
					t.Fatalf("product %s lost model %s", product.id, model)
				}
			}
		}
	}
	for _, item := range ListUpstreamPresets() {
		want := "apiKey"
		if item.ID == "ollama-native" {
			want = "optional"
		}
		if item.ID == "codex" || item.ID == "claude-code" {
			want = "oauth"
		}
		if item.CredentialMode != want || item.RequiresBaseURL != (item.DefaultURL == "") {
			t.Fatalf("credential/base contract drifted: %+v", item)
		}
	}
	if GetUpstreamPreset("not-a-product") != nil || UpstreamPresetEndpointPaths("not-a-product", "openai").IsConfigured() {
		t.Fatal("unknown products silently acquired OpenAI endpoints")
	}
}

func TestUpstreamPresetConfigurationsDoNotShareMutableState(t *testing.T) {
	first := UpstreamPresetEndpointPaths("opencode-go", "opencode")
	first.Chat.URL = "https://changed.invalid/chat"
	first.Chat.ModelWireURLs.Messages = "https://changed.invalid/messages"
	second := UpstreamPresetEndpointPaths("opencode-go-messages", "opencode")
	if second.Chat.URL != "/v1/chat/completions" || second.Chat.ModelWireURLs.Messages != "/v1/messages" {
		t.Fatal("resolving one preset corrupted another caller's defaults")
	}
}
