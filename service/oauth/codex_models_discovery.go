package oauth

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Codex model discovery / allowlist helpers for OAuth accounts.
//
// Upstream TS used a fixed 12s discovery timeout (modelService.ts) and fixtures
// that only exercised gpt-5.4. Cold-start Codex OAuth discovery often needs a
// longer first attempt plus one soft retry, and allowlists must accept the
// gpt-5.x family (including gpt-5.5) without hardcoding only obsolete models.

const (
	// CodexModelDiscoveryTimeout is the first-attempt budget for Codex cloud
	// model discovery. Raised from the historical 12s upstream default so cold
	// starts (proxy + ChatGPT backend) do not fail spuriously.
	CodexModelDiscoveryTimeout = 30 * time.Second

	// CodexModelDiscoveryRetryTimeout is the budget for the soft-retry attempt
	// after a timeout. Slightly longer than the first attempt.
	CodexModelDiscoveryRetryTimeout = 45 * time.Second

	// CodexModelDiscoveryBackoff is the pause between first timeout and retry.
	CodexModelDiscoveryBackoff = 750 * time.Millisecond

	// CodexModelDiscoveryMaxAttempts is 1 initial + 1 soft retry on timeout.
	CodexModelDiscoveryMaxAttempts = 2
)

// CodexModelDiscoveryErrorCode classifies discovery failures for operators.
type CodexModelDiscoveryErrorCode string

const (
	CodexDiscoveryTimeout      CodexModelDiscoveryErrorCode = "timeout"
	CodexDiscoveryUnauthorized CodexModelDiscoveryErrorCode = "unauthorized"
	CodexDiscoveryEmptyModels  CodexModelDiscoveryErrorCode = "empty_models"
	CodexDiscoveryUnknown      CodexModelDiscoveryErrorCode = "unknown"
)

// CodexModelDiscoveryResult is the outcome of a discovery pass.
type CodexModelDiscoveryResult struct {
	Models       []string                     `json:"models"`
	Status       OauthModelDiscoveryStatus    `json:"status"`
	ErrorCode    CodexModelDiscoveryErrorCode `json:"errorCode,omitempty"`
	ErrorMessage string                       `json:"errorMessage,omitempty"`
	Attempts     int                          `json:"attempts"`
	TimedOut     bool                         `json:"timedOut,omitempty"`
	DurationMs   int64                        `json:"durationMs"`
}

// ClassifyCodexModelDiscoveryError maps error text to a stable error code.
func ClassifyCodexModelDiscoveryError(err error) CodexModelDiscoveryErrorCode {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "timed out"),
		strings.Contains(msg, "deadline exceeded"),
		strings.Contains(msg, "context deadline"),
		strings.Contains(msg, "请求超时"):
		return CodexDiscoveryTimeout
	case strings.Contains(msg, "http 401"),
		strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "unauthenticated"):
		return CodexDiscoveryUnauthorized
	case strings.Contains(msg, "未获取到可用模型"),
		strings.Contains(msg, "empty"),
		strings.Contains(msg, "no models"):
		return CodexDiscoveryEmptyModels
	default:
		return CodexDiscoveryUnknown
	}
}

// FormatCodexModelDiscoveryTimeoutStatus returns an operator-facing message
// when first/retry discovery times out.
func FormatCodexModelDiscoveryTimeoutStatus(timeout time.Duration, attempts int) string {
	sec := int(timeout.Round(time.Second) / time.Second)
	if sec <= 0 {
		sec = int(CodexModelDiscoveryTimeout / time.Second)
	}
	return fmt.Sprintf(
		"Codex model discovery timed out after %d attempt(s) (budget %ds). "+
			"Cold-start ChatGPT Codex backends often need >12s; ensure account proxy is reachable, "+
			"retry connection refresh, or raise the discovery budget (default first attempt %ds, soft-retry %ds). "+
			"Status=abnormal until a successful discovery writes healthy + lastDiscoveredModels.",
		attempts,
		sec,
		int(CodexModelDiscoveryTimeout/time.Second),
		int(CodexModelDiscoveryRetryTimeout/time.Second),
	)
}

// DiscoverCodexModelsWithSoftRetry runs cloud discovery with an increased first
// timeout and one soft-retry/backoff on timeout. Non-timeout errors fail fast.
func DiscoverCodexModelsWithSoftRetry(
	ctx context.Context,
	input CodexModelDiscoveryInput,
	fetch func(context.Context, CodexModelDiscoveryInput) ([]string, error),
) *CodexModelDiscoveryResult {
	if fetch == nil {
		fetch = DiscoverCodexModelsFromCloud
	}
	now := input.Now
	if now == nil {
		now = time.Now
	}
	sleep := input.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		}
	}

	started := now()
	timeouts := []time.Duration{CodexModelDiscoveryTimeout, CodexModelDiscoveryRetryTimeout}
	var lastErr error
	attempts := 0
	timedOut := false

	for i, budget := range timeouts {
		if i >= CodexModelDiscoveryMaxAttempts {
			break
		}
		if err := ctx.Err(); err != nil {
			lastErr = err
			break
		}
		attempts++
		attemptCtx, cancel := context.WithTimeout(ctx, budget)
		models, err := fetch(attemptCtx, input)
		cancel()
		if err == nil {
			normalized := NormalizeDiscoveredCodexModels(models)
			// Ensure gpt-5.5 family is not dropped by partial upstream lists when
			// seed merge is desired by callers — discovery itself returns live set.
			return &CodexModelDiscoveryResult{
				Models:     normalized,
				Status:     OauthModelDiscoveryHealthy,
				Attempts:   attempts,
				DurationMs: now().Sub(started).Milliseconds(),
			}
		}
		lastErr = err
		code := ClassifyCodexModelDiscoveryError(err)
		if code != CodexDiscoveryTimeout {
			// Soft-retry only for timeouts (cold start). Auth/empty fail fast.
			msg := formatCodexDiscoveryFailure(code, err, attempts, budget, false)
			return &CodexModelDiscoveryResult{
				Models:       nil,
				Status:       OauthModelDiscoveryAbnormal,
				ErrorCode:    code,
				ErrorMessage: msg,
				Attempts:     attempts,
				TimedOut:     false,
				DurationMs:   now().Sub(started).Milliseconds(),
			}
		}
		timedOut = true
		// Soft-retry once after backoff when another attempt remains.
		if i+1 < len(timeouts) {
			if err := sleep(ctx, CodexModelDiscoveryBackoff); err != nil {
				lastErr = err
				break
			}
			continue
		}
	}

	code := ClassifyCodexModelDiscoveryError(lastErr)
	if code == "" {
		code = CodexDiscoveryUnknown
	}
	budget := CodexModelDiscoveryRetryTimeout
	if attempts == 1 {
		budget = CodexModelDiscoveryTimeout
	}
	msg := formatCodexDiscoveryFailure(code, lastErr, attempts, budget, timedOut)
	return &CodexModelDiscoveryResult{
		Models:       nil,
		Status:       OauthModelDiscoveryAbnormal,
		ErrorCode:    code,
		ErrorMessage: msg,
		Attempts:     attempts,
		TimedOut:     timedOut || code == CodexDiscoveryTimeout,
		DurationMs:   now().Sub(started).Milliseconds(),
	}
}

func formatCodexDiscoveryFailure(
	code CodexModelDiscoveryErrorCode,
	err error,
	attempts int,
	budget time.Duration,
	timedOut bool,
) string {
	if timedOut || code == CodexDiscoveryTimeout {
		return FormatCodexModelDiscoveryTimeoutStatus(budget, attempts)
	}
	raw := "codex model discovery failed"
	if err != nil {
		raw = err.Error()
	}
	return fmt.Sprintf("codex model discovery failed (%s)", raw)
}

// BuildOauthModelDiscoveryPatch builds the OauthInfo patch used to persist
// discovery status after a successful or failed pass.
func BuildOauthModelDiscoveryPatch(result *CodexModelDiscoveryResult, checkedAt string) *OauthInfo {
	if result == nil {
		return nil
	}
	if checkedAt == "" {
		checkedAt = time.Now().UTC().Format(time.RFC3339)
	}
	patch := &OauthInfo{
		ModelDiscoveryStatus: result.Status,
		LastModelSyncAt:      checkedAt,
		LastDiscoveredModels: result.Models,
	}
	if result.Status == OauthModelDiscoveryAbnormal {
		patch.LastModelSyncError = result.ErrorMessage
	} else {
		// Clear previous error on success by writing empty — callers merge carefully.
		patch.LastModelSyncError = ""
	}
	return patch
}
