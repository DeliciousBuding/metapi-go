package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type insertStmt struct {
	table   string
	columns []string
	values  []interface{}
}

// handwrittenBuilders are the tables whose copy needs per-column coercion the
// generic spec-driven builder cannot express: NOT NULL fallbacks inherited
// from the TypeScript migrator, nullable booleans where NULL means "inherit
// the global setting", and per-column JSON serialization. Every other table in
// the schema registry is copied by buildGenericTable straight from its parsed
// column spec, so a newly added table is transferred, verified and cleared
// without anyone having to remember to write a builder for it — the omission
// that silently dropped 17 of 37 tables.
var handwrittenBuilders = map[string]func([]map[string]interface{}) []insertStmt{
	"sites":                    buildSites,
	"site_api_endpoints":       buildSiteAPIEndpoints,
	"site_disabled_models":     buildSiteDisabledModels,
	"site_announcements":       buildSiteAnnouncements,
	"accounts":                 buildAccounts,
	"account_tokens":           buildAccountTokens,
	"checkin_logs":             buildCheckinLogs,
	"model_availability":       buildModelAvailability,
	"token_model_availability": buildTokenModelAvailability,
	"token_routes":             buildTokenRoutes,
	"route_channels":           buildRouteChannels,
	"route_group_sources":      buildRouteGroupSources,
	"proxy_logs":               buildProxyLogs,
	"proxy_video_tasks":        buildProxyVideoTasks,
	"downstream_api_keys":      buildDownstreamAPIKeys,
	"events":                   buildEvents,
	"settings":                 buildSettings,
	"catalog_sources":          buildCatalogSources,
	"admin_sessions":           buildAdminSessions,
}

func buildStatements(snapshot map[string][]map[string]interface{}) []insertStmt {
	var stmts []insertStmt
	// AllTableNames is topologically parents-before-children, so the insert
	// order satisfies every foreign key on the target.
	for _, table := range AllTableNames() {
		rows := snapshot[table]
		if build, ok := handwrittenBuilders[table]; ok {
			stmts = append(stmts, build(rows)...)
			continue
		}
		stmts = append(stmts, buildGenericTable(table, rows)...)
	}
	return stmts
}

// buildGenericTable copies rows of any registry table using the column spec
// parsed from its DDL. Only columns the source actually provides are listed,
// so a TypeScript source that predates a Go-added column still inserts (the
// target applies its own default) instead of writing an explicit NULL into a
// NOT NULL column.
func buildGenericTable(table string, rows []map[string]interface{}) []insertStmt {
	if len(rows) == 0 {
		return nil
	}
	cols := genericColumns(table, rows)
	if len(cols) == 0 {
		return nil
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = c.name
	}
	stmts := make([]insertStmt, 0, len(rows))
	for _, row := range rows {
		values := make([]interface{}, len(cols))
		for i, c := range cols {
			values[i] = coerceSpecValue(table, c.name, c.kind, row[c.name])
		}
		stmts = append(stmts, insertStmt{table: table, columns: names, values: values})
	}
	return stmts
}

// genericColumns intersects the registry column spec with the columns the
// source rows actually carry, keeping spec order.
func genericColumns(table string, rows []map[string]interface{}) []schemaColumn {
	present := make(map[string]bool)
	for _, row := range rows {
		for col := range row {
			present[col] = true
		}
	}
	if spec, err := schemaColumns(table); err == nil {
		out := make([]schemaColumn, 0, len(spec))
		for _, c := range spec {
			if present[c.name] {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	// Fallback, unreachable in production: RunMigration and AutoMigrate both
	// reject a malformed registry via schemaRegistryErr before this runs.
	names := make([]string, 0, len(present))
	for col := range present {
		names = append(names, col)
	}
	sort.Strings(names)
	out := make([]schemaColumn, 0, len(names))
	for _, n := range names {
		out = append(out, schemaColumn{name: n, kind: kindText})
	}
	return out
}

// coerceSpecValue converts one source value for a column of the given logical
// kind. NULL stays NULL (the target applies its own default or keeps the
// "unset" meaning); booleans arrive as Go bool so a PostgreSQL BOOLEAN column
// never sees an integer literal; JSON columns keep the TS serialization path.
func coerceSpecValue(table, column, kind string, raw interface{}) interface{} {
	if b, ok := raw.([]byte); ok {
		raw = string(b)
	}
	if raw == nil {
		return nil
	}
	var out interface{}
	switch kind {
	case kindBool:
		out = asNullableBool(raw)
	case kindInt, kindFloat:
		out = asNumber(raw, raw)
	default:
		out = asNullableString(raw)
	}
	// Only JSON columns go through the serializer; every other value keeps
	// the Go type the target driver binds natively (int64/float64/bool), so
	// an INTEGER column never receives the text "1" and a BOOLEAN column
	// never receives the text "true".
	if isJSONColumn(table, column) {
		return serializeJSONValue(out)
	}
	return out
}

func asString(v interface{}) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func asBoolean(v interface{}, fallback bool) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int64:
		return val != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func asNumber(v interface{}, fallback interface{}) interface{} {
	if v == nil {
		return fallback
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return fallback
		}
		return f
	}
	return fallback
}

func asStringDefault(v interface{}, fallback string) interface{} {
	if v == nil {
		return fallback
	}
	return asNullableString(v)
}

func asNullableString(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return fmt.Sprintf("%v", v)
}

// asNullableBool preserves NULL for nullable boolean columns (e.g.
// sites.resin_enabled). nil stays nil so per-site override semantics
// ("NULL = inherit global") survive the migration round-trip. Non-nil
// values are coerced via asBoolean so old SQLite rows (stored as 0/1
// INTEGER) and PG rows (stored as BOOLEAN) both round-trip correctly.
func asNullableBool(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return nil
	}
	return asBoolean(v, false)
}

// jsonColumnSet records which columns have logical type 'json'.
// (downstream_api_keys.tags is intentionally absent: it migrated as a plain
// TEXT passthrough before this refactor and keeps that behavior.)
var jsonColumnSet = map[string]bool{
	"sites.custom_headers":                         true,
	"accounts.extra_config":                        true,
	"token_routes.model_mapping":                   true,
	"token_routes.decision_snapshot":               true,
	"proxy_logs.conversion_details":                true,
	"proxy_logs.stream_timing":                     true,
	"proxy_logs.billing_details":                   true,
	"proxy_video_tasks.direct_identity":            true,
	"proxy_video_tasks.accounting_state":           true,
	"proxy_video_tasks.status_snapshot":            true,
	"proxy_video_tasks.upstream_response_meta":     true,
	"downstream_api_keys.supported_models":         true,
	"downstream_api_keys.allowed_route_ids":        true,
	"downstream_api_keys.site_weight_multipliers":  true,
	"downstream_api_keys.excluded_site_ids":        true,
	"downstream_api_keys.excluded_credential_refs": true,
	"downstream_api_keys.allowed_site_ids":         true,
	"downstream_api_keys.allowed_credential_refs":  true,
}

func isJSONColumn(table, column string) bool {
	return jsonColumnSet[table+"."+column]
}

func serializeColumnValue(table, column string, v interface{}) interface{} {
	if isJSONColumn(table, column) {
		return serializeJSONValue(v)
	}
	return asNullableString(v)
}

func serializeJSONValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

var runtimeDBSettingKeys = map[string]bool{
	"db_type": true,
	"db_url":  true,
	"db_ssl":  true,
}

func buildSites(rows []map[string]interface{}) []insertStmt {
	// Column list mirrors store.buildSitesDDL + additive columns exactly;
	// verifyBuilderColumnsMatchTarget enforces this at migration time.
	cols := []string{
		"id", "name", "url", "external_checkin_url", "platform", "proxy_url",
		"use_system_proxy", "custom_headers", "custom_headers_override_request_headers",
		"status", "is_pinned", "sort_order", "global_weight", "api_key",
		"max_concurrency",
		"post_refresh_probe_enabled", "post_refresh_probe_model", "post_refresh_probe_scope",
		"post_refresh_probe_latency_threshold_ms",
		"created_at", "updated_at", "tags", "browser_ua", "cf_clearance",
		"resin_enabled", "use_utls",
	}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "sites", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNullableString(row["name"]),
				asNullableString(row["url"]),
				asNullableString(row["external_checkin_url"]),
				asNullableString(row["platform"]),
				asNullableString(row["proxy_url"]),
				asBoolean(row["use_system_proxy"], false),
				serializeColumnValue("sites", "custom_headers", row["custom_headers"]),
				asBoolean(row["custom_headers_override_request_headers"], false),
				asStringDefault(row["status"], "active"),
				asBoolean(row["is_pinned"], false),
				asNumber(row["sort_order"], float64(0)),
				asNumber(row["global_weight"], float64(1)),
				asNullableString(row["api_key"]),
				asNumber(row["max_concurrency"], float64(0)),
				asBoolean(row["post_refresh_probe_enabled"], false),
				asStringDefault(row["post_refresh_probe_model"], ""),
				asStringDefault(row["post_refresh_probe_scope"], "single"),
				asNumber(row["post_refresh_probe_latency_threshold_ms"], float64(0)),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
				asNullableString(row["tags"]),
				asNullableString(row["browser_ua"]),
				asNullableString(row["cf_clearance"]),
				// resin_enabled is a nullable boolean override; preserve source
				// value verbatim (NULL stays NULL so per-site inherits global).
				asNullableBool(row["resin_enabled"]),
				// use_utls is a nullable boolean override; preserve source value
				// verbatim (NULL stays NULL so per-site inherits global UTLS_ENABLED).
				asNullableBool(row["use_utls"]),
			},
		})
	}
	return stmts
}

func buildAdminSessions(rows []map[string]interface{}) []insertStmt {
	// #1034 session model: sessions carry across a dialect cutover so an
	// operator stays signed in. token_hash is the text PK; timestamps are
	// RFC3339-UTC TEXT on both dialects.
	cols := []string{"token_hash", "created_at", "last_seen_at", "expires_at", "client_ip", "user_agent"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "admin_sessions", columns: cols,
			values: []interface{}{
				asStringDefault(row["token_hash"], ""),
				asStringDefault(row["created_at"], ""),
				asStringDefault(row["last_seen_at"], ""),
				asStringDefault(row["expires_at"], ""),
				asNullableString(row["client_ip"]),
				asNullableString(row["user_agent"]),
			},
		})
	}
	return stmts
}

func buildSiteAPIEndpoints(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "site_id", "url", "enabled", "sort_order", "cooldown_until", "last_selected_at", "last_failed_at", "last_failure_reason", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_api_endpoints", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["site_id"], float64(0)),
				asNullableString(row["url"]),
				asBoolean(row["enabled"], true),
				asNumber(row["sort_order"], float64(0)),
				asNullableString(row["cooldown_until"]),
				asNullableString(row["last_selected_at"]),
				asNullableString(row["last_failed_at"]),
				asNullableString(row["last_failure_reason"]),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
			},
		})
	}
	return stmts
}

func buildSiteDisabledModels(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "site_id", "model_name", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_disabled_models", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["site_id"], float64(0)),
				asNullableString(row["model_name"]),
				asNullableString(row["created_at"]),
			},
		})
	}
	return stmts
}

func buildSiteAnnouncements(rows []map[string]interface{}) []insertStmt {
	// The store DDL for site_announcements has no created_at/updated_at columns.
	cols := []string{"id", "site_id", "platform", "source_key", "title", "content", "level", "source_url", "starts_at", "ends_at", "upstream_created_at", "upstream_updated_at", "first_seen_at", "last_seen_at", "read_at", "dismissed_at", "raw_payload"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_announcements", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["site_id"], float64(0)),
				asNullableString(row["platform"]),
				asNullableString(row["source_key"]),
				asNullableString(row["title"]),
				asNullableString(row["content"]),
				asStringDefault(row["level"], "info"),
				asNullableString(row["source_url"]),
				asNullableString(row["starts_at"]),
				asNullableString(row["ends_at"]),
				asNullableString(row["upstream_created_at"]),
				asNullableString(row["upstream_updated_at"]),
				asNullableString(row["first_seen_at"]),
				asNullableString(row["last_seen_at"]),
				asNullableString(row["read_at"]),
				asNullableString(row["dismissed_at"]),
				asNullableString(row["raw_payload"]),
			},
		})
	}
	return stmts
}

func buildAccounts(rows []map[string]interface{}) []insertStmt {
	cols := []string{
		"id", "site_id", "username", "access_token", "api_token", "balance", "balance_used",
		"quota", "unit_cost", "value_score", "status", "is_pinned", "sort_order",
		"checkin_enabled", "last_checkin_at", "last_balance_refresh",
		"oauth_provider", "oauth_account_key", "oauth_project_id",
		"extra_config", "created_at", "updated_at", "tags", "remark",
	}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "accounts", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["site_id"], float64(0)),
				asNullableString(row["username"]),
				asNullableString(row["access_token"]),
				asNullableString(row["api_token"]),
				asNumber(row["balance"], float64(0)),
				asNumber(row["balance_used"], float64(0)),
				asNumber(row["quota"], float64(0)),
				asNumber(row["unit_cost"], nil),
				asNumber(row["value_score"], float64(0)),
				asStringDefault(row["status"], "active"),
				asBoolean(row["is_pinned"], false),
				asNumber(row["sort_order"], float64(0)),
				asBoolean(row["checkin_enabled"], true),
				asNullableString(row["last_checkin_at"]),
				asNullableString(row["last_balance_refresh"]),
				asNullableString(row["oauth_provider"]),
				asNullableString(row["oauth_account_key"]),
				asNullableString(row["oauth_project_id"]),
				serializeColumnValue("accounts", "extra_config", row["extra_config"]),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
				asNullableString(row["tags"]),
				asNullableString(row["remark"]),
			},
		})
	}
	return stmts
}

func buildAccountTokens(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "name", "token", "token_group", "value_status", "source", "enabled", "is_default", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "account_tokens", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["account_id"], float64(0)),
				asNullableString(row["name"]),
				asNullableString(row["token"]),
				asNullableString(row["token_group"]),
				asStringDefault(row["value_status"], "ready"),
				asStringDefault(row["source"], "manual"),
				asBoolean(row["enabled"], true),
				asBoolean(row["is_default"], false),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
			},
		})
	}
	return stmts
}

func buildCheckinLogs(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "status", "message", "reward", "failure_reason", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "checkin_logs", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["account_id"], float64(0)),
				asStringDefault(row["status"], "success"),
				asNullableString(row["message"]),
				asNullableString(row["reward"]),
				asNullableString(row["failure_reason"]),
				asNullableString(row["created_at"]),
			},
		})
	}
	return stmts
}

func buildModelAvailability(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "model_name", "available", "is_manual", "latency_ms", "checked_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "model_availability", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["account_id"], float64(0)),
				asNullableString(row["model_name"]),
				// available is nullable in both dialects and NULL means
				// "not probed yet", which is not the same statement as
				// false ("probed, unavailable"). Coercing NULL to false here
				// silently rewrote probe state during every copy migration.
				asNullableBool(row["available"]),
				asBoolean(row["is_manual"], false),
				asNumber(row["latency_ms"], nil),
				asNullableString(row["checked_at"]),
			},
		})
	}
	return stmts
}

func buildTokenModelAvailability(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "token_id", "model_name", "available", "latency_ms", "checked_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "token_model_availability", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["token_id"], float64(0)),
				asNullableString(row["model_name"]),
				// Nullable with no default: NULL means "no availability
				// recorded yet", which must not become false on the way
				// across a dialect boundary.
				asNullableBool(row["available"]),
				asNumber(row["latency_ms"], nil),
				asNullableString(row["checked_at"]),
			},
		})
	}
	return stmts
}

func buildTokenRoutes(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "model_pattern", "display_name", "display_icon", "model_mapping", "route_mode", "decision_snapshot", "decision_refreshed_at", "routing_strategy", "trace_sticky_mode", "context_length", "sort_order", "enabled", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "token_routes", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNullableString(row["model_pattern"]),
				asNullableString(row["display_name"]),
				asNullableString(row["display_icon"]),
				serializeColumnValue("token_routes", "model_mapping", row["model_mapping"]),
				asStringDefault(row["route_mode"], "pattern"),
				serializeColumnValue("token_routes", "decision_snapshot", row["decision_snapshot"]),
				asNullableString(row["decision_refreshed_at"]),
				asNullableString(row["routing_strategy"]),
				asStringDefault(row["trace_sticky_mode"], "disabled"),
				asNumber(row["context_length"], nil),
				asNumber(row["sort_order"], float64(0)),
				asBoolean(row["enabled"], true),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
			},
		})
	}
	return stmts
}

func buildRouteChannels(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "route_id", "account_id", "token_id", "oauth_route_unit_id", "source_model", "priority", "weight", "enabled", "manual_override", "success_count", "fail_count", "total_latency_ms", "total_cost", "last_used_at", "last_selected_at", "last_fail_at", "consecutive_fail_count", "cooldown_level", "cooldown_until", "cooldown_reason_code", "cooldown_reason", "cooldown_reason_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "route_channels", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["route_id"], float64(0)),
				asNumber(row["account_id"], float64(0)),
				asNumber(row["token_id"], nil),
				asNumber(row["oauth_route_unit_id"], nil),
				asNullableString(row["source_model"]),
				asNumber(row["priority"], float64(0)),
				asNumber(row["weight"], float64(10)),
				asBoolean(row["enabled"], true),
				asBoolean(row["manual_override"], false),
				asNumber(row["success_count"], float64(0)),
				asNumber(row["fail_count"], float64(0)),
				asNumber(row["total_latency_ms"], float64(0)),
				asNumber(row["total_cost"], float64(0)),
				asNullableString(row["last_used_at"]),
				asNullableString(row["last_selected_at"]),
				asNullableString(row["last_fail_at"]),
				asNumber(row["consecutive_fail_count"], float64(0)),
				asNumber(row["cooldown_level"], float64(0)),
				asNullableString(row["cooldown_until"]),
				asNullableString(row["cooldown_reason_code"]),
				asNullableString(row["cooldown_reason"]),
				asNullableString(row["cooldown_reason_at"]),
			},
		})
	}
	return stmts
}

func buildRouteGroupSources(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "group_route_id", "source_route_id"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "route_group_sources", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["group_route_id"], float64(0)),
				asNumber(row["source_route_id"], float64(0)),
			},
		})
	}
	return stmts
}

func buildProxyLogs(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "route_id", "channel_id", "upstream_channel_id", "upstream_grant_id", "account_id", "downstream_api_key_id", "model_requested", "model_actual", "status", "http_status", "is_stream", "first_byte_latency_ms", "first_output_latency_ms", "upstream_reported_model", "latency_ms", "prompt_tokens", "completion_tokens", "total_tokens", "estimated_cost", "billing_details", "conversion_details", "stream_timing", "client_family", "client_app_id", "client_app_name", "client_confidence", "error_message", "retry_count", "request_id", "created_at", "downstream_path", "gateway_request_id", "owner_user_id", "user_group_id"}

	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "proxy_logs", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNumber(row["route_id"], nil),
				asNumber(row["channel_id"], nil),
				asNumber(row["upstream_channel_id"], nil),
				asNumber(row["upstream_grant_id"], nil),
				asNumber(row["account_id"], nil),
				asNumber(row["downstream_api_key_id"], nil),
				asNullableString(row["model_requested"]),
				asNullableString(row["model_actual"]),
				asNullableString(row["status"]),
				asNumber(row["http_status"], nil),
				// Nullable with no default (additive sc2_022): rows logged
				// before the column existed carry NULL, and NULL is not the
				// same claim as false.
				asNullableBool(row["is_stream"]),
				asNumber(row["first_byte_latency_ms"], nil),
				asNumber(row["first_output_latency_ms"], nil),
				asNullableString(row["upstream_reported_model"]),
				asNumber(row["latency_ms"], nil),
				asNumber(row["prompt_tokens"], nil),
				asNumber(row["completion_tokens"], nil),
				asNumber(row["total_tokens"], nil),
				asNumber(row["estimated_cost"], nil),
				serializeColumnValue("proxy_logs", "billing_details", row["billing_details"]),
				serializeColumnValue("proxy_logs", "conversion_details", row["conversion_details"]),
				serializeColumnValue("proxy_logs", "stream_timing", row["stream_timing"]),
				asNullableString(row["client_family"]),
				asNullableString(row["client_app_id"]),
				asNullableString(row["client_app_name"]),
				asNullableString(row["client_confidence"]),
				asNullableString(row["error_message"]),
				asNumber(row["retry_count"], float64(0)),
				asNullableString(row["request_id"]),
				asNullableString(row["created_at"]),
				asNullableString(row["downstream_path"]),
				asNullableString(row["gateway_request_id"]),
				asNumber(row["owner_user_id"], nil),
				asNumber(row["user_group_id"], nil),
			},
		})
	}
	return stmts
}

func buildProxyVideoTasks(rows []map[string]interface{}) []insertStmt {
	cols := []string{"direct_identity", "accounting_state", "id", "public_id", "upstream_video_id", "site_url", "token_value", "requested_model", "actual_model", "channel_id", "account_id", "owner", "status_snapshot", "upstream_response_meta", "last_upstream_status", "last_polled_at", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "proxy_video_tasks", columns: cols,
			values: []interface{}{
				serializeColumnValue("proxy_video_tasks", "direct_identity", row["direct_identity"]),
				serializeColumnValue("proxy_video_tasks", "accounting_state", row["accounting_state"]),
				asNumber(row["id"], float64(0)),
				asNullableString(row["public_id"]),
				asNullableString(row["upstream_video_id"]),
				asNullableString(row["site_url"]),
				asNullableString(row["token_value"]),
				asNullableString(row["requested_model"]),
				asNullableString(row["actual_model"]),
				asNumber(row["channel_id"], nil),
				asNumber(row["account_id"], nil),
				asNullableString(row["owner"]),
				serializeColumnValue("proxy_video_tasks", "status_snapshot", row["status_snapshot"]),
				serializeColumnValue("proxy_video_tasks", "upstream_response_meta", row["upstream_response_meta"]),
				asNumber(row["last_upstream_status"], nil),
				asNullableString(row["last_polled_at"]),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
			},
		})
	}
	return stmts
}

func buildDownstreamAPIKeys(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "name", "key", "description", "group_name", "tags", "enabled", "expires_at", "max_cost", "used_cost", "max_requests", "used_requests", "supported_models", "allowed_route_ids", "site_weight_multipliers", "excluded_site_ids", "excluded_credential_refs", "allowed_site_ids", "allowed_credential_refs", "key_weight", "proxy_url", "max_rpm", "max_tpm", "ip_allowlist", "ip_blocklist", "access_policy", "last_used_at", "created_at", "updated_at", "key_owner_type", "owner_user_id"}

	var stmts []insertStmt
	for _, row := range rows {
		ownerType := "system"
		if raw := row["key_owner_type"]; raw != nil {
			ownerType = asString(raw)
		}
		stmts = append(stmts, insertStmt{
			table: "downstream_api_keys", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNullableString(row["name"]),
				asNullableString(row["key"]),
				asNullableString(row["description"]),
				asNullableString(row["group_name"]),
				asNullableString(row["tags"]),
				asBoolean(row["enabled"], true),
				asNullableString(row["expires_at"]),
				asNumber(row["max_cost"], nil),
				asNumber(row["used_cost"], float64(0)),
				asNumber(row["max_requests"], nil),
				asNumber(row["used_requests"], float64(0)),
				serializeColumnValue("downstream_api_keys", "supported_models", row["supported_models"]),
				serializeColumnValue("downstream_api_keys", "allowed_route_ids", row["allowed_route_ids"]),
				serializeColumnValue("downstream_api_keys", "site_weight_multipliers", row["site_weight_multipliers"]),
				serializeColumnValue("downstream_api_keys", "excluded_site_ids", row["excluded_site_ids"]),
				serializeColumnValue("downstream_api_keys", "excluded_credential_refs", row["excluded_credential_refs"]),
				serializeColumnValue("downstream_api_keys", "allowed_site_ids", row["allowed_site_ids"]),
				serializeColumnValue("downstream_api_keys", "allowed_credential_refs", row["allowed_credential_refs"]),
				asNumber(row["key_weight"], nil),
				asNullableString(row["proxy_url"]),
				asNumber(row["max_rpm"], nil),
				asNumber(row["max_tpm"], nil),
				asNullableString(row["ip_allowlist"]),
				asNullableString(row["ip_blocklist"]),
				asNullableString(row["access_policy"]),
				asNullableString(row["last_used_at"]),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
				ownerType,
				asNumber(row["owner_user_id"], nil),
			},
		})
	}
	return stmts
}

func buildEvents(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "type", "title", "message", "level", "read", "related_id", "related_type", "created_at", "title_key", "params"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "events", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNullableString(row["type"]),
				asNullableString(row["title"]),
				asNullableString(row["message"]),
				asStringDefault(row["level"], "info"),
				asBoolean(row["read"], false),
				asNumber(row["related_id"], nil),
				asNullableString(row["related_type"]),
				asNullableString(row["created_at"]),
				asNullableString(row["title_key"]),
				asNullableString(row["params"]),
			},
		})
	}
	return stmts
}

func buildSettings(rows []map[string]interface{}) []insertStmt {
	cols := []string{"key", "value"}
	var stmts []insertStmt
	for _, row := range rows {
		key := asString(row["key"])
		if runtimeDBSettingKeys[key] {
			continue
		}
		stmts = append(stmts, insertStmt{
			table: "settings", columns: cols,
			values: []interface{}{
				key,
				asNullableString(row["value"]),
			},
		})
	}
	return stmts
}

func buildCatalogSources(rows []map[string]interface{}) []insertStmt {
	// Column list mirrors store.buildCatalogSourcesDDL exactly;
	// verifyBuilderColumnsMatchTarget enforces this at migration time.
	cols := []string{
		"id", "name", "url", "enabled", "type", "sort_order",
		"last_success_at", "last_error", "last_count", "last_attempt_at",
		"created_at", "updated_at",
	}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "catalog_sources", columns: cols,
			values: []interface{}{
				asNumber(row["id"], float64(0)),
				asNullableString(row["name"]),
				asNullableString(row["url"]),
				asBoolean(row["enabled"], true),
				asStringDefault(row["type"], "custom"),
				asNumber(row["sort_order"], float64(0)),
				asNullableString(row["last_success_at"]),
				asNullableString(row["last_error"]),
				asNumber(row["last_count"], float64(0)),
				asNullableString(row["last_attempt_at"]),
				asNullableString(row["created_at"]),
				asNullableString(row["updated_at"]),
			},
		})
	}
	return stmts
}

// verifyBuilderColumnsMatchTarget fails the migration when any per-table
// insert builder's column list differs from the target schema created by
// store.AutoMigrate. This turns silent column drift (data loss) into a loud
// failure before any data is written.
func verifyBuilderColumnsMatchTarget(db *DB) error {
	builderColumns := builderColumnSets()
	for _, table := range AllTableNames() {
		actual, err := targetColumns(db, table)
		if err != nil {
			return fmt.Errorf("read target columns for %s: %w", table, err)
		}
		expected := builderColumns[table]
		missing, extra := diffColumnSets(expected, actual)
		if len(missing) > 0 || len(extra) > 0 {
			return fmt.Errorf(
				"cmd/migrate insert builder for table %q is out of sync with store DDL (missing=%v extra=%v); refusing to migrate",
				table, missing, extra,
			)
		}
	}
	return nil
}

// builderColumnSets derives each builder's INSERT column list by running the
// builders against one synthetic empty row per table. No hardcoded table list
// lives here — the table set comes from AllTableNames().
func builderColumnSets() map[string][]string {
	snapshot := make(map[string][]map[string]interface{}, len(AllTableNames()))
	for _, table := range AllTableNames() {
		// A synthetic row carrying every column the registry declares: the
		// hand-written builders pick their columns by name, the generic
		// builder intersects the row's keys with the spec, so either way the
		// emitted list is the full spec — exactly what the guard compares
		// against the target schema.
		row := make(map[string]interface{})
		if cols, err := schemaColumns(table); err == nil {
			for _, c := range cols {
				row[c.name] = nil
			}
		}
		snapshot[table] = []map[string]interface{}{row}
	}
	out := make(map[string][]string)
	for _, stmt := range buildStatements(snapshot) {
		out[stmt.table] = stmt.columns
	}
	return out
}

// targetColumns returns the canonical column list of a target table.
func targetColumns(db *DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s" LIMIT 0`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

// diffColumnSets reports names present in exactly one of the two sets.
func diffColumnSets(expected, actual []string) (missing, extra []string) {
	expectedSet := make(map[string]bool, len(expected))
	for _, c := range expected {
		expectedSet[c] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, c := range actual {
		actualSet[c] = true
	}
	for _, c := range expected {
		if !actualSet[c] {
			missing = append(missing, c)
		}
	}
	for _, c := range actual {
		if !expectedSet[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}
