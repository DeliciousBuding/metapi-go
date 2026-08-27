package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// AccountCredentialMode is the credential mode for an account.
type AccountCredentialMode string

const (
	CredentialModeAuto    AccountCredentialMode = "auto"
	CredentialModeSession AccountCredentialMode = "session"
	CredentialModeAPIKey  AccountCredentialMode = "apikey"
)

// AccountCapabilities describes what an account can do.
type AccountCapabilities struct {
	CanCheckin        bool `json:"canCheckin"`
	CanRefreshBalance bool `json:"canRefreshBalance"`
	ProxyOnly         bool `json:"proxyOnly"`
}

// ---- Credential mode resolution ----

// GetCredentialModeFromExtraConfig reads credentialMode from extraConfig JSON.
func GetCredentialModeFromExtraConfig(extraConfig *string) AccountCredentialMode {
	config := ParseExtraConfig(extraConfig)
	if config == nil {
		return ""
	}
	if mode, ok := config["credentialMode"].(string); ok {
		normalized := NormalizeCredentialMode(mode)
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

// NormalizeCredentialMode normalizes a credential mode string.
func NormalizeCredentialMode(input string) AccountCredentialMode {
	normalized := AccountCredentialMode(strings.TrimSpace(strings.ToLower(input)))
	switch normalized {
	case CredentialModeAuto, CredentialModeSession, CredentialModeAPIKey:
		return normalized
	}
	return ""
}

// ResolveStoredCredentialMode resolves the effective credential mode from storage.
// 1. Read extraConfig.credentialMode; if explicit and != "auto", return it
// 2. Treat legacy mirrored accessToken/apiToken rows as "apikey"
// 3. If accessToken is non-empty, return "session"
// 4. Otherwise return "apikey"
func ResolveStoredCredentialMode(account *store.Account) AccountCredentialMode {
	fromConfig := GetCredentialModeFromExtraConfig(account.ExtraConfig)
	if fromConfig != "" && fromConfig != CredentialModeAuto {
		return fromConfig
	}
	if hasMirroredAPIKeyToken(account) {
		return CredentialModeAPIKey
	}
	if strings.TrimSpace(account.AccessToken) != "" {
		return CredentialModeSession
	}
	return CredentialModeAPIKey
}

// ResolveRequestedCredentialMode resolves the requested credential mode from input.
func ResolveRequestedCredentialMode(input *string) AccountCredentialMode {
	if input == nil {
		return CredentialModeAuto
	}
	mode := NormalizeCredentialMode(*input)
	if mode == "" {
		return CredentialModeAuto
	}
	return mode
}

// HasSessionToken checks whether the account has a valid session token value.
func HasSessionToken(accessToken string) bool {
	return strings.TrimSpace(accessToken) != ""
}

// IsAPIKeyConnection checks if an account is an API key connection.
func IsAPIKeyConnection(account *store.Account) bool {
	explicit := GetCredentialModeFromExtraConfig(account.ExtraConfig)
	if explicit != "" && explicit != CredentialModeAuto {
		return explicit == CredentialModeAPIKey
	}
	return hasMirroredAPIKeyToken(account) || strings.TrimSpace(account.AccessToken) == ""
}

func hasMirroredAPIKeyToken(account *store.Account) bool {
	if account == nil || account.APIToken == nil {
		return false
	}
	accessToken := strings.TrimSpace(account.AccessToken)
	apiToken := strings.TrimSpace(*account.APIToken)
	return accessToken != "" && apiToken != "" && accessToken == apiToken
}

// BuildCapabilitiesForAccount returns the capabilities for an account.
func BuildCapabilitiesForAccount(account *store.Account) AccountCapabilities {
	mode := ResolveStoredCredentialMode(account)
	hasToken := HasSessionToken(account.AccessToken)
	return BuildCapabilitiesFromCredentialMode(mode, hasToken)
}

// BuildCapabilitiesFromCredentialMode returns capabilities from credential mode.
func BuildCapabilitiesFromCredentialMode(mode AccountCredentialMode, hasSessionToken bool) AccountCapabilities {
	sessionCapable := false
	switch mode {
	case CredentialModeSession:
		sessionCapable = hasSessionToken
	case CredentialModeAPIKey:
		sessionCapable = false
	default: // auto
		sessionCapable = hasSessionToken
	}
	return AccountCapabilities{
		CanCheckin:        sessionCapable,
		CanRefreshBalance: sessionCapable,
		ProxyOnly:         !sessionCapable,
	}
}

// ---- AES Password Encryption (delegates to account_credential.go) ----

// EncryptPassword encrypts a password for storage in extraConfig.autoRelogin.passwordCipher.
func EncryptPassword(cfg *config.Config, password string) (string, error) {
	return EncryptAccountPassword(cfg, password)
}

// DecryptPassword decrypts a password from extraConfig.
func DecryptPassword(cfg *config.Config, cipherText string) string {
	return DecryptAccountPassword(cfg, cipherText)
}

// ---- ExtraConfig helpers ----

// MergeExtraConfig merges a patch into an existing extraConfig JSON string.
func MergeExtraConfig(existing *string, patch map[string]any) *string {
	base := ParseExtraConfig(existing)
	if base == nil {
		base = make(map[string]any)
	}
	for k, v := range patch {
		if v == nil {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	return MarshalExtraConfig(base)
}

// GetProxyURLFromExtraConfig reads proxyUrl from extraConfig.
func GetProxyURLFromExtraConfig(extraConfig *string) string {
	config := ParseExtraConfig(extraConfig)
	if config == nil {
		return ""
	}
	if url, ok := config["proxyUrl"].(string); ok {
		return url
	}
	return ""
}

// GetUseSystemProxyFromExtraConfig reads useSystemProxy from extraConfig.
func GetUseSystemProxyFromExtraConfig(extraConfig *string) bool {
	config := ParseExtraConfig(extraConfig)
	if config == nil {
		return false
	}
	if enabled, ok := config["useSystemProxy"].(bool); ok {
		return enabled
	}
	return false
}

// UTLSEnabled resolves whether uTLS TLS fingerprint masking is active for a
// site. Per-site UseUTLS overrides the global UTLS_ENABLED flag:
//   - site.UseUTLS != nil → explicit per-site decision (true/false)
//   - site.UseUTLS == nil → fall back to cfg.UTLSEnabled (global default)
//
// Mirrors the ResinEnabled resolution pattern so operators can globally opt in
// via UTLS_ENABLED=true and selectively disable specific sites with use_utls=false.
func UTLSEnabled(cfg *config.Config, site *store.Site) bool {
	if site != nil && site.UseUTLS != nil {
		return *site.UseUTLS
	}
	return cfg != nil && cfg.UTLSEnabled
}

// BuildPlatformProxyConfig resolves account and site proxy settings for platform calls.
//
// Precedence (highest to lowest):
//  1. key proxy (applied separately via proxy.ApplyKeyProxyOverride)
//  2. account extraConfig.proxyUrl (explicit per-account proxy)
//  3. site.ProxyURL (explicit per-site proxy)
//  4. Resin sticky forward proxy (when RESIN_ENABLED and identity available)
//  5. system proxy (account extraConfig.useSystemProxy or site.UseSystemProxy)
//  6. direct (no proxy)
//
// The Resin tier sits between explicit proxy URLs and the generic system-proxy
// opt-in so an operator who sets a per-account/per-site proxy still wins, but
// Resin takes priority over the shared SYSTEM_PROXY_URL. Resin requires a
// business identity: acc-{account.ID} when the account row exists. For
// pre-account flows (verify/login/create) with no account row yet, use
// BuildPlatformProxyConfigForToken which derives a deterministic temp identity.
func BuildPlatformProxyConfig(cfg *config.Config, account *store.Account, site *store.Site) *platform.ProxyConfig {
	if account == nil && site == nil {
		return nil
	}

	proxyCfg := &platform.ProxyConfig{}
	if site != nil {
		// wire site opt-in so ApplyCustomHeaders can force site-wins.
		proxyCfg.CustomHeadersOverrideRequest = site.CustomHeadersOverrideRequestHeaders
		if site.CustomHeaders != nil && strings.TrimSpace(*site.CustomHeaders) != "" {
			var headers map[string]string
			if err := json.Unmarshal([]byte(*site.CustomHeaders), &headers); err == nil && len(headers) > 0 {
				proxyCfg.CustomHeaders = filterPlatformCustomHeaders(headers)
			}
		}
		if site.CfClearance != nil {
			proxyCfg.ClearanceCookie = strings.TrimSpace(*site.CfClearance)
		}
		if site.BrowserUA != nil {
			proxyCfg.BrowserUA = strings.TrimSpace(*site.BrowserUA)
		}
	}

	// Resolve uTLS from site-level override or global UTLS_ENABLED flag. This
	// is independent of the proxy-precedence chain below — uTLS masking applies
	// regardless of whether a proxy URL is configured.
	proxyCfg.UseUTLS = UTLSEnabled(cfg, site)

	// 2. account explicit proxyUrl
	if account != nil {
		if accountProxyURL := strings.TrimSpace(GetProxyURLFromExtraConfig(account.ExtraConfig)); accountProxyURL != "" {
			proxyCfg.ProxyURL = accountProxyURL
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	// 3. site explicit ProxyURL
	if site != nil {
		if site.ProxyURL != nil && strings.TrimSpace(*site.ProxyURL) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(*site.ProxyURL)
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	// 4. Resin sticky forward proxy (only when no higher-precedence proxy set).
	//    Requires a business identity: acc-{account.ID} when account exists.
	if ResinEnabled(cfg, site) {
		platformName := ResolveResinPlatform(cfg, site)
		accountIdentity := AccountFor(account, site)
		if platformName != "" && accountIdentity != "" {
			if resinURL, ok := ForwardProxyURL(cfg, platformName, accountIdentity); ok {
				proxyCfg.ProxyURL = resinURL
				proxyCfg.ResinAccount = accountIdentity
				TouchResinLease(accountIdentity)
				return normalizePlatformProxyConfig(proxyCfg)
			}
		}
	}

	// 5. system proxy fallback (account opt-in, then site opt-in)
	if account != nil {
		if GetUseSystemProxyFromExtraConfig(account.ExtraConfig) && cfg != nil && strings.TrimSpace(cfg.SystemProxyUrl) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(cfg.SystemProxyUrl)
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	if site != nil {
		if site.UseSystemProxy && cfg != nil && strings.TrimSpace(cfg.SystemProxyUrl) != "" {
			proxyCfg.ProxyURL = strings.TrimSpace(cfg.SystemProxyUrl)
			proxyCfg.UseSystemProxy = true
			return normalizePlatformProxyConfig(proxyCfg)
		}
	}

	return normalizePlatformProxyConfig(proxyCfg)
}

// BuildPlatformProxyConfigForToken is the pre-account variant of
// BuildPlatformProxyConfig for verify-token / login / create flows where no
// Account row exists yet. It derives a deterministic temp Resin identity from
// (siteID, rawToken) so all pre-account requests for the same token share an
// egress IP, then (in Tier 2) inherit-lease can transfer the lease to the
// stable acc-{id} identity after INSERT.
//
// Precedence matches BuildPlatformProxyConfig with the temp identity filling
// the resin tier: site.ProxyURL > resin(temp) > system > direct.
func BuildPlatformProxyConfigForToken(cfg *config.Config, site *store.Site, rawToken string) *platform.ProxyConfig {
	// When the site has an explicit ProxyURL, that beats resin — delegate
	// entirely so headers/cookies/UA are assembled by the shared path.
	if site != nil && site.ProxyURL != nil && strings.TrimSpace(*site.ProxyURL) != "" {
		return BuildPlatformProxyConfig(cfg, nil, site)
	}

	// No explicit site.ProxyURL: assemble the base config (headers, cookies,
	// UA, and possibly the system proxy via site.UseSystemProxy).
	proxyCfg := BuildPlatformProxyConfig(cfg, nil, site)
	if proxyCfg == nil {
		proxyCfg = &platform.ProxyConfig{}
	}

	if !ResinEnabled(cfg, site) || site == nil {
		return normalizePlatformProxyConfig(proxyCfg)
	}

	platformName := ResolveResinPlatform(cfg, site)
	tempIdentity := TempAccountFor(site.ID, rawToken)
	if platformName == "" || tempIdentity == "" {
		return normalizePlatformProxyConfig(proxyCfg)
	}
	resinURL, ok := ForwardProxyURL(cfg, platformName, tempIdentity)
	if !ok {
		return normalizePlatformProxyConfig(proxyCfg)
	}
	// Resin wins over the system proxy per precedence, so override whatever
	// ProxyURL the inner call set (or fill the empty direct case).
	proxyCfg.ProxyURL = resinURL
	proxyCfg.UseSystemProxy = false
	proxyCfg.ResinAccount = tempIdentity
	TouchResinLease(tempIdentity)
	return normalizePlatformProxyConfig(proxyCfg)
}

// BuildPlatformProxyConfigForTokenWithProxyURL is the pre-account proxy
// builder for unsaved account-form values. A non-empty request override must
// win over the saved site proxy, Resin, and system proxy for the verification
// or create request; empty input keeps the normal site-level precedence.
func BuildPlatformProxyConfigForTokenWithProxyURL(cfg *config.Config, site *store.Site, rawToken, proxyURL string) *platform.ProxyConfig {
	proxyURL = strings.TrimSpace(proxyURL)
	if proxyURL == "" {
		return BuildPlatformProxyConfigForToken(cfg, site, rawToken)
	}

	// Build the site-derived headers/UA/cookie/uTLS settings without resolving
	// Resin or the saved site proxy, then apply the explicit unsaved override.
	proxyCfg := BuildPlatformProxyConfig(cfg, nil, site)
	if proxyCfg == nil {
		proxyCfg = &platform.ProxyConfig{}
	}
	proxyCfg.ProxyURL = proxyURL
	proxyCfg.UseSystemProxy = false
	proxyCfg.ResinAccount = ""
	return normalizePlatformProxyConfig(proxyCfg)
}

func normalizePlatformProxyConfig(proxyCfg *platform.ProxyConfig) *platform.ProxyConfig {
	if proxyCfg == nil {
		return nil
	}
	if proxyCfg.ProxyURL == "" && !proxyCfg.UseSystemProxy && len(proxyCfg.CustomHeaders) == 0 && !proxyCfg.InsecureSkipTLS &&
		proxyCfg.ClearanceCookie == "" && proxyCfg.BrowserUA == "" && !proxyCfg.UseUTLS {
		return nil
	}
	return proxyCfg
}

func filterPlatformCustomHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	filtered := make(map[string]string, len(headers))
	for k, v := range headers {
		if isReservedPlatformCustomHeader(k) {
			continue
		}
		filtered[k] = v
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func isReservedPlatformCustomHeader(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "authorization", "cookie", "new-api-user", "content-type", "content-length", "host":
		return true
	default:
		return false
	}
}

// ResolvePlatformUserID resolves the platform user ID from extraConfig or username.
func ResolvePlatformUserID(extraConfig *string, username *string) int64 {
	config := ParseExtraConfig(extraConfig)
	if config != nil {
		if id, ok := config["platformUserId"].(float64); ok {
			return int64(id)
		}
	}
	return 0
}

// ---- Account CRUD helpers ----

// GetAccountByID fetches an account by its ID.
func GetAccountByID(db *sqlx.DB, id int64) (*store.Account, error) {
	var account store.Account
	err := db.Get(&account, db.Rebind("SELECT * FROM accounts WHERE id = ?"), id)
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// GetAccountWithSite joins account with site.
type AccountWithSite struct {
	Account store.Account `db:"accounts"`
	Site    store.Site    `db:"sites"`
}

// GetAccountWithSiteByID fetches an account with its site.
// accountQueryExecer is the minimal executor surface shared by *sqlx.DB and
// *sqlx.Tx, sufficient for the account read/write helpers below.
type accountQueryExecer interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	Rebind(query string) string
	DriverName() string
}

// GetAccountWithSiteByID reads an account joined with its site (no row lock).
func GetAccountWithSiteByID(db *sqlx.DB, id int64) (*AccountWithSite, error) {
	return getAccountWithSiteBy(db, id, false)
}

// GetAccountWithSiteByIDForUpdate reads the account joined with its site while
// locking the accounts row (PostgreSQL only: FOR UPDATE OF a; SQLite's
// single-connection pool already serializes transactions). Call it inside a
// transaction to make read-merge-write of mutable columns — notably
// extra_config — safe against concurrent lost updates.
func GetAccountWithSiteByIDForUpdate(q accountQueryExecer, id int64) (*AccountWithSite, error) {
	return getAccountWithSiteBy(q, id, true)
}

func getAccountWithSiteBy(q accountQueryExecer, id int64, forUpdate bool) (*AccountWithSite, error) {
	query := `SELECT a.id AS "accounts.id", a.site_id AS "accounts.site_id", a.username AS "accounts.username",
		a.access_token AS "accounts.access_token", a.api_token AS "accounts.api_token",
		a.balance AS "accounts.balance", a.balance_used AS "accounts.balance_used",
		a.quota AS "accounts.quota", a.unit_cost AS "accounts.unit_cost",
		a.value_score AS "accounts.value_score", a.status AS "accounts.status",
		a.is_pinned AS "accounts.is_pinned", a.sort_order AS "accounts.sort_order",
		a.checkin_enabled AS "accounts.checkin_enabled", a.last_checkin_at AS "accounts.last_checkin_at",
		a.last_balance_refresh AS "accounts.last_balance_refresh",
		a.oauth_provider AS "accounts.oauth_provider", a.oauth_account_key AS "accounts.oauth_account_key",
		a.oauth_project_id AS "accounts.oauth_project_id", a.extra_config AS "accounts.extra_config",
		a.created_at AS "accounts.created_at", a.updated_at AS "accounts.updated_at",
		a.remark AS "accounts.remark",
		s.id AS "sites.id", s.name AS "sites.name", s.url AS "sites.url",
		s.platform AS "sites.platform", s.proxy_url AS "sites.proxy_url",
		s.use_system_proxy AS "sites.use_system_proxy",
		s.custom_headers AS "sites.custom_headers",
			s.custom_headers_override_request_headers AS "sites.custom_headers_override_request_headers",
			s.status AS "sites.status"
			FROM accounts a INNER JOIN sites s ON a.site_id = s.id WHERE a.id = ?`
	if forUpdate && q.DriverName() == "pgx" {
		// Lock only the accounts row (the join also reads sites). A concurrent
		// writer blocks here until this transaction commits, so its merge reads
		// the committed extra_config instead of overwriting it.
		query += " FOR UPDATE OF a"
	}

	var row struct {
		Accounts struct {
			ID                 int64    `db:"id"`
			SiteID             int64    `db:"site_id"`
			Username           *string  `db:"username"`
			AccessToken        string   `db:"access_token"`
			APIToken           *string  `db:"api_token"`
			Balance            *float64 `db:"balance"`
			BalanceUsed        *float64 `db:"balance_used"`
			Quota              *float64 `db:"quota"`
			UnitCost           *float64 `db:"unit_cost"`
			ValueScore         *float64 `db:"value_score"`
			Status             string   `db:"status"`
			IsPinned           bool     `db:"is_pinned"`
			SortOrder          int64    `db:"sort_order"`
			CheckinEnabled     bool     `db:"checkin_enabled"`
			LastCheckinAt      *string  `db:"last_checkin_at"`
			LastBalanceRefresh *string  `db:"last_balance_refresh"`
			OAuthProvider      *string  `db:"oauth_provider"`
			OAuthAccountKey    *string  `db:"oauth_account_key"`
			OAuthProjectID     *string  `db:"oauth_project_id"`
			ExtraConfig        *string  `db:"extra_config"`
			CreatedAt          string   `db:"created_at"`
			UpdatedAt          string   `db:"updated_at"`
			Remark             *string  `db:"remark"`
		} `db:"accounts"`
		Sites struct {
			ID                                  int64   `db:"id"`
			Name                                string  `db:"name"`
			URL                                 string  `db:"url"`
			Platform                            string  `db:"platform"`
			ProxyURL                            *string `db:"proxy_url"`
			UseSystemProxy                      bool    `db:"use_system_proxy"`
			CustomHeaders                       *string `db:"custom_headers"`
			CustomHeadersOverrideRequestHeaders bool    `db:"custom_headers_override_request_headers"`
			Status                              string  `db:"status"`
		} `db:"sites"`
	}

	if err := q.Get(&row, q.Rebind(query), id); err != nil {
		return nil, err
	}
	return &AccountWithSite{
		Account: store.Account{
			ID: row.Accounts.ID, SiteID: row.Accounts.SiteID,
			Username: row.Accounts.Username, AccessToken: row.Accounts.AccessToken,
			APIToken: row.Accounts.APIToken, Balance: row.Accounts.Balance,
			BalanceUsed: row.Accounts.BalanceUsed, Quota: row.Accounts.Quota,
			UnitCost: row.Accounts.UnitCost, ValueScore: row.Accounts.ValueScore,
			Status: row.Accounts.Status, IsPinned: row.Accounts.IsPinned,
			SortOrder: row.Accounts.SortOrder, CheckinEnabled: row.Accounts.CheckinEnabled,
			LastCheckinAt: row.Accounts.LastCheckinAt, LastBalanceRefresh: row.Accounts.LastBalanceRefresh,
			OAuthProvider: row.Accounts.OAuthProvider, OAuthAccountKey: row.Accounts.OAuthAccountKey,
			OAuthProjectID: row.Accounts.OAuthProjectID, ExtraConfig: row.Accounts.ExtraConfig,
			CreatedAt: row.Accounts.CreatedAt, UpdatedAt: row.Accounts.UpdatedAt,
			Remark: row.Accounts.Remark,
		},
		Site: store.Site{
			ID: row.Sites.ID, Name: row.Sites.Name,
			URL: row.Sites.URL, Platform: row.Sites.Platform,
			ProxyURL: row.Sites.ProxyURL, UseSystemProxy: row.Sites.UseSystemProxy,
			CustomHeaders:                       row.Sites.CustomHeaders,
			CustomHeadersOverrideRequestHeaders: row.Sites.CustomHeadersOverrideRequestHeaders,
			Status:                              row.Sites.Status,
		},
	}, nil
}

// DeleteAccount deletes an account by ID.
func DeleteAccount(db *sqlx.DB, id int64) error {
	_, err := db.Exec(db.Rebind("DELETE FROM accounts WHERE id = ?"), id)
	return err
}

// GetNextAccountSortOrder returns the next sortOrder value for new accounts.
func GetNextAccountSortOrder(db *sqlx.DB) (int64, error) {
	var maxOrder int64
	err := db.Get(&maxOrder, db.Rebind("SELECT COALESCE(MAX(sort_order), -1) FROM accounts"))
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// InsertAccount inserts a new account and returns the ID.
func InsertAccount(db *sqlx.DB, account map[string]any) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	sortOrder := account["sortOrder"].(int64)
	extraConfig := account["extraConfig"]
	var extraConfigStr *string
	if extraConfig != nil {
		b, err := json.Marshal(extraConfig)
		if err == nil {
			s := string(b)
			extraConfigStr = &s
		}
	}

	var id int64
	err := db.QueryRowx(
		db.Rebind(`INSERT INTO accounts (site_id, username, access_token, api_token, balance, balance_used, quota,
		 unit_cost, value_score, status, is_pinned, sort_order, checkin_enabled,
		 last_checkin_at, last_balance_refresh, oauth_provider, oauth_account_key,
		 oauth_project_id, extra_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`),
		account["siteId"], account["username"], account["accessToken"],
		account["apiToken"], account["balance"], account["balanceUsed"],
		account["quota"], account["unitCost"], account["valueScore"],
		account["status"], account["isPinned"], sortOrder,
		account["checkinEnabled"], account["lastCheckinAt"], account["lastBalanceRefresh"],
		account["oauthProvider"], account["oauthAccountKey"], account["oauthProjectID"],
		extraConfigStr, now, now,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateAccountFields updates specific fields on an account.
// UpdateAccountFields writes a set of account columns in one UPDATE.
func UpdateAccountFields(db *sqlx.DB, accountID int64, updates map[string]any) error {
	return updateAccountFieldsOn(db, accountID, updates)
}

// UpdateAccountFieldsTx is the transaction variant of UpdateAccountFields. Use
// it within the same transaction that read-locked the account row so the
// read-merge-write of extra_config commits atomically (no lost updates).
func UpdateAccountFieldsTx(tx *sqlx.Tx, accountID int64, updates map[string]any) error {
	return updateAccountFieldsOn(tx, accountID, updates)
}

func updateAccountFieldsOn(q accountQueryExecer, accountID int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var setClauses []string
	var args []any

	colMap := map[string]string{
		"accessToken":        "access_token",
		"apiToken":           "api_token",
		"username":           "username",
		"status":             "status",
		"checkinEnabled":     "checkin_enabled",
		"unitCost":           "unit_cost",
		"extraConfig":        "extra_config",
		"isPinned":           "is_pinned",
		"sortOrder":          "sort_order",
		"balance":            "balance",
		"balanceUsed":        "balance_used",
		"quota":              "quota",
		"valueScore":         "value_score",
		"lastBalanceRefresh": "last_balance_refresh",
		"remark":             "remark",
		"tags":               "tags",
	}

	for key, val := range updates {
		if col, ok := colMap[key]; ok {
			setClauses = append(setClauses, col+" = ?")
			args = append(args, val)
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, now)
	args = append(args, accountID)

	query := fmt.Sprintf("UPDATE accounts SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err := q.Exec(q.Rebind(query), args...)
	return err
}

// ListAccountsWithSites returns all accounts joined with their sites.
// Each account is enriched with nested site, capabilities, credentialMode, and
// effective runtimeHealth (expired status never reports healthy).
func ListAccountsWithSites(db *sqlx.DB) ([]map[string]any, error) {
	rows, err := db.Queryx(
		`SELECT a.*, s.id as site_id_val, s.name as site_name, s.url as site_url,
		        s.platform as site_platform, s.status as site_status
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		 ORDER BY a.sort_order, a.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	var scanErr error
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		normalizeMapScanValues(row)
		account := mapKeysToCamel(row)
		result = append(result, enrichAccountOverviewRow(account))
	}
	return result, scanErr
}

// ListAccountsWithSitesPaginated returns a single page of accounts joined with
// their sites, plus the total row count matching the same join. Enrichment is
// identical to ListAccountsWithSites so a paginated page is byte-compatible
// with a slice of the unpaginated snapshot. Used by GET /api/accounts?page=...
// to bound the admin list response when the fleet grows (defensive pagination,
// same envelope as /api/channels and /api/checkin/logs).
func ListAccountsWithSitesPaginated(db *sqlx.DB, limit, offset int) ([]map[string]any, int64, error) {
	rows, err := db.Queryx(db.Rebind(
		`SELECT a.*, s.id as site_id_val, s.name as site_name, s.url as site_url,
		        s.platform as site_platform, s.status as site_status
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		 ORDER BY a.sort_order, a.id
		 LIMIT ? OFFSET ?`), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []map[string]any
	var scanErr error
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		normalizeMapScanValues(row)
		account := mapKeysToCamel(row)
		result = append(result, enrichAccountOverviewRow(account))
	}

	var total int64
	if err := db.Get(&total, db.Rebind(
		`SELECT COUNT(*) FROM accounts a INNER JOIN sites s ON a.site_id = s.id`)); err != nil {
		return result, 0, err
	}
	return result, total, scanErr
}

// enrichAccountOverviewRow attaches admin-list overview fields used by the UI.
func enrichAccountOverviewRow(account map[string]any) map[string]any {
	if account == nil {
		return account
	}

	status := asString(account["status"])
	siteStatus := asString(account["siteStatus"])
	extraConfig := asStringPtr(account["extraConfig"])
	accessToken := asString(account["accessToken"])
	oauthProvider := asStringPtr(account["oauthProvider"])

	acct := &store.Account{
		AccessToken:   accessToken,
		APIToken:      asStringPtr(account["apiToken"]),
		ExtraConfig:   extraConfig,
		OAuthProvider: oauthProvider,
	}
	credentialMode := ResolveStoredCredentialMode(acct)
	capabilities := BuildCapabilitiesForAccount(acct)
	sessionCapable := capabilities.CanRefreshBalance

	account["credentialMode"] = string(credentialMode)
	account["capabilities"] = capabilities
	account["runtimeHealth"] = BuildRuntimeHealthForAccount(RuntimeHealthInput{
		AccountStatus:  status,
		SiteStatus:     siteStatus,
		ExtraConfig:    extraConfig,
		SessionCapable: &sessionCapable,
		OAuthProvider:  oauthProvider,
	})
	account["site"] = map[string]any{
		"id":       account["siteIdVal"],
		"name":     account["siteName"],
		"url":      account["siteUrl"],
		"platform": account["sitePlatform"],
		"status":   siteStatus,
	}
	// Prefer nested site; drop flat join aliases that are not account columns.
	delete(account, "siteIdVal")
	delete(account, "siteName")
	delete(account, "siteUrl")
	delete(account, "sitePlatform")
	delete(account, "siteStatus")
	// List/overview must not return plaintext secrets. Create/update/rebind
	// responses may still echo once; those paths build maps outside this helper.
	redactAccountSecrets(account)
	return account
}

// redactAccountSecrets removes plaintext credentials from admin list/overview rows.
// Sets accessTokenMasked / apiTokenMasked when a value was present.
func redactAccountSecrets(account map[string]any) {
	if account == nil {
		return
	}
	if v := asString(account["accessToken"]); strings.TrimSpace(v) != "" {
		account["accessTokenMasked"] = maskSecretTail(v)
	}
	delete(account, "accessToken")
	if v := asString(account["apiToken"]); strings.TrimSpace(v) != "" {
		account["apiTokenMasked"] = maskSecretTail(v)
	}
	delete(account, "apiToken")
	// Never leak passwordCipher from extraConfig on list surfaces.
	if ec := asStringPtr(account["extraConfig"]); ec != nil && strings.Contains(*ec, "passwordCipher") {
		var m map[string]any
		if err := json.Unmarshal([]byte(*ec), &m); err == nil {
			if auto, ok := m["autoRelogin"].(map[string]any); ok {
				delete(auto, "passwordCipher")
				m["autoRelogin"] = auto
				if b, err := json.Marshal(m); err == nil {
					s := string(b)
					account["extraConfig"] = s
				}
			}
		}
	}
}

func maskSecretTail(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func normalizeMapScanValues(m map[string]any) {
	for k, v := range m {
		switch typed := v.(type) {
		case []byte:
			m[k] = string(typed)
		}
	}
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return nil
	}
	return &s
}

// ---- Event helpers ----

// snakeToCamel converts snake_case to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// mapKeysToCamel returns a new map with all keys converted from snake_case to camelCase.
func mapKeysToCamel(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[snakeToCamel(k)] = v
	}
	return result
}

// CreateEvent creates an event entry.
func CreateEvent(db *sqlx.DB, eventType string, title string, message string, level string, relatedID int64, relatedType string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		db.Rebind(`INSERT INTO events (type, title, message, level, read, related_id, related_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		eventType, title, message, level, false, relatedID, relatedType, now,
	)
	return err
}
