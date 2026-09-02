package oauth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// OauthQuotaWindowSnapshot represents a single quota window.
type OauthQuotaWindowSnapshot struct {
	Supported bool     `json:"supported"`
	Limit     *float64 `json:"limit,omitempty"`
	Used      *float64 `json:"used,omitempty"`
	Remaining *float64 `json:"remaining,omitempty"`
	ResetAt   string   `json:"resetAt,omitempty"`
	Message   string   `json:"message,omitempty"`
}

// OauthQuotaWindows holds the two quota windows.
type OauthQuotaWindows struct {
	FiveHour *OauthQuotaWindowSnapshot `json:"fiveHour"`
	SevenDay *OauthQuotaWindowSnapshot `json:"sevenDay"`
}

// OauthQuotaSnapshot is a full quota snapshot.
type OauthQuotaSnapshot struct {
	Status           string             `json:"status"`
	Source           string             `json:"source"`
	LastSyncAt       string             `json:"lastSyncAt,omitempty"`
	LastError        string             `json:"lastError,omitempty"`
	ProviderMessage  string             `json:"providerMessage,omitempty"`
	Subscription     *OauthSubscription `json:"subscription,omitempty"`
	Windows          *OauthQuotaWindows `json:"windows"`
	LastLimitResetAt string             `json:"lastLimitResetAt,omitempty"`
}

// OauthSubscription holds subscription info.
type OauthSubscription struct {
	PlanType    string `json:"planType,omitempty"`
	ActiveStart string `json:"activeStart,omitempty"`
	ActiveUntil string `json:"activeUntil,omitempty"`
}

// ---- Quota Refresh ----

// RefreshOauthQuotaSnapshot refreshes the quota snapshot for an OAuth account.
// Only implemented for Codex provider; non-codex accounts get an "unsupported" snapshot.
func RefreshOauthQuotaSnapshot(accountID int64) (*OauthQuotaSnapshot, error) {
	db := store.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return nil, fmt.Errorf("oauth account not found")
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil {
		return nil, fmt.Errorf("account is not managed by oauth")
	}

	if oauth.Provider != "codex" {
		// Non-codex providers: build "unsupported" snapshot and persist.
		snapshot := buildUnsupportedQuotaSnapshot()
		persistQuotaSnapshot(db, accountID, snapshot)
		return snapshot, nil
	}

	// Codex: probe request with rate limit header parsing.
	_ = account.ExtraConfig // preserved for future proxy URL resolution from config
	proxyURL := resolveAccountProxyURLForQuota(account.SiteID, account.ExtraConfig)

	// Build the probe snapshot by making a request to the upstream.
	// In production this makes an HTTP request and parses rate-limit headers.
	// For now, we build an error snapshot since we don't have HTTP client wiring here.
	snapshot := buildQuotaSnapshotFromProbe(proxyURL)
	persistQuotaSnapshot(db, accountID, snapshot)
	return snapshot, nil
}

// RefreshOauthConnectionQuotaBatch refreshes quota for multiple accounts concurrently.
// Runs with max concurrency of 4 workers.
func RefreshOauthConnectionQuotaBatch(accountIDs []int64) *QuotaBatchResult {
	// Deduplicate account IDs.
	seen := make(map[int64]bool)
	unique := make([]int64, 0, len(accountIDs))
	for _, id := range accountIDs {
		if id > 0 && !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	result := &QuotaBatchResult{
		Items: make([]QuotaBatchItem, 0, len(unique)),
	}

	if len(unique) == 0 {
		result.Success = true
		return result
	}

	// Run concurrently with max 4 workers.
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, accountID := range unique {
		wg.Add(1)
		sem <- struct{}{}
		go func(id int64) {
			defer wg.Done()
			defer func() { <-sem }()

			quota, err := RefreshOauthConnectionQuota(id)
			mu.Lock()
			if err != nil {
				result.Items = append(result.Items, QuotaBatchItem{
					AccountID: id,
					Success:   false,
					Error:     err.Error(),
				})
			} else {
				result.Items = append(result.Items, QuotaBatchItem{
					AccountID: id,
					Success:   true,
					Quota:     quota,
				})
			}
			mu.Unlock()
		}(accountID)
	}
	wg.Wait()

	result.Refreshed = 0
	result.Failed = 0
	for _, item := range result.Items {
		if item.Success {
			result.Refreshed++
		} else {
			result.Failed++
		}
	}
	result.Success = result.Failed == 0
	return result
}

// RefreshOauthConnectionQuota wraps RefreshOauthQuotaSnapshot for a single account.
func RefreshOauthConnectionQuota(accountID int64) (*OauthQuotaSnapshot, error) {
	return RefreshOauthQuotaSnapshot(accountID)
}

// QuotaBatchResult holds the result of a batch quota refresh.
type QuotaBatchResult struct {
	Success   bool             `json:"success"`
	Refreshed int              `json:"refreshed"`
	Failed    int              `json:"failed"`
	Items     []QuotaBatchItem `json:"items"`
}

// QuotaBatchItem represents a single item in a batch quota refresh result.
type QuotaBatchItem struct {
	AccountID int64               `json:"accountId"`
	Success   bool                `json:"success"`
	Quota     *OauthQuotaSnapshot `json:"quota,omitempty"`
	Error     string              `json:"error,omitempty"`
}

// ---- Internal helpers ----

func buildUnsupportedQuotaSnapshot() *OauthQuotaSnapshot {
	return &OauthQuotaSnapshot{
		Status:     "unsupported",
		Source:     "reverse_engineered",
		LastSyncAt: time.Now().Format(time.RFC3339),
		Windows: &OauthQuotaWindows{
			FiveHour: &OauthQuotaWindowSnapshot{Supported: false},
			SevenDay: &OauthQuotaWindowSnapshot{Supported: false},
		},
	}
}

func buildQuotaSnapshotFromProbe(proxyURL *string) *OauthQuotaSnapshot {
	// In production, this would make an HTTP POST /responses probe request
	// to the Codex upstream with SelectCodexQuotaProbeModel(discovered)
	// (prefers gpt-5.5 / gpt-5.x family — not obsolete-only gpt-5.4) and
	// parse the rate-limit headers (x-codex-primary-*, x-codex-secondary-*).
	// For now, return a placeholder error snapshot indicating the probe
	// infrastructure is not yet wired; model selection is exercised via
	// SelectCodexQuotaProbeModel unit tests.
	_ = SelectCodexQuotaProbeModel(nil)
	_ = proxyURL
	return &OauthQuotaSnapshot{
		Status:     "error",
		Source:     "reverse_engineered",
		LastSyncAt: time.Now().Format(time.RFC3339),
		LastError:  "quota probe HTTP client not yet wired",
		Windows: &OauthQuotaWindows{
			FiveHour: &OauthQuotaWindowSnapshot{Supported: false},
			SevenDay: &OauthQuotaWindowSnapshot{Supported: false},
		},
	}
}

// CodexQuotaProbeModelForAccount resolves the model id used for a Codex
// reverse-engineered quota probe. Prefer lastDiscoveredModels when present so
// gpt-5.5 is used once discovery has seen it; otherwise fall back to the
// version-flexible preference list (gpt-5.5 first).
func CodexQuotaProbeModelForAccount(oauth *OauthInfo) string {
	var discovered []string
	if oauth != nil {
		discovered = oauth.LastDiscoveredModels
	}
	return SelectCodexQuotaProbeModel(discovered)
}

func persistQuotaSnapshot(db *store.DB, accountID int64, snapshot *OauthQuotaSnapshot) {
	if snapshot == nil {
		return
	}
	quotaJSON, err := json.Marshal(snapshot)
	if err != nil {
		slog.Warn("failed to serialize quota snapshot", "error", err)
		return
	}

	// Merge into extraConfig.oauth.quota.
	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return
	}

	var quotaMap map[string]interface{}
	if err := json.Unmarshal(quotaJSON, &quotaMap); err != nil {
		return
	}

	extraPatch := map[string]interface{}{
		"oauth": map[string]interface{}{
			"quota": quotaMap,
		},
	}
	extraConfig := MergeAccountExtraConfig(account.ExtraConfig, extraPatch)
	now := time.Now().Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?"), extraConfig, now, accountID); err != nil {
		slog.Warn("oauth quota persist failed", "error", err, "account_id", accountID)
	}
}

func resolveAccountProxyURLForQuota(siteID int64, extraConfig *string) *string {
	proxyURL := GetProxyURLFromExtraConfig(extraConfig)
	if proxyURL != "" {
		return &proxyURL
	}
	return nil
}
