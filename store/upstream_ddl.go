package store

// These tables own direct-upstream configuration imported from providers whose
// channel identity and per-credential protocol grants do not fit sites/accounts.
// origin_key separates independent source installations; source_id mappings
// keep foreign keys correct when source primary keys collide locally.
func buildUpstreamChannelsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_channels (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
			name TEXT NOT NULL, dialect TEXT NOT NULL, enabled BOOLEAN NOT NULL,
			base_url TEXT NOT NULL, endpoint_config TEXT NOT NULL DEFAULT '{}', openai_chat_completion_path TEXT NOT NULL,
			openai_response_path TEXT NOT NULL, anthropic_message_path TEXT NOT NULL,
			proxy BOOLEAN NOT NULL, channel_proxy TEXT NOT NULL, custom_header TEXT NOT NULL,
			param_override TEXT NOT NULL, match_regex TEXT NOT NULL,
			UNIQUE(origin_key, source_id), UNIQUE(origin_key, name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_channels (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
		name TEXT NOT NULL, dialect TEXT NOT NULL, enabled INTEGER NOT NULL,
		base_url TEXT NOT NULL, endpoint_config TEXT NOT NULL DEFAULT '{}', openai_chat_completion_path TEXT NOT NULL,
		openai_response_path TEXT NOT NULL, anthropic_message_path TEXT NOT NULL,
		proxy INTEGER NOT NULL, channel_proxy TEXT NOT NULL, custom_header TEXT NOT NULL,
		param_override TEXT NOT NULL, match_regex TEXT NOT NULL,
		UNIQUE(origin_key, source_id), UNIQUE(origin_key, name)
	)`
}

func buildUpstreamCredentialsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_credentials (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL,
			channel_id INTEGER NOT NULL REFERENCES upstream_channels(id) ON DELETE CASCADE,
			source_id BIGINT NOT NULL, name TEXT NOT NULL, secret TEXT NOT NULL, enabled BOOLEAN NOT NULL,
			UNIQUE(origin_key, source_id), UNIQUE(channel_id, name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_credentials (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, channel_id INTEGER NOT NULL,
		source_id BIGINT NOT NULL, name TEXT NOT NULL, secret TEXT NOT NULL, enabled INTEGER NOT NULL,
		UNIQUE(origin_key, source_id), UNIQUE(channel_id, name),
		FOREIGN KEY (channel_id) REFERENCES upstream_channels(id) ON DELETE CASCADE
	)`
}

func buildUpstreamModelsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_models (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL, channel_id INTEGER NOT NULL REFERENCES upstream_channels(id) ON DELETE CASCADE,
			source_id BIGINT NOT NULL, name TEXT NOT NULL, enabled BOOLEAN NOT NULL,
			UNIQUE(origin_key, source_id), UNIQUE(channel_id, name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_models (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, channel_id INTEGER NOT NULL,
		source_id BIGINT NOT NULL, name TEXT NOT NULL, enabled INTEGER NOT NULL,
		UNIQUE(origin_key, source_id), UNIQUE(channel_id, name),
		FOREIGN KEY (channel_id) REFERENCES upstream_channels(id) ON DELETE CASCADE
	)`
}

func buildUpstreamGrantsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_grants (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
			model_id INTEGER NOT NULL REFERENCES upstream_models(id) ON DELETE CASCADE,
			credential_id INTEGER NOT NULL REFERENCES upstream_credentials(id) ON DELETE CASCADE,
			protocols INTEGER NOT NULL, enabled BOOLEAN NOT NULL,
      success_count BIGINT DEFAULT 0 NOT NULL, fail_count BIGINT DEFAULT 0 NOT NULL,
      total_latency_ms BIGINT DEFAULT 0 NOT NULL, total_cost DOUBLE PRECISION DEFAULT 0 NOT NULL,
      cooldown_until TEXT, cooldown_reason_code TEXT, last_used_at TEXT, last_fail_at TEXT,
			UNIQUE(origin_key, source_id), UNIQUE(model_id, credential_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_grants (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
		model_id INTEGER NOT NULL, credential_id INTEGER NOT NULL,
		protocols INTEGER NOT NULL, enabled INTEGER NOT NULL,
    success_count INTEGER DEFAULT 0 NOT NULL, fail_count INTEGER DEFAULT 0 NOT NULL,
    total_latency_ms INTEGER DEFAULT 0 NOT NULL, total_cost REAL DEFAULT 0 NOT NULL,
    cooldown_until TEXT, cooldown_reason_code TEXT, last_used_at TEXT, last_fail_at TEXT,
		UNIQUE(origin_key, source_id), UNIQUE(model_id, credential_id),
		FOREIGN KEY (model_id) REFERENCES upstream_models(id) ON DELETE CASCADE,
		FOREIGN KEY (credential_id) REFERENCES upstream_credentials(id) ON DELETE CASCADE
	)`
}

func buildUpstreamGroupsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_groups (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
			name TEXT NOT NULL, mode TEXT NOT NULL, active_item_id BIGINT NOT NULL,
			relay_config TEXT NOT NULL, enabled BOOLEAN NOT NULL,
			UNIQUE(origin_key, source_id), UNIQUE(origin_key, name)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_groups (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, source_id BIGINT NOT NULL,
		name TEXT NOT NULL, mode TEXT NOT NULL, active_item_id BIGINT NOT NULL,
		relay_config TEXT NOT NULL, enabled INTEGER NOT NULL,
		UNIQUE(origin_key, source_id), UNIQUE(origin_key, name)
	)`
}

func buildUpstreamGroupItemsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_group_items (
			id SERIAL PRIMARY KEY, origin_key TEXT NOT NULL, group_id INTEGER NOT NULL REFERENCES upstream_groups(id) ON DELETE CASCADE,
			source_id BIGINT NOT NULL, grant_id INTEGER NOT NULL REFERENCES upstream_grants(id) ON DELETE CASCADE,
			priority INTEGER NOT NULL, weight INTEGER NOT NULL,
			UNIQUE(origin_key, source_id), UNIQUE(group_id, grant_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_group_items (
		id INTEGER PRIMARY KEY AUTOINCREMENT, origin_key TEXT NOT NULL, group_id INTEGER NOT NULL,
		source_id BIGINT NOT NULL, grant_id INTEGER NOT NULL,
		priority INTEGER NOT NULL, weight INTEGER NOT NULL,
		UNIQUE(origin_key, source_id), UNIQUE(group_id, grant_id),
		FOREIGN KEY (group_id) REFERENCES upstream_groups(id) ON DELETE CASCADE,
		FOREIGN KEY (grant_id) REFERENCES upstream_grants(id) ON DELETE CASCADE
	)`
}

func buildExternalSourceIDsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS external_source_ids (
			origin_key TEXT NOT NULL, entity_type TEXT NOT NULL, source_id BIGINT NOT NULL,
			target_id INTEGER NOT NULL, PRIMARY KEY(origin_key, entity_type, source_id),
			UNIQUE(origin_key, entity_type, target_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS external_source_ids (
		origin_key TEXT NOT NULL, entity_type TEXT NOT NULL, source_id BIGINT NOT NULL,
		target_id INTEGER NOT NULL, PRIMARY KEY(origin_key, entity_type, source_id),
		UNIQUE(origin_key, entity_type, target_id)
	)`
}

func buildUpstreamRouteGroupsDDL(d string) string {
	if isPG(d) {
		return `CREATE TABLE IF NOT EXISTS upstream_route_groups (
			route_id INTEGER PRIMARY KEY REFERENCES token_routes(id) ON DELETE CASCADE,
			group_id INTEGER NOT NULL REFERENCES upstream_groups(id) ON DELETE CASCADE,
			UNIQUE(group_id)
		)`
	}
	return `CREATE TABLE IF NOT EXISTS upstream_route_groups (
		route_id INTEGER PRIMARY KEY,
		group_id INTEGER NOT NULL UNIQUE,
		FOREIGN KEY (route_id) REFERENCES token_routes(id) ON DELETE CASCADE,
		FOREIGN KEY (group_id) REFERENCES upstream_groups(id) ON DELETE CASCADE
	)`
}

func buildUpstreamImportStatsDDL(d string) string {
	return `CREATE TABLE IF NOT EXISTS upstream_import_stats (
		origin_key TEXT NOT NULL, section TEXT NOT NULL, record_key TEXT NOT NULL,
		data_json TEXT NOT NULL, PRIMARY KEY(origin_key, section, record_key)
	)`
}
