package requestoverride

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

var templateFuncs = template.FuncMap{
	"toJSON": func(value any) (string, error) {
		data, err := json.Marshal(value)
		return string(data), err
	},
}

type Warning struct {
	Index int
	Code  string
}

// Rendering errors retain the original literal, matching the source's defined
// best-effort behavior. Warnings never contain the template, input or result.
func render(value string, ctx Context) (string, bool) {
	if !strings.Contains(value, "{{") || !strings.Contains(value, "}}") {
		return value, true
	}
	tmpl, err := template.New("override").Funcs(templateFuncs).Parse(value)
	if err != nil {
		return value, false
	}
	buf := boundedBuffer{limit: 1 << 20}
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return value, false
	}
	return buf.String(), true
}

type boundedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, fmt.Errorf("rendered operation exceeds limit")
	}
	return b.Buffer.Write(p)
}

func (b *boundedBuffer) WriteString(value string) (int, error) {
	if len(value) > b.limit-b.Len() {
		return 0, fmt.Errorf("rendered operation exceeds limit")
	}
	return b.Buffer.WriteString(value)
}

func renderValue(value string, ctx Context) (any, bool) {
	rendered, ok := render(value, ctx)
	trimmed := strings.TrimSpace(rendered)
	if trimmed != "" && (strings.ContainsRune("{[-0123456789", rune(trimmed[0])) || trimmed == "true" || trimmed == "false" || trimmed == "null") {
		var scalar any
		decoder := json.NewDecoder(strings.NewReader(trimmed))
		decoder.UseNumber()
		if decoder.Decode(&scalar) == nil && json.Valid([]byte(trimmed)) {
			return scalar, ok
		}
	}
	return rendered, ok
}

func condition(op Operation, ctx Context) (bool, bool) {
	if op.Condition == "" {
		return true, true
	}
	text, ok := render(op.Condition, ctx)
	return strings.TrimSpace(text) == "true", ok
}

// ApplyBody never mutates its input. An individual path/array error leaves the
// previous result intact and later operations continue, as in the source.
func ApplyBody(body []byte, ops []Operation, ctx Context, maxBytes int) ([]byte, []Warning, error) {
	if len(ops) == 0 {
		return body, nil, nil
	}
	if !json.Valid(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, nil, fmt.Errorf("request operations require a JSON object")
	}
	if err := (Config{Body: ops}).Validate(); err != nil {
		return nil, nil, err
	}
	out := append([]byte(nil), body...)
	var warnings []Warning
	for index, op := range ops {
		enabled, conditionOK := condition(op, ctx)
		if !conditionOK {
			warnings = append(warnings, Warning{index, "condition_render_failed"})
		}
		if !enabled {
			continue
		}
		next, renderOK, err := applyBodyOperation(out, op, ctx)
		if !renderOK {
			warnings = append(warnings, Warning{index, "value_render_failed"})
		}
		if err != nil {
			warnings = append(warnings, Warning{index, "body_operation_failed"})
			continue
		}
		if maxBytes > 0 && len(next) > maxBytes {
			return nil, warnings, fmt.Errorf("transformed request exceeds body limit")
		}
		out = next
	}
	return out, warnings, nil
}

func applyBodyOperation(body []byte, op Operation, ctx Context) ([]byte, bool, error) {
	switch op.Op {
	case "set", "set_if_absent":
		existing := gjson.GetBytes(body, op.Path)
		if op.Op == "set_if_absent" && (existing.Exists() || existing.Raw == "null") {
			return body, true, nil
		}
		value, ok := renderValue(op.Value, ctx)
		if str, isString := value.(string); isString && str == "__AXONHUB_CLEAR__" {
			out, err := sjson.DeleteBytes(body, op.Path)
			return out, ok, err
		}
		out, err := sjson.SetBytes(body, op.Path, value)
		return out, ok, err
	case "delete":
		out, err := sjson.DeleteBytes(body, op.Path)
		return out, true, err
	case "rename", "copy":
		value := gjson.GetBytes(body, op.From)
		if !value.Exists() {
			return body, true, nil
		}
		next := body
		var err error
		if op.Op == "rename" {
			next, err = sjson.DeleteBytes(next, op.From)
			if err != nil {
				return body, true, err
			}
		}
		next, err = sjson.SetRawBytes(next, op.To, []byte(value.Raw))
		return next, true, err
	case "array_remove":
		existing := gjson.GetBytes(body, op.Path)
		if !existing.Exists() {
			return body, true, nil
		}
		if !existing.IsArray() {
			return body, true, fmt.Errorf("array target has a different type")
		}
		kept := []json.RawMessage{}
		for _, item := range existing.Array() {
			value := gjson.Get(item.Raw, op.Match.Path)
			if !value.Exists() || value.String() != strings.TrimSpace(op.Match.Eq) {
				kept = append(kept, json.RawMessage(item.Raw))
			}
		}
		out, err := sjson.SetBytes(body, op.Path, kept)
		return out, true, err
	default:
		value, ok := renderValue(op.Value, ctx)
		insert := []any{value}
		if values, isArray := value.([]any); isArray && (op.Splat == nil || *op.Splat) {
			insert = values
		}
		existing := gjson.GetBytes(body, op.Path)
		var current []json.RawMessage
		if existing.Exists() {
			if !existing.IsArray() {
				return body, ok, fmt.Errorf("array target has a different type")
			}
			if err := json.Unmarshal([]byte(existing.Raw), &current); err != nil {
				return body, ok, err
			}
		}
		pos := len(current)
		if op.Op == "array_prepend" {
			pos = 0
		} else if op.Op == "array_insert" {
			pos = *op.Index
			if pos < 0 {
				pos += len(current)
			}
			pos = max(0, min(pos, len(current)))
		}
		result := make([]json.RawMessage, 0, len(current)+len(insert))
		result = append(result, current[:pos]...)
		for _, value := range insert {
			raw, err := json.Marshal(value)
			if err != nil {
				return body, ok, err
			}
			result = append(result, raw)
		}
		result = append(result, current[pos:]...)
		out, err := sjson.SetBytes(body, op.Path, result)
		return out, ok, err
	}
}

// ApplyHeaders clones the final outbound headers. The validation forbids both
// copying and changing credential or transport headers. Rendered CR/LF is an
// explicit request failure, never passed to a transport as a partial success.
func ApplyHeaders(headers http.Header, ops []Operation, ctx Context) (http.Header, []Warning, error) {
	if err := (Config{Headers: ops}).Validate(); err != nil {
		return nil, nil, err
	}
	out := headers.Clone()
	if out == nil {
		out = make(http.Header)
	}
	var warnings []Warning
	for index, op := range ops {
		enabled, ok := condition(op, ctx)
		if !ok {
			warnings = append(warnings, Warning{index, "condition_render_failed"})
		}
		if !enabled {
			continue
		}
		switch op.Op {
		case "set":
			value, ok := render(op.Value, ctx)
			if !ok {
				warnings = append(warnings, Warning{index, "value_render_failed"})
			}
			if strings.ContainsAny(value, "\r\n\x00") {
				return nil, warnings, fmt.Errorf("rendered request header is invalid")
			}
			if value == "__AXONHUB_CLEAR__" {
				out.Del(op.Path)
			} else {
				out.Set(op.Path, value)
			}
		case "delete":
			out.Del(op.Path)
		case "copy", "rename":
			values := append([]string(nil), out.Values(op.From)...)
			if op.Op == "rename" {
				out.Del(op.From)
			}
			for _, value := range values {
				out.Add(op.To, value)
			}
		}
	}
	return out, warnings, nil
}
