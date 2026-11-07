package requestoverride

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// RejectDuplicateJSONKeys validates exactly one value without choosing a first
// or last duplicate. Native identity and cap checks must agree with the sender.
func RejectDuplicateJSONKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := scanJSONValue(dec); err != nil {
		return err
	}
	if tok, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return fmt.Errorf("request body must contain a single JSON value, got extra token %v", tok)
	}
	return nil
}

func scanJSONValue(dec *json.Decoder) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}

	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		return scanJSONObject(dec)
	case '[':
		return scanJSONArray(dec)
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delim)
	}
}

func scanJSONObject(dec *json.Decoder) error {
	seen := make(map[string]struct{})
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("invalid JSON object key token %v", tok)
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate JSON object key %q", key)
		}
		seen[key] = struct{}{}
		if err := scanJSONValue(dec); err != nil {
			return err
		}
	}

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('}') {
		return fmt.Errorf("expected JSON object close, got %v", tok)
	}
	return nil
}

func scanJSONArray(dec *json.Decoder) error {
	for dec.More() {
		if err := scanJSONValue(dec); err != nil {
			return err
		}
	}

	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim(']') {
		return fmt.Errorf("expected JSON array close, got %v", tok)
	}
	return nil
}
