package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	balanceService "github.com/deliciousbuding/metapi-go/service/balance"
	dailyservice "github.com/deliciousbuding/metapi-go/service/daily"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterAccountsRoutes registers all /api/accounts routes.
func RegisterAccountsRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &accountsHandler{db: db, cfg: cfg}

	r.Get("/api/accounts", handler.listAccounts)
	r.Post("/api/accounts", handler.createAccount)
	r.Post("/api/accounts/login", handler.loginAccount)
	r.Post("/api/accounts/verify-token", handler.verifyToken)
	r.Post("/api/accounts/{id}/rebind-session", handler.rebindSession)
	r.Put("/api/accounts/{id}", handler.updateAccount)
	r.Delete("/api/accounts/{id}", handler.deleteAccount)
	r.Post("/api/accounts/batch", handler.batchAccounts)
	r.Post("/api/accounts/health/refresh", handler.healthRefresh)
	r.Post("/api/accounts/{id}/balance", handler.refreshBalance)
	r.Get("/api/accounts/{id}/models", handler.getAccountModels)
	r.Post("/api/accounts/{id}/models/manual", handler.manualModels)
}

type accountsHandler struct {
	db  *sqlx.DB
	cfg *config.Config
}

// accountsSnapshotCache is an in-memory TTL cache for GET /api/accounts responses.
// Mirrors TS getAccountsSnapshot() behavior: cached response, ?refresh=true bypasses.
type accountsSnapshotCache struct {
	mu        sync.RWMutex
	data      []byte
	expiresAt time.Time
	ttl       time.Duration
}

func (c *accountsSnapshotCache) get() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && time.Now().Before(c.expiresAt) {
		return c.data, true
	}
	return nil, false
}

func (c *accountsSnapshotCache) set(data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.expiresAt = time.Now().Add(c.ttl)
}

func (c *accountsSnapshotCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = nil
	c.expiresAt = time.Time{}
}

var globalAccountsCache = &accountsSnapshotCache{ttl: 30 * time.Second}

func init() {
	// Site mutations invalidate process-local admin list cache via service hook.
	service.RegisterSiteProxyCacheInvalidator(func() {
		globalAccountsCache.clear()
	})
}

// ---- List Accounts ----

func (h *accountsHandler) listAccounts(w http.ResponseWriter, r *http.Request) {
	refresh := strings.TrimSpace(r.URL.Query().Get("refresh"))
	forceRefresh := refresh == "true" || refresh == "1"

	// Check snapshot cache
	if !forceRefresh {
		if cached, hit := globalAccountsCache.get(); hit {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-accounts-snapshot-cache", "hit")
			w.WriteHeader(http.StatusOK)
			w.Write(cached)
			return
		}
	}

	accounts, err := service.ListAccountsWithSites(h.db)
	if err != nil {
		slog.Error("Failed to load accounts", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load accounts"})
		return
	}

	// Per-account today truth. Failure degrades to "no metrics" (frontend shows
	// — instead of fake zeros) and is logged; the account list itself must not
	// fail because an auxiliary aggregation query broke.
	todayMetrics, metricsErr := dailyservice.CollectPerAccountTodayMetrics(h.db, time.Now())
	if metricsErr != nil {
		slog.Error("Failed to load per-account today metrics", "err", metricsErr)
	} else {
		for _, account := range accounts {
			accountID := coerceInt64(account["id"])
			if accountID <= 0 {
				continue
			}
			if m, exists := todayMetrics[accountID]; exists {
				account["todayReward"] = m.Reward
				account["todayRewardStatus"] = m.RewardStatus
				account["todayRewardReason"] = m.RewardReason
				account["todaySpend"] = m.Spend
				account["todaySpendStatus"] = m.SpendStatus
				account["todaySpendReason"] = m.SpendReason
				account["todayTokens"] = m.Tokens
				account["todayProxy"] = map[string]any{
					"total":   m.ProxyTotal,
					"success": m.ProxySuccess,
					"failed":  m.ProxyFailed,
					"unknown": m.ProxyUnknown,
				}
			} else {
				// Real zero, not missing: account had no rows within the local day.
				account["todayReward"] = 0.0
				account["todayRewardStatus"] = "complete"
				account["todaySpend"] = 0.0
				account["todaySpendStatus"] = "complete"
				account["todayTokens"] = int64(0)
				account["todayProxy"] = map[string]any{
					"total":   0,
					"success": 0,
					"failed":  0,
					"unknown": 0,
				}
			}
		}
	}

	// Also fetch sites for the response
	var sites []store.Site
	h.db.Select(&sites, "SELECT "+service.SiteSelectColumns+" FROM sites ORDER BY sort_order, id")

	resp := map[string]any{
		"generatedAt": time.Now().UTC().Format(time.RFC3339),
		"accounts":    accounts,
		"sites":       sites,
	}
	respBytes, _ := json.Marshal(resp)
	globalAccountsCache.set(respBytes)

	w.Header().Set("x-accounts-snapshot-cache", "miss")
	writeJSON(w, http.StatusOK, resp)
}

// ---- Create Account ----

func (h *accountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountCreatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account payload.")
		return
	}

	if body.SiteID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}

	// Check site exists
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeError(w, http.StatusBadRequest, "site not found")
		return
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
		writeError(w, http.StatusBadRequest, "请填写 Token")
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
				"message":      "批量添加失败（0/" + strconv.Itoa(len(requestedTokens)) + "）",
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
			"message":      "批量添加完成：成功 " + strconv.Itoa(createdCount) + "，失败 " + strconv.Itoa(len(requestedTokens)-createdCount),
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
		return nil, &accountCreateError{Message: "请填写 Token", RequiresVerification: false}
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return nil, &accountCreateError{
			Message:              "platform not supported: " + site.Platform,
			RequiresVerification: false,
		}
	}

	skipModelFetch := body.SkipModelFetch != nil && *body.SkipModelFetch
	proxyCfg := service.BuildPlatformProxyConfig(h.cfg, nil, &site)

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
					Message:              "API Key 验证失败：" + err.Error(),
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
					Message:              "API Key 验证失败：未获取到可用模型",
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
				Message:              "Token 验证失败，请先点击“验证 Token”，验证成功后再绑定账号",
				RequiresVerification: true,
			}
		}
		tokenType = result.TokenType

		if credentialMode == service.CredentialModeSession && tokenType != "session" {
			return nil, &accountCreateError{
				Message:              "当前凭证是 API Key，请切换到 API Key 模式，或改用 Session Token",
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

// ---- Login Account ----

func (h *accountsHandler) loginAccount(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountLoginPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid login payload.")
		return
	}

	if body.SiteID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}
	if strings.TrimSpace(body.Username) == "" {
		writeError(w, http.StatusBadRequest, "Invalid username. Expected string.")
		return
	}
	if strings.TrimSpace(body.Password) == "" {
		writeError(w, http.StatusBadRequest, "Invalid password. Expected string.")
		return
	}

	// Get site
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "unsupported platform: " + site.Platform})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	loginResult, err := adp.Login(ctx, site.URL, body.Username, body.Password, nil, service.BuildPlatformProxyConfig(h.cfg, nil, &site))
	if err != nil {
		slog.Warn("Account login failed", "err", err, "site_id", site.ID, "platform", site.Platform)
		writeError(w, http.StatusUnauthorized, "login failed")
		return
	}
	if loginResult == nil || !loginResult.Success || strings.TrimSpace(loginResult.AccessToken) == "" {
		message := "login failed"
		if loginResult != nil && strings.TrimSpace(loginResult.Message) != "" {
			message = strings.TrimSpace(loginResult.Message)
		}
		writeError(w, http.StatusUnauthorized, message)
		return
	}
	loginAccessToken := strings.TrimSpace(loginResult.AccessToken)

	// Check for existing account (reusedAccount)
	var existing store.Account
	reused := false
	err = h.db.Get(&existing,
		h.db.Rebind("SELECT * FROM accounts WHERE site_id = ? AND username = ?"),
		body.SiteID, body.Username,
	)
	if err == nil {
		reused = true
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Encrypt password for autoRelogin
	passwordCipher, encErr := service.EncryptPassword(h.cfg, body.Password)
	if encErr != nil {
		writeError(w, http.StatusInternalServerError, "Failed to encrypt password.")
		return
	}

	extraConfig := map[string]any{
		"credentialMode": "session",
		"autoRelogin": map[string]any{
			"username":       body.Username,
			"passwordCipher": passwordCipher,
			"updatedAt":      now,
		},
	}

	extraConfigStr := service.MarshalExtraConfig(extraConfig)

	if reused {
		// Update existing account
		if _, err := h.db.Exec(
			h.db.Rebind(`UPDATE accounts SET access_token = ?, checkin_enabled = ?, status = 'active',
			 extra_config = ?, updated_at = ? WHERE id = ?`),
			loginAccessToken, true, extraConfigStr, now, existing.ID,
		); err != nil {
			slog.Error("Failed to update login account", "err", err, "account_id", existing.ID)
			writeError(w, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	} else {
		sortOrder, _ := service.GetNextAccountSortOrder(h.db)
		if _, err := h.db.Exec(
			h.db.Rebind(`INSERT INTO accounts (site_id, username, access_token, checkin_enabled, status,
			 is_pinned, sort_order, extra_config, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`),
			body.SiteID, body.Username, loginAccessToken, true, false, sortOrder, extraConfigStr, now, now,
		); err != nil {
			slog.Error("Failed to insert login account", "err", err, "site_id", body.SiteID)
			writeError(w, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	}

	// Fetch the created/updated account for response
	var loginAcct store.Account
	if err := h.db.Get(&loginAcct, h.db.Rebind("SELECT * FROM accounts WHERE site_id = ? AND username = ?"), body.SiteID, body.Username); err != nil {
		slog.Error("Failed to load login account", "err", err, "site_id", body.SiteID)
		writeError(w, http.StatusInternalServerError, "Failed to load account.")
		return
	}
	loginAcctMap := map[string]any{
		"id":             loginAcct.ID,
		"siteId":         loginAcct.SiteID,
		"username":       loginAcct.Username,
		"accessToken":    loginAcct.AccessToken,
		"apiToken":       loginAcct.APIToken,
		"balance":        loginAcct.Balance,
		"status":         loginAcct.Status,
		"isPinned":       loginAcct.IsPinned,
		"sortOrder":      loginAcct.SortOrder,
		"checkinEnabled": loginAcct.CheckinEnabled,
		"extraConfig":    loginAcct.ExtraConfig,
		"createdAt":      loginAcct.CreatedAt,
		"updatedAt":      loginAcct.UpdatedAt,
	}
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"account":       loginAcctMap,
		"apiTokenFound": loginAcct.APIToken != nil,
		"tokenCount":    1,
		"reusedAccount": reused,
	})
}

// ---- Verify Token ----

func (h *accountsHandler) verifyToken(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountVerifyTokenPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid verify-token payload.")
		return
	}

	if body.SiteID <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}

	accessToken := ""
	if body.AccessToken != nil {
		accessToken = strings.TrimSpace(*body.AccessToken)
	}
	if accessToken == "" {
		writeError(w, http.StatusBadRequest, "Token 不能为空")
		return
	}

	// Get site
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeError(w, http.StatusNotFound, "site not found")
		return
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": "unsupported platform: " + site.Platform})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := adp.VerifyToken(ctx, site.URL, accessToken, body.PlatformUserID, service.BuildPlatformProxyConfig(h.cfg, nil, &site))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"success": false, "message": err.Error()})
		return
	}
	if result == nil || result.TokenType == "" || result.TokenType == "unknown" {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":   false,
			"tokenType": "unknown",
			"message":   "token verification failed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"tokenType":     result.TokenType,
		"modelCount":    len(result.Models),
		"models":        result.Models,
		"userInfo":      result.UserInfo,
		"balance":       result.Balance,
		"apiToken":      result.APIToken,
		"apiTokenFound": result.APIToken != "",
	})
}

func shouldMirrorAPIKeyToken(account store.Account, requestedAPIToken any) bool {
	if service.ResolveStoredCredentialMode(&account) != service.CredentialModeAPIKey {
		return false
	}
	if requestedAPIToken == nil {
		return true
	}
	requested, ok := requestedAPIToken.(string)
	if !ok {
		return false
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	current := ""
	if account.APIToken != nil {
		current = strings.TrimSpace(*account.APIToken)
	}
	return requested == current
}

func normalizeAPITokenUpdate(value any) any {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return value
}

func normalizeAccountStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "disabled", "expired":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

// shouldRecoverExpiredAPIKey detects the expired API-key credential recovery path:
// previous status expired, next mode apikey, credentials changed, next status not forced disabled.
func shouldRecoverExpiredAPIKey(prev, next store.Account, updates map[string]any) bool {
	if !strings.EqualFold(strings.TrimSpace(prev.Status), "expired") {
		return false
	}
	nextStatus := strings.TrimSpace(prev.Status)
	if status, ok := updates["status"].(string); ok && strings.TrimSpace(status) != "" {
		nextStatus = strings.TrimSpace(status)
	}
	if strings.EqualFold(nextStatus, "disabled") {
		return false
	}
	mode := service.ResolveStoredCredentialMode(&next)
	if mode != service.CredentialModeAPIKey {
		return false
	}
	return credentialFieldsChanged(prev, updates)
}

func credentialFieldsChanged(prev store.Account, updates map[string]any) bool {
	if accessToken, ok := updates["accessToken"].(string); ok {
		if strings.TrimSpace(accessToken) != strings.TrimSpace(prev.AccessToken) {
			return true
		}
	}
	if apiTokenRaw, ok := updates["apiToken"]; ok {
		prevAPI := ""
		if prev.APIToken != nil {
			prevAPI = strings.TrimSpace(*prev.APIToken)
		}
		switch v := apiTokenRaw.(type) {
		case string:
			if strings.TrimSpace(v) != prevAPI {
				return true
			}
		case *string:
			nextAPI := ""
			if v != nil {
				nextAPI = strings.TrimSpace(*v)
			}
			if nextAPI != prevAPI {
				return true
			}
		default:
			if v != nil {
				return true
			}
		}
	}
	return false
}

// clearAccountAuthRuntimeHealth removes auth-sourced runtimeHealth best-effort after recovery.
func clearAccountAuthRuntimeHealth(db *sqlx.DB, accountID int64) error {
	var extraConfig *string
	if err := db.Get(&extraConfig, db.Rebind("SELECT extra_config FROM accounts WHERE id = ?"), accountID); err != nil {
		return err
	}
	cfg := service.ParseExtraConfig(extraConfig)
	if cfg == nil {
		return nil
	}
	healthRaw, ok := cfg["runtimeHealth"]
	if !ok || healthRaw == nil {
		return nil
	}
	b, err := json.Marshal(healthRaw)
	if err != nil {
		return nil
	}
	var entry service.RuntimeHealthEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(string(entry.Source))) != string(service.HealthSourceAuth) {
		return nil
	}
	merged := service.MergeExtraConfig(extraConfig, map[string]any{"runtimeHealth": nil})
	now := time.Now().UTC().Format(time.RFC3339)
	if merged == nil {
		_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = NULL, updated_at = ? WHERE id = ?"), now, accountID)
		return err
	}
	_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?"), *merged, now, accountID)
	return err
}

// ---- Rebind Session ----

func (h *accountsHandler) rebindSession(w http.ResponseWriter, r *http.Request) {
	accountID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountRebindSessionPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid rebind payload.")
		return
	}

	nextAccessToken := ""
	if body.AccessToken != nil {
		nextAccessToken = strings.TrimSpace(*body.AccessToken)
	}
	if nextAccessToken == "" {
		writeError(w, http.StatusBadRequest, "请提供新的 Session Token")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, accountID)
	if err != nil {
		writeError(w, http.StatusNotFound, "账号不存在")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Sub2API managed auth: merge refreshToken/tokenExpiresAt into extraConfig.sub2apiAuth
	// without clobbering unrelated keys (spec p3-sites-accounts lines 528-542).
	extraConfigPatch := map[string]any{
		"credentialMode": "session",
	}
	if service.IsSub2ApiPlatform(row.Site.Platform) {
		if mergedAuth := service.BuildMergedSub2ApiAuth(row.Account.ExtraConfig, body.RefreshToken, body.TokenExpiresAt, nil); mergedAuth != nil {
			extraConfigPatch["sub2apiAuth"] = mergedAuth
		}
	}
	extraConfigStr := service.MergeExtraConfig(row.Account.ExtraConfig, extraConfigPatch)

	if _, err := h.db.Exec(
		h.db.Rebind("UPDATE accounts SET access_token = ?, status = 'active', extra_config = ?, updated_at = ? WHERE id = ?"),
		nextAccessToken, extraConfigStr, now, accountID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update account")
		return
	}

	// Fetch updated account for response
	var rebindAcct store.Account
	if err := h.db.Get(&rebindAcct, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), accountID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read updated account")
		return
	}
	rebindAcctMap := map[string]any{
		"id":             rebindAcct.ID,
		"siteId":         rebindAcct.SiteID,
		"username":       rebindAcct.Username,
		"accessToken":    rebindAcct.AccessToken,
		"apiToken":       rebindAcct.APIToken,
		"balance":        rebindAcct.Balance,
		"status":         rebindAcct.Status,
		"isPinned":       rebindAcct.IsPinned,
		"sortOrder":      rebindAcct.SortOrder,
		"checkinEnabled": rebindAcct.CheckinEnabled,
		"extraConfig":    rebindAcct.ExtraConfig,
		"createdAt":      rebindAcct.CreatedAt,
		"updatedAt":      rebindAcct.UpdatedAt,
	}
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"account":        rebindAcctMap,
		"tokenType":      "session",
		"credentialMode": "session",
		"capabilities":   service.BuildCapabilitiesFromCredentialMode(service.CredentialModeSession, true),
		"apiTokenFound":  false,
	})
}

// ---- Update Account ----

func (h *accountsHandler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountUpdatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account payload."})
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found"})
		return
	}

	updates := map[string]any{}
	pendingExtraConfig := row.Account.ExtraConfig
	mergeExtraConfigUpdate := func(patch map[string]any) {
		merged := service.MergeExtraConfig(pendingExtraConfig, patch)
		if merged != nil {
			pendingExtraConfig = merged
			updates["extraConfig"] = *merged
		}
	}
	if body.Username != nil {
		updates["username"] = *body.Username
	}
	mirrorAPIKeyToken := false
	if body.AccessToken != nil {
		nextAccessToken := strings.TrimSpace(*body.AccessToken)
		updates["accessToken"] = nextAccessToken
		mirrorAPIKeyToken = shouldMirrorAPIKeyToken(row.Account, body.APIToken)
		if mirrorAPIKeyToken {
			updates["apiToken"] = nextAccessToken
		}
	}
	if body.APIToken != nil && !mirrorAPIKeyToken {
		updates["apiToken"] = normalizeAPITokenUpdate(body.APIToken)
	}
	if body.Status != nil {
		status := normalizeAccountStatus(*body.Status)
		if status == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account status. Expected active, disabled, or expired."})
			return
		}
		updates["status"] = status
	}
	if body.UnitCost != nil {
		if *body.UnitCost <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid unitCost value. Expected positive number."})
			return
		}
		updates["unitCost"] = *body.UnitCost
	}
	if body.ExtraConfig != nil {
		// Merge with existing extraConfig to preserve keys not in the update.
		// For sub2api, strip nested sub2apiAuth here and merge it via managed-auth
		// helpers later so partial patches cannot clobber unrelated auth keys.
		if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
			if service.IsSub2ApiPlatform(row.Site.Platform) {
				stripped := make(map[string]any, len(ecMap))
				for k, v := range ecMap {
					if k == "sub2apiAuth" {
						continue
					}
					stripped[k] = v
				}
				if len(stripped) > 0 {
					mergeExtraConfigUpdate(stripped)
				}
			} else {
				mergeExtraConfigUpdate(ecMap)
			}
		}
	}
	nextAccount := row.Account
	nextAccount.ExtraConfig = pendingExtraConfig
	if accessToken, ok := updates["accessToken"].(string); ok {
		nextAccount.AccessToken = accessToken
	}
	if apiToken, ok := updates["apiToken"].(*string); ok {
		nextAccount.APIToken = apiToken
	} else if apiTokenStr, ok := updates["apiToken"].(string); ok {
		token := apiTokenStr
		nextAccount.APIToken = &token
	}
	if status, ok := updates["status"].(string); ok {
		nextAccount.Status = status
	}
	if service.BuildCapabilitiesForAccount(&nextAccount).CanCheckin {
		if body.CheckinEnabled != nil {
			updates["checkinEnabled"] = *body.CheckinEnabled
		}
	} else if row.Account.CheckinEnabled || body.CheckinEnabled != nil {
		updates["checkinEnabled"] = false
	}
	if body.IsPinned != nil {
		updates["isPinned"] = *body.IsPinned
	}
	if body.SortOrder != nil {
		so := service.NormalizeSortOrder(body.SortOrder)
		if so == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid sortOrder value. Expected non-negative integer."})
			return
		}
		updates["sortOrder"] = int64(*so)
	}
	if body.ProxyURL != nil {
		mergeExtraConfigUpdate(map[string]any{
			"proxyUrl": service.NormalizeNullable(body.ProxyURL),
		})
	}
	// Sub2API managed auth: merge top-level / nested refreshToken+tokenExpiresAt
	// into extraConfig.sub2apiAuth without clobbering unrelated keys.
	if service.IsSub2ApiPlatform(row.Site.Platform) {
		var extraPatch map[string]any
		if body.ExtraConfig != nil {
			if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
				extraPatch = ecMap
			}
		}
		// Merge against the original stored config so nested extraConfig.sub2apiAuth
		// patches compose with top-level fields instead of overwriting wholesale.
		if mergedAuth := service.BuildMergedSub2ApiAuth(row.Account.ExtraConfig, body.RefreshToken, body.TokenExpiresAt, extraPatch); mergedAuth != nil {
			mergeExtraConfigUpdate(map[string]any{"sub2apiAuth": mergedAuth})
		}
	}
	// Expired API-key recovery (p3-sites-accounts.md 594-602 / TS accounts.ts):
	// when credentials change on an expired apikey account and status is not forced
	// disabled, refresh models with allowInactive and reactivate only on success.
	recovery := shouldRecoverExpiredAPIKey(row.Account, nextAccount, updates)
	if recovery {
		// preserveExpiredStatus: do not activate until model refresh succeeds.
		if status, ok := updates["status"].(string); ok && status == "active" {
			delete(updates, "status")
		}
	}

	if err := service.UpdateAccountFields(h.db, id, updates); err != nil {
		slog.Error("Failed to update account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to update account"})
		return
	}

	// When status is forced to expired, align stored runtime health with  auth class.
	if status, ok := updates["status"].(string); ok && status == "expired" {
		_ = service.SetAccountRuntimeHealth(h.db, id, service.RuntimeHealthEntry{
			State:  service.HealthUnhealthy,
			Reason: "连接凭证已过期，请更新凭证",
			Source: service.HealthSourceAuth,
		})
	}

	var modelRefresh map[string]any
	if recovery {
		modelRefresh = accountModelRefresher(r.Context(), h.db, id, true)
		if modelRefreshSucceeded(modelRefresh) {
			if err := service.UpdateAccountFields(h.db, id, map[string]any{"status": "active"}); err != nil {
				slog.Error("Failed to reactivate account after model refresh", "err", err, "account_id", id)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "模型刷新成功但账号激活失败"})
				return
			}
			// Best-effort: clear stale auth-unhealthy runtime health after successful recovery.
			_ = clearAccountAuthRuntimeHealth(h.db, id)
		} else {
			// Keep expired; never claim active on failed model refresh.
			msg := modelRefreshErrorMessage(modelRefresh)
			slog.Warn("expired API-key recovery model refresh failed", "account_id", id, "message", msg)
			if err := service.UpdateAccountFields(h.db, id, map[string]any{"status": "expired"}); err != nil {
				slog.Warn("failed to preserve expired status after recovery failure", "account_id", id, "err", err)
			}
		}
	}

	updatedRow, err := service.GetAccountWithSiteByID(h.db, id)
	if err != nil {
		slog.Error("Failed to load updated account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to load updated account"})
		return
	}
	updated := updatedRow.Account
	caps := service.BuildCapabilitiesForAccount(&updated)
	sessionCapable := caps.CanRefreshBalance
	hasDiscoveredModels := false
	if recovery && modelRefreshSucceeded(modelRefresh) {
		hasDiscoveredModels = true
	}
	resp := map[string]any{
		"id":                 updated.ID,
		"siteId":             updated.SiteID,
		"username":           updated.Username,
		"accessToken":        updated.AccessToken,
		"apiToken":           updated.APIToken,
		"balance":            updated.Balance,
		"balanceUsed":        updated.BalanceUsed,
		"quota":              updated.Quota,
		"unitCost":           updated.UnitCost,
		"valueScore":         updated.ValueScore,
		"status":             updated.Status,
		"isPinned":           updated.IsPinned,
		"sortOrder":          updated.SortOrder,
		"checkinEnabled":     updated.CheckinEnabled,
		"lastCheckinAt":      updated.LastCheckinAt,
		"lastBalanceRefresh": updated.LastBalanceRefresh,
		"oauthProvider":      updated.OAuthProvider,
		"oauthAccountKey":    updated.OAuthAccountKey,
		"oauthProjectId":     updated.OAuthProjectID,
		"extraConfig":        updated.ExtraConfig,
		"createdAt":          updated.CreatedAt,
		"updatedAt":          updated.UpdatedAt,
		"credentialMode":     string(service.ResolveStoredCredentialMode(&updated)),
		"capabilities":       caps,
		"runtimeHealth": service.BuildRuntimeHealthForAccount(service.RuntimeHealthInput{
			AccountStatus:       updated.Status,
			SiteStatus:          updatedRow.Site.Status,
			ExtraConfig:         updated.ExtraConfig,
			SessionCapable:      &sessionCapable,
			HasDiscoveredModels: hasDiscoveredModels,
			OAuthProvider:       updated.OAuthProvider,
		}),
		"site": map[string]any{
			"id":       updatedRow.Site.ID,
			"name":     updatedRow.Site.Name,
			"url":      updatedRow.Site.URL,
			"platform": updatedRow.Site.Platform,
			"status":   updatedRow.Site.Status,
		},
	}
	if recovery {
		resp["modelRefresh"] = modelRefresh
		if !modelRefreshSucceeded(modelRefresh) {
			resp["message"] = "凭证已更新，但模型刷新失败，账号仍为 expired: " + modelRefreshErrorMessage(modelRefresh)
		}
	}
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, resp)
}

// ---- Delete Account ----

func (h *accountsHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := service.DeleteAccount(h.db, id); err != nil {
		slog.Error("Failed to delete account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to delete account"})
		return
	}
	service.RebuildRoutesBestEffort()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Batch Accounts ----

func (h *accountsHandler) batchAccounts(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountBatchPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid account batch payload."})
		return
	}

	if len(body.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ids is required"})
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{"enable": true, "disable": true, "delete": true, "refreshBalance": true}
	if !validActions[action] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid action"})
		return
	}

	successIDs := []int64{}
	failedItems := []map[string]any{}
	skippedItems := []map[string]any{}
	shouldRebuildRoutes := false
	shouldInvalidateRoutes := false

	for _, rawID := range body.IDs {
		id := int64(rawID)

		if action == "refreshBalance" {
			result, err := balanceService.RefreshBalance(h.cfg, h.db, id)
			if result == nil && err == nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
				continue
			}
			if err != nil {
				slog.Warn("Batch balance refresh failed", "err", err, "account_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Balance refresh failed"})
				continue
			}
			if result.Skipped {
				skippedItems = append(skippedItems, map[string]any{"id": id, "reason": result.Reason})
				continue
			}
			successIDs = append(successIDs, id)
			continue
		}

		var existing store.Account
		err := h.db.Get(&existing, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		var execErr error
		switch action {
		case "delete":
			_, execErr = h.db.Exec(h.db.Rebind("DELETE FROM accounts WHERE id = ?"), id)
		case "enable":
			_, execErr = h.db.Exec(h.db.Rebind("UPDATE accounts SET status = 'active', updated_at = ? WHERE id = ?"), now, id)
		case "disable":
			_, execErr = h.db.Exec(h.db.Rebind("UPDATE accounts SET status = 'disabled', updated_at = ? WHERE id = ?"), now, id)
		}
		if execErr != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account update failed"})
			continue
		}
		if action == "delete" {
			shouldRebuildRoutes = true
		} else if action == "enable" || action == "disable" {
			shouldInvalidateRoutes = true
		}
		successIDs = append(successIDs, id)
	}

	if shouldRebuildRoutes {
		service.RebuildRoutesBestEffort()
	} else if shouldInvalidateRoutes {
		routing.InvalidateCache()
	}
	if len(successIDs) > 0 {
		globalAccountsCache.clear()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"successIds":   successIDs,
		"failedItems":  failedItems,
		"skippedItems": skippedItems,
	})
}

// ---- Health Refresh ----

// healthRefreshResultItem is one account outcome from POST /api/accounts/health/refresh.
type healthRefreshResultItem struct {
	AccountID int64  `json:"accountId"`
	Status    string `json:"status"` // success | failed | skipped
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	ProxyOnly bool   `json:"proxyOnly,omitempty"`
}

// healthRefreshSummary aggregates wait-mode / background-task results.
type healthRefreshSummary struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Degraded  int `json:"degraded"`
	Disabled  int `json:"disabled"`
	Unknown   int `json:"unknown"`
	Success   int `json:"success"`
	Failed    int `json:"failed"`
	Skipped   int `json:"skipped"`
}

func (h *accountsHandler) refreshBalance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	result, err := balanceService.RefreshBalance(h.cfg, h.db, id)
	if result == nil && err == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found or platform not supported"})
		return
	}
	if err != nil {
		if strings.Contains(err.Error(), "unsupported platform") {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found or platform not supported"})
			return
		}
		slog.Warn("Balance refresh failed", "err", err, "account_id", id)
		writeJSON(w, http.StatusBadGateway, map[string]string{"message": "balance refresh failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"balance":     result.Balance,
		"balanceUsed": result.Used,
		"quota":       result.Quota,
		"skipped":     result.Skipped,
		"reason":      result.Reason,
	})
}

// ---- Account Models ----
