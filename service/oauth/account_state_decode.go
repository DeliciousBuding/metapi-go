package oauth

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type parsedExtraConfig struct {
	OAuth *parsedOauthInfo `json:"oauth"`
}

type parsedOauthInfo struct {
	Provider             string      `json:"provider"`
	AccountID            interface{} `json:"accountId"`
	AccountKey           interface{} `json:"accountKey"`
	Email                interface{} `json:"email"`
	PlanType             interface{} `json:"planType"`
	ProjectID            interface{} `json:"projectId"`
	TokenExpiresAt       interface{} `json:"tokenExpiresAt"`
	RefreshToken         interface{} `json:"refreshToken"`
	IDToken              interface{} `json:"idToken"`
	SessionToken         interface{} `json:"sessionToken"`
	ProviderData         interface{} `json:"providerData"`
	Quota                interface{} `json:"quota"`
	ModelDiscoveryStatus interface{} `json:"modelDiscoveryStatus"`
	LastModelSyncAt      interface{} `json:"lastModelSyncAt"`
	LastModelSyncError   interface{} `json:"lastModelSyncError"`
	LastDiscoveredModels interface{} `json:"lastDiscoveredModels"`
}

func parseRawExtraConfig(extraConfig *string) *parsedExtraConfig {
	if extraConfig == nil || strings.TrimSpace(*extraConfig) == "" {
		return nil
	}
	var result parsedExtraConfig
	if err := json.Unmarshal([]byte(*extraConfig), &result); err != nil {
		return nil
	}
	return &result
}

// GetOauthInfoFromExtraConfig reads OAuth info from extraConfig JSON.
func GetOauthInfoFromExtraConfig(extraConfig *string) *OauthInfo {
	info := parseOauthInfoFromExtraConfig(extraConfig)
	if info == nil || info.Provider == "" {
		return nil
	}
	return info
}

// Normalized accounts store identity in columns and only state in extraConfig.
// The legacy standalone getter still requires a provider in the JSON.
func parseOauthInfoFromExtraConfig(extraConfig *string) *OauthInfo {
	parsed := parseRawExtraConfig(extraConfig)
	if parsed == nil || parsed.OAuth == nil {
		return nil
	}
	o := parsed.OAuth
	provider := strings.TrimSpace(o.Provider)
	info := &OauthInfo{
		Provider:   provider,
		AccountID:  asNonEmptyString(o.AccountID),
		AccountKey: asNonEmptyString(o.AccountKey),
		ProjectID:  asNonEmptyString(o.ProjectID),
		Email:      asNonEmptyString(o.Email),
		PlanType:   asNonEmptyString(o.PlanType),
	}
	if info.AccountID == "" {
		info.AccountID = info.AccountKey
	}
	if info.AccountKey == "" {
		info.AccountKey = info.AccountID
	}
	if v := asPositiveInteger(o.TokenExpiresAt); v > 0 {
		info.TokenExpiresAt = v
	}
	info.RefreshToken = asNonEmptyString(o.RefreshToken)
	info.IDToken = asNonEmptyString(o.IDToken)
	info.SessionToken = asNonEmptyString(o.SessionToken)
	if pd, ok := o.ProviderData.(map[string]interface{}); ok {
		info.ProviderData = pd
	}
	info.Quota = asQuotaSnapshotGo(o.Quota)
	if s := asNonEmptyString(o.ModelDiscoveryStatus); s != "" {
		lowered := strings.ToLower(s)
		if lowered == "healthy" {
			info.ModelDiscoveryStatus = OauthModelDiscoveryHealthy
		} else if lowered == "abnormal" {
			info.ModelDiscoveryStatus = OauthModelDiscoveryAbnormal
		}
	}
	info.LastModelSyncAt = asISODateTime(o.LastModelSyncAt)
	info.LastModelSyncError = asNonEmptyString(o.LastModelSyncError)
	if arr, ok := o.LastDiscoveredModels.([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				trimmed := strings.TrimSpace(s)
				if trimmed != "" {
					info.LastDiscoveredModels = append(info.LastDiscoveredModels, trimmed)
				}
			}
		}
	}
	return info
}

func asPositiveInteger(value interface{}) int64 {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int64(v)
		}
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0
		}
		var n int64
		if _, err := fmt.Sscanf(trimmed, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func asISODateTime(value interface{}) string {
	s := asNonEmptyString(value)
	if s == "" {
		return ""
	}
	// Validate it's a valid ISO date.
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}
	// Try other common formats.
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z07:00",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format(time.RFC3339)
		}
	}
	return ""
}

func asQuotaSnapshotGo(value interface{}) *OauthQuotaSnapshot {
	if value == nil {
		return nil
	}
	m, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	status, _ := m["status"].(string)
	source, _ := m["source"].(string)
	if status == "" || source == "" {
		return nil
	}
	snapshot := &OauthQuotaSnapshot{
		Status: status,
		Source: source,
	}
	if v, ok := m["lastSyncAt"].(string); ok {
		snapshot.LastSyncAt = v
	}
	if v, ok := m["lastError"].(string); ok {
		snapshot.LastError = v
	}
	if v, ok := m["providerMessage"].(string); ok {
		snapshot.ProviderMessage = v
	}
	snapshot.Windows = &OauthQuotaWindows{}
	if windows, ok := m["windows"].(map[string]interface{}); ok {
		if fh, ok := windows["fiveHour"].(map[string]interface{}); ok {
			snapshot.Windows.FiveHour = normalizeQuotaWindowGo(fh)
		}
		if sd, ok := windows["sevenDay"].(map[string]interface{}); ok {
			snapshot.Windows.SevenDay = normalizeQuotaWindowGo(sd)
		}
	}
	return snapshot
}

func normalizeQuotaWindowGo(raw map[string]interface{}) *OauthQuotaWindowSnapshot {
	supported, _ := raw["supported"].(bool)
	w := &OauthQuotaWindowSnapshot{Supported: supported}
	if v, ok := raw["limit"].(float64); ok {
		w.Limit = &v
	}
	if v, ok := raw["used"].(float64); ok {
		w.Used = &v
	}
	if v, ok := raw["remaining"].(float64); ok {
		w.Remaining = &v
	}
	if v, ok := raw["resetAt"].(string); ok {
		w.ResetAt = v
	}
	if v, ok := raw["message"].(string); ok {
		w.Message = v
	}
	return w
}
