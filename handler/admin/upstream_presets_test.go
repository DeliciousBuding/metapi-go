package admin

import (
	"encoding/json"
	"net/http"
	"net/url"
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
			if provider != tc.provider || endpoint == nil || *endpoint != want {
				t.Fatalf("provider=%s endpoint=%+v; want %s %+v", provider, endpoint, tc.provider, want)
			}
		})
	}
}

func TestUpstreamPresetAdvertisedProtocolsMatchEndpoints(t *testing.T) {
	contracts := map[string][]string{
		"new-api-connection": {"chat", "responses", "messages", "gemini", "completions", "embeddings", "rerank", "imageGeneration", "imageEdit", "audioSpeech", "audioTranscription", "audioTranslation", "moderations", "video"},
		"deepseek-openai":    {"chat", "completions"},
		"codingplan-claude":  {"messages"},
		"codingplan-openai":  {"chat"},
		"bailian":            {"chat", "responses"},
		"gemini-api":         {"gemini", "geminiEmbeddings"},
		"openai-api":         {"chat", "responses", "embeddings", "imageGeneration", "imageEdit", "imageVariation", "audioSpeech", "audioTranscription", "audioTranslation", "moderations"},
		"xai-api":            {"chat", "responses"},
		"jina":               {"rerank", "jinaEmbeddings"},
		"minimax-openai":     {"chat", "imageGeneration"},
		"modelscope-openai":  {"chat", "modelscopeImageGeneration"},
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
