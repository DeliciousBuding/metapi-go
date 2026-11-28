package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func importedWriteError(w http.ResponseWriter, err error) {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key") || strings.Contains(message, "database is locked") || strings.Contains(message, "serialization") {
		writeError(w, 409, "Imported upstream configuration conflicts with another update")
		return
	}
	importedReadError(w, err)
}

func (h *importedUpstreamHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input map[string]json.RawMessage
	if decodeImportedJSON(r, &input, "providerBinding") != nil {
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
	if (input["pluginBindings"] != nil || input["providerBinding"] != nil) && input["requestTransform"] != nil {
		writeError(w, 400, "Choose plugin binding patch or whole request transform replacement")
		return
	}
	previousCustomHeaders := row.CustomHeaders
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
		"requestTransform": {"request_config", &row.RequestConfig},
		"pluginBindings":   {"request_config", &row.RequestConfig.Plugins},
		"providerBinding":  {"request_config", &row.RequestConfig.Provider},
		"rateLimit":        {"rate_limit", &row.RateLimit},
		"streamPolicy":     {"stream_policy", &row.StreamPolicy},
	}
	set := []string{}
	args := []any{}
	setColumns := map[string]bool{}
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
		if name == "requestTransform" {
			row.RequestConfig = store.DirectRequestConfig{}
		}
		if decoder.Decode(f.target) != nil {
			writeError(w, 400, "Invalid configuration field type")
			return
		}
		if !setColumns[f.column] {
			setColumns[f.column] = true
			set = append(set, f.column+"=?")
			if name == "pluginBindings" || name == "providerBinding" {
				args = append(args, &row.RequestConfig)
			} else {
				args = append(args, f.target)
			}
		}
	}
	if _, ok := input["customHeaders"]; ok {
		// The editor round-trips masked values it read from request-config.
		// Treat an unchanged mask as "keep the stored value".
		row.CustomHeaders = mergeMaskedImportedCustomHeaders(row.CustomHeaders, previousCustomHeaders)
	}
	if err = validateImportedConfig(row); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if input["pluginBindings"] != nil || input["providerBinding"] != nil || input["requestTransform"] != nil || input["endpointConfig"] != nil {
		if err = channelplugins.ValidateBindings(r.Context(), tx, row.RequestConfig, row.Endpoints); err != nil {
			writeError(w, 400, "Invalid channel plugin binding")
			return
		}
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
			available = row.Endpoints.ProtocolMask()
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
		var anonymousMasks []int
		if err = tx.SelectContext(r.Context(), &anonymousMasks, tx.Rebind(`SELECT g.protocols FROM upstream_grants g JOIN upstream_credentials k ON k.id=g.credential_id WHERE k.channel_id=? AND k.kind=?`), id, store.DirectCredentialNone); err != nil {
			importedReadError(w, err)
			return
		}
		for _, mask := range anonymousMasks {
			if mask&row.Endpoints.AnonymousProtocolMask() != mask {
				writeError(w, 400, "Endpoints used by anonymous grants must retain anonymous authentication")
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
