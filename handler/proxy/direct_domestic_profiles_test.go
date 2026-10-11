package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestDirectDomesticProfilesMoonshotSchemaDowngrade(t *testing.T) {
	body := []byte(`{"model":"kimi-test","messages":[{"role":"user","content":"JSON please"}],"response_format":{"type":"json_schema","json_schema":{"name":"receipt","strict":true,"schema":{"type":"object","properties":{"n":{"type":"integer","const":9007199254740993}},"required":["n"],"additionalProperties":false}},"vendor":{"n":9007199254740995}},"seed":9007199254740997,"metadata":{"ratio":1.2300000000000000001e+20}}`)
	original := bytes.Clone(body)
	got, err := prepareDirectDomesticProfileRequest(body, "moonshot")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, original) {
		t.Fatal("request was mutated")
	}
	doc := domesticProfileTestObject(t, got)
	format := domesticProfileTestObject(t, doc["response_format"])
	if string(format["type"]) != `"json_object"` || format["json_schema"] != nil {
		t.Fatalf("schema was not degraded to JSON Object: %s", got)
	}
	// This adapter promises JSON Object mode only. A strict schema must not be
	// forwarded or advertised as enforced after this explicit source downgrade.
	for _, removed := range []string{`"strict"`, `"schema"`, `9007199254740993`, `"required"`, `"additionalProperties"`} {
		if bytes.Contains(got, []byte(removed)) {
			t.Fatalf("schema constraint still sent upstream: %s", got)
		}
	}
	for _, kept := range []string{`9007199254740995`, `9007199254740997`, `1.2300000000000000001e+20`} {
		if !bytes.Contains(got, []byte(kept)) {
			t.Fatalf("unrelated numeric metadata changed: %s", got)
		}
	}
	again, err := prepareDirectDomesticProfileRequest(got, "moonshot")
	if err != nil || !bytes.Equal(got, again) {
		t.Fatalf("normalization is not idempotent: %s, %v", again, err)
	}
}

func TestDirectDomesticProfilesReasoningAliasesPreserveHistory(t *testing.T) {
	for _, profile := range []string{"moonshot", "longcat"} {
		for _, aliases := range []string{
			`"reasoning":"history thought"`,
			`"reasoning_content":null,"reasoning":"history thought"`,
			`"reasoning_content":"history thought","reasoning":"history thought"`,
			`"reasoning_content":"history thought","reasoning":null`,
		} {
			t.Run(profile+"/"+aliases, func(t *testing.T) {
				body := []byte(`{"model":"test","reasoning_effort":"high","messages":[{"role":"assistant","content":null,` + aliases + `,"reasoning_signature":"opaque-signature","tool_calls":[{"id":"history-id","type":"function","function":{"name":"namespace__lookup","arguments":"{\"n\":9007199254740993}"},"vendor":{"n":9007199254740995}}],"vendor_message":{"n":9007199254740997}},{"role":"tool","tool_call_id":"history-id","content":"tool result"},{"role":"user","content":"continue"}],"vendor_request":{"n":9007199254740999}}`)
				before := bytes.Clone(body)
				got, err := prepareDirectDomesticProfileRequest(body, profile)
				if err != nil || !bytes.Equal(body, before) {
					t.Fatalf("request failed or input mutated: %s, %v", got, err)
				}
				for _, kept := range []string{`"reasoning_content":"history thought"`, `"reasoning_effort":"high"`, "opaque-signature", "namespace__lookup", "history-id", "tool result", "continue", "9007199254740993", "9007199254740995", "9007199254740997", "9007199254740999"} {
					if !bytes.Contains(got, []byte(kept)) {
						t.Fatalf("history or metadata lost, missing %s: %s", kept, got)
					}
				}
				if bytes.Contains(got, []byte(`"reasoning":`)) {
					t.Fatalf("alias was not normalized: %s", got)
				}
			})
		}
	}
}

func TestDirectDomesticProfilesLongcatAllRolesAndMultimodalParts(t *testing.T) {
	for _, role := range []string{"system", "developer", "user", "assistant", "tool"} {
		for _, content := range []string{"", `,"content":null`, `,"content":""`, `,"content":[]`, `,"content":"hello\n世界"`} {
			t.Run(role+"/"+content, func(t *testing.T) {
				body := []byte(`{"messages":[{"role":` + fmt.Sprintf("%q", role) + content + `,"vendor":{"n":9007199254740993}}]}`)
				before := bytes.Clone(body)
				got, err := prepareDirectDomesticProfileRequest(body, "longcat")
				if err != nil || !bytes.Equal(body, before) {
					t.Fatalf("request failed or input mutated: %s, %v", got, err)
				}
				var messages []map[string]json.RawMessage
				_ = json.Unmarshal(domesticProfileTestObject(t, got)["messages"], &messages)
				var parts []map[string]json.RawMessage
				if err := json.Unmarshal(messages[0]["content"], &parts); err != nil || len(parts) != 1 || string(parts[0]["type"]) != `"text"` {
					t.Fatalf("content is not a text array: %s, %v", got, err)
				}
				var text string
				_ = json.Unmarshal(parts[0]["text"], &text)
				want := ""
				if strings.Contains(content, "hello") {
					want = "hello\n世界"
				}
				if text != want || !bytes.Contains(got, []byte("9007199254740993")) {
					t.Fatalf("text or metadata changed: %s", got)
				}
				again, err := prepareDirectDomesticProfileRequest(got, "longcat")
				if err != nil || !bytes.Equal(got, again) {
					t.Fatalf("normalization is not idempotent: %s, %v", again, err)
				}
			})
		}
	}
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"look","vendor":{"n":9007199254740993}},{"type":"image_url","image_url":{"url":"data:image/png;base64,aGk=","detail":"high"}},{"type":"input_audio","input_audio":{"data":"aGk=","format":"wav"}},{"type":"video_url","video_url":{"url":"https://example.invalid/video","fps":1.2300000000000000001}},{"type":"provider_part","opaque":{"id":"preserve"}}]}],"response_format":{"type":"json_schema","json_schema":{"name":"kept","schema":{"type":"object"},"strict":true}}}`)
	got, err := prepareDirectDomesticProfileRequest(body, "longcat")
	if err != nil || !bytes.Equal(body, got) {
		t.Fatalf("existing multimodal arrays, schema or extensions changed: %s, %v", got, err)
	}
}

func TestDirectDomesticProfilesRejectInvalidFormatsAndConflicts(t *testing.T) {
	for _, format := range []string{`[]`, `"json_object"`, `false`, `{}`, `{"type":null}`, `{"type":3}`, `{"type":"unknown"}`, `{"type":"json_schema"}`, `{"type":"json_schema","json_schema":null}`, `{"type":"json_schema","json_schema":[]}`, `{"type":"text","json_schema":{"schema":{}}}`} {
		body := []byte(`{"messages":[{"role":"user","content":"hi"}],"response_format":` + format + `}`)
		if _, err := prepareDirectDomesticProfileRequest(body, "moonshot"); err == nil {
			t.Fatalf("invalid response_format accepted: %s", format)
		}
	}
	for _, profile := range []string{"moonshot", "longcat"} {
		for _, body := range []string{`null`, `[]`, `{}`, `{"messages":null}`, `{"messages":[]}`, `{"messages":[null]}`, `{"messages":[1]}`, `{"messages":[{"reasoning":{}}]}`, `{"messages":[{"reasoning_content":[]}]}`, `{"messages":[{"reasoning_content":"first","reasoning":"second"}]}`, `{"messages":[{"reasoning_content":"","reasoning":"nonempty"}]}`, "{\"messages\":[{\"content\":\"\xff\"}]}"} {
			if _, err := prepareDirectDomesticProfileRequest([]byte(body), profile); err == nil {
				t.Fatalf("invalid %s request accepted: %q", profile, body)
			}
		}
	}
	for _, content := range []string{`123`, `true`, `{}`, `[null]`, `[123]`, `["plain"]`} {
		if _, err := prepareDirectDomesticProfileRequest([]byte(`{"messages":[{"role":"user","content":`+content+`}]}`), "longcat"); err == nil {
			t.Fatalf("invalid LongCat content accepted: %s", content)
		}
	}
	if _, err := prepareDirectDomesticProfileRequest([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), "moonshot-anthropic"); err == nil {
		t.Fatal("native Messages endpoint received Chat normalization")
	}
}

func TestDirectDomesticProfilesMoonshotPlainRequestUnchanged(t *testing.T) {
	for _, format := range []string{"", `,"response_format":null`, `,"response_format":{"type":"text"}`, `,"response_format":{"type":"json_object","vendor":{"n":9007199254740993}}`} {
		body := []byte(`{"messages":[{"role":"assistant","content":null,"reasoning_content":"history"},{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.invalid/image"}}]}]` + format + `}`)
		got, err := prepareDirectDomesticProfileRequest(body, "moonshot")
		if err != nil || !bytes.Equal(body, got) {
			t.Fatalf("plain Moonshot request changed: %s, %v", got, err)
		}
	}
}

func domesticProfileTestObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil || doc == nil {
		t.Fatalf("invalid fixture object: %s, %v", raw, err)
	}
	return doc
}
