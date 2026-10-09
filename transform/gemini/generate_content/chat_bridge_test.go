package generate_content

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGeminiChatToolRequestRoundTrip(t *testing.T) {
	raw := []byte(`{"model":"gemini-3-flash","messages":[{"role":"user","content":"weather"},{"role":"assistant","tool_calls":[{"id":"call-a","type":"function","function":{"name":"weather","arguments":"{\"city\":\"Hong Kong\"}"},"provider_specific_fields":{"thought_signature":"signature-real"}}]},{"role":"tool","tool_call_id":"call-a","content":"{\"temperature\":25}"}],"tools":[{"type":"function","function":{"name":"weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}],"max_completion_tokens":22,"stop":["end"]}`)
	gemini, err := FromChatRequest(raw, "gemini-3-flash")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gemini), `"thoughtSignature":"signature-real"`) {
		t.Fatalf("signature lost: %s", gemini)
	}
	chat, err := ToChatRequest(gemini, "gemini-3-flash")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(chat, &out)
	if out["max_tokens"] != float64(22) {
		t.Fatalf("token limit lost: %s", chat)
	}
	if !strings.Contains(string(chat), `"tool_call_id":"call-a"`) || !strings.Contains(string(chat), `signature-real`) {
		t.Fatalf("tool roundtrip lost: %s", chat)
	}
}

func TestGeminiChatResponseUsageAndTools(t *testing.T) {
	raw := []byte(`{"responseId":"gemini-a","modelVersion":"gemini-test","candidates":[{"content":{"role":"model","parts":[{"text":"thinking","thought":true},{"functionCall":{"id":"call-a","name":"weather","args":{"city":"HK"}},"thoughtSignature":"signed"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"thoughtsTokenCount":2,"totalTokenCount":15,"cachedContentTokenCount":4}}`)
	chat, err := ToChatResponse(raw, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	_ = json.Unmarshal(chat, &out)
	usage := out["usage"].(map[string]any)
	if usage["completion_tokens"] != float64(5) || usage["total_tokens"] != float64(15) {
		t.Fatalf("wrong usage: %s", chat)
	}
	back, err := FromChatResponse(chat)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(back), `"thoughtSignature":"signed"`) || !strings.Contains(string(back), `"thoughtsTokenCount":2`) {
		t.Fatalf("roundtrip lost: %s", back)
	}
}

func TestGeminiChatStreamingToolsAndTerminals(t *testing.T) {
	gemini := NewGeminiStream("gemini-test")
	chat := NewChatStream("gemini-test")
	var returned strings.Builder
	for _, raw := range []string{
		`{"responseId":"id-a","candidates":[{"content":{"parts":[{"text":"hello"}]}}]}`,
		`{"responseId":"id-a","candidates":[{"content":{"parts":[{"functionCall":{"name":"weather","args":{"city":"HK"}},"thoughtSignature":"sig"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":2,"totalTokenCount":5}}`,
	} {
		chunks, err := gemini.TransformEvent([]byte("data: " + raw + "\n\n"))
		if err != nil {
			t.Fatal(err)
		}
		converted, err := chat.TransformEvent(chunks)
		if err != nil {
			t.Fatal(err)
		}
		returned.Write(converted)
	}
	terminal, err := gemini.Finish()
	if err != nil {
		t.Fatal(err)
	}
	converted, err := chat.TransformEvent(terminal)
	if err != nil {
		t.Fatal(err)
	}
	returned.Write(converted)
	if _, err := chat.Finish(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"text":"hello"`, `"functionCall"`, `"thoughtSignature":"sig"`, `"finishReason":"STOP"`, `"totalTokenCount":5`} {
		if !strings.Contains(returned.String(), want) {
			t.Fatalf("missing %s: %s", want, returned.String())
		}
	}
}

func TestGeminiBridgeRejectsUnrepresentableAndTruncated(t *testing.T) {
	for _, raw := range []string{`{"contents":[{"parts":[{"text":"x"}]}],"cachedContent":"cached/1"}`, `{"contents":[{"parts":[{"text":"x","thoughtSignature":"signed"}]}]}`, `{"contents":[{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"gs://private/1"}}]}]}`} {
		if _, err := ToChatRequest([]byte(raw), "model"); err == nil {
			t.Fatalf("accepted lossy input: %s", raw)
		}
	}
	if _, err := FromChatRequest([]byte(`{"messages":[{"role":"assistant","tool_calls":[{"function":{"name":"f","arguments":"broken"}}]}]}`), "model"); err == nil {
		t.Fatal("accepted broken arguments")
	}
	gs := NewGeminiStream("model")
	_, _ = gs.TransformEvent([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"partial\"}]}}]}\n\n"))
	if _, err := gs.Finish(); err == nil {
		t.Fatal("truncated Gemini stream succeeded")
	}
	cs := NewChatStream("model")
	if _, err := cs.TransformEvent([]byte("data: [DONE]\n\n")); err == nil {
		t.Fatal("Chat missing finish succeeded")
	}
	if _, err := cs.Finish(); err == nil {
		t.Fatal("sticky failure was repaired")
	}
}

func TestGeminiBridgePreservesExactToolSemantics(t *testing.T) {
	raw := []byte(`{"messages":[{"role":"system","content":"KEEP_A"},{"role":"developer","content":"KEEP_B"},{"role":"user","content":"go"},{"role":"assistant","tool_calls":[{"id":"call-a","type":"function","function":{"name":"f","arguments":"{\"id\":9007199254740993}"}}]},{"role":"tool","tool_call_id":"call-a","content":"{\"answer\":42}"}],"tool_choice":{"type":"function","function":{"name":"f"}},"tools":[{"type":"function","function":{"name":"f","parameters":{"type":"object"}}}]}`)
	output, err := FromChatRequest(raw, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`KEEP_A`, `KEEP_B`, `"allowedFunctionNames":["f"]`, `"response":{"answer":42}`, `9007199254740993`} {
		if !strings.Contains(string(output), want) {
			t.Fatalf("missing %s: %s", want, output)
		}
	}
	chat, err := ToChatRequest(output, "gemini-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(chat), `9007199254740993`) || strings.Contains(string(chat), `9007199254740992`) {
		t.Fatalf("integer corrupted: %s", chat)
	}
	native := []byte(`{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","properties":{"type":{"type":"STRING"}},"required":["type"]}}]}]}`)
	chat, err = ToChatRequest(native, "model")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(chat), `OBJECT`) || strings.Contains(string(chat), `STRING`) {
		t.Fatalf("native Schema not converted: %s", chat)
	}
	strict := strings.Replace(string(raw), `"parameters":{"type":"object"}`, `"strict":true,"parameters":{"type":"object"}`, 1)
	if _, err := FromChatRequest([]byte(strict), "model"); err == nil {
		t.Fatal("strict semantics dropped")
	}
	invalidResponse := []byte(`{"choices":[{"message":{"role":"assistant","content":""},"finish_reason":"tool_calls"}]}`)
	if _, err := FromChatResponse(invalidResponse); err == nil {
		t.Fatal("empty tool terminal succeeded")
	}
	s := NewChatStream("model")
	_, err = s.TransformEvent([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.TransformEvent([]byte("data: [DONE]\n\n")); err == nil {
		t.Fatal("empty stream tool terminal succeeded")
	}
}
