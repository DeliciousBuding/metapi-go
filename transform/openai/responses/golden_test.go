package responses

// golden_test.go pins Responses API request sanitization and continuity
// policy decisions with checked-in snapshots.
// Regenerate snapshots with GOLDEN_UPDATE=1 go test ./transform/openai/responses.

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

type continuityGolden struct {
	Action        string `json:"action"`
	Reason        string `json:"reason"`
	ClientMessage string `json:"clientMessage"`
}

func decisionGolden(d ContinuityDecision) continuityGolden {
	return continuityGolden{Action: string(d.Action), Reason: d.Reason, ClientMessage: d.ClientMessage}
}

func policyFromFixture(p struct {
	SitePlatform      string `json:"sitePlatform"`
	Protocol          string `json:"protocol"`
	UpstreamPath      string `json:"upstreamPath"`
	IsCompactRequest  bool   `json:"isCompactRequest"`
	RequireContinuity bool   `json:"requireContinuity"`
}) ContinuityPolicyInput {
	return ContinuityPolicyInput{
		SitePlatform:      p.SitePlatform,
		Protocol:          UpstreamProtocol(p.Protocol),
		UpstreamPath:      p.UpstreamPath,
		IsCompactRequest:  p.IsCompactRequest,
		RequireContinuity: p.RequireContinuity,
	}
}

func TestGoldenSanitizeResponsesRequestBody(t *testing.T) {
	cases := []string{
		"forward_continuity",
		"strip_unknown_platform",
		"chat_fallback",
		"compact_codex",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			var input struct {
				Body   map[string]any `json:"body"`
				Policy struct {
					SitePlatform      string `json:"sitePlatform"`
					Protocol          string `json:"protocol"`
					UpstreamPath      string `json:"upstreamPath"`
					IsCompactRequest  bool   `json:"isCompactRequest"`
					RequireContinuity bool   `json:"requireContinuity"`
				} `json:"policy"`
			}
			golden.ReadInput(t, "request", "sanitize_"+name, &input)

			got, decision, err := SanitizeResponsesRequestBody(input.Body, policyFromFixture(input.Policy))
			out := struct {
				Body     map[string]any   `json:"body"`
				Decision continuityGolden `json:"decision"`
				Error    string           `json:"error"`
			}{Body: got, Decision: decisionGolden(decision)}
			if err != nil {
				out.Error = err.Error()
			}
			golden.Check(t, "request", "sanitize_"+name, out)
		})
	}
}

func TestGoldenSanitizeResponsesInputItems(t *testing.T) {
	var input struct {
		Input any `json:"input"`
	}
	golden.ReadInput(t, "request", "sanitize_reasoning_items_matrix", &input)

	got, err := SanitizeResponsesInputItems(input.Input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	golden.Check(t, "request", "sanitize_reasoning_items_matrix", got)
}

func TestGoldenSanitizeResponsesInputItems_Error(t *testing.T) {
	var input struct {
		Input any `json:"input"`
	}
	golden.ReadInput(t, "request", "sanitize_reasoning_items_error", &input)

	_, err := SanitizeResponsesInputItems(input.Input)
	if err == nil {
		t.Fatal("expected ReasoningInputError, got nil")
	}
	re, ok := err.(*ReasoningInputError)
	if !ok {
		t.Fatalf("expected *ReasoningInputError, got %T", err)
	}
	golden.Check(t, "request", "sanitize_reasoning_items_error", struct {
		Error  string `json:"error"`
		Reason string `json:"reason"`
		Index  int    `json:"index"`
	}{Error: re.Error(), Reason: re.Reason, Index: re.Index})
}

func TestGoldenResolveContinuityPolicyMatrix(t *testing.T) {
	var input struct {
		Cases []struct {
			SitePlatform      string `json:"sitePlatform"`
			Protocol          string `json:"protocol"`
			UpstreamPath      string `json:"upstreamPath"`
			IsCompactRequest  bool   `json:"isCompactRequest"`
			RequireContinuity bool   `json:"requireContinuity"`
		} `json:"cases"`
	}
	golden.ReadInput(t, "decision", "resolve_continuity_policy_matrix", &input)

	out := make([]continuityGolden, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, decisionGolden(ResolvePreviousResponseIDPolicy(policyFromFixture(c))))
	}
	golden.Check(t, "decision", "resolve_continuity_policy_matrix", out)
}

func TestGoldenNormalizeUpstreamProtocolMatrix(t *testing.T) {
	var input struct {
		Cases []struct {
			Protocol     string `json:"protocol"`
			UpstreamPath string `json:"upstreamPath"`
		} `json:"cases"`
	}
	golden.ReadInput(t, "decision", "normalize_upstream_protocol_matrix", &input)

	out := make([]string, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, string(NormalizeUpstreamProtocol(UpstreamProtocol(c.Protocol), c.UpstreamPath)))
	}
	golden.Check(t, "decision", "normalize_upstream_protocol_matrix", out)
}

func TestGoldenFallbackCompactMatrix(t *testing.T) {
	var input struct {
		Cases []struct {
			Status      int    `json:"status"`
			RawErrText  string `json:"rawErrText"`
			RequestPath string `json:"requestPath"`
		} `json:"cases"`
	}
	golden.ReadInput(t, "decision", "fallback_compact_matrix", &input)

	out := make([]bool, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, ShouldFallbackCompactResponsesToResponses(c.Status, c.RawErrText, c.RequestPath))
	}
	golden.Check(t, "decision", "fallback_compact_matrix", out)
}

func TestGoldenPlatformCapabilityMatrix(t *testing.T) {
	var input struct {
		Platforms []string `json:"platforms"`
	}
	golden.ReadInput(t, "decision", "platform_capability_matrix", &input)

	out := make([]any, 0, len(input.Platforms))
	for _, p := range input.Platforms {
		out = append(out, struct {
			SupportsPreviousResponseID    bool `json:"supportsPreviousResponseID"`
			StripCompactStore             bool `json:"stripCompactStore"`
			ForceUpstreamStreamNonCompact bool `json:"forceUpstreamStreamNonCompact"`
			ForceUpstreamStreamCompact    bool `json:"forceUpstreamStreamCompact"`
		}{
			SupportsResponsesPreviousResponseID(p),
			ShouldStripCompactResponsesStore(p),
			ShouldForceResponsesUpstreamStream(p, false),
			ShouldForceResponsesUpstreamStream(p, true),
		})
	}
	golden.Check(t, "decision", "platform_capability_matrix", out)
}

func TestGoldenUnsupportedPreviousResponseIDErrors(t *testing.T) {
	var input struct {
		Cases []string `json:"cases"`
	}
	golden.ReadInput(t, "decision", "unsupported_previous_response_id_errors", &input)

	out := make([]bool, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, IsUnsupportedPreviousResponseIDError(c))
	}
	golden.Check(t, "decision", "unsupported_previous_response_id_errors", out)
}

func TestGoldenEnsureCompactAcceptHeaders(t *testing.T) {
	var input struct {
		Cases []struct {
			Headers  map[string]string `json:"headers"`
			Platform string            `json:"platform"`
		} `json:"cases"`
	}
	golden.ReadInput(t, "decision", "ensure_compact_accept_headers", &input)

	out := make([]map[string]string, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, EnsureCompactResponsesJSONAcceptHeader(c.Headers, c.Platform))
	}
	golden.Check(t, "decision", "ensure_compact_accept_headers", out)
}
