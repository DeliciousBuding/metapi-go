package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterSearchRoutes registers all /api/search routes.
func RegisterSearchRoutes(r chi.Router, db *sqlx.DB) {
	handler := &searchHandler{db: db}
	r.Post("/api/search", handler.search)
}

type searchHandler struct {
	db *sqlx.DB
}

type searchRequestBody struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

// searchQueryMaxLength caps the trimmed query length before it is interpolated
// into a LIKE pattern. Prevents unbounded SQL pattern construction and guards
// against request-size abuse on the admin search surface.
const searchQueryMaxLength = 200

// accountPublicSelectColumns lists every accounts column EXCEPT the plaintext
// credentials (access_token, api_token). List/search endpoints pair this with
// credentialFragmentsSelect() to expose masked secrets without ever scanning
// the full plaintext into Go memory. Mirrors service.SiteSelectColumns.
const accountPublicSelectColumns = `a.id, a.site_id, a.username, a.balance, a.balance_used,
	a.quota, a.unit_cost, a.value_score, a.status, a.is_pinned, a.sort_order,
	a.checkin_enabled, a.last_checkin_at, a.last_balance_refresh, a.oauth_provider,
	a.oauth_account_key, a.oauth_project_id, a.extra_config, a.created_at,
	a.updated_at, a.tags`

// accountTokenPublicSelectColumns lists every account_tokens column EXCEPT the
// plaintext token. Paired with credentialFragmentsSelect("at.token", ...).
const accountTokenPublicSelectColumns = `at.id, at.account_id, at.name, at.token_group,
	at.value_status, at.source, at.enabled, at.is_default, at.created_at, at.updated_at`

// POST /api/search
func (h *searchHandler) search(w http.ResponseWriter, r *http.Request) {
	var body searchRequestBody
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"accounts":      []any{},
			"accountTokens": []any{},
			"sites":         []any{},
			"checkinLogs":   []any{},
			"proxyLogs":     []any{},
			"models":        []any{},
		})
		return
	}

	q := strings.TrimSpace(body.Query)
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"accounts":      []any{},
			"accountTokens": []any{},
			"sites":         []any{},
			"checkinLogs":   []any{},
			"proxyLogs":     []any{},
			"models":        []any{},
		})
		return
	}

	if len(q) > searchQueryMaxLength {
		writeError(w, http.StatusBadRequest, "search query too long (max 200 characters)")
		return
	}

	limit := body.Limit
	if limit <= 0 {
		limit = 20
	}
	perCategory := limit / 6
	if perCategory < 1 {
		perCategory = 1
	}
	if perCategory > 10 {
		perCategory = 10
	}

	likePattern := "%" + q + "%"

	// Search sites
	sites := queryRows(h.db, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE name LIKE ? OR url LIKE ? OR platform LIKE ? LIMIT ?",
		likePattern, likePattern, likePattern, perCategory)

	// Search accounts. Select only credential fragments (first4/last4/length)
	// instead of a.* — the plaintext access_token/api_token never cross the
	// DB→Go boundary, so a stray slog/metrics call cannot leak them. The masked
	// form is rebuilt in Go by redactSearchAccountSecrets().
	accounts := queryRows(h.db,
		`SELECT `+accountPublicSelectColumns+`, `+
			credentialFragmentsSelect(h.db, "a.access_token", "access_token")+`, `+
			credentialFragmentsSelect(h.db, "a.api_token", "api_token")+`,
			s.name as site_name, s.platform as site_platform
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		 WHERE a.username LIKE ? OR s.name LIKE ? OR s.platform LIKE ?
		 LIMIT ?`,
		likePattern, likePattern, likePattern, perCategory)

	// Search account tokens. Same fragment pattern for at.token.
	accountTokens := queryRows(h.db,
		`SELECT `+accountTokenPublicSelectColumns+`, `+
			credentialFragmentsSelect(h.db, "at.token", "token")+`,
			a.username as account_username, s.name as site_name
		 FROM account_tokens at
		 INNER JOIN accounts a ON at.account_id = a.id
		 INNER JOIN sites s ON a.site_id = s.id
		 WHERE at.name LIKE ? OR coalesce(at.token_group,'') LIKE ? OR a.username LIKE ? OR s.name LIKE ?
		 ORDER BY at.updated_at DESC LIMIT ?`,
		likePattern, likePattern, likePattern, likePattern, perCategory)

	// Search checkin logs
	checkinLogs := queryRows(h.db,
		`SELECT cl.*, a.username as account_username
		 FROM checkin_logs cl
		 INNER JOIN accounts a ON cl.account_id = a.id
		 WHERE coalesce(cl.message,'') LIKE ?
		 ORDER BY cl.created_at DESC LIMIT ?`,
		likePattern, perCategory)

	// Search proxy logs
	proxyLogs := queryRows(h.db,
		"SELECT * FROM proxy_logs WHERE coalesce(model_requested,'') LIKE ? ORDER BY created_at DESC LIMIT ?",
		likePattern, perCategory)

	// Search models
	modelRows := queryRows(h.db,
		`SELECT DISTINCT tma.model_name, COUNT(DISTINCT at.id) as token_count,
		        COUNT(DISTINCT a.id) as account_count, COUNT(DISTINCT s.id) as site_count
		 FROM token_model_availability tma
		 INNER JOIN account_tokens at ON tma.token_id = at.id
		 INNER JOIN accounts a ON at.account_id = a.id
		 INNER JOIN sites s ON a.site_id = s.id
		 WHERE tma.model_name LIKE ? AND tma.available = TRUE AND at.enabled = TRUE AND a.status = 'active'
		 GROUP BY tma.model_name
		 ORDER BY account_count DESC LIMIT ?`,
		likePattern, perCategory)

	for _, row := range accounts {
		redactSearchAccountSecrets(row)
	}
	for _, row := range accountTokens {
		redactSearchTokenSecrets(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accounts":      normalizeSlice(accounts),
		"accountTokens": normalizeSlice(accountTokens),
		"sites":         normalizeSlice(sites),
		"checkinLogs":   normalizeSlice(checkinLogs),
		"proxyLogs":     normalizeSlice(proxyLogs),
		"models":        normalizeSlice(modelRows),
	})
}

// redactSearchAccountSecrets rebuilds masked credentials from the
// prefix/suffix/length fragments selected by credentialFragmentsSelect().
// The plaintext access_token/api_token are never pulled from the DB, so there
// is nothing to delete — this helper exists to keep the response shape
// (accessTokenMasked / apiTokenMasked) identical to the prior behavior.
func redactSearchAccountSecrets(row map[string]any) {
	if row == nil {
		return
	}
	if accessTokenLen := coerceInt64(row["accessTokenLen"]); accessTokenLen > 0 {
		row["accessTokenMasked"] = maskSecretFromFragments(row["accessTokenPrefix"], row["accessTokenSuffix"], accessTokenLen)
	}
	if apiTokenLen := coerceInt64(row["apiTokenLen"]); apiTokenLen > 0 {
		row["apiTokenMasked"] = maskSecretFromFragments(row["apiTokenPrefix"], row["apiTokenSuffix"], apiTokenLen)
	}
}

// redactSearchTokenSecrets rebuilds the masked token from the prefix/suffix/
// length fragments selected by credentialFragmentsSelect(). The plaintext
// token is never pulled from the DB.
func redactSearchTokenSecrets(row map[string]any) {
	if row == nil {
		return
	}
	if tokenLen := coerceInt64(row["tokenLen"]); tokenLen > 0 {
		row["tokenMasked"] = maskSecretFromFragments(row["tokenPrefix"], row["tokenSuffix"], tokenLen)
	}
}
