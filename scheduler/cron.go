package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/robfig/cron/v3"
)

// stopDrainBudget bounds how long cronRunner.stop waits for in-flight cron
// jobs and intervalRunner.stop waits for in-flight runs before giving up.
// Mirrors the bounded proxy_log batch-writer drain in cmd/server/main.go:
// graceful shutdown must see a drained scheduler, but a stuck job cannot be
// allowed to block shutdown indefinitely.
const stopDrainBudget = 5 * time.Second

// maxStartupJitter caps the immediate-run startup jitter. Without a cap,
// long intervals (e.g. 60min retention passes) would delay their first run
// by up to interval/10 (6min); 30s spreads the startup burst without
// meaningfully delaying any first pass.
const maxStartupJitter = 30 * time.Second

// defaultStartupJitter returns a uniform random delay in [0, interval/10),
// capped at maxStartupJitter. Registry.StartAll starts every scheduler in
// the same instant; without jitter all immediate first runs hit the DB
// within the same ~2ms window and same-interval passes stay aligned on the
// same millisecond forever. Injecting a per-runner delay both spreads the
// startup burst and de-synchronizes steady-state ticks. math/rand is
// auto-seeded since Go 1.20, so every process gets a fresh spread.
func defaultStartupJitter(interval time.Duration) time.Duration {
	spread := interval / 10
	if spread > maxStartupJitter {
		spread = maxStartupJitter
	}
	if spread <= 0 {
		return 0
	}
	return time.Duration(rand.Int63n(int64(spread)))
}

// RandomCronInWindow picks a random HH:mm inside [start, end] and returns a
// daily cron expression "m h * * *" (5-field). Bounds are "HH:mm" in 24h
// format; start must be <= end. E1: the roll is re-done
// per scheduler start / schedule update, giving load spreading + anti-
// fingerprint behavior for daily tasks like check-in without a re-roll job.
func RandomCronInWindow(start, end string) (string, error) {
	startMin, err := parseHHMM(start)
	if err != nil {
		return "", fmt.Errorf("invalid window start %q: %w", start, err)
	}
	endMin, err := parseHHMM(end)
	if err != nil {
		return "", fmt.Errorf("invalid window end %q: %w", end, err)
	}
	if startMin > endMin {
		return "", fmt.Errorf("window start %s is after end %s", start, end)
	}

	span := endMin - startMin + 1 // inclusive
	roll := startMin + rand.Intn(span)
	return fmt.Sprintf("%d %d * * *", roll%60, roll/60), nil
}

// parseHHMM parses "HH:mm" (24h) into minutes since midnight. Zero-padded
// hours/minutes are expected but single digits are tolerated.
func parseHHMM(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:mm, got %q", raw)
	}
	var h, m int
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0, fmt.Errorf("bad hour %q", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0, fmt.Errorf("bad minute %q", parts[1])
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("out of range %q", raw)
	}
	return h*60 + m, nil
}

// normalizeCronExpr auto-detects 5-field cron expressions (minute hour dom month dow)
// and prepends "0 " to make them 6-field (second minute hour dom month dow).
// This ensures compatibility with cron.WithSeconds() while accepting 5-field
// expressions commonly stored by the TypeScript frontend.
func normalizeCronExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return expr
	}
	fields := strings.Fields(expr)
	if len(fields) == 5 {
		return "0 " + expr
	}
	return expr
}

// ValidateCronExpr validates a cron expression using robfig/cron parser
// with seconds field. Auto-converts 5-field expressions to 6-field.
// Returns true if the expression is valid.
// Thin wrapper over config.ValidateCronExpr (the canonical implementation).
func ValidateCronExpr(expr string) bool {
	return config.ValidateCronExpr(expr)
}

// ParseCronExpr tries to parse a cron expression. Auto-converts 5-field
// expressions to 6-field. Returns error if invalid.
func ParseCronExpr(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("empty cron expression")
	}
	expr = normalizeCronExpr(expr)
	parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err
}

// safeJob runs fn with panic recovery at the scheduler boundary. Every
// goroutine the scheduler package launches job code in must go through this
// wrapper (or cronRunner.addJob's equivalent for cron-triggered runs): an
// unrecovered panic in any of them crashes the entire server process, not
// just the job. Recovery logs in the same shape as cronRunner.addJob and
// returns normally so the surrounding runner (tick loop, drain WaitGroup,
// worker pool) keeps functioning.
func safeJob(name string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("scheduler job panicked", "name", name, "panic", rec)
		}
	}()
	fn()
}

// cronRunner wraps a robfig/cron scheduler with panic-safe job execution.
type cronRunner struct {
	cron *cron.Cron
}

// newCronRunner creates a new cron runner with seconds field support.
func newCronRunner() *cronRunner {
	return &cronRunner{
		cron: cron.New(cron.WithSeconds()),
	}
}

// addJob adds a cron job with panic recovery. Auto-converts 5-field cron
// expressions to 6-field for compatibility with cron.WithSeconds().
// Returns the entry ID and error.
func (cr *cronRunner) addJob(spec string, fn func()) (cron.EntryID, error) {
	spec = normalizeCronExpr(spec)
	return cr.cron.AddFunc(spec, func() {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("cron job panicked", "spec", spec, "panic", rec)
			}
		}()
		fn()
	})
}

// start begins executing scheduled jobs.
func (cr *cronRunner) start() {
	cr.cron.Start()
}

// stop halts scheduling and waits (boundedly) for in-flight jobs to finish.
// robfig's Cron.Stop returns a context that closes once every running job
// has returned; waiting on it instead of discarding it is what makes a
// graceful shutdown actually observe a drained scheduler. Waits are capped
// by stopDrainBudget so a stuck job cannot block shutdown forever.
func (cr *cronRunner) stop() {
	drained := cr.cron.Stop()
	select {
	case <-drained.Done():
	case <-time.After(stopDrainBudget):
		slog.Warn("cron stop: in-flight jobs did not drain within budget", "budget", stopDrainBudget)
	}
}

// intervalRunner owns the ticker/stopCh/running boilerplate shared by the
// fixed-interval schedulers (update-center, model-probe, site-announcement,
// channel-recovery, usage-aggregation, admin-snapshot, oauth-refresh,
// sub2api-refresh and the retention trio). Semantics:
//   - Start creates a ticker, marks running, optionally fires the first run
//     after a startup jitter delay (see defaultStartupJitter) in its own
//     goroutine, then spawns the select loop (one goroutine per tick).
//   - Every run (immediate or tick-fired) is tracked in wg so stop can
//     drain in-flight runs instead of abandoning them.
//   - The loop exits on stopCh close or ctx.Done; Stop is idempotent.
type intervalRunner struct {
	ticker  *time.Ticker
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
	// wg tracks in-flight runs so stop can wait for them to finish.
	wg sync.WaitGroup
	// jitter overrides the immediate-run startup delay; nil means
	// defaultStartupJitter. Injectable so tests can pin the delay instead of
	// depending on the random draw.
	jitter func(interval time.Duration) time.Duration
}

// startupJitter resolves the immediate-run startup delay.
func (r *intervalRunner) startupJitter(interval time.Duration) time.Duration {
	if r.jitter != nil {
		return r.jitter(interval)
	}
	return defaultStartupJitter(interval)
}

// start begins the tick loop. The run func is executed once per tick in its
// own goroutine; when immediate is true it is also executed once right away,
// delayed by a startup jitter (see defaultStartupJitter) so a full registry
// start does not fire every scheduler in the same instant. All runs are
// tracked by the drain WaitGroup. Returns nil when the runner is already
// running.
func (r *intervalRunner) start(ctx context.Context, interval time.Duration, immediate bool, run func()) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	// Boundary panic recovery: runs execute in bare goroutines (immediate
	// run and every tick), so a panicking job would otherwise crash the
	// whole process. Wrap once here so both paths are covered without
	// per-job ceremony.
	safeRun := func() { safeJob("interval-runner", run) }
	r.ticker = time.NewTicker(interval)
	r.stopCh = make(chan struct{})
	r.running = true

	if immediate {
		// Safe to Add under the lock: stop only Waits after observing
		// running==true under the same lock.
		r.wg.Add(1)
		delay := r.startupJitter(interval)
		go func() {
			defer r.wg.Done()
			if delay > 0 {
				timer := time.NewTimer(delay)
				defer timer.Stop()
				select {
				case <-timer.C:
				case <-r.stopCh:
					// Stopped while waiting out the jitter delay: drop the
					// run instead of starting work during shutdown.
					return
				}
			}
			safeRun()
		}()
	}

	go func() {
		for {
			select {
			case <-r.ticker.C:
				r.launch(safeRun)
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// launch starts run in its own goroutine tracked by the drain WaitGroup.
// The mutex serializes with stop so a late Add cannot race a Wait that
// already observed zero in-flight runs; a tick dropped because stop won the
// race is intended — stop means "no new runs".
func (r *intervalRunner) launch(run func()) {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.wg.Add(1)
	r.mu.Unlock()
	go func() {
		defer r.wg.Done()
		run()
	}()
}

// stop halts the tick loop and waits (boundedly) for in-flight runs to
// finish. Idempotent; returns nil when not running. The wait is capped by
// stopDrainBudget so a stuck run cannot block shutdown forever.
func (r *intervalRunner) stop() error {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return nil
	}
	r.running = false
	r.ticker.Stop()
	close(r.stopCh)
	r.mu.Unlock()

	drained := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(stopDrainBudget):
		slog.Warn("interval runner stop: in-flight runs did not finish within budget", "budget", stopDrainBudget)
	}
	return nil
}
