package backup

import (
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubNativeEndpointContracts(t *testing.T) {
	for _, tc := range []struct {
		name, provider, base, format, path, key, wantURL, profile, auth string
		protocol                                                        int
		modelPath                                                       bool
	}{
		{"Ollama local", "ollama", "http://127.0.0.1:11434/", "", "", "", "http://127.0.0.1:11434/api/chat", "ollama", store.DirectAuthNone, protoOllama, false},
		{"Ollama authenticated", "ollama", "https://ollama.invalid/prefix", "", "", "key", "https://ollama.invalid/prefix/api/chat", "ollama", store.DirectAuthBearer, protoOllama, false},
		{"Ollama Messages local", "ollama_anthropic", "http://localhost:11434", "", "", "", "http://localhost:11434/v1/messages", "ollama-messages", store.DirectAuthNone, protoMessages, false},
		{"Ollama Messages", "ollama_anthropic", "https://ollama.invalid/api#", "", "", "key", "https://ollama.invalid/api/messages", "ollama-messages", store.DirectAuthBearer, protoMessages, false},
		{"Ollama custom becomes direct", "ollama_anthropic", "https://ollama.invalid/api", "anthropic/messages", "/custom", "key", "https://ollama.invalid/api/custom", "", store.DirectAuthAPIKey, protoMessages, false},
		{"Bedrock model prefix", "anthropic_aws", "https://bedrock.invalid/region/", "", "", "key", "https://bedrock.invalid/region/model", "bedrock", store.DirectAuthBearer, protoMessages, true},
		{"Bedrock custom becomes direct", "anthropic_aws", "https://bedrock.invalid", "anthropic/messages", "/custom", "key", "https://bedrock.invalid/custom", "", store.DirectAuthAPIKey, protoMessages, false},
		{"Seedance", "doubao", "https://ark.invalid/api", "", "", "key", "https://ark.invalid/api/v3/contents/generations/tasks", "seedance-video", store.DirectAuthBearer, protoSeedanceVideo, false},
		{"Seedance raw base", "doubao", "https://ark.invalid/raw##", "", "", "key", "https://ark.invalid/raw/contents/generations/tasks", "seedance-video", store.DirectAuthBearer, protoSeedanceVideo, false},
		{"ZenMux default", "zenmux_video", "", "", "", "key", "https://zenmux.ai/api/v1/videos", "zenmux-video", store.DirectAuthBearer, protoZenmuxVideo, false},
		{"ZenMux custom", "zenmux", "https://zenmux.invalid/api", "zenmux/video", "/custom/tasks/", "key", "https://zenmux.invalid/api/custom/tasks", "zenmux-video", store.DirectAuthBearer, protoZenmuxVideo, false},
		{"ZenMux custom videos keeps version", "zenmux_video", "https://zenmux.invalid/api", "zenmux/video", "/videos/", "key", "https://zenmux.invalid/api/v1/videos", "zenmux-video", store.DirectAuthBearer, protoZenmuxVideo, false},
		{"System One default", "typesafe", "", "", "", "key", "https://api.typesafe.ai/v1/systemone", "", store.DirectAuthBearer, protoSystemOne, false},
		{"System One custom", "typesafe", "https://typesafe.invalid/api", "typesafe/systemone", "/ask", "key", "https://typesafe.invalid/api/ask", "", store.DirectAuthBearer, protoSystemOne, false},
		{"Alpha Search generic", "openai", "https://search.invalid", "openai/alpha_search", "", "key", "https://search.invalid/v1/alpha/search", "", store.DirectAuthBearer, protoAlphaSearch, false},
		{"Alpha Search custom", "openai", "https://search.invalid/api", "openai/alpha_search", "/ask", "key", "https://search.invalid/api/ask", "", store.DirectAuthBearer, protoAlphaSearch, false},
		{"Codex Alpha Search default", "codex", "", "", "", "key", "https://chatgpt.com/backend-api/codex/alpha/search", "codex-alpha-search", store.DirectAuthBearer, protoAlphaSearch, false},
		{"Codex Alpha Search custom", "codex", "https://search.invalid/api#", "openai/alpha_search", "/ask", "key", "https://search.invalid/api/ask", "codex-alpha-search", store.DirectAuthBearer, protoAlphaSearch, false},
		{"Fenno Alpha Search stays generic", "fenno", "https://search.invalid/api", "openai/alpha_search", "/ask", "key", "https://search.invalid/api/ask", "", store.DirectAuthBearer, protoAlphaSearch, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := AxonHubSourceChannel{ID: 1, Type: tc.provider, BaseURL: tc.base, Credentials: AxonHubSourceCredentials{APIKey: tc.key}}
			if tc.format != "" {
				ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: tc.format, Path: tc.path}}
			}
			compiled, problems, residuals := compileAxonHubChannel(ch)
			if len(problems) != 0 || compiled == nil {
				t.Fatalf("compile problems=%v residuals=%v", problems, residuals)
			}
			var endpoint *store.DirectEndpoint
			for _, entry := range compiled.Endpoints.Entries() {
				if entry.Protocol == tc.protocol {
					endpoint = entry.Endpoint
				}
			}
			want := &store.DirectEndpoint{URL: tc.wantURL, Profile: tc.profile, Auth: tc.auth, ModelPath: tc.modelPath}
			if !reflect.DeepEqual(endpoint, want) {
				t.Fatalf("endpoint=%+v want=%+v", endpoint, want)
			}
		})
	}
}

func TestAxonHubNativeUnconstructibleSourceEndpoints(t *testing.T) {
	for _, tc := range []struct{ provider, format, key, reason string }{
		{"ollama", "ollama/chat", "key", "source_custom_endpoint_not_constructible:ollama/chat"},
		{"doubao", "seedance/video", "key", "source_custom_endpoint_not_constructible:seedance/video"},
		{"openai", "ollama/chat", "key", "source_custom_endpoint_not_constructible:ollama/chat"},
		{"ollama_anthropic", "anthropic/messages", "", "source_custom_endpoint_requires_api_key"},
		{"ollama", "openai/chat_completions", "", "source_custom_endpoint_requires_api_key"},
		{"openai", "typesafe/systemone", "key", "source_endpoint_provider_mismatch"},
		{"typesafe", "openai/chat_completions", "key", "source_endpoint_provider_mismatch"},
		{"openai", "zenmux/video", "key", "source_endpoint_provider_mismatch"},
	} {
		t.Run(tc.provider+"/"+tc.format, func(t *testing.T) {
			ch := AxonHubSourceChannel{Type: tc.provider, BaseURL: "https://fixture.invalid", Credentials: AxonHubSourceCredentials{APIKey: tc.key}, Endpoints: []AxonHubSourceEndpoint{{APIFormat: tc.format}}}
			compiled, problems, _ := compileAxonHubChannel(ch)
			if compiled != nil || !reflect.DeepEqual(problems, []string{tc.reason}) {
				t.Fatalf("compiled=%v problems=%v", compiled, problems)
			}
		})
	}
}
