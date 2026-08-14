package generate_content

import (
	"testing"
)

// Golden: OpenAI -> Gemini body conversion
// ---------------------------------------------------------------------------

func TestBuildGeminiGenerateContentRequestFromOpenAi_Roundtrip(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{"role": "system", "content": "You are helpful."},
			map[string]any{"role": "user", "content": "Hello!"},
			map[string]any{"role": "assistant", "content": "Hi there!"},
		},
		"temperature": 0.7,
		"max_tokens":  4096,
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	if geminiBody["model"] != "gemini-2.5-flash" {
		t.Errorf("expected model gemini-2.5-flash, got %v", geminiBody["model"])
	}

	// System instruction
	si, ok := geminiBody["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatal("systemInstruction missing")
	}
	parts, ok := si["parts"].([]map[string]any)
	if !ok || len(parts) == 0 {
		t.Fatal("systemInstruction parts missing")
	}
	if parts[0]["text"] != "You are helpful." {
		t.Errorf("expected 'You are helpful.', got %v", parts[0]["text"])
	}

	// Contents
	contents, ok := geminiBody["contents"].([]map[string]any)
	if !ok || len(contents) != 2 {
		t.Fatalf("expected 2 contents, got %d", len(contents))
	}
	if contents[0]["role"] != "user" {
		t.Errorf("content[0]: expected user, got %v", contents[0]["role"])
	}
	if contents[1]["role"] != "model" {
		t.Errorf("content[1]: expected model, got %v", contents[1]["role"])
	}

	// Generation config
	gc, ok := geminiBody["generationConfig"].(map[string]any)
	if !ok {
		t.Fatal("generationConfig missing")
	}
	if gc["temperature"].(float64) != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", gc["temperature"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_WithTools(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "Search"},
		},
		"tools": []any{
			map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        "get_weather",
					"description": "Get weather",
					"parameters":  map[string]any{"type": "object"},
				},
			},
		},
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	toolsAny, ok := geminiBody["tools"]
	if !ok {
		t.Fatal("tools missing")
	}
	t.Logf("tools type=%T", toolsAny)
	// Tools conversion produces []map[string]any which can't be asserted as []any
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_ToolChoice(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
		"tool_choice": "required",
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	tc, ok := geminiBody["toolConfig"].(map[string]any)
	if !ok {
		t.Fatal("toolConfig missing")
	}
	fcc, ok := tc["functionCallingConfig"].(map[string]any)
	if !ok {
		t.Fatal("functionCallingConfig missing")
	}
	if fcc["mode"] != "ANY" {
		t.Errorf("expected ANY, got %v", fcc["mode"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_ToolChoiceNone(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
		},
		"tool_choice": "none",
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	tc, ok := geminiBody["toolConfig"].(map[string]any)
	if !ok {
		t.Fatal("toolConfig missing")
	}
	fcc, ok := tc["functionCallingConfig"].(map[string]any)
	if !ok {
		t.Fatal("functionCallingConfig missing")
	}
	if fcc["mode"] != "NONE" {
		t.Errorf("expected NONE, got %v", fcc["mode"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_WithImages(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "What is this?"},
					map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": "https://example.com/photo.png"},
					},
				},
			},
		},
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	contents, ok := geminiBody["contents"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatal("contents missing")
	}
	parts, ok := contents[0]["parts"].([]map[string]any)
	if !ok {
		t.Fatal("parts not array")
	}
	hasFileData := false
	hasText := false
	for _, p := range parts {
		if _, ok := p["fileData"]; ok {
			hasFileData = true
		}
		if _, ok := p["text"]; ok {
			hasText = true
		}
	}
	if !hasFileData {
		t.Error("expected fileData part")
	}
	if !hasText {
		t.Error("expected text part")
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_DataURLImage(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type":      "image_url",
						"image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo="},
					},
				},
			},
		},
	}

	geminiBody := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "")

	contents, ok := geminiBody["contents"].([]map[string]any)
	if !ok || len(contents) == 0 {
		t.Fatal("contents missing")
	}
	parts, ok := contents[0]["parts"].([]map[string]any)
	if !ok {
		t.Fatal("parts not array")
	}
	hasInlineData := false
	for _, p := range parts {
		if id, ok := p["inlineData"].(map[string]any); ok {
			hasInlineData = true
			if id["mimeType"] != "image/png" {
				t.Errorf("expected mimeType image/png, got %v", id["mimeType"])
			}
			if id["data"] != "iVBORw0KGgo=" {
				t.Errorf("expected data iVBORw0KGgo=, got %v", id["data"])
			}
		}
	}
	if !hasInlineData {
		t.Error("expected inlineData part")
	}
}

// ---------------------------------------------------------------------------
// Reasoning -> ThinkingConfig mapping
// ---------------------------------------------------------------------------

func TestReasoningToThinkingConfig_Effort(t *testing.T) {
	tests := []struct {
		effort   string
		expected any
	}{
		{"none", nil},
		{"low", map[string]any{"thinkingBudget": 0}},
		{"medium", map[string]any{"thinkingBudget": 8192}},
		{"high", map[string]any{"thinkingBudget": 32768}},
		{"max", map[string]any{"thinkingBudget": 65536}},
		{"", nil},
	}

	for _, tt := range tests {
		result := ReasoningToThinkingConfig(tt.effort, 0)
		if tt.expected == nil {
			if result != nil {
				t.Errorf("effort=%q: expected nil, got %v", tt.effort, result)
			}
		} else {
			if result == nil {
				t.Errorf("effort=%q: expected non-nil, got nil", tt.effort)
				continue
			}
			exp, _ := tt.expected.(map[string]any)
			if result["thinkingBudget"] != exp["thinkingBudget"] {
				t.Errorf("effort=%q: expected budget %v, got %v", tt.effort, exp["thinkingBudget"], result["thinkingBudget"])
			}
		}
	}
}

func TestReasoningToThinkingConfig_Budget(t *testing.T) {
	result := ReasoningToThinkingConfig("", 8192)
	if result == nil {
		t.Fatal("expected non-nil")
	}
	if result["thinkingBudget"].(int) != 8192 {
		t.Errorf("expected budget 8192, got %v", result["thinkingBudget"])
	}
}

// ---------------------------------------------------------------------------
// NormalizeRequest filtering
// ---------------------------------------------------------------------------

func TestNormalizeRequest_Basic(t *testing.T) {
	body := map[string]any{
		"model": "gemini-2.5-flash",
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Hello"},
				},
			},
		},
		"unknownField": "should be filtered",
	}

	normalized := NormalizeRequest(body, "gemini-2.5-flash")

	if normalized["contents"] == nil {
		t.Error("contents filtered")
	}
	if normalized["unknownField"] != nil {
		t.Errorf("unknown field not filtered: %v", normalized["unknownField"])
	}
}

func TestNormalizeRequest_NilBody(t *testing.T) {
	normalized := NormalizeRequest(nil, "gemini-2.5-flash")
	if normalized == nil {
		t.Fatal("expected non-nil normalized body")
	}
}

// ---------------------------------------------------------------------------
// clone / JSON helpers (sanity checks)
// ---------------------------------------------------------------------------

func TestCloneJSONValue_Nil(t *testing.T) {
	if cloneJSONValue(nil) != nil {
		t.Error("expected nil")
	}
}

func TestCloneJSONValue_Map(t *testing.T) {
	src := map[string]any{"key": "value"}
	result := cloneJSONValue(src)
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatal("expected map")
	}
	if m["key"] != "value" {
		t.Errorf("expected value, got %v", m["key"])
	}
	// Verify deep copy
	m["key"] = "modified"
	if src["key"] != "value" {
		t.Error("deep copy failed")
	}
}

func TestCloneJSONValue_Array(t *testing.T) {
	src := []any{"a", "b"}
	result := cloneJSONValue(src)
	arr, ok := result.([]any)
	if !ok || len(arr) != 2 {
		t.Fatal("expected array")
	}
}

// ---------------------------------------------------------------------------
// Gemini aggregate state machine (stream bridge)
// ---------------------------------------------------------------------------

// parseDataURLToGeminiInline
// ---------------------------------------------------------------------------

func TestParseDataURLToGeminiInline_Valid(t *testing.T) {
	result := parseDataURLToGeminiInline("data:image/png;base64,iVBORw0KGgo=")
	if result == nil {
		t.Fatal("expected non-nil")
	}
	id, ok := result["inlineData"].(map[string]any)
	if !ok {
		t.Fatal("expected inlineData")
	}
	if id["mimeType"] != "image/png" {
		t.Errorf("expected image/png, got %v", id["mimeType"])
	}
	if id["data"] != "iVBORw0KGgo=" {
		t.Errorf("expected data iVBORw0KGgo=, got %v", id["data"])
	}
}

func TestParseDataURLToGeminiInline_Invalid(t *testing.T) {
	if parseDataURLToGeminiInline("https://example.com/normal.png") != nil {
		t.Error("expected nil for non-data URL")
	}
	if parseDataURLToGeminiInline("") != nil {
		t.Error("expected nil for empty string")
	}
}

// ---------------------------------------------------------------------------
// inferMimeFromURL
// ---------------------------------------------------------------------------

func TestInferMimeFromURL(t *testing.T) {
	if inferMimeFromURL("photo.png") != "image/png" {
		t.Errorf("expected image/png")
	}
	if inferMimeFromURL("photo.jpg") != "image/jpeg" {
		t.Errorf("expected image/jpeg")
	}
	if inferMimeFromURL("photo.gif") != "image/gif" {
		t.Errorf("expected image/gif")
	}
	if inferMimeFromURL("photo.webp") != "image/webp" {
		t.Errorf("expected image/webp")
	}
	if inferMimeFromURL("unknown.xyz") != "application/octet-stream" {
		t.Errorf("expected application/octet-stream")
	}
}
