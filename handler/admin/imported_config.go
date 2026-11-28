package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

type importedChannelConfig struct {
	Ownership      string                    `json:"ownership"`
	ID             int64                     `db:"id" json:"id"`
	OriginKey      string                    `db:"origin_key" json:"originKey"`
	Provider       string                    `db:"provider" json:"provider"`
	Dialect        string                    `db:"dialect" json:"dialect"`
	Name           string                    `db:"name" json:"name"`
	Enabled        bool                      `db:"enabled" json:"enabled"`
	BaseURL        string                    `db:"base_url" json:"baseUrl"`
	Endpoints      store.DirectEndpoints     `db:"endpoint_config" json:"endpointConfig"`
	ChatPath       string                    `db:"openai_chat_completion_path" json:"openaiChatCompletionPath"`
	ResponsesPath  string                    `db:"openai_response_path" json:"openaiResponsePath"`
	MessagesPath   string                    `db:"anthropic_message_path" json:"anthropicMessagePath"`
	UseSystemProxy bool                      `db:"proxy" json:"useSystemProxy"`
	ChannelProxy   string                    `db:"channel_proxy" json:"-"`
	CustomHeaders  string                    `db:"custom_header" json:"-"`
	ParamOverride  string                    `db:"param_override" json:"-"`
	RequestConfig  store.DirectRequestConfig `db:"request_config" json:"-"`
	RateLimit      store.DirectRateLimit     `db:"rate_limit" json:"rateLimit"`
	StreamPolicy   string                    `db:"stream_policy" json:"streamPolicy"`
}

// maskedImportedHeaderValue is returned in place of every custom header value
// on the request-config read. Authorization / X-Api-Key / Cookie and any other
// operator-supplied value must not be echoed back, regardless of header name.
const maskedImportedHeaderValue = "****"

type importedCustomHeader struct {
	Key   string `json:"header_key"`
	Value string `json:"header_value"`
}

// maskImportedCustomHeaders keeps header names (so the editor still knows the
// shape) and replaces every non-empty value with a fixed mask. Malformed/non-
// array stored values are withheld entirely rather than echoed.
func maskImportedCustomHeaders(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "null" {
		return raw
	}
	var headers []importedCustomHeader
	if err := json.Unmarshal([]byte(trimmed), &headers); err != nil {
		return ""
	}
	masked := make([]importedCustomHeader, len(headers))
	for i, header := range headers {
		value := header.Value
		if strings.TrimSpace(value) != "" {
			value = maskedImportedHeaderValue
		}
		masked[i] = importedCustomHeader{Key: header.Key, Value: value}
	}
	encoded, err := json.Marshal(masked)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// mergeMaskedImportedCustomHeaders treats the mask as "keep the stored value"
// when the editor submits a previously read header unchanged. Without this, a
// save that only edits one row would overwrite every other secret with "****".
func mergeMaskedImportedCustomHeaders(next, previous string) string {
	if !strings.Contains(next, maskedImportedHeaderValue) || strings.TrimSpace(previous) == "" {
		return next
	}
	var nextRows []importedCustomHeader
	if json.Unmarshal([]byte(next), &nextRows) != nil {
		return next
	}
	var previousRows []importedCustomHeader
	if json.Unmarshal([]byte(previous), &previousRows) != nil {
		return next
	}
	previousByKey := make(map[string]string, len(previousRows))
	for _, row := range previousRows {
		previousByKey[strings.ToLower(strings.TrimSpace(row.Key))] = row.Value
	}
	for i := range nextRows {
		if nextRows[i].Value != maskedImportedHeaderValue {
			continue
		}
		if stored, ok := previousByKey[strings.ToLower(strings.TrimSpace(nextRows[i].Key))]; ok {
			nextRows[i].Value = stored
		}
	}
	encoded, err := json.Marshal(nextRows)
	if err != nil {
		return next
	}
	return string(encoded)
}

const importedConfigSelect = `SELECT id,origin_key,provider,dialect,name,enabled,base_url,endpoint_config,
openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,request_config,rate_limit,stream_policy
FROM upstream_channels WHERE id=?`

func importedReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, 404, "Imported upstream not found")
		return
	}
	writeError(w, 500, "Failed to access imported upstream storage")
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
	row.BaseURL = upstream.DisplayURL(row.BaseURL)
	row.Ownership = upstream.Ownership(row.OriginKey)
	setNoStore(w)
	writeJSON(w, 200, struct {
		importedChannelConfig
		HasChannelProxy     bool   `json:"hasChannelProxy"`
		ChannelProxyDisplay string `json:"channelProxyDisplay"`
		HasCustomHeaders    bool   `json:"hasCustomHeaders"`
		HasParamOverride    bool   `json:"hasParamOverride"`
		HasRequestTransform bool   `json:"hasRequestTransform"`
	}{row, row.ChannelProxy != "", upstream.DisplayURL(row.ChannelProxy), hasImportedConfig(row.CustomHeaders), hasImportedConfig(row.ParamOverride), row.RequestConfig.Active()})
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
	w.Header().Set("Cache-Control", noStoreCacheControl)
	var row importedChannelConfig
	if err := h.db.GetContext(r.Context(), &row, h.db.Rebind(importedConfigSelect), id); err != nil {
		importedReadError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"channelProxy": row.ChannelProxy,
		// Header names stay editable; values are masked so Authorization,
		// X-Api-Key, cookies, and any other operator secret are not echoed.
		"customHeaders":   maskImportedCustomHeaders(row.CustomHeaders),
		"paramOverride":   row.ParamOverride,
		"pluginBindings":  row.RequestConfig.Plugins,
		"providerBinding": row.RequestConfig.Provider,
		// Operation values may include private headers or prompts. Readback
		// exposes shape/counts only; edits replace the whole explicit contract.
		"requestTransformSummary": map[string]any{
			"bodyOperations":      len(row.RequestConfig.Body),
			"headerOperations":    len(row.RequestConfig.Headers),
			"hasTransformOptions": row.RequestConfig.Options.Active(),
		},
	})
}
