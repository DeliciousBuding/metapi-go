package toolusage

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestExtractCacheUsage(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		want          CacheUsage
	}{
		{"chat aliases", `{"usage":{"prompt_tokens_details":{"cached_tokens":7,"cache_write_tokens":8,"cached_creation_tokens":11,"cache_creation_input_tokens":9,"cache_write_tokens_details":{"ttl_12h_tokens":4,"ttl_30m_tokens":7}}}}`, CacheUsage{CacheReadTokens: 7, CacheCreationTokens: 11, CacheCreation12hTokens: 4}},
		{"response clamp", "event: response.completed\r\ndata: " + `{"response":{"usage":{"input_tokens_details":{"cache_write_tokens":3,"cache_creation_input_tokens":5,"cache_write_tokens_details":{"ttl_12h_tokens":7}}}}}` + "\r\n\r\n", CacheUsage{CacheCreationTokens: 5, CacheCreation12hTokens: 5}},
		{"no openai total", `{"usage":{"input_tokens_details":{"cache_write_tokens_details":{"ttl_12h_tokens":7}}}}`, CacheUsage{}},
		{"claude", `{"message":{"usage":{"cache_read_input_tokens":16,"cache_creation_input_tokens":32,"cache_creation":{"ephemeral_5m_input_tokens":12,"ephemeral_1h_input_tokens":20}}}}`, CacheUsage{CacheReadTokens: 16, CacheCreationTokens: 32, CacheCreation5mTokens: 12, CacheCreation1hTokens: 20}},
		{"claude split only", `{"usage":{"cache_creation":{"ephemeral_5m_input_tokens":12,"ephemeral_1h_input_tokens":20}}}`, CacheUsage{CacheCreationTokens: 32, CacheCreation5mTokens: 12, CacheCreation1hTokens: 20}},
		{"gemini", `{"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":40}}`, CacheUsage{CacheReadTokens: 40}},
		{"invalid alias alongside valid", `{"usage":{"input_tokens_details":{"cache_write_tokens":-1,"cached_creation_tokens":9,"cache_creation_input_tokens":1.5,"cached_tokens":"6"}}}`, CacheUsage{CacheCreationTokens: 9}},
		{"over limit", `{"usageMetadata":{"cachedContentTokenCount":1125899906842625}}`, CacheUsage{}},
		{"at limit", `{"usageMetadata":{"cachedContentTokenCount":1125899906842624}}`, CacheUsage{CacheReadTokens: 1 << 50}},
		{"split sum over limit", `{"usage":{"cache_creation":{"ephemeral_5m_input_tokens":1125899906842624,"ephemeral_1h_input_tokens":1}}}`, CacheUsage{}},
		{"trailing garbage", `{"usage":{"cache_creation_input_tokens":9}}!`, CacheUsage{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExtractCacheUsage([]byte(tc.payload)); got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestGroundingAcrossFrames(t *testing.T) {
	var tracker Tracker
	tracker.Observe([]byte(`{"candidates":[{"groundingMetadata":{"webSearchQueries":[" synthetic query one "," "]}},{"groundingMetadata":{"webSearchQueries":["synthetic query one","synthetic query two"]}}]}`))
	tracker.Observe([]byte("data: " + `{"candidates":[{"groundingMetadata":{"webSearchQueries":["synthetic query two","synthetic query three"]}}]}` + "\n\n"))
	want := []ToolUsageItem{{Tool: "grounding", Unit: "queries", Quantity: 3, Source: SourceResponseDerived}, {Tool: "grounding", Unit: "grounded_prompts", Quantity: 1, Source: SourceResponseDerived}}
	got := tracker.Snapshot()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %+v", got)
	}
	for _, value := range []any{got, &tracker} {
		b, err := json.Marshal(value)
		if err != nil || strings.Contains(string(b), "synthetic query") {
			t.Fatalf("private data or encoding error: %q, %v", b, err)
		}
	}
}

func TestHostedToolsAndVendorPrecedence(t *testing.T) {
	var tracker Tracker
	tracker.Observe([]byte(`{"tools":[{"type":"web_search"},{"type":"file_search"}],"content":[{"type":"tool_use","name":"web_search"}]}`))
	if got := tracker.Snapshot(); len(got) != 0 {
		t.Fatalf("declarations billed: %+v", got)
	}
	tracker.Observe([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"type":"file_search_call"}}`))
	tracker.Observe([]byte(`{"response":{"output":[{"type":"web_search_call","id":"w"},{"type":"file_search_call","id":"f"}]}}`))
	tracker.Observe([]byte(`{"type":"content_block_start","index":0,"content_block":{"type":"server_tool_use","name":"web_search","id":"s"}}`))
	tracker.Observe([]byte(`{"content":[{"type":"server_tool_use","name":"web_search","id":"s"}]}`))
	want := []ToolUsageItem{{Tool: "web_search", Unit: "calls", Quantity: 2, Source: SourceResponseDerived}, {Tool: "file_search", Unit: "calls", Quantity: 1, Source: SourceResponseDerived}}
	if got := tracker.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("derived: %+v", got)
	}
	tracker.Observe([]byte(`{"usage":{"server_tool_use":{"web_search_requests":4}}}`))
	if got := tracker.Snapshot()[0]; got.Quantity != 4 || got.Source != SourceVendorReported {
		t.Fatalf("vendor: %+v", got)
	}
	tracker.Observe([]byte(`{"usage":{"server_tool_use":{"web_search_requests":0}}}`))
	for _, invalid := range []string{"null", "-1", "1.5", "1125899906842625", `"3"`} {
		tracker.Observe([]byte(`{"usage":{"server_tool_use":{"web_search_requests":` + invalid + `}}}`))
	}
	tracker.Observe([]byte(`{"output":[{"type":"web_search_call","id":"later"}]}`))
	want[0].Quantity, want[0].Source = 0, SourceVendorReported
	if got := tracker.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("override: %+v", got)
	}
}
