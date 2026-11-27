package shared

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
)

var ErrTrailingJSON = errors.New("trailing JSON")

// DecodeJSON reads one complete value and preserves numbers in interface values.
func DecodeJSON(r io.Reader, dst any) error {
	d := json.NewDecoder(r)
	d.UseNumber()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrTrailingJSON
	}
	return nil
}

// RawString returns the string value of a JSON field, or "" if absent or invalid.
func RawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

// UnsupportedField reports a non-null field outside the allowlist.
func UnsupportedField(obj map[string]any, allowed ...string) (string, bool) {
	for key, value := range obj {
		if value != nil && !slices.Contains(allowed, key) {
			return key, true
		}
	}
	return "", false
}

// CloneObject returns a writable shallow copy, including for a nil source.
func CloneObject(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
