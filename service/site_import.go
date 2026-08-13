package service

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ImportDuplicateStrategy controls how already-present sites are handled.
type ImportDuplicateStrategy string

const (
	ImportDuplicateSkip  ImportDuplicateStrategy = "skip"
	ImportDuplicateMerge ImportDuplicateStrategy = "merge"
)

// ImportAccountInput is a lightweight account definition attached to an
// imported site. Imported accounts are stored as skipModelFetch API-key style
// credentials: the import wizard defers upstream model discovery to the normal
// background refresh instead of blocking the batch on per-account verification.
type ImportAccountInput struct {
	Username    *string `json:"username,omitempty"`
	AccessToken string  `json:"accessToken,omitempty"`
	APIToken    string  `json:"apiToken,omitempty"`
}

// ImportSiteInput is one candidate site in a batch import.
type ImportSiteInput struct {
	Name              string                  `json:"name"`
	URL               string                  `json:"url"`
	Platform          string                  `json:"platform,omitempty"`
	GlobalWeight      *float64                `json:"globalWeight,omitempty"`
	MaxConcurrency    *int64                  `json:"maxConcurrency,omitempty"`
	DuplicateStrategy ImportDuplicateStrategy `json:"duplicateStrategy,omitempty"`
	Accounts          []ImportAccountInput    `json:"accounts,omitempty"`
}

// ImportSiteItemResult is the per-item outcome of a batch import.
type ImportSiteItemResult struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status"` // imported | merged | skipped | failed
	Reason string `json:"reason,omitempty"`
	SiteID int64  `json:"siteId,omitempty"`
}

// ImportSitesResult is the batch import summary. Imported counts sites that
// were newly created plus sites whose accounts were merged into an existing
// site; skipped counts idempotent duplicate no-ops; failed counts hard errors.
type ImportSitesResult struct {
	Imported int                    `json:"imported"`
	Skipped  int                    `json:"skipped"`
	Failed   int                    `json:"failed"`
	Results  []ImportSiteItemResult `json:"results"`
}

// ImportSites creates sites (and optional accounts) idempotently. A site is
// considered a duplicate when (platform, canonical url) already exists; with
// skip it is a no-op, with merge any provided accounts are attached to the
// existing site. Re-running the same payload therefore converges to
// imported=0, skipped=N.
func ImportSites(db *sqlx.DB, items []ImportSiteInput, strategy ImportDuplicateStrategy) (*ImportSitesResult, error) {
	if strategy == "" {
		strategy = ImportDuplicateSkip
	}

	result := &ImportSitesResult{Results: []ImportSiteItemResult{}}
	for i := range items {
		item := &items[i]
		itemResult := importOneSite(db, *item, strategy)
		switch itemResult.Status {
		case "imported", "merged":
			result.Imported++
		case "skipped":
			result.Skipped++
		case "failed":
			result.Failed++
		}
		result.Results = append(result.Results, itemResult)
	}
	return result, nil
}

func importOneSite(db *sqlx.DB, item ImportSiteInput, strategy ImportDuplicateStrategy) ImportSiteItemResult {
	effectiveStrategy := strategy
	if item.DuplicateStrategy != "" {
		effectiveStrategy = item.DuplicateStrategy
	}
	name := strings.TrimSpace(item.Name)
	rawURL := strings.TrimSpace(item.URL)
	canonicalURL := CanonicalizeSiteURL(rawURL)

	base := ImportSiteItemResult{Name: name, URL: canonicalURL}

	if rawURL == "" {
		base.Reason = "Invalid url. Expected non-empty string."
		base.Status = "failed"
		return base
	}
	if name == "" {
		base.Reason = "Invalid name. Expected non-empty string."
		base.Status = "failed"
		return base
	}
	if !IsValidHTTPURL(rawURL) {
		base.Reason = "Invalid url. Expected a valid http(s) URL."
		base.Status = "failed"
		return base
	}

	platform := strings.TrimSpace(strings.ToLower(item.Platform))
	if platform == "" {
		if detected := DetectSite(rawURL); detected != nil {
			platform = detected.Platform
		}
	}
	if platform == "" {
		base.Reason = "Could not detect platform. Please specify manually."
		base.Status = "failed"
		return base
	}

	// Idempotent duplicate handling.
	existing, err := findSiteByPlatformURL(db, platform, canonicalURL)
	if err != nil {
		base.Reason = err.Error()
		base.Status = "failed"
		return base
	}
	if existing != nil {
		if effectiveStrategy == ImportDuplicateMerge {
			merged, mergeErr := mergeAccountsIntoExistingSite(db, existing.ID, item.Accounts)
			if mergeErr != nil {
				base.Reason = mergeErr.Error()
				base.Status = "failed"
				return base
			}
			base.SiteID = existing.ID
			if merged == 0 {
				base.Status = "skipped"
				base.Reason = "Duplicate site (no accounts to merge)."
				return base
			}
			base.Status = "merged"
			base.Reason = fmt.Sprintf("Merged %d account(s) into existing site.", merged)
			return base
		}
		base.SiteID = existing.ID
		base.Status = "skipped"
		base.Reason = "Duplicate site."
		return base
	}

	siteID, err := createImportedSite(db, item, platform, canonicalURL)
	if err != nil {
		base.Reason = err.Error()
		base.Status = "failed"
		return base
	}
	base.SiteID = siteID

	if _, err := mergeAccountsIntoExistingSite(db, siteID, item.Accounts); err != nil {
		// The site was created; account import failure is non-fatal to keep the
		// site idempotency sane. Record it in the reason but keep "imported".
		base.Reason = err.Error()
	}
	base.Status = "imported"
	return base
}

func findSiteByPlatformURL(db *sqlx.DB, platform, canonicalURL string) (*store.Site, error) {
	var site store.Site
	err := db.Get(&site, db.Rebind("SELECT "+SiteSelectColumns+" FROM sites WHERE platform = ? AND url = ?"), platform, canonicalURL)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &site, nil
}

func createImportedSite(db *sqlx.DB, item ImportSiteInput, platform, canonicalURL string) (int64, error) {
	globalWeight := 1.0
	if item.GlobalWeight != nil && *item.GlobalWeight > 0 {
		globalWeight = *item.GlobalWeight
	}
	maxConcurrency := int64(0)
	if item.MaxConcurrency != nil && *item.MaxConcurrency >= 0 {
		maxConcurrency = *item.MaxConcurrency
	}

	siteData := map[string]any{
		"name":                                strings.TrimSpace(item.Name),
		"url":                                 canonicalURL,
		"platform":                            platform,
		"status":                              "active",
		"isPinned":                            false,
		"globalWeight":                        globalWeight,
		"maxConcurrency":                      maxConcurrency,
		"postRefreshProbeEnabled":             false,
		"postRefreshProbeModel":               "",
		"postRefreshProbeScope":               "single",
		"postRefreshProbeLatencyThresholdMs":  int64(0),
		"proxyUrl":                            nil,
		"useSystemProxy":                      false,
		"customHeaders":                       nil,
		"customHeadersOverrideRequestHeaders": false,
		"externalCheckinUrl":                  nil,
	}
	return CreateSite(db, siteData)
}

func mergeAccountsIntoExistingSite(db *sqlx.DB, siteID int64, accounts []ImportAccountInput) (int, error) {
	merged := 0
	for i := range accounts {
		acct := &accounts[i]
		token := strings.TrimSpace(acct.AccessToken)
		apiToken := strings.TrimSpace(acct.APIToken)
		if token == "" && apiToken == "" {
			continue
		}

		dupe, err := findDuplicateAccount(db, siteID, token, apiToken)
		if err != nil {
			return merged, err
		}
		if dupe {
			continue
		}

		if _, err := insertImportedAccount(db, siteID, acct, token, apiToken); err != nil {
			return merged, err
		}
		merged++
	}
	return merged, nil
}

func findDuplicateAccount(db *sqlx.DB, siteID int64, accessToken, apiToken string) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM accounts WHERE site_id = ? AND ((? <> '' AND access_token = ?) OR (? <> '' AND api_token = ?))`
	args := []any{siteID, accessToken, accessToken, apiToken, apiToken}
	if err := db.Get(&count, db.Rebind(query), args...); err != nil {
		return false, err
	}
	return count > 0, nil
}

func insertImportedAccount(db *sqlx.DB, siteID int64, acct *ImportAccountInput, accessToken, apiToken string) (int64, error) {
	sortOrder, err := GetNextAccountSortOrder(db)
	if err != nil {
		return 0, err
	}

	var username *string
	if acct.Username != nil && strings.TrimSpace(*acct.Username) != "" {
		u := strings.TrimSpace(*acct.Username)
		username = &u
	}
	var apiTokenPtr *string
	if apiToken != "" {
		t := apiToken
		apiTokenPtr = &t
	}
	extraConfig := map[string]any{
		"credentialMode": string(CredentialModeAPIKey),
		"skipModelFetch": true,
	}

	account := map[string]any{
		"siteId":             siteID,
		"username":           username,
		"accessToken":        accessToken,
		"apiToken":           apiTokenPtr,
		"balance":            0.0,
		"balanceUsed":        0.0,
		"quota":              0.0,
		"unitCost":           nil,
		"valueScore":         0.0,
		"status":             "active",
		"isPinned":           false,
		"sortOrder":          sortOrder,
		"checkinEnabled":     false,
		"lastCheckinAt":      nil,
		"lastBalanceRefresh": nil,
		"oauthProvider":      nil,
		"oauthAccountKey":    nil,
		"oauthProjectID":     nil,
		"extraConfig":        extraConfig,
	}
	return InsertAccount(db, account)
}
