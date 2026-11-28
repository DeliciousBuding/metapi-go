package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// DeleteOauthConnection deletes an OAuth connection.
func DeleteOauthConnection(db *store.DB, accountID int64) error {
	if _, _, err := loadManagedOauthAccount(db, accountID); err != nil {
		return err
	}

	_, err := db.Exec("DELETE FROM accounts WHERE id = ?", accountID)
	if err != nil {
		return err
	}

	// Rebuild routes after deletion.
	hooks := getWorkflowHooks()
	if hooks != nil {
		if rebuildErr := hooks.RebuildRoutesOnly(context.Background()); rebuildErr != nil {
			slog.Warn("route rebuild failed after deleting connection", "accountID", accountID, "error", rebuildErr)
		}
		hooks.InvalidateTokenRouterCache()
	}

	return nil
}

// UpdateOauthConnectionProxySettings updates proxy settings for an OAuth connection.
func UpdateOauthConnectionProxySettings(db *store.DB, accountID int64, proxyURL *string, useSystemProxy *bool) (*UpdateProxyResult, error) {
	account, _, err := loadManagedOauthAccount(db, accountID)
	if err != nil {
		return nil, err
	}

	patch := make(map[string]interface{})
	if proxyURL != nil {
		if *proxyURL != "" {
			patch["proxyUrl"] = *proxyURL
		} else {
			patch["proxyUrl"] = nil
		}
	}
	if useSystemProxy != nil {
		patch["useSystemProxy"] = *useSystemProxy
	}

	extraConfig := MergeAccountExtraConfig(account.ExtraConfig, patch)
	now := time.Now().Format(time.RFC3339)

	_, err = db.Exec("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?",
		extraConfig, now, accountID)
	if err != nil {
		return nil, err
	}

	// Refresh models for account (allowInactive=true) and rebuild routes.
	modelRefreshStatus := "success"
	var modelRefreshErrMsg string
	hooks := getWorkflowHooks()
	if hooks != nil {
		if refreshErr := hooks.RefreshModelsForAccount(context.Background(), accountID, true); refreshErr != nil {
			modelRefreshStatus = "error"
			modelRefreshErrMsg = refreshErr.Error()
			slog.Warn("model refresh failed in UpdateProxySettings", "accountID", accountID, "error", refreshErr)
		}
		if rebuildErr := hooks.RebuildRoutesOnly(context.Background()); rebuildErr != nil {
			slog.Warn("route rebuild failed in UpdateProxySettings", "accountID", accountID, "error", rebuildErr)
		}
		hooks.InvalidateTokenRouterCache()
	}

	return &UpdateProxyResult{
		Success:         true,
		AccountID:       accountID,
		ProxyURL:        GetProxyURLFromExtraConfig(extraConfig),
		UseSystemProxy:  GetUseSystemProxyFromExtraConfig(extraConfig),
		RefreshedRoutes: true,
		ModelRefresh: ModelRefreshResult{
			Success:      modelRefreshStatus == "success",
			Status:       modelRefreshStatus,
			ErrorMessage: modelRefreshErrMsg,
		},
	}, nil
}

// UpdateProxyResult holds the result of updating proxy settings.
type UpdateProxyResult struct {
	Success         bool               `json:"success"`
	AccountID       int64              `json:"accountId"`
	ProxyURL        string             `json:"proxyUrl"`
	UseSystemProxy  bool               `json:"useSystemProxy"`
	RefreshedRoutes bool               `json:"refreshedRoutes"`
	ModelRefresh    ModelRefreshResult `json:"modelRefresh"`
}

// ModelRefreshResult holds model refresh status.
type ModelRefreshResult struct {
	Success      bool   `json:"success"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// StartOauthRebindFlow starts a rebind flow for an existing OAuth account.
func StartOauthRebindFlow(db *store.DB, accountID int64, requestOrigin string, proxyURL *string, useSystemProxy *bool) (*FlowStartResult, error) {
	account, oauth, err := loadManagedOauthAccount(db, accountID)
	if err != nil {
		return nil, err
	}

	resolvedProxyURL := ""
	if proxyURL != nil {
		resolvedProxyURL = *proxyURL
	} else {
		resolvedProxyURL = GetProxyURLFromExtraConfig(account.ExtraConfig)
	}

	resolvedUseSystemProxy := false
	if useSystemProxy != nil {
		resolvedUseSystemProxy = *useSystemProxy
	} else {
		resolvedUseSystemProxy = GetUseSystemProxyFromExtraConfig(account.ExtraConfig)
	}

	return StartFlow(db, StartFlowInput{
		Provider:        oauth.Provider,
		RebindAccountID: accountID,
		ProjectID:       oauth.ProjectID,
		ProxyURL:        resolvedProxyURL,
		UseSystemProxy:  resolvedUseSystemProxy,
		RequestOrigin:   requestOrigin,
	})
}

func loadManagedOauthAccount(db *store.DB, accountID int64) (*store.Account, *OauthInfo, error) {
	if db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return nil, nil, fmt.Errorf("oauth account not found")
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil {
		return nil, nil, fmt.Errorf("account is not managed by oauth")
	}

	return &account, oauth, nil
}
