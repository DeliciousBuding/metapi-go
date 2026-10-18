package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/deliciousbuding/metapi-go/store"
)

var errQuotaDBUnavailable = errors.New("database not initialized")

// quotaReservationTTL bounds how long an unsettled admission hold can occupy a
// key slot. A process that crashes between admission and settlement leaves its
// row behind; the next admission for that key deletes it once it is this old.
// Requests that legitimately run longer than the TTL (very long streams) lose
// their hold and may be counted twice until they settle; the settlement itself
// is idempotent and never removes another reservation.
const quotaReservationTTL = 30 * time.Minute

// quotaQueryer is the read surface shared by *sqlx.DB (read-only explainer)
// and *sqlx.Tx (the atomic check-and-reserve transaction).
type quotaQueryer interface {
	Rebind(query string) string
	QueryRowx(query string, args ...any) *sqlx.Row
}

func quotaDeny(code int, reason, message string) DownstreamTokenAuthResult {
	return DownstreamTokenAuthResult{StatusCode: code, Reason: reason, Error: message}
}

// checkManagedKeyQuota is the read-only admission check used when a request
// must not hold a slot (Responses WebSocket handshake) or by callers that only
// explain state. Live HTTP admission uses reserveManagedKeyQuota instead.
func checkManagedKeyQuota(id int64, quota *store.DownstreamQuota) DownstreamTokenAuthResult {
	db := store.GetDB()
	if db == nil {
		return quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	return quotaResultFromExplanation(ExplainManagedKeyQuota(db.DB, id, quota))
}

func quotaResultFromExplanation(explained ManagedKeyQuotaExplanation) DownstreamTokenAuthResult {
	switch explained.Blocker {
	case "":
		return DownstreamTokenAuthResult{OK: true}
	case "over_quota":
		return quotaDeny(429, "over_quota", explained.Message)
	case "quota_unavailable":
		return quotaDeny(503, "quota_unavailable", explained.Message)
	default:
		return quotaDeny(403, explained.Blocker, explained.Message)
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
// In-flight reservations count toward the used totals exactly as they do in the
// live gate, so the numbers describe the budget the next admission sees.
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
	reservedRequests, reservedTokens, err := sumManagedKeyQuotaReservations(db, id, start, end)
	if err != nil {
		explanation.Blocker = "quota_unavailable"
		explanation.Message = "API key quota usage is unavailable"
		return explanation
	}
	explanation.RequestsUsed = requests + reservedRequests
	explanation.TokensUsed = tokens + reservedTokens
	explanation.CostUsed = cost
	explanation.UnknownTokens = unknownTokens
	explanation.UnknownCost = unknownCost
	if (quota.TotalTokens != nil && unknownTokens > 0) || (quota.Cost != nil && unknownCost > 0) {
		explanation.Blocker = "quota_usage_unknown"
		explanation.Message = "API key has usage without complete quota accounting"
		return explanation
	}
	if (quota.Requests != nil && explanation.RequestsUsed >= *quota.Requests) || (quota.TotalTokens != nil && explanation.TokensUsed >= *quota.TotalTokens) || (quota.Cost != nil && cost >= *quota.Cost) {
		explanation.Blocker = "over_quota"
		explanation.Message = "API key quota exceeded"
	}
	return explanation
}

// sumManagedKeyQuotaUsage is the shared read-only SUM over the current quota
// window, used by the live gate, the reservation transaction and the explainer.
func sumManagedKeyQuotaUsage(db quotaQueryer, id int64, start, end int64) (requests, tokens int64, cost float64, unknownTokens, unknownCost int64, err error) {
	if db == nil {
		return 0, 0, 0, 0, 0, errQuotaDBUnavailable
	}
	err = db.QueryRowx(db.Rebind(`SELECT COALESCE(SUM(requests),0), COALESCE(SUM(total_tokens),0), COALESCE(SUM(cost),0),
		COALESCE(SUM(CASE WHEN total_tokens IS NULL THEN 1 ELSE 0 END),0), COALESCE(SUM(CASE WHEN cost IS NULL THEN 1 ELSE 0 END),0)
		FROM downstream_quota_usage WHERE key_id=? AND occurred_at>=? AND occurred_at<?`), id, start, end).
		Scan(&requests, &tokens, &cost, &unknownTokens, &unknownCost)
	return requests, tokens, cost, unknownTokens, unknownCost, err
}

// sumManagedKeyQuotaReservations adds the holds that overlap the query window.
// Overlap (not created_at containment) is the boundary rule: a hold admitted
// just before a window edge still occupied the budget inside that window, and
// a calendar hold from the previous period ends exactly at the new start and is
// excluded. Overlap also keeps the sum correct when two admissions evaluate
// their window after waiting for the per-key lock. Non-positive values are
// ignored: this table is written only by admission (requests=1, positive
// tokens), so a negative row can only come from tampered input and must never
// subtract from the used budget.
func sumManagedKeyQuotaReservations(db quotaQueryer, id int64, start, end int64) (requests, tokens int64, err error) {
	if db == nil {
		return 0, 0, errQuotaDBUnavailable
	}
	err = db.QueryRowx(db.Rebind(`SELECT
		COALESCE(SUM(CASE WHEN requests>0 THEN requests ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN tokens>0 THEN tokens ELSE 0 END),0)
		FROM downstream_quota_reservations WHERE key_id=? AND window_end>? AND window_start<?`), id, start, end).
		Scan(&requests, &tokens)
	return requests, tokens, err
}

// reserveManagedKeyQuota performs the quota admission and inserts a
// reservation in one transaction, returning the reservation ID that the
// completion path settles. The check is the same verdict chain as
// ExplainManagedKeyQuota; only the used totals now include in-flight holds.
//
// Serialization: PostgreSQL takes a per-key row lock (SELECT ... FOR UPDATE)
// before summing, so concurrent admissions for one key run one at a time.
// SQLite is opened with a single connection (store.Open), so its transaction
// already excludes every other writer in the process.
//
// estimatedTokens is the best-effort input estimate. It is reserved only when
// the policy has a token limit; output tokens are unknown before dispatch, so
// concurrent overshoot of a token limit is bounded by in-flight output, not by
// the request count. A cost limit gets no reservation because the selected
// channel's price is unknown before routing: cost admission stays
// completion-based and concurrent in-flight requests can exceed it by roughly
// (concurrency x per-request cost).
func reserveManagedKeyQuota(id int64, quota *store.DownstreamQuota, estimatedTokens int64) (string, DownstreamTokenAuthResult) {
	db := store.GetDB()
	if db == nil {
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}

	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		slog.Warn("quota reservation: failed to begin transaction", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	defer tx.Rollback()

	if db.Dialect == store.DialectPostgres {
		var locked int64
		if err := tx.QueryRowx(tx.Rebind(`SELECT id FROM downstream_api_keys WHERE id=? FOR UPDATE`), id).Scan(&locked); err != nil {
			slog.Warn("quota reservation: failed to lock API key", "keyId", id, "error", err)
			return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
		}
	}

	// Evaluate the window only after this admission owns the per-key lock:
	// a timestamp captured before waiting could place this request's hold
	// outside a later admission's window.
	now := time.Now()
	start, end, err := quota.Window(now)
	if err != nil {
		return "", quotaDeny(403, "access_policy", "API key quota period is invalid")
	}
	if quota.HistoryMissingBefore != nil && start < *quota.HistoryMissingBefore {
		return "", quotaDeny(403, "quota_history_missing", "API key quota history must be reconciled before this period can be used")
	}

	nowMs := now.UnixMilli()
	// Crash recovery: holds older than the TTL never settle. Removing them in
	// the same transaction keeps the sum below authoritative.
	if _, err := tx.Exec(tx.Rebind(`DELETE FROM downstream_quota_reservations WHERE key_id=? AND created_at<?`), id, nowMs-quotaReservationTTL.Milliseconds()); err != nil {
		slog.Warn("quota reservation: failed to expire stale reservations", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}

	requests, tokens, cost, unknownTokens, unknownCost, err := sumManagedKeyQuotaUsage(tx, id, start, end)
	if err != nil {
		slog.Warn("quota reservation: failed to sum usage", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	reservedRequests, reservedTokens, err := sumManagedKeyQuotaReservations(tx, id, start, end)
	if err != nil {
		slog.Warn("quota reservation: failed to sum reservations", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	requests += reservedRequests
	tokens += reservedTokens

	if (quota.TotalTokens != nil && unknownTokens > 0) || (quota.Cost != nil && unknownCost > 0) {
		return "", quotaDeny(403, "quota_usage_unknown", "API key has usage without complete quota accounting")
	}
	if (quota.Requests != nil && requests >= *quota.Requests) || (quota.TotalTokens != nil && tokens >= *quota.TotalTokens) || (quota.Cost != nil && cost >= *quota.Cost) {
		return "", quotaDeny(429, "over_quota", "API key quota exceeded")
	}

	reservationID, err := newQuotaReservationID()
	if err != nil {
		slog.Warn("quota reservation: failed to generate reservation ID", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	var reservedTokenValue any
	if quota.TotalTokens != nil {
		if estimatedTokens < 1 {
			estimatedTokens = 1
		}
		reservedTokenValue = estimatedTokens
	}
	if _, err := tx.Exec(tx.Rebind(`INSERT INTO downstream_quota_reservations
		(reservation_id,key_id,window_start,window_end,requests,tokens,cost,created_at)
		VALUES (?,?,?,?,?,?,?,?)`), reservationID, id, start, end, 1, reservedTokenValue, nil, nowMs); err != nil {
		slog.Warn("quota reservation: failed to insert reservation", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	if err := tx.Commit(); err != nil {
		slog.Warn("quota reservation: failed to commit reservation", "keyId", id, "error", err)
		return "", quotaDeny(503, "quota_unavailable", "API key quota usage is unavailable")
	}
	return reservationID, DownstreamTokenAuthResult{OK: true}
}

func newQuotaReservationID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

// ReleaseManagedKeyQuotaReservation drops an admission hold that never reached
// a billable completion (early handler exit, rejected retry, client
// cancellation before any usage). It is idempotent and safe after settlement.
// A failed delete only leaves the hold for the TTL sweep, which over-counts
// the budget rather than granting it.
func ReleaseManagedKeyQuotaReservation(reservationID string) {
	if reservationID == "" {
		return
	}
	db := store.GetDB()
	if db == nil {
		return
	}
	if _, err := db.Exec(db.Rebind(`DELETE FROM downstream_quota_reservations WHERE reservation_id=?`), reservationID); err != nil {
		slog.Warn("failed to release downstream quota reservation", "reservationId", reservationID, "error", err)
	}
}

// RecordManagedKeyQuotaUsage records a completed billable attempt independently
// of proxy-log retention. Missing usage stays NULL so token quotas fail closed.
// The event ID is generated per completion: caller request IDs are untrusted and
// cannot be used to deduplicate billing.
//
// When reservationID is non-empty the reservation is removed in the same
// transaction as the usage insert, so the admission sum never observes a gap
// between them. A missing reservation (crash cleanup, retry after a released
// hold) does not suppress the usage row: billing must not depend on admission
// bookkeeping.
func RecordManagedKeyQuotaUsage(keyID int64, reservationID string, tokens *int64, cost *float64) {
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
	eventKey := "local:" + hex.EncodeToString(id[:])
	occurredAt := time.Now().UnixMilli()

	var err error
	if reservationID != "" {
		err = settleManagedKeyQuotaUsage(db, keyID, reservationID, eventKey, occurredAt, tokens, cost)
	} else {
		_, err = db.Exec(db.Rebind(`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,?,?,1,?,?)`), keyID, eventKey, occurredAt, tokens, cost)
	}
	if err != nil {
		slog.Error("failed to persist downstream quota usage", "keyId", keyID)
		// Losing a quota event must not increase the available budget. This is
		// deliberately a key-level failure and requires operator reconciliation.
		_, _ = db.Exec(db.Rebind(`UPDATE downstream_api_keys SET enabled=? WHERE id=?`), false, keyID)
	}
}

func settleManagedKeyQuotaUsage(db *store.DB, keyID int64, reservationID, eventKey string, occurredAt int64, tokens *int64, cost *float64) error {
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(tx.Rebind(`DELETE FROM downstream_quota_reservations WHERE reservation_id=? AND key_id=?`), reservationID, keyID); err != nil {
		return err
	}
	if _, err := tx.Exec(tx.Rebind(`INSERT INTO downstream_quota_usage (key_id,event_key,occurred_at,requests,total_tokens,cost) VALUES (?,?,?,1,?,?)`), keyID, eventKey, occurredAt, tokens, cost); err != nil {
		return err
	}
	return tx.Commit()
}
