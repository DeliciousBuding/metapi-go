package checkin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/service/balance"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// CheckinExecutionStatus is the status of a checkin execution.
type CheckinExecutionStatus string

const (
	CheckinSuccess CheckinExecutionStatus = "success"
	CheckinFailed  CheckinExecutionStatus = "failed"
	CheckinSkipped CheckinExecutionStatus = "skipped"
)

// CheckinOptions configures checkin behavior.
type CheckinOptions struct {
	SkipEvent bool
	// SkipNotification suppresses the per-account failure notifications
	// ("checkin failed" / "Cloudflare challenge") emitted from CheckinAccount.
	// Callers that aggregate failures themselves (CheckinAll) set this so a
	// round produces ONE consolidated notification instead of N per-account
	// notifications (issue #667). Single-account callers (manual trigger)
	// leave it false to preserve the existing per-account alert.
	SkipNotification bool
	ScheduleMode     string // "cron" or "interval"
}

// CheckinResult is the result of a single account checkin.
type CheckinResult struct {
	Success bool
	Status  CheckinExecutionStatus
	Skipped bool
	Reason  string
	Message string
	Reward  string
}

// CheckinAllResult is a result entry for CheckinAll.
type CheckinAllResult struct {
	AccountID int64
	Username  string
	Site      string
	Result    CheckinResult
}

// checkinTimeout bounds a single adapter checkin call so a hung upstream
// cannot block the worker indefinitely (issue #669). The existing
// withProbeTimeout (5s) is for Detect; checkin needs a longer budget because
// NewApi-family adapters fall back through Bearer → cookie → sign_in paths.
// Declared as a var (not const) so tests can override it to assert enforcement
// without sleeping for 30s.
var checkinTimeout = 30 * time.Second

// checkinInProgress is a process-local set of account IDs currently being
// checked in. It prevents the scheduler and a concurrent admin manual trigger
// from double-running the same account simultaneously (issue #669). The
// scheduler-level runWithSchedulerLease guards against multi-instance
// double-runs; this map guards the in-process per-account case.
var checkinInProgress sync.Map // accountID int64 -> struct{}

// tryAcquireCheckinLease attempts to mark accountID as in-progress. It returns
// true when this caller now owns the lease (and must call releaseCheckinLease),
// false when another checkin for the same account is already running.
func tryAcquireCheckinLease(accountID int64) bool {
	_, loaded := checkinInProgress.LoadOrStore(accountID, struct{}{})
	return !loaded
}

// releaseCheckinLease clears the in-progress marker for accountID. Safe to
// call unconditionally from a defer; a second release is a no-op.
func releaseCheckinLease(accountID int64) {
	checkinInProgress.Delete(accountID)
}

// checkinContext derives a per-account checkin context bounded by
// checkinTimeout so a hung upstream cannot block the worker indefinitely
// (issue #669). Mirrors the existing withProbeTimeout (5s) used for Detect.
func checkinContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, checkinTimeout)
}

// IsSiteDisabled checks if a site status represents "disabled".
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

// isAlreadyCheckedInMessage detects "already checked in" patterns in 12 languages/forms.
func isAlreadyCheckedInMessage(message string) bool {
	text := strings.TrimSpace(message)
	if text == "" {
		return false
	}
	normalized := strings.ToLower(text)
	return strings.Contains(normalized, "already checked in") ||
		strings.Contains(normalized, "already signed") ||
		strings.Contains(normalized, "already sign in") ||
		strings.Contains(normalized, "already claim") ||
		strings.Contains(normalized, "claimed today") ||
		strings.Contains(text, "今日已签到") ||
		strings.Contains(text, "今天已签到") ||
		strings.Contains(text, "今天已经签到") ||
		strings.Contains(text, "今日已经签到") ||
		strings.Contains(text, "已经签到") ||
		strings.Contains(text, "已签到") ||
		strings.Contains(text, "重复签到") ||
		strings.Contains(text, "已经领取") ||
		strings.Contains(text, "已领取") ||
		strings.Contains(text, "领取过") ||
		strings.Contains(text, "签到达")
}

// isUnsupportedCheckinMessage detects unsupported checkin endpoints.
func isUnsupportedCheckinMessage(message string) bool {
	if message == "" {
		return false
	}
	text := strings.ToLower(message)
	return strings.Contains(text, "invalid url (post /api/user/checkin)") ||
		(strings.Contains(text, "http 404") && strings.Contains(text, "/api/user/checkin")) ||
		strings.Contains(text, "checkin endpoint not found") ||
		strings.Contains(text, "check-in is not supported") ||
		strings.Contains(text, "checkin is not supported") ||
		strings.Contains(text, "does not support checkin") ||
		strings.Contains(text, "not support checkin")
}

// isManualVerificationRequiredMessage detects Turnstile verification messages.
func isManualVerificationRequiredMessage(message string) bool {
	if message == "" {
		return false
	}
	text := strings.ToLower(message)
	return strings.Contains(text, "turnstile token 为空") ||
		(strings.Contains(text, "turnstile") && (strings.Contains(text, "token") || strings.Contains(text, "校验") || strings.Contains(text, "验证")))
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

// shouldAttemptAutoRelogin checks if auto-relogin should be attempted for checkin.
func shouldAttemptAutoRelogin(message string) bool {
	if message == "" {
		return false
	}
	if alert.IsTokenExpiredError(0, message) {
		return true
	}
	text := strings.ToLower(message)
	return strings.Contains(text, "new-api-user") || strings.Contains(text, "access token")
}

// tryAutoRelogin attempts to re-login and get a new access token.
func tryAutoRelogin(cfg *config.Config, db *sqlx.DB, account *store.Account, site *store.Site) (string, error) {
	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return "", fmt.Errorf("no adapter for platform %s", site.Platform)
	}

	relogin := service.GetAutoReloginConfig(account.ExtraConfig)
	if relogin == nil {
		return "", fmt.Errorf("no auto-relogin config")
	}

	password := service.DecryptPassword(cfg, relogin.PasswordCipher)
	if password == "" {
		return "", fmt.Errorf("failed to decrypt password")
	}

	proxyConfig := service.BuildPlatformProxyConfig(cfg, account, site)

	result, err := adp.Login(context.Background(), site.URL, relogin.Username, password, nil, proxyConfig)
	if err != nil || !result.Success || result.AccessToken == "" {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("login failed: %s", result.Message)
	}

	// Update DB
	newStatus := account.Status
	if account.Status == "expired" {
		newStatus = "active"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = db.Exec(
		db.Rebind("UPDATE accounts SET access_token = ?, status = ?, updated_at = ? WHERE id = ?"),
		result.AccessToken, newStatus, now, account.ID,
	)
	if err != nil {
		return "", err
	}

	return result.AccessToken, nil
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

// CheckinAccount performs a checkin for a single account.
// Mirrors TS checkinAccount().
func CheckinAccount(cfg *config.Config, db *sqlx.DB, accountID int64, options *CheckinOptions) CheckinResult {
	if options == nil {
		options = &CheckinOptions{}
	}

	// 0. Per-account in-progress lease: ensure the scheduler and a concurrent
	// admin manual trigger cannot double-run the same account simultaneously
	// (issue #669). Process-local; the scheduler-level advisory lock already
	// guards multi-instance runs. Released on every return path below.
	if !tryAcquireCheckinLease(accountID) {
		return CheckinResult{
			Success: true, Status: CheckinSkipped, Skipped: true,
			Reason: "already_in_progress", Message: "checkin already in progress for this account",
		}
	}
	defer releaseCheckinLease(accountID)

	// 1. Load account + site
	aws, err := service.GetAccountWithSiteByID(db, accountID)
	if err != nil {
		return CheckinResult{Success: false, Message: "account not found", Status: CheckinFailed}
	}
	account := &aws.Account
	site := &aws.Site

	// 2. Disabled checks
	if isAccountDisabled(account.Status) {
		createdAt := time.Now().UTC().Format(time.RFC3339)
		if err := service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthDisabled, Reason: "account disabled", Source: service.HealthSourceCheckin,
		}); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist runtime health: " + err.Error()}
		}

		if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, failure_reason, created_at) VALUES (?, ?, ?, ?, ?)"),
			accountID, "skipped", "account disabled", classifyAndMarshalFailureReason("account disabled", CheckinSkipped), createdAt); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
		}

		if !options.SkipEvent {
			msg := fmt.Sprintf("%s @ %s: account disabled", orUsername(account.Username, accountID), site.Name)
			if err := service.CreateEvent(db, "checkin", "checkin skipped", msg, "info", accountID, "account"); err != nil {
				slog.Warn("CheckinAccount: failed to persist disabled-account skipped event", "accountID", accountID, "error", err)
			}
		}

		return CheckinResult{
			Success: true, Status: CheckinSkipped, Skipped: true,
			Reason: "account_disabled", Message: "account disabled",
		}
	}

	if IsSiteDisabled(site.Status) {
		createdAt := time.Now().UTC().Format(time.RFC3339)
		if err := service.SetAccountRuntimeHealth(db, account.ID, service.RuntimeHealthEntry{
			State: service.HealthDisabled, Reason: "site disabled", Source: service.HealthSourceCheckin,
		}); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist runtime health: " + err.Error()}
		}

		if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, failure_reason, created_at) VALUES (?, ?, ?, ?, ?)"),
			accountID, "skipped", "site disabled", classifyAndMarshalFailureReason("site disabled", CheckinSkipped), createdAt); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
		}

		if !options.SkipEvent {
			msg := fmt.Sprintf("%s @ %s: site disabled", orUsername(account.Username, accountID), site.Name)
			if err := service.CreateEvent(db, "checkin", "checkin skipped", msg, "info", accountID, "account"); err != nil {
				slog.Warn("CheckinAccount: failed to persist skipped event", "accountID", accountID, "error", err)
			}
		}

		return CheckinResult{
			Success: true, Status: CheckinSkipped, Skipped: true,
			Reason: "site_disabled", Message: "site disabled",
		}
	}

	if !service.BuildCapabilitiesForAccount(account).CanCheckin {
		createdAt := time.Now().UTC().Format(time.RFC3339)
		message := "account credential mode does not support checkin"
		if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, failure_reason, created_at) VALUES (?, ?, ?, ?, ?)"),
			accountID, "skipped", message, classifyAndMarshalFailureReason(message, CheckinSkipped), createdAt); err != nil {
			return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
		}
		if !options.SkipEvent {
			msg := fmt.Sprintf("%s @ %s: %s", orUsername(account.Username, accountID), site.Name, message)
			if err := service.CreateEvent(db, "checkin", "checkin skipped", msg, "info", accountID, "account"); err != nil {
				slog.Warn("CheckinAccount: failed to persist proxy-only skipped event", "accountID", accountID, "error", err)
			}
		}
		return CheckinResult{
			Success: true, Status: CheckinSkipped, Skipped: true,
			Reason: "checkin_not_supported", Message: message,
		}
	}

	// 3. Get platform adapter
	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return CheckinResult{
			Success: false, Status: CheckinFailed,
			Message: fmt.Sprintf("unsupported platform: %s", site.Platform),
		}
	}

	// 4. Resolve platformUserId and proxy
	_, hasStored := service.GetPlatformUserIdFromExtraConfig(account.ExtraConfig)
	var guessedPlatformUserID int64
	if !hasStored {
		guessedPlatformUserID = service.GuessPlatformUserIdFromUsername(account.Username)
	}
	platformUserID := service.ResolvePlatformUserID(account.ExtraConfig, account.Username)
	proxyConfig := service.BuildPlatformProxyConfig(cfg, account, site)

	// Per-account checkin timeout (issue #669): a hung upstream must not block
	// the worker indefinitely. The budget covers the NewApi Bearer → cookie →
	// sign_in fallback chain; Detect still uses its own 5s withProbeTimeout.
	ctx, cancel := checkinContext(context.Background())
	defer cancel()

	// 5. First checkin attempt
	activeAccessToken := account.AccessToken
	result, err := adp.Checkin(ctx, site.URL, activeAccessToken, platformUserIDPtr(platformUserID), proxyConfig)
	if err != nil {
		result = &platform.CheckinResult{Success: false, Message: err.Error()}
	}

	// 6. Auth self-heal: on token-expiry signals OR a classified ClassAuth
	// failure (401/403 with auth residual), attempt ONE re-login to refresh
	// the session token, then retry the checkin once. Handles session expiry
	// between scheduled runs (issue #669). If re-login fails or the adapter
	// does not support login, the original auth error propagates unchanged.
	if !result.Success && shouldAttemptSelfHealLogin(result.Message, adp) {
		slog.Info("[checkin] session expired, attempting re-login", "accountID", account.ID)
		refreshedToken, reloginErr := tryAutoRelogin(cfg, db, account, site)
		if reloginErr != nil {
			slog.Warn("checkin: re-login failed; surfacing original auth error", "accountID", account.ID, "error", reloginErr)
		} else if refreshedToken != "" {
			activeAccessToken = refreshedToken
			result, err = adp.Checkin(ctx, site.URL, activeAccessToken, platformUserIDPtr(platformUserID), proxyConfig)
			if err != nil {
				result = &platform.CheckinResult{Success: false, Message: err.Error()}
			}
		}
	}

	// 6.5. Transient retry: if still failing AND the failure classifies as
	// transient (rate-limit / timeout / 5xx / network-like), wait a short
	// backoff and retry the checkin once. This protects the daily checkin
	// from a single transient blip. Auth/billing/model/validation failures
	// are NOT retried — they require operator intervention. Max 1 retry.
	if shouldRetryTransient(result) {
		time.Sleep(transientRetryBackoff())
		result, err = adp.Checkin(ctx, site.URL, activeAccessToken, platformUserIDPtr(platformUserID), proxyConfig)
		if err != nil {
			result = &platform.CheckinResult{Success: false, Message: err.Error()}
		}
	}

	// 7. Classify result
	isCloudflare := alert.IsCloudflareChallenge(result.Message)
	alreadyCheckedIn := isAlreadyCheckedInMessage(result.Message)
	unsupportedCheckin := isUnsupportedCheckinMessage(result.Message)
	manualVerificationRequired := isManualVerificationRequiredMessage(result.Message)
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

	// 8. Post-success processing
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

		if !hasStored && guessedPlatformUserID != 0 {
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

		// Parse reward
		parsedReward := ParseCheckinRewardAmount(logReward)
		if parsedReward <= 0 {
			parsedReward = ParseCheckinRewardAmount(result.Message)
			if parsedReward > 0 {
				logReward = fmt.Sprintf("%v", parsedReward)
			}
		}
		if directCheckinSuccess && parsedReward <= 0 {
			// Only infer a reward delta when the pre-checkin balance is known;
			// a NULL (never-refreshed) balance gives no baseline to diff against.
			if refreshedBalanceInfo != nil && account.Balance != nil {
				inferredReward := InferRewardFromBalanceDelta(*account.Balance, refreshedBalanceInfo.Balance)
				if inferredReward > 0 {
					parsedReward = inferredReward
					logReward = fmt.Sprintf("%v", parsedReward)
				}
			}
		}
	}

	// 9. Write checkin_logs
	createdAt := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(db.Rebind("INSERT INTO checkin_logs (account_id, status, message, reward, failure_reason, created_at) VALUES (?, ?, ?, ?, ?, ?)"),
		accountID, string(normalizedStatus), logMessage, logReward, classifyAndMarshalFailureReason(logMessage, normalizedStatus), createdAt); err != nil {
		return CheckinResult{Success: false, Status: CheckinFailed, Message: "failed to persist checkin log: " + err.Error()}
	}

	// 10. Write events
	if !options.SkipEvent {
		eventTitle := "checkin success"
		eventLevel := "info"
		if !effectiveSuccess {
			if isCloudflare {
				eventTitle = "checkin failed (cloudflare challenge)"
			} else {
				eventTitle = "checkin failed"
			}
			eventLevel = "error"
		} else if normalizedStatus == CheckinSkipped {
			eventTitle = "checkin skipped"
		}
		eventMsg := fmt.Sprintf("%s @ %s: %s", orUsername(account.Username, accountID), site.Name, logMessage)
		if err := service.CreateEvent(db, "checkin", eventTitle, eventMsg, eventLevel, accountID, "account"); err != nil {
			slog.Warn("CheckinAccount: failed to persist event", "accountID", accountID, "error", err)
		}
	}

	// 11. Post-failure processing
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
					"warning", nil,
				)
			}

			if !unsupportedCheckin && !manualVerificationRequired {
				notifypkg.SendNotification(config.RuntimeSafe(),
					"checkin failed",
					fmt.Sprintf("%s @ %s: %s", orUsername(account.Username, accountID), site.Name, result.Message),
					"error", nil,
				)
			}
		}
	}

	return CheckinResult{
		Success: effectiveSuccess,
		Status:  normalizedStatus,
		Skipped: normalizedStatus == CheckinSkipped,
		Message: logMessage,
		Reward:  logReward,
	}
}

// CheckinAll performs checkin for all eligible accounts.
// Mirrors TS checkinAll(). Per-account failure notifications are suppressed
// here (SkipNotification) and aggregated into ONE round notification sent at
// the end (issue #667): a 50-account failure burst now yields a single
// "Checkin round <id>: N ok, M failed" alert instead of 50 separate ones.
func CheckinAll(cfg *config.Config, db *sqlx.DB, accountIDs []int64, scheduleMode string) []CheckinAllResult {
	query := `SELECT a.id AS "accounts.id", a.site_id AS "accounts.site_id", a.username AS "accounts.username",
		a.access_token AS "accounts.access_token", a.balance AS "accounts.balance",
		a.balance_used AS "accounts.balance_used", a.quota AS "accounts.quota",
		a.status AS "accounts.status", a.checkin_enabled AS "accounts.checkin_enabled",
		a.last_checkin_at AS "accounts.last_checkin_at", a.extra_config AS "accounts.extra_config",
		s.id AS "sites.id", s.name AS "sites.name", s.url AS "sites.url",
		s.platform AS "sites.platform", s.status AS "sites.status"
		FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		WHERE a.checkin_enabled = TRUE AND a.status = 'active'`

	var rows []struct {
		Accounts struct {
			ID             int64   `db:"id"`
			SiteID         int64   `db:"site_id"`
			Username       *string `db:"username"`
			AccessToken    string  `db:"access_token"`
			Balance        float64 `db:"balance"`
			BalanceUsed    float64 `db:"balance_used"`
			Quota          float64 `db:"quota"`
			Status         string  `db:"status"`
			CheckinEnabled bool    `db:"checkin_enabled"`
			LastCheckinAt  *string `db:"last_checkin_at"`
			ExtraConfig    *string `db:"extra_config"`
		} `db:"accounts"`
		Sites struct {
			ID       int64  `db:"id"`
			Name     string `db:"name"`
			URL      string `db:"url"`
			Platform string `db:"platform"`
			Status   string `db:"status"`
		} `db:"sites"`
	}

	if err := db.Select(&rows, db.Rebind(query)); err != nil {
		slog.Error("CheckinAll: failed to query accounts", "error", err)
		return nil
	}

	// Filter by accountIDs if provided
	scopedIDs := make(map[int64]bool)
	if len(accountIDs) > 0 {
		for _, id := range accountIDs {
			scopedIDs[id] = true
		}
	}

	// Group by siteId
	grouped := make(map[int64][]int)
	for i, row := range rows {
		if len(scopedIDs) > 0 && !scopedIDs[row.Accounts.ID] {
			continue
		}
		siteID := row.Sites.ID
		grouped[siteID] = append(grouped[siteID], i)
	}

	var results []CheckinAllResult
	var mu sync.Mutex

	// Different sites: parallel. Same site: serial.
	var wg sync.WaitGroup
	for _, indices := range grouped {
		wg.Add(1)
		go func(indices []int) {
			defer wg.Done()
			for i, idx := range indices {
				if i > 0 {
					// Same-site pacing: space consecutive checkins on the same site
					// to avoid tripping upstream rate limits (429). Cross-site
					// parallelism is preserved by the per-site goroutine above.
					time.Sleep(sameSitePacingDelay())
				}
				row := rows[idx]
				result := CheckinAccount(cfg, db, row.Accounts.ID, &CheckinOptions{
					SkipEvent:        true,
					SkipNotification: true,
					ScheduleMode:     scheduleMode,
				})

				username := ""
				if row.Accounts.Username != nil {
					username = *row.Accounts.Username
				}

				mu.Lock()
				results = append(results, CheckinAllResult{
					AccountID: row.Accounts.ID,
					Username:  username,
					Site:      row.Sites.Name,
					Result:    result,
				})
				mu.Unlock()
			}
		}(indices)
	}
	wg.Wait()

	sendCheckinRoundNotification(cfg, results)

	return results
}

func orUsername(username *string, id int64) string {
	if username != nil && strings.TrimSpace(*username) != "" {
		return *username
	}
	return fmt.Sprintf("ID:%d", id)
}

// notifySend is the notification dispatch function used by round aggregation.
// It defaults to notifypkg.SendNotification; tests swap it to assert call
// counts and message contents without touching real notification channels.
var notifySend = notifypkg.SendNotification

// checkinFailure captures a single failed account within a checkin round, for
// inclusion in the aggregated round notification.
type checkinFailure struct {
	accountID   int64
	accountName string
	site        string
	error       string
}

// checkinRoundResult is the aggregated outcome of one CheckinAll round: the
// round identifier, the collected failures, and the success count. It backs
// the single consolidated notification that replaces N per-account alerts.
type checkinRoundResult struct {
	roundID   string
	failures  []checkinFailure
	successes int
}

// accountLabel resolves a display name for a CheckinAllResult, falling back
// to an ID-based label when the username is empty (mirrors orUsername but
// works off the already-resolved string carried by CheckinAllResult).
func accountLabel(username string, id int64) string {
	if strings.TrimSpace(username) != "" {
		return username
	}
	return fmt.Sprintf("ID:%d", id)
}

// firstLine returns the first line of a message, trimmed of surrounding
// whitespace. Keeps the aggregated notification body compact when an adapter
// returns a multi-line error blob.
func firstLine(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		return strings.TrimSpace(message[:idx])
	}
	return strings.TrimSpace(message)
}

// buildCheckinRoundResult aggregates CheckinAll results into a round summary.
// Skipped accounts (unsupported endpoint / manual verification / disabled)
// are neither successes nor failures for notification purposes. Returns nil
// when there are no failures, matching the current behavior of NOT alerting
// on a fully-clean round (all success or all skipped).
func buildCheckinRoundResult(results []CheckinAllResult) *checkinRoundResult {
	var failures []checkinFailure
	successes := 0
	for _, r := range results {
		switch r.Result.Status {
		case CheckinFailed:
			failures = append(failures, checkinFailure{
				accountID:   r.AccountID,
				accountName: accountLabel(r.Username, r.AccountID),
				site:        r.Site,
				error:       firstLine(r.Result.Message),
			})
		case CheckinSuccess:
			successes++
		}
	}
	if len(failures) == 0 {
		return nil
	}
	// Deterministic ordering across goroutine scheduling order so the
	// notification body is stable and diffable.
	sort.SliceStable(failures, func(i, j int) bool {
		if failures[i].accountName != failures[j].accountName {
			return failures[i].accountName < failures[j].accountName
		}
		return failures[i].accountID < failures[j].accountID
	})
	return &checkinRoundResult{
		roundID:   time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		successes: successes,
		failures:  failures,
	}
}

// maxFailuresPerNotification caps how many failure lines are rendered in the
// body so a 50-account outage stays a readable summary, not a wall of text.
const maxFailuresPerNotification = 25

// notificationTitle renders the round notification title:
// "Checkin round <id>: <ok> ok, <failed> failed".
func (rr *checkinRoundResult) notificationTitle() string {
	return fmt.Sprintf("Checkin round %s: %d ok, %d failed", rr.roundID, rr.successes, len(rr.failures))
}

// notificationBody renders one "account @ site: error" line per failure,
// truncated to maxFailuresPerNotification entries with a tail count.
func (rr *checkinRoundResult) notificationBody() string {
	var builder strings.Builder
	for i, f := range rr.failures {
		if i > 0 {
			builder.WriteString("\n")
		}
		if i >= maxFailuresPerNotification {
			fmt.Fprintf(&builder, "…and %d more", len(rr.failures)-maxFailuresPerNotification)
			break
		}
		fmt.Fprintf(&builder, "%s @ %s: %s", f.accountName, f.site, f.error)
	}
	return builder.String()
}

// sendCheckinRoundNotification aggregates a CheckinAll round's results into a
// single failure notification. No-op on a clean round (no failures). This is
// the issue #667 fix: one failed round → one notification, not N.
func sendCheckinRoundNotification(cfg *config.Config, results []CheckinAllResult) {
	round := buildCheckinRoundResult(results)
	if round == nil {
		return
	}
	notifySend(config.RuntimeSafe(), round.notificationTitle(), round.notificationBody(), "error", nil)
}

// shouldRetryTransient reports whether the current checkin result should be
// retried once as a transient failure (rate-limit / timeout / 5xx / network).
// Auth/billing/model/validation failures return false — they require operator
// intervention, not a retry. Used by CheckinAccount after the auto-relogin
// path (issue #668: transient-retry). Max 1 retry.
func shouldRetryTransient(result *platform.CheckinResult) bool {
	if result == nil || result.Success {
		return false
	}
	return platform.ClassifyUpstreamError(0, result.Message) == platform.ClassTransient
}

// adapterSupportsLogin reports whether adp can perform a real (network-backed)
// Login to refresh an expired session. StandardAdapter-based adapters and
// Sub2Api return a hardcoded "unsupported" message from Login without any
// network call, so re-login would always no-op. NewApi, the OneApi family
// and Veloera inherit BaseAdapter.Login (real POST /api/user/login) or
// override it. Unknown adapters default to true: a re-login attempt against
// an unsupported adapter is a cheap non-network no-op (issue #669).
func adapterSupportsLogin(adp platform.PlatformAdapter) bool {
	switch a := adp.(type) {
	case *platform.Sub2ApiAdapter:
		// JWT-only; Login always returns unsupported without a network call.
		return false
	case *platform.StandardAdapter:
		return a.LoginUnsupportedMessage == ""
	case *platform.OpenAiAdapter, *platform.ClaudeAdapter, *platform.GeminiAdapter,
		*platform.GeminiCliAdapter, *platform.AntigravityAdapter,
		*platform.CodexAdapter, *platform.CliProxyApiAdapter:
		// All StandardAdapter-based with Login explicitly unsupported.
		return false
	}
	return true
}

// shouldAttemptSelfHealLogin reports whether CheckinAccount should attempt a
// single re-login before retrying the checkin. True for confirmed token-expiry
// signals (the legacy shouldAttemptAutoRelogin path) OR for a classified
// ClassAuth failure (401/403 with auth residual) when the adapter can actually
// re-login. ClassAuth covers session-expiry-between-runs cases the legacy
// message-patterns miss (issue #669).
func shouldAttemptSelfHealLogin(message string, adp platform.PlatformAdapter) bool {
	if shouldAttemptAutoRelogin(message) {
		return true
	}
	return platform.ClassifyUpstreamError(0, message) == platform.ClassAuth && adapterSupportsLogin(adp)
}

// sameSitePacingDelay returns a ~1s delay (with minor jitter) used between
// consecutive same-site checkins in CheckinAll to avoid upstream rate limits
// (issue #668: same-site pacing).
func sameSitePacingDelay() time.Duration {
	return time.Second + time.Duration(rand.IntN(250))*time.Millisecond
}

// transientRetryBackoff returns a ~2-3s backoff used before retrying a transient
// checkin failure (rate-limit / timeout / 5xx / network) in CheckinAccount.
func transientRetryBackoff() time.Duration {
	return 2*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
}
