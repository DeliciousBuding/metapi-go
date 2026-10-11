package proxy

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestRoutingProtocolForPathMatchesRequestAuthorization(t *testing.T) {
	for _, tc := range []struct {
		path       string
		required   int
		conversion bool
	}{
		{"/v1/chat/completions", store.DirectProtocolChat, true},
		{"/v1/responses", store.DirectProtocolResponses, true},
		{"/v1/messages", store.DirectProtocolMessages, true},
		{"/v1/messages/count_tokens", store.DirectProtocolMessages, false},
		{"/v1/messages/count_tokens/", store.DirectProtocolMessages, false},
		{"/v1beta/models/fixture:generateContent", store.DirectProtocolGemini, true},
		{"/api/chat", store.DirectProtocolOllama, true},
		{"/v1/embeddings", store.DirectProtocolEmbeddings | store.DirectProtocolJinaEmbeddings, false},
		{"/v1/images/generations", store.DirectProtocolImageGeneration | store.DirectProtocolModelScopeImageGeneration, false},
		{"/v1/videos", store.DirectVideoProtocols, false},
		{"/v1/unknown", 0, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			required, conversion := RoutingProtocolForPath(tc.path)
			if required != tc.required || conversion != tc.conversion {
				t.Fatalf("got (%d,%v), want (%d,%v)", required, conversion, tc.required, tc.conversion)
			}
		})
	}
}
