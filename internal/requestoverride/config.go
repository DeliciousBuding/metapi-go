// Package requestoverride owns the pure, ordered request operations imported
// from AxonHub. It does not select routes, obtain credentials or perform I/O.
package requestoverride

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"unicode"
)

type Match struct {
	Path string `json:"path"`
	Eq   string `json:"eq"`
}

type Operation struct {
	Op        string `json:"op"`
	Path      string `json:"path,omitempty"`
	From      string `json:"from,omitempty"`
	To        string `json:"to,omitempty"`
	Value     string `json:"value,omitempty"`
	Condition string `json:"condition,omitempty"`
	Match     *Match `json:"match,omitempty"`
	Index     *int   `json:"index,omitempty"`
	Splat     *bool  `json:"splat,omitempty"`
}

type EffortMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type TransformOptions struct {
	ForceArrayInstructions         bool            `json:"forceArrayInstructions,omitempty"`
	ForceArrayInputs               bool            `json:"forceArrayInputs,omitempty"`
	ReplaceDeveloperRoleWithSystem bool            `json:"replaceDeveloperRoleWithSystem,omitempty"`
	ReasoningEffortMapping         []EffortMapping `json:"reasoningEffortMapping,omitempty"`
}

func (o TransformOptions) Active() bool {
	return o.ForceArrayInstructions || o.ForceArrayInputs || o.ReplaceDeveloperRoleWithSystem || len(o.ReasoningEffortMapping) > 0
}

type Config struct {
	Body    []Operation      `json:"body,omitempty"`
	Headers []Operation      `json:"headers,omitempty"`
	Options TransformOptions `json:"options,omitempty"`
	// NativeChatMapping identifies source channels whose primary OpenAI Chat
	// serializer consumes the mapping for every client. Other serializers
	// consume it only through the source's Claude CLI compatibility fallback.
	NativeChatMapping bool `json:"nativeChatMapping,omitempty"`
}

func (c Config) Active() bool {
	return len(c.Body) > 0 || len(c.Headers) > 0 || c.Options.Active()
}

// Decode rejects unknown fields, including fields inside operations/match/options.
// Import previews and persisted configuration use the same shape.
func Decode(raw []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request transform shape")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid request transform document")
	}
	return nil
}

// Validate returns a stable reason without quoting operator strings or secrets.
func (c Config) Validate() error {
	if len(c.Body)+len(c.Headers) > 128 {
		return fmt.Errorf("too_many_request_operations")
	}
	for _, group := range []struct {
		ops    []Operation
		header bool
	}{{c.Body, false}, {c.Headers, true}} {
		for _, op := range group.ops {
			if err := validateOperation(op, group.header); err != nil {
				return err
			}
		}
	}
	for _, mapping := range c.Options.ReasoningEffortMapping {
		if strings.TrimSpace(mapping.From) == "" || strings.TrimSpace(mapping.To) == "" ||
			strings.IndexFunc(mapping.From+mapping.To, unicode.IsControl) >= 0 {
			return fmt.Errorf("reasoning_mapping_invalid")
		}
	}
	return nil
}

func validateOperation(op Operation, header bool) error {
	if len(op.Value)+len(op.Condition) > 64<<10 {
		return fmt.Errorf("request_operation_too_large")
	}
	switch op.Op {
	case "set", "set_if_absent", "delete", "array_append", "array_prepend", "array_insert", "array_remove":
		if header && op.Op != "set" && op.Op != "delete" {
			return fmt.Errorf("header_operation_unsupported")
		}
		if !validPath(op.Path, header, true) {
			return fmt.Errorf("operation_destination_unsupported")
		}
	case "rename", "copy":
		if !validPath(op.From, header, op.Op == "rename") || !validPath(op.To, header, true) {
			return fmt.Errorf("operation_path_unsupported")
		}
	default:
		return fmt.Errorf("operation_unsupported")
	}
	if op.Op == "array_insert" && op.Index == nil {
		return fmt.Errorf("array_index_required")
	}
	if op.Op == "array_remove" && (op.Match == nil || strings.TrimSpace(op.Match.Path) == "" || strings.IndexFunc(op.Match.Path, unicode.IsControl) >= 0 || strings.TrimSpace(op.Match.Eq) == "") {
		return fmt.Errorf("array_match_invalid")
	}
	for _, value := range []string{op.Value, op.Condition} {
		if strings.Contains(value, "{{") && strings.Contains(value, "}}") {
			if _, err := template.New("override").Funcs(templateFuncs).Parse(value); err != nil {
				return fmt.Errorf("operation_template_invalid")
			}
		}
	}
	if header && strings.ContainsAny(op.Value, "\r\n\x00") {
		return fmt.Errorf("header_value_invalid")
	}
	return nil
}

func validPath(path string, header, mutates bool) bool {
	if header {
		return validHeaderName(path) && !protectedHeader(path)
	}
	if !validJSONPath(path) {
		return false
	}
	if !mutates {
		return true
	}
	first, remaining := splitPathPart(path)
	switch strings.ToLower(first) {
	case "model", "stream", "stream_options", "max_tokens", "max_completion_tokens", "max_output_tokens":
		return false
	case "generationconfig", "generation_config":
		// Decode sjson's escaped keys before checking both the cap and its
		// descendants: setting a child would replace the numeric cap.
		field, _ := splitPathPart(remaining)
		return remaining != "" && !strings.EqualFold(field, "maxOutputTokens") && !strings.EqualFold(field, "max_output_tokens")
	default:
		return true
	}
}

// gjson/sjson share escaped dotted paths and numeric array indexes. Query,
// projection and modifier paths are deliberately not mutation destinations:
// they can traverse outside the declared root and cannot preserve grant bounds.
func validJSONPath(path string) bool {
	if path == "" || strings.IndexFunc(path, unicode.IsControl) >= 0 {
		return false
	}
	escaped := false
	for _, char := range path {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' {
			escaped = true
			continue
		}
		if strings.ContainsRune("#@|*?[]():", char) {
			return false
		}
	}
	return !escaped && firstPathPart(path) != ""
}

func firstPathPart(path string) string {
	first, _ := splitPathPart(path)
	return first
}

func splitPathPart(path string) (string, string) {
	var result strings.Builder
	escaped := false
	for index, char := range path {
		if escaped {
			result.WriteRune(char)
			escaped = false
		} else if char == '\\' {
			escaped = true
		} else if char == '.' {
			return result.String(), path[index+1:]
		} else {
			result.WriteRune(char)
		}
	}
	return result.String(), ""
}

func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, c := range name {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
			strings.ContainsRune("!#$%&'*+-.^_`|~", c) {
			continue
		}
		return false
	}
	return true
}

// These headers belong to authentication, framing, transport or protocol
// identity. A channel operation must not replace or copy their values.
func protectedHeader(name string) bool {
	switch strings.ToLower(name) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key",
		"x-admin-confirm-token", "host", "content-length", "content-type", "accept-encoding",
		"transfer-encoding", "connection", "upgrade", "trailer", "te", "expect",
		"anthropic-version", "openai-beta", "chatgpt-account-id", "x-goog-user-project",
		"api-key", "x-google-api-key", "www-authenticate", "keep-alive", "trailers":
		return true
	}
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") || strings.HasPrefix(lower, "x-metapi-") || strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "api-key")
}

func sensitiveContextHeader(name string) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "api-key",
		"x-goog-api-key", "x-google-api-key", "www-authenticate", "x-admin-confirm-token":
		return true
	}
	return strings.Contains(lower, "token") || strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") || strings.Contains(lower, "api-key") ||
		strings.HasPrefix(lower, "x-metapi-")
}

// Context contains only the source's public template fields. No secret,
// environment, filesystem, function callback or authenticated-key object exists.
type Context struct {
	RequestModel    string
	Model           string
	Metadata        map[string]string
	RequestHeader   map[string]string
	PromptCacheKey  string
	ReasoningEffort string
}

func NewContext(body []byte, headers http.Header, requestedModel, model, protocol string) Context {
	ctx := Context{RequestModel: requestedModel, Model: model, RequestHeader: map[string]string{}}
	for name, values := range headers {
		if len(values) == 0 || sensitiveContextHeader(name) {
			continue
		}
		ctx.RequestHeader[http.CanonicalHeaderKey(name)] = values[0]
		ctx.RequestHeader[strings.ToLower(name)] = values[0]
	}
	var fields struct {
		Metadata        json.RawMessage `json:"metadata"`
		PromptCacheKey  string          `json:"prompt_cache_key"`
		ReasoningEffort string          `json:"reasoning_effort"`
		Reasoning       struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
		Thinking *struct {
			Type   string `json:"type"`
			Budget int64  `json:"budget_tokens"`
		} `json:"thinking"`
		OutputConfig struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
		GenerationConfig struct {
			ThinkingConfig *struct {
				ThinkingLevel  string `json:"thinkingLevel"`
				ThinkingBudget *int64 `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	if json.Unmarshal(body, &fields) == nil {
		_ = json.Unmarshal(fields.Metadata, &ctx.Metadata)
		ctx.PromptCacheKey, ctx.ReasoningEffort = fields.PromptCacheKey, fields.ReasoningEffort
		if ctx.ReasoningEffort == "" {
			ctx.ReasoningEffort = fields.Reasoning.Effort
		}
		if protocol == "messages" {
			var metadata struct {
				UserID string `json:"user_id"`
			}
			ctx.Metadata = map[string]string{}
			if json.Unmarshal(fields.Metadata, &metadata) == nil {
				ctx.Metadata["user_id"] = metadata.UserID
			}
			ctx.ReasoningEffort = ""
			if fields.Thinking != nil {
				switch fields.Thinking.Type {
				case "enabled":
					ctx.ReasoningEffort = "high"
					if fields.Thinking.Budget <= 5000 {
						ctx.ReasoningEffort = "low"
					} else if fields.Thinking.Budget <= 15000 {
						ctx.ReasoningEffort = "medium"
					}
				case "disabled":
					ctx.ReasoningEffort = "none"
				case "adaptive":
					ctx.ReasoningEffort = "high"
				}
			}
			if fields.OutputConfig.Effort != "" {
				ctx.ReasoningEffort = fields.OutputConfig.Effort
				if ctx.ReasoningEffort == "max" {
					ctx.ReasoningEffort = "xhigh"
				}
			}
		} else if protocol == "gemini" {
			ctx.Metadata = nil
			ctx.PromptCacheKey = ""
			ctx.ReasoningEffort = ""
			if thinking := fields.GenerationConfig.ThinkingConfig; thinking != nil {
				ctx.ReasoningEffort = "medium"
				if thinking.ThinkingLevel != "" {
					ctx.ReasoningEffort = strings.ToLower(thinking.ThinkingLevel)
					if ctx.ReasoningEffort == "minimal" {
						ctx.ReasoningEffort = "low"
					}
				} else if thinking.ThinkingBudget != nil {
					ctx.ReasoningEffort = "high"
					if *thinking.ThinkingBudget <= 1024 {
						ctx.ReasoningEffort = "low"
					} else if *thinking.ThinkingBudget <= 8192 {
						ctx.ReasoningEffort = "medium"
					}
				}
			}
		}
	}
	return ctx
}
