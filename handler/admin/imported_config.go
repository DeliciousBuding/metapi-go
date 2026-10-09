package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

type importedChannelConfig struct {
	Ownership      string                `json:"ownership"`
	ID             int64                 `db:"id" json:"id"`
	OriginKey      string                `db:"origin_key" json:"originKey"`
	Provider       string                `db:"provider" json:"provider"`
	Dialect        string                `db:"dialect" json:"dialect"`
	Name           string                `db:"name" json:"name"`
	Enabled        bool                  `db:"enabled" json:"enabled"`
	BaseURL        string                `db:"base_url" json:"baseUrl"`
	Endpoints      store.DirectEndpoints `db:"endpoint_config" json:"endpointConfig"`
	ChatPath       string                `db:"openai_chat_completion_path" json:"openaiChatCompletionPath"`
	ResponsesPath  string                `db:"openai_response_path" json:"openaiResponsePath"`
	MessagesPath   string                `db:"anthropic_message_path" json:"anthropicMessagePath"`
	UseSystemProxy bool                  `db:"proxy" json:"useSystemProxy"`
	ChannelProxy   string                `db:"channel_proxy" json:"-"`
	CustomHeaders  string                `db:"custom_header" json:"-"`
	ParamOverride  string                `db:"param_override" json:"-"`
}

const importedConfigSelect = `SELECT id,origin_key,provider,dialect,name,enabled,base_url,endpoint_config,
openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override
FROM upstream_channels WHERE id=?`

func importedReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Imported upstream not found")
		return
	}
	writeError(w, 500, "Failed to access imported upstream storage")
}

func importedWriteError(w http.ResponseWriter, err error) {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") || strings.Contains(message, "database is locked") || strings.Contains(message, "serialization") {
		writeError(w, 409, "Imported upstream configuration conflicts with another update")
		return
	}
	importedReadError(w, err)
}

// All admin mutations reject null, ambiguous keys, unknown struct fields, and
// additional JSON values. Error text never reflects request contents.
func decodeImportedJSON(r *http.Request, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return fmt.Errorf("invalid request body")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return fmt.Errorf("invalid JSON object")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
		return fmt.Errorf("expected a non-empty JSON object")
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil || importedJSONHasNull(tree) {
		return fmt.Errorf("null is not supported")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request fields")
	}
	return nil
}

func importedJSONHasNull(value any) bool {
	if value == nil {
		return true
	}
	switch value := value.(type) {
	case map[string]any:
		for _, v := range value {
			if importedJSONHasNull(v) {
				return true
			}
		}
	case []any:
		for _, v := range value {
			if importedJSONHasNull(v) {
				return true
			}
		}
	}
	return false
}

func importedDisplayURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (h *importedUpstreamHandler) detail(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var row importedChannelConfig
	if err := h.db.GetContext(r.Context(), &row, h.db.Rebind(importedConfigSelect), id); err != nil {
		importedReadError(w, err)
		return
	}
	row.BaseURL = importedDisplayURL(row.BaseURL)
	row.Ownership = upstream.Ownership(row.OriginKey)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, struct {
		importedChannelConfig
		HasChannelProxy     bool   `json:"hasChannelProxy"`
		ChannelProxyDisplay string `json:"channelProxyDisplay"`
		HasCustomHeaders    bool   `json:"hasCustomHeaders"`
		HasParamOverride    bool   `json:"hasParamOverride"`
	}{row, row.ChannelProxy != "", importedDisplayURL(row.ChannelProxy), hasImportedConfig(row.CustomHeaders), hasImportedConfig(row.ParamOverride)})
}

func hasImportedConfig(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value != "{}" && value != "[]" && value != "null"
}

func (h *importedUpstreamHandler) requestConfig(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var row importedChannelConfig
	if err := h.db.GetContext(r.Context(), &row, h.db.Rebind(importedConfigSelect), id); err != nil {
		importedReadError(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"channelProxy": row.ChannelProxy, "customHeaders": row.CustomHeaders, "paramOverride": row.ParamOverride})
}

func (h *importedUpstreamHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input map[string]json.RawMessage
	if decodeImportedJSON(r, &input) != nil {
		writeError(w, 400, "Expected one non-empty configuration object")
		return
	}
	tx, err := h.db.BeginTxx(r.Context(), nil)
	if err != nil {
		importedWriteError(w, err)
		return
	}
	defer tx.Rollback()
	if err = upstream.LockLifecycleTx(r.Context(), tx); err != nil {
		importedWriteError(w, err)
		return
	}
	query := importedConfigSelect
	if h.db.DriverName() == "pgx" {
		query += " FOR UPDATE"
	}
	var row importedChannelConfig
	if err = tx.GetContext(r.Context(), &row, tx.Rebind(query), id); err != nil {
		importedReadError(w, err)
		return
	}
	previousEndpoints := row.Endpoints
	type field struct {
		column string
		target any
	}
	fields := map[string]field{
		"name": {"name", &row.Name}, "enabled": {"enabled", &row.Enabled}, "baseUrl": {"base_url", &row.BaseURL},
		"endpointConfig": {"endpoint_config", &row.Endpoints}, "openaiChatCompletionPath": {"openai_chat_completion_path", &row.ChatPath},
		"openaiResponsePath": {"openai_response_path", &row.ResponsesPath}, "anthropicMessagePath": {"anthropic_message_path", &row.MessagesPath},
		"useSystemProxy": {"proxy", &row.UseSystemProxy}, "channelProxy": {"channel_proxy", &row.ChannelProxy},
		"customHeaders": {"custom_header", &row.CustomHeaders}, "paramOverride": {"param_override", &row.ParamOverride},
	}
	set := []string{}
	args := []any{}
	for name, raw := range input {
		f, known := fields[name]
		if !known {
			writeError(w, 400, "Unknown or read-only configuration field")
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if name == "endpointConfig" {
			row.Endpoints = store.DirectEndpoints{}
		}
		if decoder.Decode(f.target) != nil {
			writeError(w, 400, "Invalid configuration field type")
			return
		}
		set = append(set, f.column+"=?")
		args = append(args, f.target)
	}
	if err = validateImportedConfig(row); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	_, endpointsChanged := input["endpointConfig"]
	_, chatChanged := input["openaiChatCompletionPath"]
	_, responsesChanged := input["openaiResponsePath"]
	_, messagesChanged := input["anthropicMessagePath"]
	if endpointsChanged || chatChanged || responsesChanged || messagesChanged {
		for _, pair := range [][2]*store.DirectEndpoint{{previousEndpoints.Responses, row.Endpoints.Responses}, {previousEndpoints.Messages, row.Endpoints.Messages}} {
			if pair[0] != nil && pair[0].Profile != "" && (pair[1] == nil || pair[1].Profile != pair[0].Profile) {
				writeError(w, 400, "A provider wire profile cannot be removed")
				return
			}
		}
		if previousEndpoints.IsConfigured() && !row.Endpoints.IsConfigured() {
			writeError(w, 400, "Configured endpoints cannot fall back to legacy routing")
			return
		}
		var masks []int
		if err = tx.SelectContext(r.Context(), &masks, tx.Rebind(`SELECT protocols FROM upstream_grants g JOIN upstream_models m ON m.id=g.model_id WHERE m.channel_id=?`), id); err != nil {
			importedReadError(w, err)
			return
		}
		available := 0
		if row.Endpoints.IsConfigured() {
			for bit, ep := range map[int]*store.DirectEndpoint{2: row.Endpoints.Chat, 4: row.Endpoints.Responses, 8: row.Endpoints.Messages, 16: row.Endpoints.Gemini} {
				if ep != nil {
					available |= bit
				}
			}
		} else {
			if row.ChatPath != "" {
				available |= 2
			}
			if row.ResponsesPath != "" {
				available |= 4
			}
			if row.MessagesPath != "" {
				available |= 8
			}
		}
		for _, mask := range masks {
			if mask&available != mask {
				writeError(w, 400, "Endpoints still used by a grant cannot be removed")
				return
			}
		}
	}
	args = append(args, id)
	if _, err = tx.ExecContext(r.Context(), tx.Rebind(`UPDATE upstream_channels SET `+strings.Join(set, ",")+` WHERE id=?`), args...); err != nil {
		importedWriteError(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		importedWriteError(w, err)
		return
	}
	routing.InvalidateCache()
	writeJSON(w, 200, map[string]any{"success": true, "id": id, "enabled": row.Enabled})
}

func validateImportedConfig(row importedChannelConfig) error {
	if strings.TrimSpace(row.Name) == "" || len(row.Name) > 200 {
		return fmt.Errorf("Name must contain 1 to 200 characters")
	}
	if !validImportedURL(row.BaseURL, false) {
		return fmt.Errorf("Invalid base URL")
	}
	for _, path := range []string{row.ChatPath, row.ResponsesPath, row.MessagesPath} {
		if path != "" && (!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#\\\r\n") || proxy.ContainsPathTraversal(path)) {
			return fmt.Errorf("Invalid legacy endpoint path")
		}
		decoded, err := url.PathUnescape(path)
		if err != nil || proxy.ContainsPathTraversal(decoded) {
			return fmt.Errorf("Invalid legacy endpoint path")
		}
	}
	raw, _ := row.Endpoints.Value()
	var checked store.DirectEndpoints
	if checked.Scan(raw) != nil {
		return fmt.Errorf("Invalid endpoint URL or authentication")
	}
	for _, ep := range []*store.DirectEndpoint{row.Endpoints.Chat, row.Endpoints.Responses, row.Endpoints.Messages, row.Endpoints.Gemini} {
		if ep == nil {
			continue
		}
		if !validImportedURL(ep.URL, false) {
			return fmt.Errorf("Invalid endpoint URL")
		}
		if ep.Profile == "codex" && row.Provider != "codex" && row.Provider != "fenno" || ep.Profile == "claudecode" && row.Provider != "claudecode" {
			return fmt.Errorf("Endpoint profile does not match the provider and protocol")
		}
	}
	if row.ChannelProxy != "" && !validImportedURL(row.ChannelProxy, true) {
		return fmt.Errorf("Invalid channel proxy URL")
	}
	if row.CustomHeaders != "" {
		if rejectDuplicateJSONKeys([]byte(row.CustomHeaders)) != nil {
			return fmt.Errorf("Invalid custom headers")
		}
		var headers []struct {
			Key   string `json:"header_key"`
			Value string `json:"header_value"`
		}
		decoder := json.NewDecoder(strings.NewReader(row.CustomHeaders))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&headers) != nil || headers == nil {
			return fmt.Errorf("Custom headers must be a JSON array")
		}
		seen := map[string]bool{}
		for _, header := range headers {
			key := strings.ToLower(header.Key)
			if key == "" || seen[key] || platform.IsDeniedCustomHeader(key) || strings.ContainsAny(header.Value, "\r\n") {
				return fmt.Errorf("Unsupported custom header")
			}
			for _, char := range header.Key {
				if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
					return fmt.Errorf("Invalid custom header name")
				}
			}
			if _, ok := httpclient.ExpandClientHeaderTemplate(header.Value, nil); !ok {
				return fmt.Errorf("Unsupported client header template")
			}
			seen[key] = true
		}
	}
	if row.ParamOverride != "" {
		var obj map[string]json.RawMessage
		if rejectDuplicateJSONKeys([]byte(row.ParamOverride)) != nil || json.Unmarshal([]byte(row.ParamOverride), &obj) != nil || obj == nil {
			return fmt.Errorf("Parameter override must be a JSON object")
		}
	}
	return nil
}

func validImportedURL(raw string, isProxy bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.RawQuery != "" || service.IsForbiddenSiteTargetURL(raw) {
		return false
	}
	if !isProxy && u.User != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || isProxy && (u.Scheme == "socks5" || u.Scheme == "socks5h")
}
