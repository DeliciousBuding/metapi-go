package service

import (
	"cmp"
	"slices"
	"strings"
)

// UpstreamPreset reuses site defaults for shared providers. Native-only formats
// are listed separately because the site adapters cannot execute those formats.
type UpstreamPreset struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Label             string   `json:"label"`
	Provider          string   `json:"provider"`
	Platform          string   `json:"platform"`
	Group             string   `json:"group"`
	DefaultURL        string   `json:"defaultUrl"`
	Protocols         []string `json:"protocols"`
	RecommendedModels []string `json:"recommendedModels"`
}

func upstreamPresetProtocols(id, platform string) []string {
	var protocols []string
	for _, entry := range UpstreamPresetEndpointPaths(id, platform).Entries() {
		if entry.Endpoint != nil {
			protocols = append(protocols, entry.Key)
		}
	}
	return protocols
}

// The set is an executable capability boundary. A new site discovery preset
// does not silently become a direct-upstream contract. Hosts remain in the
// site registry; only source IDs with known executable contracts live here.
var upstreamPresetProviders = map[string]string{
	"bailian": "bailian", "bailian-claude": "bailian_anthropic",
	"codingplan-openai": "bailian", "codingplan-claude": "bailian_anthropic",
	"zhipu-openai": "zhipu", "zhipu-coding-plan-openai": "zhipu", "zhipu-coding-plan-claude": "zhipu_anthropic",
	"zai-openai": "zai", "zai-coding-plan-openai": "zai", "zai-coding-plan-claude": "zai_anthropic",
	"doubao-openai": "doubao", "doubao-coding-openai": "doubao", "doubao-coding-claude": "doubao_anthropic",
	"kimi-coding-openai": "moonshot", "kimi-coding-claude": "moonshot_coding",
	"xiaomi-openai": "xiaomi", "xiaomi-token-plan-claude": "xiaomi_anthropic",
	"ppio-openai": "ppio", "qiniu-openai": "qiniu", "qiniu-claude": "qiniu_anthropic",
	"deepseek-openai": "deepseek", "deepseek-claude": "deepseek_anthropic",
	"moonshot-openai": "moonshot", "moonshot-claude": "moonshot_anthropic",
	"minimax-openai": "minimax", "minimax-claude": "minimax_anthropic",
	"modelscope-openai": "modelscope", "siliconflow": "siliconflow",
	"jina":       "jina",
	"openrouter": "openrouter", "groq": "groq", "fireworks": "fireworks", "cerebras": "cerebras", "mistral": "mistral",
	"openai-api": "openai", "anthropic-api": "anthropic", "gemini-api": "gemini", "xai-api": "xai",
}

func ListUpstreamPresets() []UpstreamPreset {
	groups := map[string][]UpstreamPreset{}
	for _, preset := range ListSiteInitializationPresets() {
		provider, ok := upstreamPresetProviders[preset.ID]
		if !ok {
			continue
		}
		group := "domestic"
		switch {
		case strings.Contains(preset.ID, "coding") || strings.Contains(preset.ID, "token-plan"):
			group = "coding"
		case preset.ID == "openrouter" || preset.ID == "siliconflow" || preset.ID == "ppio-openai" || preset.ID == "qiniu-openai" || preset.ID == "qiniu-claude":
			group = "gateway"
		case preset.ID == "openai-api" || preset.ID == "anthropic-api" || preset.ID == "gemini-api" || preset.ID == "xai-api" || preset.ID == "groq" || preset.ID == "mistral" || preset.ID == "fireworks" || preset.ID == "cerebras" || preset.ID == "jina":
			group = "other"
		}
		groups[group] = append(groups[group], UpstreamPreset{ID: preset.ID, Name: preset.ProviderLabel, Label: preset.Label, Provider: provider, Platform: preset.Platform, Group: group, DefaultURL: preset.DefaultURL, Protocols: upstreamPresetProtocols(preset.ID, preset.Platform), RecommendedModels: append([]string{}, preset.RecommendedModels...)})
	}
	for _, preset := range nativeUpstreamPresets() {
		preset.Protocols = upstreamPresetProtocols(preset.ID, preset.Platform)
		groups[preset.Group] = append(groups[preset.Group], preset)
	}
	result := []UpstreamPreset{{ID: "new-api-connection", Name: "New API", Label: "New API", Provider: "new-api", Platform: "new-api", Group: "gateway", Protocols: upstreamPresetProtocols("new-api-connection", "new-api"), RecommendedModels: []string{}}}
	for _, group := range []string{"domestic", "coding", "gateway", "other"} {
		presets := groups[group]
		familyOrder := map[string]int{}
		for _, preset := range presets {
			family := strings.Split(preset.Provider, "_")[0]
			if _, exists := familyOrder[family]; !exists {
				familyOrder[family] = len(familyOrder)
			}
		}
		slices.SortStableFunc(presets, func(a, b UpstreamPreset) int {
			order := cmp.Compare(familyOrder[strings.Split(a.Provider, "_")[0]], familyOrder[strings.Split(b.Provider, "_")[0]])
			if order != 0 {
				return order
			}
			// Keep product variants adjacent, with Chat before Messages.
			if a.Platform == "openai" && b.Platform != "openai" {
				return -1
			}
			if b.Platform == "openai" && a.Platform != "openai" {
				return 1
			}
			return 0
		})
		result = append(result, presets...)
	}
	return result
}

func nativeUpstreamPresets() []UpstreamPreset {
	ark := GetSiteInitializationPreset("doubao-openai")
	return []UpstreamPreset{
		{ID: "seedance-video", Name: "Seedance", Label: "Seedance", Provider: "doubao", Platform: "doubao", Group: "domestic", DefaultURL: ark.DefaultURL, RecommendedModels: []string{}},
		{ID: "zenmux-video", Name: "ZenMux", Label: "ZenMux Video", Provider: "zenmux_video", Platform: "zenmux", Group: "gateway", DefaultURL: "https://zenmux.ai/api/v1", RecommendedModels: []string{}},
		{ID: "ollama-native", Name: "Ollama", Label: "Ollama", Provider: "ollama", Platform: "ollama", Group: "other", DefaultURL: "http://localhost:11434", RecommendedModels: []string{}},
		{ID: "ollama-anthropic", Name: "Ollama", Label: "Ollama / Messages", Provider: "ollama_anthropic", Platform: "ollama", Group: "other", DefaultURL: "http://localhost:11434", RecommendedModels: []string{}},
		{ID: "bedrock-messages", Name: "Amazon Bedrock", Label: "Amazon Bedrock", Provider: "anthropic_aws", Platform: "bedrock", Group: "other", DefaultURL: "https://bedrock-runtime.us-east-1.amazonaws.com", RecommendedModels: []string{}},
		{ID: "typesafe-systemone", Name: "TypeSafe", Label: "TypeSafe System One", Provider: "typesafe", Platform: "typesafe", Group: "other", DefaultURL: "https://api.typesafe.ai/v1", RecommendedModels: []string{"jev-latest", "jev-preview"}},
		{ID: "openai-alpha-search", Name: "Alpha Search", Label: "Alpha Search", Provider: "openai", Platform: "openai", Group: "other", RecommendedModels: []string{}},
		{ID: "codex-alpha-search", Name: "Codex", Label: "Codex Alpha Search", Provider: "codex", Platform: "codex", Group: "other", DefaultURL: "https://chatgpt.com/backend-api/codex", RecommendedModels: []string{}},
	}
}

func GetUpstreamPreset(id string) *UpstreamPreset {
	for _, preset := range ListUpstreamPresets() {
		if preset.ID == id {
			return &preset
		}
	}
	return nil
}
