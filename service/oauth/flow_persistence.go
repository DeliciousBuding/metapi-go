package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ActivateInput holds the input for activating a persisted OAuth account.
type ActivateInput struct {
	Definition      *OAuthProviderDefinition
	Exchange        *TokenSet
	RebindAccountID int64
	ProxyURL        string
	UseSystemProxy  bool
	PersistedStatus string // "active" or "disabled"
}

// PersistResult holds the result of account persistence.
type PersistResult struct {
	AccountID int64
	SiteID    int64
}

// activatePersistedOAuthAccount persists or updates an OAuth account with full pipeline:
// upsert -> refresh models -> rebuild routes, with rollback on failures.
func activatePersistedOAuthAccount(ctx context.Context, db *store.DB, input ActivateInput) (*PersistResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	def := input.Definition

	// Ensure site exists.
	site, err := ensureOAuthProviderSite(db, def)
	if err != nil {
		return nil, err
	}

	// Find existing account and save snapshot for rollback.
	existing := findExistingOAuthAccount(db, def.Metadata.Provider, input.Exchange, input.RebindAccountID)
	var snapshot *accountSnapshot
	if existing != nil {
		snapshot = captureAccountSnapshot(db, existing)
	}

	// Build username.
	username := input.Exchange.Email
	if username == "" {
		username = input.Exchange.AccountKey
	}
	if username == "" {
		username = input.Exchange.AccountID
	}
	if username == "" {
		username = fmt.Sprintf("%s-user", def.Metadata.Provider)
	}

	// Build OAuth info.
	oauthInfo, err := BuildOauthInfo(nil, &OauthInfo{
		Provider:       string(def.Metadata.Provider),
		AccountID:      input.Exchange.AccountID,
		AccountKey:     input.Exchange.AccountKey,
		Email:          input.Exchange.Email,
		PlanType:       input.Exchange.PlanType,
		ProjectID:      input.Exchange.ProjectID,
		RefreshToken:   input.Exchange.RefreshToken,
		TokenExpiresAt: input.Exchange.TokenExpiresAt,
		IDToken:        input.Exchange.IDToken,
		SessionToken:   input.Exchange.SessionToken,
		ProviderData:   input.Exchange.ProviderData,
	})
	if err != nil {
		return nil, err
	}

	// Build extra config merge patch.
	extraPatch := map[string]interface{}{
		"credentialMode": "session",
	}
	if input.ProxyURL != "" {
		extraPatch["proxyUrl"] = input.ProxyURL
	}
	if input.UseSystemProxy {
		extraPatch["useSystemProxy"] = true
	}
	oauthMap := make(map[string]interface{})
	if oauthInfo.Email != "" {
		oauthMap["email"] = oauthInfo.Email
	}
	if oauthInfo.PlanType != "" {
		oauthMap["planType"] = oauthInfo.PlanType
	}
	if oauthInfo.TokenExpiresAt > 0 {
		oauthMap["tokenExpiresAt"] = oauthInfo.TokenExpiresAt
	}
	if oauthInfo.RefreshToken != "" {
		oauthMap["refreshToken"] = oauthInfo.RefreshToken
	}
	if oauthInfo.IDToken != "" {
		oauthMap["idToken"] = oauthInfo.IDToken
	}
	if oauthInfo.ProviderData != nil {
		oauthMap["providerData"] = oauthInfo.ProviderData
	}
	extraPatch["oauth"] = oauthMap

	var accountID int64
	created := false
	now := time.Now().Format(time.RFC3339)
	var writtenAccessToken string
	var writtenExtraConfig *string

	if existing != nil {
		// Update existing account.
		status := input.PersistedStatus
		if status == "" {
			status = "disabled"
		}
		accountKey := oauthInfo.AccountKey
		if accountKey == "" {
			accountKey = oauthInfo.AccountID
		}
		projectID := oauthInfo.ProjectID

		extraConfigStr := MergeAccountExtraConfig(existing.ExtraConfig, extraPatch)

		_, err := db.Exec(
			`UPDATE accounts SET site_id = ?, username = ?, access_token = ?, api_token = NULL,
			 checkin_enabled = ?, status = ?, oauth_provider = ?, oauth_account_key = ?,
			 oauth_project_id = ?, extra_config = ?, updated_at = ? WHERE id = ?`,
			site.ID, username, input.Exchange.AccessToken, false, status,
			string(def.Metadata.Provider), accountKey, projectID,
			extraConfigStr, now, existing.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update oauth account: %w", err)
		}
		accountID = existing.ID
		writtenAccessToken = input.Exchange.AccessToken
		writtenExtraConfig = extraConfigStr
	} else {
		// Create new account.
		created = true
		status := input.PersistedStatus
		if status == "" {
			status = "active"
		}
		accountKey := oauthInfo.AccountKey
		if accountKey == "" {
			accountKey = oauthInfo.AccountID
		}
		projectID := oauthInfo.ProjectID

		extraConfigStr := MergeAccountExtraConfig(nil, extraPatch)

		// Get next sort order.
		var maxSortOrder int64
		if err := db.Get(&maxSortOrder, db.Rebind("SELECT COALESCE(MAX(sort_order), -1) FROM accounts")); err != nil {
			slog.Warn("oauth: failed to query max sort_order for accounts, defaulting to 1", "error", err)
		}
		sortOrder := maxSortOrder + 1

		accountID, err = execStoreInsertID(db,
			`INSERT INTO accounts (site_id, username, access_token, api_token, checkin_enabled,
			 status, oauth_provider, oauth_account_key, oauth_project_id, extra_config,
			 is_pinned, sort_order, balance, balance_used, quota, value_score, created_at, updated_at)
			 VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0, 0, 0, ?, ?)`,
			site.ID, username, input.Exchange.AccessToken, false, status,
			string(def.Metadata.Provider), accountKey, projectID,
			extraConfigStr, false, sortOrder, now, now,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create oauth account: %w", err)
		}
	}

	// Step 7c: Refresh models for account (allowInactive for existing updates).
	hooks := getWorkflowHooks()
	if hooks != nil {
		allowInactive := !created
		if err := hooks.RefreshModelsForAccount(ctx, accountID, allowInactive); err != nil {
			// Step 7d: Full rollback on model refresh failure (CAS-fenced to avoid clobbering a newer rebind).
			slog.Warn("model refresh failed for account, rolling back", "accountID", accountID, "error", err)
			rollbackErr := revertPersistedOauthAccount(db, accountID, created, snapshot, writtenAccessToken, writtenExtraConfig)
			if rollbackErr != nil {
				slog.Warn("rollback failed for account", "accountID", accountID, "error", rollbackErr)
			}
			return nil, fmt.Errorf("model refresh failed for oauth account: %w", err)
		}

		// If existing and we just updated: activate the account now that model refresh succeeded.
		if !created && input.PersistedStatus == "" {
			if writtenExtraConfig == nil {
				if _, err := db.Exec(db.Rebind("UPDATE accounts SET status = 'active', updated_at = ? WHERE id = ? AND access_token = ? AND extra_config IS NULL"), now, accountID, writtenAccessToken); err != nil {
					slog.Warn("oauth status activation failed", "error", err, "account_id", accountID)
				}
			} else {
				if _, err := db.Exec(db.Rebind("UPDATE accounts SET status = 'active', updated_at = ? WHERE id = ? AND access_token = ? AND extra_config = ?"), now, accountID, writtenAccessToken, *writtenExtraConfig); err != nil {
					slog.Warn("oauth status activation failed", "error", err, "account_id", accountID)
				}
			}
		}

		// Step 7f: Rebuild routes.
		if err := hooks.RebuildRoutesOnly(ctx); err != nil {
			// Step 7g: On route rebuild fail: rollback account + restore routes (CAS-fenced).
			slog.Warn("route rebuild failed after account, rolling back", "accountID", accountID, "error", err)
			rollbackErr := revertPersistedOauthAccount(db, accountID, created, snapshot, writtenAccessToken, writtenExtraConfig)
			if rollbackErr != nil {
				slog.Warn("rollback failed for account", "accountID", accountID, "error", rollbackErr)
			}
			// Best-effort retry rebuild after rollback.
			_ = hooks.RebuildRoutesOnly(ctx)
			return nil, fmt.Errorf("route rebuild failed for oauth account: %w", err)
		}

		// Invalidate token router cache after successful rebuild.
		hooks.InvalidateTokenRouterCache()
	}

	return &PersistResult{
		AccountID: accountID,
		SiteID:    site.ID,
	}, nil
}

// ensureOAuthProviderSite ensures the site for a provider exists.
func ensureOAuthProviderSite(db *store.DB, def *OAuthProviderDefinition) (*store.Site, error) {
	var site store.Site
	err := db.Get(&site,
		"SELECT id, name, url, external_checkin_url, platform, proxy_url, use_system_proxy, custom_headers, custom_headers_override_request_headers, status, is_pinned, sort_order, global_weight, api_key, max_concurrency, post_refresh_probe_enabled, post_refresh_probe_model, post_refresh_probe_scope, post_refresh_probe_latency_threshold_ms, created_at, updated_at FROM sites WHERE platform = ? AND url = ? LIMIT 1",
		def.Site.Platform, def.Site.URL)
	if err == nil {
		return &site, nil
	}

	// Get next sort order.
	var maxSortOrder int64
	if err := db.Get(&maxSortOrder, db.Rebind("SELECT COALESCE(MAX(sort_order), -1) FROM sites")); err != nil {
		slog.Warn("oauth: failed to query max sort_order for sites, defaulting to 1", "error", err)
	}
	sortOrder := maxSortOrder + 1
	now := time.Now().Format(time.RFC3339)

	id, err := execStoreInsertID(db,
		`INSERT INTO sites (name, url, platform, status, use_system_proxy, is_pinned, global_weight, sort_order, created_at, updated_at)
		 VALUES (?, ?, ?, 'active', ?, ?, 1, ?, ?, ?)`,
		def.Site.Name, def.Site.URL, def.Site.Platform, false, false, sortOrder, now, now,
	)
	if err != nil {
		// Try reading again in case of race.
		err2 := db.Get(&site,
			"SELECT id, name, url, external_checkin_url, platform, proxy_url, use_system_proxy, custom_headers, custom_headers_override_request_headers, status, is_pinned, sort_order, global_weight, api_key, max_concurrency, post_refresh_probe_enabled, post_refresh_probe_model, post_refresh_probe_scope, post_refresh_probe_latency_threshold_ms, created_at, updated_at FROM sites WHERE platform = ? AND url = ? LIMIT 1",
			def.Site.Platform, def.Site.URL)
		if err2 == nil {
			return &site, nil
		}
		return nil, fmt.Errorf("failed to create oauth provider site: %w", err)
	}

	site.ID = id
	site.Name = def.Site.Name
	site.URL = def.Site.URL
	site.Platform = def.Site.Platform
	site.Status = "active"
	return &site, nil
}

func execStoreInsertID(db *store.DB, query string, args ...any) (int64, error) {
	if db.Dialect == store.DialectPostgres {
		var id int64
		err := db.QueryRowx(query+" RETURNING id", args...).Scan(&id)
		return id, err
	}

	result, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// findExistingOAuthAccount finds an existing OAuth account for dedup.
func findExistingOAuthAccount(db *store.DB, provider OAuthProviderId, exchange *TokenSet, rebindAccountID int64) *store.Account {
	if rebindAccountID > 0 {
		var account store.Account
		err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", rebindAccountID)
		if err == nil {
			return &account
		}
		return nil
	}

	accountKey := exchange.AccountKey
	if accountKey == "" {
		accountKey = exchange.AccountID
	}

	if accountKey != "" {
		var account store.Account
		projectID := strings.TrimSpace(exchange.ProjectID)
		var err error
		if projectID != "" {
			err = db.Get(&account,
				"SELECT * FROM accounts WHERE oauth_provider = ? AND oauth_account_key = ? AND oauth_project_id = ? LIMIT 1",
				string(provider), accountKey, projectID)
		} else {
			err = db.Get(&account,
				"SELECT * FROM accounts WHERE oauth_provider = ? AND oauth_account_key = ? AND (oauth_project_id IS NULL OR oauth_project_id = '') LIMIT 1",
				string(provider), accountKey)
		}
		if err == nil {
			return &account
		}
	}

	email := exchange.Email
	if accountKey == "" && email != "" && provider != ProviderCodex {
		var account store.Account
		err := db.Get(&account,
			"SELECT * FROM accounts WHERE oauth_provider = ? AND username = ? LIMIT 1",
			string(provider), email)
		if err == nil {
			return &account
		}
	}

	return nil
}
