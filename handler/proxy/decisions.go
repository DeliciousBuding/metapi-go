package proxyhandler

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Decisions is an independent synchronous JSON surface. It never acquires a
// generation/stream grant or rewrites input and questions into Chat messages.
func HandleDecisions(w http.ResponseWriter, r *http.Request) {
	ctx, failure := PrepareCtx(r, SurfConfig{Endpoint: "native-json", DownstreamPath: "/v1/decisions", RequireModel: true})
	if failure != nil {
		writeJSONError(w, failure.Status, failure.Error, failure.ErrorType)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if ctx.Multipart || contentType != "" && !strings.Contains(strings.ToLower(contentType), "application/json") {
		writeJSONError(w, 400, "Decisions requires a JSON request", "invalid_request_error")
		return
	}
	if stream := ctx.Body["stream"]; stream != nil && stream != false {
		writeJSONError(w, 400, "Decisions does not support streaming", "invalid_request_error")
		return
	}
	questions, ok := ctx.Body["questions"].([]any)
	if !ok || len(questions) == 0 || !validDecisionsInput(ctx.Body["input"]) {
		writeJSONError(w, 400, "Decisions requires supported input and a nonempty questions array", "invalid_request_error")
		return
	}
	dispatchUpstream(w, r, ctx)
}

func validDecisionsInput(input any) bool {
	switch value := input.(type) {
	case string:
		return value != ""
	case []any:
		return validDecisionsItems(value)
	default:
		return false
	}
}

func validDecisionsItems(items []any) bool {
	if len(items) == 0 {
		return false
	}
	// The JSON decoder already bounds nesting. Visit message content using a
	// work list so deeply nested source-compatible messages do not recurse.
	pending := append([]any(nil), items...)
	for len(pending) > 0 {
		item := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		object, ok := item.(map[string]any)
		if !ok {
			return false
		}
		switch object["type"] {
		case "input_text":
			if text, ok := object["text"].(string); !ok || text == "" {
				return false
			}
		case "input_image":
			if image, ok := object["image_url"].(string); !ok || image == "" {
				return false
			}
		case "message":
			switch content := object["content"].(type) {
			case string:
				if content == "" {
					return false
				}
			case []any:
				if len(content) == 0 {
					return false
				}
				pending = append(pending, content...)
			default:
				return false
			}
		default:
			return false
		}
	}
	return true
}

func decisionsResponseShape(body []byte) (valid, hasOutput bool) {
	// Match the audited source's typed response fields while preserving the
	// original bytes and extensions. Missing usage remains unknown.
	var wire struct {
		Model   string            `json:"model"`
		Answers []json.RawMessage `json:"answers"`
		Usage   *struct {
			InputTokens int64 `json:"input_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &wire) != nil {
		return false, false
	}
	return true, len(wire.Answers) > 0
}
