package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/daily"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/deliciousbuding/metapi-go/store"
)

const dailySummaryDefaultCron = "58 23 * * *"

// DailySummaryScheduler sends a daily summary notification at 23:58 daily.
type DailySummaryScheduler struct {
	cronRunner *cronRunner
}

// NewDailySummaryScheduler creates a new daily summary scheduler.
func NewDailySummaryScheduler() *DailySummaryScheduler {
	return &DailySummaryScheduler{}
}

func (s *DailySummaryScheduler) Name() string { return "daily-summary" }

func (s *DailySummaryScheduler) Start(ctx context.Context) error {
	activeCron := resolveCronSetting("daily_summary_cron", dailySummaryDefaultCron)
	s.cronRunner = newCronRunner()
	_, err := s.cronRunner.addJob(activeCron, s.runJob)
	if err != nil {
		slog.Error("daily-summary: failed to add cron job", "error", err)
		return err
	}
	s.cronRunner.start()
	slog.Info("daily-summary scheduler started", "cron", activeCron)
	return nil
}

func (s *DailySummaryScheduler) Stop() error {
	if s.cronRunner != nil {
		s.cronRunner.stop()
		s.cronRunner = nil
	}
	return nil
}

func (s *DailySummaryScheduler) runJob() {
	slog.Info("daily-summary: collecting metrics")
	dbw := store.GetDB()
	if dbw == nil {
		slog.Error("daily-summary: database not available")
		return
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), dailySummaryJobTimeout)
	defer cancel()
	runWithSchedulerLease(jobCtx, dbw, s.Name(), func() {
		s.runJobLocked(dbw)
	})
}

func (s *DailySummaryScheduler) runJobLocked(dbw *store.DB) {
	now := time.Now()
	metrics, err := daily.CollectDailySummaryMetrics(dbw.DB, now)
	if err != nil {
		slog.Error("daily-summary: failed to collect metrics", "error", err)
		return
	}

	title, message := daily.BuildDailySummaryNotification(metrics)

	_, err = notifypkg.SendNotification(config.RuntimeSafe(), title, message, string(notifypkg.LevelInfo),
		&notifypkg.SendNotificationOptions{
			BypassThrottle: true,
			RequireChannel: true,
			ThrowOnFailure: true,
		},
	)
	if err != nil {
		slog.Error("daily-summary: notification failed", "error", err)
		return
	}
	slog.Info("daily-summary: sent", "title", title)
}
