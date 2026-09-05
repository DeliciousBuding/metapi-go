package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterAccountTokensRoutes registers all /api/account-tokens routes.
func RegisterAccountTokensRoutes(r chi.Router, db *sqlx.DB) {
	RegisterAccountTokensRoutesWithConfig(r, db, nil)
}

// RegisterAccountTokensRoutesWithConfig is like RegisterAccountTokensRoutes but
// accepts optional runtime config for proxy-aware upstream calls.
func RegisterAccountTokensRoutesWithConfig(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &accountTokensHandler{db: db, cfg: cfg}

	r.Get("/api/account-tokens", handler.listTokens)
	r.Post("/api/account-tokens", handler.createToken)
	r.Post("/api/account-tokens/batch", handler.batchTokens)
	r.Put("/api/account-tokens/{id}", handler.updateToken)
	r.Post("/api/account-tokens/{id}/default", handler.setDefault)
	r.Get("/api/account-tokens/{id}/value", handler.getTokenValue)
	r.Delete("/api/account-tokens/{id}", handler.deleteToken)
	r.Post("/api/account-tokens/sync/{accountId}", handler.syncAccount)
	r.Post("/api/account-tokens/sync-all", handler.syncAll)
	r.Get("/api/account-tokens/groups/{accountId}", handler.getGroups)
	r.Get("/api/account-tokens/account/{accountId}/default", handler.getAccountDefault)
}

type accountTokensHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

// ---- List Tokens ----

func (h *accountTokensHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.URL.Query().Get("accountId")
	var accountID *int64
	if accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = &id
		}
	}

	tokens, err := service.ListTokensWithRelations(h.db, accountID)
	if err != nil {
		slog.Error("Failed to load tokens", "err", err)
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load tokens")
		return
	}

	if tokens == nil {
		tokens = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

// ---- Create Token ----

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
	row, err := service.GetAccountWithSiteByID(h.db, int64(body.AccountID))
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
			token, _ = service.GetTokenByID(h.db, defaultID)
		}
	}
	if token == nil {
		token, _ = service.GetDefaultTokenForAccount(h.db, row.Account.ID)
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
	if err := h.db.Select(&existingTokens, h.db.Rebind("SELECT * FROM account_tokens WHERE account_id = ?"), body.AccountID); err != nil {
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
		h.db.Rebind(`INSERT INTO account_tokens (account_id, name, token, token_group, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		body.AccountID, name, tokenValue, nullIfEmpty(tokenGroup), valueStatus, source, enabled, isDefault, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	tokenID, err := result.LastInsertId()
	if err != nil {
		// Fallback for Postgres which doesn't support LastInsertId.
		if err := h.db.Get(&tokenID, h.db.Rebind("SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1"), body.AccountID, tokenValue); err != nil {
			return nil, fmt.Errorf("failed to read the new token: %w", err)
		}
	}
	cleanupInsertedToken := func() {
		_, _ = h.db.Exec(h.db.Rebind("DELETE FROM account_tokens WHERE id = ?"), tokenID)
	}

	// Set as default if appropriate
	if valueStatus == service.TokenValueStatusReady && (isDefault || (len(existingTokens) == 0 && enabled)) {
		if ok, err := service.SetDefaultToken(h.db, tokenID); err != nil {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token: %w", err)
		} else if !ok {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token")
		}
	} else if valueStatus == service.TokenValueStatusReady && !hasDefaultToken(existingTokens) && enabled {
		if ok, err := service.SetDefaultToken(h.db, tokenID); err != nil {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token: %w", err)
		} else if !ok {
			cleanupInsertedToken()
			return nil, fmt.Errorf("failed to set default token")
		}
	}

	// Fetch the created token
	var token store.AccountToken
	if err := h.db.Get(&token, h.db.Rebind("SELECT * FROM account_tokens WHERE id = ?"), tokenID); err != nil {
		return nil, fmt.Errorf("failed to read the new token: %w", err)
	}

	return map[string]any{
		"success": true,
		"token":   tokenToMap(token, row.Site.Platform),
	}, nil
}

// ---- Batch Tokens ----

func (h *accountTokensHandler) batchTokens(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountTokenBatchPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account token batch payload.")
		return
	}

	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{"enable": true, "disable": true, "delete": true}
	if !validActions[action] {
		writeError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	var successIDs []int64
	var failedItems []map[string]any

	for _, rawID := range body.IDs {
		id := int64(rawID)

		existing, err := service.GetTokenByID(h.db, id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Token not found"})
			continue
		}

		owner, ownerErr := service.GetAccountByID(h.db, existing.AccountID)
		if ownerErr != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}
		if owner == nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}
		if service.IsAPIKeyConnection(owner) {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "API key connections do not support managing account tokens"})
			continue
		}

		if action == "delete" {
			if err := h.deleteTokenWithUpstream(r.Context(), existing); err != nil {
				slog.Error("Token deletion failed", "err", err, "token_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": err.Error()})
				continue
			}
		} else {
			if service.IsMaskedPendingAccountToken(existing) {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "pending tokens cannot change their enabled state; complete the plaintext token first"})
				continue
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := h.db.Exec(h.db.Rebind("UPDATE account_tokens SET enabled = ?, updated_at = ? WHERE id = ?"), action == "enable", now, id); err != nil {
				slog.Error("Token status update failed", "err", err, "token_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Token status update failed"})
				continue
			}

			if existing.IsDefault && action == "disable" {
				if _, err := service.RepairDefaultToken(h.db, existing.AccountID); err != nil {
					slog.Error("Default token repair failed", "err", err, "account_id", existing.AccountID)
					failedItems = append(failedItems, map[string]any{"id": id, "message": "Default token repair failed"})
					continue
				}
			}
		}
		successIDs = append(successIDs, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}

// ---- Update Token ----

func (h *accountTokensHandler) updateToken(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountTokenUpdatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account token payload.")
		return
	}

	existing, err := service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db, existing.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if service.IsAPIKeyConnection(owner) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}

	updates := map[string]any{}

	nextValueStatus := service.ResolveAccountTokenValueStatus(existing)

	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Token != nil {
		tv := strings.TrimSpace(*body.Token)
		if tv == "" {
			writeError(w, http.StatusBadRequest, "token must not be empty")
			return
		}
		updates["token"] = tv
		if IsMaskedTokenValue(tv) {
			nextValueStatus = service.TokenValueStatusMaskedPending
		} else {
			nextValueStatus = service.TokenValueStatusReady
		}
		updates["valueStatus"] = nextValueStatus
	}
	if body.Group != nil {
		updates["tokenGroup"] = nullIfEmpty(strings.TrimSpace(*body.Group))
	}
	if body.Source != nil {
		updates["source"] = strings.TrimSpace(*body.Source)
	}

	if nextValueStatus == service.TokenValueStatusMaskedPending {
		updates["enabled"] = false
		updates["isDefault"] = false
	} else {
		if body.Enabled != nil {
			updates["enabled"] = *body.Enabled
		}
		if body.IsDefault != nil {
			updates["isDefault"] = *body.IsDefault
		}
	}

	if err := service.UpdateTokenFields(h.db, tokenID, updates); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Refresh and handle default logic
	latest, err := service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if latest == nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	if body.IsDefault != nil && *body.IsDefault && service.IsUsableAccountToken(latest) {
		if ok, err := service.SetDefaultToken(h.db, tokenID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to set default token")
			return
		} else if !ok {
			writeError(w, http.StatusBadRequest, "token cannot be set as default")
			return
		}
	} else if latest.IsDefault && service.IsUsableAccountToken(latest) {
		if ok, err := service.SetDefaultToken(h.db, tokenID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to set default token")
			return
		} else if !ok {
			writeError(w, http.StatusBadRequest, "token cannot be set as default")
			return
		}
	} else if existing.IsDefault && !service.IsUsableAccountToken(latest) {
		if _, err := service.RepairDefaultToken(h.db, existing.AccountID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to repair the default token")
			return
		}
	} else if body.IsDefault != nil && !(*body.IsDefault) && existing.IsDefault {
		if _, err := service.RepairDefaultToken(h.db, existing.AccountID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to repair the default token")
			return
		}
	}

	latest, err = service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read tokens")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   latest,
	})
}

// ---- Set Default ----

func (h *accountTokensHandler) setDefault(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db, token.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if service.IsAPIKeyConnection(owner) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}
	if service.IsMaskedPendingAccountToken(token) {
		writeError(w, http.StatusBadRequest, "pending tokens cannot be set as default; complete the plaintext token first")
		return
	}

	defaultSet, err := service.SetDefaultToken(h.db, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set default token")
		return
	}
	if !defaultSet {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// ---- Get Token Value ----

func (h *accountTokensHandler) getTokenValue(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db, token.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if service.IsAPIKeyConnection(owner) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}

	if service.IsMaskedPendingAccountToken(token) || IsMaskedTokenValue(token.Token) {
		writeError(w, http.StatusConflict, "only a masked token is stored, so it cannot be revealed or copied; regenerate and sync it on the site, or update it manually with the full token")
		return
	}

	// Get site platform
	var site store.Site
	var account store.Account
	if err := h.db.Get(&account, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), token.AccountID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the account")
		return
	}
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), account.SiteID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the site")
		return
	}

	displayToken := service.NormalizeTokenForDisplay(token.Token, site.Platform)
	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"id":          token.ID,
		"name":        token.Name,
		"token":       displayToken,
		"tokenMasked": service.MaskToken(token.Token, site.Platform),
	})
}

// ---- Delete Token ----

func (h *accountTokensHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db, token.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if service.IsAPIKeyConnection(owner) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}

	if err := h.deleteTokenWithUpstream(r.Context(), token); err != nil {
		slog.Error("Token deletion failed", "err", err, "token_id", tokenID)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *accountTokensHandler) deleteTokenWithUpstream(ctx context.Context, token *store.AccountToken) error {
	if token == nil {
		return fmt.Errorf("token not found")
	}

	// Known limitation: masked_pending tokens only store redacted key material, so remote
	// delete cannot match a real upstream key and is intentionally skipped.
	shouldSkipUpstream := service.IsMaskedPendingAccountToken(token) || IsMaskedTokenValue(token.Token)

	row, err := service.GetAccountWithSiteByID(h.db, token.AccountID)
	if err != nil {
		return service.DeleteTokenByID(h.db, token.ID)
	}

	if !shouldSkipUpstream {
		siteDisabled := strings.EqualFold(strings.TrimSpace(row.Site.Status), "disabled")
		accessToken := strings.TrimSpace(row.Account.AccessToken)
		adapter := platform.GetAdapter(row.Site.Platform)

		if !siteDisabled && accessToken != "" && adapter != nil {
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &row.Account, &row.Site)
			platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)
			if delErr := adapter.DeleteAPIToken(callCtx, row.Site.URL, accessToken, token.Token, platformUserID, proxyCfg); delErr != nil {
				return fmt.Errorf("failed to delete the upstream token: %w", delErr)
			}
		}
	}

	return service.DeleteTokenByID(h.db, token.ID)
}

// ---- Sync Account ----

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

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func hasDefaultToken(tokens []store.AccountToken) bool {
	for _, t := range tokens {
		if t.IsDefault {
			return true
		}
	}
	return false
}

func tokenToMap(token store.AccountToken, platform string) map[string]any {
	vs := service.ResolveAccountTokenValueStatus(&token)
	return map[string]any{
		"id":          token.ID,
		"accountId":   token.AccountID,
		"name":        token.Name,
		"tokenMasked": service.MaskToken(token.Token, platform),
		"tokenGroup":  token.TokenGroup,
		"valueStatus": vs,
		"source":      token.Source,
		"enabled":     token.Enabled,
		"isDefault":   token.IsDefault,
		"createdAt":   token.CreatedAt,
		"updatedAt":   token.UpdatedAt,
	}
}

func tokenToMapMasked(token store.AccountToken, platform string) map[string]any {
	return map[string]any{
		"id":          token.ID,
		"accountId":   token.AccountID,
		"name":        token.Name,
		"tokenGroup":  token.TokenGroup,
		"valueStatus": service.ResolveAccountTokenValueStatus(&token),
		"source":      token.Source,
		"enabled":     token.Enabled,
		"isDefault":   token.IsDefault,
		"tokenMasked": service.MaskToken(token.Token, platform),
		"createdAt":   token.CreatedAt,
		"updatedAt":   token.UpdatedAt,
	}
}
