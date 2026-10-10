package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
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
	for _, preset := range result.Items[1:] {
		source := service.GetSiteInitializationPreset(preset.ID)
		if source == nil {
			// Native formats are upstream-only: site adapters cannot execute them.
			if !slices.Contains([]string{"seedance-video", "zenmux-video", "ollama-native", "ollama-anthropic", "bedrock-messages", "typesafe-systemone", "openai-alpha-search", "codex-alpha-search", "opencode-go", "opencode-go-messages", "cline"}, preset.ID) {
				t.Errorf("shared preset %s missing from the site registry", preset.ID)
			}
			continue
		}
		if source == nil || source.DefaultURL != preset.DefaultURL || source.ProviderLabel != preset.Name || source.Label != preset.Label || source.Platform != preset.Platform || !slices.Equal(source.RecommendedModels, preset.RecommendedModels) {
			t.Errorf("preset %s drifted from site registry", preset.ID)
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
		{"opencode-go-messages", "opencode_go_anthropic", "messages", "https://opencode.ai/zen/go/v1/messages", "x-api-key", "", false},
		{"cline", "cline", "chat", "https://api.cline.bot/api/v1/chat/completions", "bearer", "cline", false},
		{"ollama-native", "ollama", "ollama", "http://localhost:11434/api/chat", "none", "ollama", false},
		{"ollama-anthropic", "ollama_anthropic", "messages", "http://localhost:11434/v1/messages", "none", "ollama-messages", false},
		{"bedrock-messages", "anthropic_aws", "messages", "https://bedrock-runtime.us-east-1.amazonaws.com/model", "bearer", "bedrock", true},
		{"seedance-video", "doubao", "seedanceVideo", "https://ark.cn-beijing.volces.com/api/v3/contents/generations/tasks", "bearer", "seedance-video", false},
		{"zenmux-video", "zenmux_video", "zenmuxVideo", "https://zenmux.ai/api/v1/videos", "bearer", "zenmux-video", false},
		{"typesafe-systemone", "typesafe", "systemOne", "https://api.typesafe.ai/v1/systemone", "bearer", "", false},
		{"codex-alpha-search", "codex", "alphaSearch", "https://chatgpt.com/backend-api/codex/alpha/search", "bearer", "codex-alpha-search", false},
		{"deepseek-openai", "deepseek", "chat", "https://api.deepseek.com/v1/chat/completions", "bearer", "deepseek", false},
		{"zhipu-coding-plan-openai", "zhipu", "chat", "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions", "bearer", "zai", false},
		{"doubao-openai", "doubao", "chat", "https://ark.cn-beijing.volces.com/api/v3/chat/completions", "bearer", "", false},
		{"codingplan-claude", "bailian_anthropic", "messages", "https://coding.dashscope.aliyuncs.com/apps/anthropic/v1/messages", "x-api-key", "", false},
		{"xiaomi-token-plan-claude", "xiaomi_anthropic", "messages", "https://token-plan-cn.xiaomimimo.com/anthropic/v1/messages", "x-api-key", "", false},
		{"gemini-api", "gemini", "gemini", "https://generativelanguage.googleapis.com/v1beta/models", "x-goog-api-key", "", true},
		{"openai-api", "openai", "responses", "https://api.openai.com/v1/responses", "bearer", "", false},
		{"bailian", "bailian", "responses", "https://dashscope.aliyuncs.com/compatible-mode/v1/responses", "bearer", "", false},
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
		"opencode-go":          {"chat"},
		"opencode-go-messages": {"messages"},
		"cline":                {"chat"},
		"new-api-connection":   {"chat", "responses", "messages", "gemini", "completions", "embeddings", "rerank", "imageGeneration", "imageEdit", "audioSpeech", "audioTranscription", "audioTranslation", "moderations", "video"},
		"deepseek-openai":      {"chat", "completions"},
		"codingplan-claude":    {"messages"},
		"codingplan-openai":    {"chat"},
		"bailian":              {"chat", "responses"},
		"gemini-api":           {"gemini", "geminiEmbeddings"},
		"openai-api":           {"chat", "responses", "embeddings", "imageGeneration", "imageEdit", "imageVariation", "audioSpeech", "audioTranscription", "audioTranslation", "moderations"},
		"xai-api":              {"chat", "responses"},
		"jina":                 {"rerank", "jinaEmbeddings"},
		"minimax-openai":       {"chat", "imageGeneration"},
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

func TestUpstreamPresetCustomHostDoesNotInheritProfile(t *testing.T) {
	for _, id := range []string{"deepseek-openai", "zhipu-coding-plan-openai", "new-api-connection"} {
		t.Run(id, func(t *testing.T) {
			_, config := resolvePreset(t, presetRouter(), id, " https://relay.example.com/custom/v1/ ")
			if config.Chat == nil || config.Chat.URL != "https://relay.example.com/custom/v1/chat/completions" || config.Chat.Profile != "" {
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
	for _, base := range []string{"", "ftp://example.com", withUserInfo, "https://example.com?key=x", "https://example.com#fragment", "http://169.254.169.254", "http://metadata.google.internal"} {
		catalogCall(t, presetRouter(), "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": "deepseek-openai", "baseUrl": base}, http.StatusBadRequest)
	}
	catalogCall(t, presetRouter(), "POST", catalogPrefix+"/presets/resolve", map[string]string{"presetId": "unknown", "baseUrl": "https://example.com"}, http.StatusBadRequest)
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
