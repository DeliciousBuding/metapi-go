package routecondition

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func predicate(t *testing.T, field, operator, value string) *Condition {
	t.Helper()
	raw := fmt.Sprintf(`{"type":"group","conditions":[{"type":"condition","field":%q,"operator":%q,"value":%s}]}`, field, operator, value)
	result, err := Decode([]byte(raw))
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return result
}

func TestConditionNumericBoundariesAndAliases(t *testing.T) {
	for _, tc := range []struct {
		op    string
		value string
		input int64
		want  bool
	}{
		{"lt", "100", 99, true}, {"<", "100", 100, false},
		{"lte", "100", 100, true}, {"<=", "100", 101, false},
		{"gt", "100", 101, true}, {">", "100", 100, false},
		{"gte", "100", 100, true}, {">=", "100", 99, false},
		{"gte", "1e2", 100, true}, {"lt", "100.0", 99, true},
		{"gte", "0", 0, true},
		{"lt", "9007199254740993", 9007199254740992, true},
		{"gte", "9223372036854775807", 9223372036854775807, true},
	} {
		t.Run(tc.op+"/"+tc.value+fmt.Sprint(tc.input), func(t *testing.T) {
			condition := predicate(t, "prompt_tokens", tc.op, tc.value)
			got, evaluated := condition.Match(&Request{PromptTokens: tc.input})
			if !evaluated || got != tc.want {
				t.Fatalf("matched=%v evaluated=%v want=%v", got, evaluated, tc.want)
			}
		})
	}
}

func TestConditionBooleanFormatAndNestedGroups(t *testing.T) {
	request := &Request{Stream: true, RequestFormat: "openai/responses", HasImage: true, HasDocument: true}
	for _, field := range []string{"stream", "has_image", "has_video", "has_document", "has_audio"} {
		expected := field == "stream" || field == "has_image" || field == "has_document"
		for _, op := range []string{"eq", "=", "==", "ne", "!="} {
			got, evaluated := predicate(t, field, op, "true").Match(request)
			want := expected
			if op == "ne" || op == "!=" {
				want = !want
			}
			if !evaluated || got != want {
				t.Fatalf("%s %s: matched=%v evaluated=%v", field, op, got, evaluated)
			}
		}
	}
	for _, tc := range []struct {
		op, value string
		want      bool
	}{
		{"eq", `"openai/responses"`, true},
		{"!=", `"anthropic/messages"`, true},
		{"==", `"gemini/contents"`, false},
	} {
		got, evaluated := predicate(t, "request_format", tc.op, tc.value).Match(request)
		if !evaluated || got != tc.want {
			t.Fatalf("format %s: matched=%v evaluated=%v", tc.op, got, evaluated)
		}
	}
	condition, err := Decode([]byte(`{"logic":"AND","conditions":[
		{"type":"condition","field":"stream","operator":"eq","value":true},
		{"type":"group","logic":"or","conditions":[
			{"type":"condition","field":"has_audio","operator":"eq","value":true},
			{"type":"group","conditions":[{"type":"condition","field":"prompt_tokens","operator":"lt","value":10}]}
		]}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	got, evaluated := condition.Match(request)
	if !got || !evaluated {
		t.Fatal("source three-level group should match")
	}
	request.PromptTokens = 10
	if got, evaluated = condition.Match(request); got || !evaluated {
		t.Fatal("false nested alternatives should reject")
	}
}

func TestConditionRequestHeaders(t *testing.T) {
	request := &Request{Headers: map[string]string{"X-Client-Name": "claude-cli/fixture"}}
	for _, tc := range []struct {
		op, value string
		want      bool
	}{
		{"eq", `"claude-cli/fixture"`, true},
		{"<>", `"other"`, true},
		{"contains", `"cli/"`, true},
		{"not_contains", `"secret"`, true},
		{"start_with", `"claude"`, true},
		{"end_with", `"fixture"`, true},
		{"eq", `"CLAUDE-CLI/FIXTURE"`, false},
		{"not_contains", `"cli"`, false},
	} {
		got, evaluated := predicate(t, "request_header.x-client-name", tc.op, tc.value).Match(request)
		if !evaluated || got != tc.want {
			t.Fatalf("%s: matched=%v evaluated=%v", tc.op, got, evaluated)
		}
	}
	// The source evaluator reads a missing map[string]string key as "".
	for _, op := range []string{"ne", "not_contains"} {
		got, evaluated := predicate(t, "request_header.X-Absent", op, `"fixture"`).Match(request)
		if !got || !evaluated {
			t.Fatalf("missing-header %s differs from source string-map semantics", op)
		}
	}
	if got, evaluated := predicate(t, "request_header.X-Absent", "contains", `"fixture"`).Match(request); got || !evaluated {
		t.Fatal("missing header must not contain a nonempty string")
	}
}

func TestConditionDailyTimeUsesInjectedLocationAndBoundaries(t *testing.T) {
	loc := time.FixedZone("fixture-local", 8*60*60)
	for _, tc := range []struct {
		hour, minute int
		rangeValue   string
		want         bool
	}{
		{10, 0, "09:00-17:00", true},
		{8, 59, "09:00-17:00", false},
		{9, 0, "09:00-17:00", true},
		{16, 59, "09:00-17:00", true},
		{17, 0, "09:00-17:00", false},
		{23, 30, "22:00-06:00", true},
		{0, 0, "22:00-06:00", true},
		{5, 59, "22:00-06:00", true},
		{6, 0, "22:00-06:00", false},
		{12, 0, "22:00-06:00", false},
	} {
		t.Run(fmt.Sprintf("%s/%02d:%02d", tc.rangeValue, tc.hour, tc.minute), func(t *testing.T) {
			request := &Request{Now: time.Date(2026, 11, 5, tc.hour, tc.minute, 59, 0, loc)}
			for _, op := range []string{"within", "not_within"} {
				expected := tc.want
				if op == "not_within" {
					expected = !expected
				}
				got, evaluated := predicate(t, "daily_time", op, fmt.Sprintf("%q", tc.rangeValue)).Match(request)
				if !evaluated || got != expected {
					t.Fatalf("%s matched=%v evaluated=%v want=%v", op, got, evaluated, expected)
				}
			}
		})
	}
}

func TestConditionDecodeRejectsAmbiguousOrUnsupportedAuthorization(t *testing.T) {
	for _, raw := range []string{
		``, `null null`, `[]`, `{}`, `{"type":"group","conditions":[]}`,
		`{"type":"condition","field":"stream","operator":"eq","value":true}`,
		`{"logic":"xor","conditions":[{"field":"stream","operator":"eq","value":true}]}`,
		`{"conditions":[{"field":"stream","operator":"eq","value":true}],"secret":"synthetic-sensitive-marker"}`,
		`{"conditions":[{"field":"stream","operator":"eq","value":true,"future":false}]}`,
		`{"conditions":[{"field":"stream","operator":"eq","value":true,"value":false}]}`,
		`{"Conditions":[{"field":"stream","operator":"eq","value":true}]}`,
		`{"conditions":[{"type":"group","field":"stream","conditions":[{"field":"stream","operator":"eq","value":true}]}]}`,
		`{"conditions":[{"type":"group","conditions":[{"type":"group","conditions":[{"type":"group","conditions":[{"field":"stream","operator":"eq","value":true}]}]}]}]}`,
		`{"conditions":[{"field":"unknown","operator":"eq","value":true}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"eq","value":100}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"gt","value":-1}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"gt","value":1.5}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"gt","value":"100"}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"gt","value":9223372036854775808}]}`,
		`{"conditions":[{"field":"prompt_tokens","operator":"gt","value":1e999999999}]}`,
		`{"conditions":[{"field":"stream","operator":"eq","value":null}]}`,
		`{"conditions":[{"field":"stream","operator":"eq","value":"true"}]}`,
		`{"conditions":[{"field":"stream","operator":"<>","value":true}]}`,
		`{"conditions":[{"field":"request_format","operator":"contains","value":"openai"}]}`,
		`{"conditions":[{"field":"request_format","operator":"eq","value":""}]}`,
		`{"conditions":[{"field":"daily_time","operator":"within","value":"09:00-09:00"}]}`,
		`{"conditions":[{"field":"daily_time","operator":"within","value":"24:00-06:00"}]}`,
		`{"conditions":[{"field":"daily_time","operator":"within","value":"09:00:01-17:00"}]}`,
		`{"conditions":[{"field":"request_header.Authorization","operator":"ne","value":"synthetic-sensitive-marker"}]}`,
		`{"conditions":[{"field":"request_header.X-Admin-Confirm-Token","operator":"eq","value":"synthetic-sensitive-marker"}]}`,
		`{"conditions":[{"field":"request_header.X-Metapi-Key","operator":"ne","value":"synthetic-sensitive-marker"}]}`,
		`{"conditions":[{"field":"request_header.X-Test\\nBad","operator":"eq","value":"fixture"}]}`,
		`{"conditions":[{"field":"request_header.X-Test","operator":"gt","value":"fixture"}]}`,
		`{"conditions":[{"field":"request_header.X-Test","operator":"eq","value":5}]}`,
	} {
		t.Run(raw, func(t *testing.T) {
			condition, err := Decode([]byte(raw))
			if condition != nil || err == nil {
				t.Fatalf("invalid predicate accepted: %v", condition)
			}
			if strings.Contains(err.Error(), "synthetic-sensitive-marker") {
				t.Fatal("validation leaked operator-supplied data")
			}
		})
	}
}

func TestConditionNoRequestAndMalformedASTNeverMatch(t *testing.T) {
	var absent *Condition
	if matched, evaluated := absent.Match(nil); !matched || !evaluated {
		t.Fatal("absent predicate must remain unconditional")
	}
	if absent, err := Decode([]byte(" null ")); err != nil || absent != nil {
		t.Fatal("null predicate must remain absent")
	}
	condition := predicate(t, "stream", "eq", "false")
	if matched, evaluated := condition.Match(nil); matched || evaluated {
		t.Fatal("missing request context must not evaluate as false stream")
	}
	for _, invalid := range []*Condition{
		{},
		{Type: "group", Conditions: []Condition{{Field: "unknown", Operator: "ne", Value: json.RawMessage(`true`)}}},
	} {
		if matched, evaluated := invalid.Match(&Request{}); matched || evaluated {
			t.Fatal("invalid direct AST must not grant access")
		}
	}
	if matched, evaluated := predicate(t, "daily_time", "not_within", `"09:00-17:00"`).Match(&Request{}); matched || evaluated {
		t.Fatal("missing clock must not match an inverse time condition")
	}
	if matched, evaluated := predicate(t, "request_format", "ne", `"anthropic/messages"`).Match(&Request{}); matched || evaluated {
		t.Fatal("missing format must not match an inverse format condition")
	}
}

func TestConditionConcurrentMatchesDoNotMutateASTOrInput(t *testing.T) {
	condition := predicate(t, "request_header.X-Client", "contains", `"public"`)
	before, err := json.Marshal(condition)
	if err != nil {
		t.Fatal(err)
	}
	request := &Request{Headers: map[string]string{"X-Client": "public-client"}}
	headers := map[string]string{"X-Client": "public-client"}
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if matched, evaluated := condition.Match(request); !matched || !evaluated {
				t.Error("same immutable request did not match")
			}
		}()
	}
	group.Wait()
	after, _ := json.Marshal(condition)
	if string(before) != string(after) || !reflect.DeepEqual(request.Headers, headers) {
		t.Fatal("matcher mutated its inputs")
	}
}
