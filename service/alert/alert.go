package alert

import (
	"fmt"
	"log/slog"
	"time"

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
func ReportTokenExpired(cfg *config.Config, db *sqlx.DB, params TokenExpiredParams) {
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
		"error", &notifypkg.SendNotificationOptions{TaskTag: "token_expired"})
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
func ReportLowBalance(cfg *config.Config, db *sqlx.DB, params LowBalanceParams) {
	if db == nil || params.Balance >= params.Threshold {
		return
	}

	// Dedup: skip if a low-balance event for this account was logged recently.
	since := time.Now().UTC().Add(-lowBalanceEventWindow).Format(time.RFC3339)
	var recent int
	if err := db.Get(&recent,
		`SELECT COUNT(*) FROM events
		 WHERE type = 'balance' AND related_id = ? AND related_type = 'account'
		   AND created_at >= ?`,
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

	notifypkg.SendNotification(cfg, "Low balance", msg, "warning",
		&notifypkg.SendNotificationOptions{TaskTag: "low_balance"})
}

type ProxyAllFailedParams struct {
	Model  string
	Reason string
}

// ReportProxyAllFailed reports a proxy all-failed event.
// Mirrors TS reportProxyAllFailed().
func ReportProxyAllFailed(cfg *config.Config, db *sqlx.DB, params ProxyAllFailedParams) {
	createdAt := service.FormatUtcSqlDateTime(time.Now())

	message := enrichAlertMessage(db,
		fmt.Sprintf("model=%s, reason=%s", params.Model, params.Reason),
		alertEnrichmentScope{model: params.Model})

	service.CreateEvent(db, "proxy", "All proxies failed",
		message, "error", 0, "route")

	notifypkg.SendNotification(cfg, "All proxies failed", message,
		"error", &notifypkg.SendNotificationOptions{TaskTag: "proxy_all_failed"})

	_ = createdAt // already used above
}

func orID(username *string, id int64) string {
	if username != nil && *username != "" {
		return *username
	}
	return fmt.Sprintf("ID:%d", id)
}
