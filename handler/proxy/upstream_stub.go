package proxyhandler

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"
)

func isProxyStubEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("METAPI_ENABLE_PROXY_STUB")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// writeStubResponse returns a local stub response only when explicitly enabled
// for tests or demos. Production defaults to 503 if upstream forwarding is not wired.
func writeStubResponse(w http.ResponseWriter, ctx *Ctx) {
	if ctx.IsStream {
		writeSSEHeaders(w)
		w.WriteHeader(200)
		flusher, _ := w.(http.Flusher)

		stubChunk := func(content string, finishReason any) []byte {
			payload, err := json.Marshal(map[string]any{
				"id":      "stub-metapi-go",
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   ctx.RequestedModel,
				"choices": []any{
					map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": finishReason},
				},
			})
			if err != nil {
				return []byte(`{"error":"stub marshal failed"}`)
			}
			return payload
		}

		_, _ = w.Write(sseEvent(string(stubChunk("Hello from Metapi Go (stub)", nil))))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write(sseEvent(string(stubChunk("", "stop"))))
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = w.Write(sseDone())
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	stubResp := map[string]any{
		"id":      "stub-metapi-go",
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   ctx.RequestedModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"message":       map[string]any{"role": "assistant", "content": "Hello from Metapi Go (stub)"},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
	writeJSON(w, 200, stubResp)
}
