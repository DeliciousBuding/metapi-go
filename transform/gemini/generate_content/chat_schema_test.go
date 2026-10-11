package generate_content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestGeminiStructuredRequestSchemas(t *testing.T) {
	for _, tc := range []struct {
		name, field, schema string
		want                []string
	}{
		{"legacy", "responseSchema", `{"type":"OBJECT","properties":{"type":{"type":"INTEGER","minimum":9007199254740993},"label":{"type":"STRING","nullable":true,"enum":["a","b"]},"tags":{"type":"ARRAY","items":{"type":"STRING","minLength":"2"},"minItems":"1","maxItems":"9223372036854775807"}},"required":["type"],"default":{"type":"KEEP_UPPERCASE","nullable":true,"value":9007199254740993}}`, []string{`"type":"integer"`, `"minimum":9007199254740993`, `"minLength":2`, `"maxItems":9223372036854775807`, `"type":"null"`, `"enum":["a","b"]`, `"default":{"nullable":true,"type":"KEEP_UPPERCASE","value":9007199254740993}`}},
		{"modern", "responseJsonSchema", `{"type":"object","properties":{"z":{"type":"integer","minimum":9007199254740993},"a":{"$ref":"#/$defs/label"}},"$defs":{"label":{"type":"string"}},"additionalProperties":false,"required":["z","a"]}`, []string{`"properties":{"z":`, `"minimum":9007199254740993`, `"$ref":"#/$defs/label"`, `"additionalProperties":false`}},
		{"ordered", "responseSchema", `{"type":"OBJECT","properties":{"alpha":{"type":"STRING"},"zeta":{"type":"INTEGER"}},"propertyOrdering":["zeta","alpha"]}`, []string{`"properties":{"zeta":{"type":"integer"},"alpha":{"type":"string"}}`}},
		{"modern ordered", "responseJsonSchema", `{"type":"object","properties":{"alpha":{"type":"string"},"zeta":{"type":"integer"}},"propertyOrdering":["zeta","alpha"]}`, []string{`"properties":{"zeta":{"type":"integer"},"alpha":{"type":"string"}}`}},
		{"native oneOf is anyOf", "responseJsonSchema", `{"oneOf":[{"type":"number"},{"type":"integer"}]}`, []string{`"anyOf":[{"type":"number"},{"type":"integer"}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"contents":[{"parts":[{"text":"go"}]}],"generationConfig":{"responseMimeType":"application/json","` + tc.field + `":` + tc.schema + `}}`)
			out, err := ToChatRequest(body, "model")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range append(tc.want, `"type":"json_schema"`, `"strict":true`) {
				if !bytes.Contains(out, []byte(want)) {
					t.Fatalf("missing %s: %s", want, out)
				}
			}
			if bytes.Contains(out, []byte(`"propertyOrdering"`)) || bytes.Contains(out, []byte(`"oneOf"`)) {
				t.Fatalf("Gemini-only schema behavior leaked: %s", out)
			}
		})
	}
}

func TestChatStructuredRequestSchemas(t *testing.T) {
	schema := `{"type":"object","properties":{"z":{"type":"integer","minimum":9007199254740993},"a":{"$ref":"#/$defs/label"}},"$defs":{"label":{"type":["string","null"]}},"additionalProperties":false,"required":["z","a"]}`
	for _, strict := range []string{"true", "false", "null"} {
		t.Run(strict, func(t *testing.T) {
			body := []byte(`{"messages":[{"role":"user","content":"go"}],"temperature":0.3,"max_tokens":32,"response_format":{"type":"json_schema","json_schema":{"name":"Result-v1","strict":` + strict + `,"schema":` + schema + `}}}`)
			out, err := FromChatRequest(body, "gemini-test")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{`"responseJsonSchema":` + schema, `"responseMimeType":"application/json"`, `"temperature":0.3`, `"maxOutputTokens":32`} {
				if !bytes.Contains(out, []byte(want)) {
					t.Fatalf("missing %s: %s", want, out)
				}
			}
			back, err := ToChatRequest(out, "gemini-test")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rawSchema(back, "response_format", "json_schema", "schema"), []byte(schema)) {
				t.Fatalf("schema changed on roundtrip: %s", back)
			}
		})
	}
	for _, typ := range []string{"text", "json_object"} {
		body := []byte(`{"messages":[{"role":"user","content":"go"}],"response_format":{"type":"` + typ + `"}}`)
		native, err := FromChatRequest(body, "m")
		if err != nil {
			t.Fatal(err)
		}
		back, err := ToChatRequest(native, "m")
		if err != nil || !bytes.Contains(back, []byte(`"response_format":{"type":"`+typ+`"}`)) {
			t.Fatalf("format %s roundtrip: %s %v", typ, back, err)
		}
	}
}

func TestChatSchemaUsesLegacyConstraintDialect(t *testing.T) {
	input := []byte(`{"messages":[{"role":"user","content":"go"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","strict":true,"description":"Outer hint","schema":{"type":"object","description":"Inner hint","minProperties":1,"properties":{"name":{"type":"string","minLength":2,"maxLength":15,"pattern":"^[a-z]+$"}},"required":["name"]}}}}`)
	out, err := FromChatRequest(input, "m")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"responseSchema":`, `"minLength":"2"`, `"maxLength":"15"`, `"pattern":"^[a-z]+$"`, `"description":"Outer hint\n\nInner hint"`} {
		if !bytes.Contains(out, []byte(want)) {
			t.Fatalf("missing %s: %s", want, out)
		}
	}
	back, err := ToChatRequest(out, "m")
	if err != nil || !bytes.Contains(back, []byte(`"minLength":2`)) {
		t.Fatalf("legacy roundtrip: %s %v", back, err)
	}
}

func TestStructuredSchemaRejectsLossAndConflicts(t *testing.T) {
	for _, gc := range []string{
		`"invalid"`, `{"responseMimeType":1}`, `{"responseMimeType":"text/x.enum"}`, `{"responseSchema":{"type":"OBJECT"}}`,
		`{"responseMimeType":"text/plain","responseSchema":{"type":"OBJECT"}}`,
		`{"responseMimeType":"application/json","responseSchema":{},"responseJsonSchema":{}}`,
		`{"responseMimeType":"application/json","responseSchema":[]}`,
		`{"responseMimeType":"application/json","responseSchema":{"type":"OBJECT","nullable":"true"}}`,
		`{"responseMimeType":"application/json","responseSchema":{"type":"ARRAY","minItems":"-1"}}`,
		`{"responseMimeType":"application/json","responseSchema":{"type":"ARRAY","maxItems":"9223372036854775808"}}`,
		`{"responseMimeType":"application/json","responseSchema":{"type":"OBJECT","properties":{"x":{"type":"STRING"}},"propertyOrdering":["missing"]}}`,
		`{"responseMimeType":"application/json","responseJsonSchema":{"type":"object","not":{"type":"object"}}}`,
	} {
		_, err := ToChatRequest([]byte(`{"contents":[{"parts":[{"text":"go"}]}],"generationConfig":`+gc+`}`), "m")
		if err == nil {
			t.Fatalf("accepted invalid/lossy config: %s", gc)
		}
	}
	for _, format := range []string{
		`[]`, `{"type":"grammar"}`, `{"type":"json_object","schema":{}}`, `{"type":"json_schema"}`,
		`{"type":"json_schema","json_schema":{"name":"bad name","schema":{}}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{},"strict":1}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{},"description":1}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{"oneOf":[{"type":"integer"},{"type":"number"}]}}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{"type":"number","multipleOf":3}}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{"$ref":"#/$defs/a","minimum":3}}}`,
		`{"type":"json_schema","json_schema":{"name":"r","schema":{"type":"object","additionalProperties":false,"properties":{"s":{"type":"string","pattern":"^a"}}}}}`,
	} {
		_, err := FromChatRequest([]byte(`{"messages":[{"role":"user","content":"go"}],"response_format":`+format+`}`), "m")
		if err == nil {
			t.Fatalf("accepted invalid/lossy format: %s", format)
		}
	}
}

func TestGeminiNormalizeRetainsJSONSchema(t *testing.T) {
	in, err := bridgeObject([]byte(`{"contents":[{"parts":[{"text":"go"}]}],"generationConfig":{"responseMimeType":"application/json","responseJsonSchema":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993}},"additionalProperties":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	out := NormalizeRequest(in, "gemini-test")
	if !reflect.DeepEqual(in["generationConfig"], out["generationConfig"]) {
		t.Fatalf("native request schema changed: %#v", out)
	}
}

// Actual HTTP transport around both codecs verifies the body seen by a fixture
// provider, plus JSON and SSE responses. This does not call a real model.
func TestStructuredSchemaHTTPCodecMatrix(t *testing.T) {
	schema := `{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993}},"required":["id"],"additionalProperties":false}`
	for _, direction := range []string{"gemini-to-chat", "chat-to-gemini"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", direction, stream), func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						w.WriteHeader(500)
						return
					}
					var actual json.RawMessage
					var response string
					if direction == "gemini-to-chat" {
						actual = rawSchema(body, "response_format", "json_schema", "schema")
						field := "message"
						if stream {
							field = "delta"
						}
						response = `{"choices":[{"` + field + `":{"content":"{\"id\":9007199254740993}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`
					} else {
						actual = rawSchema(body, "generationConfig", "responseJsonSchema")
						response = `{"candidates":[{"content":{"role":"model","parts":[{"text":"{\"id\":9007199254740993}"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5}}`
					}
					if !bytes.Equal(actual, []byte(schema)) {
						t.Errorf("wire schema mismatch: %s", body)
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = fmt.Fprint(w, "data: "+response+"\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = fmt.Fprint(w, response)
					}
				}))
				defer upstream.Close()
				var body []byte
				var err error
				if direction == "gemini-to-chat" {
					body, err = ToChatRequest([]byte(`{"contents":[{"parts":[{"text":"go"}]}],"generationConfig":{"responseMimeType":"application/json","responseJsonSchema":`+schema+`}}`), "m")
				} else {
					body, err = FromChatRequest([]byte(`{"messages":[{"role":"user","content":"go"}],"response_format":{"type":"json_schema","json_schema":{"name":"r","strict":true,"schema":`+schema+`}}}`), "m")
				}
				if err != nil {
					t.Fatal(err)
				}
				response, err := http.Post(upstream.URL, "application/json", bytes.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				raw, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				var out []byte
				if direction == "gemini-to-chat" {
					if stream {
						codec := NewChatStream("m")
						out, err = codec.TransformEvent(raw)
						if err == nil {
							var terminal []byte
							terminal, err = codec.TransformEvent([]byte("data: [DONE]\n\n"))
							out = append(out, terminal...)
						}
					} else {
						out, err = FromChatResponse(raw)
					}
				} else if stream {
					codec := NewGeminiStream("m")
					out, err = codec.TransformEvent(raw)
					if err == nil {
						_, err = codec.Finish()
					}
				} else {
					out, err = ToChatResponse(raw, "m")
				}
				if err != nil || !strings.Contains(string(out), `9007199254740993`) {
					t.Fatalf("response: %s %v", out, err)
				}
			})
		}
	}
}
