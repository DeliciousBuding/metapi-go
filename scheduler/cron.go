package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/robfig/cron/v3"
)

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
			if r := recover(); r != nil {
				_ = r // panic recovered; logged inside the job itself
			}
		}()
		fn()
	})
}

// removeJob removes a cron job by entry ID.
func (cr *cronRunner) removeJob(id cron.EntryID) {
	cr.cron.Remove(id)
}

// start begins executing scheduled jobs.
func (cr *cronRunner) start() {
	cr.cron.Start()
}

// stop returns a context that is done when all running jobs have completed.
func (cr *cronRunner) stop() context.Context {
	return cr.cron.Stop()
}

// intervalRunner owns the ticker/stopCh/running boilerplate shared by the
// fixed-interval schedulers (update-center, model-probe, site-announcement,
// channel-recovery, usage-aggregation, admin-snapshot, oauth-refresh,
// sub2api-refresh and the retention trio). Semantics:
//   - Start creates a ticker, marks running, optionally fires the first run
//     immediately in its own goroutine, then spawns the select loop (one
//     goroutine per tick).
//   - The loop exits on stopCh close or ctx.Done; Stop is idempotent.
type intervalRunner struct {
	ticker  *time.Ticker
	stopCh  chan struct{}
	running bool
	mu      sync.Mutex
}

// start begins the tick loop. The run func is executed once per tick in its
// own goroutine; when immediate is true it is also executed once right away.
// Returns nil when the runner is already running.
func (r *intervalRunner) start(ctx context.Context, interval time.Duration, immediate bool, run func()) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return nil
	}
	r.ticker = time.NewTicker(interval)
	r.stopCh = make(chan struct{})
	r.running = true

	if immediate {
		go run()
	}

	go func() {
		for {
			select {
			case <-r.ticker.C:
				go run()
			case <-r.stopCh:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// stop halts the tick loop. Idempotent; returns nil when not running.
func (r *intervalRunner) stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return nil
	}
	r.running = false
	r.ticker.Stop()
	close(r.stopCh)
	return nil
}
