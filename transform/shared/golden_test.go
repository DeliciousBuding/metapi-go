package shared

// golden_test.go pins the shared protocol helpers with checked-in snapshots.
// Regenerate snapshots with GOLDEN_UPDATE=1 go test ./transform/shared.

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

func TestGoldenAsTrimmedString(t *testing.T) {
	var input struct {
		Cases []any `json:"cases"`
	}
	golden.ReadInput(t, "unit", "as_trimmed_string", &input)

	out := make([]string, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, AsTrimmedString(c))
	}
	golden.Check(t, "unit", "as_trimmed_string", out)
}

func TestGoldenParseJSONLike(t *testing.T) {
	var input struct {
		Cases []string `json:"cases"`
	}
	golden.ReadInput(t, "unit", "parse_json_like", &input)

	out := make([]any, 0, len(input.Cases))
	for _, c := range input.Cases {
		out = append(out, ParseJSONLike(c))
	}
	golden.Check(t, "unit", "parse_json_like", out)
}
