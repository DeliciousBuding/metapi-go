package scheduler

import (
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// ProbeRunSummary summarizes one background probe pass.
type ProbeRunSummary struct {
	AccountsConsidered int
	AccountsProbed     int
	TargetsScanned     int
	Success            int
	Failed             int
	Inconclusive       int
	Skipped            int
	StartedAtMs        int64
	CompletedAtMs      int64
}

// TryAcquireAccountLease attempts to acquire a lease for probing a specific account.
// Returns true if the lease was acquired.
func (s *ModelProbeScheduler) TryAcquireAccountLease(accountID int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.accountLeases[accountID] {
		return false
	}
	s.accountLeases[accountID] = true
	return true
}

// ReleaseAccountLease releases the lease for an account.
func (s *ModelProbeScheduler) ReleaseAccountLease(accountID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.accountLeases, accountID)
}

// ResetLeases clears all account leases (for tests).
func (s *ModelProbeScheduler) ResetLeases() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountLeases = make(map[int64]bool)
}

// probeRunHistoryDepth caps how many completed passes RecentRunSummaries
// retains. Enough for the "last few runs" scheduler-status view without
// unbounded memory growth (each entry is a handful of ints).
const probeRunHistoryDepth = 10

// LastRunSummary returns a copy of the most recent probe pass summary.
func (s *ModelProbeScheduler) LastRunSummary() ProbeRunSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRunSummary
}

// RecentRunSummaries returns up to probeRunHistoryDepth completed passes,
// newest first. Only background passes (runProbe) land in the ring buffer —
// one-shot site probe-now passes are not scheduler runs and are excluded.
func (s *ModelProbeScheduler) RecentRunSummaries() []ProbeRunSummary {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProbeRunSummary, len(s.lastRuns))
	copy(out, s.lastRuns)
	return out
}

func (s *ModelProbeScheduler) runProbeLocked(dbw *store.DB) {
	slog.Info("model-probe: starting availability probe")

	summary := ProbeRunSummary{StartedAtMs: time.Now().UnixMilli()}
	defer func() {
		summary.CompletedAtMs = time.Now().UnixMilli()
		s.mu.Lock()
		s.lastRunSummary = summary
		s.lastRuns = append([]ProbeRunSummary{summary}, s.lastRuns...)
		if len(s.lastRuns) > probeRunHistoryDepth {
			s.lastRuns = s.lastRuns[:probeRunHistoryDepth]
		}
		s.mu.Unlock()
	}()

	targets, err := s.loadProbeTargets(dbw)
	if err != nil {
		slog.Error("model-probe: failed to load probe targets", "error", err)
		return
	}
	if len(targets) == 0 {
		slog.Info("model-probe: no probe targets")
		return
	}

	// Group by account so account leases still gate concurrent work.
	byAccount := make(map[int64][]ProbeTarget)
	for _, t := range targets {
		byAccount[t.AccountID] = append(byAccount[t.AccountID], t)
	}
	summary.AccountsConsidered = len(byAccount)

	var availableAccounts []int64
	for accountID := range byAccount {
		if s.TryAcquireAccountLease(accountID) {
			availableAccounts = append(availableAccounts, accountID)
		}
	}
	if len(availableAccounts) == 0 {
		slog.Info("model-probe: no accounts available (all leased)")
		return
	}

	timeoutMs := s.cfg.ModelAvailabilityProbeTimeoutMs
	if timeoutMs < 3000 {
		timeoutMs = 3000
	}
	concurrency := s.cfg.ModelAvailabilityProbeConcurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > 16 {
		concurrency = 16
	}

	// Flatten leased targets and run with bounded concurrency.
	var work []ProbeTarget
	for _, accountID := range availableAccounts {
		work = append(work, byAccount[accountID]...)
	}
	summary.AccountsProbed = len(availableAccounts)
	summary.TargetsScanned = len(work)

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, target := range work {
		target := target
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			// Boundary recovery: a panicking probe (e.g. adapter nil
			// dereference) must not take the process down; the wg/sem
			// defers above still run, so the pass drains normally.
			safeJob("model-probe-target", func() {
				outcome := s.probeOne(target, timeoutMs, dbw)
				mu.Lock()
				switch outcome {
				case "success":
					summary.Success++
				case "failure":
					summary.Failed++
				case "inconclusive":
					summary.Inconclusive++
				default:
					summary.Skipped++
				}
				mu.Unlock()
			})
		}()
	}
	wg.Wait()

	for _, accountID := range availableAccounts {
		s.ReleaseAccountLease(accountID)
	}

	slog.Info("model-probe: probe complete",
		"accounts", summary.AccountsProbed,
		"targets", summary.TargetsScanned,
		"success", summary.Success,
		"failed", summary.Failed,
		"inconclusive", summary.Inconclusive,
		"skipped", summary.Skipped,
	)
}
