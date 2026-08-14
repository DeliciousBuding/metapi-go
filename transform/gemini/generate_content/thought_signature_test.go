package generate_content

import (
	"testing"
)

func collectFunctionCallParts(contents any) []map[string]any {
	var out []map[string]any
	switch arr := contents.(type) {
	case []map[string]any:
		for _, content := range arr {
			switch parts := content["parts"].(type) {
			case []map[string]any:
				for _, p := range parts {
					if _, ok := p["functionCall"]; ok {
						out = append(out, p)
					}
				}
			case []any:
				for _, raw := range parts {
					if p, ok := raw.(map[string]any); ok {
						if _, ok := p["functionCall"]; ok {
							out = append(out, p)
						}
					}
				}
			}
		}
	case []any:
		for _, item := range arr {
			content, ok := item.(map[string]any)
			if !ok {
				continue
			}
			switch parts := content["parts"].(type) {
			case []map[string]any:
				for _, p := range parts {
					if _, ok := p["functionCall"]; ok {
						out = append(out, p)
					}
				}
			case []any:
				for _, raw := range parts {
					if p, ok := raw.(map[string]any); ok {
						if _, ok := p["functionCall"]; ok {
							out = append(out, p)
						}
					}
				}
			}
		}
	}
	return out
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_PreservesProviderThoughtSignature(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-3-flash-preview",
		"messages": []any{
			map[string]any{"role": "user", "content": "What is the weather?"},
			map[string]any{
				"role":    "assistant",
				"content": "Let me check.",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_123",
						"type": "function",
						"function": map[string]any{
							"name":      "get_weather",
							"arguments": `{"city":"Tokyo"}`,
						},
						"provider_specific_fields": map[string]any{
							"thought_signature": "real_sig_abc",
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_123", "content": `{"temp":"22C"}`},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-3-flash-preview")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if fcParts[0]["thoughtSignature"] != "real_sig_abc" {
		t.Fatalf("expected real thoughtSignature, got %v", fcParts[0]["thoughtSignature"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_SplitsSignedFunctionCallFromText(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-3-flash-preview",
		"messages": []any{
			map[string]any{"role": "user", "content": "Read the file."},
			map[string]any{
				"role":    "assistant",
				"content": "I will read it.",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_456",
						"type": "function",
						"function": map[string]any{
							"name":      "Read",
							"arguments": `{"path":"/tmp/x"}`,
						},
						"provider_specific_fields": map[string]any{
							"thought_signature": "sig_split_test",
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_456", "content": "file content here"},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-3-flash-preview")
	contents, ok := result["contents"].([]map[string]any)
	if !ok {
		t.Fatalf("contents type = %T", result["contents"])
	}
	var modelMsgs []map[string]any
	for _, c := range contents {
		if c["role"] == "model" {
			modelMsgs = append(modelMsgs, c)
		}
	}
	if len(modelMsgs) != 2 {
		t.Fatalf("expected 2 model messages (text + signed functionCall), got %d", len(modelMsgs))
	}
	firstParts, _ := modelMsgs[0]["parts"].([]map[string]any)
	secondParts, _ := modelMsgs[1]["parts"].([]map[string]any)
	if len(firstParts) == 0 || firstParts[0]["text"] == nil {
		t.Fatalf("first model message should be text-only, got %#v", firstParts)
	}
	if _, ok := secondParts[0]["functionCall"]; !ok {
		t.Fatalf("second model message should be functionCall, got %#v", secondParts)
	}
	if secondParts[0]["thoughtSignature"] != "sig_split_test" {
		t.Fatalf("expected split signature, got %v", secondParts[0]["thoughtSignature"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_InjectsDummyWhenThinkingEnabled(t *testing.T) {
	oaiBody := map[string]any{
		"model":            "gemini-3-flash-preview",
		"reasoning_effort": "high",
		"messages": []any{
			map[string]any{"role": "user", "content": "Do something."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_no_sig",
						"type": "function",
						"function": map[string]any{
							"name":      "Bash",
							"arguments": `{"command":"ls"}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_no_sig", "content": "file1\nfile2"},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-3-flash-preview")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	sig, _ := fcParts[0]["thoughtSignature"].(string)
	if sig == "" {
		t.Fatal("expected dummy thoughtSignature when thinking enabled")
	}
	if sig != DummyThoughtSignature {
		t.Fatalf("expected dummy sentinel, got %q", sig)
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_InjectsDummyForGemini3WithoutThinking(t *testing.T) {
	// Official Gemini 3.x rejects tool history without thought_signature even without explicit thinking.
	oaiBody := map[string]any{
		"model": "gemini-3.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "List files."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_ls",
						"type": "function",
						"function": map[string]any{
							"name":      "ls",
							"arguments": `{"path":"/tmp"}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_ls", "content": "file1\nfile2"},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-3.5-flash")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if fcParts[0]["thoughtSignature"] != DummyThoughtSignature {
		t.Fatalf("expected dummy thoughtSignature for Gemini 3 tool history, got %v", fcParts[0]["thoughtSignature"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_NoDummyWhenThinkingDisabledOnGemini25(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-2.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello"},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_no_think",
						"type": "function",
						"function": map[string]any{
							"name":      "Read",
							"arguments": `{"path":"/x"}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_no_think", "content": "data"},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-2.5-flash")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if _, ok := fcParts[0]["thoughtSignature"]; ok {
		t.Fatalf("did not expect thoughtSignature when thinking disabled on gemini-2.5, got %v", fcParts[0]["thoughtSignature"])
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_NonGeminiNoDummyDisablesThinking(t *testing.T) {
	oaiBody := map[string]any{
		"model":            "claude-sonnet-4-5",
		"reasoning_effort": "high",
		"messages": []any{
			map[string]any{"role": "user", "content": "Do something."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_no_sig_non_gemini",
						"type": "function",
						"function": map[string]any{
							"name":      "Bash",
							"arguments": `{"command":"ls"}`,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_no_sig_non_gemini", "content": "file1\nfile2"},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "claude-sonnet-4-5")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if _, ok := fcParts[0]["thoughtSignature"]; ok {
		t.Fatalf("did not expect dummy signature for non-gemini model, got %v", fcParts[0]["thoughtSignature"])
	}
	if gc, ok := result["generationConfig"].(map[string]any); ok {
		if _, hasTC := gc["thinkingConfig"]; hasTC {
			t.Fatalf("expected thinkingConfig disabled for non-gemini missing signature, got %#v", gc["thinkingConfig"])
		}
	}
}

func TestBuildGeminiGenerateContentRequestFromOpenAi_MultiTurnToolHistoryPreservesSignatures(t *testing.T) {
	oaiBody := map[string]any{
		"model": "gemini-3.5-flash",
		"messages": []any{
			map[string]any{"role": "user", "content": "Read two files."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   "call_a",
						"type": "function",
						"function": map[string]any{
							"name":      "Read",
							"arguments": `{"path":"/a"}`,
						},
						"provider_specific_fields": map[string]any{"thought_signature": "sig_a"},
					},
					map[string]any{
						"id":   "call_b",
						"type": "function",
						"function": map[string]any{
							"name":      "Read",
							"arguments": `{"path":"/b"}`,
						},
						"provider_specific_fields": map[string]any{"thought_signature": "sig_b"},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": "call_a", "content": "content a"},
			map[string]any{"role": "tool", "tool_call_id": "call_b", "content": "content b"},
			map[string]any{"role": "user", "content": "Summarize them."},
		},
	}

	result := BuildGeminiGenerateContentRequestFromOpenAi(oaiBody, "gemini-3.5-flash")
	fcParts := collectFunctionCallParts(result["contents"])
	if len(fcParts) != 2 {
		t.Fatalf("expected 2 functionCall parts, got %d", len(fcParts))
	}
	sigs := map[string]bool{}
	for _, p := range fcParts {
		sig, _ := p["thoughtSignature"].(string)
		sigs[sig] = true
	}
	if !sigs["sig_a"] || !sigs["sig_b"] {
		t.Fatalf("expected multi-turn signatures preserved, got %#v", sigs)
	}
}

func TestNormalizeRequest_InjectsDummyThoughtSignatureForGemini3ToolHistory(t *testing.T) {
	body := map[string]any{
		"model": "gemini-3.5-flash",
		"contents": []any{
			map[string]any{
				"role":  "user",
				"parts": []any{map[string]any{"text": "List files."}},
			},
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "ls",
							"args": map[string]any{"path": "/tmp"},
						},
					},
				},
			},
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"functionResponse": map[string]any{
							"name":     "ls",
							"response": map[string]any{"result": "a\nb"},
						},
					},
				},
			},
		},
	}

	normalized := NormalizeRequest(body, "gemini-3.5-flash")
	fcParts := collectFunctionCallParts(normalized["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if fcParts[0]["thoughtSignature"] != DummyThoughtSignature {
		t.Fatalf("expected dummy signature injected into native contents, got %v", fcParts[0]["thoughtSignature"])
	}
}

func TestNormalizeRequest_PreservesExistingThoughtSignature(t *testing.T) {
	body := map[string]any{
		"model": "gemini-3.5-flash",
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"functionCall": map[string]any{
							"name": "ls",
							"args": map[string]any{},
						},
						"thoughtSignature": "keep_me",
					},
				},
			},
		},
	}

	normalized := NormalizeRequest(body, "gemini-3.5-flash")
	fcParts := collectFunctionCallParts(normalized["contents"])
	if len(fcParts) != 1 {
		t.Fatalf("expected 1 functionCall part, got %d", len(fcParts))
	}
	if fcParts[0]["thoughtSignature"] != "keep_me" {
		t.Fatalf("expected existing signature preserved, got %v", fcParts[0]["thoughtSignature"])
	}
}
