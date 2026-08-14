package proxy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

// TokenRouterInterface is the interface for channel selection.
// This allows the proxy layer to depend on routing without circular imports.
type TokenRouterInterface interface {
	SelectChannel(ctx context.Context, requestedModel string, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error)
	SelectNextChannel(ctx context.Context, requestedModel string, excludeChannelIDs []int64, policy routing.DownstreamRoutingPolicy) (*routing.SelectedChannel, error)
	SelectPreferredChannel(ctx context.Context, requestedModel string, preferredChannelID int64, policy routing.DownstreamRoutingPolicy, excludeChannelIDs []int64) (*routing.SelectedChannel, error)
	RecordSuccess(ctx context.Context, channelID int64, latencyMs float64, cost float64, modelName *string, actualAccountID *int64) error
	RecordFailure(ctx context.Context, channelID int64, failureCtx routing.SiteRuntimeFailureContext, actualAccountID *int64) error
}

// RouteRefreshWorkflow is the interface for route refresh.
type RouteRefreshWorkflow interface {
	RefreshModelsAndRebuildRoutes(ctx context.Context) error
}

// ChannelSelectionInput is the input for SelectProxyChannelForAttempt.
type ChannelSelectionInput struct {
	RequestedModel   string
	DownstreamPolicy routing.DownstreamRoutingPolicy
	// ExcludeChannelIDs is a request-local list of already-tried channel IDs.
	// Scope is channel-only: callers must not expand this to all channels of a
	// site. Same-site siblings stay eligible unless routing policy (cooldown /
	// site breaker / credential-scoped usage-limit) independently filters them.
	//
	ExcludeChannelIDs []int64
	RetryCount        int
	ForcedChannelID   *int64
}

// Tester header constants.
const (
	TesterRequestHeader     = "x-metapi-tester-request"
	TesterForcedChannelHeader = "x-metapi-tester-forced-channel-id"
)

// ---- Tester Helpers ----

// IsLoopbackClientIP checks if a client IP is loopback.
func IsLoopbackClientIP(ip string) bool {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return false
	}
	if trimmed == "::1" || trimmed == "127.0.0.1" {
		return true
	}
	if strings.HasPrefix(trimmed, "::ffff:") {
		return strings.TrimPrefix(trimmed, "::ffff:") == "127.0.0.1"
	}
	return false
}

// IsTrustedTesterRequest checks if the request is from a trusted tester (loopback + header).
func IsTrustedTesterRequest(headers map[string]string, clientIP string) bool {
	if !IsLoopbackClientIP(clientIP) {
		return false
	}
	return headerValueEquals(headers, TesterRequestHeader, "1")
}

// GetTesterForcedChannelID extracts the forced channel ID from tester headers.
func GetTesterForcedChannelID(headers map[string]string, clientIP string) *int64 {
	if !IsTrustedTesterRequest(headers, clientIP) {
		return nil
	}
	for k, v := range headers {
		if strings.ToLower(strings.TrimSpace(k)) == TesterForcedChannelHeader {
			return normalizeForcedChannelID(v)
		}
	}
	return nil
}

func headerValueEquals(headers map[string]string, key, expectedValue string) bool {
	lowerKey := strings.TrimSpace(strings.ToLower(key))
	expected := strings.TrimSpace(strings.ToLower(expectedValue))
	for k, v := range headers {
		if strings.ToLower(strings.TrimSpace(k)) == lowerKey {
			if strings.TrimSpace(strings.ToLower(v)) == expected {
				return true
			}
		}
	}
	return false
}

func normalizeForcedChannelID(value string) *int64 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	n, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || n <= 0 {
		return nil
	}
	return &n
}

// BuildForcedChannelUnavailableMessage builds a human-readable message for forced channel unavailability.
func BuildForcedChannelUnavailableMessage(forcedChannelID *int64) string {
	if forcedChannelID == nil || *forcedChannelID <= 0 {
		return "No available channels for this model"
	}
	return fmt.Sprintf("指定通道 #%d 当前不可用，固定通道模式不会自动切换其他通道", *forcedChannelID)
}

// CanRetryChannelSelection checks if a channel retry is possible.
// Returns false if a forced channel is set (no fallback), or if max retries reached.
func CanRetryChannelSelection(retryCount int, maxRetries int, forcedChannelID *int64) bool {
	if forcedChannelID != nil && *forcedChannelID > 0 {
		return false
	}
	return retryCount < maxRetries
}

// ---- Channel Selection ----

// SelectProxyChannelForAttempt selects a channel for the current attempt.
// Paths: tester forced -> normal selection (first attempt may retry selection
// once when the store returned no channels).
//
// routeRefresher is retained in the signature for handler wiring, but is never
// populated in production (RouteRefreshWorkflow has no wired implementer), so
// no refresh step runs here.
func SelectProxyChannelForAttempt(
	ctx context.Context,
	router TokenRouterInterface,
	coord *ProxyChannelCoordinator,
	routeRefresher RouteRefreshWorkflow,
	input ChannelSelectionInput,
) (*routing.SelectedChannel, error) {
	// Tester forced channel
	if input.ForcedChannelID != nil && *input.ForcedChannelID > 0 {
		if input.RetryCount > 0 {
			return nil, nil
		}
		return router.SelectPreferredChannel(
			ctx,
			input.RequestedModel,
			*input.ForcedChannelID,
			input.DownstreamPolicy,
			input.ExcludeChannelIDs,
		)
	}

	var selected *routing.SelectedChannel
	var err error

	// Normal selection
	if input.RetryCount == 0 {
		selected, err = router.SelectChannel(ctx, input.RequestedModel, input.DownstreamPolicy)
	} else {
		selected, err = router.SelectNextChannel(
			ctx,
			input.RequestedModel,
			input.ExcludeChannelIDs,
			input.DownstreamPolicy,
		)
	}

	// Retry selection once on first attempt when nothing was selected.
	if selected == nil && input.RetryCount == 0 {
		selected, err = router.SelectChannel(ctx, input.RequestedModel, input.DownstreamPolicy)
	}

	return selected, err
}
