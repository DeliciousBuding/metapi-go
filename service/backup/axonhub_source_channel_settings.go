package backup

import (
	"bytes"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/store"
)

var axonHubChannelSettingsFields = []string{
	"extraModelPrefix", "autoTrimedModelPrefixes", "modelMappings", "hideOriginalModels",
	"hideMappedModels", "lowercaseModelId", "overrideParameters", "bodyOverrideOperations",
	"overrideHeaders", "headerOverrideOperations", "proxy", "transformOptions",
	"passThroughUserAgent", "passThroughBody", "rateLimit", "retryableStatusCodes",
	"retryableErrorPatterns", "providerQuota", "modelProtocols",
}

var axonHubChannelPolicyFields = []string{"stream", "apiKeyAutoDisableRules"}

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
