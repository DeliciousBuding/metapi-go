package images

// golden_test.go pins the pass-through contract with checked-in snapshots:
// ParseRequest/ParseResponse must remain byte/JSON identity transforms.
// Regenerate snapshots with GOLDEN_UPDATE=1 go test ./transform/openai/images.

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

func TestGoldenParseRequest(t *testing.T) {
	var body map[string]any
	golden.ReadInput(t, "request", "passthrough", &body)

	got, err := ParseRequest(body)
	if err != nil {
		t.Fatalf("ParseRequest: %v", err)
	}
	golden.Check(t, "request", "passthrough", got)
}

func TestGoldenParseResponse(t *testing.T) {
	raw := golden.ReadRawInput(t, "response", "passthrough", ".json")

	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("ParseResponse: %v", err)
	}
	golden.CheckBytes(t, "response", "passthrough", ".json", got)
}

// Stream chunks never route through this package in production (the SSE relay
// copies bytes); the snapshot pins that ParseResponse leaves arbitrary stream
// bytes untouched so a future "improvement" cannot silently start rewriting
// chunks that pass through this layer.
func TestGoldenParseResponse_StreamBytes(t *testing.T) {
	raw := golden.ReadRawInput(t, "stream", "passthrough", ".sse")

	got, err := ParseResponse(raw)
	if err != nil {
		t.Fatalf("ParseResponse(stream bytes): %v", err)
	}
	golden.CheckBytes(t, "stream", "passthrough", ".sse", got)
}
