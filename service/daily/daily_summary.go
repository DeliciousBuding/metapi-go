package daily

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/checkin"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/deliciousbuding/metapi-go/store"
)

// DailySummaryMetrics holds daily summary statistics.
type DailySummaryMetrics struct {
	LocalDay           string
	WindowStartUTC     string
	WindowEndUTC       string
	GeneratedAtLocal   string
	TimeZone           string
	TotalAccounts      int
	ActiveAccounts     int
	LowBalanceAccounts int
	CheckinTotal       int
	CheckinSuccess     int
	CheckinSkipped     int
	CheckinFailed      int
	ProxyTotal         int
	ProxySuccess       int
	ProxyFailed        int
	ProxyUnknown       int
	ProxyUnattributed  int
	ProxyMissingCost   int
	ProxyMetricStatus  string
	ProxyMetricReason  string
	ProxyTotalTokens   int64
	TodaySpend         float64
	TodaySpendStatus   string
	TodaySpendReason   string
	TodayReward        float64
	TodayRewardStatus  string
	TodayRewardReason  string
}

// CollectDailySummaryMetrics aggregates daily metrics from the database.
// Mirrors TS collectDailySummaryMetrics().
func CollectDailySummaryMetrics(db *sqlx.DB, now time.Time) (*DailySummaryMetrics, error) {
	dayRange := service.GetLocalDayRangeUTC(now)
	timeZone, _ := now.Zone()

	// Accounts on active sites
	var accountRows []struct {
		store.Account
	}
	err := db.Select(&accountRows, `
		SELECT a.* FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE s.status = 'active'
	`)
	if err != nil {
		return nil, fmt.Errorf("load active-site accounts: %w", err)
	}

	activeAccounts := 0
	lowBalanceAccounts := 0
	for _, a := range accountRows {
		if a.Status == "active" {
			activeAccounts++
		}
		if a.Balance < 1 {
			lowBalanceAccounts++
		}
	}

	// Today's checkin logs
	var checkinRows []struct {
		store.CheckinLog
		AccountID int64 `db:"account_id"`
	}
	err = db.Select(&checkinRows, db.Rebind(`
		SELECT cl.* FROM checkin_logs cl
		INNER JOIN accounts a ON cl.account_id = a.id
		INNER JOIN sites s ON a.site_id = s.id
		WHERE `+legacyCreatedAtExpr("cl")+` >= ?
		AND `+legacyCreatedAtExpr("cl")+` < ?
		AND s.status = 'active'
	`), dayRange.StartUTC, dayRange.EndUTC)
	if err != nil {
		return nil, fmt.Errorf("load local-day checkin logs: %w", err)
	}

	checkinSuccess := 0
	checkinSkipped := 0
	checkinFailed := 0
	rewardByAccount := make(map[int64]float64)
	successCountByAccount := make(map[int64]int)
	parsedRewardCountByAccount := make(map[int64]int)

	for _, row := range checkinRows {
		switch row.Status {
		case "success":
			checkinSuccess++
		case "skipped":
			checkinSkipped++
		case "failed":
			checkinFailed++
		default:
			checkinSuccess++
		}
		if row.Status == "success" {
			successCountByAccount[row.AccountID]++
			rewardVal := checkin.ParseCheckinRewardAmount(row.Reward)
			if rewardVal <= 0 && row.Message != nil {
				rewardVal = checkin.ParseCheckinRewardAmount(*row.Message)
			}
			if rewardVal > 0 {
				rewardByAccount[row.AccountID] += rewardVal
				parsedRewardCountByAccount[row.AccountID]++
			}
		}
	}

	// Today's proxy logs
	var proxyTotal, proxySuccess, proxyFailed, proxyUnknown, proxyUnattributed int
	var proxyTotalTokens int64
	var todaySpend float64
	todaySpendStatus := "complete"
	todaySpendReason := ""
	proxyMetricStatus := "complete"
	proxyMetricReason := ""

	// Query proxy_logs for today's metrics
	type proxyLogAgg struct {
		Count              *int64   `db:"count"`
		SuccessCount       *int64   `db:"success_count"`
		FailedCount        *int64   `db:"failed_count"`
		UnknownStatusCount *int64   `db:"unknown_status_count"`
		UnattributedCount  *int64   `db:"unattributed_count"`
		MissingCostCount   *int64   `db:"missing_cost_count"`
		TotalTokens        *int64   `db:"total_tokens"`
		TotalCost          *float64 `db:"total_cost"`
	}
	var agg proxyLogAgg
	err = db.Get(&agg, db.Rebind(`
		SELECT
			COALESCE(SUM(CASE WHEN s.status = 'active' THEN 1 ELSE 0 END), 0) AS count,
			COALESCE(SUM(CASE WHEN s.status = 'active' AND pl.status = 'success' THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN s.status = 'active' AND pl.status IS NOT NULL AND pl.status != 'success' THEN 1 ELSE 0 END), 0) AS failed_count,
			COALESCE(SUM(CASE WHEN s.status = 'active' AND pl.status IS NULL THEN 1 ELSE 0 END), 0) AS unknown_status_count,
			COALESCE(SUM(CASE WHEN a.id IS NULL OR s.id IS NULL THEN 1 ELSE 0 END), 0) AS unattributed_count,
			COALESCE(SUM(CASE
				WHEN s.status = 'active'
					AND pl.estimated_cost IS NULL
					AND (COALESCE(pl.total_tokens, 0) > 0 OR COALESCE(pl.prompt_tokens, 0) > 0 OR COALESCE(pl.completion_tokens, 0) > 0)
				THEN 1 ELSE 0 END), 0) AS missing_cost_count,
			COALESCE(SUM(`+service.EffectiveProxyTokensOnActiveSitesSQL+`), 0) AS total_tokens,
			COALESCE(SUM(CASE WHEN s.status = 'active' THEN COALESCE(pl.estimated_cost, 0) ELSE 0 END), 0.0) AS total_cost
		FROM proxy_logs pl
		LEFT JOIN accounts a ON a.id = pl.account_id
		LEFT JOIN sites s ON s.id = a.site_id
		WHERE pl.created_at >= ? AND pl.created_at < ?
	`), dayRange.StartUTC, dayRange.EndUTC)
	if err != nil {
		return nil, fmt.Errorf("load local-day proxy metrics: %w", err)
	}
	if agg.Count != nil {
		proxyTotal = int(*agg.Count)
		if agg.SuccessCount != nil {
			proxySuccess = int(*agg.SuccessCount)
		}
		if agg.FailedCount != nil {
			proxyFailed = int(*agg.FailedCount)
		}
		if agg.UnknownStatusCount != nil {
			proxyUnknown = int(*agg.UnknownStatusCount)
		}
		if agg.UnattributedCount != nil {
			proxyUnattributed = int(*agg.UnattributedCount)
		}
		if agg.TotalTokens != nil {
			proxyTotalTokens = *agg.TotalTokens
		}
		if agg.TotalCost != nil {
			todaySpend = *agg.TotalCost
		}
	}
	missingCostCount := 0
	if agg.MissingCostCount != nil {
		missingCostCount = int(*agg.MissingCostCount)
	}
	if proxyUnattributed > 0 {
		todaySpendStatus = "partial"
		todaySpendReason = "unattributed"
		proxyMetricStatus = "partial"
		proxyMetricReason = "unattributed"
	} else {
		if missingCostCount > 0 {
			todaySpendStatus = "partial"
			todaySpendReason = "source_partial"
		}
		if proxyUnknown > 0 {
			proxyMetricStatus = "partial"
			proxyMetricReason = "legacy_unknown"
		}
	}

	// Calculate today reward using todayIncome fallback
	var todayReward float64
	todayRewardStatus := "complete"
	todayRewardReason := ""
	for _, a := range accountRows {
		successCount := successCountByAccount[a.ID]
		parsedRewardCount := parsedRewardCountByAccount[a.ID]
		parsedReward := rewardByAccount[a.ID]
		accountReward := service.EstimateRewardWithTodayIncomeFallback(service.EstimateRewardInput{
			Day:               dayRange.LocalDay,
			SuccessCount:      successCount,
			ParsedRewardCount: parsedRewardCount,
			RewardSum:         parsedReward,
			ExtraConfig:       a.ExtraConfig,
		})
		todayReward += accountReward
		if successCount > parsedRewardCount && accountReward <= parsedReward {
			todayRewardStatus = "partial"
			todayRewardReason = "source_partial"
		}
	}

	return &DailySummaryMetrics{
		LocalDay:           dayRange.LocalDay,
		WindowStartUTC:     dayRange.StartUTC,
		WindowEndUTC:       dayRange.EndUTC,
		GeneratedAtLocal:   service.FormatLocalDateTime(now),
		TimeZone:           timeZone,
		TotalAccounts:      len(accountRows),
		ActiveAccounts:     activeAccounts,
		LowBalanceAccounts: lowBalanceAccounts,
		CheckinTotal:       len(checkinRows),
		CheckinSuccess:     checkinSuccess,
		CheckinSkipped:     checkinSkipped,
		CheckinFailed:      checkinFailed,
		ProxyTotal:         proxyTotal,
		ProxySuccess:       proxySuccess,
		ProxyFailed:        proxyFailed,
		ProxyUnknown:       proxyUnknown,
		ProxyUnattributed:  proxyUnattributed,
		ProxyMissingCost:   missingCostCount,
		ProxyMetricStatus:  proxyMetricStatus,
		ProxyMetricReason:  proxyMetricReason,
		ProxyTotalTokens:   proxyTotalTokens,
		TodaySpend:         Round6(todaySpend),
		TodaySpendStatus:   todaySpendStatus,
		TodaySpendReason:   todaySpendReason,
		TodayReward:        Round6(todayReward),
		TodayRewardStatus:  todayRewardStatus,
		TodayRewardReason:  todayRewardReason,
	}, nil
}

// BuildDailySummaryNotification builds the daily summary notification text.
// Mirrors TS buildDailySummaryNotification().
func BuildDailySummaryNotification(metrics *DailySummaryMetrics) (title, message string) {
	net := Round6(metrics.TodayReward - metrics.TodaySpend)
	rewardTruthSuffix := ""
	if metrics.TodayRewardStatus != "complete" {
		rewardTruthSuffix = " (部分可观测)"
	}
	title = fmt.Sprintf("每日总结 %s", metrics.LocalDay)
	message = fmt.Sprintf(
		"日期: %s\n生成时间: %s (%s)\n\n"+
			"账号概览: 总计 %d | 活跃 %d | 低余额(<$1) %d\n"+
			"签到统计: 总计 %d | 成功 %d | 跳过 %d | 失败 %d\n"+
			"代理统计: 总计 %d | 成功 %d | 失败 %d | 未知 %d | 未归属 %d | Tokens %s\n"+
			"费用统计: 支出 $%s | 奖励 $%s%s | 净值 $%s%s",
		metrics.LocalDay, metrics.GeneratedAtLocal, metrics.TimeZone,
		metrics.TotalAccounts, metrics.ActiveAccounts, metrics.LowBalanceAccounts,
		metrics.CheckinTotal, metrics.CheckinSuccess, metrics.CheckinSkipped, metrics.CheckinFailed,
		metrics.ProxyTotal, metrics.ProxySuccess, metrics.ProxyFailed, metrics.ProxyUnknown, metrics.ProxyUnattributed, formatTokens(metrics.ProxyTotalTokens),
		fmt.Sprintf("%.6f", metrics.TodaySpend),
		fmt.Sprintf("%.6f", metrics.TodayReward),
		rewardTruthSuffix,
		fmt.Sprintf("%.6f", net),
		rewardTruthSuffix,
	)
	return
}

// SendDailySummary collects metrics and sends the daily summary notification.
func SendDailySummary(cfg *config.Config, db *sqlx.DB) {
	now := time.Now()
	metrics, err := CollectDailySummaryMetrics(db, now)
	if err != nil {
		slog.Error("daily-summary: failed to collect metrics", "error", err)
		return
	}
	title, message := BuildDailySummaryNotification(metrics)
	notifypkg.SendNotification(cfg, title, message, "info", nil)
}

// Round6 rounds a value to 6 decimal places.
func Round6(value float64) float64 {
	return math.Round(value*1_000_000) / 1_000_000
}

func formatTokens(n int64) string {
	if n == 0 {
		return "0"
	}
	result := ""
	s := fmt.Sprintf("%d", n)
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(c)
	}
	return result
}
