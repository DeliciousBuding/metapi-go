package backup

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/store"
)

var axonHubChannelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "type", "base_url", "name", "status",
	"credentials", "disabled_api_keys", "supported_models", "manual_models",
	"auto_sync_supported_models", "auto_sync_model_pattern", "tags", "default_test_model",
	"policies", "settings", "ordering_weight", "error_message", "auto_disabled_at",
	"auto_disable_expires_at", "remark", "endpoints", "edges",
}

var axonHubChannelSettingsFields = []string{
	"extraModelPrefix", "autoTrimedModelPrefixes", "modelMappings", "hideOriginalModels",
	"hideMappedModels", "lowercaseModelId", "overrideParameters", "bodyOverrideOperations",
	"overrideHeaders", "headerOverrideOperations", "proxy", "transformOptions",
	"passThroughUserAgent", "passThroughBody", "rateLimit", "retryableStatusCodes",
	"retryableErrorPatterns", "providerQuota", "modelProtocols",
}

var axonHubChannelPolicyFields = []string{"stream", "apiKeyAutoDisableRules"}

var axonHubCredentialFields = []string{"apiKey", "apiKeys", "oauth", "azure", "gcp", "managementApiKey"}

var axonHubEndpointFields = []string{"api_format", "path", "base_url", "transport"}

var axonHubModelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "developer", "model_id", "type", "name",
	"icon", "group", "model_card", "status", "settings", "remark", "edges",
}

var axonHubModelSettingsFields = []string{
	"disableDeveloperSettingsInheritance", "associations", "loadBalancerStrategy", "traceStickyMode",
}

var axonHubAssociationFields = []string{
	"type", "priority", "disabled", "when", "channelModel", "channelRegex", "regex", "modelId",
	"channelTagsModel", "channelTagsRegex",
}

var axonHubExcludeFields = []string{"channelNamePattern", "channelIds", "channelTags"}

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

func decodeAxonHubSettings(raw json.RawMessage, dst *AxonHubSourceChannelSettings) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: channel settings must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubChannelSettingsFields)
	if rawProtocols, present := obj["modelProtocols"]; present {
		var rows []json.RawMessage
		if err := json.Unmarshal(rawProtocols, &rows); err != nil {
			return axonHubErr("invalid AxonHub modelProtocols")
		}
		for _, row := range rows {
			value, err := rowObject(row)
			if err != nil {
				return axonHubErr("invalid AxonHub model protocol entry")
			}
			for _, field := range unknownKeys(value, []string{"model", "apiFormats", "enabled"}) {
				dst.UnsupportedFields = append(dst.UnsupportedFields, "modelProtocols."+field)
			}
		}
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: channel settings has an invalid field type")
	}
	// The source gives a non-nil operations slice precedence over the legacy
	// settings, including an explicit empty array that clears the old override.
	var bodyOps, headerOps []requestoverride.Operation
	if rawOps, ok := obj["bodyOverrideOperations"]; ok {
		if err := json.Unmarshal(rawOps, &bodyOps); err != nil {
			return axonHubErr("invalid AxonHub backup: body override operations must be an array")
		}
		if requestoverride.Decode(rawOps, &bodyOps) != nil {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "bodyOverrideOperations")
		}
	}
	if rawOps, ok := obj["headerOverrideOperations"]; ok {
		if err := json.Unmarshal(rawOps, &headerOps); err != nil {
			return axonHubErr("invalid AxonHub backup: header override operations must be an array")
		}
		if requestoverride.Decode(rawOps, &headerOps) != nil {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "headerOverrideOperations")
		}
	}
	if bodyOps != nil {
		dst.OverrideParameters = ""
	} else if strings.HasPrefix(strings.TrimSpace(dst.OverrideParameters), "[") {
		rawOps := []byte(dst.OverrideParameters)
		if json.Unmarshal(rawOps, &bodyOps) != nil {
			return axonHubErr("invalid AxonHub backup: legacy operation array is invalid")
		}
		if requestoverride.Decode(rawOps, &bodyOps) != nil {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "overrideParameters.operations")
		}
		dst.OverrideParameters = ""
	}
	if headerOps == nil && present(obj["overrideHeaders"]) {
		var headers []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if json.Unmarshal(obj["overrideHeaders"], &headers) != nil {
			return axonHubErr("invalid AxonHub backup: legacy override headers must be an array")
		}
		if requestoverride.Decode(obj["overrideHeaders"], &headers) != nil {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "overrideHeaders")
		}
		for _, header := range headers {
			headerOps = append(headerOps, requestoverride.Operation{Op: "set", Path: header.Key, Value: header.Value})
		}
	}
	dst.BodyOverrideOperations, dst.HeaderOverrideOperations = bodyOps, headerOps
	if rawOptions := obj["transformOptions"]; len(rawOptions) > 0 && string(bytes.TrimSpace(rawOptions)) != "null" {
		if json.Unmarshal(rawOptions, &dst.TransformOptions) != nil {
			return axonHubErr("invalid AxonHub backup: transform options have invalid fields")
		}
		if requestoverride.Decode(rawOptions, &dst.TransformOptions) != nil {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "transformOptions")
		}
	}
	dst.RateLimit, dst.HasUnsupportedRateLimit, err = decodeAxonHubRateLimit(obj["rateLimit"])
	if err != nil {
		return err
	}
	dst.HasRetryableErrorPatterns = present(obj["retryableErrorPatterns"])
	dst.HasProviderQuota = present(obj["providerQuota"])
	dst.ProxyConfigured = present(obj["proxy"])
	if dst.ProxyConfigured {
		var proxy struct {
			Type                   string `json:"type"`
			URL                    string `json:"url"`
			Username               string `json:"username"`
			Password               string `json:"password"`
			DisableConnectionReuse bool   `json:"disableConnectionReuse"`
		}
		if err := json.Unmarshal(obj["proxy"], &proxy); err != nil {
			return axonHubErr("invalid AxonHub backup: channel proxy configuration is invalid")
		}
		proxyObj, err := rowObject(obj["proxy"])
		if err != nil {
			return axonHubErr("invalid AxonHub backup: channel proxy configuration is invalid")
		}
		for _, field := range unknownKeys(proxyObj, []string{"type", "url", "username", "password", "disableConnectionReuse"}) {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy."+field)
		}
		if proxy.DisableConnectionReuse {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy.disableConnectionReuse")
		}
		switch proxy.Type {
		case "", "environment":
			// The source ignores any stale URL/auth when using the environment.
			dst.ProxyConfigured = false
		case "url":
			dst.ProxyURL = strings.TrimSpace(proxy.URL)
			if proxy.Username != "" && proxy.Password != "" {
				parsed, err := url.Parse(dst.ProxyURL)
				if err != nil {
					return axonHubErr("invalid AxonHub backup: channel proxy URL is invalid")
				}
				parsed.User = url.UserPassword(proxy.Username, proxy.Password)
				dst.ProxyURL = parsed.String()
			}
		case "disabled":
			// Direct grants currently inherit the process environment. Refuse
			// an explicit no-proxy contract instead of using a leftover URL.
		default:
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy.type")
		}
	}
	return nil
}

// AxonHub enables RPM/TPM and concurrency only for positive limits. Queue
// settings have no effect without a positive maxConcurrent. Unknown fields
// still refuse the channel, even when their values look inactive.
func decodeAxonHubRateLimit(raw json.RawMessage) (store.DirectRateLimit, bool, error) {
	result := store.DirectRateLimit{}
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return result, false, nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return result, false, axonHubErr("invalid AxonHub backup: rate limit must be an object")
	}
	var limits struct {
		RPM           *int64 `json:"rpm"`
		TPM           *int64 `json:"tpm"`
		MaxConcurrent *int64 `json:"maxConcurrent"`
		QueueSize     *int64 `json:"queueSize"`
		QueueTimeout  *int64 `json:"queueTimeoutMs"`
	}
	if err := json.Unmarshal(raw, &limits); err != nil {
		return result, false, axonHubErr("invalid AxonHub backup: rate limit has an invalid field type")
	}
	if len(unknownKeys(obj, []string{"rpm", "tpm", "maxConcurrent", "queueSize", "queueTimeoutMs"})) > 0 {
		return result, true, nil
	}
	positive := func(value *int64) int64 {
		if value != nil && *value > 0 {
			return *value
		}
		return 0
	}
	result.RPM, result.TPM, result.MaxConcurrent = positive(limits.RPM), positive(limits.TPM), positive(limits.MaxConcurrent)
	if result.MaxConcurrent > 0 {
		result.QueueSize, result.QueueTimeoutMs = positive(limits.QueueSize), positive(limits.QueueTimeout)
	}
	if result.Validate() != nil {
		return store.DirectRateLimit{}, true, nil
	}
	return result, false, nil
}

func decodeAxonHubPolicies(raw json.RawMessage, dst *AxonHubSourceChannelPolicies) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: channel policies must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubChannelPolicyFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: channel policies has an invalid field type")
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(obj["apiKeyAutoDisableRules"], &rules); err == nil {
		dst.AutoDisableRuleCount = len(rules)
	}
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

func decodeAxonHubModels(raw json.RawMessage) ([]AxonHubSourceModel, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: models must be an array")
	}
	out := make([]AxonHubSourceModel, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] is not an object", i)
		}
		if extra := unknownKeys(obj, axonHubModelFields); len(extra) > 0 {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has unsupported fields", i)
		}
		var model AxonHubSourceModel
		if err := json.Unmarshal(row, &model); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has an invalid field type", i)
		}
		if err := decodeAxonHubModelSettings(obj["settings"], &model.Settings); err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, nil
}

func decodeAxonHubModelSettings(raw json.RawMessage, dst *AxonHubSourceModelSettings) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model settings must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubModelSettingsFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: model settings has an invalid field type")
	}
	rows, err := rawSectionRows(obj["associations"])
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model associations must be an array")
	}
	for i, row := range rows {
		assoc, err := decodeAxonHubAssociation(row)
		if err != nil {
			return axonHubErr("invalid AxonHub backup: model associations[%d] is invalid", i)
		}
		dst.Associations = append(dst.Associations, assoc)
	}
	return nil
}

func decodeAxonHubAssociation(raw json.RawMessage) (AxonHubSourceAssociation, error) {
	var assoc AxonHubSourceAssociation
	obj, err := rowObject(raw)
	if err != nil {
		return assoc, err
	}
	if extra := unknownKeys(obj, axonHubAssociationFields); len(extra) > 0 {
		assoc.UnsupportedField = extra[0]
	}
	var shape struct {
		Type         string                     `json:"type"`
		Priority     int                        `json:"priority"`
		Disabled     bool                       `json:"disabled"`
		When         json.RawMessage            `json:"when"`
		ChannelModel *AxonHubSourceChannelModel `json:"channelModel"`
		ChannelRegex *AxonHubSourceChannelRegex `json:"channelRegex"`
		Regex        *struct {
			Pattern string            `json:"pattern"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"regex"`
		ModelID *struct {
			ModelID string            `json:"modelId"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"modelId"`
		ChannelTagsModel *AxonHubSourceTagsModel `json:"channelTagsModel"`
		ChannelTagsRegex *AxonHubSourceTagsRegex `json:"channelTagsRegex"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return assoc, err
	}
	assoc.Type = shape.Type
	assoc.Priority = shape.Priority
	assoc.Disabled = shape.Disabled
	if present(shape.When) {
		whenObject, err := rowObject(shape.When)
		if err != nil {
			return assoc, axonHubErr("invalid association condition")
		}
		var when struct {
			Enabled   bool            `json:"enabled"`
			Condition json.RawMessage `json:"condition"`
		}
		if err := json.Unmarshal(shape.When, &when); err != nil {
			return assoc, axonHubErr("invalid association condition")
		}
		if raw, exists := whenObject["enabled"]; exists && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return assoc, axonHubErr("invalid association condition enabled flag")
		}
		if extra := unknownKeys(whenObject, []string{"enabled", "condition"}); len(extra) > 0 && assoc.UnsupportedField == "" {
			assoc.UnsupportedField = "when." + sanitizeIdentifier(extra[0])
		}
		// AxonHub evaluates an enabled rule without a predicate as true.
		condition := bytes.TrimSpace(when.Condition)
		if when.Enabled && len(condition) > 0 && !bytes.Equal(condition, []byte("null")) {
			assoc.Condition, err = routecondition.Decode(condition)
			if err != nil {
				assoc.UnsupportedField = "when.condition"
			}
		}
	}
	assoc.ChannelModel = shape.ChannelModel
	assoc.ChannelRegex = shape.ChannelRegex
	assoc.ChannelTagsModel = shape.ChannelTagsModel
	assoc.ChannelTagsRegex = shape.ChannelTagsRegex
	if shape.Regex != nil {
		excludes, err := decodeAxonHubExcludes(shape.Regex.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.Regex = &AxonHubSourceRegex{Pattern: shape.Regex.Pattern, Exclude: excludes}
	}
	if shape.ModelID != nil {
		excludes, err := decodeAxonHubExcludes(shape.ModelID.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.ModelID = &AxonHubSourceModelIDMatch{ModelID: shape.ModelID.ModelID, Exclude: excludes}
	}
	return assoc, nil
}

func decodeAxonHubExcludes(rows []json.RawMessage) ([]AxonHubSourceExclude, error) {
	out := make([]AxonHubSourceExclude, 0, len(rows))
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, err
		}
		if extra := unknownKeys(obj, axonHubExcludeFields); len(extra) > 0 {
			return nil, axonHubErr("unsupported exclude field")
		}
		var exclude AxonHubSourceExclude
		if err := json.Unmarshal(row, &exclude); err != nil {
			return nil, err
		}
		out = append(out, exclude)
	}
	return out, nil
}

func decodeAxonHubSystemConfigs(raw json.RawMessage) ([]AxonHubSourceSystemConfig, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: system_configs must be an array")
	}
	out := make([]AxonHubSourceSystemConfig, 0, len(rows))
	for i, row := range rows {
		var entry AxonHubSourceSystemConfig
		if err := json.Unmarshal(row, &entry); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: system_configs[%d] is invalid", i)
		}
		out = append(out, entry)
	}
	return out, nil
}

func decodeAxonHubProjects(raw json.RawMessage) ([]AxonHubSourceProject, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: projects must be an array")
	}
	out := make([]AxonHubSourceProject, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ID          int             `json:"id"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Status      string          `json:"status"`
			DeletedAt   int             `json:"deleted_at"`
			Profiles    json.RawMessage `json:"profiles"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: projects[%d] is invalid", i)
		}
		profiles, err := decodeAxonHubAccessProfiles(shape.Profiles, true)
		if err != nil {
			return nil, err
		}
		out = append(out, AxonHubSourceProject{
			Profiles: profiles, DeletedAt: shape.DeletedAt,
			ID: shape.ID, Name: shape.Name, Description: shape.Description,
			Status: shape.Status, ProfileRows: profileRowCount(shape.Profiles),
		})
	}
	return out, nil
}

func profileRowCount(raw json.RawMessage) int {
	var shape struct {
		Profiles []json.RawMessage `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return 0
	}
	return len(shape.Profiles)
}

func decodeAxonHubPrices(raw json.RawMessage) ([]AxonHubSourceModelPrice, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: channel_model_prices must be an array")
	}
	out := make([]AxonHubSourceModelPrice, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ChannelName string          `json:"channel_name"`
			ModelID     string          `json:"model_id"`
			Price       json.RawMessage `json:"price"`
			ReferenceID string          `json:"reference_id"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channel_model_prices[%d] is invalid", i)
		}
		out = append(out, AxonHubSourceModelPrice{
			ChannelName: shape.ChannelName, ModelID: shape.ModelID,
			Price: shape.Price, ReferenceID: shape.ReferenceID,
		})
	}
	return out, nil
}

func decodeAxonHubAPIKeys(raw json.RawMessage) ([]AxonHubSourceAPIKey, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: api_keys must be an array")
	}
	out := make([]AxonHubSourceAPIKey, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			Key        string          `json:"key"`
			ID         int             `json:"id"`
			ProjectID  int             `json:"project_id"`
			Name       string          `json:"name"`
			Type       string          `json:"type"`
			Status     string          `json:"status"`
			DeletedAt  int             `json:"deleted_at"`
			Scopes     []string        `json:"scopes"`
			Profiles   json.RawMessage `json:"profiles"`
			AllowedIPs []string        `json:"allowed_ips"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: api_keys[%d] is invalid", i)
		}
		profiles, err := decodeAxonHubAccessProfiles(shape.Profiles, false)
		if err != nil {
			return nil, err
		}
		out = append(out, AxonHubSourceAPIKey{
			Key: shape.Key, Profiles: profiles, IPAllowlist: shape.AllowedIPs,
			ID: shape.ID, ProjectID: shape.ProjectID, Name: shape.Name, Type: shape.Type,
			Status: shape.Status, DeletedAt: shape.DeletedAt, Scopes: shape.Scopes,
			ProfileRows: profileRowCount(shape.Profiles), AllowedIPs: len(shape.AllowedIPs),
			HasQuota: apiKeyProfilesHaveQuota(shape.Profiles),
		})
	}
	return out, nil
}

func apiKeyProfilesHaveQuota(raw json.RawMessage) bool {
	var shape struct {
		Profiles []struct {
			Quota json.RawMessage `json:"quota"`
		} `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return false
	}
	for _, profile := range shape.Profiles {
		if present(profile.Quota) {
			return true
		}
	}
	return false
}
