package checkin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/deliciousbuding/metapi-go/service/events"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/jmoiron/sqlx"
)

func skipIneligibleAccount(db *sqlx.DB, aws *service.AccountWithSite, options *CheckinOptions) *CheckinResult {
	var reason, message, eventWarning string
	disabled := false
	switch {
	case isAccountDisabled(aws.Account.Status):
		reason, message = "account_disabled", "account disabled"
		eventWarning = "CheckinAccount: failed to persist disabled-account skipped event"
		disabled = true
	case IsSiteDisabled(aws.Site.Status):
		reason, message = "site_disabled", "site disabled"
		eventWarning = "CheckinAccount: failed to persist skipped event"
		disabled = true
	case !service.BuildCapabilitiesForAccount(&aws.Account).CanCheckin:
		reason, message = "checkin_not_supported", "account credential mode does not support checkin"
		eventWarning = "CheckinAccount: failed to persist proxy-only skipped event"
	default:
		return nil
	}

	accountID := aws.Account.ID
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if disabled {
		if err := service.SetAccountRuntimeHealth(db, accountID, service.RuntimeHealthEntry{
			State: service.HealthDisabled, Reason: message, Source: service.HealthSourceCheckin,
		}); err != nil {
			return &CheckinResult{Status: CheckinFailed, Message: "failed to persist runtime health: " + err.Error()}
		}
	}
	if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, failure_reason, created_at) VALUES (?, ?, ?, ?, ?)"),
		accountID, "skipped", message, classifyAndMarshalFailureReason(message, CheckinSkipped), createdAt); err != nil {
		return &CheckinResult{Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
	}
	if !options.SkipEvent {
		if err := events.WriteEvent(db, events.Ref{Key: "checkinSkipped", Params: map[string]any{
			"account": orUsername(aws.Account.Username, accountID), "site": aws.Site.Name, "reason": message,
		}}, events.Options{Level: "info", RelatedID: accountID, RelatedType: "account"}); err != nil {
			slog.Warn(eventWarning, "accountID", accountID, "error", err)
		}
	}
	return &CheckinResult{Success: true, Status: CheckinSkipped, Skipped: true, Reason: reason, Message: message}
}

func recordCheckinResult(cfg *config.Config, db *sqlx.DB, aws *service.AccountWithSite, options *CheckinOptions, result *platform.CheckinResult, guessedPlatformUserID int64) CheckinResult {
	account, site := &aws.Account, &aws.Site
	accountID := account.ID
	isCloudflare := alert.IsCloudflareChallenge(result.Message)
	alreadyCheckedIn := platform.IsAlreadyCheckedInMessage(result.Message)
	// One vocabulary, four readers (runner, failure classifier, health reader,
	// New API cookie ladder): platform owns it.
	unsupportedCheckin := platform.IsUnsupportedCheckinMessage(result.Message)
	manualVerificationRequired := platform.IsManualVerificationRequiredMessage(result.Message)
	manualVerificationMessage := "the site requires Turnstile verification; manual check-in is needed"
	logMessage := result.Message
	if manualVerificationRequired {
		logMessage = manualVerificationMessage
	}
	effectiveSuccess := result.Success || alreadyCheckedIn || unsupportedCheckin || manualVerificationRequired
	shouldRefreshBalance := result.Success || alreadyCheckedIn
	directCheckinSuccess := result.Success && !alreadyCheckedIn && !unsupportedCheckin
	shouldAdvanceLastCheckinAt := directCheckinSuccess || (alreadyCheckedIn && options.ScheduleMode != "interval")

	normalizedStatus := CheckinSuccess
	if !effectiveSuccess {
		normalizedStatus = CheckinFailed
	} else if unsupportedCheckin || manualVerificationRequired {
		normalizedStatus = CheckinSkipped
	}

	logReward := result.Reward
	var refreshedBalanceInfo *platform.BalanceInfo

	// Post-success processing
	if effectiveSuccess {
		var healthState service.RuntimeHealthState
		healthReason := ""
		if unsupportedCheckin {
			healthState = service.HealthDegraded
			healthReason = "site does not support check-in endpoint"
		} else if manualVerificationRequired {
			healthState = service.HealthDegraded
			healthReason = manualVerificationMessage
		} else if alreadyCheckedIn {
			healthState = service.HealthHealthy
			healthReason = "already checked in today"
		} else {
			healthState = service.HealthHealthy
			healthReason = "check-in succeeded"
			if result.Message != "" {
				healthReason = result.Message
			}
		}
		if err := service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: healthState, Reason: healthReason, Source: service.HealthSourceCheckin,
		}); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist runtime health: " + err.Error()}
		}

		// Update account fields
		var setClauses []string
		var args []any

		if shouldAdvanceLastCheckinAt {
			now := time.Now().UTC().Format(time.RFC3339)
			setClauses = append(setClauses, "last_checkin_at = ?")
			args = append(args, now)
		}

		if guessedPlatformUserID != 0 {
			newConfig := service.MergeExtraConfig(account.ExtraConfig, map[string]any{
				"platformUserId": float64(guessedPlatformUserID),
			})
			if newConfig != nil {
				setClauses = append(setClauses, "extra_config = ?")
				args = append(args, *newConfig)
			}
		}

		if account.Status == "expired" {
			setClauses = append(setClauses, "status = ?", "updated_at = ?")
			args = append(args, "active", time.Now().UTC().Format(time.RFC3339))
		}

		if len(setClauses) > 0 {
			query := "UPDATE accounts SET " + strings.Join(setClauses, ", ") + " WHERE id = ?"
			args = append(args, accountID)
			if _, err := db.Exec(db.Rebind(query), args...); err != nil {
				return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to update account after checkin: " + err.Error()}
			}
		}

		// Refresh balance if needed
		if shouldRefreshBalance {
			balanceResult, balErr := balance.RefreshBalance(cfg, db, accountID)
			if balErr == nil && balanceResult != nil {
				refreshedBalanceInfo = balanceResult.BalanceInfo
			}
		}

		// The adapter owns native award fields and their units. A non-empty
		// reward (including explicit "0") is authoritative; free-form message
		// numbers may be dates or total balances, not money awarded here.
		// The account snapshot predates the refresh above. DEFAULT 0 is not a
		// known baseline: require evidence of a prior successful balance read.
		if directCheckinSuccess && strings.TrimSpace(logReward) == "" &&
			refreshedBalanceInfo != nil && account.Balance != nil &&
			account.LastBalanceRefresh != nil && strings.TrimSpace(*account.LastBalanceRefresh) != "" {
			inferredReward := InferRewardFromBalanceDelta(*account.Balance, refreshedBalanceInfo.Balance)
			if inferredReward > 0 {
				logReward = strconv.FormatFloat(inferredReward, 'f', -1, 64)
			}
		}
	}

	// Write checkin_logs
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, reward, failure_reason, created_at) VALUES (?, ?, ?, ?, ?, ?)"),
		accountID, string(normalizedStatus), logMessage, logReward, classifyAndMarshalFailureReason(logMessage, normalizedStatus), createdAt); err != nil {
		return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
	}

	outcome := CheckinResult{
		Success: effectiveSuccess,
		Status:  normalizedStatus,
		Skipped: normalizedStatus == CheckinSkipped,
		Message: logMessage,
		Reward:  logReward,
	}
	if !options.SkipEvent {
		writeCheckinEvent(db, aws, outcome, isCloudflare)
	}

	// Post-failure processing
	if !effectiveSuccess {
		if err := service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthUnhealthy, Reason: result.Message,
			Source: service.HealthSourceCheckin,
		}); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist runtime health: " + err.Error()}
		}

		if alert.ShouldMarkAccountExpired(0, result.Message) {
			alert.ReportTokenExpired(config.RuntimeSafe(), db, alert.TokenExpiredParams{
				AccountID: account.ID, Username: account.Username,
				SiteName: &site.Name, Detail: result.Message,
			})
		}

		// Per-account failure notifications are suppressed when the caller
		// aggregates the round itself (CheckinAll → one consolidated alert,
		// issue #667). Single-account manual triggers leave it on.
		if !options.SkipNotification {
			if isCloudflare {
				notifypkg.SendNotification(config.RuntimeSafe(),
					"Cloudflare challenge",
					fmt.Sprintf("%s @ %s: %s", orUsername(account.Username, accountID), site.Name, result.Message),
					string(notifypkg.LevelWarning), nil,
				)
			}

			if !unsupportedCheckin && !manualVerificationRequired {
				notifypkg.SendNotification(config.RuntimeSafe(),
					"checkin failed",
					fmt.Sprintf("%s @ %s: %s", orUsername(account.Username, accountID), site.Name, result.Message),
					string(notifypkg.LevelError), nil,
				)
			}
		}
	}

	return outcome
}

func writeCheckinEvent(db *sqlx.DB, aws *service.AccountWithSite, outcome CheckinResult, isCloudflare bool) {
	accountID := aws.Account.ID
	eventRef := events.Ref{Key: "checkinSuccess"}
	eventLevel := "info"
	if !outcome.Success {
		if isCloudflare {
			eventRef.Key = "checkinFailedCloudflare"
		} else {
			eventRef.Key = "checkinFailed"
		}
		eventLevel = "error"
	} else if outcome.Status == CheckinSkipped {
		eventRef.Key = "checkinSkipped"
	}
	params := map[string]any{
		"account": orUsername(aws.Account.Username, accountID),
		"site":    aws.Site.Name,
	}
	if eventRef.Key == "checkinSuccess" {
		// The registry's success definition carries the reward, not a reason:
		// its MessageEn is the historical "{{account}} @ {{site}}: {{reward}}",
		// and WriteEvent rejects params a definition does not declare. Sending
		// reason for every key made each successful check-in lose its event and
		// leave a WARN behind, so the feed only ever showed failures and skips.
		params["reward"] = outcome.Reward
	} else {
		params["reason"] = outcome.Message
	}
	eventRef.Params = params
	if err := events.WriteEvent(db, eventRef, events.Options{Level: eventLevel, RelatedID: accountID, RelatedType: "account"}); err != nil {
		slog.Warn("CheckinAccount: failed to persist event", "accountID", accountID, "error", err)
	}
}

// classifyAndMarshalFailureReason runs the structured failure classifier
// (ClassifyFailureReason) and serializes the result to JSON for the
// failure_reason column. Returns nil for success or on marshal error so the
// column stays NULL (success / unclassified) — forward-compatible with old
// rows and the API's parsed `failureReason` object shape.
func classifyAndMarshalFailureReason(message string, status CheckinExecutionStatus) *string {
	if status == CheckinSuccess {
		return nil
	}
	reason := ClassifyFailureReason(ClassifyFailureInput{
		Message: message,
		Status:  string(status),
	})
	b, err := json.Marshal(reason)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func orUsername(username *string, id int64) string {
	if username != nil && strings.TrimSpace(*username) != "" {
		return *username
	}
	return fmt.Sprintf("ID:%d", id)
}
