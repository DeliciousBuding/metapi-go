package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jmoiron/sqlx"
	"log/slog"
	"math"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

var errQuotaDBUnavailable = errors.New("database not initialized")

func checkManagedKeyQuota(id int64, quota *store.DownstreamQuota) DownstreamTokenAuthResult {
	deny := func(code int, reason, message string) DownstreamTokenAuthResult {
		return DownstreamTokenAuthResult{StatusCode: code, Reason: reason, Error: message}
	}
	db := store.GetDB()
	if db == nil {
		return deny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	explained := ExplainManagedKeyQuota(db.DB, id, quota)
	switch explained.Blocker {
	case "":
		return DownstreamTokenAuthResult{OK: true}
	case "over_quota":
		return deny(429, "over_quota", explained.Message)
	case "quota_unavailable":
		return deny(503, "quota_unavailable", explained.Message)
	default:
		return deny(403, explained.Blocker, explained.Message)
	}
}

// ManagedKeyQuotaExplanation is the read-only quota-window answer for the
// key-context admission explainer: the same window math as the live gate, but
// it returns the numbers behind the verdict and records nothing.
type ManagedKeyQuotaExplanation struct {
	WindowStart    string   `json:"windowStart"`
	WindowEnd      string   `json:"windowEnd"`
	RequestsUsed   int64    `json:"requestsUsed"`
	RequestsMax    *int64   `json:"requestsMax,omitempty"`
	TokensUsed     int64    `json:"tokensUsed"`
	TokensMax      *int64   `json:"tokensMax,omitempty"`
	CostUsed       float64  `json:"costUsed"`
	CostMax        *float64 `json:"costMax,omitempty"`
	UnknownTokens  int64    `json:"unknownTokens"`
	UnknownCost    int64    `json:"unknownCost"`
	HistoryMissing bool     `json:"historyMissing"`
	// Blocker is "" when the window currently permits the request; otherwise
	// the live chain's reason: over_quota | quota_usage_unknown |
	// quota_history_missing | quota_unavailable | access_policy.
	Blocker string `json:"blocker"`
	Message string `json:"message"`
}

// ExplainManagedKeyQuota evaluates the quota window without recording usage.
func ExplainManagedKeyQuota(db *sqlx.DB, id int64, quota *store.DownstreamQuota) ManagedKeyQuotaExplanation {
	explanation := ManagedKeyQuotaExplanation{
		RequestsMax: quota.Requests,
		TokensMax:   quota.TotalTokens,
		CostMax:     quota.Cost,
	}
	start, end, err := quota.Window(time.Now())
	if err != nil {
		explanation.Blocker = "access_policy"
		explanation.Message = "API key quota period is invalid"
		return explanation
	}
	explanation.WindowStart = time.UnixMilli(start).UTC().Format(time.RFC3339)
	explanation.WindowEnd = time.UnixMilli(end).UTC().Format(time.RFC3339)
	if quota.HistoryMissingBefore != nil && start < *quota.HistoryMissingBefore {
		explanation.HistoryMissing = true
		explanation.Blocker = "quota_history_missing"
		explanation.Message = "API key quota history must be reconciled before this period can be used"
		return explanation
	}
	requests, tokens, cost, unknownTokens, unknownCost, err := sumManagedKeyQuotaUsage(db, id, start, end)
	if err != nil {
		explanation.Blocker = "quota_unavailable"
		explanation.Message = "API key quota usage is unavailable"
		return explanation
	}
	explanation.RequestsUsed = requests
	explanation.TokensUsed = tokens
	explanation.CostUsed = cost
	explanation.UnknownTokens = unknownTokens
	explanation.UnknownCost = unknownCost
	if (quota.TotalTokens != nil && unknownTokens > 0) || (quota.Cost != nil && unknownCost > 0) {
		explanation.Blocker = "quota_usage_unknown"
		explanation.Message = "API key has usage without complete quota accounting"
		return explanation
	}
	if (quota.Requests != nil && requests >= *quota.Requests) || (quota.TotalTokens != nil && tokens >= *quota.TotalTokens) || (quota.Cost != nil && cost >= *quota.Cost) {
		explanation.Blocker = "over_quota"
		explanation.Message = "API key quota exceeded"
	}
	return explanation
}

// sumManagedKeyQuotaUsage is the shared read-only SUM over the current quota
// window, used by both the live gate and the explainer.
func sumManagedKeyQuotaUsage(db *sqlx.DB, id int64, start, end int64) (requests, tokens int64, cost float64, unknownTokens, unknownCost int64, err error) {
	if db == nil {
		return 0, 0, 0, 0, 0, errQuotaDBUnavailable
	}
	err = db.QueryRowx(db.Rebind(`SELECT COALESCE(SUM(requests),0), COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost),0),
		COALESCE(SUM(CASE WHEN total_tokens IS NULL THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN cost IS NULL THEN 1 ELSE 0 END),0)
		FROM downstream_quota_usage WHERE key_id=? AND occurred_at>=? AND occurred_at<?`), id, start, end).
		Scan(&requests, &tokens, &cost, &unknownTokens, &unknownCost)
	return requests, tokens, cost, unknownTokens, unknownCost, err
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
	_, err := db.Exec(db.Rebind(`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,?,?,1,?,?)`), keyID, "local:"+hex.EncodeToString(id[:]), time.Now().UnixMilli(), tokens, cost)
	if err != nil {
		slog.Error("failed to persist downstream quota usage", "keyId", keyID)
		// Losing a quota event must not increase the available budget. This is
		// deliberately a key-level failure and requires operator reconciliation.
		_, _ = db.Exec(db.Rebind(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`), false, keyID)
	}
}
