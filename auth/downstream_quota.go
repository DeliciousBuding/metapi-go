package auth

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func checkManagedKeyQuota(id int64, quota *store.DownstreamQuota) DownstreamTokenAuthResult {
	deny := func(code int, reason, message string) DownstreamTokenAuthResult {
		return DownstreamTokenAuthResult{StatusCode: code, Reason: reason, Error: message}
	}
	start, end, err := quota.Window(time.Now())
	if err != nil {
		return deny(403, "access_policy", "API key quota period is invalid")
	}
	if quota.HistoryMissingBefore != nil && start < *quota.HistoryMissingBefore {
		return deny(403, "quota_history_missing", "API key quota history must be reconciled before this period can be used")
	}
	db := store.GetDB()
	if db == nil {
		return deny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	var requests, tokens, unknownTokens, unknownCost int64
	var cost float64
	err = db.QueryRowx(`SELECT COALESCE(SUM(requests),0), COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost),0),
		COALESCE(SUM(CASE WHEN total_tokens IS NULL THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN cost IS NULL THEN 1 ELSE 0 END),0)
		FROM downstream_quota_usage WHERE key_id=? AND occurred_at>=? AND occurred_at<?`, id, start, end).Scan(&requests, &tokens, &cost, &unknownTokens, &unknownCost)
	if err != nil {
		return deny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	if (quota.TotalTokens != nil && unknownTokens > 0) || (quota.Cost != nil && unknownCost > 0) {
		return deny(403, "quota_usage_unknown", "API key has usage without complete quota accounting")
	}
	if (quota.Requests != nil && requests >= *quota.Requests) || (quota.TotalTokens != nil && tokens >= *quota.TotalTokens) || (quota.Cost != nil && cost >= *quota.Cost) {
		return deny(429, "over_quota", "API key quota exceeded")
	}
	return DownstreamTokenAuthResult{OK: true}
}

// RecordManagedKeyQuotaUsage records a completed billable attempt independently
// of proxy-log retention. Missing usage stays NULL so token quotas fail closed.
// The event ID is generated per completion: caller request IDs are untrusted and
// cannot be used to deduplicate billing.
func RecordManagedKeyQuotaUsage(keyID int64, tokens *int64, cost *float64) {
	db := store.GetDB()
	if db == nil || keyID <= 0 {
		return
	}
	if tokens != nil && *tokens < 0 {
		tokens = nil
	}
	if cost != nil && (*cost < 0 || math.IsNaN(*cost) || math.IsInf(*cost, 0)) {
		cost = nil
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return
	}
	_, err := db.Exec(`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,?,?,1,?,?)`, keyID, "local:"+hex.EncodeToString(id[:]), time.Now().UnixMilli(), tokens, cost)
	if err != nil {
		slog.Error("failed to persist downstream quota usage", "keyId", keyID)
		// Losing a quota event must not increase the available budget. This is
		// deliberately a key-level failure and requires operator reconciliation.
		_, _ = db.Exec(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`, false, keyID)
	}
}
