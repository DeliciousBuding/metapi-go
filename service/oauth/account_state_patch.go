package oauth

import (
	"fmt"

	"github.com/deliciousbuding/metapi-go/store"
)

// BuildOauthInfo builds OauthInfo from extraConfig with an optional patch.
// Returns an error when no OAuth provider can be determined — the caller should
// return a clean 4xx error to the client rather than panicking.
func BuildOauthInfo(extraConfig *string, patch *OauthInfo) (*OauthInfo, error) {
	provider := ""
	if patch != nil {
		provider = patch.Provider
	}
	if provider == "" {
		existing := GetOauthInfoFromExtraConfig(extraConfig)
		if existing != nil {
			provider = existing.Provider
		}
	}
	if provider == "" {
		return nil, fmt.Errorf("oauth provider is required")
	}
	current := GetOauthInfoFromExtraConfig(extraConfig)
	info := &OauthInfo{Provider: provider}
	if current != nil {
		*info = *current
	}
	if patch != nil {
		if patch.AccountID != "" {
			info.AccountID = patch.AccountID
		}
		if patch.AccountKey != "" {
			info.AccountKey = patch.AccountKey
		}
		if patch.Email != "" {
			info.Email = patch.Email
		}
		if patch.PlanType != "" {
			info.PlanType = patch.PlanType
		}
		if patch.ProjectID != "" {
			info.ProjectID = patch.ProjectID
		}
		if patch.TokenExpiresAt > 0 {
			info.TokenExpiresAt = patch.TokenExpiresAt
		}
		if patch.RefreshToken != "" {
			info.RefreshToken = patch.RefreshToken
		}
		if patch.IDToken != "" {
			info.IDToken = patch.IDToken
		}
		if patch.SessionToken != "" {
			info.SessionToken = patch.SessionToken
		}
		if patch.ProviderData != nil {
			info.ProviderData = patch.ProviderData
		}
		if patch.Quota != nil {
			info.Quota = patch.Quota
		}
		if patch.ModelDiscoveryStatus != "" {
			info.ModelDiscoveryStatus = patch.ModelDiscoveryStatus
		}
		if patch.LastModelSyncAt != "" {
			info.LastModelSyncAt = patch.LastModelSyncAt
		}
		if patch.LastModelSyncError != "" {
			info.LastModelSyncError = patch.LastModelSyncError
		}
		if patch.LastDiscoveredModels != nil {
			info.LastDiscoveredModels = patch.LastDiscoveredModels
		}
	}
	if info.AccountKey == "" && info.AccountID != "" {
		info.AccountKey = info.AccountID
	}
	if info.AccountID == "" && info.AccountKey != "" {
		info.AccountID = info.AccountKey
	}
	return info, nil
}

// BuildOauthInfoFromAccount builds OauthInfo from an account with optional patch.
// Returns an error when no OAuth provider can be determined.
func BuildOauthInfoFromAccount(account *store.Account, patch *OauthInfo) (*OauthInfo, error) {
	provider := ""
	if patch != nil {
		provider = patch.Provider
	}
	if provider == "" {
		existing := GetOauthInfoFromAccount(account)
		if existing != nil {
			provider = existing.Provider
		}
	}
	if provider == "" {
		return nil, fmt.Errorf("oauth provider is required")
	}
	current := GetOauthInfoFromAccount(account)
	info := &OauthInfo{Provider: provider}
	if current != nil {
		*info = *current
	}
	if patch != nil {
		if patch.AccountID != "" {
			info.AccountID = patch.AccountID
		}
		if patch.AccountKey != "" {
			info.AccountKey = patch.AccountKey
		}
		if patch.Email != "" {
			info.Email = patch.Email
		}
		if patch.PlanType != "" {
			info.PlanType = patch.PlanType
		}
		if patch.ProjectID != "" {
			info.ProjectID = patch.ProjectID
		}
		if patch.TokenExpiresAt > 0 {
			info.TokenExpiresAt = patch.TokenExpiresAt
		}
		if patch.RefreshToken != "" {
			info.RefreshToken = patch.RefreshToken
		}
		if patch.IDToken != "" {
			info.IDToken = patch.IDToken
		}
		if patch.SessionToken != "" {
			info.SessionToken = patch.SessionToken
		}
		if patch.ProviderData != nil {
			info.ProviderData = patch.ProviderData
		}
		if patch.Quota != nil {
			info.Quota = patch.Quota
		}
	}
	if info.AccountKey == "" && info.AccountID != "" {
		info.AccountKey = info.AccountID
	}
	if info.AccountID == "" && info.AccountKey != "" {
		info.AccountID = info.AccountKey
	}
	return info, nil
}

// BuildStoredOauthState strips identity fields from OauthInfo.
func BuildStoredOauthState(oauth *OauthInfo) *StoredOauthState {
	if oauth == nil {
		return nil
	}
	return &StoredOauthState{
		Email:                oauth.Email,
		PlanType:             oauth.PlanType,
		TokenExpiresAt:       oauth.TokenExpiresAt,
		RefreshToken:         oauth.RefreshToken,
		IDToken:              oauth.IDToken,
		SessionToken:         oauth.SessionToken,
		ProviderData:         oauth.ProviderData,
		Quota:                oauth.Quota,
		ModelDiscoveryStatus: oauth.ModelDiscoveryStatus,
		LastModelSyncAt:      oauth.LastModelSyncAt,
		LastModelSyncError:   oauth.LastModelSyncError,
		LastDiscoveredModels: oauth.LastDiscoveredModels,
	}
}

// BuildStoredOauthStateFromAccount builds stored state from account with optional patch.
func BuildStoredOauthStateFromAccount(account *store.Account, patch *OauthInfo) (*StoredOauthState, error) {
	info, err := BuildOauthInfoFromAccount(account, patch)
	if err != nil {
		return nil, err
	}
	return BuildStoredOauthState(info), nil
}
