package daily

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tokendancelab/metapi-go/service"
	"github.com/tokendancelab/metapi-go/service/checkin"
	"github.com/tokendancelab/metapi-go/store"
)

// AccountTodayMetrics holds per-account today truth. Status semantics match
// CollectDailySummaryMetrics exactly: complete = fully observable (0 is a real
// zero), partial = rows exist but some fields are missing.
type AccountTodayMetrics struct {
	Reward       float64
	RewardStatus string // complete | partial
	RewardReason string
	Spend        float64
	SpendStatus  string // complete | partial
	SpendReason  string
	Tokens       int64
	ProxyTotal   int
	ProxySuccess int
	ProxyFailed  int
	ProxyUnknown int
}

// legacyCreatedAtExpr normalizes legacy TEXT timestamps ('YYYY-MM-DD HH:MM:SS')
// to ISO 8601 so they compare consistently with RFC 3339 writes. Supported by
// both SQLite and PostgreSQL.
func legacyCreatedAtExpr(alias string) string {
	return "(CASE WHEN SUBSTR(" + alias + ".created_at, 11, 1) = ' ' " +
		"THEN REPLACE(SUBSTR(" + alias + ".created_at, 1, 19), ' ', 'T') || 'Z' " +
		"ELSE " + alias + ".created_at END)"
}

// CollectPerAccountTodayMetrics aggregates today's checkin reward and proxy
// spend per account. It shares the exact semantics of
// CollectDailySummaryMetrics: same local-day window, same reward parser and
// income fallback, active sites only. Accounts with no rows today have no
// entry — callers treat that as 0/complete (real zero, not missing).
func CollectPerAccountTodayMetrics(db *sqlx.DB, now time.Time) (map[int64]*AccountTodayMetrics, error) {
	dayRange := service.GetLocalDayRangeUTC(now)
	out := make(map[int64]*AccountTodayMetrics)

	// Today's checkin rows (same window and filtering as the daily summary).
	var checkinRows []struct {
		store.CheckinLog
		AccountID   int64   `db:"account_id"`
		ExtraConfig *string `db:"extra_config"`
	}
	err := db.Select(&checkinRows, db.Rebind(`
		SELECT cl.*, a.extra_config FROM checkin_logs cl
		INNER JOIN accounts a ON cl.account_id = a.id
		INNER JOIN sites s ON a.site_id = s.id
		WHERE `+legacyCreatedAtExpr("cl")+` >= ?
		AND `+legacyCreatedAtExpr("cl")+` < ?
		AND s.status = 'active'
	`), dayRange.StartUTC, dayRange.EndUTC)
	if err != nil {
		return nil, fmt.Errorf("load per-account checkin logs: %w", err)
	}

	successCountByAccount := make(map[int64]int)
	parsedRewardCountByAccount := make(map[int64]int)
	rewardByAccount := make(map[int64]float64)
	extraConfigByAccount := make(map[int64]*string)
	for _, row := range checkinRows {
		if row.Status != "success" {
			continue
		}
		successCountByAccount[row.AccountID]++
		extraConfigByAccount[row.AccountID] = row.ExtraConfig
		rewardVal := checkin.ParseCheckinRewardAmount(row.Reward)
		if rewardVal <= 0 && row.Message != nil {
			rewardVal = checkin.ParseCheckinRewardAmount(*row.Message)
		}
		if rewardVal > 0 {
			rewardByAccount[row.AccountID] += rewardVal
			parsedRewardCountByAccount[row.AccountID]++
		}
	}
	for accountID, successCount := range successCountByAccount {
		m := out[accountID]
		if m == nil {
			m = &AccountTodayMetrics{}
			out[accountID] = m
		}
		parsedRewardCount := parsedRewardCountByAccount[accountID]
		parsedReward := rewardByAccount[accountID]
		m.Reward = Round6(service.EstimateRewardWithTodayIncomeFallback(service.EstimateRewardInput{
			Day:               dayRange.LocalDay,
			SuccessCount:      successCount,
			ParsedRewardCount: parsedRewardCount,
			RewardSum:         parsedReward,
			ExtraConfig:       extraConfigByAccount[accountID],
		}))
		if successCount > parsedRewardCount && m.Reward <= parsedReward {
			m.RewardStatus = "partial"
			m.RewardReason = "source_partial"
		}
	}

	// Today's proxy rows grouped by account (RFC 3339 writes, no normalization
	// needed; unattributed rows cannot belong to any account).
	type proxyAgg struct {
		AccountID   int64   `db:"account_id"`
		Total       int64   `db:"total"`
		Success     int64   `db:"success"`
		Failed      int64   `db:"failed"`
		Unknown     int64   `db:"unknown"`
		MissingCost int64   `db:"missing_cost"`
		TotalTokens int64   `db:"total_tokens"`
		TotalCost   float64 `db:"total_cost"`
	}
	var proxyRows []proxyAgg
	err = db.Select(&proxyRows, db.Rebind(`
		SELECT
			pl.account_id,
			COUNT(*) AS total,
			COALESCE(SUM(CASE WHEN pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success,
			COALESCE(SUM(CASE WHEN pl.status IS NOT NULL AND pl.status != 'success' THEN 1 ELSE 0 END), 0) AS failed,
			COALESCE(SUM(CASE WHEN pl.status IS NULL THEN 1 ELSE 0 END), 0) AS unknown,
			COALESCE(SUM(CASE WHEN pl.estimated_cost IS NULL THEN 1 ELSE 0 END), 0) AS missing_cost,
			COALESCE(SUM(
				CASE
					WHEN COALESCE(pl.total_tokens, 0) > 0 THEN COALESCE(pl.total_tokens, 0)
					ELSE COALESCE(pl.prompt_tokens, 0) + COALESCE(pl.completion_tokens, 0)
				END
			), 0) AS total_tokens,
			COALESCE(SUM(COALESCE(pl.estimated_cost, 0)), 0) AS total_cost
		FROM proxy_logs pl
		INNER JOIN accounts a ON a.id = pl.account_id
		INNER JOIN sites s ON s.id = a.site_id
		WHERE pl.created_at >= ? AND pl.created_at < ? AND s.status = 'active'
		GROUP BY pl.account_id
	`), dayRange.StartUTC, dayRange.EndUTC)
	if err != nil {
		return nil, fmt.Errorf("load per-account proxy metrics: %w", err)
	}
	for _, row := range proxyRows {
		m := out[row.AccountID]
		if m == nil {
			m = &AccountTodayMetrics{}
			out[row.AccountID] = m
		}
		m.Spend = Round6(row.TotalCost)
		m.Tokens = row.TotalTokens
		m.ProxyTotal = int(row.Total)
		m.ProxySuccess = int(row.Success)
		m.ProxyFailed = int(row.Failed)
		m.ProxyUnknown = int(row.Unknown)
		if row.MissingCost > 0 {
			m.SpendStatus = "partial"
			m.SpendReason = "source_partial"
		}
		if row.Unknown > 0 {
			m.SpendStatus = "partial"
			m.SpendReason = "legacy_unknown"
		}
	}

	// Normalize status: only partial is set explicitly; everything else is
	// complete (fully observable — including zero).
	for _, m := range out {
		if m.RewardStatus == "" {
			m.RewardStatus = "complete"
		}
		if m.SpendStatus == "" {
			m.SpendStatus = "complete"
		}
	}

	return out, nil
}
