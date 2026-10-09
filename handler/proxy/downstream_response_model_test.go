package proxyhandler

import (
	"io"
	"strings"
	"testing"
)

func TestDownstreamResponseModelSSEPreservesUpstreamObservation(t *testing.T) {
	for _, tc := range []struct{ path, frame string }{
		{"/v1/chat/completions", `data: {"model":"provider-model","choices":[{"index":0,"delta":{"content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}` + "\n\ndata: [DONE]\n\n"},
		{"/v1/responses", `event: response.completed` + "\n" + `data: {"type":"response.completed","response":{"id":"fixture","model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}}` + "\n\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			body := withNativeTerminalBody(io.NopCloser(strings.NewReader(tc.frame)), tc.path, "client-alias")
			out, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(out), `"model":"client-alias"`) || strings.Contains(string(out), `"model":"provider-model"`) {
				t.Fatalf("model alias not restored: %s", out)
			}
			native := body.(*nativeTerminalBody)
			usage := native.original.Result().Usage
			if usage.TotalTokens != 7 || usage.UpstreamReportedModel == nil || *usage.UpstreamReportedModel != "provider-model" {
				t.Fatalf("upstream usage/provenance changed: %+v", usage)
			}
		})
	}
	frame := `data: {"model":"provider-model","choices":[],"x":"` + strings.Repeat("x", maxIncrementalSsePendingBytes) + `"}` + "\n\n"
	if out, err := io.ReadAll(withNativeTerminalBody(io.NopCloser(strings.NewReader(frame)), "/v1/responses", "client-alias")); err == nil || len(out) != 0 {
		t.Fatal("oversized mapping frame must fail without forwarding unrewritten data")
	}
}

func TestDownstreamResponseModelLeavesToolPayloadAlone(t *testing.T) {
	raw := []byte(`{"model":"provider-model","choices":[{"message":{"content":"receipt","tool_calls":[{"function":{"arguments":"{\"model\":\"tool-model\"}"}}]}}]}`)
	out := string(restoreDownstreamResponseModel(raw, "alias"))
	if !strings.Contains(out, `"model":"alias"`) || !strings.Contains(out, `tool-model`) {
		t.Fatal("model rewrite touched user payload")
	}
	if string(restoreDownstreamResponseModel(raw, "")) != string(raw) {
		t.Fatal("unconfigured key response changed")
	}
}
