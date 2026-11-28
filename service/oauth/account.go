package oauth

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// OauthModelDiscoveryStatus is the model discovery status for an OAuth account.
type OauthModelDiscoveryStatus string

const (
	OauthModelDiscoveryHealthy  OauthModelDiscoveryStatus = "healthy"
	OauthModelDiscoveryAbnormal OauthModelDiscoveryStatus = "abnormal"
)

// OauthInfo holds the full OAuth information for an account.
type OauthInfo struct {
	Provider             string                    `json:"provider"`
	AccountID            string                    `json:"accountId,omitempty"`
	AccountKey           string                    `json:"accountKey,omitempty"`
	Email                string                    `json:"email,omitempty"`
	PlanType             string                    `json:"planType,omitempty"`
	ProjectID            string                    `json:"projectId,omitempty"`
	TokenExpiresAt       int64                     `json:"tokenExpiresAt,omitempty"`
	RefreshToken         string                    `json:"refreshToken,omitempty"`
	IDToken              string                    `json:"idToken,omitempty"`
	SessionToken         string                    `json:"sessionToken,omitempty"`
	ProviderData         map[string]interface{}    `json:"providerData,omitempty"`
	Quota                *OauthQuotaSnapshot       `json:"quota,omitempty"`
	ModelDiscoveryStatus OauthModelDiscoveryStatus `json:"modelDiscoveryStatus,omitempty"`
	LastModelSyncAt      string                    `json:"lastModelSyncAt,omitempty"`
	LastModelSyncError   string                    `json:"lastModelSyncError,omitempty"`
	LastDiscoveredModels []string                  `json:"lastDiscoveredModels,omitempty"`
}

// StoredOauthState is OauthInfo minus identity fields (provider, accountId, accountKey, projectId).
type StoredOauthState struct {
	Email                string                    `json:"email,omitempty"`
	PlanType             string                    `json:"planType,omitempty"`
	TokenExpiresAt       int64                     `json:"tokenExpiresAt,omitempty"`
	RefreshToken         string                    `json:"refreshToken,omitempty"`
	IDToken              string                    `json:"idToken,omitempty"`
	SessionToken         string                    `json:"sessionToken,omitempty"`
	ProviderData         map[string]interface{}    `json:"providerData,omitempty"`
	Quota                *OauthQuotaSnapshot       `json:"quota,omitempty"`
	ModelDiscoveryStatus OauthModelDiscoveryStatus `json:"modelDiscoveryStatus,omitempty"`
	LastModelSyncAt      string                    `json:"lastModelSyncAt,omitempty"`
	LastModelSyncError   string                    `json:"lastModelSyncError,omitempty"`
	LastDiscoveredModels []string                  `json:"lastDiscoveredModels,omitempty"`
}

// GetOauthInfoFromAccount reads OAuth info from an account record.
// Prefers column fields over extraConfig.oauth fields.
func GetOauthInfoFromAccount(account *store.Account) *OauthInfo {
	if account == nil {
		return nil
	}
	storedState := parseOauthInfoFromExtraConfig(account.ExtraConfig)
	var storedIdentity *OauthInfo
	if storedState != nil && storedState.Provider != "" {
		storedIdentity = storedState
	}
	provider := ""
	if account.OAuthProvider != nil {
		provider = strings.TrimSpace(*account.OAuthProvider)
	}
	if provider == "" && storedIdentity != nil {
		provider = storedIdentity.Provider
	}
	if provider == "" {
		return nil
	}

	var accountKey string
	if account.OAuthAccountKey != nil {
		accountKey = strings.TrimSpace(*account.OAuthAccountKey)
	}
	if accountKey == "" && storedIdentity != nil {
		accountKey = storedIdentity.AccountKey
	}
	if accountKey == "" && storedIdentity != nil {
		accountKey = storedIdentity.AccountID
	}

	// Guard against storedIdentity being nil when the OAuth provider is set
	// via the column field but no extraConfig was ever written. Without this
	// guard the unconditional access below would panic with a nil dereference.
	var accountID string
	if storedIdentity != nil {
		accountID = storedIdentity.AccountID
	}
	if accountID == "" {
		accountID = accountKey
	}

	var projectID string
	if account.OAuthProjectID != nil {
		projectID = strings.TrimSpace(*account.OAuthProjectID)
	}
	if projectID == "" && storedIdentity != nil {
		projectID = storedIdentity.ProjectID
	}

	info := &OauthInfo{
		Provider:   provider,
		AccountID:  accountID,
		AccountKey: accountKey,
		ProjectID:  projectID,
	}
	if storedState != nil {
		info.Email = storedState.Email
		info.PlanType = storedState.PlanType
		info.TokenExpiresAt = storedState.TokenExpiresAt
		info.RefreshToken = storedState.RefreshToken
		info.IDToken = storedState.IDToken
		info.SessionToken = storedState.SessionToken
		info.ProviderData = storedState.ProviderData
		info.Quota = storedState.Quota
		info.ModelDiscoveryStatus = storedState.ModelDiscoveryStatus
		info.LastModelSyncAt = storedState.LastModelSyncAt
		info.LastModelSyncError = storedState.LastModelSyncError
		info.LastDiscoveredModels = storedState.LastDiscoveredModels
	}
	return info
}

// BuildOauthIdentityBackfillPatch produces backfill patches for column fields.
func BuildOauthIdentityBackfillPatch(account *store.Account) map[string]*string {
	if account == nil {
		return nil
	}
	legacy := GetOauthInfoFromExtraConfig(account.ExtraConfig)
	if legacy == nil {
		return nil
	}
	patch := make(map[string]*string)
	if (account.OAuthProvider == nil || strings.TrimSpace(*account.OAuthProvider) == "") && legacy.Provider != "" {
		s := legacy.Provider
		patch["oauthProvider"] = &s
	}
	if (account.OAuthAccountKey == nil || strings.TrimSpace(*account.OAuthAccountKey) == "") && (legacy.AccountKey != "" || legacy.AccountID != "") {
		s := legacy.AccountKey
		if s == "" {
			s = legacy.AccountID
		}
		if s != "" {
			patch["oauthAccountKey"] = &s
		}
	}
	if (account.OAuthProjectID == nil || strings.TrimSpace(*account.OAuthProjectID) == "") && legacy.ProjectID != "" {
		s := legacy.ProjectID
		patch["oauthProjectId"] = &s
	}
	if len(patch) == 0 {
		return nil
	}
	return patch
}
