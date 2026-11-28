package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *accountTokensHandler) createToken(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountTokenCreatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account token payload.")
		return
	}

	if body.AccountID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid accountId. Expected positive number.")
		return
	}

	// Get account with site
	row, err := service.GetAccountWithSiteByID(h.db.DB, int64(body.AccountID))
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	if service.IsAPIKeyConnection(&row.Account) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support creating account tokens")
		return
	}

	tokenValue := ""
	if body.Token != nil {
		tokenValue = strings.TrimSpace(*body.Token)
	}

	// Local path: token value provided
	if tokenValue != "" {
		result, err := h.createLocalToken(body, row, tokenValue)
		if err != nil {
			slog.Error("Token creation failed", "err", err)
			writeError(w, http.StatusInternalServerError, "Token creation failed")
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}

	// Upstream path: create token on the target site
	if row.Site.Status == "disabled" || strings.TrimSpace(row.Site.Status) == "disabled" {
		writeError(w, http.StatusBadRequest, "site is disabled; cannot create tokens")
		return
	}
	if strings.TrimSpace(row.Account.AccessToken) == "" {
		writeError(w, http.StatusBadRequest, "account has no access token; cannot create site tokens")
		return
	}

	options, optErr := buildCreateAPITokenOptions(body)
	if optErr != nil {
		writeError(w, http.StatusBadRequest, optErr.Error())
		return
	}

	adapter := platform.GetAdapter(row.Site.Platform)
	if adapter == nil {
		writeError(w, http.StatusBadGateway, "unsupported platform; cannot create site tokens: "+row.Site.Platform)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &row.Account, &row.Site)
	platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)

	created, createErr := adapter.CreateAPIToken(
		ctx,
		row.Site.URL,
		row.Account.AccessToken,
		platformUserID,
		options,
		proxyCfg,
	)
	if createErr != nil {
		slog.Warn("Upstream create API token failed", "err", createErr, "account_id", row.Account.ID, "platform", row.Site.Platform)
		writeError(w, http.StatusBadGateway, "site token creation failed: "+createErr.Error())
		return
	}
	if !created {
		writeError(w, http.StatusBadGateway, "site token creation failed (upstream reported failure)")
		return
	}

	syncResult, syncErr := executeAccountTokenSync(ctx, h.db, h.cfg, row)
	if syncErr != nil {
		slog.Warn("Token sync after upstream create failed", "err", syncErr, "account_id", row.Account.ID)
		writeError(w, http.StatusBadGateway, "site token created, but syncing the local token failed: "+syncErr.Error())
		return
	}

	var token *store.AccountToken
	if syncResult != nil {
		if defaultID, ok := syncResult["defaultTokenId"].(int64); ok {
			token, _ = service.GetTokenByID(h.db.DB, defaultID)
		}
	}
	if token == nil {
		token, _ = service.GetDefaultTokenForAccount(h.db.DB, row.Account.ID)
	}
	if token == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"synced":  true,
			"created": syncResult["created"],
			"updated": syncResult["updated"],
			"total":   syncResult["total"],
			"message": "upstream token created and synced",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   tokenToMap(*token, row.Site.Platform),
		"synced":  true,
		"created": syncResult["created"],
		"updated": syncResult["updated"],
		"total":   syncResult["total"],
	})
}

func (h *accountTokensHandler) createLocalToken(body payloads.AccountTokenCreatePayload, row *service.AccountWithSite, tokenValue string) (map[string]any, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	var existingTokens []store.AccountToken
	if err := h.db.Select(&existingTokens, "SELECT * FROM account_tokens WHERE account_id = ?", body.AccountID); err != nil {
		return nil, fmt.Errorf("failed to load existing tokens: %w", err)
	}

	valueStatus := service.TokenValueStatusReady
	if IsMaskedTokenValue(tokenValue) {
		valueStatus = service.TokenValueStatusMaskedPending
	}
	enabled := valueStatus == service.TokenValueStatusReady
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	isDefault := false
	if valueStatus == service.TokenValueStatusReady && body.IsDefault != nil {
		isDefault = *body.IsDefault
	}

	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	if name == "" {
		if len(existingTokens) == 0 {
			name = "default"
		} else {
			name = fmt.Sprintf("token-%d", len(existingTokens)+1)
		}
	}

	tokenGroup := ""
	if body.Group != nil {
		tokenGroup = strings.TrimSpace(*body.Group)
	}

	source := "manual"
	if body.Source != nil {
		source = strings.TrimSpace(*body.Source)
	}

	result, err := h.db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, token_group, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		body.AccountID, name, tokenValue, nullIfEmpty(tokenGroup), valueStatus, source, enabled, isDefault, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	tokenID, err := result.LastInsertId()
	if err != nil {
		// Fallback for Postgres which doesn't support LastInsertId.
		if err := h.db.Get(&tokenID, "SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1", body.AccountID, tokenValue); err != nil {
			return nil, fmt.Errorf("failed to read the new token: %w", err)
		}
	}
	cleanupInsertedToken := func() {
		_, _ = h.db.Exec("DELETE FROM account_tokens WHERE id = ?", tokenID)
	}

	// Set as default if appropriate
	if valueStatus == service.TokenValueStatusReady && (isDefault || (len(existingTokens) == 0 && enabled)) {
		if ok, err := service.SetDefaultToken(h.db.DB, tokenID); err != nil {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token: %w", err)
		} else if !ok {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token")
		}
	} else if valueStatus == service.TokenValueStatusReady && !hasDefaultToken(existingTokens) && enabled {
		if ok, err := service.SetDefaultToken(h.db.DB, tokenID); err != nil {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token: %w", err)
		} else if !ok {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token")
		}
	}

	// Fetch the created token
	var token store.AccountToken
	if err := h.db.Get(&token, "SELECT * FROM account_tokens WHERE id = ?", tokenID); err != nil {
		return nil, fmt.Errorf("failed to read the new token: %w", err)
	}

	return map[string]any{
		"success": true,
		"token":   tokenToMap(token, row.Site.Platform),
	}, nil
}

func buildCreateAPITokenOptions(body payloads.AccountTokenCreatePayload) (*platform.CreateAPITokenOptions, error) {
	unlimitedQuota := true
	if body.UnlimitedQuota != nil {
		unlimitedQuota = *body.UnlimitedQuota
	}

	var remainQuota float64
	if body.RemainQuota != nil {
		parsed, ok := parseFlexibleFloat(body.RemainQuota)
		if !ok {
			return nil, fmt.Errorf("Invalid remainQuota. Expected number.")
		}
		remainQuota = parsed
	} else if !unlimitedQuota {
		return nil, fmt.Errorf("remainQuota is required when unlimitedQuota is false")
	}

	expiredTime := int64(-1)
	if body.ExpiredTime != nil {
		parsed, ok := parseFlexibleEpochSeconds(body.ExpiredTime)
		if !ok {
			return nil, fmt.Errorf("Invalid expiredTime. Expected unix seconds or ISO date string.")
		}
		expiredTime = parsed
	}

	name := ""
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	group := ""
	if body.Group != nil {
		group = strings.TrimSpace(*body.Group)
	}
	allowIPs := ""
	if body.AllowIPs != nil {
		allowIPs = strings.TrimSpace(*body.AllowIPs)
	}
	modelLimits := ""
	if body.ModelLimits != nil {
		modelLimits = strings.TrimSpace(*body.ModelLimits)
	}
	modelLimitsEnabled := false
	if body.ModelLimitsEnabled != nil {
		modelLimitsEnabled = *body.ModelLimitsEnabled
	}

	return &platform.CreateAPITokenOptions{
		Name:               name,
		Group:              group,
		UnlimitedQuota:     unlimitedQuota,
		RemainQuota:        remainQuota,
		ExpiredTime:        expiredTime,
		AllowIPs:           allowIPs,
		ModelLimitsEnabled: modelLimitsEnabled,
		ModelLimits:        modelLimits,
	}, nil
}

func hasDefaultToken(tokens []store.AccountToken) bool {
	for _, t := range tokens {
		if t.IsDefault {
			return true
		}
	}
	return false
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func parseFlexibleEpochSeconds(v any) (int64, bool) {
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	case int:
		return int64(val), true
	case int64:
		return val, true
	case string:
		s := strings.TrimSpace(val)
		if s == "" {
			return 0, false
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return n, true
		}
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t.Unix(), true
		}
		if t, err := time.Parse("2006-01-02", s); err == nil {
			return t.Unix(), true
		}
		return 0, false
	default:
		if n, ok := parseFlexibleFloat(v); ok {
			return int64(n), true
		}
		return 0, false
	}
}
