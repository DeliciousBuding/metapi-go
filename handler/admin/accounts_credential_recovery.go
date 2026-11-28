package admin

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func shouldMirrorAPIKeyToken(account store.Account, requestedAPIToken any) bool {
	if service.ResolveStoredCredentialMode(&account) != service.CredentialModeAPIKey {
		return false
	}
	if requestedAPIToken == nil {
		return true
	}
	requested, ok := requestedAPIToken.(string)
	if !ok {
		return false
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	current := ""
	if account.APIToken != nil {
		current = strings.TrimSpace(*account.APIToken)
	}
	return requested == current
}

func normalizeAPITokenUpdate(value any) any {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return value
}

func normalizeAccountStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "disabled", "expired":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

// shouldRecoverExpiredAPIKey detects the expired API-key credential recovery path:
// previous status expired, next mode apikey, credentials changed, next status not forced disabled.
func shouldRecoverExpiredAPIKey(prev, next store.Account, updates map[string]any) bool {
	if !strings.EqualFold(strings.TrimSpace(prev.Status), "expired") {
		return false
	}
	nextStatus := strings.TrimSpace(prev.Status)
	if status, ok := updates["status"].(string); ok && strings.TrimSpace(status) != "" {
		nextStatus = strings.TrimSpace(status)
	}
	if strings.EqualFold(nextStatus, "disabled") {
		return false
	}
	mode := service.ResolveStoredCredentialMode(&next)
	if mode != service.CredentialModeAPIKey {
		return false
	}
	return credentialFieldsChanged(prev, updates)
}

func credentialFieldsChanged(prev store.Account, updates map[string]any) bool {
	if accessToken, ok := updates["accessToken"].(string); ok {
		if strings.TrimSpace(accessToken) != strings.TrimSpace(prev.AccessToken) {
			return true
		}
	}
	if apiTokenRaw, ok := updates["apiToken"]; ok {
		prevAPI := ""
		if prev.APIToken != nil {
			prevAPI = strings.TrimSpace(*prev.APIToken)
		}
		switch v := apiTokenRaw.(type) {
		case string:
			if strings.TrimSpace(v) != prevAPI {
				return true
			}
		case *string:
			nextAPI := ""
			if v != nil {
				nextAPI = strings.TrimSpace(*v)
			}
			if nextAPI != prevAPI {
				return true
			}
		default:
			if v != nil {
				return true
			}
		}
	}
	return false
}

// clearAccountAuthRuntimeHealth removes auth-sourced runtimeHealth best-effort after recovery.
func clearAccountAuthRuntimeHealth(db *store.DB, accountID int64) error {
	var extraConfig *string
	if err := db.Get(&extraConfig, "SELECT extra_config FROM accounts WHERE id = ?", accountID); err != nil {
		return err
	}
	cfg := service.ParseExtraConfig(extraConfig)
	if cfg == nil {
		return nil
	}
	healthRaw, ok := cfg["runtimeHealth"]
	if !ok || healthRaw == nil {
		return nil
	}
	b, err := json.Marshal(healthRaw)
	if err != nil {
		return nil
	}
	var entry service.RuntimeHealthEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(string(entry.Source))) != string(service.HealthSourceAuth) {
		return nil
	}
	merged := service.MergeExtraConfig(extraConfig, map[string]any{"runtimeHealth": nil})
	now := time.Now().UTC().Format(time.RFC3339)
	if merged == nil {
		_, err = db.Exec("UPDATE accounts SET extra_config = NULL, updated_at = ? WHERE id = ?", now, accountID)
		return err
	}
	_, err = db.Exec("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?", *merged, now, accountID)
	return err
}
