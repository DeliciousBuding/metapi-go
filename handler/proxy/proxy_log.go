package proxyhandler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// defaultLogProxyWriter inserts a proxy.ProxyLogEntry into proxy_logs.
// Prefer injecting via UpstreamConfig.LogProxy so tests can capture writes.
func defaultLogProxyWriter(ctx context.Context, entry proxy.ProxyLogEntry) error {
	db := store.GetDB()
	if db == nil {
		return nil
	}
	return InsertProxyLog(ctx, db, entry)
}

// InsertProxyLog writes one proxy_logs row. Columns match store.ProxyLog / DDL.
// Fields present only on ProxyLogEntry (usage_source, upstream_path) are not
// persisted until the schema grows those columns; they remain on the entry for
// in-process consumers and tests.
func InsertProxyLog(ctx context.Context, db *store.DB, entry proxy.ProxyLogEntry) error {
	if db == nil {
		return nil
	}
	createdAt := time.Now().UTC().Format(time.RFC3339)
	args := proxyLogEntryArgs(entry, createdAt)
	query := "INSERT INTO proxy_logs (" + proxyLogInsertColumns + ") VALUES " + proxyLogSingleRowPlaceholders
	if ctx != nil {
		_, err := db.ExecContext(ctx, query, args...)
		return err
	}
	_, err := db.Exec(query, args...)
	return err
}

// proxyLogInsertColumns is the canonical column list for a proxy_logs INSERT.
// Order MUST match proxyLogEntryArgs. Shared by the single-row InsertProxyLog
// path and the multi-row batch writer so the two never drift.
const proxyLogInsertColumns = `route_id, channel_id, account_id, downstream_api_key_id,
			model_requested, model_actual, status, http_status, is_stream,
			first_byte_latency_ms, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			estimated_cost, billing_details,
			client_family, client_app_id, client_app_name, client_confidence,
			error_message, retry_count, request_id, created_at`

// proxyLogSingleRowPlaceholders is the 24 "?" placeholders for one row.
// store.DB rebinds ? to $N for PostgreSQL, so the batch writer builds rows of
// the same placeholder shape and lets the rebind do the dialect work.
const proxyLogSingleRowPlaceholders = `(?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?,
			?, ?, ?,
			?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?)`

// proxyLogEntryArgs builds the positional args for one proxy_logs INSERT row,
// matching proxyLogInsertColumns order. Shared by InsertProxyLog (single) and
// batchInsertProxyLogs (multi-row). createdAt is injected by the caller so the
// batch writer can stamp a whole batch with one timestamp while the single-row
// path keeps its per-call time.Now().
func proxyLogEntryArgs(entry proxy.ProxyLogEntry, createdAt string) []any {
	var billingDetails any
	if entry.BillingDetails != nil {
		switch v := entry.BillingDetails.(type) {
		case string:
			billingDetails = v
		default:
			if b, err := json.Marshal(v); err == nil {
				billingDetails = string(b)
			}
		}
	}
	return []any{
		nullInt64(entry.RouteID),
		nullInt64(entry.ChannelID),
		nullInt64(entry.AccountID),
		nullInt64(entry.DownstreamAPIKeyID),
		nullString(strPtrOrEmpty(entry.ModelRequested)),
		nullString(entry.ModelActual),
		nullString(strPtrOrEmpty(entry.Status)),
		entry.HTTPStatus,
		nullBool(entry.IsStream),
		nullInt64(entry.FirstByteLatencyMs),
		entry.LatencyMs,
		nullInt64(entry.PromptTokens),
		nullInt64(entry.CompletionTokens),
		nullInt64(entry.TotalTokens),
		entry.EstimatedCost,
		billingDetails,
		nullString(strPtrOrEmpty(entry.ClientFamily)),
		nullString(strPtrOrEmpty(entry.ClientAppID)),
		nullString(strPtrOrEmpty(entry.ClientAppName)),
		nullString(strPtrOrEmpty(entry.ClientConfidence)),
		nullString(entry.ErrorMessage),
		entry.RetryCount,
		nullString(strPtrOrEmpty(entry.RequestID)),
		createdAt,
	}
}

func logProxy(ctx context.Context, cfg *UpstreamConfig, entry proxy.ProxyLogEntry) {
	if cfg == nil {
		return
	}
	// Prefer explicit entry.RequestID; otherwise inherit chi/MetAPI request id.
	if entry.RequestID == "" {
		entry.RequestID = proxy.RequestIDFromContext(ctx)
	}
	writer := cfg.LogProxy
	if writer == nil {
		writer = defaultLogProxyWriter
	}
	if err := writer(ctx, entry); err != nil {
		slog.Warn("LogProxy failed",
			"err", err,
			"status", entry.Status,
			"model", entry.ModelRequested,
			"request_id", entry.RequestID,
			"retry_count", entry.RetryCount,
		)
	}
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullBool(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullString(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func strPtrOrEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func int64Ptr(v int64) *int64 { return &v }

func boolPtr(v bool) *bool { return &v }

func writeSuccessProxyLog(
	ctx context.Context,
	cfg *UpstreamConfig,
	selected *routing.SelectedChannel,
	proxyCtx *Ctx,
	upstreamModel string,
	upstreamPath string,
	latencyMs int64,
	httpStatus int,
	isStream bool,
	usage ParsedUsage,
	retryCount int,
	requestID string,
) {
	if cfg == nil || selected == nil {
		return
	}
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
	accountID := selected.Account.ID
	source := usage.Source
	if source == "" {
		if usage.Found {
			source = usageSourceUpstream
		} else {
			source = usageSourceUnknown
		}
	}
	platformName := ""
	if selected.Site.Platform != "" {
		platformName = selected.Site.Platform
	}
	// K1b: billing attribution uses the requested (canonical) name so a
	// redirect/rewrite to the upstream actual name never changes cost
	// accounting — ratio lookups stay on the canonical model.
	billing := EstimateBillingCostFromUsage(requestedModel, platformName, usage)
	entry := proxy.ProxyLogEntry{
		RouteID:            routeIDPtr,
		ChannelID:          &channelID,
		AccountID:          &accountID,
		DownstreamAPIKeyID: keyID,
		ModelRequested:     requestedModel,
		ModelActual:        &modelActual,
		Status:             "success",
		HTTPStatus:         httpStatus,
		IsStream:           boolPtr(isStream),
		FirstByteLatencyMs: int64Ptr(latencyMs),
		LatencyMs:          latencyMs,
		PromptTokens:       int64Ptr(usage.PromptTokens),
		CompletionTokens:   int64Ptr(usage.CompletionTokens),
		TotalTokens:        int64Ptr(usage.TotalTokens),
		EstimatedCost:      billing.EstimatedCost,
		BillingDetails:     billing.BillingDetails,
		ClientFamily:       clientFamily,
		ClientAppID:        clientAppID,
		ClientAppName:      clientAppName,
		ClientConfidence:   clientConfidence,
		RetryCount:         retryCount,
		RequestID:          requestID,
		UpstreamPath:       &upstreamPath,
		UsageSource:        source,
	}
	logProxy(ctx, cfg, entry)
	// Advance managed-key used_cost so max_cost can gate subsequent traffic.
	// Stream + non-stream both sink here once; helper no-ops zero/NaN/Inf.
	// Failure paths intentionally do not call this (known limitation stays).
	recordManagedKeyCostOnSuccess(keyID, billing.EstimatedCost)
}

// recordManagedKeyCostOnSuccess increments used_cost for managed keys after a
// successful proxy attempt. Nil KeyID (global token) is a no-op; zero/NaN/Inf
// costs are skipped by auth.RecordManagedKeyCostUsage itself.
func recordManagedKeyCostOnSuccess(keyID *int64, estimatedCost float64) {
	if keyID == nil {
		return
	}
	auth.RecordManagedKeyCostUsage(*keyID, estimatedCost)
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
	accountID := selected.Account.ID
	source := usage.Source
	if source == "" {
		if usage.Found {
			source = usageSourceUpstream
		} else {
			source = usageSourceUnknown
		}
	}
	platformName := ""
	if selected.Site.Platform != "" {
		platformName = selected.Site.Platform
	}
	// Only attach cost when usage was found; avoid inventing spend on pure
	// network/timeout failures with zero tokens. Attribution name, not the
	// upstream rewritten name.
	var estimatedCost float64
	var billingDetails any
	if usage.Found {
		billing := EstimateBillingCostFromUsage(requestedModel, platformName, usage)
		estimatedCost = billing.EstimatedCost
		billingDetails = billing.BillingDetails
	}
	errMsg := strings.TrimSpace(errText)
	var errPtr *string
	if errMsg != "" {
		errPtr = &errMsg
	}
	entry := proxy.ProxyLogEntry{
		RouteID:            routeIDPtr,
		ChannelID:          &channelID,
		AccountID:          &accountID,
		DownstreamAPIKeyID: keyID,
		ModelRequested:     requestedModel,
		ModelActual:        &modelActual,
		Status:             "failed",
		HTTPStatus:         httpStatus,
		IsStream:           boolPtr(isStream),
		FirstByteLatencyMs: int64Ptr(latencyMs),
		LatencyMs:          latencyMs,
		PromptTokens:       int64Ptr(usage.PromptTokens),
		CompletionTokens:   int64Ptr(usage.CompletionTokens),
		TotalTokens:        int64Ptr(usage.TotalTokens),
		EstimatedCost:      estimatedCost,
		BillingDetails:     billingDetails,
		ClientFamily:       clientFamily,
		ClientAppID:        clientAppID,
		ClientAppName:      clientAppName,
		ClientConfidence:   clientConfidence,
		ErrorMessage:       errPtr,
		RetryCount:         retryCount,
		RequestID:          requestID,
		UpstreamPath:       &upstreamPath,
		UsageSource:        source,
	}
	logProxy(ctx, cfg, entry)
}

// truncateErrText bounds proxy_logs.error_message size for large upstream bodies.
func truncateErrText(s string) string {
	const maxErrRunes = 2000
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= maxErrRunes {
		return s
	}
	return string(r[:maxErrRunes]) + "..."
}
