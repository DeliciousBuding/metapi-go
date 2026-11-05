package routecondition

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"
	"time"
)

// Expected counts were obtained from the audited source inbound transformers
// and its conditional estimator, independently of this implementation.
func TestBuildRequestSourceVectors(t *testing.T) {
	cases := []struct {
		name, format, body string
		tokens             int64
		features           [4]bool
	}{
		{"chat-string", "openai/chat_completions", `{"model":"test","messages":[{"role":"user","content":"Hello 世界"}]}`, 3, [4]bool{false, false, false, false}},
		{"chat-array", "openai/chat_completions", `{"model":"test","messages":[{"role":"user","content":[{"type":"text","text":"Hello 世界"},{"type":"image_url","image_url":{"url":"https://image.invalid","detail":"high"}}]}],"tools":[{"type":"function","function":{"name":"tool","description":"use it","parameters":{"type":"object","properties":{"n":{"type":"integer"}},"required":["n"]}}}]}`, 157, [4]bool{true, false, false, false}},
		{"responses-tools", "openai/responses", `{"model":"test","instructions":"你好world","input":[{"type":"reasoning","summary":[{"text":"I think"}]},{"type":"function_call","name":"a","arguments":{"n":9007199254740993}},{"type":"function_call","name":"b","arguments":"{}"},{"type":"function_call_output","name":"a","call_id":"call1","output":"result"}],"tools":[{"type":"function","name":"a","parameters":{"type":"object"}},{"type":"custom","name":"patch","format":{"type":"grammar","syntax":"lark","definition":"start: text"}}]}`, 39, [4]bool{false, false, false, false}},
		{"anthropic-mixed", "anthropic/messages", `{"model":"test","max_tokens":64,"system":[{"type":"text","text":"Hi"},{"type":"text","text":"世界"}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"甲乙丙"},{"type":"tool_use","id":"1","name":"echo","input":{"x":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"1","content":[{"type":"text","text":"result"}]},{"type":"text","text":"next"}]}]}`, 22, [4]bool{false, false, false, false}},
		{"gemini-tools", "gemini/contents", `{"systemInstruction":{"parts":[{"text":"hi"},{"text":"there"}]},"contents":[{"role":"user","parts":[{"text":"Hello 世界"}]},{"role":"model","parts":[{"functionCall":{"id":"call1","name":"echo","args":{"n":9007199254740993}}}]},{"role":"user","parts":[{"functionResponse":{"name":"echo","response":{"n":9007199254740993}}}]}],"tools":[{"functionDeclarations":[{"name":"echo","description":"Echo","parameters":{"type":"OBJECT","properties":{"n":{"type":"INTEGER"}},"required":["n"]}}]}]}`, 45, [4]bool{false, false, false, false}},
		{"gemini-empty-schema", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{}},{"name":"g","parameters":{"type":"OBJECT","properties":{"x":{}}}}]}]}`, 19, [4]bool{false, false, false, false}},
		{"gemini-media", "gemini/contents", `{"contents":[{"parts":[{"fileData":{"mimeType":"application/pdf","fileUri":"https://file.invalid/a.pdf"}},{"inlineData":{"mimeType":"audio/mpeg","data":"abc"}},{"fileData":{"mimeType":"video/mp4","fileUri":"https://file.invalid/v"}},{"inlineData":{"mimeType":"image/png","data":"abc"}}]}]}`, 403, [4]bool{true, true, true, true}},
		{"chat-reasoning-empty", "openai/chat_completions", `{"model":"test","messages":[{"role":"assistant","reasoning_content":"","reasoning":"fallbacklonger","content":""}]}`, 2, [4]bool{false, false, false, false}},
		{"anthropic-tool-result-split", "anthropic/messages", `{"model":"test","max_tokens":1,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]},{"type":"text","text":"x"}]}]}`, 8, [4]bool{false, false, false, false}},
		{"anthropic-thinking-empty", "anthropic/messages", `{"model":"test","max_tokens":1,"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":""}]},{"role":"user","content":[{"type":"image","source":{"type":"base64","data":""}},{"type":"text","text":"hello"}]}]}`, 2, [4]bool{false, false, false, false}},
		{"responses-empty-input", "openai/responses", `{"model":"test","input":[]}`, 0, [4]bool{false, false, false, false}},
		{"responses-output-array", "openai/responses", `{"model":"test","input":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"abc"},{"type":"output_text","text":"defgh"}]},{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`, 8, [4]bool{false, false, false, false}},
		{"responses-web", "openai/responses", `{"model":"test","input":"hello","tools":[{"type":"web_search","filters":{"allowed_domains":["example.com"]},"user_location":{"city":"city","country":"country"}},{"type":"namespace","name":"fs","tools":[{"type":"function","name":"read","parameters":{}}]}]}`, 15, [4]bool{false, false, false, false}},
		{"anthropic-web", "anthropic/messages", `{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"web_search_20250305","max_uses":1,"strict":false,"allowed_domains":["example.com"],"user_location":{"city":"city","type":"approximate"}}]}`, 12, [4]bool{false, false, false, false}},
		{"gemini-schema-0", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","properties":{"empty":{},"nil":null,"b":false,"a":{"anyOf":[{"type":"STRING"},{"type":"NULL"}]}}}}]}]}`, 34, [4]bool{false, false, false, false}},
		{"gemini-schema-1", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":["OBJECT","NULL"],"title":"","description":"","deprecated":false,"required":[],"properties":{},"allOf":[],"default":null}}]}]}`, 15, [4]bool{false, false, false, false}},
		{"gemini-schema-2", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","items":[{"type":"STRING"},{"type":"INTEGER"}],"dependencies":{"a":{"type":"OBJECT"},"b":["x"]},"enum":[]}}]}]}`, 33, [4]bool{false, false, false, false}},
		{"gemini-schema-3", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","properties":{"x":{"default":{"type":"UPPER","n":9007199254740993}}},"unknownField":{"a":1.0},"minimum":1.0}}]}]}`, 35, [4]bool{false, false, false, false}},
		{"gemini-schema-edge-0", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":null}]}]}`, 6, [4]bool{}},
		{"gemini-schema-edge-1", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":false}]}]}`, 6, [4]bool{}},
		{"gemini-schema-edge-2", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":true}]}]}`, 6, [4]bool{}},
		{"gemini-schema-edge-3", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":null}}]}]}`, 8, [4]bool{}},
		{"gemini-schema-edge-4", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","additionalProperties":null,"minimum":null}}]}]}`, 9, [4]bool{}},
		{"gemini-schema-edge-5", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"properties":null,"enum":null,"not":null}}]}]}`, 6, [4]bool{}},
		{"gemini-schema-edge-6", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"enum":[1.0,9007199254740993],"const":9007199254740993}}]}]}`, 18, [4]bool{}},
		{"gemini-schema-edge-7", "gemini/contents", `{"contents":[{"parts":[{"text":"hi"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","minLength":0.0}}]}]}`, 12, [4]bool{}},
		{"edge2-0", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","minLength":1.5,"title":"x"}}]}]}`, 16, [4]bool{}},
		{"edge2-1", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"$defs":{"a":{"type":"STRING"}},"definitions":{"b":{"type":"INTEGER"}}}}]}]}`, 23, [4]bool{}},
		{"edge2-2", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","const":{"n":1.0},"enum":[{"n":1.0}],"default":{"text":"<你好>"}}}]}]}`, 27, [4]bool{}},
		{"edge2-3", "gemini/contents", `{"contents":[{"parts":[{"text":"x"}]}],"tools":[{"functionDeclarations":[{"name":"f","parameters":{"type":"OBJECT","properties":{"n":{"minimum":null,"type":"INTEGER"}},"not":null,"examples":null}}]}]}`, 18, [4]bool{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(tc.body)
			before := bytes.Clone(body)
			clock := time.Date(2026, 11, 5, 9, 10, 0, 0, time.UTC)
			got, err := BuildRequest(body, tc.format, nil, true, clock)
			if err != nil {
				t.Fatal(err)
			}
			if got.PromptTokens != tc.tokens {
				t.Errorf("promptTokens=%d want=%d", got.PromptTokens, tc.tokens)
			}
			features := [4]bool{got.HasImage, got.HasVideo, got.HasDocument, got.HasAudio}
			if features != tc.features {
				t.Errorf("features=%v want=%v", features, tc.features)
			}
			if got.RequestFormat != tc.format || !got.Stream || got.Now != clock {
				t.Errorf("request identity changed: %+v", got)
			}
			if !bytes.Equal(before, body) {
				t.Error("description mutated raw request")
			}
		})
	}
}

func TestBuildRequestHeaderSnapshotAndSensitiveBoundaries(t *testing.T) {
	headers := http.Header{"x-model": {"first", "second"}, "User-Agent": {"test-client"}, "Authorization": {"private"}, "X-Api-Key": {"private"}, "X-Admin-Confirm-Token": {"private"}, "X-Metapi-Key": {"private"}}
	original := headers.Clone()
	got, err := BuildRequest([]byte(`{"messages":[{"role":"user","content":"hello"}]}`), "openai/chat_completions", headers, false, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"X-Model": "first", "User-Agent": "test-client"}
	if !reflect.DeepEqual(got.Headers, want) {
		t.Errorf("headers=%v", got.Headers)
	}
	if !reflect.DeepEqual(headers, original) {
		t.Error("headers mutated")
	}
	// Deliberately mutate the noncanonical input spelling to prove that the
	// normalized descriptor owns a copy of values from the original map.
	map[string][]string(headers)["x-model"][0] = "changed"
	if got.Headers["X-Model"] != "first" {
		t.Error("description aliases incoming headers")
	}
}

func TestBuildRequestRejectsUnknownOrInvalidContent(t *testing.T) {
	for _, tc := range []struct{ format, body string }{
		{"unknown", `{}`}, {"openai/chat_completions", `null`}, {"openai/chat_completions", `{"messages":42}`},
		{"openai/chat_completions", `{"messages":[{"role":"user","content":[{"type":"future_media"}]}]}`},
		{"openai/responses", `{"input":[{"type":"future_message"}]}`},
		{"openai/responses", `{"input":[{"type":"function_call_output"}]}`},
		{"anthropic/messages", `{"messages":[{"role":"user","content":[{"type":"future_media"}]}]}`},
		{"gemini/contents", `{"contents":"bad"}`},
		{"gemini/contents", `{"contents":[{"parts":[{"futureMedia":{"url":"secret"}}]}]}`},
		{"gemini/contents", `{}`},
		{"anthropic/messages", `{}`},
		{"openai/chat_completions", `{"messages":[]}`},
	} {
		if got, err := BuildRequest([]byte(tc.body), tc.format, nil, false, time.Time{}); err == nil || got != nil {
			t.Errorf("invalid descriptor accepted: %s %s", tc.format, tc.body)
		}
	}
}

func TestSourceTokenHeuristicWhitespaceAndUnicode(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int64
	}{{"", 0}, {"   ", 1}, {"abcd", 1}, {"abcdefgh", 2}, {"中文", 1}, {"中文甲", 2}, {"😀😀😀😀", 1}, {"ab中文", 1}} {
		if got := sourceTokens(tc.text); got != tc.want {
			t.Errorf("sourceTokens(%q)=%d want=%d", tc.text, got, tc.want)
		}
	}
}

func TestBuildRequestNonGenerationSourceFields(t *testing.T) {
	for _, format := range []string{"openai/embeddings", "openai/moderations", "jina/rerank", "jina/embeddings", "gemini/embeddings", "openai/completions"} {
		got, err := BuildRequest([]byte(`{"model":"test","prompt":"this stays outside source Messages","input":"same","query":"same"}`), format, nil, true, time.Time{})
		if err != nil || got.PromptTokens != 0 || got.HasImage || got.HasVideo || got.HasDocument || got.HasAudio {
			t.Fatalf("%s: %+v %v", format, got, err)
		}
		if got.Stream != (format == "openai/completions") {
			t.Errorf("wrong non-generation stream: %s", format)
		}
	}
}
