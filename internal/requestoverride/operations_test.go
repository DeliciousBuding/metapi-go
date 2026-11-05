package requestoverride

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestOperationsPreserveUnmodifiedLargeIntegers(t *testing.T) {
	body := []byte(`{"items":[{"value":9007199254740993}],"source":{"value":18446744073709551615}}`)
	ops := []Operation{
		{Op: "array_append", Path: "items", Value: `{"value":9007199254740995}`},
		{Op: "copy", From: "source", To: "copy"},
		{Op: "rename", From: "copy", To: "renamed"},
		{Op: "array_prepend", Path: "items", Value: `[9007199254740997]`, Splat: boolPtr(false)},
	}
	out, _, err := ApplyBody(body, ops, Context{}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"9007199254740993", "9007199254740995", "9007199254740997", "18446744073709551615"} {
		if !strings.Contains(string(out), value) {
			t.Fatalf("integer %s rounded in %s", value, out)
		}
	}
}

func TestOperationsCannotRemoveGeminiOutputLimitThroughEscapedOrNestedPaths(t *testing.T) {
	for _, path := range []string{
		`generationConfig.maxOutputToken\s`,
		`generationConfig.maxOutputTokens.nested`,
		`generation_config.max_output_token\s`,
		`generation_config.max_output_tokens.nested`,
	} {
		t.Run(path, func(t *testing.T) {
			_, _, err := ApplyBody([]byte(`{"generationConfig":{"maxOutputTokens":16}}`),
				[]Operation{{Op: "set", Path: path, Value: `"unbounded"`}}, Context{}, 1<<20)
			if err == nil {
				t.Fatal("request operation could replace or remove the bounded output limit")
			}
		})
	}
}

func TestOperationConditionsErrorsAndNullPresence(t *testing.T) {
	body := []byte(`{"value":null,"items":7}`)
	ops := []Operation{
		{Op: "set_if_absent", Path: "value", Value: "1"},
		{Op: "set", Path: "ignored", Value: "true", Condition: "false"},
		{Op: "array_append", Path: "items", Value: "1"},
		{Op: "set", Path: "continued", Value: "true", Condition: `{{eq .Model "m"}}`},
		{Op: "set", Path: "literal", Value: "{{.Missing}}"},
	}
	out, warnings, err := ApplyBody(body, ops, Context{Model: "m"}, 1<<20)
	if err != nil || len(warnings) != 2 {
		t.Fatalf("warnings=%+v err=%v", warnings, err)
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal(out, &object)
	if string(object["value"]) != "null" || len(object["ignored"]) != 0 || string(object["items"]) != "7" ||
		string(object["continued"]) != "true" || string(object["literal"]) != `"{{.Missing}}"` {
		t.Fatalf("best effort contract changed: %s", out)
	}
}

func TestContextNeverExposesAuthenticationHeaders(t *testing.T) {
	headers := http.Header{}
	for _, name := range []string{"Authorization", "X-Api-Key", "Cookie", "X-Google-Api-Key", "Api-Key", "Proxy-Authorization", "X-Admin-Confirm-Token"} {
		headers.Set(name, "synthetic-hidden")
	}
	headers.Set("X-Request-ID", "public-request")
	ctx := NewContext([]byte(`{"metadata":{"task":"public"},"reasoning_effort":"high"}`), headers, "client", "provider", "chat")
	raw, _ := json.Marshal(ctx)
	if strings.Contains(string(raw), "synthetic-hidden") || ctx.RequestHeader["X-Request-Id"] != "public-request" {
		t.Fatalf("unsafe context %s", raw)
	}
	_, _, err := ApplyHeaders(nil, []Operation{{Op: "set", Path: "X-Output", Value: "{{.Model}}"}}, Context{Model: "injected\r\nvalue"})
	if err == nil {
		t.Fatal("rendered header newline accepted")
	}
}

func TestOperationBodyLimitAndRuntimeWarningsDoNotExposeValues(t *testing.T) {
	ops := []Operation{{Op: "set", Path: "metadata", Value: `"a value larger than the request limit"`}}
	out, _, err := ApplyBody([]byte(`{}`), ops, Context{}, 16)
	if err == nil || out != nil || strings.Contains(err.Error(), "a value") {
		t.Fatalf("body limit=%s %v", out, err)
	}
	secret := "synthetic-private-render-input"
	ctx := Context{Model: secret}
	ops = []Operation{{Op: "set", Path: "value", Value: `{{index .Model 999999}}`}}
	_, warnings, err := ApplyBody([]byte(`{}`), ops, ctx, 1024)
	if err != nil || len(warnings) != 1 {
		t.Fatalf("render failure=%v warnings=%v", err, warnings)
	}
	raw, _ := json.Marshal(warnings)
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), "999999") {
		t.Fatal("warning exposed values")
	}
}

func TestValidateRejectsGrantBoundaryMutations(t *testing.T) {
	for _, op := range []Operation{
		{Op: "set", Path: "model", Value: "other"},
		{Op: "set", Path: "stream", Value: "false"},
		{Op: "rename", From: "model", To: "other"},
		{Op: "set", Path: ":model", Value: "other"},
		{Op: "set", Path: "max_tokens", Value: "10000000"},
	} {
		if err := (Config{Body: []Operation{op}}).Validate(); err == nil {
			t.Fatalf("operation %+v unexpectedly accepted", op)
		}
	}
	for _, op := range []Operation{
		{Op: "set", Path: "Authorization", Value: "Bearer x"},
		{Op: "copy", From: "X-Request-ID", To: "X-Api-Key"},
	} {
		if err := (Config{Headers: []Operation{op}}).Validate(); err == nil {
			t.Fatalf("header operation %+v unexpectedly accepted", op)
		}
	}
}

func TestApplyBodyOrderedOperationsAndTemplates(t *testing.T) {
	body := []byte(`{"model":"public","messages":[{"role":"user","content":"hello"}],"metadata":{"source":"fixture"},"items":[{"value":"a"}]}`)
	ctx := Context{
		RequestModel: "public",
		Model:        "public",
		Metadata:     map[string]string{"trace": "t-1"},
		RequestHeader: map[string]string{
			"X-Request-Id": "req-1",
		},
	}
	ops := []Operation{
		{Op: "set", Path: "metadata.trace", Value: "{{index .RequestHeader \"X-Request-Id\"}}"},
		{Op: "set_if_absent", Path: "metadata.source", Value: "changed"},
		{Op: "array_append", Path: "items", Value: `[{"value":"b"},{"value":"c"}]`},
		{Op: "array_insert", Path: "items", Index: intPtr(1), Value: `{"value":"middle"}`},
		{Op: "array_remove", Path: "items", Match: &Match{Path: "value", Eq: "b"}},
		{Op: "copy", From: "metadata.source", To: "metadata.copy"},
		{Op: "rename", From: "metadata.copy", To: "metadata.renamed"},
		{Op: "delete", Path: "metadata.source"},
	}
	got, warnings, err := ApplyBody(body, ops, ctx, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", warnings)
	}
	want := `{"model":"public","messages":[{"role":"user","content":"hello"}],"metadata":{"trace":"req-1","renamed":"fixture"},"items":[{"value":"a"},{"value":"middle"},{"value":"c"}]}`
	if string(got) != want {
		t.Fatalf("body = %s, want %s", got, want)
	}
	if string(body) != `{"model":"public","messages":[{"role":"user","content":"hello"}],"metadata":{"source":"fixture"},"items":[{"value":"a"}]}` {
		t.Fatalf("input body was mutated: %s", body)
	}
}

func TestApplyHeadersUsesSafeContextAndClones(t *testing.T) {
	headers := http.Header{"X-Request-Id": {"req-1"}, "X-Old": {"old"}}
	ctx := Context{RequestHeader: map[string]string{"X-Request-Id": "req-1"}}
	got, warnings, err := ApplyHeaders(headers, []Operation{
		{Op: "set", Path: "X-Trace", Value: "{{index .RequestHeader \"X-Request-Id\"}}"},
		{Op: "copy", From: "X-Old", To: "X-Copied"},
		{Op: "rename", From: "X-Old", To: "X-New"},
		{Op: "delete", Path: "X-Trace"},
	}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 || got.Get("X-Trace") != "" || got.Get("X-Copied") != "old" || got.Get("X-New") != "old" {
		t.Fatalf("headers=%v warnings=%v", got, warnings)
	}
	if headers.Get("X-Old") != "old" || headers.Get("X-Trace") != "" {
		t.Fatalf("input headers were mutated: %v", headers)
	}
}

func TestDecodeRejectsUnknownFields(t *testing.T) {
	var cfg Config
	if err := Decode([]byte(`{"body":[{"op":"set","path":"x","future":true}]}`), &cfg); err == nil {
		t.Fatal("unknown operation field unexpectedly accepted")
	}
	if err := Decode([]byte(`{"body":[]}`), &cfg); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(cfg.Body, []Operation{}) {
		t.Fatalf("decoded body=%#v", cfg.Body)
	}
}

func intPtr(value int) *int    { return &value }
func boolPtr(value bool) *bool { return &value }
