package admin

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- Disabled Models ----

func (h *sitesHandler) getDisabledModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var existing store.Site
	if err := h.db.Get(&existing, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", id); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "Site not found")
		return
	}

	var models []string
	h.db.Select(&models, "SELECT model_name FROM site_disabled_models WHERE site_id = ?", id)

	writeJSON(w, http.StatusOK, map[string]any{
		"siteId": id,
		"models": models,
	})
}

func (h *sitesHandler) updateDisabledModels(w http.ResponseWriter, r *http.Request) {
	var body payloads.SiteDisabledModelsPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid models. Expected string[].")
		return
	}

	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var existing store.Site
	if err := h.db.Get(&existing, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", id); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "Site not found")
		return
	}

	// Normalize: deduplicate and trim
	seen := map[string]bool{}
	var uniqueModels []string
	for _, m := range body.Models {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			uniqueModels = append(uniqueModels, m)
		}
	}

	// Full replace
	h.db.Exec("DELETE FROM site_disabled_models WHERE site_id = ?", id)

	now := time.Now().UTC().Format(time.RFC3339)
	for _, m := range uniqueModels {
		h.db.Exec("INSERT INTO site_disabled_models (site_id, model_name, created_at) VALUES (?, ?, ?)", id, m, now)
	}

	routing.InvalidateCache()
	writeJSON(w, http.StatusOK, map[string]any{
		"siteId": id,
		"models": uniqueModels,
	})
}

// ---- Available Models ----

func (h *sitesHandler) getAvailableModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var existing store.Site
	if err := h.db.Get(&existing, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", id); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "Site not found")
		return
	}

	// Account-level models
	var accountModels []string
	h.db.Select(&accountModels,
		`SELECT DISTINCT ma.model_name FROM model_availability ma
		 INNER JOIN accounts a ON ma.account_id = a.id
		 WHERE a.site_id = ? AND ma.available = TRUE`, id,
	)

	// Token-level models
	var tokenModels []string
	h.db.Select(&tokenModels,
		`SELECT DISTINCT tma.model_name FROM token_model_availability tma
		 INNER JOIN account_tokens at ON tma.token_id = at.id
		 INNER JOIN accounts a ON at.account_id = a.id
		 WHERE a.site_id = ? AND tma.available = TRUE`, id,
	)

	// Merge, deduplicate, sort
	allModels := append(accountModels, tokenModels...)
	seen := map[string]bool{}
	result := []string{}
	for _, m := range allModels {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			result = append(result, m)
		}
	}
	// Case-insensitive sort
	sortStringsCI(result)

	writeJSON(w, http.StatusOK, map[string]any{
		"siteId": id,
		"models": result,
	})
}

func sortStringsCI(strs []string) {
	sort.Slice(strs, func(i, j int) bool {
		return strings.ToLower(strs[i]) < strings.ToLower(strs[j])
	})
}
