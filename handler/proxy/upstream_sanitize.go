package proxyhandler

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	generate_content "github.com/deliciousbuding/metapi-go/transform/gemini/generate_content"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func sanitizeUpstreamJSONBody(bodyBytes []byte, sitePlatform, upstreamPath, upstreamModel string) ([]byte, error) {
	if len(bodyBytes) == 0 {
		return bodyBytes, nil
	}
	// Cheap gate: only rewrite when continuity, compact, multi-turn reasoning
	// input, or Gemini tool-history signature inject is involved. Avoid full
	// JSON parse on hot path otherwise.
	pathLower := strings.ToLower(upstreamPath)
	needsCompact := strings.Contains(pathLower, "/responses/compact")
	isResponsesPath := needsCompact || strings.Contains(pathLower, "/responses")
	needsContinuity := bytes.Contains(bodyBytes, []byte(`"previous_response_id"`))
	// Multi-turn Hermes/Codex ():
	// 1) Exact compact markers (common client encoding).
	// 2) Responses path + "input" always parse - covers pretty-printed /
	// spaced JSON where "type" : "reasoning" does not match contiguous bytes.
	// 3) encrypted_content anywhere (continuity payload even without type key).
	needsReasoningInput := bytes.Contains(bodyBytes, []byte(`"type":"reasoning"`)) ||
		bytes.Contains(bodyBytes, []byte(`"type": "reasoning"`)) ||
		bytes.Contains(bodyBytes, []byte(`"encrypted_content"`)) ||
		(isResponsesPath && bytes.Contains(bodyBytes, []byte(`"input"`)))
	needsGeminiThought := needsGeminiThoughtSignatureSanitize(sitePlatform, upstreamPath, bodyBytes)
	if !needsCompact && !needsContinuity && !needsReasoningInput && !needsGeminiThought {
		return bodyBytes, nil
	}
	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		return bodyBytes, nil
	}
	next := body
	if needsCompact || needsContinuity || needsReasoningInput {
		protocol := responses.ProtocolUnknown
		switch ep, ok := proxy.EndpointFromPath(upstreamPath); {
		case ok && ep == proxy.EndpointChat:
			protocol = responses.ProtocolChat
		case ok && ep == proxy.EndpointMessages:
			protocol = responses.ProtocolMessages
		case ok && ep == proxy.EndpointResponses:
			protocol = responses.ProtocolResponses
		case needsCompact || strings.Contains(pathLower, "/responses"):
			protocol = responses.ProtocolResponses
		}
		sanitized, _, err := responses.SanitizeResponsesRequestBody(next, responses.ContinuityPolicyInput{
			SitePlatform:     sitePlatform,
			Protocol:         protocol,
			UpstreamPath:     upstreamPath,
			IsCompactRequest: needsCompact,
		})
		if err != nil {
			return nil, err
		}
		next = sanitized
	}
	if needsGeminiThought {
		next = applyGeminiThoughtSignatureSanitize(next, upstreamPath, upstreamModel)
	}
	out, marshalErr := json.Marshal(next)
	if marshalErr != nil {
		return bodyBytes, nil
	}
	return out, nil
}

// needsGeminiThoughtSignatureSanitize is a cheap byte/path gate for official
// Gemini native generateContent / gemini-cli tool-history inject. Avoids parse
// when the body clearly has no functionCall / tool_calls tool history.
func needsGeminiThoughtSignatureSanitize(sitePlatform, upstreamPath string, bodyBytes []byte) bool {
	if !isGeminiThoughtSignaturePlatform(sitePlatform) {
		return false
	}
	if !isGeminiNativeGenerateContentPath(upstreamPath) {
		return false
	}
	// Tool-history markers only (functionCall native or OpenAI tool_calls).
	return bytes.Contains(bodyBytes, []byte(`"functionCall"`)) ||
		bytes.Contains(bodyBytes, []byte(`"function_call"`)) ||
		bytes.Contains(bodyBytes, []byte(`"tool_calls"`))
}

func isGeminiThoughtSignaturePlatform(sitePlatform string) bool {
	switch strings.ToLower(strings.TrimSpace(sitePlatform)) {
	case "gemini", "gemini-cli", "google":
		return true
	default:
		return false
	}
}

// isGeminiNativeGenerateContentPath reports paths that carry Gemini contents
// (official generateContent / streamGenerateContent / gemini-cli v1internal).
// OpenAI-compat chat/completions paths are intentionally excluded — they keep
// OpenAI shape and do not accept native thoughtSignature inject.
func isGeminiNativeGenerateContentPath(upstreamPath string) bool {
	pathLower := strings.ToLower(strings.TrimSpace(upstreamPath))
	if pathLower == "" {
		return false
	}
	if strings.Contains(pathLower, "generatecontent") ||
		strings.Contains(pathLower, "streamgeneratecontent") {
		return true
	}
	// Gemini CLI internal: /v1internal::generateContent (double-colon in routes)
	// and /v1internal:generateContent (single-colon detect form).
	if strings.Contains(pathLower, "/v1internal") {
		return true
	}
	return false
}

// applyGeminiThoughtSignatureSanitize rewrites a Gemini-shaped (or CLI-wrapped)
// request so tool-history functionCall parts carry thoughtSignature.
// Prefer real signatures already present; inject dummy for Gemini 3.x /
// thinking-enabled models via generate_content.NormalizeRequest.
// When the client posts OpenAI messages onto a native generateContent path,
// rebuild via BuildGeminiGenerateContentRequestFromOpenAi.
// Known limitation: process-local aggregate ThoughtSignatures are not re-attached here
// (no multi-instance session store); clients must echo provider_specific_fields
// or send native contents that NormalizeRequest can patch.
func applyGeminiThoughtSignatureSanitize(body map[string]any, upstreamPath, upstreamModel string) map[string]any {
	if body == nil {
		return body
	}
	// gemini-cli envelope: { "model", "request": { contents... } }
	if req, ok := body["request"].(map[string]any); ok && req != nil {
		modelName := resolveGeminiSanitizeModel(body, req, upstreamPath, upstreamModel)
		inner := applyGeminiThoughtSignatureSanitizeBody(req, modelName)
		if inner != nil {
			// Clone top-level map so we do not mutate the unmarshaled input in place
			// when callers share maps (tests / retries).
			out := cloneJSONMapShallow(body)
			out["request"] = inner
			if sharedModel := strings.TrimSpace(asJSONString(out["model"])); sharedModel == "" && modelName != "" {
				out["model"] = modelName
			}
			return out
		}
		return body
	}
	modelName := resolveGeminiSanitizeModel(body, nil, upstreamPath, upstreamModel)
	return applyGeminiThoughtSignatureSanitizeBody(body, modelName)
}

func applyGeminiThoughtSignatureSanitizeBody(body map[string]any, modelName string) map[string]any {
	if body == nil {
		return body
	}
	// Native Gemini: contents present → NormalizeRequest (inject/preserve).
	if _, hasContents := body["contents"]; hasContents {
		return generate_content.NormalizeRequest(body, modelName)
	}
	// OpenAI chat body posted on a native generateContent path (rare bridge).
	if _, hasMessages := body["messages"]; hasMessages {
		return generate_content.BuildGeminiGenerateContentRequestFromOpenAi(body, modelName)
	}
	return body
}

func resolveGeminiSanitizeModel(body, nested map[string]any, upstreamPath, upstreamModel string) string {
	if m := strings.TrimSpace(upstreamModel); m != "" {
		return m
	}
	if nested != nil {
		if m := asJSONString(nested["model"]); m != "" {
			return m
		}
	}
	if body != nil {
		if m := asJSONString(body["model"]); m != "" {
			return m
		}
	}
	_, model, _ := ParseGeminiPath(upstreamPath)
	return strings.TrimSpace(model)
}

func asJSONString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func cloneJSONMapShallow(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// swapModelInJSON performs a shallow JSON re-encode to replace the "model" field.
// It uses json.RawMessage to avoid deep-unmarshalling nested values, then sets
// the model key and marshals once. This replaces the previous approach of:

//	cloneAndSetModel(ctx.Body) -> json.Marshal

// which allocated a full map copy PLUS serialized the deep structure twice.
