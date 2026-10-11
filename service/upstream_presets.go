package service

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// UpstreamPreset is one product, including all configured protocols. Legacy
// protocol variants remain resolvable aliases, not separate gallery cards.
type UpstreamPreset struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Label             string   `json:"label"`
	Provider          string   `json:"provider"`
	Platform          string   `json:"platform"`
	Group             string   `json:"group"`
	DefaultURL        string   `json:"defaultUrl"`
	RequiresBaseURL   bool     `json:"requiresBaseUrl"`
	CredentialMode    string   `json:"credentialMode"`
	Protocols         []string `json:"protocols"`
	RecommendedModels []string `json:"recommendedModels"`
	endpoints         store.DirectEndpoints
	endpointBases     map[string]string
	defaultBases      []string
}

type upstreamPresetProduct struct {
	id, name, provider, group, credentialMode string
	sources                                   []string
}

// Membership is explicit: a shared brand or URL prefix does not make an API
// and a subscription plan the same product.
var upstreamPresetProducts = []upstreamPresetProduct{
	{"new-api-connection", "New API", "new-api", "gateway", "apiKey", []string{"new-api-connection"}},
	{"bailian", "Alibaba Bailian", "bailian", "domestic", "apiKey", []string{"bailian", "bailian-claude"}},
	{"deepseek-openai", "DeepSeek", "deepseek", "domestic", "apiKey", []string{"deepseek-openai", "deepseek-claude"}},
	{"moonshot-openai", "Moonshot / Kimi", "moonshot", "domestic", "apiKey", []string{"moonshot-openai", "moonshot-claude"}},
	{"minimax-openai", "MiniMax", "minimax", "domestic", "apiKey", []string{"minimax-openai", "minimax-claude"}},
	{"zhipu-openai", "Zhipu GLM", "zhipu", "domestic", "apiKey", []string{"zhipu-openai", "zhipu-messages"}},
	{"zai-openai", "Z.AI", "zai", "domestic", "apiKey", []string{"zai-openai", "zai-messages"}},
	{"doubao-openai", "Volcengine Ark", "doubao", "domestic", "apiKey", []string{"doubao-openai", "doubao-messages", "seedance-video"}},
	{"longcat", "LongCat", "longcat", "domestic", "apiKey", []string{"longcat", "longcat-messages"}},
	{"xiaomi-openai", "Xiaomi MiMo", "xiaomi", "domestic", "apiKey", []string{"xiaomi-openai"}},
	{"modelscope-openai", "ModelScope", "modelscope", "domestic", "apiKey", []string{"modelscope-openai"}},
	{"codingplan-openai", "Alibaba Bailian Coding Plan", "bailian", "coding", "apiKey", []string{"codingplan-openai", "codingplan-claude"}},
	{"zhipu-coding-plan-openai", "Zhipu Coding Plan", "zhipu", "coding", "apiKey", []string{"zhipu-coding-plan-openai", "zhipu-coding-plan-claude"}},
	{"zai-coding-plan-openai", "Z.AI Coding Plan", "zai", "coding", "apiKey", []string{"zai-coding-plan-openai", "zai-coding-plan-claude"}},
	{"doubao-coding-openai", "Doubao Coding Plan", "doubao", "coding", "apiKey", []string{"doubao-coding-openai", "doubao-coding-claude"}},
	{"kimi-coding-openai", "Kimi Coding Plan", "moonshot", "coding", "apiKey", []string{"kimi-coding-openai", "kimi-coding-claude"}},
	{"xiaomi-token-plan-claude", "MiMo Token Plan", "xiaomi_anthropic", "coding", "apiKey", []string{"xiaomi-token-plan-claude"}},
	{"opencode-go", "OpenCode Go", "opencode_go", "coding", "apiKey", []string{"opencode-go", "opencode-go-messages"}},
	{"cline", "Cline", "cline", "coding", "apiKey", []string{"cline"}},
	{"qiniu-openai", "Qiniu", "qiniu", "gateway", "apiKey", []string{"qiniu-openai", "qiniu-claude"}},
	{"ppio-openai", "PPIO", "ppio", "gateway", "apiKey", []string{"ppio-openai"}},
	{"siliconflow", "SiliconFlow", "siliconflow", "gateway", "apiKey", []string{"siliconflow"}},
	{"openrouter", "OpenRouter", "openrouter", "gateway", "apiKey", []string{"openrouter"}},
	{"nanogpt", "NanoGPT", "nanogpt", "gateway", "apiKey", []string{"nanogpt", "nanogpt-responses"}},
	{"zenmux", "ZenMux", "zenmux", "gateway", "apiKey", []string{"zenmux", "zenmux-responses", "zenmux-messages", "zenmux-gemini", "zenmux-video"}},
	{"ollama-native", "Ollama", "ollama", "other", "optional", []string{"ollama-native", "ollama-anthropic"}},
	{"openai-api", "OpenAI", "openai", "other", "apiKey", []string{"openai-api"}},
	{"anthropic-api", "Anthropic", "anthropic", "other", "apiKey", []string{"anthropic-api"}},
	{"gemini-api", "Google Gemini", "gemini", "other", "apiKey", []string{"gemini-api"}},
	{"xai-api", "xAI", "xai", "other", "apiKey", []string{"xai-api"}},
	{"groq", "Groq", "groq", "other", "apiKey", []string{"groq"}},
	{"fireworks", "Fireworks AI", "fireworks", "other", "apiKey", []string{"fireworks"}},
	{"cerebras", "Cerebras", "cerebras", "other", "apiKey", []string{"cerebras"}},
	{"mistral", "Mistral AI", "mistral", "other", "apiKey", []string{"mistral"}},
	{"jina", "Jina AI", "jina", "other", "apiKey", []string{"jina"}},
	{"bedrock-messages", "Amazon Bedrock", "anthropic_aws", "other", "apiKey", []string{"bedrock-messages"}},
	{"typesafe-systemone", "TypeSafe", "typesafe", "other", "apiKey", []string{"typesafe-systemone"}},
	{"openai-alpha-search", "Alpha Search", "openai", "other", "apiKey", []string{"openai-alpha-search"}},
	{"codex", "Codex", "codex", "other", "oauth", []string{"codex-alpha-search", "codex-responses", "codex-images"}},
	{"claude-code", "Claude Code", "claudecode", "other", "oauth", []string{"claude-code"}},
}

func ListUpstreamPresets() []UpstreamPreset {
	result := make([]UpstreamPreset, 0, len(upstreamPresetProducts))
	for _, product := range upstreamPresetProducts {
		result = append(result, buildUpstreamPreset(product))
	}
	return result
}

func GetUpstreamPreset(id string) *UpstreamPreset {
	id = strings.TrimSpace(id)
	for _, product := range upstreamPresetProducts {
		if id == product.id || slices.Contains(product.sources, id) {
			preset := buildUpstreamPreset(product)
			return &preset
		}
	}
	return nil
}

func buildUpstreamPreset(product upstreamPresetProduct) UpstreamPreset {
	preset := UpstreamPreset{ID: product.id, Name: product.name, Label: product.name, Provider: product.provider, Group: product.group, CredentialMode: product.credentialMode, RecommendedModels: []string{}, endpointBases: map[string]string{}}
	fields := map[string]*store.DirectEndpoint{}
	for i, id := range product.sources {
		source := upstreamPresetSource(id)
		if source == nil {
			continue
		}
		if i == 0 {
			preset.DefaultURL, preset.Platform = source.DefaultURL, source.Platform
		}
		preset.defaultBases = append(preset.defaultBases, source.DefaultURL)
		for _, model := range source.RecommendedModels {
			if !slices.Contains(preset.RecommendedModels, model) {
				preset.RecommendedModels = append(preset.RecommendedModels, model)
			}
		}
		for _, entry := range upstreamPresetVariantEndpoints(id, source.Platform).Entries() {
			if entry.Endpoint != nil {
				fields[entry.Key] = entry.Endpoint
				preset.endpointBases[entry.Key] = source.DefaultURL
			}
		}
	}
	// Keep store.Entries as the field-to-protocol owner instead of duplicating
	// its field list for merging native and site-derived endpoint contracts.
	raw, _ := json.Marshal(fields)
	_ = json.Unmarshal(raw, &preset.endpoints)
	for _, entry := range preset.endpoints.Entries() {
		if entry.Endpoint != nil {
			preset.Protocols = append(preset.Protocols, entry.Key)
		}
	}
	preset.RequiresBaseURL = preset.DefaultURL == ""
	return preset
}

func (p UpstreamPreset) EndpointBaseURL(key string) string { return p.endpointBases[key] }

// IsDefaultBaseURL recognizes legacy variant roots as product defaults, so an
// old Messages preset cannot accidentally become the new Chat endpoint root.
func (p UpstreamPreset) IsDefaultBaseURL(base string) bool {
	normalize := func(value string) string {
		return strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(value), "/"), "/v1")
	}
	for _, original := range p.defaultBases {
		if original != "" && normalize(original) == normalize(base) {
			return true
		}
	}
	return false
}

func upstreamPresetSource(id string) *UpstreamPreset {
	if source := GetSiteInitializationPreset(id); source != nil {
		return &UpstreamPreset{ID: id, Platform: source.Platform, DefaultURL: source.DefaultURL, RecommendedModels: source.RecommendedModels}
	}
	native := map[string]UpstreamPreset{
		"new-api-connection":   {Platform: "new-api"},
		"opencode-go":          {Platform: "opencode", DefaultURL: "https://opencode.ai/zen/go"},
		"opencode-go-messages": {Platform: "opencode", DefaultURL: "https://opencode.ai/zen/go"},
		"cline":                {Platform: "cline", DefaultURL: "https://api.cline.bot/api/v1"},
		"zhipu-messages":       {Platform: "claude", DefaultURL: "https://open.bigmodel.cn/api/anthropic"},
		"zai-messages":         {Platform: "claude", DefaultURL: "https://api.z.ai/api/anthropic"},
		"doubao-messages":      {Platform: "claude", DefaultURL: "https://ark.cn-beijing.volces.com/api/compatible"},
		"seedance-video":       {Platform: "doubao", DefaultURL: GetSiteInitializationPreset("doubao-openai").DefaultURL},
		"longcat":              {Platform: "openai", DefaultURL: "https://api.longcat.chat/openai/v1", RecommendedModels: []string{"LongCat-Flash-Chat", "LongCat-Flash-Thinking"}},
		"longcat-messages":     {Platform: "claude", DefaultURL: "https://api.longcat.chat/anthropic"},
		"nanogpt":              {Platform: "openai", DefaultURL: "https://nano-gpt.com/api/v1"},
		"nanogpt-responses":    {Platform: "openai", DefaultURL: "https://nano-gpt.com/api/v1"},
		"zenmux":               {Platform: "openai", DefaultURL: "https://zenmux.ai/api/v1"},
		"zenmux-responses":     {Platform: "openai", DefaultURL: "https://zenmux.ai/api/v1"},
		"zenmux-messages":      {Platform: "claude", DefaultURL: "https://zenmux.ai/api/anthropic"},
		"zenmux-gemini":        {Platform: "gemini", DefaultURL: "https://zenmux.ai/api/vertex-ai"},
		"zenmux-video":         {Platform: "zenmux", DefaultURL: "https://zenmux.ai/api/v1"},
		"ollama-native":        {Platform: "ollama", DefaultURL: "http://localhost:11434"},
		"ollama-anthropic":     {Platform: "ollama", DefaultURL: "http://localhost:11434"},
		"bedrock-messages":     {Platform: "bedrock", DefaultURL: "https://bedrock-runtime.us-east-1.amazonaws.com"},
		"typesafe-systemone":   {Platform: "typesafe", DefaultURL: "https://api.typesafe.ai/v1", RecommendedModels: []string{"jev-latest", "jev-preview"}},
		"openai-alpha-search":  {Platform: "openai"},
		"codex-alpha-search":   {Platform: "codex", DefaultURL: "https://chatgpt.com/backend-api/codex"},
		"codex-responses":      {Platform: "codex", DefaultURL: "https://chatgpt.com/backend-api/codex"},
		"codex-images":         {Platform: "codex", DefaultURL: "https://chatgpt.com/backend-api/codex"},
		"claude-code":          {Platform: "claudecode", DefaultURL: "https://api.anthropic.com"},
	}
	source, ok := native[id]
	if !ok {
		return nil
	}
	source.ID = id
	return &source
}
