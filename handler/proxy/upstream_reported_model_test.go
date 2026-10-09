package proxyhandler

import "testing"

func TestUpstreamReportedModelDoesNotInventUsageOrIdentity(t *testing.T) {
	for _, tc := range []struct{ body, model string }{
		{`{"model":"gpt-6"}`, "gpt-6"},
		{`{"response":{"model":"gpt-6"}}`, "gpt-6"},
		{`{"message":{"model":"claude-opus-5-5"}}`, "claude-opus-5-5"},
		{`{"modelVersion":"gemini-3.8-flash"}`, "gemini-3.8-flash"},
		{`{"choices":[{"message":{"model":"user-content"}}]}`, ""},
		{`{"error":{"message":"failure"},"model":"error-only"}`, ""},
		{`{"model":"line\nbreak"}`, ""},
		{`{"model":5}`, ""},
		{`{}`, ""},
	} {
		got := ParseUsageFromBody([]byte(tc.body))
		if got.Found || got.Source != usageSourceUnknown {
			t.Fatalf("model metadata invented usage: %+v", got)
		}
		if tc.model == "" {
			if got.UpstreamReportedModel != nil {
				t.Fatalf("unexpected model: %s", *got.UpstreamReportedModel)
			}
		} else if got.UpstreamReportedModel == nil || *got.UpstreamReportedModel != tc.model {
			t.Fatalf("model was not observed: %+v", got)
		}
	}
}

func TestStreamingModelMetadataSurvivesUsageOnlyFinalEvent(t *testing.T) {
	a := newIncrementalSseAnalyzer()
	a.Push([]byte("data: {\"message\":{\"model\":\"claude-opus-5-5\"}}\n\n"))
	a.Push([]byte("data: {\"usage\":{\"output_tokens\":8}}\n\n"))
	got := a.Result().Usage
	if got.UpstreamReportedModel == nil || *got.UpstreamReportedModel != "claude-opus-5-5" || !got.Found || got.CompletionTokens != 8 {
		t.Fatalf("metadata lost at final usage: %+v", got)
	}
}
