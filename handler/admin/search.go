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

	// Search accounts
	accounts := queryRows(h.db,
		`SELECT a.*, s.name as site_name, s.platform as site_platform
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		 WHERE a.username LIKE ? OR s.name LIKE ? OR s.platform LIKE ?
		 LIMIT ?`,
		likePattern, likePattern, likePattern, perCategory)

	// Search account tokens
	accountTokens := queryRows(h.db,
		`SELECT at.*, a.username as account_username, s.name as site_name
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

// redactSearchAccountSecrets removes plaintext credentials from search account hits.
func redactSearchAccountSecrets(row map[string]any) {
	if row == nil {
		return
	}
	if s, ok := row["accessToken"].(string); ok && strings.TrimSpace(s) != "" {
		row["accessTokenMasked"] = maskSecret(s)
	}
	delete(row, "accessToken")
	if s, ok := row["apiToken"].(string); ok && strings.TrimSpace(s) != "" {
		row["apiTokenMasked"] = maskSecret(s)
	}
	delete(row, "apiToken")
}

// redactSearchTokenSecrets removes plaintext token from search accountTokens hits.
func redactSearchTokenSecrets(row map[string]any) {
	if row == nil {
		return
	}
	if s, ok := row["token"].(string); ok && strings.TrimSpace(s) != "" {
		row["tokenMasked"] = maskSecret(s)
	}
	delete(row, "token")
}
