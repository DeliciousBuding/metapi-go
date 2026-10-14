package store

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"
)

func TestDirectModelWireURLsContract(t *testing.T) {
	valid := func() DirectEndpoints {
		return DirectEndpoints{Chat: &DirectEndpoint{URL: "https://go.example/v1/chat/completions", Auth: DirectAuthBearer, Profile: "opencode-go", ModelWireURLs: &DirectModelWireURLs{Responses: "https://go.example/v1/responses", Messages: "https://go.example/v1/messages"}}}
	}
	for _, tc := range []struct {
		name   string
		change func(*DirectEndpoints)
		valid  bool
	}{
		{"valid", func(*DirectEndpoints) {}, true},
		{"missing routes", func(e *DirectEndpoints) { e.Chat.ModelWireURLs = nil }, false},
		{"missing destination", func(e *DirectEndpoints) { e.Chat.ModelWireURLs.Messages = "" }, false},
		{"wrong profile", func(e *DirectEndpoints) { e.Chat.Profile = "" }, false},
		{"wrong slot", func(e *DirectEndpoints) { e.Responses, e.Chat = e.Chat, nil }, false},
		{"wrong auth", func(e *DirectEndpoints) { e.Chat.Auth = DirectAuthAPIKey }, false},
		{"credentials in URL", func(e *DirectEndpoints) {
			e.Chat.ModelWireURLs.Responses = (&url.URL{Scheme: "https", Host: "go.example", Path: "/responses", User: url.UserPassword("fixture-user", "fixture-password")}).String()
		}, false},
		{"fragment", func(e *DirectEndpoints) { e.Chat.ModelWireURLs.Messages += "#raw" }, false},
		{"query", func(e *DirectEndpoints) { e.Chat.ModelWireURLs.Messages += "?token=fixture" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := valid()
			tc.change(&value)
			raw, err := value.Value()
			if err != nil {
				t.Fatal(err)
			}
			var restored DirectEndpoints
			err = restored.Scan(raw)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && !reflect.DeepEqual(value, restored) {
				t.Fatal("wire URLs lost in storage round trip")
			}
		})
	}
	// These destinations are implementation details of the Chat capability.
	if got := valid().ProtocolMask(); got != DirectProtocolChat {
		t.Fatalf("wire destinations expanded grant mask: %d", got)
	}
}

func TestDirectProviderProfilesRequireMatchingWire(t *testing.T) {
	for _, profile := range []string{"moonshot", "longcat", "openrouter", "cerebras", "nanogpt", "github-copilot", "openrouter-image"} {
		for _, protocol := range []int{DirectProtocolChat, DirectProtocolMessages, DirectProtocolImageGeneration, DirectProtocolImageEdit} {
			for _, auth := range []string{DirectAuthBearer, DirectAuthAPIKey, DirectAuthNone} {
				endpoints := map[string]*DirectEndpoint{}
				for _, entry := range (DirectEndpoints{}).Entries() {
					if entry.Protocol == protocol {
						endpoints[entry.Key] = &DirectEndpoint{URL: "https://provider.example/endpoint", Auth: auth, Profile: profile}
					}
				}
				raw, err := json.Marshal(endpoints)
				if err != nil {
					t.Fatal(err)
				}
				var parsed DirectEndpoints
				err = parsed.Scan(raw)
				want := auth == DirectAuthBearer && (profile != "openrouter-image" && protocol == DirectProtocolChat || profile == "openrouter-image" && (protocol == DirectProtocolImageGeneration || protocol == DirectProtocolImageEdit))
				if (err == nil) != want {
					t.Fatalf("%s protocol=%d auth=%s expected valid=%t, got %v", profile, protocol, auth, want, err)
				}
			}
		}
	}
}

func TestDirectXAISubscriptionProfileRequiresResponses(t *testing.T) {
	valid := DirectEndpoints{Responses: &DirectEndpoint{URL: "https://cli-chat-proxy.grok.com/v1/responses", Auth: DirectAuthBearer, Profile: "xai-subscription"}}
	raw, err := valid.Value()
	if err != nil {
		t.Fatal(err)
	}
	var restored DirectEndpoints
	if err := restored.Scan(raw); err != nil || restored.Responses == nil || restored.Responses.Profile != "xai-subscription" {
		t.Fatalf("valid profile rejected: %v", err)
	}

	invalid := DirectEndpoints{Chat: &DirectEndpoint{URL: "https://cli-chat-proxy.grok.com/v1/chat/completions", Auth: DirectAuthBearer, Profile: "xai-subscription"}}
	raw, err = invalid.Value()
	if err != nil {
		t.Fatal(err)
	}
	restored = DirectEndpoints{}
	if err := restored.Scan(raw); err == nil {
		t.Fatal("xAI subscription profile was accepted on Chat")
	}
}
