package store

import (
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// AdditiveStep is one forward-only schema change applied after the base
// CREATE TABLE IF NOT EXISTS bootstrap. Steps must be additive (new tables,
// columns, indexes) and safe to re-attempt if bookkeeping is incomplete.

// Version IDs are stable strings (not integers) so SC2+ can insert descriptive
// IDs such as "sc2_001_downstream_proxy_url" without renumbering history.
type AdditiveStep struct {
	// Version is the primary key stored in schema_migrations.
	Version string
	// Description is a short human-readable note for operators and logs.
	Description string
	// Apply performs the DDL/DML for this step. It should be idempotent where
	// practical (e.g. EnsureColumn) so a crash between DDL and bookkeeping
	// does not break the next startup.
	Apply func(db *DB) error
}

// enterpriseAdditiveSteps is the ordered registry of production additive
// upgrades. SC2 registers the enterprise columns documented in
// §5. Fresh installs also get these columns
// from base CREATE TABLE builders; EnsureColumn keeps old installs converging.

// Keep this list append-only. Never edit or remove a shipped Version string.
var enterpriseAdditiveSteps = []AdditiveStep{
	{
		Version:     "sc2_001_downstream_proxy_url",
		Description: "downstream_api_keys.proxy_url TEXT NULL — per-key egress proxy; NULL falls back to site/system",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "downstream_api_keys", "proxy_url", "TEXT", "TEXT", "")
		},
	},
	{
		Version:     "sc2_002_site_max_concurrency",
		Description: "sites.max_concurrency INTEGER DEFAULT 0 — 0 means unlimited (legacy behavior)",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "sites", "max_concurrency", "INTEGER", "INTEGER", "DEFAULT 0")
		},
	},
	{
		// SC0 Option A: route-level context window metadata. No model_catalog
		// table exists; token_routes is the clear column home until a catalog lands.
		Version:     "sc2_003_route_context_length",
		Description: "token_routes.context_length INTEGER NULL — unknown/no enforcement when NULL",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "token_routes", "context_length", "INTEGER", "INTEGER", "")
		},
	},
	{
		// end-to-end request/trace ids across channel retries.
		Version:     "sc2_004_proxy_logs_request_id",
		Description: "proxy_logs.request_id TEXT NULL — ingress X-Request-Id shared across retry/failover attempts",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "proxy_logs", "request_id", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureIndex(db, "proxy_logs_request_id_created_at_idx",
				`CREATE INDEX IF NOT EXISTS proxy_logs_request_id_created_at_idx ON proxy_logs (request_id, created_at)`)
		},
	},
	{
		// downstream-key RPM/TPM soft admission fields.
		Version:     "sc2_005_downstream_key_rate_limits",
		Description: "downstream_api_keys.max_rpm / max_tpm INTEGER NULL - optional per-key rate windows; NULL means unlimited",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "downstream_api_keys", "max_rpm", "INTEGER", "INTEGER", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "downstream_api_keys", "max_tpm", "INTEGER", "INTEGER", "")
		},
	},
	{
		// (upstream issue 590 / 284): admin drag reorder of token routes (list order).
		Version:     "sc2_006_token_routes_sort_order",
		Description: "token_routes.sort_order INTEGER DEFAULT 0 - list/admin drag reorder; lower first, then id",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "token_routes", "sort_order", "INTEGER", "INTEGER", "DEFAULT 0"); err != nil {
				return err
			}
			return EnsureIndex(db, "token_routes_sort_order_id_idx",
				`CREATE INDEX IF NOT EXISTS token_routes_sort_order_id_idx ON token_routes (sort_order, id)`)
		},
	},
	{
		// (upstream issue 547): per-downstream-key weight scalar for weighted LB.
		Version:     "sc2_007_downstream_key_weight",
		Description: "downstream_api_keys.key_weight REAL/DOUBLE NULL — optional per-key weight; NULL means 1.0",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "downstream_api_keys", "key_weight", "REAL", "DOUBLE PRECISION", "")
		},
	},
	{
		// (upstream issue 584): site custom header override priority (site-wins vs request-wins).
		Version:     "sc2_008_site_custom_headers_override_request_headers",
		Description: "sites.custom_headers_override_request_headers BOOL DEFAULT FALSE - when true site custom headers overwrite same-name request headers",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "sites", "custom_headers_override_request_headers", "INTEGER", "BOOLEAN", "DEFAULT FALSE")
		},
	},
	{
		// (upstream issue 579): allow-list bind sites / credentials on one downstream key.
		// Empty JSON/NULL = no allow-list restriction (legacy exclude-only behavior).
		Version:     "sc2_009_downstream_key_allow_lists",
		Description: "downstream_api_keys.allowed_site_ids / allowed_credential_refs TEXT NULL — optional allow-lists; empty means unrestricted",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "downstream_api_keys", "allowed_site_ids", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "downstream_api_keys", "allowed_credential_refs", "TEXT", "TEXT", "")
		},
	},
	{
		// N1 security: per-downstream-key IP allowlist/blocklist.
		// ip_allowlist: newline/comma-separated CIDR or exact IPs; empty = unrestricted.
		// ip_blocklist: same format; deny first (blocklist wins over allowlist).
		// Enforced at the ProxyAuth edge via auth.parseAllowlist/isIPAllowed.
		Version:     "sc2_010_downstream_key_ip_lists",
		Description: "downstream_api_keys.ip_allowlist / ip_blocklist TEXT NULL — per-key IP allow/block; empty means unrestricted",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "downstream_api_keys", "ip_allowlist", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "downstream_api_keys", "ip_blocklist", "TEXT", "TEXT", "")
		},
	},
	{
		// I1: accounts/sites global tag system.
		// tags holds a JSON array text like ["prod","priority"]; NULL = none.
		// Filtering is done client-side on the full snapshot; the column is
		// for storage + the /api/tags aggregation endpoint.
		Version:     "sc2_011_account_site_tags",
		Description: "accounts.tags / sites.tags TEXT NULL — JSON array of operator labels for classification and filtering",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "accounts", "tags", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "sites", "tags", "TEXT", "TEXT", "")
		},
	},
	{
		// 13-report §4 / plan.md §5.5.5: wire the existing ClassifyFailureReason
		// (service/checkin/failure_reason.go) into the production checkin path.
		// Stores the serialized FailureReason JSON so the UI can render the
		// structured "分类" column instead of a phantom always-`-` value.
		// NULL = success or unclassified (forward-compatible with old rows).
		Version:     "sc2_012_checkin_logs_failure_reason",
		Description: "checkin_logs.failure_reason TEXT NULL — structured failure classification JSON (ClassifyFailureReason output); NULL = success or unclassified",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "checkin_logs", "failure_reason", "TEXT", "TEXT", "")
		},
	},
	{
		// Per-site anti-bot identity: browser UA override + Cloudflare
		// clearance cookie. cf_clearance is injected via a dedicated typed
		// field (not custom_headers, where Cookie is deny-listed).
		Version:     "sc2_013_site_cf_clearance_browser_ua",
		Description: "sites.browser_ua / sites.cf_clearance TEXT NULL — per-site browser User-Agent override and Cloudflare clearance cookie",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "sites", "browser_ua", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "sites", "cf_clearance", "TEXT", "TEXT", "")
		},
	},
	{
		// Tier 2 (#678): per-site Resin override. NULL = inherit global
		// RESIN_ENABLED flag; true/false = explicit per-site opt-in/opt-out.
		// This is observability-only on the storage side; service.ResinEnabled
		// resolves site-level precedence at request time.
		Version:     "sc2_014_site_resin_enabled",
		Description: "sites.resin_enabled BOOLEAN NULL — per-site Resin sticky-proxy override; NULL inherits global RESIN_ENABLED",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "sites", "resin_enabled", "INTEGER", "BOOLEAN", "")
		},
	},
	{
		// Tier 1 (#672): per-site uTLS TLS fingerprint masking override.
		// NULL = inherit global UTLS_ENABLED flag; true/false = explicit
		// per-site opt-in/opt-out. Uses INTEGER/BOOLEAN (not TEXT) so the
		// *bool struct field scans correctly via database/sql, matching the
		// resin_enabled pattern.
		Version:     "sc2_015_site_use_utls",
		Description: "sites.use_utls BOOLEAN NULL — per-site uTLS Chrome-ClientHello fingerprint masking override; NULL inherits global UTLS_ENABLED",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "sites", "use_utls", "INTEGER", "BOOLEAN", "")
		},
	},
	{
		// #804: accounts.remark — free-form human-readable note (account
		// source, expiry, donor). Complements the machine-readable tags
		// column. NULL means no remark. Surfaced via PUT /api/accounts/{id}.
		Version:     "sc2_016_account_remark",
		Description: "accounts.remark TEXT NULL — free-form human-readable note; complements tags; NULL means no remark",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "accounts", "remark", "TEXT", "TEXT", "")
		},
	},
	{
		// TS-heritage sites columns: columns added by TypeScript drizzle
		// migrations (0008 backfill, 0025/0026 probe) that a TS-era database
		// may predate. AutoMigrate's CREATE TABLE IF NOT EXISTS never mutates
		// an existing table, so every column the Go code SELECTs must be
		// converged here or a legacy TS hub.db crashes on startup with
		// "no such column".
		Version:     "sc2_017_ts_legacy_sites_columns",
		Description: "sites TS-heritage columns — proxy_url, use_system_proxy, custom_headers, external_checkin_url, global_weight",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "sites", "proxy_url", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "sites", "use_system_proxy", "INTEGER", "BOOLEAN", "DEFAULT FALSE"); err != nil {
				return err
			}
			if err := EnsureColumn(db, "sites", "custom_headers", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "sites", "external_checkin_url", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "sites", "global_weight", "REAL", "DOUBLE PRECISION", "DEFAULT 1")
		},
	},
	{
		Version:     "sc2_018_site_post_refresh_probe",
		Description: "sites post-refresh probe columns (TS 0025/0026) — post_refresh_probe_enabled / _model / _scope / _latency_threshold_ms",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "sites", "post_refresh_probe_enabled", "INTEGER", "BOOLEAN", "DEFAULT FALSE"); err != nil {
				return err
			}
			if err := EnsureColumn(db, "sites", "post_refresh_probe_model", "TEXT", "TEXT", "DEFAULT ''"); err != nil {
				return err
			}
			if err := EnsureColumn(db, "sites", "post_refresh_probe_scope", "TEXT", "TEXT", "DEFAULT 'single'"); err != nil {
				return err
			}
			return EnsureColumn(db, "sites", "post_refresh_probe_latency_threshold_ms", "INTEGER", "INTEGER", "DEFAULT 0")
		},
	},
	{
		Version:     "sc2_019_ts_legacy_token_routes_columns",
		Description: "token_routes TS-heritage columns (0008 backfill, 0014) — display_name, display_icon, decision_snapshot, decision_refreshed_at, route_mode, routing_strategy",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "token_routes", "display_name", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "token_routes", "display_icon", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "token_routes", "decision_snapshot", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "token_routes", "decision_refreshed_at", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "token_routes", "route_mode", "TEXT", "TEXT", "DEFAULT 'pattern'"); err != nil {
				return err
			}
			return EnsureColumn(db, "token_routes", "routing_strategy", "TEXT", "TEXT", "DEFAULT 'weighted'")
		},
	},
	{
		Version:     "sc2_020_ts_legacy_route_channels_columns",
		Description: "route_channels TS-heritage columns (0008 backfill, 0014, 0021) — oauth_route_unit_id, source_model, last_selected_at, consecutive_fail_count, cooldown_level",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "route_channels", "oauth_route_unit_id", "INTEGER", "INTEGER", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "route_channels", "source_model", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "route_channels", "last_selected_at", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "route_channels", "consecutive_fail_count", "INTEGER", "INTEGER", "NOT NULL DEFAULT 0"); err != nil {
				return err
			}
			return EnsureColumn(db, "route_channels", "cooldown_level", "INTEGER", "INTEGER", "NOT NULL DEFAULT 0")
		},
	},
	{
		Version:     "sc2_021_ts_legacy_proxy_logs_columns",
		Description: "proxy_logs TS-heritage columns (0005, 0010, 0016, 0019) — billing_details, downstream_api_key_id, client_family, client_app_id, client_app_name, client_confidence, is_stream, first_byte_latency_ms",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "proxy_logs", "billing_details", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "downstream_api_key_id", "INTEGER", "INTEGER", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "client_family", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "client_app_id", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "client_app_name", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "client_confidence", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "proxy_logs", "is_stream", "INTEGER", "BOOLEAN", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "proxy_logs", "first_byte_latency_ms", "INTEGER", "INTEGER", "")
		},
	},
	{
		Version:     "sc2_022_ts_legacy_account_oauth_columns",
		Description: "accounts TS-heritage OAuth columns (0013) — oauth_provider, oauth_account_key, oauth_project_id",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "accounts", "oauth_provider", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			if err := EnsureColumn(db, "accounts", "oauth_account_key", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "accounts", "oauth_project_id", "TEXT", "TEXT", "")
		},
	},
	{
		Version:     "sc2_023_ts_legacy_account_tokens_columns",
		Description: "account_tokens TS-heritage columns (0007, 0012) — token_group, value_status",
		Apply: func(db *DB) error {
			if err := EnsureColumn(db, "account_tokens", "token_group", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "account_tokens", "value_status", "TEXT", "TEXT", "DEFAULT 'ready' NOT NULL")
		},
	},
	{
		Version:     "sc2_024_model_availability_is_manual",
		Description: "model_availability.is_manual (TS 0009) — manual availability override flag",
		Apply: func(db *DB) error {
			return EnsureColumn(db, "model_availability", "is_manual", "INTEGER", "BOOLEAN", "DEFAULT FALSE")
		},
	},
	{
		// P0-3: structured cooldown/breaker reasons. Answers "why is this channel
		// cooling" without log archaeology: classification code + truncated error
		// summary + recorded-at. NULL on rows cooled before this step ran — the UI
		// reports that honestly instead of guessing.
		Version:     "sc2_025_cooldown_reasons",
		Description: "route_channels/oauth_route_unit_members cooldown_reason_code/cooldown_reason/cooldown_reason_at TEXT NULL — structured cooldown reason; NULL = not recorded (legacy rows)",
		Apply: func(db *DB) error {
			for _, table := range []string{"route_channels", "oauth_route_unit_members"} {
				if err := EnsureColumn(db, table, "cooldown_reason_code", "TEXT", "TEXT", ""); err != nil {
					return err
				}
				if err := EnsureColumn(db, table, "cooldown_reason", "TEXT", "TEXT", ""); err != nil {
					return err
				}
				if err := EnsureColumn(db, table, "cooldown_reason_at", "TEXT", "TEXT", ""); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// #1034 session model: server-side admin UI sessions. New table (not
		// column convergence), so CREATE TABLE IF NOT EXISTS is the idempotent
		// form. The raw cookie token is never stored -- only its SHA-256 hash.
		// expires_at is a fixed-precision RFC3339-UTC string so the expiry
		// sweep can compare lexicographically in both dialects.
		Version:     "sc2_026_admin_sessions",
		Description: "admin_sessions table -- server-side admin UI sessions (token_hash PK, sliding expires_at); #1034 session model",
		Apply: func(db *DB) error {
			if _, err := db.Exec(buildAdminSessionsDDL(db.Dialect)); err != nil {
				return err
			}
			return EnsureIndex(db, "admin_sessions_expires_at_idx",
				`CREATE INDEX IF NOT EXISTS admin_sessions_expires_at_idx ON admin_sessions (expires_at)`)
		},
	},
	{
		// Wave 18 index audit (admin high-frequency read paths). Pure additive
		// indexes on base-schema columns, measured on a 300k-proxy_logs audit
		// fixture before adoption (SQLite EXPLAIN QUERY PLAN + timing):
		//   - proxy_logs_channel_id_created_at_idx: the proxy-logs page
		//     "channelId" filter ("why is this channel failing") degraded from
		//     an ORDER BY created_at scan-until-match into a direct SEARCH
		//     (~18ms median / >1s tail on sparse channels -> <1ms).
		//   - proxy_logs_created_at_covering_summary_idx: covering index for
		//     the range-filtered proxy-logs summary aggregate (COUNT + five
		//     SUMs); created_at leads so range filters SEARCH, and the
		//     aggregate columns are satisfied from the index alone instead of
		//     heap lookups (~3x faster at 300k rows).
		//   - checkin_logs_status_created_at_idx: the checkin-history status
		//     filter previously matched on status then re-sorted all matches
		//     by created_at (temp B-tree); the composite index serves the
		//     ORDER BY ... DESC LIMIT page directly.
		// Fresh installs also receive these via this step (AutoMigrate runs
		// the additive registry after buildIndexes); EnsureIndex uses CREATE
		// INDEX IF NOT EXISTS so re-runs are no-ops on both dialects.
		Version:     "sc2_027_admin_read_path_indexes",
		Description: "indexes for admin read paths: proxy_logs (channel_id, created_at), proxy_logs covering (created_at, status, estimated_cost, total_tokens, prompt_tokens, completion_tokens), checkin_logs (status, created_at)",
		Apply: func(db *DB) error {
			for _, idx := range []struct {
				name string
				sql  string
			}{
				{"proxy_logs_channel_id_created_at_idx",
					`CREATE INDEX IF NOT EXISTS proxy_logs_channel_id_created_at_idx ON proxy_logs (channel_id, created_at)`},
				{"proxy_logs_created_at_covering_summary_idx",
					`CREATE INDEX IF NOT EXISTS proxy_logs_created_at_covering_summary_idx ON proxy_logs (created_at, status, estimated_cost, total_tokens, prompt_tokens, completion_tokens)`},
				{"checkin_logs_status_created_at_idx",
					`CREATE INDEX IF NOT EXISTS checkin_logs_status_created_at_idx ON checkin_logs (status, created_at)`},
			} {
				if err := EnsureIndex(db, idx.name, idx.sql); err != nil {
					return err
				}
			}
			return nil
		},
	},
	{
		// F5: structured events. New events carry a stable titleKey + JSON
		// params so the UI renders them in the viewer's locale; legacy rows
		// keep NULL and fall back to the stored English title/message.
		// Design: docs/internal/design/events-structured.md.
		Version:     "sc2_028_events_structured",
		Description: "events.title_key TEXT NULL + events.params TEXT NULL — structured event rendering (F5); NULL = legacy row rendered as-is",
		Apply: func(db *DB) error {
			// Production schemas always carry events (TS-era table); the
			// legacy-schema upgrade test fixture deliberately omits it, so
			// guard for a missing table the same way EnsureColumn guards a
			// missing column — an install without events cannot use F5.
			exists, err := tableExists(db, "events")
			if err != nil {
				return fmt.Errorf("store: probe events table: %w", err)
			}
			if !exists {
				slog.Info("store: skipping events_structured — no events table on this schema", "dialect", db.Dialect)
				return nil
			}
			if err := EnsureColumn(db, "events", "title_key", "TEXT", "TEXT", ""); err != nil {
				return err
			}
			return EnsureColumn(db, "events", "params", "TEXT", "TEXT", "")
		},
	},
	{
		// TS takeover hygiene: drizzle datetime('now') wrote
		// 'YYYY-MM-DD HH:MM:SS' while Go writes RFC3339 UTC. Mixed shapes in
		// the same TEXT column break every lexicographic comparison (space
		// sorts before 'T'), so TS-era rows read as older than they are —
		// ORDER BY listings skew and the checkin sweep re-checkins every
		// TS-era account on first Go boot. One-shot in-place rewrite to
		// RFC3339; idempotent (see ts_timestamp_normalize.go).
		Version:     "sc2_029_ts_timestamp_normalization",
		Description: "rewrite TS-era 'YYYY-MM-DD HH:MM:SS' values in TEXT *_at/*_until columns to RFC3339 UTC ('...T...Z') so lexicographic ordering and range comparisons agree across takeover rows",
		Apply: func(db *DB) error {
			n, err := normalizeLegacyTimestamps(db)
			if err != nil {
				return err
			}
			if n > 0 {
				slog.Info("store: normalized legacy TS timestamps", "rows", n, "dialect", db.Dialect)
			}
			return nil
		},
	},
}

// schemaMigrationsDDL creates the version bookkeeping table.
// Dual-dialect identical: text PK + ISO-8601 applied_at filled by the app.
const schemaMigrationsDDL = `CREATE TABLE IF NOT EXISTS schema_migrations (
	version TEXT PRIMARY KEY,
	applied_at TEXT NOT NULL,
	description TEXT
)`

// ensureSchemaMigrationsTable creates the schema_migrations bookkeeping table.
func ensureSchemaMigrationsTable(db *DB) error {
	if _, err := db.Exec(schemaMigrationsDDL); err != nil {
		return fmt.Errorf("store: create schema_migrations: %w", err)
	}
	return nil
}

// appliedVersions returns the set of Version strings already recorded.
func appliedVersions(db *DB) (map[string]struct{}, error) {
	rows, err := db.Query(`SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("store: list schema_migrations: %w", err)
	}
	defer rows.Close()

	out := make(map[string]struct{})
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: scan schema_migrations: %w", err)
		}
		out[v] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate schema_migrations: %w", err)
	}
	return out, nil
}

// markMigrationApplied records a successful step. Uses INSERT OR IGNORE /
// ON CONFLICT DO NOTHING so a concurrent or retried mark is safe.
func markMigrationApplied(db *DB, version, description string) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	var q string
	if db.Dialect == DialectPostgres {
		q = `INSERT INTO schema_migrations (version, applied_at, description)
			VALUES (?, ?, ?)
			ON CONFLICT (version) DO NOTHING`
	} else {
		q = `INSERT OR IGNORE INTO schema_migrations (version, applied_at, description)
			VALUES (?, ?, ?)`
	}
	if _, err := db.Exec(q, version, now, description); err != nil {
		return fmt.Errorf("store: mark migration %s: %w", version, err)
	}
	return nil
}

// ApplyAdditiveMigrations ensures the bookkeeping table exists, then runs any
// pending enterpriseAdditiveSteps in registry order. Safe on fresh and old DBs.
func ApplyAdditiveMigrations(db *DB) error {
	return applyAdditiveMigrations(db, enterpriseAdditiveSteps)
}

// applyAdditiveMigrations is the testable core that accepts an explicit step
// list. It preserves the original signature; callers that need the count of
// steps actually executed use applyAdditiveMigrationsCounted.
func applyAdditiveMigrations(db *DB, steps []AdditiveStep) error {
	_, err := applyAdditiveMigrationsCounted(db, steps)
	return err
}

// applyAdditiveMigrationsCounted runs the additive core and reports how many
// steps actually executed in this call. Steps already recorded in
// schema_migrations are skipped and not counted, so a fully converged
// database reports 0.
func applyAdditiveMigrationsCounted(db *DB, steps []AdditiveStep) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("store: ApplyAdditiveMigrations: db is nil")
	}

	if err := ensureSchemaMigrationsTable(db); err != nil {
		return 0, err
	}

	applied, err := appliedVersions(db)
	if err != nil {
		return 0, err
	}

	appliedCount := 0
	for _, step := range steps {
		if step.Version == "" {
			return 0, fmt.Errorf("store: additive step missing version")
		}
		if _, ok := applied[step.Version]; ok {
			continue
		}
		if step.Apply == nil {
			return 0, fmt.Errorf("store: additive step %s has nil Apply", step.Version)
		}

		slog.Info("store: applying additive migration",
			"version", step.Version,
			"description", step.Description,
			"dialect", db.Dialect,
		)
		if err := step.Apply(db); err != nil {
			return 0, fmt.Errorf("store: additive migration %s: %w", step.Version, err)
		}
		if err := markMigrationApplied(db, step.Version, step.Description); err != nil {
			return 0, err
		}
		applied[step.Version] = struct{}{}
		appliedCount++
	}

	return appliedCount, nil
}

// tableExists reports whether the given table exists in the current
// database. SQLite: sqlite_master; PostgreSQL: information_schema.tables
// (current schema). Returns false (no error) when the table is absent.
func tableExists(db *DB, table string) (bool, error) {
	table = strings.TrimSpace(table)
	if table == "" {
		return false, fmt.Errorf("store: tableExists: empty table")
	}
	if db.Dialect == DialectPostgres {
		var n int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = ?`, table).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("store: tableExists(%s) pg: %w", table, err)
		}
		return n > 0, nil
	}
	var n int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`,
		table).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("store: tableExists(%s) sqlite: %w", table, err)
	}
	return n > 0, nil
}

// columnExists reports whether table.column is present.
// SQLite: PRAGMA table_info; PostgreSQL: information_schema.columns.
func columnExists(db *DB, table, column string) (bool, error) {
	table = strings.TrimSpace(table)
	column = strings.TrimSpace(column)
	if table == "" || column == "" {
		return false, fmt.Errorf("store: columnExists: empty table or column")
	}

	if db.Dialect == DialectPostgres {
		var n int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND table_name = ?
			  AND column_name = ?`, table, column).Scan(&n)
		if err != nil {
			return false, fmt.Errorf("store: columnExists(%s.%s) pg: %w", table, column, err)
		}
		return n > 0, nil
	}

	// SQLite — PRAGMA table_info does not accept bound parameters for the table name.
	// Table names come only from our own registry / tests (not user input).
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, quoteIdentSQLite(table)))
	if err != nil {
		return false, fmt.Errorf("store: columnExists(%s.%s) sqlite: %w", table, column, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("store: columnExists scan: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// quoteIdentSQLite double-quotes a simple identifier for PRAGMA use.
// Rejects identifiers that are not alphanumeric/underscore to avoid injection.
func quoteIdentSQLite(name string) string {
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		// Fall back to empty quoted form; callers validate names from registry.
		return `""`
	}
	return `"` + name + `"`
}

// normalizeBooleanDefault rewrites numeric boolean literals (DEFAULT 0 / DEFAULT 1)
// into the SQL-standard DEFAULT FALSE / DEFAULT TRUE when the column type is
// BOOLEAN. SQLite stores booleans as INTEGER and accepts either spelling, but
// PostgreSQL type-checks the default expression against the column and rejects
// `ADD COLUMN flag BOOLEAN DEFAULT 0` with "column is of type boolean but
// default expression is of type integer" (SQLSTATE 42804) — which aborts
// AutoMigrate and stops a legacy database from starting at all. Registry entries
// are written once for both dialects, so the numeric spelling is normalized here
// instead of relying on every call site remembering the difference.
//
// Any trailing modifiers (e.g. NOT NULL) are preserved. Non-boolean column types
// and defaults that are not a bare 0/1 literal are returned untouched.
func normalizeBooleanDefault(colType, defaultClause string) string {
	if defaultClause == "" || !strings.EqualFold(strings.TrimSpace(colType), "BOOLEAN") {
		return defaultClause
	}
	fields := strings.Fields(defaultClause)
	if len(fields) < 2 || !strings.EqualFold(fields[0], "DEFAULT") {
		return defaultClause
	}
	switch fields[1] {
	case "0":
		fields[1] = "FALSE"
	case "1":
		fields[1] = "TRUE"
	default:
		return defaultClause
	}
	return strings.Join(fields, " ")
}

// buildAddColumnDDL renders the ALTER TABLE statement EnsureColumn executes.
// Split out from the execution path so the exact DDL for a dialect can be
// asserted without a live database.
func buildAddColumnDDL(table, column, colType, defaultClause string) string {
	if defaultClause == "" {
		return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, colType)
	}
	return fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s %s`, table, column, colType, defaultClause)
}

// EnsureColumn adds table.column if missing. Types are dialect-specific
// fragments (e.g. sqliteType="TEXT", pgType="TEXT", or INTEGER vs BOOLEAN).
// defaultClause is optional SQL after the type (e.g. "DEFAULT 0" or "DEFAULT FALSE");
// pass "" for nullable columns with no default (NULL → old behavior).

// This is the primary primitive SC2 uses for enterprise columns.
func EnsureColumn(db *DB, table, column, sqliteType, pgType, defaultClause string) error {
	exists, err := columnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	colType := sqliteType
	if db.Dialect == DialectPostgres {
		colType = pgType
	}
	colType = strings.TrimSpace(colType)
	if colType == "" {
		return fmt.Errorf("store: EnsureColumn %s.%s: empty type for dialect %s", table, column, db.Dialect)
	}

	// Validate identifiers from registry only.
	if quoteIdentSQLite(table) == `""` || quoteIdentSQLite(column) == `""` {
		return fmt.Errorf("store: EnsureColumn: invalid identifier %q.%q", table, column)
	}

	def := normalizeBooleanDefault(colType, strings.TrimSpace(defaultClause))
	ddl := buildAddColumnDDL(table, column, colType, def)

	if _, err := db.Exec(ddl); err != nil {
		// Race / concurrent startup: column may already exist.
		if existsNow, checkErr := columnExists(db, table, column); checkErr == nil && existsNow {
			return nil
		}
		return fmt.Errorf("store: EnsureColumn %s.%s: %w", table, column, err)
	}
	slog.Info("store: added column", "table", table, "column", column, "dialect", db.Dialect)
	return nil
}

// EnsureIndex creates a non-unique index if missing. Both dialects support
// CREATE INDEX IF NOT EXISTS.
func EnsureIndex(db *DB, name, createSQL string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(createSQL) == "" {
		return fmt.Errorf("store: EnsureIndex: empty name or SQL")
	}
	if _, err := db.Exec(createSQL); err != nil {
		return fmt.Errorf("store: EnsureIndex %s: %w", name, err)
	}
	return nil
}
