package admin

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterAccountTokensRoutes registers all /api/account-tokens routes.
func RegisterAccountTokensRoutes(r chi.Router, db *store.DB) {
	RegisterAccountTokensRoutesWithConfig(r, db, nil)
}

// RegisterAccountTokensRoutesWithConfig is like RegisterAccountTokensRoutes but
// accepts optional runtime config for proxy-aware upstream calls.
func RegisterAccountTokensRoutesWithConfig(r chi.Router, db *store.DB, cfg *config.Config) {
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
	db  *store.DB
	cfg *config.Config
}

func (h *accountTokensHandler) listTokens(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.URL.Query().Get("accountId")
	var accountID *int64
	if accountIDStr != "" {
		if id, err := strconv.ParseInt(accountIDStr, 10, 64); err == nil {
			accountID = &id
		}
	}

	tokens, err := service.ListTokensWithRelations(h.db.DB, accountID)
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

func (h *accountTokensHandler) setDefault(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db.DB, token.AccountID)
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

	defaultSet, err := service.SetDefaultToken(h.db.DB, tokenID)
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

func (h *accountTokensHandler) getTokenValue(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db.DB, token.AccountID)
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
	if err := h.db.Get(&account, "SELECT * FROM accounts WHERE id = ?", token.AccountID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the account")
		return
	}
	if err := h.db.Get(&site, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", account.SiteID); err != nil {
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

func (h *accountTokensHandler) getGroups(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db.DB, accountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	if service.IsAPIKeyConnection(&row.Account) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support fetching account token groups")
		return
	}

	accessToken := strings.TrimSpace(row.Account.AccessToken)
	adapter := platform.GetAdapter(row.Site.Platform)
	proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &row.Account, &row.Site)
	platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	groups, err := service.GetTokenGroups(
		ctx,
		h.db.DB,
		accountID,
		adapter,
		row.Site.URL,
		accessToken,
		platformUserID,
		proxyCfg,
	)
	if err != nil {
		slog.Error("Failed to load token groups", "err", err, "account_id", accountID)
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load token groups")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"groups":  groups,
	})
}

func (h *accountTokensHandler) getAccountDefault(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	var account store.Account
	if err := h.db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	if service.IsAPIKeyConnection(&account) {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	token, err := service.GetDefaultTokenForAccount(h.db.DB, accountID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "token": nil})
		return
	}

	var site store.Site
	if err := h.db.Get(&site, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", account.SiteID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the site")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   tokenToMap(*token, site.Platform),
	})
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
