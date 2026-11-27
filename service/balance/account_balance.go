package balance

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/jmoiron/sqlx"
)

// lowBalanceAlertThreshold is the per-account balance floor that triggers a
// real-time low-balance alert after a successful refresh (G1). Matches the TS
// lowBalanceAccounts threshold (balance < 1). Tunable: raise to alert sooner.
const lowBalanceAlertThreshold = 1.0

func recordBalanceFailure(db *sqlx.DB, aws *service.AccountWithSite, err error) error {
	account, site := &aws.Account, &aws.Site
	message := alert.AppendSessionTokenRebindHint(err.Error())
	service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
		State: service.HealthUnhealthy, Reason: message, Source: service.HealthSourceBalance,
	})
	if alert.ShouldMarkAccountExpired(0, message) {
		alert.ReportTokenExpired(config.RuntimeSafe(), db, alert.TokenExpiredParams{
			AccountID: account.ID, Username: account.Username,
			SiteName: &site.Name, Detail: message,
		})
	}
	return fmt.Errorf("%s", message)
}

func saveRefreshedBalance(db *sqlx.DB, aws *service.AccountWithSite, balanceInfo *platform.BalanceInfo) error {
	account, site := &aws.Account, &aws.Site
	// Update todayIncome snapshot
	nextExtraConfig := account.ExtraConfig
	if balanceInfo.TodayIncome != nil && !math.IsNaN(*balanceInfo.TodayIncome) && !math.IsInf(*balanceInfo.TodayIncome, 0) {
		nextExtraConfig = service.UpdateTodayIncomeSnapshot(nextExtraConfig, *balanceInfo.TodayIncome, time.Now())
	}
	if balanceInfo.SubscriptionSummary != nil && service.IsSub2ApiPlatform(site.Platform) {
		subSummary := service.BuildStoredSub2ApiSubscriptionSummary(balanceInfo.SubscriptionSummary)
		nextExtraConfig = service.MergeExtraConfig(nextExtraConfig, map[string]any{
			"sub2apiSubscription": subSummary,
		})
	}

	// Check if unsupported checkin degraded state should be preserved
	existingHealth := service.ExtractRuntimeHealth(nextExtraConfig)
	keepUnsupportedCheckinDegraded := service.IsUnsupportedCheckinRuntimeHealth(existingHealth)

	// Prepare updates
	updates := map[string]any{
		"balance":     balanceInfo.Balance,
		"balanceUsed": balanceInfo.Used,
		"quota":       balanceInfo.Quota,
		"status": func() string {
			if account.Status == "expired" {
				return "active"
			}
			return account.Status
		}(),
		"lastBalanceRefresh": time.Now().UTC().Format(time.RFC3339),
		"updatedAt":          time.Now().UTC().Format(time.RFC3339),
	}

	if nextExtraConfig != nil && account.ExtraConfig != nil && *nextExtraConfig != *account.ExtraConfig {
		updates["extraConfig"] = *nextExtraConfig
	}
	if nextExtraConfig != nil && account.ExtraConfig == nil {
		updates["extraConfig"] = *nextExtraConfig
	}

	// Update DB
	err := service.UpdateAccountFields(db, account.ID, updates)
	if err != nil {
		return fmt.Errorf("%s%w", errSaveBalancePrefix, err)
	}

	// A1: snapshot balance to balance_history so operators
	// get a balance trend. UPSERT per (local_day, account_id): a same-day
	// re-refresh overwrites the earlier snapshot (latest-known balance of the
	// day, not a noisy multi-point curve). Best-effort — a snapshot failure
	// must not break the refresh path.
	recordBalanceSnapshot(db, account.ID, balanceInfo.Balance, balanceInfo.Used, balanceInfo.Quota)

	// Set runtime health
	if keepUnsupportedCheckinDegraded {
		reason := "site does not support check-in endpoint"
		source := service.HealthSourceCheckin
		if existingHealth != nil && existingHealth.Reason != "" {
			reason = existingHealth.Reason
		}
		if existingHealth != nil && existingHealth.Source != "" {
			source = existingHealth.Source
		}
		service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthDegraded, Reason: reason, Source: source,
		})
	} else {
		service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthHealthy, Reason: "balance refresh succeeded", Source: service.HealthSourceBalance,
		})
	}

	// G1: real-time low-balance alert (better than TS daily-summary count).
	// Deduped per account per 24h inside ReportLowBalance so sustained-low
	// balances don't spam. Threshold 1.0 matches TS lowBalanceAccounts.
	if balanceInfo.Balance < lowBalanceAlertThreshold {
		alert.ReportLowBalance(config.RuntimeSafe(), db, alert.LowBalanceParams{
			AccountID: account.ID,
			Username:  account.Username,
			SiteName:  &site.Name,
			Balance:   balanceInfo.Balance,
			Threshold: lowBalanceAlertThreshold,
		})
	}
	return nil
}

// recordBalanceSnapshot UPSERTs a per-day balance snapshot.
// One row per (local_day, account_id); same-day re-refresh overwrites so the
// trend reflects the latest-known balance of the day. Best-effort: errors are
// logged, never returned, so a history write cannot break the refresh path.
// SQL is dialect-unified — both Postgres and SQLite ≥3.24 support
// ON CONFLICT (...) DO UPDATE with the excluded pseudo-row.
func recordBalanceSnapshot(db *sqlx.DB, accountID int64, balance, used, quota float64) {
	if db == nil {
		return
	}
	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	ts := now.Format(time.RFC3339)
	const q = `INSERT INTO balance_history (account_id, balance, balance_used, quota, local_day, captured_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (local_day, account_id) DO UPDATE SET
			balance = excluded.balance,
			balance_used = excluded.balance_used,
			quota = excluded.quota,
			captured_at = excluded.captured_at,
			created_at = excluded.created_at`
	if _, err := db.Exec(db.Rebind(q), accountID, balance, used, quota, day, ts, ts); err != nil {
		slog.Warn("balance_history: snapshot upsert failed (non-fatal)",
			"accountID", accountID, "day", day, "error", err)
	}
}
