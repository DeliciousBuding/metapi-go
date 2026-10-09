package admin

import "testing"

func TestProxyLogCacheProjectionKeepsUnknownAndInvalidUnobserved(t *testing.T) {
	for _, raw := range []string{
		`{"usage_source":"unknown","usage":{"cache_read_tokens":0,"cache_creation_tokens":0}}`,
		`{"usage":{"cache_read_tokens":-1,"cache_creation_tokens":1.5}}`,
		`{"usage":{}}`, `invalid JSON`,
	} {
		row := map[string]any{"totalTokens": 100.0, "estimatedCost": 0.0}
		projectProxyLogCacheUsage(row, decodeProxyLogBilling(raw))
		if _, ok := row["cacheReadTokens"]; ok {
			t.Fatalf("invalid/missing cache reads became a measurement: %v", row)
		}
		if _, ok := row["cacheCreationTokens"]; ok {
			t.Fatalf("invalid/missing cache writes became a measurement: %v", row)
		}
		if row["totalTokens"] != 100.0 {
			t.Fatal("cache tokens were added to total usage")
		}
		if row["usageSource"] == "unknown" && row["estimatedCost"] != nil {
			t.Fatal("unknown zero-cost fallback was presented as a measured free request")
		}
	}
}
