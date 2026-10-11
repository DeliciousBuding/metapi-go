package proxyhandler

import (
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectFallbackRejectionPolicy(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{400, `{"error":{"code":"model_not_found","message":"model is unavailable on this endpoint"}}`, true},
		{404, `{"error":{"message":"Unknown endpoint"}}`, true},
		{404, `{"error":{"message":"Invalid URL (POST /v1/responses)","type":"invalid_request_error","code":""}}`, true},
		{404, `{"error":{"message":"Invalid URL (POST /v1beta/models/gemini-fixture:generateContent)"}}`, true},
		{404, `{"error":{"message":"Invalid URL (POST /unknown)"}}`, false},
		{405, "please use /v1/responses", true},
		{422, "当前 api 不支持所选模型", true},
		{501, "unsupported endpoint", true},
		{400, `{"error":"unsupported model","usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`, true},
		{404, "404 page not found", false},
		{404, `<html>unknown endpoint</html>`, false},
		{401, "unsupported model", false},
		{403, "unsupported endpoint", false},
		{429, "unsupported model", false},
		{503, "unsupported model", false},
		{200, "unsupported model", false},
		{400, "model not found: you do not have access", false},
		{400, "unsupported model: quota exceeded", false},
		{400, `{"error":"unsupported model","usage":{"prompt_tokens":1}}`, false},
		{400, `{"error":"unsupported model","usage":false}`, false},
		{400, `{"error":"unsupported model","usage":{"cost":1e-1000}}`, false},
		{400, `{"error":"unsupported model","response":{"usage":{"input_tokens":1}}}`, false},
		{400, `{"error":"unsupported model","choices":[{"message":{"content":"already generated"}}]}`, false},
		{404, `{"message":"not found","input":"unsupported model"}`, false},
		{400, `{"error":"invalid JSON"}`, false},
	} {
		if got := directProtocolRejection(tc.status, tc.body); got != tc.want {
			t.Errorf("status=%d body=%s fallback=%v want=%v", tc.status, tc.body, got, tc.want)
		}
	}
}

func TestDirectFallbackCandidateOrder(t *testing.T) {
	endpoint := func() *store.DirectEndpoint { return &store.DirectEndpoint{URL: "https://upstream.example/endpoint"} }
	candidate := &store.DirectUpstreamCandidate{Protocols: store.DirectStandardGenerationProtocols, Endpoints: store.DirectEndpoints{Chat: endpoint(), Responses: endpoint(), Messages: endpoint(), Gemini: endpoint()}}
	paths, err := directCandidatePaths(candidate, "/v1/messages", "model", false)
	want := []string{"/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1beta/models/model:generateContent"}
	if err != nil || !reflect.DeepEqual(paths, want) {
		t.Fatalf("native-first default: %v %v", paths, err)
	}
	candidate.ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolResponses, store.DirectProtocolChat}
	paths, err = directCandidatePaths(candidate, "/v1/messages", "model", false)
	if err != nil || !reflect.DeepEqual(paths, []string{"/v1/responses", "/v1/chat/completions"}) {
		t.Fatalf("explicit restriction broadened: %v %v", paths, err)
	}
	if _, err := directCandidatePaths(candidate, "/v1/messages/count_tokens", "model", false); err == nil {
		t.Fatal("count_tokens crossed protocols")
	}
	candidate.Protocols = store.DirectProtocolChat
	paths, err = directCandidatePaths(candidate, "/v1/responses", "model", false)
	if err != nil || !reflect.DeepEqual(paths, []string{"/v1/chat/completions"}) {
		t.Fatalf("grant mask broadened: %v %v", paths, err)
	}
}
