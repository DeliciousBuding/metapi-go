package store

import "testing"

func TestKeyRoutingPolicyValidationAndRegexSubset(t *testing.T) {
	for _, raw := range []string{
		`{"routingStrategy":"weighted","traceStickyMode":"prefer_previous_channel"}`,
		`{"routingStrategy":"default","traceStickyMode":"disabled"}`,
		`{"routingStrategy":"system_default","traceStickyMode":"default"}`,
	} {
		if _, err := ParseDownstreamAccessPolicy(raw); err != nil {
			t.Fatalf("valid policy rejected: %v", err)
		}
	}
	for _, raw := range []string{
		`{"routingStrategy":"adaptive"}`, `{"traceStickyMode":"always"}`,
		`{"modelMappings":[{"from":"(?=gpt).*","to":"fixed"}]}`,
		`{"modelMappings":[{"from":"(a)\\1","to":"fixed"}]}`,
	} {
		if _, err := ParseDownstreamAccessPolicy(raw); err == nil {
			t.Fatalf("unsupported policy accepted: %s", raw)
		}
	}
	// Vectors checked against AxonHub d96748f1 internal/pkg/xregexp.MatchString.
	for _, vector := range []struct {
		pattern, model string
		match          bool
	}{
		{"gpt-4", "gpt-4", true}, {"gpt-4", "gpt-4o", false},
		{"*", "anything", true}, {"gpt-.*", "gpt-4o", true}, {"gpt-.*", "prefix-gpt-4o", false},
		{"(?i)gpt-.*", "GPT-4", true}, {"^gpt-(4|5)$", "gpt-5", true},
		{"model-[0-9]+", "model-12", true}, {"model-[0-9]+", "model-x", false},
	} {
		policy := &DownstreamAccessPolicy{ModelMappings: []DownstreamModelMapping{{From: vector.pattern, To: "first"}, {From: "*", To: "second"}}}
		want := "second"
		if vector.match {
			want = "first"
		}
		if got := policy.MapModel(vector.model); got != want {
			t.Fatalf("%s/%s mapped to %s want %s", vector.pattern, vector.model, got, want)
		}
	}
}
