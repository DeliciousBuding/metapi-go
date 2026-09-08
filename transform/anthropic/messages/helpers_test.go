package messages_test

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"testing"
)

func decodeJSON(t *testing.T, data []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		t.Fatalf("invalid JSON %q: %v", data, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		t.Fatalf("trailing JSON in %q: %v", data, err)
	}
	return value
}

func jsonObject(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %#v", value)
	}
	return out
}

func jsonArray(t *testing.T, value any) []any {
	t.Helper()
	out, ok := value.([]any)
	if !ok {
		t.Fatalf("expected array, got %#v", value)
	}
	return out
}

func marshalJSON(t *testing.T, value any) []byte {
	t.Helper()
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func requireJSON(t *testing.T, got []byte, want string) {
	t.Helper()
	if !reflect.DeepEqual(decodeJSON(t, got), decodeJSON(t, []byte(want))) {
		t.Errorf("JSON mismatch\ngot:  %s\nwant: %s", got, want)
	}
}
