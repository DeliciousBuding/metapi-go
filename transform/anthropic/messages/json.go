package messages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

type rawObject map[string]json.RawMessage

type wireObject = map[string]any

func invalid(path, reason string) error {
	return fmt.Errorf("messages Chat bridge: %s %s", path, reason)
}

func unsupported(path string) error {
	return invalid(path, "is not supported by Chat fallback")
}

func absent(raw json.RawMessage) bool {
	return len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func object(raw []byte, path string) (rawObject, error) {
	var value rawObject
	if !utf8.Valid(raw) || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, invalid(path, "must be a JSON object")
	}
	return value, nil
}

func array(raw json.RawMessage, path string) ([]json.RawMessage, error) {
	var value []json.RawMessage
	if absent(raw) || json.Unmarshal(raw, &value) != nil {
		return nil, invalid(path, "must be an array")
	}
	return value, nil
}

func allowOnly(value rawObject, path string, fields ...string) error {
	var unknown []string
	for field := range value {
		if !slices.Contains(fields, field) {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) != 0 {
		slices.Sort(unknown)
		return unsupported(path + "." + unknown[0])
	}
	return nil
}

func text(raw json.RawMessage, path string) (string, error) {
	var value string
	if absent(raw) || json.Unmarshal(raw, &value) != nil {
		return "", invalid(path, "must be a string")
	}
	return value, nil
}

func optionalText(raw json.RawMessage, path string) (string, error) {
	if absent(raw) {
		return "", nil
	}
	return text(raw, path)
}

func identity(raw json.RawMessage, path string) (string, error) {
	value, err := text(raw, path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", invalid(path, "must not be empty")
	}
	return value, nil
}

func boolean(raw json.RawMessage, path string) (bool, error) {
	var value bool
	if absent(raw) || json.Unmarshal(raw, &value) != nil {
		return false, invalid(path, "must be a boolean")
	}
	return value, nil
}

func nonnegativeInt(raw json.RawMessage, path string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil || value < 0 {
		return 0, invalid(path, "must be a nonnegative integer")
	}
	return value, nil
}

// Cache hints and attribution metadata have no transcript semantics. Validate
// their container, but do not claim to reproduce their caching/billing effects.
func hint(raw json.RawMessage, path string) error {
	if absent(raw) {
		return nil
	}
	_, err := object(raw, path)
	return err
}
