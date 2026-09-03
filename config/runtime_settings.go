package config

import (
	"sync"
	"sync/atomic"
)

// RuntimeSettings carries every configuration value that may change while
// the server is running: admin runtime-settings updates
// (PUT /api/settings/runtime), auth-token rotation, scheduler schedule
// write-backs, and the runtime database switch.
//
// Concurrency contract (Wave 18 config-race fix, Milestone C1):
//
//   - Readers take the current immutable snapshot via Runtime() /
//     RuntimeSafe() and must treat it as strictly read-only. The pointer
//     load is atomic, so hot paths (proxy auth, admin auth, routing status
//     ranges) never lock and can never observe a torn value.
//   - The ONLY write path is UpdateRuntime, which copies the current
//     snapshot, applies the mutation to the private draft, and atomically
//     publishes the copy. Inside the mutation, slice/map/any fields must be
//     replaced wholesale — never mutated in place — because published
//     snapshots share those references with earlier snapshots.
//   - Everything left on Config is static (frozen at boot before
//     config.Set), so long-lived components may keep reading cached
//     *Config pointers safely.
type RuntimeSettings struct {
	// Auth (4 fields)
	AuthToken  string
	ProxyToken string
	DbType     string
	DbUrl      string
	DbSsl      bool
	// Cron (5 fields)
	CheckinCron          string
	CheckinScheduleMode  string
	CheckinIntervalHours int
	// window mode — random HH:mm inside [start, end]
	// re-rolled per start/setting change (load spreading + anti-fingerprint).
	CheckinWindowStart string
	CheckinWindowEnd   string
	BalanceRefreshCron string
	// Global kill switches for the two always-on periodic jobs that touch
	// upstream accounts (check-in, balance refresh) — issue #1027. Defaults
	// preserve historical behavior (both jobs on); operators turn either job
	// off via env (CHECKIN_ENABLED / BALANCE_REFRESH_ENABLED) or the runtime
	// settings of the same camelCase names. Stored inverted (zero value =
	// enabled) so bare config literals keep the default-on behavior.
	CheckinDisabled        bool
	BalanceRefreshDisabled bool
	ModelSyncCron          string
	LogCleanupCron         string
	// Site & Branding (5 fields) - empty defaults keep the embedded frontend
	// branding and login-page copy unchanged. homePageContent was removed
	// (Wave 8 Lane D): the value was stored but never rendered anywhere.
	SystemName    string
	Logo          string
	Footer        string
	About         string
	ServerAddress string
	// Log Cleanup (4 fields)
	LogCleanupUsageLogsEnabled   bool
	LogCleanupProgramLogsEnabled bool
	LogCleanupRetentionDays      int
	// Notify: Webhook (2 fields)
	WebhookUrl     string
	WebhookEnabled bool
	// Notify: Bark (2 fields)
	BarkUrl     string
	BarkEnabled bool
	// Notify: ServerChan (2 fields)
	ServerChanKey     string
	ServerChanEnabled bool
	// Notify: Telegram (6 fields)
	TelegramEnabled         bool
	TelegramApiBaseUrl      string
	TelegramBotToken        string
	TelegramChatId          string
	TelegramUseSystemProxy  bool
	TelegramMessageThreadId string
	// Notify: SMTP (8 fields)
	SmtpEnabled bool
	SmtpHost    string
	SmtpPort    int
	SmtpSecure  bool
	SmtpUser    string
	SmtpPass    string
	SmtpFrom    string
	SmtpTo      string
	// Notify: Feishu (3 fields) —
	FeishuEnabled bool
	FeishuWebhook string
	FeishuSecret  string
	// Notify: DingTalk (3 fields) —
	DingtalkEnabled bool
	DingtalkWebhook string
	DingtalkSecret  string
	// Notify: WeCom (2 fields) —
	WecomEnabled bool
	WecomWebhook string
	// Notify: Ntfy (4 fields) —
	NtfyEnabled bool
	NtfyUrl     string
	NtfyTopic   string
	NtfyToken   string
	// Notify: General (2 fields)
	NotifyCooldownSec int
	SystemProxyUrl    string
	// ProxyRetryStatusRanges / ProxyDisableStatusRanges carry the
	// operator-tunable status-code range specs (routing.StatusRange policy):
	// retry decides which upstream statuses
	// count as retryable channel faults; disable decides which auto-disable
	// the failing channel. Runtime settings only (settings table +
	// PUT /api/settings/runtime, rehydrated at startup); blank keeps the
	// defaults, which reproduce the historical behavior exactly.
	ProxyRetryStatusRanges   string
	ProxyDisableStatusRanges string
	// NotifyTaskToggles gates per-alert-type notifications.
	// Keys are alert task slugs ("token_expired", "low_balance", "proxy_all_failed").
	// Default nil = all enabled (backward-compatible). When a key is present and
	// false, SendNotification skips that task type so operators can mute, e.g.,
	// low_balance while still receiving token_expired alerts.
	NotifyTaskToggles map[string]bool
	// Admin (3 fields)
	AdminIpAllowlist        []string
	RoutingFallbackUnitCost float64
	// CacheRatioDefault / CacheRatioClaude override the prompt-cache ratio
	// fallbacks used when an upstream pricing row omits cache_ratio. 0/missing =
	// use the code defaults (DefaultCacheRatio=1.0, ClaudeCacheRatio=0.1).
	CacheRatioDefault float64
	CacheRatioClaude  float64
	// ProxyFirstByteTimeoutSec is the operator-facing first-byte / first-token
	// timeout in SECONDS (env PROXY_FIRST_BYTE_TIMEOUT_SEC). Internal dispatch
	// converts to milliseconds via proxy.FirstByteTimeoutMs (sec * 1000).
	// 0 disables observed first-byte timeout.
	ProxyFirstByteTimeoutSec int
	// Proxy: Token Router (2 fields)
	TokenRouterFailureCooldownMaxSec int
	// Proxy: Session (4 fields)
	ProxySessionChannelConcurrencyLimit int
	ProxySessionChannelQueueWaitMs      int
	// Proxy: Misc (7 fields)
	CodexUpstreamWebsocketEnabled              bool
	ResponsesCompactFallbackToResponsesEnabled bool
	DisableCrossProtocolFallback               bool
	ProxyEmptyContentFailEnabled               bool
	ProxyErrorKeywords                         []string
	GlobalBlockedBrands                        []string
	GlobalAllowedModels                        []string
	// Proxy: Debug (9 fields)
	ProxyDebugTraceEnabled        bool
	ProxyDebugCaptureHeaders      bool
	ProxyDebugCaptureBodies       bool
	ProxyDebugCaptureStreamChunks bool
	ProxyDebugTargetSessionId     string
	ProxyDebugTargetClientKind    string
	ProxyDebugTargetModel         string
	ProxyDebugRetentionHours      int
	ProxyDebugMaxBodyBytes        int
	// Routing Weights (5 fields)
	// Model availability probe kill switch (settings toggle hot-applies the
	// running ticker, scheduler/model_probe.go SetEnabled).
	ModelAvailabilityProbeEnabled bool

	RoutingWeights RoutingWeights
	// Payload Rules (2 JSON fields)
	PayloadRules any
}

var (
	runtimePtr atomic.Pointer[RuntimeSettings]
	runtimeMu  sync.Mutex // serializes copy-mutate-publish in UpdateRuntime
)

// SetRuntime publishes the initial runtime snapshot. Composition-root only
// (boot, after DB settings hydration) plus tests. A nil argument clears the
// singleton (test teardown parity with Set(nil)).
func SetRuntime(rt *RuntimeSettings) {
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	if rt == nil {
		runtimePtr.Store(nil)
		return
	}
	cp := *rt
	runtimePtr.Store(&cp)
}

// Runtime returns the current immutable runtime-settings snapshot.
// Panics when called before boot publication — the same contract as Get().
func Runtime() *RuntimeSettings {
	rt := runtimePtr.Load()
	if rt == nil {
		panic("config.Runtime() called before publication — publish runtime settings first")
	}
	return rt
}

// RuntimeSafe returns the current runtime snapshot, or nil before
// publication. Prefer Runtime() on request paths; RuntimeSafe is for
// optional callbacks that may run before (or without) config publication.
func RuntimeSafe() *RuntimeSettings {
	return runtimePtr.Load()
}

// UpdateRuntime is the sole gate for runtime mutation. It copies the
// current snapshot, hands the private draft to mutate, and atomically
// publishes the result. mutate must only reassign fields on the draft it is
// given (wholesale replacement for slices/maps/any) and must not retain the
// draft pointer.
func UpdateRuntime(mutate func(*RuntimeSettings)) {
	if mutate == nil {
		return
	}
	runtimeMu.Lock()
	defer runtimeMu.Unlock()
	cur := runtimePtr.Load()
	var draft RuntimeSettings
	if cur != nil {
		draft = *cur
	}
	mutate(&draft)
	runtimePtr.Store(&draft)
}
