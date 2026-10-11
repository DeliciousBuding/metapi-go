package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/transform/anthropic/messages"
)

// These independently authored client requests exercise multimodal input and a
// completed tool round, including native Messages' existing reasoning replay.
var domesticProfileClientFixtures = []struct{ name, path, body, terminal string }{
	{"chat", "/v1/chat/completions", `{"model":"provider-model","messages":[{"role":"system","content":"system-receipt"},{"role":"user","content":[{"type":"text","text":"input-receipt"},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGk="}}]},{"role":"assistant","content":null,"reasoning":"history-thought","tool_calls":[{"id":"history-id","type":"function","function":{"name":"echo","arguments":"{\"n\":9007199254740993}"}}]},{"role":"tool","tool_call_id":"history-id","content":"tool-history"},{"role":"user","content":"continue-receipt"}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"type":"object","properties":{"n":{"type":"integer"}}}}}]}`, "[DONE]"},
	{"responses", "/v1/responses", `{"model":"provider-model","instructions":"system-receipt","input":[{"role":"user","content":[{"type":"input_text","text":"input-receipt"}]},{"type":"reasoning","summary":[{"type":"summary_text","text":"history-thought"}]},{"type":"function_call","call_id":"history-id","name":"echo","arguments":"{\"n\":9007199254740993}"},{"type":"function_call_output","call_id":"history-id","output":"tool-history"},{"role":"user","content":"continue-receipt"}],"tools":[{"type":"function","name":"echo","parameters":{"type":"object","properties":{"n":{"type":"integer"}}}}]}`, "event: response.completed"},
	{"messages", "/v1/messages", `{"model":"provider-model","system":"system-receipt","max_tokens":256,"thinking":{"type":"adaptive"},"messages":[{"role":"user","content":[{"type":"text","text":"input-receipt"}]},{"role":"assistant","content":[{"type":"tool_use","id":"history-id","name":"echo","input":{"n":9007199254740993}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"history-id","content":"tool-history"}]},{"role":"user","content":"continue-receipt"}],"tools":[{"name":"echo","input_schema":{"type":"object","properties":{"n":{"type":"integer"}}}}]}`, "event: message_stop"},
	{"gemini", "/v1beta/models/provider-model:generateContent", `{"systemInstruction":{"parts":[{"text":"system-receipt"}]},"contents":[{"role":"user","parts":[{"text":"input-receipt"},{"inlineData":{"mimeType":"image/png","data":"aGk="}}]},{"role":"model","parts":[{"thought":true,"text":"history-thought"},{"functionCall":{"id":"history-id","name":"echo","args":{"n":9007199254740993}}}]},{"role":"user","parts":[{"functionResponse":{"id":"history-id","name":"echo","response":{"text":"tool-history"}}}]},{"role":"user","parts":[{"text":"continue-receipt"}]}],"tools":[{"functionDeclarations":[{"name":"echo","parameters":{"type":"object","properties":{"n":{"type":"integer"}}}}]}]}`, `"finishReason":"STOP"`},
}

const domesticProfileResponseFixture = `{"id":"reply-id","object":"chat.completion","model":"provider-model","choices":[{"index":0,"message":{"role":"assistant","content":"response-text","reasoning_content":"response-thought","tool_calls":[{"id":"reply-call","type":"function","function":{"name":"echo","arguments":"{\"n\":9007199254740993}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`

func domesticProfileStreamFixture() []byte {
	var output []byte
	for _, data := range []string{
		`{"id":"reply-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"response-thought"}}]}`,
		`{"id":"reply-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"content":"response-text"}}]}`,
		`{"id":"reply-id","object":"chat.completion.chunk","model":"provider-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"reply-call","type":"function","function":{"name":"echo","arguments":"{\"n\":9007199254740993}"}}]},"finish_reason":"tool_calls"}]}`,
		`{"id":"reply-id","object":"chat.completion.chunk","model":"provider-model","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`,
		`[DONE]`,
	} {
		output = append(output, []byte("data: "+data+"\n\n")...)
	}
	return output
}

// This verifies the leaf helpers with actual HTTP transport and the existing
// converters, independently of the profile dispatch wiring added by the caller.
func TestDirectDomesticProfilesHTTPConverterMatrix(t *testing.T) {
	for _, profile := range []string{"moonshot", "longcat"} {
		for _, client := range domesticProfileClientFixtures {
			for _, stream := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream_%t", profile, client.name, stream), func(t *testing.T) {
					var savedReasoning string
					options := messages.Options{
						LoadReasoning: func(ids []string) (string, bool, error) {
							if len(ids) != 1 || ids[0] != "history-id" {
								return "", false, fmt.Errorf("unexpected fixture history identity")
							}
							return "history-thought", true, nil
						},
						SaveReasoning: func(ids []string, reasoning string) error {
							if len(ids) != 1 || ids[0] != "reply-call" {
								return fmt.Errorf("unexpected fixture reply identity")
							}
							savedReasoning = reasoning
							return nil
						},
					}
					request := []byte(client.body)
					if client.name != "gemini" {
						doc := domesticProfileTestObject(t, request)
						doc["stream"] = json.RawMessage(fmt.Sprint(stream))
						request, _ = json.Marshal(doc)
					}
					converted, err := directConvertRequest(request, client.path, "/v1/chat/completions", "provider-model", stream, options)
					if err != nil {
						t.Fatal(err)
					}
					wire, err := prepareDirectDomesticProfileRequest(converted, profile)
					if err != nil {
						t.Fatal(err)
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, _ := io.ReadAll(r.Body)
						if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer fixture-only" {
							t.Error("incorrect HTTP method, path or authentication")
						}
						markers := []string{"system-receipt", "input-receipt", "history-thought", "history-id", "echo", "9007199254740993", "tool-history", "continue-receipt"}
						if client.name == "chat" || client.name == "gemini" {
							markers = append(markers, "data:image/png;base64,aGk=")
						}
						for _, marker := range markers {
							if !bytes.Contains(body, []byte(marker)) {
								t.Errorf("upstream lost %s: %s", marker, body)
							}
						}
						if bytes.Contains(body, []byte(`"reasoning":`)) || !bytes.Contains(body, []byte(`"reasoning_content":"history-thought"`)) {
							t.Errorf("upstream received the wrong reasoning field: %s", body)
						}
						if profile == "longcat" {
							var doc struct {
								Messages []struct{ Content json.RawMessage }
							}
							if err := json.Unmarshal(body, &doc); err != nil {
								t.Error(err)
							}
							for _, message := range doc.Messages {
								if len(message.Content) == 0 || message.Content[0] != '[' {
									t.Errorf("LongCat received non-array content: %s", body)
								}
							}
						}
						if stream {
							w.Header().Set("Content-Type", "text/event-stream")
							for _, frame := range strings.Split(string(domesticProfileStreamFixture()), "\n\n") {
								if frame != "" {
									_, _ = io.WriteString(w, frame+"\n\n")
									w.(http.Flusher).Flush()
								}
							}
						} else {
							w.Header().Set("Content-Type", "application/json")
							_, _ = io.WriteString(w, domesticProfileResponseFixture)
						}
					}))
					defer server.Close()
					req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(wire))
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer fixture-only")
					resp, err := server.Client().Do(req)
					if err != nil {
						t.Fatal(err)
					}
					defer resp.Body.Close()
					var output []byte
					var usage ParsedUsage
					if stream {
						converter := directResponseStream(client.path, "/v1/chat/completions", "provider-model", options)
						if converter == nil {
							output, err = io.ReadAll(resp.Body)
							if !bytes.Equal(output, domesticProfileStreamFixture()) {
								t.Fatal("native Chat response was rewritten")
							}
							analyzer := newIncrementalSseAnalyzer()
							analyzer.Push(output)
							usage = analyzer.Result().Usage
						} else {
							reader := newProtocolBridgeBody(resp.Body, converter, 1<<20)
							output, err = io.ReadAll(reader)
							usage = reader.original.Result().Usage
						}
						if !bytes.Contains(output, []byte(client.terminal)) {
							t.Errorf("missing actual client terminal: %s", output)
						}
					} else {
						var raw []byte
						raw, err = io.ReadAll(resp.Body)
						if err != nil {
							t.Fatal(err)
						}
						usage = ParseUsageFromBody(raw)
						output, err = directConvertResponse(raw, client.path, "/v1/chat/completions", "provider-model", options)
					}
					if err != nil {
						t.Fatalf("response conversion failed: %s, %v", output, err)
					}
					for _, marker := range []string{"response-text", "echo", "9007199254740993"} {
						if !bytes.Contains(output, []byte(marker)) {
							t.Errorf("response lost %s: %s", marker, output)
						}
					}
					if client.name == "messages" {
						if savedReasoning != "response-thought" || bytes.Contains(output, []byte("response-thought")) {
							t.Fatal("Messages reasoning replay was lost or hidden thoughts exposed")
						}
					} else if !bytes.Contains(output, []byte("response-thought")) {
						t.Errorf("response reasoning was lost: %s", output)
					}
					if !usage.Found || usage.PromptTokens != 11 || usage.CompletionTokens != 7 || usage.TotalTokens != 18 {
						t.Fatalf("actual upstream usage changed: %+v", usage)
					}
				})
			}
		}
	}
}

func TestDirectDomesticProfilesMoonshotConvertedJSONMode(t *testing.T) {
	for _, client := range []struct{ path, body string }{
		{"/v1beta/models/kimi-test:generateContent", `{"contents":[{"role":"user","parts":[{"text":"JSON please"}]}],"generationConfig":{"responseMimeType":"application/json"}}`},
	} {
		converted, err := directConvertRequest([]byte(client.body), client.path, "/v1/chat/completions", "kimi-test", false, messages.Options{})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := prepareDirectDomesticProfileRequest(converted, "moonshot")
		if err != nil {
			t.Fatal(err)
		}
		format := domesticProfileTestObject(t, domesticProfileTestObject(t, wire)["response_format"])
		if string(format["type"]) != `"json_object"` || format["json_schema"] != nil {
			t.Fatalf("converted JSON Object mode changed: %s", wire)
		}
	}
}
