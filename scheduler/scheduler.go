// Package scheduler implements the 15+ background schedulers from.
// Each scheduler implements the Scheduler interface and is started/stopped by
// the registry on app startup/shutdown.
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// registryStopBudget bounds the overall StopAll wait. Individual schedulers
// already cap their own drain (see stopDrainBudget in cron.go); this outer
// budget is belt-and-braces so a stuck Stop cannot stall graceful shutdown.
const registryStopBudget = 10 * time.Second

// Scheduler is the interface for all background schedulers.
type Scheduler interface {
	Name() string
	Start(ctx context.Context) error
	Stop() error
}

// Registry manages all registered schedulers.
type Registry struct {
	schedulers []Scheduler
}

// NewRegistry creates a new empty scheduler registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a scheduler to the registry.
func (r *Registry) Register(s Scheduler) {
	r.schedulers = append(r.schedulers, s)
}

// StartAll starts all registered schedulers. Each runs in its own goroutine
// with panic recovery so a single scheduler panic does not affect others.
func (r *Registry) StartAll(ctx context.Context) {
	for _, s := range r.schedulers {
		go func(s Scheduler) {
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("scheduler panicked during start",
						"name", s.Name(),
						"panic", rec,
					)
				}
			}()
			if err := s.Start(ctx); err != nil {
				slog.Warn("scheduler start failed",
					"name", s.Name(),
					"error", err,
				)
			}
		}(s)
	}
}

// StopAll stops all registered schedulers, logging errors but continuing
// through all schedulers. Stops run concurrently and the call is bounded by
// registryStopBudget so a stuck Stop cannot stall graceful shutdown.
func (r *Registry) StopAll() {
	r.StopAllWithTimeout(registryStopBudget)
}

// StopAllWithTimeout stops all registered schedulers concurrently, waiting
// up to budget for every Stop to return. Each scheduler's Stop is internally
// bounded (see cron.go), so this outer deadline only matters if a Stop hangs
// outright. Stops still running when the budget expires are logged and left
// to finish on their own (process exit follows shortly after).
func (r *Registry) StopAllWithTimeout(budget time.Duration) {
	var wg sync.WaitGroup
	for _, s := range r.schedulers {
		wg.Add(1)
		go func(s Scheduler) {
			defer wg.Done()
			defer func() {
				if rec := recover(); rec != nil {
					slog.Error("scheduler panicked during stop",
						"name", s.Name(),
						"panic", rec,
					)
				}
			}()
			if err := s.Stop(); err != nil {
				slog.Warn("scheduler stop failed",
					"name", s.Name(),
					"error", err,
				)
			}
		}(s)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(budget):
		slog.Warn("scheduler StopAll budget exceeded; some schedulers may still be draining",
			"budget", budget,
		)
	}
}

// List returns the names of all registered schedulers.
func (r *Registry) List() []string {
	names := make([]string, len(r.schedulers))
	for i, s := range r.schedulers {
		names[i] = s.Name()
	}
	return names
}
