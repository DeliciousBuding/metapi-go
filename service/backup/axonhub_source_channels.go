package backup

import (
	"bytes"
	"encoding/json"
)

var axonHubChannelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "type", "base_url", "name", "status",
	"credentials", "disabled_api_keys", "supported_models", "manual_models",
	"auto_sync_supported_models", "auto_sync_model_pattern", "tags", "default_test_model",
	"policies", "settings", "ordering_weight", "error_message", "auto_disabled_at",
	"auto_disable_expires_at", "remark", "endpoints", "edges",
}

var axonHubCredentialFields = []string{"apiKey", "apiKeys", "oauth", "azure", "gcp", "managementApiKey"}

var axonHubEndpointFields = []string{"api_format", "path", "base_url", "transport"}

func decodeAxonHubChannels(raw json.RawMessage) ([]AxonHubSourceChannel, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: channels must be an array")
	}
	out := make([]AxonHubSourceChannel, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d] is not an object", i)
		}
		extra := unknownKeys(obj, axonHubChannelFields)
		var ch AxonHubSourceChannel
		if err := json.Unmarshal(row, &ch); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d] has an invalid identity or field type", i)
		}
		if len(extra) > 0 {
			ch.UnsupportedFields = extra
		}
		if err := decodeAxonHubCredentials(obj["credentials"], &ch.Credentials); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d].credentials is invalid", i)
		}
		if err := decodeAxonHubEndpoints(obj["endpoints"], &ch.Endpoints); err != nil {
			return nil, err
		}
		if err := decodeAxonHubSettings(obj["settings"], &ch.Settings); err != nil {
			return nil, err
		}
		if err := decodeAxonHubPolicies(obj["policies"], &ch.Policies); err != nil {
			return nil, err
		}
		if err := decodeAxonHubDisabledKeys(obj["disabled_api_keys"], &ch.DisabledAPIKeys); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d].disabled_api_keys is invalid", i)
		}
		out = append(out, ch)
	}
	return out, nil
}

func decodeAxonHubCredentials(raw json.RawMessage, dst *AxonHubSourceCredentials) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return err
	}
	if dst.APIKey, err = decodeAxonHubString(obj["apiKey"], "credentials.apiKey"); err != nil {
		return err
	}
	if rawKeys, ok := obj["apiKeys"]; ok {
		if err := json.Unmarshal(rawKeys, &dst.APIKeys); err != nil {
			return axonHubErr("credentials.apiKeys must be an array")
		}
	}
	if err := decodeAxonHubOAuthCredentials(obj["oauth"], dst); err != nil {
		return err
	}
	dst.Azure = present(obj["azure"])
	dst.HasManagementKey = present(obj["managementApiKey"])
	if rawGCP, ok := obj["gcp"]; ok && present(rawGCP) {
		var gcp AxonHubSourceGCP
		if err := json.Unmarshal(rawGCP, &gcp); err != nil {
			return axonHubErr("credentials.gcp must be an object")
		}
		dst.GCP = &gcp
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubCredentialFields)
	return nil
}

func decodeAxonHubEndpoints(raw json.RawMessage, dst *[]AxonHubSourceEndpoint) error {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: endpoints must be an array")
	}
	out := make([]AxonHubSourceEndpoint, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] is not an object", i)
		}
		if extra := unknownKeys(obj, axonHubEndpointFields); len(extra) > 0 {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] has unsupported fields", i)
		}
		var ep AxonHubSourceEndpoint
		if err := json.Unmarshal(row, &ep); err != nil {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] has an invalid field type", i)
		}
		out = append(out, ep)
	}
	*dst = out
	return nil
}

func decodeAxonHubDisabledKeys(raw json.RawMessage, dst *[]AxonHubSourceDisabledKey) error {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return err
	}
	out := make([]AxonHubSourceDisabledKey, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return axonHubErr("disabled_api_keys[%d] is not an object", i)
		}
		key, err := decodeAxonHubString(obj["key"], "disabled_api_keys.key")
		if err != nil {
			return err
		}
		out = append(out, AxonHubSourceDisabledKey{Key: key})
	}
	*dst = out
	return nil
}
