package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/jmoiron/sqlx"
)

// TokenExpiredParams holds parameters for reportTokenExpired.
type TokenExpiredParams struct {
	AccountID  int64
	Username   *string
	SiteName   *string
	Detail     string
	HTTPStatus int // optional; 0 means classify from Detail alone
}

// ReportTokenExpired reports a confirmed token expiration event and marks the account expired.
// Defense-in-depth : even if a caller forgets ShouldMarkAccountExpired,
// this function no-ops unless classification confirms ClassExpired. Generic 401,
// network, 429, and 5xx must never force-mark accounts.status='expired'.
// Mirrors TS reportTokenExpired() with a hard mark guard.
func ReportTokenExpired(cfg *config.RuntimeSettings, db *sqlx.DB, params TokenExpiredParams) {
	if !ShouldMarkAccountExpired(params.HTTPStatus, params.Detail) {
		slog.Info("ReportTokenExpired skipped: detail is not confirmed credential expiry",
			"accountID", params.AccountID,
			"httpStatus", params.HTTPStatus,
			"detail", params.Detail,
		)
		return
	}

	accountLabel := orID(params.Username, params.AccountID)
	siteLabel := "unknown-site"
	if params.SiteName != nil {
		siteLabel = *params.SiteName
	}

	detailText := ""
	if params.Detail != "" {
		detailText = AppendSessionTokenRebindHint(params.Detail)
	}
	detail := ""
	if detailText != "" {
		detail = fmt.Sprintf(" (%s)", detailText)
	}

	createdAt := service.FormatUtcSqlDateTime(time.Now())

	// Write events
	_ = createdAt
	message := enrichAlertMessage(db, fmt.Sprintf("%s @ %s token is invalid or expired%s", accountLabel, siteLabel, detail),
		alertEnrichmentScope{accountID: &params.AccountID})
	service.CreateEvent(db, "token", "Token expired",
		message, "error", params.AccountID, "account")

	// Update account status
	db.Exec(db.Rebind("UPDATE accounts SET status = 'expired', updated_at = ? WHERE id = ?"),
		time.Now().UTC().Format(time.RFC3339), params.AccountID)

	// Set runtime health
	healthReason := "access token expired"
	if detailText != "" {
		healthReason = "access token expired: " + detailText
	}
	service.SetAccountRuntimeHealth(db, params.AccountID, service.RuntimeHealthEntry{
		State: service.HealthUnhealthy, Reason: healthReason, Source: service.HealthSourceAuth,
	})

	// Send notification
	notifypkg.SendNotification(cfg, "Token expired", message,
		string(notifypkg.LevelError), &notifypkg.SendNotificationOptions{TaskTag: "token_expired"})
}

// LowBalanceParams holds parameters for reportLowBalance.
type LowBalanceParams struct {
	AccountID int64
	Username  *string
	SiteName  *string
	Balance   float64 // current balance after refresh
	Threshold float64 // alert when Balance < Threshold
}

// lowBalanceEventWindow is the dedup window: at most one low-balance alert
// per account per window to avoid spamming on every refresh while the balance
// stays low. Matches the "real-time on first crossing" intent (TS only counted
// lowBalanceAccounts in a daily summary; this fires immediately on refresh).
const lowBalanceEventWindow = 24 * time.Hour

// ReportLowBalance fires a real-time low-balance alert when an account's
// balance drops below a threshold right after a refresh. Deduped per account
// per lowBalanceEventWindow via the events table (no spam while low).
//
// G1: the TS original only counted lowBalanceAccounts in a daily summary
// (balance < 1) — it never fired a real-time trigger. metapi-go does better:
// the alert lands as soon as the refresh observes the low balance.
func ReportLowBalance(cfg *config.RuntimeSettings, db *sqlx.DB, params LowBalanceParams) {
	if db == nil || params.Balance >= params.Threshold {
		return
	}

	// Dedup: skip if a low-balance event for this account was logged recently.
	since := time.Now().UTC().Add(-lowBalanceEventWindow).Format(time.RFC3339)
	var recent int
	if err := db.Get(&recent,
		db.Rebind(`SELECT COUNT(*) FROM events
		 WHERE type = 'balance' AND related_id = ? AND related_type = 'account'
		   AND created_at >= ?`),
		params.AccountID, since); err == nil && recent > 0 {
		return
	}

	accountLabel := orID(params.Username, params.AccountID)
	siteLabel := "unknown-site"
	if params.SiteName != nil {
		siteLabel = *params.SiteName
	}

	msg := enrichAlertMessage(db,
		fmt.Sprintf("%s @ %s low balance: current $%.2f (threshold $%.2f)",
			accountLabel, siteLabel, params.Balance, params.Threshold),
		alertEnrichmentScope{accountID: &params.AccountID})

	service.CreateEvent(db, "balance", "Low balance", msg, "warning",
		params.AccountID, "account")

	notifypkg.SendNotification(cfg, "Low balance", msg, string(notifypkg.LevelWarning),
		&notifypkg.SendNotificationOptions{TaskTag: "low_balance"})
}

type ProxyAllFailedParams struct {
	Model  string
	Reason string
}

// proxyAllFailedThrottle dedupes proxy all-failed alerts per model using the
// same signature-level cooldown mechanism as notify.GlobalThrottle. Without
// it, a permanently dead upstream would fire an event + notification on
// every single request (alert storm).
var proxyAllFailedThrottle = notifypkg.NewNotificationThrottle()

// proxyAllFailedCooldownFloorMs floors the dedup window even when
// NOTIFY_COOLDOWN_SEC=0: one event per request on a dead upstream is never
// acceptable, so the throttle always applies.
const proxyAllFailedCooldownFloorMs = int64(config.DefaultNotifyCooldownSec) * 1000

// proxyAllFailedModelDisplayLimit bounds how much of the client-supplied
// model name is rendered into events, notifications and logs. The throttle
// signature is a hash, so an attacker cannot grow the throttle map with
// long keys or read them back out of it.
const proxyAllFailedModelDisplayLimit = 128

var lastProxyAllFailedPruneMs atomic.Int64

func proxyAllFailedSignature(model string) string {
	sum := sha256.Sum256([]byte(model))
	return "proxy_all_failed:" + hex.EncodeToString(sum[:])
}

func truncateModelForDisplay(model string) string {
	if len(model) <= proxyAllFailedModelDisplayLimit {
		return model
	}
	keep := proxyAllFailedModelDisplayLimit
	for keep > 0 && !utf8.RuneStart(model[keep]) {
		keep--
	}
	return model[:keep] + "…[truncated]"
}

// maybePruneProxyAllFailedThrottle drops entries that are far past their
// cooldown. Production never used to prune this map, so every distinct model
// name a client sent stayed resident forever.
func maybePruneProxyAllFailedThrottle(nowMs int64, cooldownMs int64) {
	stale := cooldownMs * 2
	if stale < 10*60*1000 {
		stale = 10 * 60 * 1000
	}
	last := lastProxyAllFailedPruneMs.Load()
	if nowMs-last < 10*60*1000 {
		return
	}
	if lastProxyAllFailedPruneMs.CompareAndSwap(last, nowMs) {
		proxyAllFailedThrottle.PruneNotificationThrottleState(nowMs, stale)
	}
}

// ReportProxyAllFailed reports a proxy all-failed event.
// Mirrors TS reportProxyAllFailed(). Deduped per model: at most one event +
// notification per cooldown window (cfg.NotifyCooldownSec, floored at
// proxyAllFailedCooldownFloorMs) so a dead upstream cannot spam operators on
// every request.
func ReportProxyAllFailed(cfg *config.RuntimeSettings, db *sqlx.DB, params ProxyAllFailedParams) {
	if db == nil || params.Model == "" {
		return
	}

	cooldownMs := int64(proxyAllFailedCooldownFloorMs)
	if cfg != nil && cfg.NotifyCooldownSec > 0 {
		cooldownMs = int64(cfg.NotifyCooldownSec) * 1000
	}
	nowMs := time.Now().UnixMilli()
	maybePruneProxyAllFailedThrottle(nowMs, cooldownMs)

	displayModel := truncateModelForDisplay(params.Model)
	signature := proxyAllFailedSignature(params.Model)
	if !proxyAllFailedThrottle.EvaluateNotificationThrottle(signature, nowMs, cooldownMs).ShouldSend {
		slog.Debug("proxy all-failed alert throttled", "model", displayModel)
		return
	}

	createdAt := service.FormatUtcSqlDateTime(time.Now())

	message := enrichAlertMessage(db,
		fmt.Sprintf("model=%s, reason=%s", displayModel, params.Reason),
		alertEnrichmentScope{model: params.Model})

	service.CreateEvent(db, "proxy", "All proxies failed",
		message, "error", 0, "route")

	notifypkg.SendNotification(cfg, "All proxies failed", message,
		string(notifypkg.LevelError), &notifypkg.SendNotificationOptions{TaskTag: "proxy_all_failed"})

	_ = createdAt // already used above
}

func orID(username *string, id int64) string {
	if username != nil && *username != "" {
		return *username
	}
	return fmt.Sprintf("ID:%d", id)
}
