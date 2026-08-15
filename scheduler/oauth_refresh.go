package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

const (
	oauthRefreshSchedulerIntervalMs    = 60_000 // 60s, matches TS original
	oauthRefreshSchedulerMinIntervalMs = 60_000

	// Default lead time when provider not listed (5 min).
	defaultOauthRefreshLeadMs = 5 * 60 * 1000
)

// leadMsByProvider mirrors OAUTH_REFRESH_LEAD_BY_PROVIDER in the TS original.
// Tokens are refreshed when tokenExpiresAt - now <= lead time.
var leadMsByProvider = map[string]int64{
	"codex":       5 * 24 * 60 * 60 * 1000, // 5 days
	"claude":      4 * 60 * 60 * 1000,      // 4 hours
	"gemini-cli":  5 * 60 * 1000,           // 5 minutes
	"antigravity": 5 * 60 * 1000,           // 5 minutes
}

// OAuthRefreshScheduler periodically scans OAuth accounts and refreshes
// tokens nearing expiry. Mirrors the TS oauthRefreshScheduler.ts.
type OAuthRefreshScheduler struct {
	cfg          *config.Config
	mu           sync.Mutex
	passInFlight bool
	runner       *intervalRunner
	// ctx is the lifecycle context captured from Start. Job timeouts derive
	// from it instead of context.Background() so Stop (which cancels it) also
	// cancels in-flight refresh passes on shutdown. Defaults to
	// context.Background() so behavior without Start matches the old code.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewOAuthRefreshScheduler creates a new OAuth token refresh scheduler.
func NewOAuthRefreshScheduler(cfg *config.Config) *OAuthRefreshScheduler {
	return &OAuthRefreshScheduler{
		cfg:    cfg,
		runner: &intervalRunner{},
		ctx:    context.Background(),
	}
}

func (s *OAuthRefreshScheduler) Name() string { return "oauth-refresh" }

func (s *OAuthRefreshScheduler) Start(ctx context.Context) error {
	// Capture the runner lifecycle context so per-job timeouts derive from it
	// (cancelled on Stop) instead of context.Background() (never cancelled).
	s.ctx, s.cancel = context.WithCancel(ctx)

	intervalMs := config.MaxInt(oauthRefreshSchedulerIntervalMs, oauthRefreshSchedulerMinIntervalMs)
	interval := time.Duration(intervalMs) * time.Millisecond

	slog.Info("oauth-refresh scheduler started", "interval_ms", intervalMs)
	return s.runner.start(ctx, interval, true, s.runPass)
}

func (s *OAuthRefreshScheduler) Stop() error {
	// Cancel the lifecycle context first so any in-flight refresh pass whose
	// job timeout derives from it aborts promptly. The runner stop then halts
	// future ticks. Lease Release falls back to context.Background() when the
	// job ctx is already done, so this does not strand an advisory lock.
	if s.cancel != nil {
		s.cancel()
	}
	return s.runner.stop()
}

// OAuthRefreshResult is the outcome of a single refresh pass.
type OAuthRefreshResult struct {
	Scanned             int
	Refreshed           int
	Failed              int
	Skipped             int
	RefreshedAccountIDs []int64
	FailedAccountIDs    []int64
}

func getOauthRefreshLeadMs(provider string) int64 {
	if v, ok := leadMsByProvider[provider]; ok {
		return v
	}
	return defaultOauthRefreshLeadMs
}

func (s *OAuthRefreshScheduler) runPass() {
	s.mu.Lock()
	if s.passInFlight {
		s.mu.Unlock()
		return
	}
	s.passInFlight = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.passInFlight = false
		s.mu.Unlock()
	}()

	dbw := store.GetDB()
	if dbw == nil {
		return
	}
	// Derive the job timeout from the lifecycle ctx (cancellable on Stop),
	// not context.Background(). runWithSchedulerLease still releases the
	// lease via context.Background() when this ctx is already done.
	jobCtx, cancel := context.WithTimeout(s.ctx, oauthRefreshJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runPassLocked()
	})
}

func (s *OAuthRefreshScheduler) runPassLocked() {
	nowMs := time.Now().UnixMilli()
	rows, err := oauth.ListOAuthRefreshCandidates()
	if err != nil {
		slog.Warn("oauth-refresh: query failed", "error", err)
		return
	}

	result := &OAuthRefreshResult{
		Scanned: len(rows),
	}

	for _, row := range rows {
		// Skip if account or site is not active.
		if row.Account.Status != "active" || row.SiteStatus != "active" {
			result.Skipped++
			continue
		}

		oauthInfo := oauth.GetOauthInfoFromAccount(&row.Account)
		if oauthInfo == nil || oauthInfo.RefreshToken == "" {
			result.Skipped++
			continue
		}

		if oauthInfo.TokenExpiresAt <= 0 {
			result.Skipped++
			continue
		}

		leadMs := getOauthRefreshLeadMs(oauthInfo.Provider)
		if oauthInfo.TokenExpiresAt-nowMs > leadMs {
			result.Skipped++
			continue
		}

		// Token is within lead window — refresh.
		_, err := oauth.RefreshAccessTokenSingleflight(row.Account.ID)
		if err != nil {
			result.Failed++
			result.FailedAccountIDs = append(result.FailedAccountIDs, row.Account.ID)
			slog.Warn("oauth-refresh: refresh failed",
				"account_id", row.Account.ID,
				"provider", oauthInfo.Provider,
				"error", err)
		} else {
			result.Refreshed++
			result.RefreshedAccountIDs = append(result.RefreshedAccountIDs, row.Account.ID)
		}
	}

	if result.Refreshed > 0 || result.Failed > 0 {
		slog.Info("oauth-refresh pass complete",
			"scanned", result.Scanned,
			"refreshed", result.Refreshed,
			"failed", result.Failed,
			"skipped", result.Skipped)
	}
}
