package store

import (
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
