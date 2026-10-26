package service

import (
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestNewAPIInspiredMultiProtocolPresets(t *testing.T) {
	for _, test := range []struct {
		id          string
		protocols   []string
		messageAuth string
	}{
		{"custom", []string{"chat", "responses", "messages", "gemini"}, store.DirectAuthAPIKey},
		{"sub2api", []string{"chat", "responses", "messages", "gemini"}, store.DirectAuthBearer},
		{"vllm", []string{"chat", "responses", "messages", "completions", "embeddings"}, store.DirectAuthBearer},
		{"sglang", []string{"chat", "responses", "messages", "completions", "embeddings"}, store.DirectAuthBearer},
	} {
		t.Run(test.id, func(t *testing.T) {
			preset := GetUpstreamPreset(test.id)
			if preset == nil {
				t.Fatalf("missing multi-protocol preset %s", test.id)
			}
			if !preset.RequiresBaseURL || preset.CredentialMode != "apiKey" || !reflect.DeepEqual(preset.Protocols, test.protocols) {
				t.Fatalf("incorrect self-hosted connection contract: %+v", preset)
			}
			endpoints := UpstreamPresetEndpointPaths(preset.ID, preset.Platform)
			if endpoints.Chat.URL != "/v1/chat/completions" || endpoints.Responses.URL != "/v1/responses" || endpoints.Messages.URL != "/v1/messages" || endpoints.Messages.Auth != test.messageAuth {
				t.Fatal("native generation paths or authentication drifted")
			}
			if endpoints.Rerank != nil || endpoints.Video != nil || endpoints.ImageGeneration != nil {
				t.Fatal("preset advertises an unimplemented provider-specific media or rerank contract")
			}
			if endpoints.Gemini != nil && (!endpoints.Gemini.ModelPath || endpoints.Gemini.URL != "/v1beta/models") {
				t.Fatal("Gemini lost its model action URL contract")
			}
		})
	}
}
