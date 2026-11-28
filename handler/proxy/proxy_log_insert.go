package proxyhandler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

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
const proxyLogInsertColumns = `route_id, channel_id, upstream_channel_id, upstream_grant_id, account_id, downstream_api_key_id,
			model_requested, model_actual, status, http_status, is_stream,
			first_byte_latency_ms, first_output_latency_ms, upstream_reported_model, latency_ms,
			prompt_tokens, completion_tokens, total_tokens,
			estimated_cost, billing_details, conversion_details, stream_timing,
			client_family, client_app_id, client_app_name, client_confidence,
			error_message, retry_count, request_id, created_at, downstream_path, gateway_request_id, owner_user_id, user_group_id`

// proxyLogSingleRowPlaceholders matches the canonical INSERT column list.
// store.DB rebinds ? to $N for PostgreSQL, so the batch writer builds rows of
// the same placeholder shape and lets the rebind do the dialect work.
const proxyLogSingleRowPlaceholders = `(?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?)`

// Drop oversized observations without losing the accounting row.
func encodeProxyObservation(value any) any {
	if value == nil {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) > 8192 || len(b) == 0 || b[0] != '{' {
		return nil
	}
	return string(b)
}

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
		nullInt64(entry.UpstreamChannelID),
		nullInt64(entry.UpstreamGrantID),
		nullInt64(entry.AccountID),
		nullInt64(entry.DownstreamAPIKeyID),
		nullString(strPtrOrEmpty(entry.ModelRequested)),
		nullString(entry.ModelActual),
		nullString(strPtrOrEmpty(entry.Status)),
		proxyLogHTTPStatus(entry.HTTPStatus),
		nullBool(entry.IsStream),
		nullInt64(entry.FirstByteLatencyMs),
		nullInt64(entry.FirstOutputLatencyMs),
		nullString(entry.UpstreamReportedModel),
		entry.LatencyMs,
		nullInt64(entry.PromptTokens),
		nullInt64(entry.CompletionTokens),
		nullInt64(entry.TotalTokens),
		entry.EstimatedCost,
		billingDetails, encodeProxyObservation(entry.ConversionDetails), encodeProxyObservation(entry.StreamTiming),
		nullString(strPtrOrEmpty(entry.ClientFamily)),
		nullString(strPtrOrEmpty(entry.ClientAppID)),
		nullString(strPtrOrEmpty(entry.ClientAppName)),
		nullString(strPtrOrEmpty(entry.ClientConfidence)),
		nullString(entry.ErrorMessage),
		entry.RetryCount,
		nullString(strPtrOrEmpty(entry.RequestID)),
		createdAt,
		nullString(entry.DownstreamPath),
		nullString(strPtrOrEmpty(entry.GatewayRequestID)),
		nullInt64(entry.OwnerUserID),
		nullInt64(entry.UserGroupID),
	}
}

func proxyLogHTTPStatus(status int) any {
	if status == 0 {
		return nil
	}
	return status
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
