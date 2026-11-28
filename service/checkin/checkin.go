package checkin

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
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

// CheckinAccount performs a checkin for a single account.
// Mirrors TS checkinAccount().
func CheckinAccount(cfg *config.Config, db *sqlx.DB, accountID int64, options *CheckinOptions) CheckinResult {
	if options == nil {
		options = &CheckinOptions{}
	}

	// Per-account in-progress lease: ensure the scheduler and a concurrent
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

	// Load account + site
	aws, err := service.GetAccountWithSiteByID(db, accountID)
	if err != nil {
		return CheckinResult{Success: false, Message: "account not found", Status: CheckinFailed}
	}

	if skipped := skipIneligibleAccount(db, aws, options); skipped != nil {
		return *skipped
	}

	account := &aws.Account
	site := &aws.Site
	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		return CheckinResult{
			Success: false, Status: CheckinFailed,
			Message: fmt.Sprintf("unsupported platform: %s", site.Platform),
		}
	}

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

	attempt := func(accessToken string) *platform.CheckinResult {
		result, err := adp.Checkin(ctx, site.URL, accessToken, platformUserIDPtr(platformUserID), proxyConfig)
		if err != nil {
			return &platform.CheckinResult{Message: err.Error()}
		}
		return result
	}
	activeAccessToken := account.AccessToken
	result := attempt(activeAccessToken)

	// Auth self-heal: on token-expiry signals OR a classified ClassAuth
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
			result = attempt(activeAccessToken)
		}
	}

	// Transient retry: if still failing AND the failure classifies as
	// transient (rate-limit / timeout / 5xx / network-like), wait a short
	// backoff and retry the checkin once. This protects the daily checkin
	// from a single transient blip. Auth/billing/model/validation failures
	// are NOT retried — they require operator intervention. Max 1 retry.
	if shouldRetryTransient(result) {
		// The backoff is cancellable: the ctx already bounds the retry below, so
		// sleeping it out after the budget expired only delayed the verdict and
		// then paid for one more upstream call on top of it.
		if waitErr := sleepCtx(ctx, transientRetryBackoff()); waitErr != nil {
			slog.Warn("[checkin] transient-retry backoff interrupted; skipping the retry",
				"accountID", account.ID, "error", waitErr)
			// Keep the upstream message (the failure classifier and checkin_logs
			// read it) and record why the retry never happened.
			result.Message = fmt.Sprintf("%s; transient retry skipped: %v", result.Message, waitErr)
		} else {
			result = attempt(activeAccessToken)
		}
	}

	return recordCheckinResult(cfg, db, aws, options, result, guessedPlatformUserID)
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

	result, err := platform.LoginReusingCredential(adp, context.Background(), site.URL, relogin.Username, password, nil, proxyConfig, account.AccessToken)
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

// transientRetryBackoff returns a ~2-3s backoff used before retrying a transient
// checkin failure (rate-limit / timeout / 5xx / network) in CheckinAccount.
func transientRetryBackoff() time.Duration {
	return 2*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
}

// sleepCtx waits for d and returns nil, or returns ctx.Err() as soon as ctx
// ends. It is the cancellable form of time.Sleep for the waits that sit between
// upstream calls in this package (the transient-retry backoff and the same-site
// pacing): a bare sleep keeps the worker asleep — and keeps the upstream call
// after it alive — once a shutdown or an exhausted budget has already decided
// the work is over. The duration semantics are unchanged, only the wait became
// interruptible; service/oauth/retry.go guards its backoff the same way.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
