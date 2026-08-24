# Configuration Reference

**Last updated**: 2026-08-22

Complete environment-variable reference. The single machine-readable source
is [`.env.example`](../.env.example); this page groups and explains every
knob a deployment is likely to touch. Docker Compose users pass these via
`environment:` (see [`deployment.md`](deployment.md)).

> **How to read this page**: first-time deployment only needs the two
> [Required](#required) variables (plus the
> [Recommended](#recommended) credential secret). Everything else is optional
> and grouped by area — [Server](#server), [Database](#database),
> [Scheduling](#scheduling), [Notifications](#notifications),
> [Proxy behavior](#proxy-behavior), [Routing](#routing) and so on. When in
> doubt, `.env.example` is the source of truth for names and defaults.

## Required

| Variable | Default | Description |
|:---------|:--------|:------------|
| `AUTH_TOKEN` | `change-me-admin-token` | Admin web UI login token and management API bearer. Change it. |
| `PROXY_TOKEN` | `change-me-proxy-sk-token` | Default downstream key for `/v1/*` proxy calls. Additional per-project keys are created in the UI (downstream keys). |

## Recommended

| Variable | Default | Description |
|:---------|:--------|:------------|
| `ACCOUNT_CREDENTIAL_SECRET` | falls back to `AUTH_TOKEN` | Independent secret for encrypting stored upstream credentials (AES key derived via SHA-256). Set a unique 32+ byte random value in production; empty falls back to `AUTH_TOKEN` with a startup warning, short values log weak-secret warnings. |
| `TZ` | system local time | Timezone for cron schedules and daily aggregation. No code-level default; `.env.example` and the compose files preset `Asia/Shanghai`. |

## Server

| Variable | Default | Description |
|:---------|:--------|:------------|
| `PORT` | `4000` | HTTP listen port. |
| `HOST` | platform-dependent | Empty = `127.0.0.1` on Windows dev builds (avoids firewall prompts), `0.0.0.0` on server platforms; explicit values always win; the shipped container configs set `0.0.0.0`. |
| `DATA_DIR` | `./data` | SQLite database and upload storage directory. |
| `LOG_LEVEL` | `info` | slog threshold: `debug` / `info` / `warn` / `error`. |
| `METAPI_HEALTHCHECK_URL` / `METAPI_HEALTHCHECK_PATH` | — / `/ready` | Override the container healthcheck target. |
| `SYSTEM_NAME`, `LOGO`, `FOOTER`, `ABOUT`, `HOME_PAGE_CONTENT` | brand defaults | Optional site branding; runtime settings in the admin UI override these. |

## Database

| Variable | Default | Description |
|:---------|:--------|:------------|
| `DB_TYPE` | `sqlite` | `sqlite` or `postgres`; auto-inferred as `postgres` when a PostgreSQL URL is provided. |
| `DATABASE_URL` / `DB_URL` | empty | PostgreSQL connection string (or SQLite file path). `DB_URL` wins; `DATABASE_URL` exists for platform compatibility. Empty = single-node SQLite at `DATA_DIR/hub.db`. |
| `DB_SSLMODE` | empty | PostgreSQL TLS mode: `disable` / `allow` / `prefer` / `require` / `verify-ca` / `verify-full`; overrides the DSN `sslmode` when set. |
| `DB_SSL` | empty | Coarse PG TLS toggle; prefer `DB_SSLMODE`. |
| `DB_PROFILE` | `normal` | Pool preset: `shared-tiny` (2/1) / `normal` (10/3) / `dedicated` (20/5). |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | per profile | Explicit PostgreSQL pool budget; overrides the profile. Must respect your database role connection limit. |
| `DB_CONN_MAX_LIFETIME_SEC` | `1800` | PostgreSQL connection max lifetime (seconds). |
| `DB_CONN_MAX_IDLE_TIME_SEC` | `300` | PostgreSQL idle connection reaper (seconds). |
| `DB_APPLICATION_NAME` | `metapi-<hostname>` | `application_name` shown in `pg_stat_activity`. |
| `REDIS_URL` / `METAPI_REDIS_URL` | empty | Optional Redis for **multi-instance shared RPM/TPM admission** of downstream keys. Empty = process-local counters only. Unreachable Redis fails open back to process-local windows. Sticky sessions remain process-local regardless of Redis. |

## Scheduling

| Variable | Default | Description |
|:---------|:--------|:------------|
| `CHECKIN_CRON` | `0 8 * * *` | Daily check-in cron. |
| `CHECKIN_SCHEDULE_MODE` | `cron` | `cron`, `interval`, or `window` scheduling modes (other values are rejected at startup). |
| `CHECKIN_INTERVAL_HOURS` | `6` | Interval-mode period. |
| `CHECKIN_WINDOW_START` / `CHECKIN_WINDOW_END` | `00:00` / `23:59` | Random-window mode bounds. |
| `BALANCE_REFRESH_CRON` | `0 * * * *` | Balance refresh cron (hourly). |
| `LOG_CLEANUP_CRON` | `0 6 * * *` | Retention pruning cron. |
| `LOG_CLEANUP_USAGE_LOGS_ENABLED` / `LOG_CLEANUP_PROGRAM_LOGS_ENABLED` | auto | Which log families the cleanup prunes. |
| `LOG_CLEANUP_RETENTION_DAYS` | `30` | Proxy/program log retention. |

## Notifications

All providers are disabled unless their toggle/URL is set (webhook enabled by
default when `WEBHOOK_URL` is non-empty). Alert cooldown:
`NOTIFY_COOLDOWN_SEC` (default `300`).

| Provider | Variables |
|:---------|:----------|
| Webhook | `WEBHOOK_URL` |
| Bark | `BARK_URL` |
| ServerChan | `SERVERCHAN_KEY` |
| Telegram | `TELEGRAM_ENABLED`, `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`, `TELEGRAM_MESSAGE_THREAD_ID`, `TELEGRAM_USE_SYSTEM_PROXY` |
| Feishu | `FEISHU_ENABLED`, `FEISHU_WEBHOOK`, `FEISHU_SECRET` (HMAC signing) |
| DingTalk | `DINGTALK_ENABLED`, `DINGTALK_WEBHOOK`, `DINGTALK_SECRET` (HMAC signing) |
| WeCom | `WECOM_ENABLED`, `WECOM_WEBHOOK` |
| ntfy | `NTFY_ENABLED`, `NTFY_URL`, `NTFY_TOPIC`, `NTFY_TOKEN` |
| SMTP | `SMTP_ENABLED`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `SMTP_FROM`, `SMTP_TO`, `SMTP_SECURE` |

## Proxy behavior

| Variable | Default | Description |
|:---------|:--------|:------------|
| `REQUEST_BODY_LIMIT_MB` | `20` | Body limit for `/v1` routes (clamped 1–200). |
| `FILE_UPLOAD_LIMIT_MB` | `100` | Multipart limit for `/v1/files` and `/v1/images/*` (clamped 1–1000). |
| `PROXY_RATE_LIMIT_RPM` | `60` | Per-IP requests/min on `/v1`; `0` disables. |
| `PROXY_GLOBAL_TOKEN_RPM` | `0` | Global cap across all IPs for `PROXY_TOKEN`; safety net if a token leaks. |
| `PROXY_MAX_CHANNEL_ATTEMPTS` | `3` | Max retry/failover channel attempts per request. |
| `PROXY_MAX_BUFFERED_RESPONSE_BYTES` | `20971520` | Non-streaming upstream response buffer cap (20 MiB); over → 502. |
| `PROXY_MAX_STREAM_RESPONSE_BYTES` | `1048576` | Bytes relayed per SSE stream before controlled termination. |
| `PROXY_FIRST_BYTE_TIMEOUT_SEC` | unset | First upstream response byte timeout. |
| `PROXY_EMPTY_CONTENT_FAIL` | empty | Treat empty upstream content as failure. |
| `PROXY_ERROR_KEYWORDS` | empty | Extra keywords marking upstream responses as errors. |
| `DISABLE_CROSS_PROTOCOL_FALLBACK` | empty | Disable OpenAI ⇄ Claude cross-protocol fallback. |
| `PROXY_STICKY_SESSION_ENABLED` | empty | Pin a session to one channel (process-local). |
| `PROXY_STICKY_SESSION_TTL_MS` | `1800000` | Sticky binding TTL. |
| `PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT` | `2` | Concurrent streams per session-channel lease. |
| `PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS` | `1500` | Queue wait for a busy session channel. |
| `PROXY_SESSION_CHANNEL_LEASE_TTL_MS` / `..._KEEPALIVE_MS` | `90000` / `15000` | Session channel lease lifetime / keepalive. |
| `METAPI_ENABLE_PROXY_STUB` | empty | Test/demo-only local stub; leave empty in production (unconfigured forwarding returns an honest 503). |
| `SYSTEM_PROXY_URL` | empty | Outbound HTTP proxy for upstream calls. |

### Proxy log writer & retention

| Variable | Default | Description |
|:---------|:--------|:------------|
| `PROXY_LOG_ASYNC` | `true` | Batched async INSERTs (production latency win); set `false` for tests needing write-through. |
| `PROXY_LOG_BATCH_SIZE` | `50` | Flush threshold rows (1–1000). |
| `PROXY_LOG_FLUSH_INTERVAL_MS` | `1000` | Flush period (1–60000 ms). |
| `PROXY_LOG_RETENTION_DAYS` | `30` | Proxy log retention; `PROXY_LOG_RETENTION_PRUNE_INTERVAL_MINUTES` (30). |
| `PROXY_FILE_RETENTION_DAYS` | `30` | Uploaded file retention; prune interval 60 min. |
| `PROXY_VIDEO_TASK_RETENTION_DAYS` | `0` | Video task retention (0 = keep forever). |

## Routing

| Variable | Default | Description |
|:---------|:--------|:------------|
| `COST_WEIGHT` / `BALANCE_WEIGHT` / `USAGE_WEIGHT` | tuned | Channel weighting inputs for probability allocation. |
| `BASE_WEIGHT_FACTOR` / `VALUE_SCORE_FACTOR` | tuned | Weight shaping factors. |
| `ROUTING_FALLBACK_UNIT_COST` | — | Unit cost used when no price signal exists. |
| `TOKEN_ROUTER_CACHE_TTL_MS` | `1500` | Router decision cache. |
| `TOKEN_ROUTER_FAILURE_COOLDOWN_MAX_SEC` | — | Upper bound for failure cooldown. |
| `PRICING_CATALOG_ENABLED` | on | models.dev catalog as cold-start price signal; third-party relays are always labeled `catalog_estimate`. |
| `PRICING_CATALOG_REFRESH_MIN` / `PRICING_CATALOG_URL` | — | Catalog refresh period / mirror. |
| `MODEL_AVAILABILITY_PROBE_ENABLED` | `false` | Background channel health probing; interval/timeout/concurrency via `MODEL_AVAILABILITY_PROBE_*` (floor 60 s / 3 s, concurrency 1–16). |
| `ROUTE_REBUILD_PROBE_FILTER_ENABLED` | empty | Rebuild only routes whose probe status changed; include/exclude model lists available. |
| `PAYLOAD_RULES` / `PAYLOAD_RULES_JSON` | empty | Per-model payload rewrite rules. |
| `OPENAI_SERVICE_TIER_RULES(_JSON)` | empty | service_tier injection rules. |

## Security & network

| Variable | Default | Description |
|:---------|:--------|:------------|
| `ADMIN_IP_ALLOWLIST` | empty | CSV of allowed admin IPs/CIDRs. |
| `ADMIN_RATE_LIMIT_RPS` / `ADMIN_RATE_LIMIT_BURST` | `100` / `200` | Admin per-IP token bucket. |
| `OAUTH_RATE_LIMIT_RPS` / `OAUTH_RATE_LIMIT_BURST` | `10` / `20` | OAuth per-IP token bucket. |
| `TRUSTED_PROXY_CIDRS` | empty | Reverse-proxy CIDRs allowed to supply `X-Forwarded-For` / `X-Real-IP`; empty ignores forwarded headers. |
| `ADMIN_CORS_ALLOWED_ORIGINS` | empty | Exact `http(s)` origins allowed for `/api/*`; empty = same-origin admin UI only (`*` rejected). |
| `PROMPT_FILTER_ENABLED` | empty | Opt-in pattern filter blocking jailbreak/exfiltration prompts before shared OAuth upstreams; `PROMPT_FILTER_DENY_PATTERNS` extends the seed list. |

## Upstream transport hardening

| Variable | Default | Description |
|:---------|:--------|:------------|
| `RESIN_ENABLED` / `RESIN_URL` / `RESIN_PLATFORM_NAME` | off | Sticky residential proxy pool pinning OAuth accounts to stable IPs; per-site `resin_enabled` overrides the global flag. |
| `UTLS_ENABLED` | `false` | uTLS Chrome ClientHello fingerprint masking for outbound platform requests; per-site `use_utls` overrides. |

## OAuth overrides

| Variable | Description |
|:---------|:------------|
| `CLAUDE_CLIENT_ID` / `CLAUDE_CLIENT_SECRET` | Override bundled Claude OAuth app (secret has no fallback). |
| `CODEX_CLIENT_ID` | Override bundled Codex OAuth app. |
| `GEMINI_CLI_CLIENT_ID` / `GEMINI_CLI_CLIENT_SECRET` | Gemini CLI override (set both). |

## Codex / Responses compatibility

| Variable | Default | Description |
|:---------|:--------|:------------|
| `CODEX_UPSTREAM_WEBSOCKET_ENABLED` | empty | Codex upstream WebSocket transport (C3); non-upgrade GETs get an honest 426. |
| `CODEX_RESPONSES_WEBSOCKET_BETA` | empty | Responses-over-WS beta header. |
| `RESPONSES_COMPACT_FALLBACK_TO_RESPONSES_ENABLED` | empty | Compact → Responses fallback. |
| `CODEX_HEADER_DEFAULTS_USER_AGENT` / `..._BETA_FEATURES` | — | Header defaults for Codex upstreams. |

## Misc

| Variable | Default | Description |
|:---------|:--------|:------------|
| `LDOH_BASE_URL` | `https://ldoh.105117.xyz` | Upstream dashboard proxied by the monitor surface; point at a self-hosted instance. |
| `LDOH_PROXY_TIMEOUT_SEC` | `30` | LDOH upstream timeout. |
| `METAPI_ENABLE_UPDATE_CENTER` | empty | Re-enables the update-center scheduler (log-only no-op). |
