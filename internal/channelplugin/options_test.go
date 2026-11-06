package channelplugin

import (
	"encoding/json"
	"testing"
)

func TestNumericOptionsHaveExplicitFiniteBoundsAndExactTypes(t *testing.T) {
	min, max := 0.0, 2.0
	option := Option{Type: "number", Min: &min, Max: &max}
	for _, raw := range []string{"0", "1.25", "2"} {
		if !option.valid(json.RawMessage(raw)) {
			t.Errorf("valid option rejected: %s", raw)
		}
	}
	for _, raw := range []string{"null", " null ", `"1"`, "false", "[]", "{}", "3", "-1", "1e100"} {
		if option.valid(json.RawMessage(raw)) {
			t.Errorf("invalid option accepted: %s", raw)
		}
	}
	option.Type = "integer"
	if option.valid(json.RawMessage("1.25")) || !option.valid(json.RawMessage("1")) {
		t.Fatal("integer type not enforced")
	}
	option.Min = nil
	if option.valid(json.RawMessage("1")) {
		t.Fatal("missing bound accepted")
	}
}

func TestCancelledHookDoesNotPoisonNextCallAndMemoryIsBounded(t *testing.T) {
	p, err := Compile(t.Context(), meta+`export function request(i){if(i.model==="allocate"){let a=[];while(true){a.push(new Array(4096).fill("allocate"));}}return {};}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.Call(t.Context(), "request", Input{Protocol: "chat", Model: "allocate", Body: `{}`}); err == nil {
		t.Fatal("unbounded allocation accepted")
	}
	if _, err = p.Call(t.Context(), "request", Input{Protocol: "chat", Model: "normal", Body: `{}`}); err != nil {
		t.Fatalf("failed runtime poisoned program: %v", err)
	}
}
