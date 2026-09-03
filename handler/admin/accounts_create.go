package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Create Account ----

func (h *accountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountCreatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid account payload: "+err.Error())
		return
	}

	if body.SiteID <= 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}

	// Check site exists
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "site not found")
		return
	}

	if body.ProxyURL != nil {
		normalizedProxyURL := strings.TrimSpace(*body.ProxyURL)
		if normalizedProxyURL != "" && !service.IsValidProxyURL(normalizedProxyURL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid proxyUrl. Expected a valid http(s)/socks proxy URL.")
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}

	// Resolve credential mode
	credentialMode := service.ResolveRequestedCredentialMode(body.CredentialMode)
	if len(body.AccessTokens) > 0 {
		credentialMode = service.CredentialModeAPIKey
	}

	// Resolve tokens to create
	var requestedTokens []string
	appendRequestedToken := func(token string) {
		token = strings.TrimSpace(token)
		if token != "" {
			requestedTokens = append(requestedTokens, token)
		}
	}
	if credentialMode != service.CredentialModeAPIKey {
		if body.AccessToken != nil && strings.TrimSpace(*body.AccessToken) != "" {
			appendRequestedToken(*body.AccessToken)
		}
	} else {
		if len(body.AccessTokens) > 0 {
			for _, token := range body.AccessTokens {
				appendRequestedToken(token)
			}
		} else if body.AccessToken != nil && strings.TrimSpace(*body.AccessToken) != "" {
			appendRequestedToken(*body.AccessToken)
		}
	}

	if len(requestedTokens) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "token is required")
		return
	}

	// Batch API key creation (multiple tokens)
	if credentialMode == service.CredentialModeAPIKey && len(requestedTokens) > 1 {
		var items []map[string]any
		createdCount := 0
		for i, token := range requestedTokens {
			username := body.Username
			if username != nil {
				name := *username
				if len(name) > 0 {
					n := buildBatchAPIKeyName(name, i, len(requestedTokens))
					username = &n
				}
			}

			created, err := h.createSingleAccount(r.Context(), body, site, credentialMode, token, username)
			if err != nil {
				slog.Error("Batch account creation failed", "err", err, "index", i)
				item := map[string]any{
					"index":   i,
					"status":  "failed",
					"message": err.Error(),
				}
				var verifyErr *accountCreateError
				if errors.As(err, &verifyErr) {
					item["message"] = verifyErr.Message
					item["requiresVerification"] = verifyErr.RequiresVerification
				}
				items = append(items, item)
			} else {
				createdCount++
				items = append(items, map[string]any{
					"index":      i,
					"status":     "created",
					"id":         created.ID,
					"username":   coalescePtr(created.Username, ""),
					"queued":     false,
					"message":    nil,
					"modelCount": created.ModelCount,
				})
			}
		}

		if createdCount == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"success":      false,
				"batch":        true,
				"totalCount":   len(requestedTokens),
				"createdCount": 0,
				"failedCount":  len(requestedTokens),
				"message":      "batch add failed (0/" + strconv.Itoa(len(requestedTokens)) + ")",
				"items":        items,
			})
			return
		}

		routing.InvalidateCache()
		globalAccountsCache.clear()
		writeJSON(w, http.StatusOK, map[string]any{
			"success":      true,
			"batch":        true,
			"totalCount":   len(requestedTokens),
			"createdCount": createdCount,
			"failedCount":  len(requestedTokens) - createdCount,
			"message":      "batch add completed: " + strconv.Itoa(createdCount) + " succeeded, " + strconv.Itoa(len(requestedTokens)-createdCount) + " failed",
			"items":        items,
		})
		return
	}

	// Single token creation
	created, err := h.createSingleAccount(r.Context(), body, site, credentialMode, requestedTokens[0], body.Username)
	if err != nil {
		slog.Error("Account creation failed", "err", err)
		message := err.Error()
		requiresVerification := false
		var verifyErr *accountCreateError
		if errors.As(err, &verifyErr) {
			message = verifyErr.Message
			requiresVerification = verifyErr.RequiresVerification
			if credentialMode != service.CredentialModeAPIKey {
				message = alert.AppendSessionTokenRebindHint(message)
			}
		} else if credentialMode != service.CredentialModeAPIKey {
			message = alert.AppendSessionTokenRebindHint(message)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":              false,
			"requiresVerification": requiresVerification,
			"message":              message,
		})
		return
	}

	// Fetch created account
	var account store.Account
	h.db.Get(&account, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), created.ID)

	// Auto-sync upstream tokens via the existing sync path (#1002). The report
	// is truth-only: a sync failure never rolls back the persisted account.
	syncReport := syncTokensAfterAccountCreate(r.Context(), h.db, h.cfg, created.ID)

	caps := service.BuildCapabilitiesForAccount(&account)
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               account.ID,
		"siteId":           account.SiteID,
		"username":         account.Username,
		"accessToken":      account.AccessToken,
		"status":           account.Status,
		"tokenType":        created.TokenType,
		"credentialMode":   string(service.ResolveStoredCredentialMode(&account)),
		"capabilities":     caps,
		"modelCount":       created.ModelCount,
		"apiTokenFound":    created.APITokenFound,
		"usernameDetected": created.UsernameDetected,
		"queued":           false,
		"tokenCount":       syncReport.TokenCount,
		"tokenSyncStatus":  syncReport.Status,
		"tokenSyncMessage": syncReport.Message,
	})
}

// accountCreateError is returned when create fails closed on token verification.
type accountCreateError struct {
	Message              string
	RequiresVerification bool
}

func (e *accountCreateError) Error() string {
	if e == nil {
		return "account create failed"
	}
	return e.Message
}

// createAccountOutcome holds metadata from a successful createSingleAccount call.
type createAccountOutcome struct {
	ID               int64
	Username         *string
	TokenType        string
	ModelCount       int
	APITokenFound    bool
	UsernameDetected bool
}

func (h *accountsHandler) createSingleAccount(ctx context.Context, body payloads.AccountCreatePayload, site store.Site, credentialMode service.AccountCredentialMode, rawAccessToken string, username *string) (*createAccountOutcome, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	rawAccessToken = strings.TrimSpace(rawAccessToken)
	if rawAccessToken == "" {
		return nil, &accountCreateError{Message: "token is required", RequiresVerification: false}
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return nil, &accountCreateError{
			Message:              "platform not supported: " + site.Platform,
			RequiresVerification: false,
		}
	}

	skipModelFetch := body.SkipModelFetch != nil && *body.SkipModelFetch
	proxyOverride := ""
	if body.ProxyURL != nil {
		proxyOverride = *body.ProxyURL
	}
	proxyCfg := service.BuildPlatformProxyConfigForTokenWithProxyURL(h.cfg, &site, rawAccessToken, proxyOverride)

	resolvedUsername := strings.TrimSpace(coalescePtr(username, ""))
	usernameProvided := resolvedUsername != ""
	accessTokenVal := rawAccessToken
	apiTokenVal := ""
	if body.APIToken != nil {
		apiTokenVal = strings.TrimSpace(*body.APIToken)
	}
	var tokenType string
	verifiedModels := []string{}

	verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	switch credentialMode {
	case service.CredentialModeAPIKey:
		if skipModelFetch {
			// Explicit skip: accept as API-key without upstream model verify.
			tokenType = "apikey"
			accessTokenVal = ""
			if apiTokenVal == "" {
				apiTokenVal = rawAccessToken
			}
		} else {
			models, err := adp.GetModels(verifyCtx, site.URL, rawAccessToken, body.PlatformUserID, proxyCfg)
			if err != nil {
				return nil, &accountCreateError{
					Message:              "API key verification failed: " + err.Error(),
					RequiresVerification: true,
				}
			}
			for _, m := range models {
				m = strings.TrimSpace(m)
				if m != "" {
					verifiedModels = append(verifiedModels, m)
				}
			}
			if len(verifiedModels) == 0 {
				return nil, &accountCreateError{
					Message:              "API key verification failed: no available models found",
					RequiresVerification: true,
				}
			}
			tokenType = "apikey"
			accessTokenVal = ""
			if apiTokenVal == "" {
				apiTokenVal = rawAccessToken
			}
		}
	default:
		// auto / session: require platform.VerifyToken success (fail closed).
		result, err := adp.VerifyToken(verifyCtx, site.URL, rawAccessToken, body.PlatformUserID, proxyCfg)
		if err != nil {
			return nil, &accountCreateError{
				Message:              err.Error(),
				RequiresVerification: true,
			}
		}
		if result == nil || result.TokenType == "" || result.TokenType == "unknown" {
			return nil, &accountCreateError{
				Message:              "token verification failed; click \"Verify Token\" first, then bind the account after verification succeeds",
				RequiresVerification: true,
			}
		}
		tokenType = result.TokenType

		if credentialMode == service.CredentialModeSession && tokenType != "session" {
			return nil, &accountCreateError{
				Message:              "the current credential is an API key; switch to API key mode or use a session token instead",
				RequiresVerification: false,
			}
		}

		if tokenType == "session" {
			if resolvedUsername == "" && result.UserInfo != nil {
				if u := strings.TrimSpace(result.UserInfo.Username); u != "" {
					resolvedUsername = u
				} else if u := strings.TrimSpace(result.UserInfo.DisplayName); u != "" {
					resolvedUsername = u
				}
			}
			if apiTokenVal == "" && strings.TrimSpace(result.APIToken) != "" {
				apiTokenVal = strings.TrimSpace(result.APIToken)
			}
			accessTokenVal = rawAccessToken
		} else if tokenType == "apikey" {
			accessTokenVal = ""
			if apiTokenVal == "" {
				apiTokenVal = rawAccessToken
			}
			for _, m := range result.Models {
				m = strings.TrimSpace(m)
				if m != "" {
					verifiedModels = append(verifiedModels, m)
				}
			}
		}
	}

	// Resolve platform user id (explicit body wins; else guess from username).
	var resolvedPlatformUserID *int
	if body.PlatformUserID != nil {
		resolvedPlatformUserID = body.PlatformUserID
	} else if guessed := service.GuessPlatformUserIdFromUsername(&resolvedUsername); guessed > 0 {
		id := int(guessed)
		resolvedPlatformUserID = &id
	}

	resolvedCredentialMode := service.CredentialModeSession
	if tokenType == "apikey" {
		resolvedCredentialMode = service.CredentialModeAPIKey
	}

	checkinEnabled := false
	if tokenType == "session" {
		checkinEnabled = true
		if body.CheckinEnabled != nil {
			checkinEnabled = *body.CheckinEnabled
		}
	}

	extraConfig := map[string]any{
		"credentialMode": string(resolvedCredentialMode),
	}
	if body.ProxyURL != nil && strings.TrimSpace(*body.ProxyURL) != "" {
		extraConfig["proxyUrl"] = strings.TrimSpace(*body.ProxyURL)
	}
	if resolvedPlatformUserID != nil {
		extraConfig["platformUserId"] = *resolvedPlatformUserID
	}
	// Store tokenExpiresAt for session-mode accounts
	if body.TokenExpiresAt != nil && resolvedCredentialMode == service.CredentialModeSession {
		extraConfig["tokenExpiresAt"] = *body.TokenExpiresAt
	}
	// Preserve skipModelFetch intent for docs / future init queues.
	if skipModelFetch {
		extraConfig["skipModelFetch"] = true
	}

	extraConfigStr := service.MarshalExtraConfig(extraConfig)

	var usernameVal *string
	if resolvedUsername != "" {
		u := resolvedUsername
		usernameVal = &u
	}
	var apiTokenPtr *string
	if apiTokenVal != "" {
		t := apiTokenVal
		apiTokenPtr = &t
	}

	sortOrder, _ := service.GetNextAccountSortOrder(h.db)
	status := "active"
	isPinned := false

	var id int64
	err := h.db.QueryRowx(
		h.db.Rebind(`INSERT INTO accounts (site_id, username, access_token, api_token, status, is_pinned, sort_order,
		 checkin_enabled, extra_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`),
		site.ID, usernameVal, accessTokenVal, apiTokenPtr, status, isPinned, sortOrder,
		checkinEnabled, extraConfigStr, now, now,
	).Scan(&id)
	if err != nil {
		return nil, err
	}

	return &createAccountOutcome{
		ID:               id,
		Username:         usernameVal,
		TokenType:        tokenType,
		ModelCount:       len(verifiedModels),
		APITokenFound:    apiTokenPtr != nil,
		UsernameDetected: !usernameProvided && usernameVal != nil,
	}, nil
}
