package proxyhandler

import (
	"context"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
)

func observedUsageCount(found bool, count int64) *int64 {
	if !found {
		return nil
	}
	return int64Ptr(count)
}

func int64Ptr(v int64) *int64 { return &v }

func boolPtr(v bool) *bool { return &v }

func newProxyAttemptLogEntry(
	ctx context.Context,
	selected *routing.SelectedChannel,
	proxyCtx *Ctx,
	upstreamModel, upstreamPath string,
	latencyMs int64,
	firstByteLatencyMs *int64,
	httpStatus int,
	isStream bool,
	usage ParsedUsage,
	retryCount int,
	requestID string,
) proxy.ProxyLogEntry {
	if requestID == "" {
		requestID = proxy.RequestIDFromContext(ctx)
	}
	requestedModel := ""
	var keyID *int64
	clientFamily, clientAppID, clientAppName, clientConfidence := "", "", "", ""
	if proxyCtx != nil {
		requestedModel = proxyCtx.RequestedModel
		if proxyCtx.Auth != nil {
			keyID = proxyCtx.Auth.KeyID
		}
		clientFamily = proxyCtx.ClientCtx.ClientKind
		clientAppID = proxyCtx.ClientCtx.ClientAppID
		clientAppName = proxyCtx.ClientCtx.ClientAppName
		clientConfidence = proxyCtx.ClientCtx.ClientConfidence
	}
	if requestedModel == "" {
		requestedModel = upstreamModel
	}
	modelActual := upstreamModel
	routeID := selected.Channel.RouteID
	var routeIDPtr *int64
	if routeID != 0 {
		routeIDPtr = &routeID
	}
	channelID := selected.Channel.ID
	var channelIDPtr, upstreamChannelIDPtr, upstreamGrantIDPtr *int64
	if selected.Direct == nil {
		channelIDPtr = &channelID
	} else {
		upstreamChannelIDPtr = &selected.Direct.ChannelID
		upstreamGrantIDPtr = &selected.Direct.GrantID
	}
	accountID := selected.Account.ID
	var accountIDPtr *int64
	if selected.Direct == nil {
		accountIDPtr = &accountID
	}
	source := usage.Source
	if source == "" {
		if usage.Found {
			source = usageSourceUpstream
		} else {
			source = usageSourceUnknown
		}
	}
	return proxy.ProxyLogEntry{
		DownstreamPath:        proxyLogDownstreamPath(proxyCtx),
		RouteID:               routeIDPtr,
		ChannelID:             channelIDPtr,
		UpstreamChannelID:     upstreamChannelIDPtr,
		UpstreamGrantID:       upstreamGrantIDPtr,
		AccountID:             accountIDPtr,
		DownstreamAPIKeyID:    keyID,
		ModelRequested:        requestedModel,
		ModelActual:           &modelActual,
		HTTPStatus:            httpStatus,
		IsStream:              boolPtr(isStream),
		FirstByteLatencyMs:    firstByteLatencyMs,
		FirstOutputLatencyMs:  usage.FirstOutputLatencyMs,
		UpstreamReportedModel: usage.UpstreamReportedModel,
		LatencyMs:             latencyMs,
		PromptTokens:          observedUsageCount(usage.Found, usage.PromptTokens),
		CompletionTokens:      observedUsageCount(usage.Found, usage.CompletionTokens),
		TotalTokens:           observedUsageCount(usage.Found, usage.TotalTokens),
		ClientFamily:          clientFamily,
		ClientAppID:           clientAppID,
		ClientAppName:         clientAppName,
		ClientConfidence:      clientConfidence,
		RetryCount:            retryCount,
		RequestID:             requestID,
		UpstreamPath:          &upstreamPath,
		UsageSource:           source,
	}
}

func writeSuccessProxyLog(
	ctx context.Context,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	proxyCtx *Ctx,
	upstreamModel string,
	upstreamPath string,
	latencyMs int64,
	firstByteLatencyMs *int64,
	httpStatus int,
	isStream bool,
	usage ParsedUsage,
	retryCount int,
	requestID string,
) {
	if cfg == nil || selected == nil {
		return
	}
	usage = applyDirectVideoAccounting(proxyCtx, usage)
	rememberSuccessfulRouting(cfg, selected)
	recordChannelRateUsage(ctx, usage)
	requestLifecycleFromContext(ctx).observeUserBudgetUsageValidity(usage)
	entry := newProxyAttemptLogEntry(ctx, selected, proxyCtx, upstreamModel, upstreamPath,
		latencyMs, firstByteLatencyMs, httpStatus, isStream, usage, retryCount, requestID)
	// Billing attribution uses the requested (canonical) name so a
	// redirect/rewrite to the upstream actual name never changes cost
	// accounting — ratio lookups stay on the canonical model.
	billing := directVideoBillingResult(ctx, entry.ModelRequested, selected, usage)
	entry.Status = "success"
	entry.EstimatedCost = billing.EstimatedCost
	entry.BillingDetails = billing.BillingDetails
	entry.StreamTiming = attemptTimingSnapshot(ctx, isStream, usage, false)
	logProxy(directVideoLogContext(ctx, proxyCtx), cfg, entry)
	// Advance managed-key used_cost so max_cost can gate subsequent traffic.
	// Stream + non-stream both sink here once; helper no-ops zero/NaN/Inf.
	// Failure paths intentionally do not call this (known limitation stays).
	if usage.videoAccounting == nil {
		recordManagedKeyCostOnSuccess(proxyScopeFromContext(ctx).db, entry.DownstreamAPIKeyID, billing.EstimatedCost)
	}
}

// writeFailureProxyLog persists a failed attempt into proxy_logs so stats /
// usage aggregation do not silently under-count tokens when upstream still
// reported usage on error or content-detected failure paths.
// Matches SurfaceFailureToolkit status="failed" semantics.
// Does not invent tokens: zeros + usage_source=unknown when usage.Found is false.
func writeFailureProxyLog(
	ctx context.Context,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	proxyCtx *Ctx,
	upstreamModel string,
	upstreamPath string,
	latencyMs int64,
	firstByteLatencyMs *int64,
	httpStatus int,
	isStream bool,
	usage ParsedUsage,
	retryCount int,
	requestID string,
	errText string,
) {
	if cfg == nil || selected == nil {
		return
	}
	forgetFailedRouting(cfg, selected)
	usage = applyDirectVideoAccounting(proxyCtx, usage)
	recordChannelRateUsage(ctx, usage)
	requestLifecycleFromContext(ctx).observeUserBudgetUsageValidity(usage)
	entry := newProxyAttemptLogEntry(ctx, selected, proxyCtx, upstreamModel, upstreamPath,
		latencyMs, firstByteLatencyMs, httpStatus, isStream, usage, retryCount, requestID)
	// Only attach cost when usage was found; avoid inventing spend on pure
	// network/timeout failures with zero tokens. Attribution name, not the
	// upstream rewritten name.
	entry.Status = "failed"
	if usage.Found || len(usage.ToolUsage) > 0 || usage.videoAccounting != nil {
		billing := directVideoBillingResult(ctx, entry.ModelRequested, selected, usage)
		entry.EstimatedCost = billing.EstimatedCost
		entry.BillingDetails = billing.BillingDetails
	}
	entry.ErrorMessage = strPtrOrEmpty(strings.TrimSpace(errText))
	entry.StreamTiming = attemptTimingSnapshot(ctx, isStream, usage, true)
	logProxy(directVideoLogContext(ctx, proxyCtx), cfg, entry)
}
