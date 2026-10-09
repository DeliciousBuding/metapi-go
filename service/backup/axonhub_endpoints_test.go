package backup

import (
	"net/url"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubEndpointSourceURLs(t *testing.T) {
	for _, tc := range []struct {
		name, provider, base, path, want string
		protocol                         int
		custom                           bool
	}{
		{"openai", "openai", "https://provider.invalid", "", "https://provider.invalid/v1/chat/completions", protoChat, false},
		{"nested version", "openai", "https://provider.invalid/api/v1/tenant", "", "https://provider.invalid/api/v1/tenant/chat/completions", protoChat, false},
		{"doubao default", "doubao", "https://provider.invalid/api", "", "https://provider.invalid/api/v3/chat/completions", protoChat, false},
		{"zhipu default", "zhipu", "https://provider.invalid/api/paas", "", "https://provider.invalid/api/paas/v4/chat/completions", protoChat, false},
		{"gemini compatibility", "gemini_openai", "https://provider.invalid", "", "https://provider.invalid/v1beta/openai/chat/completions", protoChat, false},
		{"openrouter primary", "openrouter", "https://provider.invalid/api", "", "https://provider.invalid/api/chat/completions", protoChat, false},
		{"openrouter custom generic", "openrouter", "https://provider.invalid/api", "", "https://provider.invalid/api/v1/chat/completions", protoChat, true},
		{"custom version historical", "doubao", "https://provider.invalid/api", "", "https://provider.invalid/api/v1/chat/completions", protoChat, true},
		{"custom family version", "doubao", "https://provider.invalid/api/v3", "", "https://provider.invalid/api/v3/chat/completions", protoChat, true},
		{"custom exact append", "openai", "https://provider.invalid/v1", "/v1/custom", "https://provider.invalid/v1/v1/custom", protoChat, true},
		{"no version marker", "openai_responses", "https://provider.invalid/api#", "", "https://provider.invalid/api/responses", protoResponses, false},
		{"raw marker", "openai_responses", "https://provider.invalid/custom/infer##", "/ignored", "https://provider.invalid/custom/infer", protoResponses, true},
		{"messages custom", "anthropic", "https://provider.invalid/api", "/custom/messages", "https://provider.invalid/api/custom/messages", protoMessages, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, problem := resolveAxonHubEndpointURL(tc.provider, tc.protocol, tc.base, tc.path, tc.custom)
			if problem != "" || got != tc.want {
				t.Fatalf("URL=%q problem=%q, want %q", got, problem, tc.want)
			}
		})
	}
}

func TestAxonHubEndpointsMergeAndAuthentication(t *testing.T) {
	channel := AxonHubSourceChannel{Type: "xai", BaseURL: "https://primary.invalid/api", Endpoints: []AxonHubSourceEndpoint{
		{APIFormat: "openai/chat_completions", BaseURL: "https://chat.invalid/prefix", Path: "/custom"},
		{APIFormat: "anthropic/messages", BaseURL: "https://messages.invalid/prefix"},
	}}
	protocols, endpoints, problems, _ := resolveChannelEndpoints(channel, axonHubProviderTypes[channel.Type])
	if len(problems) != 0 || protocols != protoChat|protoResponses|protoMessages {
		t.Fatalf("protocols=%d problems=%v", protocols, problems)
	}
	if endpoints.Chat.URL != "https://chat.invalid/prefix/custom" || endpoints.Responses.URL != "https://primary.invalid/api/v1/responses" || endpoints.Messages.URL != "https://messages.invalid/prefix/v1/messages" {
		t.Fatalf("resolved endpoints = %+v / %+v / %+v", endpoints.Chat, endpoints.Responses, endpoints.Messages)
	}
	for _, tc := range []struct {
		provider string
		custom   bool
		want     string
	}{
		{"anthropic", false, store.DirectAuthAPIKey},
		{"longcat_anthropic", false, store.DirectAuthBearer},
		{"longcat_anthropic", true, store.DirectAuthAPIKey},
		{"ollama_anthropic", false, store.DirectAuthBearer},
		{"commandcode", true, store.DirectAuthBearer},
		{"commandcode_anthropic", false, store.DirectAuthBearer},
	} {
		t.Run(tc.provider+map[bool]string{true: " custom", false: " default"}[tc.custom], func(t *testing.T) {
			ch := AxonHubSourceChannel{Type: tc.provider, BaseURL: "https://provider.invalid"}
			if tc.custom {
				ch.Endpoints = []AxonHubSourceEndpoint{{APIFormat: "anthropic/messages"}}
			}
			_, endpoints, reasons, _ := resolveChannelEndpoints(ch, axonHubProviderTypes[ch.Type])
			if len(reasons) != 0 || endpoints.Messages == nil || endpoints.Messages.Auth != tc.want {
				t.Fatalf("endpoints=%+v reasons=%v", endpoints, reasons)
			}
		})
	}
}

func TestAxonHubCustomEndpointKeepsDefaultAndResiduals(t *testing.T) {
	channel := AxonHubSourceChannel{Type: "openai", BaseURL: "https://provider.invalid", Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/responses"}}}
	protocols, endpoints, reasons, residuals := resolveChannelEndpoints(channel, axonHubProviderTypes[channel.Type])
	if len(reasons) != 0 || protocols != protoChat|protoResponses || endpoints.Chat == nil || endpoints.Responses == nil {
		t.Fatalf("protocols=%d endpoints=%+v reasons=%v", protocols, endpoints, reasons)
	}
	if !residualContains(residuals, "declared_protocol_not_servable:openai/embeddings") || !residualContains(residuals, "declared_protocol_not_servable:openai/audio_speech") {
		t.Fatalf("lost default residuals: %v", residuals)
	}
}

func TestAxonHubEndpointOverrideRejectsUnsafeTargets(t *testing.T) {
	credentialURL := (&url.URL{Scheme: "https", Host: "provider.invalid", User: url.UserPassword("user", "secret")}).String()
	for _, base := range []string{credentialURL, "http://169.254.169.254/latest", "file:///etc/passwd", "https://provider.invalid?key=secret", "https://provider.invalid#fragment", "https://provider.invalid###"} {
		channel := AxonHubSourceChannel{Type: "openai", BaseURL: "https://safe.invalid", Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/responses", BaseURL: base}}}
		_, _, reasons, _ := resolveChannelEndpoints(channel, axonHubProviderTypes[channel.Type])
		if len(reasons) == 0 {
			t.Fatalf("accepted unsafe override %q", base)
		}
	}
}
