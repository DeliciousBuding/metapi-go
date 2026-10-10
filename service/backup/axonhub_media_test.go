package backup

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// Expectations come from e863c6fe's outbound constructors/builders, including
// their deliberately different handling of custom paths and trailing markers.
func TestAxonHubMediaSourceURLs(t *testing.T) {
	for _, tc := range []struct {
		name, provider, base, path, want, profile string
		protocol                                  int
		custom, modelPath                         bool
	}{
		{"completions", "openai", "https://provider.invalid", "", "https://provider.invalid/v1/completions", "", protoCompletions, true, false},
		{"completions raw", "openai", "https://provider.invalid/custom/##", "/ignored", "https://provider.invalid/custom/", "", protoCompletions, true, false},
		{"deepseek default beta", "deepseek", "https://provider.invalid/v1", "", "https://provider.invalid/beta/completions", "", protoCompletions, false, false},
		{"deepseek custom generic", "deepseek", "https://provider.invalid", "", "https://provider.invalid/v1/completions", "", protoCompletions, true, false},
		{"embeddings", "openai", "https://provider.invalid", "", "https://provider.invalid/v1/embeddings", "", protoEmbeddings, false, false},
		{"embedding no version", "openai", "https://provider.invalid/base#", "", "https://provider.invalid/base/embeddings", "", protoEmbeddings, true, false},
		{"embedding raw still appends", "openai", "https://provider.invalid/base/##", "", "https://provider.invalid/base//embeddings", "", protoEmbeddings, true, false},
		{"image raw custom still appends", "openai", "https://provider.invalid/base##", "/custom-image", "https://provider.invalid/base/custom-image", "", protoImageGeneration, true, false},
		{"image edit custom", "openai", "https://provider.invalid/prefix", "/custom-edit", "https://provider.invalid/prefix/custom-edit", "", protoImageEdit, true, false},
		{"image variation", "openai", "https://provider.invalid/v1", "", "https://provider.invalid/v1/images/variations", "", protoImageVariation, false, false},
		{"speech", "openrouter", "https://provider.invalid/api", "", "https://provider.invalid/api/v1/audio/speech", "", protoAudioSpeech, false, false},
		{"transcription custom", "groq", "https://provider.invalid/api", "/transcribe", "https://provider.invalid/api/transcribe", "", protoAudioTranscription, true, false},
		{"translation raw appends", "openai", "https://provider.invalid/base##", "", "https://provider.invalid/base/audio/translations", "", protoAudioTranslation, true, false},
		{"moderation", "openai", "https://provider.invalid/api/v1/tenant", "", "https://provider.invalid/api/v1/tenant/moderations", "", protoModerations, false, false},
		{"video ignores custom path", "openai", "https://provider.invalid/api", "/ignored-video", "https://provider.invalid/api/videos", "", protoVideo, true, false},
		{"video raw still appends", "openai", "https://provider.invalid/api/##", "", "https://provider.invalid/api//videos", "", protoVideo, true, false},
		{"gemini embedding", "gemini", "https://provider.invalid", "", "https://provider.invalid/v1beta/models", "", protoGeminiEmbeddings, false, true},
		{"gemini ignores custom path", "gemini", "https://provider.invalid/v1", "/ignored-embedding", "https://provider.invalid/v1/models", "", protoGeminiEmbeddings, true, true},
		{"gemini trailing slash", "gemini", "https://provider.invalid/", "", "https://provider.invalid//v1beta/models", "", protoGeminiEmbeddings, false, true},
		{"jina embedding", "jina", "https://provider.invalid", "", "https://provider.invalid/v1/embeddings", "jina-embeddings", protoJinaEmbeddings, false, false},
		{"jina custom remains jina", "jina", "https://provider.invalid/api", "/embed", "https://provider.invalid/api/embed", "jina-embeddings", protoJinaEmbeddings, true, false},
		{"generic embedding on jina", "jina", "https://provider.invalid", "/generic", "https://provider.invalid/generic", "", protoEmbeddings, true, false},
		{"jina rerank", "jina", "https://provider.invalid/api#", "", "https://provider.invalid/api/rerank", "", protoRerank, false, false},
		{"minimax default", "minimax", "https://provider.invalid", "", "https://provider.invalid/v1/image_generation", "minimax-image", protoImageGeneration, false, false},
		{"minimax custom dedup", "minimax", "https://provider.invalid/v1", "/v1/custom-image", "https://provider.invalid/v1/custom-image", "minimax-image", protoImageGeneration, true, false},
		{"modelscope exact base", "modelscope", "https://provider.invalid", "", "https://provider.invalid/images/generations", "modelscope-image", protoModelScopeImage, false, false},
		{"modelscope custom generic", "modelscope", "https://provider.invalid", "/generic-image", "https://provider.invalid/generic-image", "", protoImageGeneration, true, false},
		{"codex custom path ignored", "codex", "https://provider.invalid/backend#", "/ignored-image", "https://provider.invalid/backend/responses", "codex-image", protoImageGeneration, true, false},
		{"fenno raw", "fenno", "https://provider.invalid/raw##", "/ignored", "https://provider.invalid/raw", "codex-image", protoImageEdit, true, false},
		{"codex legacy base", "codex", "https://api.openai.com/v1", "", "https://chatgpt.com/backend-api/codex/responses", "codex-image", protoImageGeneration, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, problem := resolveAxonHubMediaEndpoint(AxonHubSourceChannel{Type: tc.provider}, tc.protocol, tc.base, tc.path, tc.custom)
			if problem != "" || endpoint.URL != tc.want || endpoint.Profile != tc.profile || endpoint.ModelPath != tc.modelPath {
				t.Fatalf("endpoint=%+v problem=%s want URL=%s profile=%s modelPath=%v", endpoint, problem, tc.want, tc.profile, tc.modelPath)
			}
			auth := store.DirectAuthBearer
			if tc.protocol == protoGeminiEmbeddings {
				auth = store.DirectAuthGoogle
			}
			if endpoint.Auth != auth {
				t.Fatalf("auth=%s want=%s", endpoint.Auth, auth)
			}
		})
	}
	for _, p := range []int{protoJinaEmbeddings, protoRerank, protoGeminiEmbeddings, protoModelScopeImage} {
		if _, err := resolveAxonHubMediaEndpoint(AxonHubSourceChannel{Type: "jina"}, p, "https://provider.invalid/raw##", "", false); err == "" {
			t.Fatalf("unsupported marker accepted for %d", p)
		}
	}
}

func TestAxonHubMediaFormatsKeepDistinctIdentitiesAndSourceOrder(t *testing.T) {
	seen := map[int]string{}
	for format, bit := range axonHubServableFormats {
		if prev := seen[bit]; prev != "" {
			t.Fatalf("formats %s and %s share persisted protocol %d", prev, format, bit)
		}
		seen[bit] = format
		if !store.ValidDirectProtocol(bit) {
			t.Fatalf("format %s has no persisted protocol", format)
		}
	}
	ch := AxonHubSourceChannel{Type: "jina", BaseURL: "https://provider.invalid", Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/embeddings", Path: "/generic"}, {APIFormat: "jina/embeddings", Path: "/native"}}}
	mask, endpoints, reasons, residuals := resolveChannelEndpoints(ch, axonHubProviderTypes[ch.Type])
	if len(reasons) > 0 || len(residuals) > 0 || mask != protoRerank|protoJinaEmbeddings|protoEmbeddings {
		t.Fatalf("mask=%d reasons=%v residuals=%v", mask, reasons, residuals)
	}
	if endpoints.Embeddings.URL != "https://provider.invalid/generic" || endpoints.Embeddings.Profile != "" || endpoints.JinaEmbeddings.URL != "https://provider.invalid/native" || endpoints.JinaEmbeddings.Profile != "jina-embeddings" {
		t.Fatalf("formats collapsed: %+v", endpoints)
	}
	if got := axonHubEndpointOrder(ch, axonHubProviderTypes[ch.Type]); !reflect.DeepEqual(got, store.DirectProtocolOrder{protoRerank, protoJinaEmbeddings, protoEmbeddings}) {
		t.Fatalf("source order=%v", got)
	}
	ch = AxonHubSourceChannel{Type: "modelscope", BaseURL: "https://provider.invalid/api", Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/image_generation", Path: "/generic-image"}}}
	_, endpoints, reasons, _ = resolveChannelEndpoints(ch, axonHubProviderTypes[ch.Type])
	if len(reasons) > 0 || endpoints.ImageGeneration.Profile != "" || endpoints.ModelScopeImageGeneration.Profile != "modelscope-image" {
		t.Fatalf("ModelScope/custom OpenAI collapsed: %+v %v", endpoints, reasons)
	}
}

func TestAxonHubCodexImagePrimaryModelAndOAuthCustomContract(t *testing.T) {
	for _, tc := range []struct {
		model, want string
		settings    AxonHubSourceChannelSettings
	}{
		{"", "gpt-6-luna", AxonHubSourceChannelSettings{}},
		{"GPT-IMAGE-2", "gpt-6-luna", AxonHubSourceChannelSettings{}},
		{"main-alias", "real-main", AxonHubSourceChannelSettings{ModelMappings: []AxonHubSourceModelMapping{{From: "main-alias", To: "real-main"}}}},
		{"MAIN-ALIAS", "real-main", AxonHubSourceChannelSettings{LowercaseModelID: true, ModelMappings: []AxonHubSourceModelMapping{{From: "Main-Alias", To: "real-main"}}}},
	} {
		ch := AxonHubSourceChannel{Type: "codex", DefaultTestModel: tc.model, SupportedModels: []string{"real-main"}, Settings: tc.settings}
		endpoint, err := resolveAxonHubMediaEndpoint(ch, protoImageGeneration, "https://provider.invalid#", "", false)
		if err != "" || endpoint.RequestModel != tc.want {
			t.Fatalf("model=%s endpoint=%+v err=%s", tc.model, endpoint, err)
		}
	}
	raw := axonHubSettingsPayload(`{}`)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	ch := payload["channels"].([]any)[0].(map[string]any)
	ch["type"] = "codex"
	ch["default_test_model"] = "gpt-main"
	ch["credentials"] = map[string]any{"oauth": map[string]any{"access_token": "fixture-access", "refresh_token": "fixture-refresh"}}
	ch["endpoints"] = []any{map[string]any{"api_format": "openai/image_edit", "path": "/ignored"}}
	raw, _ = json.Marshal(payload)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.skipped) > 0 || len(plan.channels) != 1 {
		t.Fatalf("OAuth image custom refused: %+v", plan.skipped)
	}
	if plan.channels[0].Endpoints.ImageEdit.RequestModel != "gpt-main" {
		t.Fatal("source main model lost")
	}
	if residualContains(plan.residuals, "default_test_model_not_imported") {
		t.Fatal("imported main model still marked residual")
	}
}

func TestOctopusV5DoesNotReinterpretMediaBits(t *testing.T) {
	// Octopus 0538c3e's Protocol uint8 enum and relay registration expose only
	// chat/Responses/Messages at bits 2/4/8; bit 1 remains reserved.
	for _, bit := range []int{1, 16, 32, 64, 128, 256, 512, 1024, 131072, 262144} {
		raw := strings.Replace(octopusV5RealShapeFixture, `"protocols":2`, `"protocols":`+jsonNumber(bit), 1)
		if _, err := ParseOctopusV5([]byte(raw)); err == nil {
			t.Fatalf("Octopus source bit %d silently reinterpreted", bit)
		}
	}
}

func jsonNumber(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestAxonHubMediaModelTypesRequireActualEndpoints(t *testing.T) {
	for _, tc := range []struct{ kind, format string }{
		{"embedding", "openai/embeddings"}, {"rerank", "jina/rerank"},
		{"image_generation", "openai/image_generation"}, {"video_generation", "openai/video"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			var payload map[string]any
			_ = json.Unmarshal(axonHubSettingsPayload(`{}`), &payload)
			channel := payload["channels"].([]any)[0].(map[string]any)
			channel["type"] = "qiniu"
			model := payload["models"].([]any)[0].(map[string]any)
			model["type"] = tc.kind
			for _, configured := range []bool{false, true} {
				channel["endpoints"] = []any{}
				if configured {
					channel["endpoints"] = []any{map[string]any{"api_format": tc.format}}
				}
				raw, _ := json.Marshal(payload)
				src, err := ParseAxonHubSource(raw)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := CompileAxonHubPlan(src)
				if err != nil || (len(plan.routes) == 1) != configured {
					t.Fatalf("configured=%v routes=%v err=%v", configured, plan.routes, err)
				}
			}
		})
	}
}
