package shared

import "testing"

func TestAsTrimmedString(t *testing.T) {
	if AsTrimmedString("  hello  ") != "hello" {
		t.Error("trimmed failed")
	}
	if AsTrimmedString(123) != "" {
		t.Error("non-string should return empty")
	}
}

func TestParseJSONLike_Valid(t *testing.T) {
	result := ParseJSONLike(`{"key":"value"}`)
	m, ok := result.(map[string]any)
	if !ok || m["key"] != "value" {
		t.Errorf("unexpected: %v", result)
	}
}

func TestParseJSONLike_Empty(t *testing.T) {
	result := ParseJSONLike("")
	_, ok := result.(map[string]any)
	if !ok {
		t.Errorf("expected empty map, got %T", result)
	}
}

func TestParseJSONLike_Invalid(t *testing.T) {
	result := ParseJSONLike("not json")
	m, ok := result.(map[string]any)
	if !ok || m["value"] != "not json" {
		t.Errorf("expected wrapped string, got %v", result)
	}
}
