package store

// ---- Table DDL builders ----

func buildSitesDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS sites (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			external_checkin_url TEXT,
			platform TEXT NOT NULL,
			proxy_url TEXT,
			use_system_proxy BOOLEAN DEFAULT FALSE,
			custom_headers TEXT,
			custom_headers_override_request_headers BOOLEAN DEFAULT FALSE,
			status TEXT NOT NULL DEFAULT 'active',
			is_pinned BOOLEAN DEFAULT FALSE,
			sort_order INTEGER DEFAULT 0,
			global_weight DOUBLE PRECISION DEFAULT 1,
			api_key TEXT,
			max_concurrency INTEGER DEFAULT 0,
			post_refresh_probe_enabled BOOLEAN DEFAULT FALSE,
			post_refresh_probe_model TEXT DEFAULT '',
			post_refresh_probe_scope TEXT DEFAULT 'single',
			post_refresh_probe_latency_threshold_ms INTEGER DEFAULT 0,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT sites_platform_url_unique UNIQUE (platform, url)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS sites (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL,
		external_checkin_url TEXT,
		platform TEXT NOT NULL,
		proxy_url TEXT,
		use_system_proxy INTEGER DEFAULT 0,
		custom_headers TEXT,
		custom_headers_override_request_headers INTEGER DEFAULT 0,
		status TEXT NOT NULL DEFAULT 'active',
		is_pinned INTEGER DEFAULT 0,
		sort_order INTEGER DEFAULT 0,
		global_weight REAL DEFAULT 1,
		api_key TEXT,
		max_concurrency INTEGER DEFAULT 0,
		post_refresh_probe_enabled INTEGER DEFAULT 0,
		post_refresh_probe_model TEXT DEFAULT '',
		post_refresh_probe_scope TEXT DEFAULT 'single',
		post_refresh_probe_latency_threshold_ms INTEGER DEFAULT 0,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT sites_platform_url_unique UNIQUE (platform, url)
	)`
}

func buildSiteAPIEndpointsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS site_api_endpoints (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			url TEXT NOT NULL,
			enabled BOOLEAN DEFAULT TRUE,
			sort_order INTEGER DEFAULT 0,
			cooldown_until TEXT,
			last_selected_at TEXT,
			last_failed_at TEXT,
			last_failure_reason TEXT,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT site_api_endpoints_site_url_unique UNIQUE (site_id, url)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS site_api_endpoints (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		url TEXT NOT NULL,
		enabled INTEGER DEFAULT 1,
		sort_order INTEGER DEFAULT 0,
		cooldown_until TEXT,
		last_selected_at TEXT,
		last_failed_at TEXT,
		last_failure_reason TEXT,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT site_api_endpoints_site_url_unique UNIQUE (site_id, url),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildSiteDisabledModelsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS site_disabled_models (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			model_name TEXT NOT NULL,
			created_at TEXT,
			CONSTRAINT site_disabled_models_site_model_unique UNIQUE (site_id, model_name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS site_disabled_models (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		model_name TEXT NOT NULL,
		created_at TEXT,
		CONSTRAINT site_disabled_models_site_model_unique UNIQUE (site_id, model_name),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildAccountsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS accounts (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			username TEXT,
			access_token TEXT NOT NULL,
			api_token TEXT,
			balance DOUBLE PRECISION DEFAULT 0,
			balance_used DOUBLE PRECISION DEFAULT 0,
			quota DOUBLE PRECISION DEFAULT 0,
			unit_cost DOUBLE PRECISION,
			value_score DOUBLE PRECISION DEFAULT 0,
			status TEXT DEFAULT 'active',
			is_pinned BOOLEAN DEFAULT FALSE,
			sort_order INTEGER DEFAULT 0,
			checkin_enabled BOOLEAN DEFAULT TRUE,
			last_checkin_at TEXT,
			last_balance_refresh TEXT,
			oauth_provider TEXT,
			oauth_account_key TEXT,
			oauth_project_id TEXT,
			extra_config TEXT,
			created_at TEXT,
			updated_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		username TEXT,
		access_token TEXT NOT NULL,
		api_token TEXT,
		balance REAL DEFAULT 0,
		balance_used REAL DEFAULT 0,
		quota REAL DEFAULT 0,
		unit_cost REAL,
		value_score REAL DEFAULT 0,
		status TEXT DEFAULT 'active',
		is_pinned INTEGER DEFAULT 0,
		sort_order INTEGER DEFAULT 0,
		checkin_enabled INTEGER DEFAULT 1,
		last_checkin_at TEXT,
		last_balance_refresh TEXT,
		oauth_provider TEXT,
		oauth_account_key TEXT,
		oauth_project_id TEXT,
		extra_config TEXT,
		created_at TEXT,
		updated_at TEXT,
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildAccountTokensDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS account_tokens (
			id SERIAL PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			token TEXT NOT NULL,
			token_group TEXT,
			value_status TEXT NOT NULL DEFAULT 'ready',
			source TEXT DEFAULT 'manual',
			enabled BOOLEAN DEFAULT TRUE,
			is_default BOOLEAN DEFAULT FALSE,
			created_at TEXT,
			updated_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS account_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		name TEXT NOT NULL,
		token TEXT NOT NULL,
		token_group TEXT,
		value_status TEXT NOT NULL DEFAULT 'ready',
		source TEXT DEFAULT 'manual',
		enabled INTEGER DEFAULT 1,
		is_default INTEGER DEFAULT 0,
		created_at TEXT,
		updated_at TEXT,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

func buildCheckinLogsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS checkin_logs (
			id SERIAL PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			status TEXT NOT NULL,
			message TEXT,
			reward TEXT,
			failure_reason TEXT,
			created_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS checkin_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		message TEXT,
		reward TEXT,
		failure_reason TEXT,
		created_at TEXT,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

func buildModelAvailabilityDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS model_availability (
			id SERIAL PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			model_name TEXT NOT NULL,
			available BOOLEAN,
			is_manual BOOLEAN DEFAULT FALSE,
			latency_ms INTEGER,
			checked_at TEXT,
			CONSTRAINT model_availability_account_model_unique UNIQUE (account_id, model_name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS model_availability (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		model_name TEXT NOT NULL,
		available INTEGER,
		is_manual INTEGER DEFAULT 0,
		latency_ms INTEGER,
		checked_at TEXT,
		CONSTRAINT model_availability_account_model_unique UNIQUE (account_id, model_name),
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

func buildTokenModelAvailabilityDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS token_model_availability (
			id SERIAL PRIMARY KEY,
			token_id INTEGER NOT NULL REFERENCES account_tokens(id) ON DELETE CASCADE,
			model_name TEXT NOT NULL,
			available BOOLEAN,
			latency_ms INTEGER,
			checked_at TEXT,
			CONSTRAINT token_model_availability_token_model_unique UNIQUE (token_id, model_name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS token_model_availability (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		token_id INTEGER NOT NULL,
		model_name TEXT NOT NULL,
		available INTEGER,
		latency_ms INTEGER,
		checked_at TEXT,
		CONSTRAINT token_model_availability_token_model_unique UNIQUE (token_id, model_name),
		FOREIGN KEY (token_id) REFERENCES account_tokens(id) ON DELETE CASCADE
	)`
}

func buildTokenRoutesDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS token_routes (
			id SERIAL PRIMARY KEY,
			model_pattern TEXT NOT NULL,
			display_name TEXT,
			display_icon TEXT,
			route_mode TEXT DEFAULT 'pattern',
			model_mapping TEXT,
			decision_snapshot TEXT,
			decision_refreshed_at TEXT,
			routing_strategy TEXT DEFAULT 'weighted',
			context_length INTEGER,
			sort_order INTEGER DEFAULT 0,
			enabled BOOLEAN DEFAULT TRUE,
			created_at TEXT,
			updated_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS token_routes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		model_pattern TEXT NOT NULL,
		display_name TEXT,
		display_icon TEXT,
		route_mode TEXT DEFAULT 'pattern',
		model_mapping TEXT,
		decision_snapshot TEXT,
		decision_refreshed_at TEXT,
		routing_strategy TEXT DEFAULT 'weighted',
		context_length INTEGER,
		sort_order INTEGER DEFAULT 0,
		enabled INTEGER DEFAULT 1,
		created_at TEXT,
		updated_at TEXT
	)`
}

func buildRouteGroupSourcesDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS route_group_sources (
			id SERIAL PRIMARY KEY,
			group_route_id INTEGER NOT NULL REFERENCES token_routes(id) ON DELETE CASCADE,
			source_route_id INTEGER NOT NULL REFERENCES token_routes(id) ON DELETE CASCADE,
			CONSTRAINT route_group_sources_group_source_unique UNIQUE (group_route_id, source_route_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS route_group_sources (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		group_route_id INTEGER NOT NULL,
		source_route_id INTEGER NOT NULL,
		CONSTRAINT route_group_sources_group_source_unique UNIQUE (group_route_id, source_route_id),
		FOREIGN KEY (group_route_id) REFERENCES token_routes(id) ON DELETE CASCADE,
		FOREIGN KEY (source_route_id) REFERENCES token_routes(id) ON DELETE CASCADE
	)`
}

func buildOAuthRouteUnitsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS oauth_route_units (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			name TEXT NOT NULL,
			strategy TEXT NOT NULL DEFAULT 'round_robin',
			enabled BOOLEAN DEFAULT TRUE,
			created_at TEXT,
			updated_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS oauth_route_units (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		provider TEXT NOT NULL,
		name TEXT NOT NULL,
		strategy TEXT NOT NULL DEFAULT 'round_robin',
		enabled INTEGER DEFAULT 1,
		created_at TEXT,
		updated_at TEXT,
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildOAuthRouteUnitMembersDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS oauth_route_unit_members (
			id SERIAL PRIMARY KEY,
			unit_id INTEGER NOT NULL REFERENCES oauth_route_units(id) ON DELETE CASCADE,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			sort_order INTEGER DEFAULT 0,
			success_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			total_latency_ms INTEGER DEFAULT 0,
			total_cost DOUBLE PRECISION DEFAULT 0,
			last_used_at TEXT,
			last_selected_at TEXT,
			last_fail_at TEXT,
			consecutive_fail_count INTEGER NOT NULL DEFAULT 0,
			cooldown_level INTEGER NOT NULL DEFAULT 0,
			cooldown_until TEXT,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT oauth_route_unit_members_unit_account_unique UNIQUE (unit_id, account_id),
			CONSTRAINT oauth_route_unit_members_account_unique UNIQUE (account_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS oauth_route_unit_members (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		unit_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		sort_order INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		fail_count INTEGER DEFAULT 0,
		total_latency_ms INTEGER DEFAULT 0,
		total_cost REAL DEFAULT 0,
		last_used_at TEXT,
		last_selected_at TEXT,
		last_fail_at TEXT,
		consecutive_fail_count INTEGER NOT NULL DEFAULT 0,
		cooldown_level INTEGER NOT NULL DEFAULT 0,
		cooldown_until TEXT,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT oauth_route_unit_members_unit_account_unique UNIQUE (unit_id, account_id),
		CONSTRAINT oauth_route_unit_members_account_unique UNIQUE (account_id),
		FOREIGN KEY (unit_id) REFERENCES oauth_route_units(id) ON DELETE CASCADE,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

func buildRouteChannelsDDL(d string) string {
	// CRITICAL: token_id FK uses ON DELETE SET NULL (not CASCADE).
	// oauth_route_unit_id has NO FK constraint.
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS route_channels (
			id SERIAL PRIMARY KEY,
			route_id INTEGER NOT NULL REFERENCES token_routes(id) ON DELETE CASCADE,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			token_id INTEGER REFERENCES account_tokens(id) ON DELETE SET NULL,
			oauth_route_unit_id INTEGER,
			source_model TEXT,
			priority INTEGER DEFAULT 0,
			weight INTEGER DEFAULT 10,
			enabled BOOLEAN DEFAULT TRUE,
			manual_override BOOLEAN DEFAULT FALSE,
			success_count INTEGER DEFAULT 0,
			fail_count INTEGER DEFAULT 0,
			total_latency_ms INTEGER DEFAULT 0,
			total_cost DOUBLE PRECISION DEFAULT 0,
			last_used_at TEXT,
			last_selected_at TEXT,
			last_fail_at TEXT,
			consecutive_fail_count INTEGER NOT NULL DEFAULT 0,
			cooldown_level INTEGER NOT NULL DEFAULT 0,
			cooldown_until TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS route_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		route_id INTEGER NOT NULL,
		account_id INTEGER NOT NULL,
		token_id INTEGER,
		oauth_route_unit_id INTEGER,
		source_model TEXT,
		priority INTEGER DEFAULT 0,
		weight INTEGER DEFAULT 10,
		enabled INTEGER DEFAULT 1,
		manual_override INTEGER DEFAULT 0,
		success_count INTEGER DEFAULT 0,
		fail_count INTEGER DEFAULT 0,
		total_latency_ms INTEGER DEFAULT 0,
		total_cost REAL DEFAULT 0,
		last_used_at TEXT,
		last_selected_at TEXT,
		last_fail_at TEXT,
		consecutive_fail_count INTEGER NOT NULL DEFAULT 0,
		cooldown_level INTEGER NOT NULL DEFAULT 0,
		cooldown_until TEXT,
		FOREIGN KEY (route_id) REFERENCES token_routes(id) ON DELETE CASCADE,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
		FOREIGN KEY (token_id) REFERENCES account_tokens(id) ON DELETE SET NULL
	)`
}

func buildProxyLogsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS proxy_logs (
			id SERIAL PRIMARY KEY,
			route_id INTEGER,
			channel_id INTEGER,
			account_id INTEGER,
			downstream_api_key_id INTEGER,
			model_requested TEXT,
			model_actual TEXT,
			status TEXT,
			http_status INTEGER,
			is_stream BOOLEAN,
			first_byte_latency_ms INTEGER,
			latency_ms INTEGER,
			prompt_tokens INTEGER,
			completion_tokens INTEGER,
			total_tokens INTEGER,
			estimated_cost DOUBLE PRECISION,
			billing_details TEXT,
			client_family TEXT,
			client_app_id TEXT,
			client_app_name TEXT,
			client_confidence TEXT,
			error_message TEXT,
			retry_count INTEGER DEFAULT 0,
			request_id TEXT,
			created_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS proxy_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		route_id INTEGER,
		channel_id INTEGER,
		account_id INTEGER,
		downstream_api_key_id INTEGER,
		model_requested TEXT,
		model_actual TEXT,
		status TEXT,
		http_status INTEGER,
		is_stream INTEGER,
		first_byte_latency_ms INTEGER,
		latency_ms INTEGER,
		prompt_tokens INTEGER,
		completion_tokens INTEGER,
		total_tokens INTEGER,
		estimated_cost REAL,
		billing_details TEXT,
		client_family TEXT,
		client_app_id TEXT,
		client_app_name TEXT,
		client_confidence TEXT,
		error_message TEXT,
		retry_count INTEGER DEFAULT 0,
		request_id TEXT,
		created_at TEXT
	)`
}

func buildProxyDebugTracesDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS proxy_debug_traces (
			id SERIAL PRIMARY KEY,
			downstream_path TEXT NOT NULL,
			client_kind TEXT,
			session_id TEXT,
			trace_hint TEXT,
			requested_model TEXT,
			downstream_api_key_id INTEGER,
			request_headers_json TEXT,
			request_body_json TEXT,
			sticky_session_key TEXT,
			sticky_hit_channel_id INTEGER,
			selected_channel_id INTEGER,
			selected_route_id INTEGER,
			selected_account_id INTEGER,
			selected_site_id INTEGER,
			selected_site_platform TEXT,
			endpoint_candidates_json TEXT,
			endpoint_runtime_state_json TEXT,
			decision_summary_json TEXT,
			final_status TEXT,
			final_http_status INTEGER,
			final_upstream_path TEXT,
			final_response_headers_json TEXT,
			final_response_body_json TEXT,
			created_at TEXT,
			updated_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS proxy_debug_traces (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		downstream_path TEXT NOT NULL,
		client_kind TEXT,
		session_id TEXT,
		trace_hint TEXT,
		requested_model TEXT,
		downstream_api_key_id INTEGER,
		request_headers_json TEXT,
		request_body_json TEXT,
		sticky_session_key TEXT,
		sticky_hit_channel_id INTEGER,
		selected_channel_id INTEGER,
		selected_route_id INTEGER,
		selected_account_id INTEGER,
		selected_site_id INTEGER,
		selected_site_platform TEXT,
		endpoint_candidates_json TEXT,
		endpoint_runtime_state_json TEXT,
		decision_summary_json TEXT,
		final_status TEXT,
		final_http_status INTEGER,
		final_upstream_path TEXT,
		final_response_headers_json TEXT,
		final_response_body_json TEXT,
		created_at TEXT,
		updated_at TEXT
	)`
}

func buildProxyDebugAttemptsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS proxy_debug_attempts (
			id SERIAL PRIMARY KEY,
			trace_id INTEGER NOT NULL REFERENCES proxy_debug_traces(id) ON DELETE CASCADE,
			attempt_index INTEGER NOT NULL,
			endpoint TEXT NOT NULL,
			request_path TEXT NOT NULL,
			target_url TEXT NOT NULL,
			runtime_executor TEXT,
			request_headers_json TEXT,
			request_body_json TEXT,
			response_status INTEGER,
			response_headers_json TEXT,
			response_body_json TEXT,
			raw_error_text TEXT,
			recover_applied BOOLEAN DEFAULT FALSE,
			downgrade_decision BOOLEAN DEFAULT FALSE,
			downgrade_reason TEXT,
			memory_write_json TEXT,
			created_at TEXT,
			CONSTRAINT proxy_debug_attempts_trace_attempt_unique UNIQUE (trace_id, attempt_index)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS proxy_debug_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		trace_id INTEGER NOT NULL,
		attempt_index INTEGER NOT NULL,
		endpoint TEXT NOT NULL,
		request_path TEXT NOT NULL,
		target_url TEXT NOT NULL,
		runtime_executor TEXT,
		request_headers_json TEXT,
		request_body_json TEXT,
		response_status INTEGER,
		response_headers_json TEXT,
		response_body_json TEXT,
		raw_error_text TEXT,
		recover_applied INTEGER DEFAULT 0,
		downgrade_decision INTEGER DEFAULT 0,
		downgrade_reason TEXT,
		memory_write_json TEXT,
		created_at TEXT,
		CONSTRAINT proxy_debug_attempts_trace_attempt_unique UNIQUE (trace_id, attempt_index),
		FOREIGN KEY (trace_id) REFERENCES proxy_debug_traces(id) ON DELETE CASCADE
	)`
}

func buildProxyVideoTasksDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS proxy_video_tasks (
			id SERIAL PRIMARY KEY,
			public_id TEXT NOT NULL,
			upstream_video_id TEXT NOT NULL,
			site_url TEXT NOT NULL,
			token_value TEXT NOT NULL,
			requested_model TEXT,
			actual_model TEXT,
			channel_id INTEGER,
			account_id INTEGER,
			status_snapshot TEXT,
			upstream_response_meta TEXT,
			last_upstream_status INTEGER,
			last_polled_at TEXT,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT proxy_video_tasks_public_id_unique UNIQUE (public_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS proxy_video_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		public_id TEXT NOT NULL,
		upstream_video_id TEXT NOT NULL,
		site_url TEXT NOT NULL,
		token_value TEXT NOT NULL,
		requested_model TEXT,
		actual_model TEXT,
		channel_id INTEGER,
		account_id INTEGER,
		status_snapshot TEXT,
		upstream_response_meta TEXT,
		last_upstream_status INTEGER,
		last_polled_at TEXT,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT proxy_video_tasks_public_id_unique UNIQUE (public_id)
	)`
}

func buildProxyFilesDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS proxy_files (
			id SERIAL PRIMARY KEY,
			public_id TEXT NOT NULL,
			owner_type TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			filename TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			purpose TEXT,
			byte_size INTEGER NOT NULL,
			sha256 TEXT NOT NULL,
			content_base64 TEXT NOT NULL,
			created_at TEXT,
			updated_at TEXT,
			deleted_at TEXT,
			CONSTRAINT proxy_files_public_id_unique UNIQUE (public_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS proxy_files (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		public_id TEXT NOT NULL,
		owner_type TEXT NOT NULL,
		owner_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		mime_type TEXT NOT NULL,
		purpose TEXT,
		byte_size INTEGER NOT NULL,
		sha256 TEXT NOT NULL,
		content_base64 TEXT NOT NULL,
		created_at TEXT,
		updated_at TEXT,
		deleted_at TEXT,
		CONSTRAINT proxy_files_public_id_unique UNIQUE (public_id)
	)`
}

func buildSettingsDDL(d string) string {
	return `CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT
	)`
}

func buildAdminSnapshotsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS admin_snapshots (
			id SERIAL PRIMARY KEY,
			namespace TEXT NOT NULL,
			snapshot_key TEXT NOT NULL,
			payload TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			stale_until TEXT NOT NULL,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT admin_snapshots_namespace_key_unique UNIQUE (namespace, snapshot_key)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS admin_snapshots (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		namespace TEXT NOT NULL,
		snapshot_key TEXT NOT NULL,
		payload TEXT NOT NULL,
		generated_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		stale_until TEXT NOT NULL,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT admin_snapshots_namespace_key_unique UNIQUE (namespace, snapshot_key)
	)`
}

func buildAnalyticsProjectionCheckpointsDDL(d string) string {
	return `CREATE TABLE IF NOT EXISTS analytics_projection_checkpoints (
		projector_key TEXT PRIMARY KEY,
		time_zone TEXT NOT NULL DEFAULT 'Local',
		last_proxy_log_id INTEGER NOT NULL DEFAULT 0,
		watermark_created_at TEXT,
		lease_owner TEXT,
		lease_token TEXT,
		lease_expires_at TEXT,
		recompute_from_id INTEGER,
		recompute_requested_at TEXT,
		recompute_reason TEXT,
		recompute_started_at TEXT,
		recompute_completed_at TEXT,
		last_projected_at TEXT,
		last_successful_at TEXT,
		last_error TEXT,
		created_at TEXT,
		updated_at TEXT
	)`
}

func buildSiteDayUsageDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS site_day_usage (
			id SERIAL PRIMARY KEY,
			local_day TEXT NOT NULL,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			total_calls INTEGER NOT NULL DEFAULT 0,
			success_calls INTEGER NOT NULL DEFAULT 0,
			failed_calls INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_summary_spend DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_site_spend DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			latency_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT site_day_usage_day_site_unique UNIQUE (local_day, site_id),
			CONSTRAINT site_day_usage_non_negative CHECK (
				total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
				total_tokens >= 0 AND total_summary_spend >= 0 AND total_site_spend >= 0 AND
				total_latency_ms >= 0 AND latency_count >= 0
			)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS site_day_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		local_day TEXT NOT NULL,
		site_id INTEGER NOT NULL,
		total_calls INTEGER NOT NULL DEFAULT 0,
		success_calls INTEGER NOT NULL DEFAULT 0,
		failed_calls INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		total_summary_spend REAL NOT NULL DEFAULT 0,
		total_site_spend REAL NOT NULL DEFAULT 0,
		total_latency_ms INTEGER NOT NULL DEFAULT 0,
		latency_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT site_day_usage_day_site_unique UNIQUE (local_day, site_id),
		CONSTRAINT site_day_usage_non_negative CHECK (
			total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
			total_tokens >= 0 AND total_summary_spend >= 0 AND total_site_spend >= 0 AND
			total_latency_ms >= 0 AND latency_count >= 0
		),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildSiteHourUsageDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS site_hour_usage (
			id SERIAL PRIMARY KEY,
			bucket_start_utc TEXT NOT NULL,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			total_calls INTEGER NOT NULL DEFAULT 0,
			success_calls INTEGER NOT NULL DEFAULT 0,
			failed_calls INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_summary_spend DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_site_spend DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			latency_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT site_hour_usage_hour_site_unique UNIQUE (bucket_start_utc, site_id),
			CONSTRAINT site_hour_usage_non_negative CHECK (
				total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
				total_tokens >= 0 AND total_summary_spend >= 0 AND total_site_spend >= 0 AND
				total_latency_ms >= 0 AND latency_count >= 0
			)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS site_hour_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		bucket_start_utc TEXT NOT NULL,
		site_id INTEGER NOT NULL,
		total_calls INTEGER NOT NULL DEFAULT 0,
		success_calls INTEGER NOT NULL DEFAULT 0,
		failed_calls INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		total_summary_spend REAL NOT NULL DEFAULT 0,
		total_site_spend REAL NOT NULL DEFAULT 0,
		total_latency_ms INTEGER NOT NULL DEFAULT 0,
		latency_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT site_hour_usage_hour_site_unique UNIQUE (bucket_start_utc, site_id),
		CONSTRAINT site_hour_usage_non_negative CHECK (
			total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
			total_tokens >= 0 AND total_summary_spend >= 0 AND total_site_spend >= 0 AND
			total_latency_ms >= 0 AND latency_count >= 0
		),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildModelDayUsageDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS model_day_usage (
			id SERIAL PRIMARY KEY,
			local_day TEXT NOT NULL,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			model TEXT NOT NULL,
			total_calls INTEGER NOT NULL DEFAULT 0,
			success_calls INTEGER NOT NULL DEFAULT 0,
			failed_calls INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			total_spend DOUBLE PRECISION NOT NULL DEFAULT 0,
			total_latency_ms INTEGER NOT NULL DEFAULT 0,
			latency_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT model_day_usage_day_site_model_unique UNIQUE (local_day, site_id, model),
			CONSTRAINT model_day_usage_non_negative CHECK (
				total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
				total_tokens >= 0 AND total_spend >= 0 AND
				total_latency_ms >= 0 AND latency_count >= 0
			)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS model_day_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		local_day TEXT NOT NULL,
		site_id INTEGER NOT NULL,
		model TEXT NOT NULL,
		total_calls INTEGER NOT NULL DEFAULT 0,
		success_calls INTEGER NOT NULL DEFAULT 0,
		failed_calls INTEGER NOT NULL DEFAULT 0,
		total_tokens INTEGER NOT NULL DEFAULT 0,
		total_spend REAL NOT NULL DEFAULT 0,
		total_latency_ms INTEGER NOT NULL DEFAULT 0,
		latency_count INTEGER NOT NULL DEFAULT 0,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT model_day_usage_day_site_model_unique UNIQUE (local_day, site_id, model),
		CONSTRAINT model_day_usage_non_negative CHECK (
			total_calls >= 0 AND success_calls >= 0 AND failed_calls >= 0 AND
			total_tokens >= 0 AND total_spend >= 0 AND
			total_latency_ms >= 0 AND latency_count >= 0
		),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildDownstreamAPIKeysDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS downstream_api_keys (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			key TEXT NOT NULL,
			description TEXT,
			group_name TEXT,
			tags TEXT,
			enabled BOOLEAN DEFAULT TRUE,
			expires_at TEXT,
			max_cost DOUBLE PRECISION,
			used_cost DOUBLE PRECISION DEFAULT 0,
			max_requests INTEGER,
			used_requests INTEGER DEFAULT 0,
			supported_models TEXT,
			allowed_route_ids TEXT,
			site_weight_multipliers TEXT,
			excluded_site_ids TEXT,
			excluded_credential_refs TEXT,
			allowed_site_ids TEXT,
			allowed_credential_refs TEXT,
			key_weight DOUBLE PRECISION,
			proxy_url TEXT,
			max_rpm INTEGER,
			max_tpm INTEGER,
			ip_allowlist TEXT,
			ip_blocklist TEXT,
			last_used_at TEXT,
			created_at TEXT,
			updated_at TEXT,
			CONSTRAINT downstream_api_keys_key_unique UNIQUE (key)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS downstream_api_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		key TEXT NOT NULL,
		description TEXT,
		group_name TEXT,
		tags TEXT,
		enabled INTEGER DEFAULT 1,
		expires_at TEXT,
		max_cost REAL,
		used_cost REAL DEFAULT 0,
		max_requests INTEGER,
		used_requests INTEGER DEFAULT 0,
		supported_models TEXT,
		allowed_route_ids TEXT,
		site_weight_multipliers TEXT,
		excluded_site_ids TEXT,
		excluded_credential_refs TEXT,
		allowed_site_ids TEXT,
		allowed_credential_refs TEXT,
		key_weight REAL,
		proxy_url TEXT,
		max_rpm INTEGER,
		max_tpm INTEGER,
		ip_allowlist TEXT,
		ip_blocklist TEXT,
		last_used_at TEXT,
		created_at TEXT,
		updated_at TEXT,
		CONSTRAINT downstream_api_keys_key_unique UNIQUE (key)
	)`
}

func buildSiteAnnouncementsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS site_announcements (
			id SERIAL PRIMARY KEY,
			site_id INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
			platform TEXT NOT NULL,
			source_key TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			level TEXT NOT NULL DEFAULT 'info',
			source_url TEXT,
			starts_at TEXT,
			ends_at TEXT,
			upstream_created_at TEXT,
			upstream_updated_at TEXT,
			first_seen_at TEXT,
			last_seen_at TEXT,
			read_at TEXT,
			dismissed_at TEXT,
			raw_payload TEXT,
			CONSTRAINT site_announcements_site_source_key_unique UNIQUE (site_id, source_key)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS site_announcements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		site_id INTEGER NOT NULL,
		platform TEXT NOT NULL,
		source_key TEXT NOT NULL,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		level TEXT NOT NULL DEFAULT 'info',
		source_url TEXT,
		starts_at TEXT,
		ends_at TEXT,
		upstream_created_at TEXT,
		upstream_updated_at TEXT,
		first_seen_at TEXT,
		last_seen_at TEXT,
		read_at TEXT,
		dismissed_at TEXT,
		raw_payload TEXT,
		CONSTRAINT site_announcements_site_source_key_unique UNIQUE (site_id, source_key),
		FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE
	)`
}

func buildAdminBackgroundTasksDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS admin_background_tasks (
			id SERIAL PRIMARY KEY,
			task_id TEXT NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			message TEXT,
			error TEXT,
			result_json TEXT,
			dedupe_key TEXT,
			created_at TEXT,
			updated_at TEXT,
			started_at TEXT,
			finished_at TEXT,
			logs_json TEXT,
			CONSTRAINT admin_background_tasks_task_id_unique UNIQUE (task_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS admin_background_tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		message TEXT,
		error TEXT,
		result_json TEXT,
		dedupe_key TEXT,
		created_at TEXT,
		updated_at TEXT,
		started_at TEXT,
		finished_at TEXT,
		logs_json TEXT,
		CONSTRAINT admin_background_tasks_task_id_unique UNIQUE (task_id)
	)`
}

// buildBalanceHistoryDDL creates the balance_history table.
// One row per account per UTC day, UPSERTed by RefreshBalance on success.
// UNIQUE (local_day, account_id) lets a same-day re-refresh overwrite the
// earlier snapshot so the trend shows the latest-known balance of the day,
// not a noisy multi-point curve.
func buildBalanceHistoryDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS balance_history (
			id SERIAL PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			balance DOUBLE PRECISION NOT NULL DEFAULT 0,
			balance_used DOUBLE PRECISION NOT NULL DEFAULT 0,
			quota DOUBLE PRECISION NOT NULL DEFAULT 0,
			local_day TEXT NOT NULL,
			captured_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			CONSTRAINT balance_history_day_account_unique UNIQUE (local_day, account_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS balance_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		balance REAL NOT NULL DEFAULT 0,
		balance_used REAL NOT NULL DEFAULT 0,
		quota REAL NOT NULL DEFAULT 0,
		local_day TEXT NOT NULL,
		captured_at TEXT NOT NULL,
		created_at TEXT NOT NULL,
		CONSTRAINT balance_history_day_account_unique UNIQUE (local_day, account_id),
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

// buildModelVerifyHistoryDDL creates the model_verify_history table
// . One row per model/channel probe result from an
// operator-initiated batch verification pass (POST /api/models/verify-batch).
// batch_id groups one operator action; status is success | failure |
// inconclusive | skipped (same vocabulary as the background model probe).
func buildModelVerifyHistoryDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS model_verify_history (
			id SERIAL PRIMARY KEY,
			batch_id TEXT NOT NULL,
			model_name TEXT NOT NULL,
			channel_id INTEGER,
			account_id INTEGER REFERENCES accounts(id) ON DELETE CASCADE,
			site_id INTEGER,
			status TEXT NOT NULL,
			latency_ms DOUBLE PRECISION,
			http_status INTEGER,
			error_text TEXT,
			created_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS model_verify_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		batch_id TEXT NOT NULL,
		model_name TEXT NOT NULL,
		channel_id INTEGER,
		account_id INTEGER,
		site_id INTEGER,
		status TEXT NOT NULL,
		latency_ms REAL,
		http_status INTEGER,
		error_text TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

// buildModelProbeResultsDDL creates the model_probe_results table.
// One row per background model-probe result (scheduler/model_probe.go pass),
// used as the connectivity signal for the route-rebuild probe filter (#625).
// status shares the probe vocabulary: success | failure | inconclusive | skipped.
func buildModelProbeResultsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS model_probe_results (
			id SERIAL PRIMARY KEY,
			channel_id INTEGER,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			site_id INTEGER NOT NULL,
			model_name TEXT NOT NULL,
			status TEXT NOT NULL,
			latency_ms DOUBLE PRECISION,
			http_status INTEGER,
			error_text TEXT,
			created_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS model_probe_results (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel_id INTEGER,
		account_id INTEGER NOT NULL,
		site_id INTEGER NOT NULL,
		model_name TEXT NOT NULL,
		status TEXT NOT NULL,
		latency_ms REAL,
		http_status INTEGER,
		error_text TEXT,
		created_at TEXT NOT NULL,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

// buildProductAnnouncementsDDL creates the product_announcements table
// . Operator-authored severity-ranked banners shown on
// the Dashboard. Content edits (PUT) reset any dismissal so a new revision is
// seen again (dismiss-revision semantics).
func buildProductAnnouncementsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS product_announcements (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			severity TEXT NOT NULL DEFAULT 'info',
			link TEXT,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS product_announcements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		message TEXT NOT NULL,
		severity TEXT NOT NULL DEFAULT 'info',
		link TEXT,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	)`
}

// buildAnnouncementDismissalsDDL creates the announcement_dismissals table
// . One row per dismissed announcement; content
// revisions delete the row so the new revision surfaces again.
func buildAnnouncementDismissalsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS announcement_dismissals (
			announcement_id INTEGER PRIMARY KEY REFERENCES product_announcements(id) ON DELETE CASCADE,
			dismissed_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS announcement_dismissals (
		announcement_id INTEGER PRIMARY KEY,
		dismissed_at TEXT NOT NULL,
		FOREIGN KEY (announcement_id) REFERENCES product_announcements(id) ON DELETE CASCADE
	)`
}

// buildModelNameRedirectsDDL creates the model_name_redirects table
// . Maps a canonical route model name to the actual
// upstream name per account (e.g. claude-3-5-sonnet → claude-3-5-sonnet-20241022).
// source is sync (auto-generated) or manual (operator-authored, never
// overwritten by sync generation).
func buildModelNameRedirectsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS model_name_redirects (
			id SERIAL PRIMARY KEY,
			account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
			canonical TEXT NOT NULL,
			actual TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT 'sync',
			last_seen_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			CONSTRAINT model_name_redirects_account_canonical_unique UNIQUE (account_id, canonical)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS model_name_redirects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		canonical TEXT NOT NULL,
		actual TEXT NOT NULL,
		source TEXT NOT NULL DEFAULT 'sync',
		last_seen_at TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		CONSTRAINT model_name_redirects_account_canonical_unique UNIQUE (account_id, canonical),
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	)`
}

func buildEventsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS events (
			id SERIAL PRIMARY KEY,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT,
			level TEXT NOT NULL DEFAULT 'info',
			read BOOLEAN DEFAULT FALSE,
			related_id INTEGER,
			related_type TEXT,
			created_at TEXT
		)`
	}
	return `CREATE TABLE IF NOT EXISTS events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		title TEXT NOT NULL,
		message TEXT,
		level TEXT NOT NULL DEFAULT 'info',
		read INTEGER DEFAULT 0,
		related_id INTEGER,
		related_type TEXT,
		created_at TEXT
	)`
}

// buildAdminAuditLogsDDL creates the admin_audit_logs table
// . Records authenticated admin write
// operations (POST/PUT/PATCH/DELETE) for traceability and compliance.
// actor is a sha256 prefix of the admin bearer token — never the raw token.
func buildAdminAuditLogsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS admin_audit_logs (
			id BIGSERIAL PRIMARY KEY,
			actor TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			status INTEGER NOT NULL DEFAULT 0,
			request_id TEXT,
			remote_ip TEXT,
			created_at TEXT NOT NULL
		)`
	}
	return `CREATE TABLE IF NOT EXISTS admin_audit_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		actor TEXT NOT NULL,
		method TEXT NOT NULL,
		path TEXT NOT NULL,
		status INTEGER NOT NULL DEFAULT 0,
		request_id TEXT,
		remote_ip TEXT,
		created_at TEXT NOT NULL
	)`
}

