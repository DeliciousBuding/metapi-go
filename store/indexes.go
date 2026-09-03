package store

// buildIndexes returns all 70 non-UNIQUE index creation statements.
// Both SQLite and PostgreSQL support CREATE INDEX IF NOT EXISTS syntax.
// UNIQUE constraints are already handled inside CREATE TABLE via CONSTRAINT... UNIQUE.
func buildIndexes() []struct {
	name string
	sql  string
} {
	return []struct {
		name string
		sql  string
	}{
		// sites
		{"sites_status_idx", `CREATE INDEX IF NOT EXISTS sites_status_idx ON sites (status)`},
		// site_api_endpoints
		{"site_api_endpoints_site_enabled_sort_idx", `CREATE INDEX IF NOT EXISTS site_api_endpoints_site_enabled_sort_idx ON site_api_endpoints (site_id, enabled, sort_order)`},
		{"site_api_endpoints_site_cooldown_idx", `CREATE INDEX IF NOT EXISTS site_api_endpoints_site_cooldown_idx ON site_api_endpoints (site_id, cooldown_until)`},
		// site_disabled_models
		{"site_disabled_models_site_id_idx", `CREATE INDEX IF NOT EXISTS site_disabled_models_site_id_idx ON site_disabled_models (site_id)`},
		// accounts
		{"accounts_site_id_idx", `CREATE INDEX IF NOT EXISTS accounts_site_id_idx ON accounts (site_id)`},
		{"accounts_status_idx", `CREATE INDEX IF NOT EXISTS accounts_status_idx ON accounts (status)`},
		{"accounts_site_status_idx", `CREATE INDEX IF NOT EXISTS accounts_site_status_idx ON accounts (site_id, status)`},
		{"accounts_oauth_provider_idx", `CREATE INDEX IF NOT EXISTS accounts_oauth_provider_idx ON accounts (oauth_provider)`},
		{"accounts_oauth_identity_idx", `CREATE INDEX IF NOT EXISTS accounts_oauth_identity_idx ON accounts (oauth_provider, oauth_account_key, oauth_project_id)`},
		// account_tokens
		{"account_tokens_account_id_idx", `CREATE INDEX IF NOT EXISTS account_tokens_account_id_idx ON account_tokens (account_id)`},
		{"account_tokens_account_enabled_idx", `CREATE INDEX IF NOT EXISTS account_tokens_account_enabled_idx ON account_tokens (account_id, enabled)`},
		{"account_tokens_enabled_idx", `CREATE INDEX IF NOT EXISTS account_tokens_enabled_idx ON account_tokens (enabled)`},
		// checkin_logs
		{"checkin_logs_account_created_at_idx", `CREATE INDEX IF NOT EXISTS checkin_logs_account_created_at_idx ON checkin_logs (account_id, created_at)`},
		{"checkin_logs_created_at_idx", `CREATE INDEX IF NOT EXISTS checkin_logs_created_at_idx ON checkin_logs (created_at)`},
		{"checkin_logs_status_idx", `CREATE INDEX IF NOT EXISTS checkin_logs_status_idx ON checkin_logs (status)`},
		// model_availability
		{"model_availability_account_available_idx", `CREATE INDEX IF NOT EXISTS model_availability_account_available_idx ON model_availability (account_id, available)`},
		{"model_availability_model_name_idx", `CREATE INDEX IF NOT EXISTS model_availability_model_name_idx ON model_availability (model_name)`},
		// token_model_availability
		{"token_model_availability_token_available_idx", `CREATE INDEX IF NOT EXISTS token_model_availability_token_available_idx ON token_model_availability (token_id, available)`},
		{"token_model_availability_model_name_idx", `CREATE INDEX IF NOT EXISTS token_model_availability_model_name_idx ON token_model_availability (model_name)`},
		{"token_model_availability_available_idx", `CREATE INDEX IF NOT EXISTS token_model_availability_available_idx ON token_model_availability (available)`},
		// token_routes
		{"token_routes_model_pattern_idx", `CREATE INDEX IF NOT EXISTS token_routes_model_pattern_idx ON token_routes (model_pattern)`},
		{"token_routes_enabled_idx", `CREATE INDEX IF NOT EXISTS token_routes_enabled_idx ON token_routes (enabled)`},
		// route_group_sources
		{"route_group_sources_source_route_id_idx", `CREATE INDEX IF NOT EXISTS route_group_sources_source_route_id_idx ON route_group_sources (source_route_id)`},
		// oauth_route_units
		{"oauth_route_units_site_provider_idx", `CREATE INDEX IF NOT EXISTS oauth_route_units_site_provider_idx ON oauth_route_units (site_id, provider)`},
		{"oauth_route_units_enabled_idx", `CREATE INDEX IF NOT EXISTS oauth_route_units_enabled_idx ON oauth_route_units (enabled)`},
		// oauth_route_unit_members
		{"oauth_route_unit_members_unit_sort_idx", `CREATE INDEX IF NOT EXISTS oauth_route_unit_members_unit_sort_idx ON oauth_route_unit_members (unit_id, sort_order)`},
		{"oauth_route_unit_members_unit_cooldown_idx", `CREATE INDEX IF NOT EXISTS oauth_route_unit_members_unit_cooldown_idx ON oauth_route_unit_members (unit_id, cooldown_until)`},
		// route_channels
		{"route_channels_route_id_idx", `CREATE INDEX IF NOT EXISTS route_channels_route_id_idx ON route_channels (route_id)`},
		{"route_channels_account_id_idx", `CREATE INDEX IF NOT EXISTS route_channels_account_id_idx ON route_channels (account_id)`},
		{"route_channels_token_id_idx", `CREATE INDEX IF NOT EXISTS route_channels_token_id_idx ON route_channels (token_id)`},
		{"route_channels_oauth_route_unit_id_idx", `CREATE INDEX IF NOT EXISTS route_channels_oauth_route_unit_id_idx ON route_channels (oauth_route_unit_id)`},
		{"route_channels_route_enabled_idx", `CREATE INDEX IF NOT EXISTS route_channels_route_enabled_idx ON route_channels (route_id, enabled)`},
		{"route_channels_route_token_idx", `CREATE INDEX IF NOT EXISTS route_channels_route_token_idx ON route_channels (route_id, token_id)`},
		// proxy_logs
		{"proxy_logs_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_created_at_idx ON proxy_logs (created_at)`},
		{"proxy_logs_account_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_account_created_at_idx ON proxy_logs (account_id, created_at)`},
		{"proxy_logs_status_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_status_created_at_idx ON proxy_logs (status, created_at)`},
		{"proxy_logs_model_actual_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_model_actual_created_at_idx ON proxy_logs (model_actual, created_at)`},
		{"proxy_logs_downstream_api_key_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_downstream_api_key_created_at_idx ON proxy_logs (downstream_api_key_id, created_at)`},
		{"proxy_logs_client_app_id_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_client_app_id_created_at_idx ON proxy_logs (client_app_id, created_at)`},
		{"proxy_logs_client_family_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_client_family_created_at_idx ON proxy_logs (client_family, created_at)`},
		// proxy_logs_summary_covering_idx covers the proxy-logs summary aggregate
		// (COUNT + success/failed CASEs on status, SUM of estimated_cost, and the
		// effective-token CASE over total_tokens with prompt/completion fallback),
		// letting SQLite/PostgreSQL satisfy the five SUMs from the index alone
		// instead of scanning the heap (~1s at 500k rows; the covering scan
		// measured ~40% faster than the table scan on the audit fixture). All
		// five columns are base-schema columns, so fresh AND existing installs
		// converge via this normal buildIndexes pass (no additive step needed,
		// unlike proxy_logs_request_id_created_at_idx whose column is additive).
		{"proxy_logs_summary_covering_idx", `CREATE INDEX IF NOT EXISTS proxy_logs_summary_covering_idx ON proxy_logs (status, estimated_cost, total_tokens, prompt_tokens, completion_tokens)`},
		// proxy_logs_request_id_created_at_idx is created by additive step
		// sc2_004_proxy_logs_request_id (after the request_id column exists).
		// proxy_video_tasks
		{"proxy_video_tasks_upstream_video_id_idx", `CREATE INDEX IF NOT EXISTS proxy_video_tasks_upstream_video_id_idx ON proxy_video_tasks (upstream_video_id)`},
		{"proxy_video_tasks_created_at_idx", `CREATE INDEX IF NOT EXISTS proxy_video_tasks_created_at_idx ON proxy_video_tasks (created_at)`},
		// analytics_projection_checkpoints
		{"analytics_projection_checkpoints_recompute_from_id_idx", `CREATE INDEX IF NOT EXISTS analytics_projection_checkpoints_recompute_from_id_idx ON analytics_projection_checkpoints (recompute_from_id)`},
		{"analytics_projection_checkpoints_lease_expires_at_idx", `CREATE INDEX IF NOT EXISTS analytics_projection_checkpoints_lease_expires_at_idx ON analytics_projection_checkpoints (lease_expires_at)`},
		// site_day_usage
		{"site_day_usage_day_idx", `CREATE INDEX IF NOT EXISTS site_day_usage_day_idx ON site_day_usage (local_day)`},
		{"site_day_usage_site_id_idx", `CREATE INDEX IF NOT EXISTS site_day_usage_site_id_idx ON site_day_usage (site_id)`},
		// site_hour_usage
		{"site_hour_usage_hour_idx", `CREATE INDEX IF NOT EXISTS site_hour_usage_hour_idx ON site_hour_usage (bucket_start_utc)`},
		{"site_hour_usage_site_id_idx", `CREATE INDEX IF NOT EXISTS site_hour_usage_site_id_idx ON site_hour_usage (site_id)`},
		// model_day_usage
		{"model_day_usage_day_idx", `CREATE INDEX IF NOT EXISTS model_day_usage_day_idx ON model_day_usage (local_day)`},
		{"model_day_usage_site_id_idx", `CREATE INDEX IF NOT EXISTS model_day_usage_site_id_idx ON model_day_usage (site_id)`},
		{"model_day_usage_model_idx", `CREATE INDEX IF NOT EXISTS model_day_usage_model_idx ON model_day_usage (model)`},
		// balance_history
		{"balance_history_day_idx", `CREATE INDEX IF NOT EXISTS balance_history_day_idx ON balance_history (local_day)`},
		{"balance_history_account_idx", `CREATE INDEX IF NOT EXISTS balance_history_account_idx ON balance_history (account_id)`},
		// model_verify_history
		{"model_verify_history_batch_idx", `CREATE INDEX IF NOT EXISTS model_verify_history_batch_idx ON model_verify_history (batch_id, created_at)`},
		{"model_verify_history_model_idx", `CREATE INDEX IF NOT EXISTS model_verify_history_model_idx ON model_verify_history (model_name, created_at)`},
		// model_probe_results
		{"model_probe_results_channel_model_idx", `CREATE INDEX IF NOT EXISTS model_probe_results_channel_model_idx ON model_probe_results (channel_id, model_name, created_at)`},
		{"model_probe_results_account_model_idx", `CREATE INDEX IF NOT EXISTS model_probe_results_account_model_idx ON model_probe_results (account_id, model_name, created_at)`},
		{"model_probe_results_created_at_idx", `CREATE INDEX IF NOT EXISTS model_probe_results_created_at_idx ON model_probe_results (created_at)`},
		// model_name_redirects
		{"model_name_redirects_account_actual_idx", `CREATE INDEX IF NOT EXISTS model_name_redirects_account_actual_idx ON model_name_redirects (account_id, actual)`},
		// admin_audit_logs
		{"admin_audit_logs_created_at_idx", `CREATE INDEX IF NOT EXISTS admin_audit_logs_created_at_idx ON admin_audit_logs (created_at)`},
		{"admin_audit_logs_method_idx", `CREATE INDEX IF NOT EXISTS admin_audit_logs_method_idx ON admin_audit_logs (method)`},
		// downstream_api_keys
		{"downstream_api_keys_name_idx", `CREATE INDEX IF NOT EXISTS downstream_api_keys_name_idx ON downstream_api_keys (name)`},
		{"downstream_api_keys_enabled_idx", `CREATE INDEX IF NOT EXISTS downstream_api_keys_enabled_idx ON downstream_api_keys (enabled)`},
		{"downstream_api_keys_expires_at_idx", `CREATE INDEX IF NOT EXISTS downstream_api_keys_expires_at_idx ON downstream_api_keys (expires_at)`},
		// site_announcements
		{"site_announcements_site_id_first_seen_at_idx", `CREATE INDEX IF NOT EXISTS site_announcements_site_id_first_seen_at_idx ON site_announcements (site_id, first_seen_at)`},
		{"site_announcements_read_at_idx", `CREATE INDEX IF NOT EXISTS site_announcements_read_at_idx ON site_announcements (read_at)`},
		// events
		{"events_read_created_at_idx", `CREATE INDEX IF NOT EXISTS events_read_created_at_idx ON events (read, created_at)`},
		{"events_type_created_at_idx", `CREATE INDEX IF NOT EXISTS events_type_created_at_idx ON events (type, created_at)`},
		{"events_created_at_idx", `CREATE INDEX IF NOT EXISTS events_created_at_idx ON events (created_at)`},
		// admin_background_tasks
		{"admin_background_tasks_created_at_idx", `CREATE INDEX IF NOT EXISTS admin_background_tasks_created_at_idx ON admin_background_tasks (created_at)`},
		// announcement_dismissals
		{"announcement_dismissals_announcement_id_idx", `CREATE INDEX IF NOT EXISTS announcement_dismissals_announcement_id_idx ON announcement_dismissals (announcement_id)`},
	}
}
