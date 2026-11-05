package upstreamadapter

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func antigravityTestInput(body string) Input {
	return Input{
		Body: []byte(body), OAuth: true, ActualModel: "gemini-3-pro",
		ProjectID: "fixture-project",
		Downstream: http.Header{
			"Authorization":  {"Bearer must-not-be-copied"},
			"X-Goog-Api-Key": {"must-not-be-copied"},
		},
	}
}

func antigravityTestRequest(t *testing.T, wire *Wire) (map[string]any, map[string]any) {
	t.Helper()
	envelope, err := antigravityObject(wire.Body)
	if err != nil {
		t.Fatal(err)
	}
	request, ok := envelope["request"].(map[string]any)
	if !ok {
		t.Fatal("request is not an object")
	}
	return envelope, request
}

func TestAntigravityPrepareEnvelopePreservesToolsNumbersAndSignature(t *testing.T) {
	in := antigravityTestInput(`{
		"contents":[
			{"role":"user","parts":[{"text":"hello"}]},
			{"role":"model","parts":[{"functionCall":{"name":"echo","args":{"id":9007199254740993}},"thoughtSignature":"signed-receipt"}]},
			{"role":"user","parts":[{"functionResponse":{"name":"echo","response":{"id":9007199254740995}}}]}
		],
		"systemInstruction":{"role":"system","parts":[{"text":"Keep exact tool IDs"}]},
		"tools":[{"functionDeclarations":[{"name":"echo","parametersJsonSchema":{"type":"object","properties":{"id":{"type":"integer","minimum":9007199254740993},"type":{"type":"string"}},"required":["id"],"additionalProperties":false}}]}],
		"generationConfig":{"seed":9007199254740997,"responseJsonSchema":{"type":"object","properties":{"id":{"type":"integer","const":9007199254740999}}}}
	}`)
	original, headers := bytes.Clone(in.Body), in.Downstream.Clone()
	wire := &Wire{}
	if err := prepareAntigravity(in, wire); err != nil {
		t.Fatal(err)
	}
	envelope, request := antigravityTestRequest(t, wire)
	if envelope["project"] != "fixture-project" || envelope["model"] != "gemini-3-pro-low" ||
		envelope["requestType"] != "agent" || envelope["userAgent"] != "antigravity" {
		t.Fatalf("wrong envelope metadata: %#v", envelope)
	}
	id, _ := envelope["requestId"].(string)
	if !strings.HasPrefix(id, "agent-") {
		t.Fatalf("request ID missing namespace: %q", id)
	}
	if _, err := uuid.Parse(strings.TrimPrefix(id, "agent-")); err != nil {
		t.Fatalf("request ID is not a UUID: %v", err)
	}
	for _, value := range []string{"9007199254740993", "9007199254740995", "9007199254740997", "9007199254740999", "signed-receipt", `"functionResponse"`, `"functionCall"`} {
		if !bytes.Contains(wire.Body, []byte(value)) {
			t.Errorf("lost exact tool/number contract %s", value)
		}
	}
	tools := request["tools"].([]any)
	declaration := tools[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
	if declaration["parametersJsonSchema"] != nil {
		t.Fatal("modern schema field leaked into protobuf request")
	}
	schema := declaration["parameters"].(map[string]any)
	if schema["type"] != "OBJECT" || schema["additionalProperties"] != nil {
		t.Fatalf("invalid tool schema: %#v", schema)
	}
	properties := schema["properties"].(map[string]any)
	if properties["type"].(map[string]any)["type"] != "STRING" {
		t.Fatal("a property named type was treated as a keyword")
	}
	if request["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)["mode"] != "VALIDATED" {
		t.Fatal("tool mode is not VALIDATED")
	}
	system := request["systemInstruction"].(map[string]any)
	systemParts := system["parts"].([]any)
	if system["role"] != "user" ||
		systemParts[0].(map[string]any)["text"] != antigravityWireSystem+"\n\nKeep exact tool IDs" ||
		systemParts[1].(map[string]any)["text"] != "\n\n"+antigravityToolInstruction {
		t.Fatal("source system/tool instructions are not preserved")
	}
	generation := request["generationConfig"].(map[string]any)
	if generation["responseJsonSchema"] != nil ||
		generation["responseSchema"].(map[string]any)["type"] != "OBJECT" ||
		generation["thinkingConfig"].(map[string]any)["thinkingLevel"] != "low" {
		t.Fatal("response schema or thinking configuration missing")
	}
	if wire.Headers.Get("Authorization") != "" || wire.Headers.Get("X-Goog-Api-Key") != "" ||
		wire.Headers.Get("User-Agent") != antigravityWireUserAgent ||
		wire.Headers.Get("Accept") != "application/json" || wire.ForceStream {
		t.Fatalf("provider headers overrode authentication/stream ownership: %#v", wire.Headers)
	}
	if !bytes.Equal(in.Body, original) || !reflect.DeepEqual(in.Downstream, headers) {
		t.Fatal("adapter mutated caller input")
	}
	another := &Wire{}
	if err := prepareAntigravity(in, another); err != nil {
		t.Fatal(err)
	}
	second, _ := antigravityTestRequest(t, another)
	if second["requestId"] == envelope["requestId"] {
		t.Fatal("independent requests reused an identifier")
	}
}

func TestAntigravityModelAndThinkingContract(t *testing.T) {
	for _, tc := range []struct{ source, want string }{
		{"gemini-3-pro", "gemini-3-pro-low"},
		{"antigravity-gemini-3-pro:antigravity", "gemini-3-pro-low"},
		{"gemini-3-pro-high:gemini-cli", "gemini-3-pro-high"},
		{"gemini-2.5-pro", "gemini-2.5-pro"},
		{"claude-sonnet-4-5", "claude-sonnet-4-5"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			if got := antigravityModel(tc.source); got != tc.want {
				t.Fatalf("model=%q want %q", got, tc.want)
			}
		})
	}
	in := antigravityTestInput(`{"contents":[{"role":"model","parts":[{"thought":true,"text":"private reasoning"},{"functionCall":{"name":"echo","args":{}},"thoughtSignature":"keep-signature"}]}],"generationConfig":{"thinkingConfig":{"thinkingBudget":1024,"thinkingLevel":"high","includeThoughts":true}}}`)
	in.Stream = true
	wire := &Wire{}
	if err := prepareAntigravity(in, wire); err != nil {
		t.Fatal(err)
	}
	_, request := antigravityTestRequest(t, wire)
	thinking := request["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)
	if thinking["thinkingBudget"] != nil || thinking["thinkingLevel"] != "high" || wire.Headers.Get("Accept") != "text/event-stream" {
		t.Fatal("Gemini 3 stream/thinking contract changed")
	}
	in.ActualModel = "claude-sonnet-4-5"
	if err := prepareAntigravity(in, wire); err != nil {
		t.Fatal(err)
	}
	_, request = antigravityTestRequest(t, wire)
	parts := request["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["thoughtSignature"] != "keep-signature" {
		t.Fatal("Claude thought filtering lost a tool signature")
	}
	if request["generationConfig"].(map[string]any)["thinkingConfig"].(map[string]any)["thinkingBudget"] != json.Number("1024") {
		t.Fatal("non-Gemini-3 thinking budget changed")
	}
}

func TestAntigravityCreatesToolSystemInstructionWithoutExtraWhitespace(t *testing.T) {
	in := antigravityTestInput(`{"contents":[],"tools":[{"functionDeclarations":[{"name":"echo","parameters":{"type":"object"}}]}]}`)
	wire := &Wire{}
	if err := prepareAntigravity(in, wire); err != nil {
		t.Fatal(err)
	}
	_, request := antigravityTestRequest(t, wire)
	parts := request["systemInstruction"].(map[string]any)["parts"].([]any)
	if len(parts) != 1 || parts[0].(map[string]any)["text"] != antigravityWireSystem+"\n\n"+antigravityToolInstruction {
		t.Fatalf("unexpected source system instruction: %#v", parts)
	}
}

func TestAntigravitySchemaSourceVectors(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"const", `{"type":"integer","const":9007199254740993}`, `{"type":"INTEGER","enum":[9007199254740993]}`},
		{"constraints", `{"type":"string","minLength":5,"maxLength":10}`, `{"type":"STRING","description":"minLength: 5 (maxLength: 10)"}`},
		{"enum", `{"type":"string","enum":["a","b"]}`, `{"type":"STRING","enum":["a","b"],"description":"Allowed: a, b"}`},
		{"property keyword", `{"type":"object","properties":{"default":{"type":"string"},"title":{"type":"string"}}}`, `{"type":"OBJECT","properties":{"default":{"type":"STRING"},"title":{"type":"STRING"}}}`},
		{"reference", `{"$ref":"#/$defs/User","description":"Person"}`, `{"type":"OBJECT","description":"Person (See: User)","properties":{"_placeholder":{"type":"BOOLEAN","description":"Placeholder. Always pass true."}},"required":["_placeholder"]}`},
		{"empty object", `{"type":"object","additionalProperties":false}`, `{"type":"OBJECT","description":"No extra properties allowed","properties":{"_placeholder":{"type":"BOOLEAN","description":"Placeholder. Always pass true."}},"required":["_placeholder"]}`},
		{"allOf", `{"allOf":[{"properties":{"a":{"type":"string"}},"required":["a"]},{"properties":{"b":{"type":"integer"}}}]}`, `{"properties":{"a":{"type":"STRING"},"b":{"type":"INTEGER"}},"required":["a"]}`},
		{"allOf keeps child hints", `{"allOf":[{"type":"string","minLength":3}]}`, `{"type":"STRING","description":"minLength: 3"}`},
		{"union", `{"anyOf":[{"type":"string"},{"type":"integer"}]}`, `{"type":"STRING","description":"Accepts: string | integer"}`},
		{"union keeps selected constraints", `{"anyOf":[{"type":"string","minLength":3},{"type":"integer"}]}`, `{"type":"STRING","description":"minLength: 3 (Accepts: string | integer)"}`},
		{"union enum", `{"oneOf":[{"const":"a"},{"const":"b"}]}`, `{"type":"STRING","enum":["a","b"]}`},
		{"nullable", `{"type":"object","properties":{"a":{"type":["string","null"]}},"required":["a","missing"]}`, `{"type":"OBJECT","properties":{"a":{"type":"STRING","description":"nullable"}}}`},
		{"enum data is not a schema", `{"type":"string","enum":[{"type":"literal","title":"keep"}]}`, `{"type":"STRING","enum":[{"type":"literal","title":"keep"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, err := antigravityObject([]byte(tc.input))
			if err != nil {
				t.Fatal(err)
			}
			want, err := antigravityObject([]byte(tc.want))
			if err != nil {
				t.Fatal(err)
			}
			got := antigravitySchema(input)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("schema=%#v want %#v", got, want)
			}
		})
	}
}

func TestAntigravityPrepareRejectsInvalidInputs(t *testing.T) {
	for _, body := range []string{
		`null`, `[]`, `{}`, `{"contents":[]} trailing`,
		`{"generationConfig":[]}`, `{"tools":{}}`, `{"tools":[null]}`,
		`{"tools":[{"functionDeclarations":{}}]}`,
		`{"tools":[{"functionDeclarations":[null]}]}`,
		`{"tools":[{"functionDeclarations":[{"parametersJsonSchema":true}]}]}`,
		`{"systemInstruction":{"parts":[null]}}`,
	} {
		in := antigravityTestInput(body)
		if body == `{}` {
			in.OAuth = false
		}
		if err := prepareAntigravity(in, &Wire{}); err == nil {
			t.Errorf("accepted invalid request %s", body)
		}
	}
	for _, change := range []func(*Input){
		func(in *Input) { in.ProjectID = " " },
		func(in *Input) { in.ActualModel = "" },
	} {
		in := antigravityTestInput(`{"contents":[]}`)
		change(&in)
		if prepareAntigravity(in, &Wire{}) == nil {
			t.Fatal("accepted absent project/model")
		}
	}
}

func TestAntigravityUnwrapJSONAndSSE(t *testing.T) {
	const native = `{"candidates":[{"content":{"parts":[{"functionCall":{"args":{"id":9007199254740993}},"thoughtSignature":"opaque-signature"}]},"finishReason":"STOP"}],"usageMetadata":{"totalTokenCount":7},"future":{"raw":9007199254740995}}`
	for _, stream := range []bool{false, true} {
		body := []byte(`{"response":` + native + `,"trace":"not-a-Gemini-field"}`)
		got, err := unwrapAntigravityResponse(body, stream)
		if err != nil || string(got) != native {
			t.Fatalf("unwrap=%s err=%v", got, err)
		}
		got[0] = '['
		if !bytes.Contains(body, []byte(`"response":{`)) {
			t.Fatal("unwrapped response aliases caller memory")
		}
		for _, raw := range []string{`null`, `[]`, `{}`, `{"response":null}`, `{"response":[]}`, `{"response":{}}`, `{"response":{},"error":{"code":500}}`, `{"response":{"error":{"code":500}}}`, `{"response":{"candidates":[]}} garbage`, `{"response":{"candidates":null}}`, `{"response":{"candidates":[null]}}`, `{"response":{"candidates":"broken"}}`, `{"response":{"usageMetadata":[]}}`, `[DONE]`} {
			if _, err := unwrapAntigravityResponse([]byte(raw), stream); err == nil {
				t.Errorf("accepted invalid envelope %s", raw)
			}
		}
		for _, raw := range []string{`{"response":{"promptFeedback":{"blockReason":"SAFETY"}}}`, `{"response":{"usageMetadata":{"totalTokenCount":4}}}`} {
			if _, err := unwrapAntigravityResponse([]byte(raw), stream); err != nil {
				t.Errorf("rejected native metadata event: %v", err)
			}
		}
	}
}
