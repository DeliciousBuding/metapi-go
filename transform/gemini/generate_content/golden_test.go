package generate_content

// golden_test.go pins OpenAI→Gemini request conversion and Gemini request
// normalization with checked-in snapshots (thought signatures, multimodal
// placeholders, tool calls, thinking config, field filtering).
// Regenerate snapshots with GOLDEN_UPDATE=1 go test ./transform/gemini/generate_content.

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

type buildInput struct {
	ModelName string         `json:"modelName"`
	Body      map[string]any `json:"body"`
}

func TestGoldenBuildGeminiRequestFromOpenAI(t *testing.T) {
	cases := []string{
		"basic_chat",
		"multimodal_image",
		"tool_calls_signed_gemini3",
		"tool_calls_dummy_signature",
		"tool_calls_nonsafe_model",
		"tools_and_choice",
		"reasoning_content_thought",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			var input buildInput
			golden.ReadInput(t, "request", name, &input)

			got := BuildGeminiGenerateContentRequestFromOpenAi(input.Body, input.ModelName)
			golden.Check(t, "request", name, got)
		})
	}
}

func TestGoldenToolChoiceMatrix(t *testing.T) {
	var input struct {
		Cases []any `json:"cases"`
	}
	golden.ReadInput(t, "request", "tool_choice_matrix", &input)

	out := make([]any, 0, len(input.Cases))
	for _, tc := range input.Cases {
		body := map[string]any{
			"model": "gemini-2.5-flash",
			"messages": []any{
				map[string]any{"role": "user", "content": "pick a tool"},
			},
			"tool_choice": tc,
		}
		got := BuildGeminiGenerateContentRequestFromOpenAi(body, "")
		out = append(out, got["toolConfig"])
	}
	golden.Check(t, "request", "tool_choice_matrix", out)
}

func TestGoldenThinkingConfigMatrix(t *testing.T) {
	var input struct {
		Cases []buildInput `json:"cases"`
	}
	golden.ReadInput(t, "request", "thinking_config_matrix", &input)

	out := make([]any, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, ResolveGeminiThinkingConfigFromRequest(c.ModelName, c.Body))
	}
	golden.Check(t, "request", "thinking_config_matrix", out)
}

func TestGoldenNormalizeRequest(t *testing.T) {
	cases := []string{
		"normalize_field_filter",
		"normalize_thinking_merge",
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			var input buildInput
			golden.ReadInput(t, "request", name, &input)

			got := NormalizeRequest(input.Body, input.ModelName)
			golden.Check(t, "request", name, got)
		})
	}
}
