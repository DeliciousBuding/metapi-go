package balance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/jmoiron/sqlx"
)

// IsSiteDisabled checks if a site is disabled.
func IsSiteDisabled(status string) bool {
	normalized := strings.TrimSpace(status)
	if normalized == "" {
		normalized = "active"
	}
	return strings.EqualFold(normalized, "disabled")
}

func isAccountDisabled(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "disabled")
}

// platformUserIDPtr converts a resolved int64 platform user id to the
// *int form expected by platform adapter methods (nil when not available).
func platformUserIDPtr(platformUserID int64) *int {
	if platformUserID <= 0 {
		return nil
	}
	value := int(platformUserID)
	return &value
}

// BalanceResult is the result of a balance refresh.
type BalanceResult struct {
	Balance     float64
	Used        float64
	Quota       float64
	Skipped     bool
	Reason      string
	BalanceInfo *platform.BalanceInfo
}

// Balance-refresh failure wording (#1210). These prefixes are how RefreshBalance
// marks the two failures that are local to Metapi rather than upstream, so
// ExplainRefreshFailure can name them without pattern-matching driver text.
const (
	errReadAccountPrefix = "read account for balance refresh: "
	errSaveBalancePrefix = "save refreshed balance: "
)

// ExplainRefreshFailure names why a RefreshBalance error happened, in one compact
// credential-safe line, for the 502 body and the operator-facing account health
// reason. The raw error still reaches the server log in full; this is the part an
// operator is allowed to see.
//
// It owns only the shapes the balance path itself produces and delegates every
// upstream signal to platform.ExplainUpstreamFailure, so the vocabulary keeps a
// single owner. "" means nothing safe could be named, and the caller then keeps
// its bare stable prefix instead of inventing a cause.
func ExplainRefreshFailure(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	switch {
	case message == "":
		return ""
	case strings.HasPrefix(message, "unsupported platform"):
		// The caller answers 404 "account not found or platform not supported"
		// for this one; it is not a refresh failure to explain.
		return ""
	case strings.HasPrefix(message, errReadAccountPrefix):
		return "the account could not be read from the database"
	case strings.HasPrefix(message, errSaveBalancePrefix):
		return "the refreshed balance could not be saved"
	case message == "failed to fetch balance":
		// The adapter got an answer it could not turn into a balance.
		return "upstream returned no usable balance"
	}
	return platform.ExplainUpstreamFailure(0, message)
}

// RefreshBalance refreshes the balance for a single account.
// Mirrors TS refreshBalance().
func RefreshBalance(cfg *config.Config, db *sqlx.DB, accountID int64) (*BalanceResult, error) {
	aws, err := service.GetAccountWithSiteByID(db, accountID)
	if err != nil {
		// A missing row is a legitimate "not found" and the caller answers 404.
		// Every other failure is a read failure and must not be reported as
		// "account not found": `nil, nil` here dressed a fetch failure up as a
		// valid empty answer, which is the shape #1179 made a product rule out of
		// (a failure that cannot be told apart from success).
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s%w", errReadAccountPrefix, err)
	}
	account := &aws.Account
	site := &aws.Site

	// Disabled checks
	if isAccountDisabled(account.Status) {
		service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthDisabled, Reason: "account disabled", Source: service.HealthSourceBalance,
		})
		return &BalanceResult{
			Balance: account.BalanceOrZero(),
			Used:    account.BalanceUsedOrZero(),
			Quota:   account.QuotaOrZero(),
			Skipped: true,
			Reason:  "account_disabled",
		}, nil
	}

	if IsSiteDisabled(site.Status) {
		service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthDisabled, Reason: "site disabled", Source: service.HealthSourceBalance,
		})
		return &BalanceResult{
			Balance: account.BalanceOrZero(),
			Used:    account.BalanceUsedOrZero(),
			Quota:   account.QuotaOrZero(),
			Skipped: true,
			Reason:  "site_disabled",
		}, nil
	}

	// Get adapter
	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return nil, fmt.Errorf("unsupported platform: %s", site.Platform)
	}

	// API key connection check — skip balance refresh for apikey accounts
	if service.IsAPIKeyConnection(account) {
		return &BalanceResult{
			Balance: account.BalanceOrZero(),
			Used:    account.BalanceUsedOrZero(),
			Quota:   account.QuotaOrZero(),
			Skipped: true,
			Reason:  "proxy_only",
		}, nil
	}

	platformUserID := service.ResolvePlatformUserID(account.ExtraConfig, account.Username)
	proxyConfig := service.BuildPlatformProxyConfig(cfg, account, site)
	activeAccessToken := account.AccessToken
	activeExtraConfig := account.ExtraConfig

	// Pre-refresh: Sub2Api managed session
	if service.IsSub2ApiPlatform(site.Platform) {
		managedAuth := service.GetSub2ApiAuthFromExtraConfig(activeExtraConfig)
		if managedAuth != nil && managedAuth["refreshToken"] != nil {
			if service.IsManagedSub2ApiTokenDue(managedAuth["tokenExpiresAt"]) {
				refreshedToken, refErr := refreshSub2ApiManagedSessionSingleflight(cfg, db, account.ID, account, site, managedAuth)
				if refErr == nil && refreshedToken != "" {
					activeAccessToken = refreshedToken
					slog.Info("Sub2Api token refreshed via singleflight", "accountID", accountID)
				} else {
					slog.Warn("Sub2Api token refresh failed", "accountID", accountID, "error", refErr)
				}
			}
		}
	}

	// Read balance helper
	readBalance := func(token string) (*platform.BalanceInfo, error) {
		return adp.GetBalance(context.Background(), site.URL, token, platformUserIDPtr(platformUserID), proxyConfig)
	}

	// First attempt
	balanceInfo, err := readBalance(activeAccessToken)
	if err != nil {
		errMsg := err.Error()

		// Sub2Api managed refresh retry
		isSub2Api := service.IsSub2ApiPlatform(site.Platform)
		managedAuth := service.GetSub2ApiAuthFromExtraConfig(activeExtraConfig)
		if isSub2Api && managedAuth != nil && managedAuth["refreshToken"] != nil && shouldAttemptAutoReloginBalance(errMsg) {
			refreshedToken, refErr := refreshSub2ApiManagedSessionSingleflight(cfg, db, account.ID, account, site, managedAuth)
			if refErr == nil && refreshedToken != "" {
				activeAccessToken = refreshedToken
				balanceInfo, err = readBalance(activeAccessToken)
				if err != nil {
					return nil, recordBalanceFailure(db, aws, err)
				}
				// Successfully retried with refreshed token — skip auto-relogin below
				goto afterRetry
			}
		}

		if shouldAttemptAutoReloginBalance(errMsg) {
			refreshedToken, reloginErr := tryAutoReloginBalance(cfg, db, account, site)
			if reloginErr == nil && refreshedToken != "" {
				activeAccessToken = refreshedToken
				balanceInfo, err = readBalance(activeAccessToken)
				if err != nil {
					return nil, recordBalanceFailure(db, aws, err)
				}
			} else {
				return nil, recordBalanceFailure(db, aws, err)
			}
		} else {
			return nil, recordBalanceFailure(db, aws, err)
		}
	}

afterRetry:
	if balanceInfo == nil {
		return nil, fmt.Errorf("failed to fetch balance")
	}

	// Today income fallback from logs
	if balanceInfo.TodayIncome == nil || math.IsNaN(*balanceInfo.TodayIncome) || math.IsInf(*balanceInfo.TodayIncome, 0) {
		if supportsTodayIncomeLogFallback(site.Platform) {
			fallbackIncome, fallbackErr := fetchTodayIncomeFromLogs(site.URL, activeAccessToken, site.Platform, platformUserID, proxyConfig)
			if fallbackErr == nil {
				balanceInfo.TodayIncome = &fallbackIncome
			}
		}
	}

	if err := saveRefreshedBalance(db, aws, balanceInfo); err != nil {
		return nil, err
	}

	return &BalanceResult{
		Balance:     balanceInfo.Balance,
		Used:        balanceInfo.Used,
		Quota:       balanceInfo.Quota,
		Skipped:     false,
		BalanceInfo: balanceInfo,
	}, nil
}

// RefreshAllBalances refreshes balances for all active accounts.
// Mirrors TS refreshAllBalances().
func RefreshAllBalances(cfg *config.Config, db *sqlx.DB) []struct {
	AccountID int64
	Balance   *float64
} {
	// Select only IDs: a SELECT * here would scan every column into store.Account,
	// whose balance/balance_used/quota are non-nullable float64 — a NULL balance
	// (migrated/never-refreshed account) would abort the whole refresh job.
	var accountIDs []int64
	if err := db.Select(&accountIDs, "SELECT id FROM accounts WHERE status = 'active'"); err != nil {
		slog.Error("RefreshAllBalances: failed to query accounts", "error", err)
		return nil
	}

	type resultEntry struct {
		AccountID int64
		Balance   *float64
	}

	results := make([]resultEntry, len(accountIDs))
	var wg sync.WaitGroup

	for i, accountID := range accountIDs {
		wg.Add(1)
		go func(idx int, accID int64) {
			defer wg.Done()
			balanceResult, err := RefreshBalance(cfg, db, accID)
			if err != nil || balanceResult == nil {
				results[idx] = resultEntry{AccountID: accID, Balance: nil}
			} else {
				b := balanceResult.Balance
				results[idx] = resultEntry{AccountID: accID, Balance: &b}
			}
		}(i, accountID)
	}
	wg.Wait()

	// Convert to export type
	out := make([]struct {
		AccountID int64
		Balance   *float64
	}, len(results))
	for i, r := range results {
		out[i].AccountID = r.AccountID
		out[i].Balance = r.Balance
	}
	return out
}
