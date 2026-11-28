package service

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// enrichAccountOverviewRow attaches admin-list overview fields used by the UI.
func enrichAccountOverviewRow(account map[string]any) map[string]any {
	if account == nil {
		return account
	}

	status := asString(account["status"])
	siteStatus := asString(account["siteStatus"])
	extraConfig := asStringPtr(account["extraConfig"])
	accessToken := asString(account["accessToken"])
	oauthProvider := asStringPtr(account["oauthProvider"])

	acct := &store.Account{
		AccessToken:   accessToken,
		APIToken:      asStringPtr(account["apiToken"]),
		ExtraConfig:   extraConfig,
		OAuthProvider: oauthProvider,
	}
	credentialMode := ResolveStoredCredentialMode(acct)
	capabilities := BuildCapabilitiesFromCredentialMode(credentialMode, HasSessionToken(accessToken))
	sessionCapable := capabilities.CanRefreshBalance

	account["credentialMode"] = string(credentialMode)
	account["capabilities"] = capabilities
	account["runtimeHealth"] = BuildRuntimeHealthForAccount(RuntimeHealthInput{
		AccountStatus:  status,
		SiteStatus:     siteStatus,
		ExtraConfig:    extraConfig,
		SessionCapable: &sessionCapable,
		OAuthProvider:  oauthProvider,
	})
	account["site"] = map[string]any{
		"id":       account["siteIdVal"],
		"name":     account["siteName"],
		"url":      account["siteUrl"],
		"platform": account["sitePlatform"],
		"status":   siteStatus,
	}
	// Prefer nested site; drop flat join aliases that are not account columns.
	delete(account, "siteIdVal")
	delete(account, "siteName")
	delete(account, "siteUrl")
	delete(account, "sitePlatform")
	delete(account, "siteStatus")
	// One credential policy for every admin surface (see RedactAccountSecrets):
	// plaintext secrets never leave the process on a read or a write response.
	RedactAccountSecrets(account)
	return account
}

func normalizeMapScanValues(m map[string]any) {
	for k, v := range m {
		switch typed := v.(type) {
		case []byte:
			m[k] = string(typed)
		}
	}
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return nil
	}
	return &s
}

// snakeToCamel converts snake_case to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// mapKeysToCamel returns a new map with all keys converted from snake_case to camelCase.
func mapKeysToCamel(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[snakeToCamel(k)] = v
	}
	return result
}
