package admin

import (
	"context"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

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

// credentialVerificationFailureMessage explains what a session-credential bind
// needs when VerifyToken returns nil / empty / unknown. The copy is platform
// specific where the upstream contract differs (AnyRouter is cookie/session-only
// and rejects standard access tokens); otherwise it keeps the pre-existing
// generic guidance plus a minimal shape hint.
func credentialVerificationFailureMessage(platformName string) string {
	if strings.EqualFold(platformName, "anyrouter") {
		return "token verification failed: AnyRouter does not support Access Token/API key binding. Use session mode and paste session=<value> or the full Cookie: header into Access Token / Cookie. Platform User ID is required only if the site enforces New-Api-User/User-id headers. Click \"Verify Token\" first, then bind the account only after verification returns tokenType=session."
	}
	return "token verification failed; click \"Verify Token\" first, then bind the account after verification succeeds. For session platforms paste session=<value> or the full Cookie: header; for API-key platforms use API Key mode."
}

// createAccountOutcome holds metadata from a successful createSingleAccount call.
type createAccountOutcome struct {
	ID               int64
	Username         *string
	TokenType        string
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
				Message:              credentialVerificationFailureMessage(site.Platform),
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

	sortOrder, _ := service.GetNextAccountSortOrder(h.db.DB)
	status := "active"
	isPinned := false

	var id int64
	err := h.db.QueryRowx(
		`INSERT INTO accounts (site_id, username, access_token, api_token, status, is_pinned, sort_order,
		 checkin_enabled, extra_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`,
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
		UsernameDetected: !usernameProvided && usernameVal != nil,
	}, nil
}
