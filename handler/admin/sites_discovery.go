package admin

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// listInitializationPresets shares the creation/detection registry with the UI.
func (h *sitesHandler) listInitializationPresets(w http.ResponseWriter, r *http.Request) {
	type presetResponse struct {
		ID                        string   `json:"id"`
		Label                     string   `json:"label"`
		ProviderLabel             string   `json:"providerLabel"`
		Platform                  string   `json:"platform"`
		DefaultURL                string   `json:"defaultUrl"`
		RecommendedSkipModelFetch bool     `json:"recommendedSkipModelFetch"`
		RecommendedModels         []string `json:"recommendedModels"`
		DocsURL                   string   `json:"docsUrl"`
	}
	presets := service.ListSiteInitializationPresets()
	result := make([]presetResponse, 0, len(presets))
	for _, preset := range presets {
		result = append(result, presetResponse{
			ID: preset.ID, Label: preset.Label, ProviderLabel: preset.ProviderLabel,
			Platform: preset.Platform, DefaultURL: preset.DefaultURL,
			RecommendedSkipModelFetch: preset.RecommendedSkipModelFetch,
			RecommendedModels:         append([]string{}, preset.RecommendedModels...),
			DocsURL:                   preset.DocsURL,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// ---- Detect Platform ----

func (h *sitesHandler) detectSite(w http.ResponseWriter, r *http.Request) {
	var body payloads.SiteDetectPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected non-empty string.")
		return
	}
	if strings.TrimSpace(body.URL) == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid url. Expected non-empty string.")
		return
	}

	result := service.DetectSite(body.URL)
	if result != nil {
		writeJSON(w, http.StatusOK, result)
		return
	}
	// Failure must not use HTTP 200 with an error body.
	writeErrorWithRequest(w, r, http.StatusBadRequest, "Could not detect platform")
}

// ---- Import Sites (batch, idempotent) ----

func (h *sitesHandler) importSites(w http.ResponseWriter, r *http.Request) {
	var body payloads.SiteImportPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid import payload.")
		return
	}
	if len(body.Items) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "items is required")
		return
	}

	strategy := service.ImportDuplicateStrategy(strings.TrimSpace(body.DuplicateStrategy))
	if strategy == "" {
		strategy = service.ImportDuplicateSkip
	}
	if strategy != service.ImportDuplicateSkip && strategy != service.ImportDuplicateMerge {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid duplicateStrategy. Expected skip or merge.")
		return
	}

	items := make([]service.ImportSiteInput, 0, len(body.Items))
	for _, item := range body.Items {
		platform := ""
		if item.Platform != nil {
			platform = *item.Platform
		}
		accounts := make([]service.ImportAccountInput, 0, len(item.Accounts))
		for _, acct := range item.Accounts {
			accounts = append(accounts, service.ImportAccountInput{
				Username:    acct.Username,
				AccessToken: acct.AccessToken,
				APIToken:    acct.APIToken,
			})
		}
		items = append(items, service.ImportSiteInput{
			Name:              item.Name,
			URL:               item.URL,
			Platform:          platform,
			GlobalWeight:      item.GlobalWeight,
			MaxConcurrency:    item.MaxConcurrency,
			DuplicateStrategy: service.ImportDuplicateStrategy(item.DuplicateStrategy),
			Accounts:          accounts,
		})
	}

	result, err := service.ImportSites(h.db.DB, items, strategy)
	if err != nil {
		slog.Error("ImportSites failed", "err", err)
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Import sites failed")
		return
	}

	if result.Imported > 0 {
		service.InvalidateSiteCaches()
		routing.InvalidateCache()
	}
	writeJSON(w, http.StatusOK, result)
}
