package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/deliciousbuding/metapi-go/store"
)

const (
	sub2apiRefreshIntervalMs    = 60_000 // 60s
	sub2apiRefreshMinIntervalMs = 60_000
	sub2apiRefreshConcurrency   = 4
)

// Sub2APIRefreshScheduler periodically refreshes Sub2API managed authentication
// tokens for eligible accounts via balance.RefreshBalance.
type Sub2APIRefreshScheduler struct {
	cfg          *config.Config
	mu           sync.Mutex
	passInFlight bool
	runner       *intervalRunner
}

// NewSub2APIRefreshScheduler creates a new Sub2API refresh scheduler.
func NewSub2APIRefreshScheduler(cfg *config.Config) *Sub2APIRefreshScheduler {
	return &Sub2APIRefreshScheduler{cfg: cfg, runner: &intervalRunner{}}
}

func (s *Sub2APIRefreshScheduler) Name() string { return "sub2api-refresh" }

func (s *Sub2APIRefreshScheduler) Start(ctx context.Context) error {
	intervalMs := int64(config.MaxInt(sub2apiRefreshIntervalMs, sub2apiRefreshMinIntervalMs))
	interval := time.Duration(intervalMs) * time.Millisecond

	slog.Info("sub2api-refresh scheduler started", "interval_ms", intervalMs)
	return s.runner.start(ctx, interval, true, s.runPass)
}

func (s *Sub2APIRefreshScheduler) Stop() error {
	return s.runner.stop()
}

// Sub2ApiRefreshResult is the result of a refresh pass.
type Sub2ApiRefreshResult struct {
	Scanned             int
	Eligible            int
	Refreshed           int
	Failed              int
	Skipped             int
	RefreshedAccountIDs []int64
	FailedAccountIDs    []int64
}

func (s *Sub2APIRefreshScheduler) runPass() {
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
	runWithSchedulerLease(context.Background(), dbw, s.Name(), func() {
		s.runPassLocked(dbw)
	})
}

func (s *Sub2APIRefreshScheduler) runPassLocked(dbw *store.DB) {
	slog.Info("sub2api-refresh: running pass")

	rows, err := dbw.Query(`
		SELECT a.id, a.extra_config
		FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE LOWER(TRIM(COALESCE(s.platform, ''))) = 'sub2api'
		  AND a.status = 'active'
		  AND s.status = 'active'
	`)
	if err != nil {
		slog.Error("sub2api-refresh: failed to query accounts", "error", err)
		return
	}
	defer rows.Close()

	type candidate struct {
		id int64
	}

	var scanned int
	var eligible []candidate
	for rows.Next() {
		var id int64
		var extraConfig *string
		if err := rows.Scan(&id, &extraConfig); err != nil {
			continue
		}
		scanned++
		if !isSub2APIRefreshCandidate(extraConfig) {
			continue
		}
		eligible = append(eligible, candidate{id: id})
	}

	if len(eligible) == 0 {
		slog.Info("sub2api-refresh: pass complete",
			"scanned", scanned,
			"eligible", 0,
			"refreshed", 0,
			"failed", 0,
		)
		return
	}

	concurrency := sub2apiRefreshConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var refreshed atomic.Int64
	var failed atomic.Int64

	for _, c := range eligible {
		c := c
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := balance.RefreshBalance(s.cfg, dbw.DB, c.id)
			if err != nil {
				failed.Add(1)
				slog.Warn("sub2api-refresh: RefreshBalance failed",
					"account_id", c.id, "error", err)
				return
			}
			refreshed.Add(1)
		}()
	}
	wg.Wait()

	slog.Info("sub2api-refresh: pass complete",
		"scanned", scanned,
		"eligible", len(eligible),
		"refreshed", refreshed.Load(),
		"failed", failed.Load(),
	)
}

// isSub2APIRefreshCandidate reports whether extraConfig carries managed Sub2API
// auth that is due for refresh.
func isSub2APIRefreshCandidate(extraConfig *string) bool {
	auth := service.GetSub2ApiAuthFromExtraConfig(extraConfig)
	if auth == nil {
		return false
	}
	rt, ok := service.NormalizeManagedRefreshToken(auth["refreshToken"])
	if !ok || rt == "" {
		return false
	}
	return service.IsManagedSub2ApiTokenDue(auth["tokenExpiresAt"])
}
