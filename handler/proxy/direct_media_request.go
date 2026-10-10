package proxyhandler

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func directMultipartOverrides(model, raw string) (map[string]string, error) {
	values := map[string]string{"model": model}
	if raw == "" {
		return values, nil
	}
	var overrides map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &overrides) != nil || overrides == nil {
		return nil, fmt.Errorf("invalid parameter overrides")
	}
	for key, value := range overrides {
		if key == "model" || key == "stream" {
			continue
		}
		var text string
		if json.Unmarshal(value, &text) == nil && string(value) != "null" {
			values[key] = text
		} else {
			values[key] = string(value)
		}
	}
	return values, nil
}

func prepareJinaEmbeddings(body []byte) ([]byte, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("invalid Jina embeddings request")
	}
	if _, ok := obj["task"]; ok {
		return body, nil
	}
	obj["task"] = json.RawMessage(`"text-matching"`)
	return json.Marshal(obj)
}

// The URL selects the authorized embedding model. Batch items repeat that
// model as a resource name; opaque item fields remain RawMessage so numeric
// values and provider extensions survive mapping and parameter overrides.
func prepareGeminiEmbeddings(body []byte, path, model string) ([]byte, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("invalid Gemini embeddings request")
	}
	mapped, _ := json.Marshal("models/" + strings.TrimPrefix(model, "models/"))
	if _, exists := obj["model"]; exists {
		obj["model"] = mapped
	}
	if strings.HasSuffix(path, ":batchEmbedContents") {
		var requests []map[string]json.RawMessage
		if json.Unmarshal(obj["requests"], &requests) != nil || len(requests) == 0 {
			return nil, fmt.Errorf("Gemini batch embeddings requires a non-empty requests array")
		}
		for _, request := range requests {
			if request == nil {
				return nil, fmt.Errorf("Gemini batch embeddings requires object requests")
			}
			request["model"] = mapped
		}
		obj["requests"], _ = json.Marshal(requests)
	}
	return json.Marshal(obj)
}

func mediaContentType(h http.Header) string {
	mediaType, _, _ := mime.ParseMediaType(h.Get("Content-Type"))
	return strings.ToLower(mediaType)
}
func isJSONMediaResponse(h http.Header) bool {
	typ := mediaContentType(h)
	return typ == "application/json" || strings.HasSuffix(typ, "+json")
}
func isSSEMediaResponse(h http.Header) bool { return mediaContentType(h) == "text/event-stream" }

func mediaRequestPreservesBodyModel(path, method string) bool {
	return proxy.DirectProtocolForPath(path) == store.DirectProtocolVideo && (method != http.MethodPost || strings.HasSuffix(path, "/remix"))
}
