package notify

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
)

// NotificationLevel is the severity of a notification.
type NotificationLevel string

const (
	LevelInfo    NotificationLevel = "info"
	LevelWarning NotificationLevel = "warning"
	LevelError   NotificationLevel = "error"
)

// NotificationChannel is the channel identifier.
type NotificationChannel string

const (
	ChannelWebhook   NotificationChannel = "webhook"
	ChannelBark      NotificationChannel = "bark"
	ChannelServerChan NotificationChannel = "serverchan"
	ChannelTelegram  NotificationChannel = "telegram"
	ChannelSMTP      NotificationChannel = "smtp"
	ChannelFeishu    NotificationChannel = "feishu"
	ChannelDingTalk  NotificationChannel = "dingtalk"
	ChannelWeCom     NotificationChannel = "wecom"
	ChannelNtfy      NotificationChannel = "ntfy"
)

// Channel is the interface for notification channels.
type Channel interface {
	Name() string
	Send(cfg *config.Config, title, message, level, timeFootnote string) error
}

// SendNotificationOptions configures notification behavior.
type SendNotificationOptions struct {
	BypassThrottle bool
	RequireChannel bool
	ThrowOnFailure bool
	// TaskTag labels the alert type. When set and
	// cfg.NotifyTaskToggles[TaskTag] is false, the notification is skipped so
	// operators can mute a specific alert type (e.g. "low_balance") while
	// still receiving others. Empty TaskTag = no gating (backward-compatible).
	TaskTag string
}

// DispatchResult is the result of a notification dispatch.
type DispatchResult struct {
	Throttled      bool
	MergedCount    int // suppressed duplicates merged into the sent notification
	Attempted      int
	Succeeded      int
	Failed         int
	FailedChannels []NotificationChannel
}

// All notification channels
var channels = []Channel{
	&WebhookChannel{},
	&BarkChannel{},
	&ServerChanChannel{},
	&TelegramChannel{},
	&SMTPChannel{},
	&FeishuChannel{},
	&DingtalkChannel{},
	&WecomChannel{},
	&NtfyChannel{},
}

// SendNotification dispatches a notification through all configured channels.
// Mirrors TS sendNotification().
func SendNotification(cfg *config.Config, title, message, level string, options *SendNotificationOptions) (*DispatchResult, error) {
	if options == nil {
		options = &SendNotificationOptions{}
	}

	// per-task mute gate. Empty TaskTag = no gate.
	if options.TaskTag != "" {
		if enabled, ok := cfg.NotifyTaskToggles[options.TaskTag]; ok && !enabled {
			return &DispatchResult{Throttled: false, Attempted: 0, Succeeded: 0, Failed: 0}, nil
		}
	}

	now := time.Now()
	timeFootnote := service.BuildTimeFootnote(now)
	cooldownMs := int64(cfg.NotifyCooldownSec) * 1000
	if cooldownMs < 0 {
		cooldownMs = 0
	}

	resolvedMessage := message

	// Throttle check
	if !options.BypassThrottle && cooldownMs > 0 {
		nowMs := time.Now().UnixMilli()
		staleMs := cooldownMs * 6
		if staleMs < 600_000 {
			staleMs = 600_000
		}
		GlobalThrottle.PruneNotificationThrottleState(nowMs, staleMs)

		signature := CreateNotificationSignature(title, message, level)
		if options.TaskTag != "" {
			// Aggregate by task type + level instead of the full message:
			// a burst of per-account failures (same task, different message
			// text) collapses into one notification with a merge count
			// instead of one notification per account (anti-spam).
			signature = "tag:" + options.TaskTag + ":" + level
		}
		decision := GlobalThrottle.EvaluateNotificationThrottle(signature, nowMs, cooldownMs)
		if !decision.ShouldSend {
			return &DispatchResult{
				Throttled:      true,
				MergedCount:    decision.MergedCount,
				Attempted:      0,
				Succeeded:      0,
				Failed:         0,
				FailedChannels: nil,
			}, nil
		}
		if decision.MergedCount > 0 {
			resolvedMessage = fmt.Sprintf("%s\n\n[通知合并] 冷静期内已合并 %d 条重复告警", message, decision.MergedCount)
		}
	}

	// Build task list
	type task struct {
		channel NotificationChannel
		run     func() error
	}

	var tasks []task

	if cfg.WebhookEnabled && cfg.WebhookUrl != "" {
		wh := &WebhookChannel{}
		tasks = append(tasks, task{
			channel: ChannelWebhook,
			run:     func() error { return wh.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	if cfg.BarkEnabled && cfg.BarkUrl != "" {
		bk := &BarkChannel{}
		tasks = append(tasks, task{
			channel: ChannelBark,
			run:     func() error { return bk.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	if cfg.ServerChanEnabled && cfg.ServerChanKey != "" {
		sc := &ServerChanChannel{}
		tasks = append(tasks, task{
			channel: ChannelServerChan,
			run:     func() error { return sc.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	if cfg.TelegramEnabled && cfg.TelegramBotToken != "" && cfg.TelegramChatId != "" {
		tg := &TelegramChannel{}
		tasks = append(tasks, task{
			channel: ChannelTelegram,
			run:     func() error { return tg.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	if cfg.SmtpEnabled && cfg.SmtpHost != "" && cfg.SmtpPort > 0 && cfg.SmtpFrom != "" && cfg.SmtpTo != "" {
		smtpCh := &SMTPChannel{}
		tasks = append(tasks, task{
			channel: ChannelSMTP,
			run:     func() error { return smtpCh.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	// Feishu / DingTalk / WeCom / Ntfy dedicated channels.
	if cfg.FeishuEnabled && cfg.FeishuWebhook != "" {
		fc := &FeishuChannel{}
		tasks = append(tasks, task{
			channel: ChannelFeishu,
			run:     func() error { return fc.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}
	if cfg.DingtalkEnabled && cfg.DingtalkWebhook != "" {
		dc := &DingtalkChannel{}
		tasks = append(tasks, task{
			channel: ChannelDingTalk,
			run:     func() error { return dc.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}
	if cfg.WecomEnabled && cfg.WecomWebhook != "" {
		wc := &WecomChannel{}
		tasks = append(tasks, task{
			channel: ChannelWeCom,
			run:     func() error { return wc.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}
	if cfg.NtfyEnabled && cfg.NtfyUrl != "" && cfg.NtfyTopic != "" {
		nc := &NtfyChannel{}
		tasks = append(tasks, task{
			channel: ChannelNtfy,
			run:     func() error { return nc.Send(cfg, title, resolvedMessage, level, timeFootnote) },
		})
	}

	// No channels configured
	if len(tasks) == 0 {
		err := fmt.Errorf("no notification channels configured")
		if options.RequireChannel || options.ThrowOnFailure {
			slog.Error("SendNotification: " + err.Error())
			return nil, err
		}
		return &DispatchResult{
			Throttled:      false,
			Attempted:      0,
			Succeeded:      0,
			Failed:         0,
			FailedChannels: nil,
		}, nil
	}

	// Parallel dispatch
	type taskResult struct {
		channel NotificationChannel
		ok      bool
		err     error
	}

	resultCh := make(chan taskResult, len(tasks))
	var wg sync.WaitGroup

	for _, t := range tasks {
		wg.Add(1)
		go func(t task) {
			defer wg.Done()
			err := t.run()
			resultCh <- taskResult{channel: t.channel, ok: err == nil, err: err}
		}(t)
	}
	wg.Wait()
	close(resultCh)

	// Aggregate results
	var failedResults []taskResult
	succeeded := 0
	for r := range resultCh {
		if r.ok {
			succeeded++
		} else {
			failedResults = append(failedResults, r)
		}
	}

	failedChannels := make([]NotificationChannel, 0, len(failedResults))
	for _, fr := range failedResults {
		failedChannels = append(failedChannels, fr.channel)
	}

	// Observability (2026-08-01): every dispatch leaves a log line so
	// production can answer "did the notification go out, and why not"
	// without guessing — an earlier deployment had 3 channels enabled
	// with empty credentials and zero log trace of the silent failures.
	if len(tasks) > 0 {
		attrs := []any{
			"title", title,
			"level", level,
			"attempted", len(tasks),
			"succeeded", succeeded,
			"failed", len(failedResults),
		}
		for _, fr := range failedResults {
			attrs = append(attrs, "channel_"+string(fr.channel), truncateErr(fr.err, 100))
		}
		if len(failedResults) > 0 {
			slog.Warn("notify: dispatch partial/failed", attrs...)
		} else {
			slog.Info("notify: dispatch ok", attrs...)
		}
	}

	if options.ThrowOnFailure && succeeded == 0 && len(failedResults) > 0 {
		firstErr := failedResults[0].err
		slog.Error("SendNotification: all channels failed",
			"first_error", firstErr)
		return nil, firstErr
	}

	return &DispatchResult{
		Throttled:      false,
		Attempted:      len(tasks),
		Succeeded:      succeeded,
		Failed:         len(failedResults),
		FailedChannels: failedChannels,
	}, nil
}

// truncateErr bounds an error message for logging — channel errors may embed
// URLs responses, so cap length (never log credentials).
func truncateErr(err error, max int) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
