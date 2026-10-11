package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func presetRouter() *chi.Mux {
	r := chi.NewRouter()
	RegisterUpstreamPresetRoutes(r)
	return r
}

func resolvePreset(t *testing.T, r http.Handler, id, base string) (string, store.DirectEndpoints) {
	t.Helper()
	out := catalogRequest(r, "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": id, "baseUrl": base})
	if out.Code != http.StatusOK {
		t.Fatalf("resolve %s: %d %s", id, out.Code, out.Body.String())
	}
	var result struct {
		Provider  string                `json:"provider"`
		Endpoints store.DirectEndpoints `json:"endpointConfig"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Provider, result.Endpoints
}

func TestUpstreamPresetListSharesSiteRegistry(t *testing.T) {
	// Register and serve without a DB or service container: this API projects
	// local metadata and does not probe a provider or create an account.
	out := catalogRequest(presetRouter(), "GET", catalogPrefix+"/presets", nil)
	if out.Code != http.StatusOK {
		t.Fatalf("list: %d %s", out.Code, out.Body.String())
	}
	var result struct {
		Items []service.UpstreamPreset `json:"items"`
	}
	if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) < 10 || result.Items[0].ID != "new-api-connection" || result.Items[0].DefaultURL != "" {
		t.Fatalf("missing prioritized New API entry: %+v", result.Items)
	}
	seen := map[string]bool{}
	for _, preset := range result.Items {
		if seen[preset.ID] || preset.Label != preset.Name || len(preset.Protocols) == 0 {
			t.Fatalf("duplicate or unconfigured product: %+v", preset)
		}
		seen[preset.ID] = true
		if preset.RequiresBaseURL != (preset.DefaultURL == "") || !slices.Contains([]string{"apiKey", "optional", "oauth"}, preset.CredentialMode) {
			t.Fatalf("invalid quick-connect metadata: %+v", preset)
		}
		source := service.GetSiteInitializationPreset(preset.ID)
		if source == nil {
			continue
		}
		if source.DefaultURL != preset.DefaultURL || source.Platform != preset.Platform {
			t.Errorf("preset %s drifted from site registry", preset.ID)
		}
		for _, model := range source.RecommendedModels {
			if !slices.Contains(preset.RecommendedModels, model) {
				t.Errorf("preset %s lost recommended model %s", preset.ID, model)
			}
		}
		if preset.ID == "modelscope-claude" {
			t.Fatal("unsupported Messages contract exposed")
		}
	}
}

func TestUpstreamPresetEndpointContracts(t *testing.T) {
	cases := []struct {
		id, provider, key, url, auth, profile string
		modelPath                             bool
	}{
		{"opencode-go", "opencode_go", "chat", "https://opencode.ai/zen/go/v1/chat/completions", "bearer", "opencode-go", false},
		{"opencode-go-messages", "opencode_go", "messages", "https://opencode.ai/zen/go/v1/messages", "x-api-key", "", false},
		{"cline", "cline", "chat", "https://api.cline.bot/api/v1/chat/completions", "bearer", "cline", false},
		{"ollama-native", "ollama", "ollama", "http://localhost:11434/api/chat", "none", "ollama", false},
		{"ollama-anthropic", "ollama", "messages", "http://localhost:11434/v1/messages", "none", "ollama-messages", false},
		{"bedrock-messages", "anthropic_aws", "messages", "https://bedrock-runtime.us-east-1.amazonaws.com/model", "bearer", "bedrock", true},
		{"seedance-video", "doubao", "seedanceVideo", "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks", "bearer", "seedance-video", false},
		{"zenmux-video", "zenmux", "zenmuxVideo", "https://zenmux.ai/api/v1/videos", "bearer", "zenmux-video", false},
		{"typesafe-systemone", "typesafe", "systemOne", "https://api.typesafe.ai/v1/systemone", "bearer", "", false},
		{"codex-alpha-search", "codex", "alphaSearch", "https://chatgpt.com/backend-api/codex/alpha/search", "bearer", "codex-alpha-search", false},
		{"deepseek-openai", "deepseek", "chat", "https://api.deepseek.com/v1/chat/completions", "bearer", "deepseek", false},
		{"zhipu-coding-plan-openai", "zhipu", "chat", "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", "bearer", "zai", false},
		{"doubao-openai", "doubao", "chat", "https://ark.cn-beijing.volces.com/api/v3/chat/completions", "bearer", "", false},
		{"codingplan-claude", "bailian", "messages", "https://coding.dashscope.aliyuncs.com/apps/anthropic/v1/messages", "x-api-key", "", false},
		{"xiaomi-token-plan-claude", "xiaomi_anthropic", "messages", "https://token-plan-cn.xiaomimimo.com/anthropic/v1/messages", "x-api-key", "", false},
		{"gemini-api", "gemini", "gemini", "https://generativelanguage.googleapis.com/v1beta/models", "x-goog-api-key", "", true},
		{"openai-api", "openai", "responses", "https://api.openai.com/v1/responses", "bearer", "", false},
		{"bailian", "bailian", "responses", "https://dashscope.aliyuncs.com/compatible-mode/v1/responses", "bearer", "", false},
		{"bailian", "bailian", "messages", "https://dashscope.aliyuncs.com/apps/anthropic/v1/messages", "x-api-key", "", false},
		{"longcat", "longcat", "chat", "https://api.longcat.chat/openai/v1/chat/completions", "bearer", "longcat", false},
		{"longcat", "longcat", "messages", "https://api.longcat.chat/anthropic/v1/messages", "bearer", "", false},
		{"nanogpt", "nanogpt", "responses", "https://nano-gpt.com/api/v1/responses", "bearer", "", false},
		{"openrouter", "openrouter", "imageGeneration", "https://openrouter.ai/api/v1/images", "bearer", "openrouter-image", false},
		{"openrouter", "openrouter", "imageEdit", "https://openrouter.ai/api/v1/images", "bearer", "openrouter-image", false},
		{"zenmux", "zenmux", "messages", "https://zenmux.ai/api/anthropic/v1/messages", "x-api-key", "", false},
		{"zenmux", "zenmux", "gemini", "https://zenmux.ai/api/vertex-ai/v1beta/models", "x-goog-api-key", "", true},
		{"jina", "jina", "jinaEmbeddings", "https://api.jina.ai/v1/embeddings", "bearer", "jina-embeddings", false},
		{"minimax-openai", "minimax", "imageGeneration", "https://api.minimaxi.com/v1/image_generation", "bearer", "minimax-image", false},
		{"modelscope-openai", "modelscope", "modelscopeImageGeneration", "https://api-inference.modelscope.cn/v1/images/generations", "bearer", "modelscope-image", false},
		{"gemini-api", "gemini", "geminiEmbeddings", "https://generativelanguage.googleapis.com/v1beta/models", "x-goog-api-key", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			preset := service.GetUpstreamPreset(tc.id)
			provider, config := resolvePreset(t, presetRouter(), tc.id, preset.DefaultURL)
			var endpoint *store.DirectEndpoint
			for _, entry := range config.Entries() {
				if entry.Key == tc.key {
					endpoint = entry.Endpoint
				}
			}
			want := store.DirectEndpoint{URL: tc.url, Auth: tc.auth, Profile: tc.profile, ModelPath: tc.modelPath}
			if tc.id == "opencode-go" {
				want.ModelWireURLs = &store.DirectModelWireURLs{Responses: "https://opencode.ai/zen/go/v1/responses", Messages: "https://opencode.ai/zen/go/v1/messages"}
			}
			if provider != tc.provider || endpoint == nil || !reflect.DeepEqual(*endpoint, want) {
				t.Fatalf("provider=%s endpoint=%+v; want %s %+v", provider, endpoint, tc.provider, want)
			}
		})
	}
}

func TestUpstreamPresetAdvertisedProtocolsMatchEndpoints(t *testing.T) {
	contracts := map[string][]string{
		"opencode-go":          {"chat", "responses", "messages"},
		"opencode-go-messages": {"chat", "responses", "messages"},
		"cline":                {"chat"},
		"new-api-connection":   {"chat", "responses", "messages", "gemini", "completions", "embeddings", "rerank", "imageGeneration", "imageEdit", "audioSpeech", "audioTranscription", "audioTranslation", "moderations", "video"},
		"deepseek-openai":      {"chat", "messages", "completions"},
		"codingplan-claude":    {"chat", "messages"},
		"codingplan-openai":    {"chat", "messages"},
		"bailian":              {"chat", "responses", "messages"},
		"gemini-api":           {"gemini", "geminiEmbeddings"},
		"openai-api":           {"chat", "responses", "embeddings", "imageGeneration", "imageEdit", "imageVariation", "audioSpeech", "audioTranscription", "audioTranslation", "moderations", "video"},
		"xai-api":              {"chat", "responses"},
		"jina":                 {"rerank", "jinaEmbeddings"},
		"minimax-openai":       {"chat", "messages", "imageGeneration"},
		"longcat":              {"chat", "messages"},
		"nanogpt":              {"chat", "responses", "embeddings", "imageGeneration", "imageEdit", "imageVariation", "audioSpeech", "audioTranscription", "audioTranslation", "moderations", "video"},
		"ollama-native":        {"messages", "ollama"},
		"codex":                {"responses", "imageGeneration", "imageEdit", "alphaSearch"},
		"modelscope-openai":    {"chat", "modelscopeImageGeneration"},
	}
	for id, want := range contracts {
		t.Run(id, func(t *testing.T) {
			preset := service.GetUpstreamPreset(id)
			if !slices.Equal(preset.Protocols, want) {
				t.Fatalf("advertised protocols=%v; want %v", preset.Protocols, want)
			}
			_, endpoints := resolvePreset(t, presetRouter(), id, "https://relay.example.com")
			for _, entry := range endpoints.Entries() {
				if (entry.Endpoint != nil) != slices.Contains(want, entry.Key) {
					t.Errorf("%s endpoint=%+v; advertised protocols=%v", entry.Key, entry.Endpoint, want)
				}
			}
		})
	}
}

func TestUpstreamPresetCustomHostKeepsSelectedProductProfile(t *testing.T) {
	for _, id := range []string{"deepseek-openai", "zhipu-coding-plan-openai", "new-api-connection"} {
		t.Run(id, func(t *testing.T) {
			_, config := resolvePreset(t, presetRouter(), id, " https://relay.example.com/custom/v1/ ")
			profile := map[string]string{"deepseek-openai": "deepseek", "zhipu-coding-plan-openai": "zai", "new-api-connection": ""}[id]
			if config.Chat == nil || config.Chat.URL != "https://relay.example.com/custom/v1/chat/completions" || config.Chat.Profile != profile {
				t.Fatalf("old host/profile survived: %+v", config.Chat)
			}
			if id == "new-api-connection" {
				if config.Responses == nil || config.Responses.URL != "https://relay.example.com/custom/v1/responses" || config.Messages == nil || config.Messages.URL != "https://relay.example.com/custom/v1/messages" || config.Messages.Auth != "bearer" {
					t.Fatalf("New API contract: %+v", config)
				}
			}
		})
	}
}

func TestUpstreamPresetRejectsInvalidInput(t *testing.T) {
	withUserInfo := (&url.URL{Scheme: "https", Host: "example.com", User: url.UserPassword("fixture-user", "fixture-password")}).String()
	for _, base := range []string{"ftp://example.com", withUserInfo, "https://example.com?key=x", "https://example.com#fragment", "http://169.254.169.254", "http://metadata.google.internal"} {
		catalogCall(t, presetRouter(), "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": "deepseek-openai", "baseUrl": base}, http.StatusBadRequest)
	}
	catalogCall(t, presetRouter(), "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": "unknown", "baseUrl": "https://example.com"}, http.StatusBadRequest)
	for _, id := range []string{"new-api-connection", "openai-alpha-search"} {
		catalogCall(t, presetRouter(), "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": id}, http.StatusBadRequest)
	}
}

func TestUpstreamPresetResolverDefaultsAliasesAndCustomBases(t *testing.T) {
	for _, id := range []string{"bailian", "codingplan-openai", "longcat", "zenmux", "opencode-go", "ollama-native", "codex"} {
		preset, endpoints, base, err := resolveUpstreamPresetConfig(id, "")
		if err != nil || base != preset.DefaultURL || !endpoints.IsConfigured() {
			t.Fatalf("default resolve %s failed: %v", id, err)
		}
	}
	for alias, canonical := range map[string]string{"bailian-claude": "bailian", "codingplan-claude": "codingplan-openai", "deepseek-claude": "deepseek-openai", "moonshot-claude": "moonshot-openai", "minimax-claude": "minimax-openai", "zhipu-coding-plan-claude": "zhipu-coding-plan-openai", "zai-coding-plan-claude": "zai-coding-plan-openai", "kimi-coding-claude": "kimi-coding-openai"} {
		oldBase := service.GetSiteInitializationPreset(alias).DefaultURL
		preset, legacy, base, err := resolveUpstreamPresetConfig(alias, oldBase)
		_, defaults, defaultBase, defaultErr := resolveUpstreamPresetConfig(canonical, "")
		if err != nil || defaultErr != nil || preset.ID != canonical || base != defaultBase || !reflect.DeepEqual(legacy, defaults) {
			t.Fatalf("legacy %s resolved its old Messages root as a custom Chat root: %v %v", alias, err, defaultErr)
		}
	}
	for _, preset := range service.ListUpstreamPresets() {
		_, config, actualBase, err := resolveUpstreamPresetConfig(preset.ID, " https://relay.example/custom/v1/ ")
		if err != nil || actualBase != "https://relay.example/custom/v1" {
			t.Fatalf("custom %s: %s %v", preset.ID, actualBase, err)
		}
		paths := service.UpstreamPresetEndpointPaths(preset.ID, preset.Platform)
		for _, entry := range config.Entries() {
			if entry.Endpoint == nil {
				continue
			}
			original := paths.ForProtocol(entry.Protocol)
			if original.Auth != entry.Endpoint.Auth || original.Profile != entry.Endpoint.Profile || original.ModelPath != entry.Endpoint.ModelPath || original.RequestModel != entry.Endpoint.RequestModel {
				t.Fatalf("custom base changed %s %s wire contract", preset.ID, entry.Key)
			}
			for _, address := range entry.Endpoint.URLFields() {
				if !strings.HasPrefix(*address, "https://relay.example/custom/") {
					t.Fatalf("custom %s leaked default destination %s", preset.ID, *address)
				}
			}
		}
	}
}

func TestUpstreamPresetOpenRouterImagesUseDedicatedAPI(t *testing.T) {
	for _, tc := range []struct{ base, want string }{
		{"", "https://openrouter.ai/api/v1/images"},
		{"https://relay.example/custom/v1", "https://relay.example/custom/v1/images"},
	} {
		_, config, _, err := resolveUpstreamPresetConfig("openrouter", tc.base)
		if err != nil {
			t.Fatal(err)
		}
		// The OpenRouter image adapter exchanges input_references and a top-level
		// images result. That is the dedicated images API, not Chat multimodality.
		for _, endpoint := range []*store.DirectEndpoint{config.ImageGeneration, config.ImageEdit} {
			if endpoint == nil || endpoint.URL != tc.want || endpoint.Auth != "bearer" || endpoint.Profile != "openrouter-image" {
				t.Fatalf("OpenRouter image adapter resolved to the wrong API: %+v", endpoint)
			}
		}
		if config.Chat.URL == config.ImageGeneration.URL || config.Chat.Profile != "openrouter" {
			t.Fatal("dedicated image route replaced Chat")
		}
	}
}

func TestUpstreamPresetCreatesNativeChannelWithoutLegacyRows(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			r := catalogMux(db)
			RegisterUpstreamPresetRoutes(r)
			provider, endpoints := resolvePreset(t, r, "deepseek-openai", "https://api.deepseek.com/v1")
			created := catalogCall(t, r, "POST", catalogPrefix, map[string]any{"name": "DeepSeek", "provider": provider, "baseUrl": "https://api.deepseek.com/v1", "dialect": "generic", "endpointConfig": endpoints, "enabled": true, "useSystemProxy": false}, http.StatusCreated)
			if created["ownership"] != "native" || catalogID(created) < 1 {
				t.Fatalf("create=%+v", created)
			}
			for _, table := range []string{"sites", "accounts", "upstream_credentials", "upstream_models"} {
				var count int
				if err := db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Errorf("creation populated %s: %d", table, count)
				}
			}
		})
	}
}

func TestUpstreamPresetPlatformWireCRUD(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := catalogTestDB(t, dialect, ":memory:")
			r := catalogMux(db)
			RegisterUpstreamPresetRoutes(r)
			for _, id := range []string{"opencode-go", "opencode-go-messages", "cline", "bailian", "codingplan-openai"} {
				provider, endpoints := resolvePreset(t, r, id, "https://relay.example/custom/v1")
				if id == "bailian" || id == "codingplan-openai" {
					if endpoints.Chat.Profile != "bailian" {
						t.Fatal("explicit Bailian preset lost its adapter")
					}
				}
				input := map[string]any{"name": id, "provider": provider, "baseUrl": "https://relay.example/custom/v1", "endpointConfig": endpoints, "enabled": true}
				created := catalogCall(t, r, "POST", catalogPrefix, input, 201)
				path := fmt.Sprintf("%s/%d", catalogPrefix, catalogID(created))
				out := catalogRequest(r, "GET", path, nil)
				var detail struct {
					Endpoints store.DirectEndpoints `json:"endpointConfig"`
				}
				if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &detail) != nil || !reflect.DeepEqual(detail.Endpoints, endpoints) {
					t.Fatalf("%s did not round-trip: %d %s", id, out.Code, out.Body.String())
				}
				if id != "opencode-go" {
					continue
				}
				if endpoints.Chat.ModelWireURLs.Responses != "https://relay.example/custom/v1/responses" || endpoints.Chat.ModelWireURLs.Messages != "https://relay.example/custom/v1/messages" {
					t.Fatal("internal model URLs were not resolved")
				}
				withUserInfo := (&url.URL{Scheme: "https", Host: "relay.example", Path: "/messages", User: url.UserPassword("fixture-user", "fixture-password")}).String()
				for _, bad := range []string{"", withUserInfo, "https://relay.example/messages?key=fixture", "https://relay.example/messages#fragment", "http://169.254.169.254/messages", "http://metadata.google.internal/messages"} {
					invalid := endpoints
					chat := *endpoints.Chat
					urls := *chat.ModelWireURLs
					urls.Messages = bad
					chat.ModelWireURLs = &urls
					invalid.Chat = &chat
					catalogCall(t, r, "PATCH", path, map[string]any{"endpointConfig": invalid}, 400)
				}
				catalogCall(t, r, "PATCH", path, map[string]any{"provider": "generic"}, 400)
				endpoints.Responses = &store.DirectEndpoint{URL: "https://custom.example/responses", Auth: store.DirectAuthBearer}
				catalogCall(t, r, "PATCH", path, map[string]any{"endpointConfig": endpoints}, 200)
				out = catalogRequest(r, "GET", path, nil)
				if json.Unmarshal(out.Body.Bytes(), &detail) != nil || !reflect.DeepEqual(detail.Endpoints, endpoints) {
					t.Fatal("custom Responses replaced internal wire destination")
				}
			}
		})
	}
}
