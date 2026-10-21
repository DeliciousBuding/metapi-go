package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GatewayRequest records one gateway-observed HTTP request or WebSocket turn.
// ExternalRequestID is untrusted client correlation, never its identity.
type GatewayRequest struct {
	OwnerUserID          *int64  `db:"owner_user_id" json:"ownerUserId"`
	UserGroupID          *int64  `db:"user_group_id" json:"userGroupId"`
	ID                   string  `db:"id" json:"id"`
	ExternalRequestID    *string `db:"external_request_id" json:"externalRequestId"`
	DownstreamPath       *string `db:"downstream_path" json:"downstreamPath"`
	Transport            string  `db:"transport" json:"transport"`
	ModelRequested       *string `db:"model_requested" json:"modelRequested"`
	DownstreamAPIKeyID   *int64  `db:"downstream_api_key_id" json:"downstreamApiKeyId"`
	TerminalStatus       string  `db:"terminal_status" json:"terminalStatus"`
	AttemptCount         int     `db:"attempt_count" json:"attemptCount"`
	ReasonCode           *string `db:"reason_code" json:"reasonCode"`
	HTTPStatus           *int    `db:"http_status" json:"httpStatus"`
	OutputStarted        bool    `db:"output_started" json:"outputStarted"`
	FirstByteLatencyMs   *int64  `db:"first_byte_latency_ms" json:"firstByteLatencyMs"`
	FirstOutputLatencyMs *int64  `db:"first_output_latency_ms" json:"firstOutputLatencyMs"`
	LatencyMs            int64   `db:"latency_ms" json:"latencyMs"`
	StartedAt            string  `db:"started_at" json:"startedAt"`
	EndedAt              string  `db:"ended_at" json:"endedAt"`
}

func buildGatewayRequestsDDL(d string) string {
	boolType := "INTEGER"
	if isPG(d) {
		boolType = "BOOLEAN"
	}
	return `CREATE TABLE IF NOT EXISTS gateway_requests (
		id TEXT PRIMARY KEY NOT NULL,
		owner_user_id BIGINT,
		user_group_id BIGINT,
		external_request_id TEXT,
		downstream_path TEXT,
		transport TEXT NOT NULL CHECK (transport IN ('http','websocket')),
		model_requested TEXT,
		downstream_api_key_id INTEGER,
		terminal_status TEXT NOT NULL CHECK (terminal_status IN ('completed','upstream_failed','output_interrupted','client_cancelled','rejected_no_route','rejected_policy','gateway_failed')),
		attempt_count INTEGER NOT NULL CHECK (attempt_count >= 0),
		reason_code TEXT,
		http_status INTEGER,
		output_started ` + boolType + ` NOT NULL,
		first_byte_latency_ms BIGINT,
		first_output_latency_ms BIGINT,
		latency_ms BIGINT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT NOT NULL
	)`
}

// InsertGatewayRequest persists a terminal observation once. Repeated completion
// calls must not overwrite the first outcome, including after cancellation.
func InsertGatewayRequest(ctx context.Context, db *DB, entry GatewayRequest) error {
	return InsertGatewayRequests(ctx, db, []GatewayRequest{entry})
}

// InsertGatewayRequests shares validation and wire layout with the single-row
// path. The bounded log writer uses one INSERT per batch in either dialect.
func InsertGatewayRequests(ctx context.Context, db *DB, entries []GatewayRequest) error {
	if len(entries) == 0 {
		return nil
	}
	args := make([]any, 0, len(entries)*16)
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		row, err := gatewayRequestArgs(entry)
		if err != nil {
			return err
		}
		values = append(values, "(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)")
		args = append(args, row...)
	}
	_, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO gateway_requests
		(id,external_request_id,downstream_path,transport,model_requested,downstream_api_key_id,
		terminal_status,attempt_count,reason_code,http_status,output_started,first_byte_latency_ms,first_output_latency_ms,
		latency_ms,started_at,ended_at,owner_user_id,user_group_id) VALUES `+strings.Join(values, ",")+` ON CONFLICT (id) DO NOTHING`), args...)
	return err
}

func gatewayRequestArgs(entry GatewayRequest) ([]any, error) {
	if strings.TrimSpace(entry.ID) == "" {
		return nil, fmt.Errorf("store: gateway request ID is required")
	}
	if entry.AttemptCount < 0 {
		return nil, fmt.Errorf("store: gateway request attempt count must not be negative")
	}
	started, err := time.Parse(time.RFC3339Nano, entry.StartedAt)
	if err != nil {
		return nil, fmt.Errorf("store: invalid gateway request start: %w", err)
	}
	ended, err := time.Parse(time.RFC3339Nano, entry.EndedAt)
	if err != nil {
		return nil, fmt.Errorf("store: invalid gateway request end: %w", err)
	}
	entry.StartedAt = started.UTC().Format(GatewayRequestTimeLayout)
	entry.EndedAt = ended.UTC().Format(GatewayRequestTimeLayout)
	return []any{entry.ID, entry.ExternalRequestID, entry.DownstreamPath, entry.Transport, entry.ModelRequested,
		entry.DownstreamAPIKeyID, entry.TerminalStatus, entry.AttemptCount, entry.ReasonCode, entry.HTTPStatus, entry.OutputStarted,
		entry.FirstByteLatencyMs, entry.FirstOutputLatencyMs, entry.LatencyMs, entry.StartedAt, entry.EndedAt, entry.OwnerUserID, entry.UserGroupID}, nil
}

// GatewayRequestTimeLayout keeps TEXT timestamps chronologically sortable at
// fractional-second boundaries and across client time-zone offsets.
const GatewayRequestTimeLayout = "2006-01-02T15:04:05.000000000Z"

// PruneProxyRequestLogs shares the existing usage-log retention window. Remove
// linked attempts with their expired request in one transaction; legacy attempts
// retain their original created_at policy. No foreign key is used because attempt
// writes precede terminal request persistence, which is intentionally best-effort.
func PruneProxyRequestLogs(db *DB, cutoff string) (int64, error) {
	parsed, err := time.Parse(time.RFC3339Nano, cutoff)
	if err != nil {
		return 0, err
	}
	requestCutoff := parsed.UTC().Format(GatewayRequestTimeLayout)
	tx, err := db.Beginx()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.Exec(db.Rebind(`DELETE FROM proxy_logs WHERE
		(created_at < ? AND NOT EXISTS (SELECT 1 FROM gateway_requests gr WHERE gr.id = proxy_logs.gateway_request_id))
		OR gateway_request_id IN
		(SELECT id FROM gateway_requests WHERE ended_at < ?)`), cutoff, requestCutoff)
	if err != nil {
		return 0, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	result, err = tx.Exec(db.Rebind(`DELETE FROM gateway_requests WHERE ended_at < ?`), requestCutoff)
	if err != nil {
		return 0, err
	}
	requests, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted + requests, nil
}
