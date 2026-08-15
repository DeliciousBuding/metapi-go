# Admin API Reference

**Last updated**: 2026-08-15

Base URL: `http://localhost:4000/api`

All admin endpoints require authentication via Bearer token:

```
Authorization: Bearer <AUTH_TOKEN>
```

## Response Format

### Success

```json
{
  "success": true,
  "data": { ... }
}
```

### Error

```json
{
  "success": false,
  "message": "Error description"
}
```

HTTP status codes: 200 (OK), 201 (Created), 202 (Accepted), 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 500 (Internal Server Error).

## Request Body Rules

Request bodies are capped at 20 MiB by the HTTP router. Admin JSON handlers also apply the same cap when decoding, so direct route handlers still read bounded input.

Admin JSON requests must contain one JSON value. Duplicate object keys and trailing JSON values are rejected with `400`.

---

## Stats & Dashboard

### GET /api/stats/dashboard

Returns the admin dashboard snapshot.

**Response**: Site/account/token counts, total cost, usage summaries.

### GET /api/stats/proxy-logs

Query proxy request logs.

**Query params**: `page`, `limit`, `status`, `model`, `client`, `from`, `to`

### GET /api/stats/proxy-logs/:id

Get a single proxy log entry by ID.

### GET /api/stats/site-distribution

Token distribution across sites.

### GET /api/stats/site-trend

Usage trend data by site.

### GET /api/stats/model-by-site

Model usage breakdown by site.

### GET /api/stats/usage-heatmap

Hour × site/model usage density for admin analytics.

**Query params**: `days` (1–31, default 7), `dimension` (`site`|`model`, default `site`)

**Response**: `{ dimension, days, since, source, cellLimit, count, cells:[{bucket,key,label,calls,tokens,spend}] }`. Bounded with hard `LIMIT` (2000). Prefers `site_hour_usage` for site dimension; model dimension aggregates `proxy_logs`. Never includes chat content.

### GET /api/stats/slow-requests

Top slow proxy requests by `latency_ms` within a time window.

**Query params**: `limit` (1–200, default 50), `minLatencyMs` (default 1000), `hours` (1–168, default 24)

**Response**: `{ hours, minLatencyMs, limit, since, count, items:[{id,model,status,latencyMs,firstByteLatencyMs,httpStatus,requestId,accountId,siteId,siteName,createdAt}] }`. Metadata only — no request/response bodies.

---

## Models & Routes

### GET /api/routes/lite

Lightweight route list (id, modelPattern, displayName, displayIcon, routeMode, routingStrategy, enabled).

### GET /api/routes/summary

Route list with channel counts and site names.

### GET /api/routes

Full route list with channels, accounts, and site information.

### GET /api/routes/:id/channels

Channels for a specific route.

### POST /api/routes

Create a new route.

### PUT /api/routes/:id

Update an existing route.

### DELETE /api/routes/:id

Delete a route and its channels.

### POST /api/routes/batch

Batch enable/disable/delete routes. Body: `{ "ids": [1, 2, 3], "action": "enable" }`.

### POST /api/routes/rebuild

Trigger route rebuild. Body: `{ "refreshModels": true }`.

### POST /api/routes/:id/cooldown/clear

Clear cooldown state for all channels on a route.

### POST /api/routes/:id/channels/batch

Batch add/update channels on a route.

### POST /api/routes/:id/channels

Add a single channel to a route.

### PUT /api/channels/batch

Batch update channel properties (weight, enabled, priority).

### PUT /api/channels/:channelId

Update a single channel.

### DELETE /api/channels/:channelId

Delete a channel.

### POST /api/admin/test-channel

**Auth**: admin Bearer token. Alias: `POST /api/debug/channel-probe` (same handler).

Forces a single upstream request against a specific channel or site, bypassing weighted selection. Useful for smoke-testing a channel after a credential rotation.

**Body**:
```json
{
  "channelId": 12,
  "siteId": 3,
  "model": "gpt-4o-mini",
  "prompt": "ping",
  "mode": "chat",
  "timeoutMs": 15000
}
```

Either `channelId` or `siteId` is required. `mode` is `chat` (default) or `models`; `models` issues a `GET /v1/models` instead of a chat completion. `timeoutMs` is clamped to `[1000, 60000]` (default `15000`). `prompt` is truncated to 256 runes and never persisted.

**Response** (200):
```json
{
  "success": true,
  "statusCode": 200,
  "latencyMs": 842,
  "truncatedBody": "...",
  "error": "",
  "channelId": 12,
  "siteId": 3,
  "accountId": 5,
  "model": "gpt-4o-mini",
  "mode": "chat",
  "bodyTruncated": true
}
```

Returns a ~2 KiB redacted body summary (`bodyTruncated` flags truncation). Secret-like tokens in the body/error are redacted. A channel with no usable token returns `success: false` with `error: "No usable token on channel/account ..."`.

---

## Route Decision

### GET /api/routes/decision

Get route decision (which channel was selected for which model).

### POST /api/routes/decision/batch

Batch decision query for specific models.

### POST /api/routes/decision/by-route/batch

Batch decision query for specific routes.

### POST /api/routes/decision/route-wide/batch

Route-wide decision query.

### POST /api/routes/decision/refresh

Trigger decision snapshot refresh.

---

## Model Marketplace & Probing

### GET /api/models/marketplace

Available models by site.

### GET /api/models/price-compare

Cross-site effective model price comparison for operators.

Query: `model` (optional name/substring), `days` (default 30), `limit` (default 50), `topModels` (default 12 when model empty).

Returns `{ model, days, limit, sampleUsage, items: [{ siteId, siteName, platform, model, accountId, username, inputPerMillion, outputPerMillion, source, ratesSource, estimatedCostSample, observedSamples, configuredUnitCost, missingPrice, recommended }], meta }`.

`source` is one of `billing_details` | `observed` | `configured` | `fallback`. Fallback is always labeled; `missingPrice=true` when no catalog/observed/configured signal exists. Rows are sorted cheapest-first and `recommended=true` marks the best channel per model.

Alias: `GET /api/stats/model-prices` (same handler).

### GET /api/models/redirect-fix-candidates

List disabled models that an existing redirect mapping can restore.

Returns `{ items: [{ siteId, siteName, accountId, modelName, canonical, actual }], count }`.

### POST /api/models/redirect-fix-candidates

Apply the listed redirect fixes. Body: `{ "dryRun": false }` (`false` applies; `true` previews without deleting). Returns `{ success, dryRun, removed, count }`.

### GET /api/models/token-candidates

Available token candidates for route configuration.

### POST /api/models/check/:accountId

Check model availability for a specific account. Returns `{ "success": true, "models": [...] }`.

### POST /api/models/probe

Trigger a model probe. Body: `{ "models": ["gpt-4o"], "wait": false }`. Returns 202 with probe job.

---

## Proxy Debug

### GET /api/stats/proxy-debug/traces

List proxy debug traces.

### GET /api/stats/proxy-debug/traces/:id

Get a specific debug trace with related attempts.

---

## Sites

### GET /api/sites, POST /api/sites

List all sites. Create a new site.

### GET /api/sites/:id, PUT /api/sites/:id, DELETE /api/sites/:id

Get, update, delete a site.

### POST /api/sites/detect

Detect the platform for a single URL (used by the import wizard's auto-detect step). Tries site-initialization presets first, then hostname heuristics.

**Auth**: admin Bearer token.

**Body**:
```json
{ "url": "https://api.deepseek.com/v1" }
```

**Response** (200):
```json
{
  "url": "https://api.deepseek.com/v1",
  "canonicalUrl": "https://api.deepseek.com/v1",
  "platform": "openai",
  "siteType": "openai",
  "confidence": 1.0,
  "initializationPresetId": "deepseek-openai"
}
```

`confidence` is `0..1` (`1.0` when a preset matched). `initializationPresetId` is omitted when no preset matched. Returns `400` with `{"error":"Could not detect platform"}` when no platform can be resolved, and `400` with `{"error":"Invalid url. Expected non-empty string."}` for an empty/missing URL.

### POST /api/sites/import

Idempotent batch import of sites (and optional accounts) from the import wizard. A site is keyed by `(platform, url)`; duplicates are resolved per the chosen strategy.

**Auth**: admin Bearer token.

**Body**:
```json
{
  "items": [
    {
      "name": "DeepSeek",
      "url": "https://api.deepseek.com/v1",
      "platform": "openai",
      "globalWeight": 1.0,
      "maxConcurrency": 0,
      "duplicateStrategy": "skip",
      "accounts": [
        { "username": "svc-1", "accessToken": "sk-...", "apiToken": "" }
      ]
    }
  ],
  "duplicateStrategy": "skip"
}
```

`items[].platform` is optional (auto-detected when omitted). `duplicateStrategy` is `skip` (default) or `merge`; per-item `duplicateStrategy` overrides the batch default. `accounts` is optional; each account is stored as an API-key style credential (model discovery is deferred to background refresh).

**Response** (200):
```json
{
  "imported": 1,
  "skipped": 0,
  "failed": 0,
  "results": [
    {
      "name": "DeepSeek",
      "url": "https://api.deepseek.com/v1",
      "status": "imported",
      "siteId": 12
    }
  ]
}
```

`status` is one of `imported` | `merged` | `skipped` | `failed`; `reason` and `siteId` are omitted when not applicable. `imported` counts newly created sites plus accounts merged into an existing site; `skipped` counts idempotent no-ops; `failed` counts hard errors. Returns `400` with `{"error":"items is required"}` for an empty `items` array and `500` with `{"error":"Import sites failed"}` on internal failure. Site/route caches are invalidated when at least one site was imported.

---

## Accounts

### GET /api/accounts, POST /api/accounts

List all accounts. Create a new account.

### GET /api/accounts/:id, PUT /api/accounts/:id, DELETE /api/accounts/:id

Get, update, delete an account.

---

## Account Tokens

### GET /api/account-tokens, POST /api/account-tokens

List all account tokens. Create a new account token.

### GET /api/account-tokens/:id, PUT /api/account-tokens/:id, DELETE /api/account-tokens/:id

Get, update, delete an account token.

---

## Site Announcements

### GET /api/site-announcements

List site announcements.

### POST /api/site-announcements/{id}/read

Mark one site announcement as read.

### POST /api/site-announcements/read-all

Mark all site announcements as read.

### POST /api/site-announcements/sync

Sync site announcements from configured sites.

### DELETE /api/site-announcements

Delete all site announcements.

---

## Events

### GET /api/events

List system events (paginated). **Query params**: `page`, `limit`, `level`.

### GET /api/events/count

Get count of unread events.

### POST /api/events/read-all

Mark all events as read.

### POST /api/events/{id}/read

Mark a single event as read.

---

## Downstream API Keys

### GET /api/downstream-keys

List all downstream API keys.

### GET /api/downstream-keys/summary

Key list with usage summaries. **Query params**: `group`, `tags`, `tagMatch`.

### GET /api/downstream-keys/:id/overview

Usage overview for a specific key (24h, 7d, all-time).

### GET /api/downstream-keys/:id/trend

Usage trend data for a specific key.

### POST /api/downstream-keys

Create a new downstream API key.

**Body**:
```json
{
  "name": "My Key",
  "groupName": "production",
  "tags": "tag1,tag2",
  "supportedModels": ["gpt-4o", "claude-sonnet-4-20250514"],
  "allowedRouteIds": [1, 2],
  "maxCost": 100.0,
  "maxRequests": 10000,
  "expiresAt": "2026-12-31T23:59:59Z"
}
```

### PUT /api/downstream-keys/:id

Update a downstream API key.

### DELETE /api/downstream-keys/:id

Delete a downstream API key.

### POST /api/downstream-keys/:id/reset-usage

Reset usage counters (used_cost, used_requests) to zero.

---

## Settings

### GET /api/settings/runtime

Get all runtime settings as a flat JSON object. Sensitive values (proxyToken, tokens, passwords) are masked. The response includes branding fields (`systemName`, `logo`, `footer`, `about`, `homePageContent`, `serverAddress`) and semantic schedule mirrors (`checkinSchedule`, `balanceRefreshSchedule`, `logCleanupSchedule`).

`ScheduleSpec` v1 uses `{ "version": 1, "kind": "daily|interval|window|custom", ... }`. The legacy `*_cron` fields remain available and remain the runtime compatibility source of truth.

### PUT /api/settings/runtime

Update runtime settings. Partial update -- only send fields you want to change. Schedule updates atomically write both the legacy cron key and the corresponding v1 semantic mirror.

### GET /api/settings/migration/preview

Preview the additive settings migration. Returns `currentVersion`, `targetVersion`, `pending`, `customCount`, `legacyFieldsPreserved`, and per-task migration items.

### POST /api/settings/migration/apply

Apply the settings migration in one transaction. It only adds `*_schedule_v2` mirrors and `settings_schema_version`; existing legacy keys are not removed or changed. Repeated calls are no-ops after completion.

### GET /api/settings/brand-list

Get list of AI model brands.

### POST /api/settings/system-proxy/test

Test system proxy connectivity.

---

## Settings - Database

### GET /api/settings/database/runtime

Get database runtime state. `active` reports the database used by the current process, with PostgreSQL credentials masked. `saved` reports a restart-pending override from settings when one exists.

### PUT /api/settings/database/runtime

Save database configuration for the next restart. The response keeps `active` separate from `saved` so operators can see whether the process has switched yet.

### POST /api/settings/database/test-connection

Test a SQLite or PostgreSQL connection string. Returns `400` when the dialect is unsupported or the connection cannot be opened. Error messages mask credentials.

### POST /api/settings/database/migrate

Returns `501`. Runtime database migration is not wired into the admin API yet. Use `metapi-migrate` for SQLite to PostgreSQL migration.

---

## Settings - Backup

### GET /api/settings/backup/export

Export all settings and data as JSON.

### POST /api/settings/backup/import

Import settings and data from JSON. Runtime-local settings such as `auth_token`, database connection settings, and WebDAV sync state are skipped.

### GET /api/settings/backup/webdav

Get WebDAV backup configuration and last sync state. Passwords are never returned; use `hasPassword` and `passwordMasked` to show saved credential status.

The returned `state.lastSyncAt` is the last successful WebDAV import/export time. `state.lastAttemptAt` is the most recent attempt time, including failed attempts. `state.lastError` is set only when the latest attempt failed.

### PUT /api/settings/backup/webdav

Update WebDAV backup configuration. `fileUrl` must be an `http` or `https` URL without embedded userinfo. `exportType` supports `all`, `accounts`, or `preferences`.

### POST /api/settings/backup/webdav/export

Export a restorable backup payload to `fileUrl` with HTTP `PUT`. The payload uses the same `tables` structure as `GET /api/settings/backup/export`.

### POST /api/settings/backup/webdav/import

Download a backup payload from `fileUrl` with HTTP `GET` and import its `tables`. Runtime-local settings are skipped. The response includes imported row counts and updated sync state. The maximum downloaded backup size is 64 MiB.

---

## Settings - Notifications

### POST /api/settings/notify/test

Send a test notification.

---

## Settings - Maintenance

### POST /api/settings/maintenance/clear-cache

Clear model availability cache and rebuild routes. Returns deleted counts.

### POST /api/settings/maintenance/clear-usage

Clear all proxy usage data (proxy_logs, route_channel stats, account balanceUsed).

### POST /api/settings/maintenance/factory-reset

Reset all data to factory defaults.

---

## Checkin

### POST /api/checkin/trigger

Trigger manual checkin for all accounts.

### POST /api/checkin/trigger/{id}

Trigger manual checkin for a single account.

### GET /api/checkin/logs

List checkin execution logs.

### PUT /api/checkin/schedule

Update checkin schedule (cron, interval, or random-window mode). The handler keeps the legacy fields and `checkin_schedule_v2` mirror synchronized.

---

## Update Center

> **501 residual**：本组接口当前返回 501（未实现）。版本更新提示走 GitHub Releases / GHCR 镜像 tag，前端不依赖此 API。详见 [`STATE.md`](STATE.md)。

### GET /api/update-center/status

Get update center status.

### POST /api/update-center/check

Trigger update center check.

---

## Monitor

### GET /api/monitor/health

Monitoring health snapshot.

### GET /api/monitor/config

Get monitoring configuration.

### PUT /api/monitor/config

Save monitoring configuration.

### POST /api/monitor/session

Start a monitoring session.

### DELETE /api/monitor/session

Clear the monitoring session.

---

## Admin Diagnostics & Observability

Read-only surfaces for resin, scheduler health, and rate overview. These endpoints never mutate state; they exist so operators can confirm automations and rate tuning at a glance.

### GET /api/admin/resin/status

Resin sticky-proxy-pool observability snapshot (#698).

**Auth**: admin Bearer token.

**Response** (200):
```json
{
  "enabled": false,
  "resinUrl": "http://resin.local:2260/my-token",
  "platformName": "",
  "activeLeases": [
    { "accountId": 5, "lastUsed": "2026-08-15T08:12:00Z" }
  ],
  "perSiteOverrides": [
    { "siteId": 3, "name": "anthropic", "platform": "anthropic", "resinEnabled": true }
  ],
  "generatedAt": "2026-08-15T08:12:30Z"
}
```

`enabled` reflects `RESIN_ENABLED` plus a non-empty `RESIN_URL`. `activeLeases` lists accounts whose last-used timestamp is fresher than the 5-minute stale TTL. `perSiteOverrides` only lists sites with an explicit non-NULL `resin_enabled` column (NULL-inherit rows are absent so the snapshot stays bounded).

### GET /api/scheduler/status

Unified run-history view of recurring schedulers. Each entry reports the job's last-run signal and a coarse 24h activity window.

**Auth**: admin Bearer token.

**Response** (200):
```json
{
  "items": [
    {
      "job": "checkin",
      "enabled": true,
      "lastRunAt": "2026-08-15T08:00:00Z",
      "lastStatus": "success",
      "runs24h": 4,
      "success24h": 4
    },
    {
      "job": "model-probe",
      "enabled": false,
      "lastStatus": "never",
      "note": "not enabled (MODEL_AVAILABILITY_PROBE_ENABLED)"
    }
  ],
  "generatedAt": "2026-08-15T08:12:30Z"
}
```

Jobs surfaced: `checkin`, `balance-refresh`, `model-probe`, `site-announcements`, `daily-summary`, `log-cleanup`, `usage-aggregation`. `lastStatus` is one of `success` | `failed` | `running` | `never`. Data sources are existing run signals only (checkin_logs, accounts.last_balance_refresh, in-memory probe summary, events rows) — no scheduler code changes.

### GET /api/models/rates

Read-only aggregation of every multiplier/rate surface: account unit cost, channel weight, site global weight, downstream-key weight, and 30-day observed model spend.

**Auth**: admin Bearer token.

**Response** (200):
```json
{
  "generatedAt": "2026-08-15T08:12:30Z",
  "summary": {
    "accountsWithUnitCost": 3,
    "accountsTotal": 5,
    "channelsTotal": 12,
    "channelsEnabled": 9
  },
  "accounts": [
    { "accountId": 5, "username": "svc-1", "siteId": 3, "siteName": "anthropic", "unitCost": 0.003, "channelCount": 2, "totalWeight": 40 }
  ],
  "channels": [
    { "channelId": 12, "routeId": 1, "routePattern": "gpt-4o", "accountId": 5, "username": "svc-1", "modelName": "gpt-4o", "weight": 20, "enabled": true }
  ],
  "sites": [
    { "siteId": 3, "siteName": "anthropic", "globalWeight": 1.0 }
  ],
  "keys": [
    { "keyId": 1, "name": "prod-key", "keyWeight": 1.0 }
  ],
  "models": [
    { "model": "gpt-4o", "calls": 1200, "spend": 1.23, "tokens": 450000 }
  ]
}
```

`unitCost` is a display/planning field (estimated cost is ratio-based, never account-priced). Observed model costs are aggregated from `model_day_usage` over the trailing 30 days.

### PUT /api/models/rates

Batch update account unit cost and route-channel weight. Pure config writes; `unit_cost` stays a display/planning field.

**Auth**: admin Bearer token.

**Body**:
```json
{
  "accounts": [
    { "id": 5, "unitCost": 0.003 }
  ],
  "channels": [
    { "id": 12, "weight": 20 }
  ]
}
```

`unitCost` and `weight` must be `>= 0`. Missing arrays are no-ops; both empty returns `400 "nothing to update"`. Weights feed the routing cache, so the handler invalidates it on success.

**Response** (200):
```json
{
  "success": true,
  "updatedAccounts": 1,
  "updatedChannels": 1
}
```

---

## Search

### POST /api/search

Global search across sites, accounts, routes, and keys.

---

## Tasks

### GET /api/tasks

List background tasks.

### GET /api/tasks/:id

Get task status.

---

## Test

### GET /api/test/read

Read-only test endpoint.

### POST /api/test/write

Write test endpoint.

### PUT /api/test/update

Update test endpoint.

### PATCH /api/test/patch

Patch test endpoint.

### DELETE /api/test/delete

Delete test endpoint.

### POST /api/test/boom

Intentional panic test endpoint.

---

## OAuth

### GET /api/oauth/providers

List OAuth providers.

### POST /api/oauth/providers/{provider}/start

Start OAuth authorization flow for a provider.

### GET /api/oauth/callback/{provider}

OAuth callback endpoint for a provider.

---

## Auth Settings

### GET /api/settings/auth/info

Get authentication settings (admin IP allowlist, proxy token config).

### POST /api/settings/auth/change

Update authentication settings.

---

## Health

### GET /health

Liveness check (no auth required). It does not touch dependencies and returns `{"status":"ok"}` when the HTTP process is alive.

### GET /ready

Readiness check (no auth required). It pings the active database and returns `200 {"status":"ok","database":"ok"}` when ready, `503 {"status":"degraded","database":"error"}` when the database is unavailable, or `503 {"status":"draining","database":"ok"}` while graceful shutdown is in progress.

### GET /api/desktop/health

Desktop health check. Returns `{"status":"ok"}`.

## Security Notes

### LDOH monitor proxy

The `/monitor-proxy/ldoh/*` admin surface proxies an upstream LDOH dashboard. The base URL is configurable via `LDOH_BASE_URL` (default `https://ldoh.105117.xyz`). The `LDOH_BASE_URL` env var lets operators redirect the monitor iframe at a self-hosted LDOH instance without rebuilding.

**At-rest cookie concern**: The upstream LDOH session cookie (`ld_auth_session=…`) is stored **plaintext** in the `settings` table (key `monitor_ldoh_cookie`). Anyone with read access to the database can impersonate the LDOH session until it expires upstream. This is an accepted short-term trade-off; a future improvement should encrypt this secret at rest (e.g. AES-GCM keyed by `ACCOUNT_CREDENTIAL_SECRET`). Treat database read access as equivalent to LDOH credential disclosure until then.

**Error leakage**: Upstream request failures return a generic `"LDOH upstream request failed"` message to the browser. The full error (DNS/TLS/network details) is logged server-side via `slog` only, preventing upstream topology leakage to end users.

### WebDAV backup SSRF hardening

WebDAV import/export URLs (`fileUrl`) are validated against SSRF at two layers:

1. **URL validation** (`isValidWebdavFileURL`): rejects schemes other than `http`/`https`, embedded userinfo, invalid ports, `localhost` hostnames, and literal IP addresses in private (RFC 1918), loopback (127.0.0.0/8, ::1), link-local (169.254.0.0/16, fe80::/10 — blocks cloud metadata endpoints like 169.254.169.254), multicast, and unspecified (0.0.0.0/8, ::) ranges.
2. **Dial-time DNS resolution** (`rejectUnsafeWebdavDialHost`): resolves the hostname and rejects any resolved IP in the same unsafe ranges, preventing TOCTOU races where a hostname resolves differently between validation and connection.

Redirect targets are validated with the same `isValidWebdavFileURL` check before being followed (max 5 redirects; HTTPS-to-HTTP downgrades refused).

## Browser CORS

Admin routes under `/api/*` are same-origin by default. Set `ADMIN_CORS_ALLOWED_ORIGINS` to a comma-separated list of exact trusted `http(s)` browser origins only when the admin UI is hosted separately. Wildcards, paths, query strings, and fragments are rejected. Proxy routes and health/metrics endpoints retain wildcard CORS.

## Trusted Client IPs

Forwarded client IP headers are ignored by default. Set `TRUSTED_PROXY_CIDRS` only for reverse-proxy source ranges you control; admin IP allowlists and rate limits otherwise use the direct peer IP.


### GET /api/downstream-keys/:id/export


---

## Proxy files (`/v1/files`)

OpenAI-compatible Files surface (proxy auth required). Forwards to the selected upstream channel; does not persist customer file bytes on MetAPI disk.

| Method | Path | Notes |
|--------|------|--------|
| POST | `/v1/files` | Multipart upload (`file` field). |
| GET | `/v1/files` | List files from upstream. |
| GET | `/v1/files/{fileId}` | File metadata. |
| GET | `/v1/files/{fileId}/content` | File content download. |
| DELETE | `/v1/files/{fileId}` | Delete file. |

Channel selection uses model key: body/multipart `model`, else `?model=`, else `X-Metapi-Files-Model`, else default `gpt-4o`. Residual platforms without a Files API return upstream errors.

---

## Downstream Pricing (`/v1/pricing`)

Cross-site effective model price catalog exposed behind downstream-key (ProxyAuth) auth — NOT admin auth. A downstream consumer (中转站) holding a managed key can query effective cross-site model pricing for its own planning. Reuses the `modelPriceCompare` handler so the data surface is identical to `GET /api/models/price-compare`; no separate catalog to drift.

Mounted under `/v1/*` so it inherits ProxyAuth + wildcard CORS.

### GET /v1/pricing

**Auth**: ProxyAuth (downstream API key), not admin Bearer token.

**Query params**: `model` (optional name/substring), `days` (1–365, default 30), `limit` (default 50, max 200), `topModels` (1–50, default 12, used only when `model` is empty).

**Response** (200):
```json
{
  "model": "",
  "days": 30,
  "limit": 50,
  "sampleUsage": { "promptTokens": 1000, "completionTokens": 500, "totalTokens": 1500 },
  "items": [
    {
      "siteId": 3,
      "siteName": "anthropic",
      "platform": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "accountId": 5,
      "username": "svc-1",
      "inputPerMillion": 3.0,
      "outputPerMillion": 15.0,
      "source": "billing_details",
      "ratesSource": "billing_details",
      "estimatedCostSample": 0.0105,
      "observedSamples": 1200,
      "configuredUnitCost": 0.003,
      "missingPrice": false,
      "recommended": true
    }
  ],
  "meta": {
    "count": 12,
    "modelsConsidered": 12,
    "sources": ["billing_details", "observed", "configured", "fallback"],
    "notes": "Rates/costs from billing_details, observed proxy_logs, account unit_cost, or labeled fallback. missingPrice=true means no catalog/observed/configured signal."
  }
}
```

`source` is one of `billing_details` | `observed` | `configured` | `fallback`. Fallback is always labeled; `missingPrice=true` when no catalog/observed/configured signal exists. Rows are sorted cheapest-first and `recommended=true` marks the best channel per model. Alias: `GET /v1/models/price-compare` (same handler).

---

## Admin Route Inventory
Complete list of registered `/api` admin routes (generated from the router registration). Path parameters use `:param` syntax.

### GET
- `/api/account-tokens`
- `/api/account-tokens/:id/value`
- `/api/account-tokens/account/:accountId/default`
- `/api/account-tokens/groups/:accountId`
- `/api/accounts`
- `/api/accounts/:id/models`
- `/api/admin/audit-logs`
- `/api/admin/ops/ws`
- `/api/announcements`
- `/api/announcements/active`
- `/api/channels`
- `/api/checkin/logs`
- `/api/debug/vars`
- `/api/desktop/health`
- `/api/downstream-keys`
- `/api/downstream-keys/:id/export`
- `/api/downstream-keys/:id/overview`
- `/api/downstream-keys/:id/trend`
- `/api/downstream-keys/summary`
- `/api/events`
- `/api/events/count`
- `/api/model-redirects`
- `/api/models/marketplace`
- `/api/models/price-compare`
- `/api/models/rates`
- `/api/models/redirect-fix-candidates`
- `/api/models/token-candidates`
- `/api/models/verify-history`
- `/api/monitor/config`
- `/api/monitor/health`
- `/api/oauth/callback/:provider`
- `/api/oauth/connections`
- `/api/oauth/providers`
- `/api/oauth/sessions/:state`
- `/api/ping`
- `/api/routes`
- `/api/routes/:id/channels`
- `/api/routes/decision`
- `/api/routes/lite`
- `/api/routes/summary`
- `/api/scheduler/status`
- `/api/settings/auth/info`
- `/api/settings/backup/export`
- `/api/settings/backup/webdav`
- `/api/settings/brand-list`
- `/api/settings/database/runtime`
- `/api/settings/migration/preview`
- `/api/settings/runtime`
- `/api/site-announcements`
- `/api/sites`
- `/api/sites/:id/available-models`
- `/api/sites/:id/disabled-models`
- `/api/sites/:id/probe-stream`
- `/api/stats/attention`
- `/api/stats/balance-history`
- `/api/stats/balance-income-outcome`
- `/api/stats/dashboard`
- `/api/stats/latency-histogram`
- `/api/stats/latency-trend`
- `/api/stats/model-by-site`
- `/api/stats/model-cost-distribution`
- `/api/stats/model-prices`
- `/api/stats/proxy-debug/traces`
- `/api/stats/proxy-debug/traces/:id`
- `/api/stats/proxy-logs`
- `/api/stats/proxy-logs/:id`
- `/api/stats/site-distribution`
- `/api/stats/site-trend`
- `/api/stats/slow-requests`
- `/api/stats/usage-heatmap`
- `/api/tags`
- `/api/tasks`
- `/api/tasks/:id`
- `/api/test/chat/jobs/:jobId`
- `/api/test/proxy/jobs/:jobId`
- `/api/test/read`
- `/api/update-center/status`
- `/api/update-center/tasks/:id/stream`

### POST
- `/api/account-tokens`
- `/api/account-tokens/:id/default`
- `/api/account-tokens/batch`
- `/api/account-tokens/sync-all`
- `/api/account-tokens/sync/:accountId`
- `/api/accounts`
- `/api/accounts/:id/balance`
- `/api/accounts/:id/models/manual`
- `/api/accounts/:id/rebind-session`
- `/api/accounts/batch`
- `/api/accounts/health/refresh`
- `/api/accounts/login`
- `/api/accounts/verify-token`
- `/api/admin/test-channel`
- `/api/announcements`
- `/api/announcements/:id/dismiss`
- `/api/checkin/trigger`
- `/api/checkin/trigger/:id`
- `/api/debug/channel-probe`
- `/api/downstream-keys`
- `/api/downstream-keys/:id/reset-usage`
- `/api/downstream-keys/batch`
- `/api/events/:id/read`
- `/api/events/read-all`
- `/api/model-redirects/apply`
- `/api/model-redirects/generate`
- `/api/models/check/:accountId`
- `/api/models/probe`
- `/api/models/redirect-fix-candidates`
- `/api/models/verify-batch`
- `/api/monitor/session`
- `/api/oauth/connections/:accountId/quota/refresh`
- `/api/oauth/connections/:accountId/rebind`
- `/api/oauth/connections/quota/refresh-batch`
- `/api/oauth/import`
- `/api/oauth/providers/:provider/start`
- `/api/oauth/route-units`
- `/api/oauth/sessions/:state/manual-callback`
- `/api/ping`
- `/api/routes`
- `/api/routes/:id/channels`
- `/api/routes/:id/channels/batch`
- `/api/routes/:id/cooldown/clear`
- `/api/routes/batch`
- `/api/routes/decision/batch`
- `/api/routes/decision/by-route/batch`
- `/api/routes/decision/refresh`
- `/api/routes/decision/route-wide/batch`
- `/api/routes/rebuild`
- `/api/search`
- `/api/settings/auth/change`
- `/api/settings/backup/import`
- `/api/settings/backup/import/preview`
- `/api/settings/backup/webdav/export`
- `/api/settings/backup/webdav/import`
- `/api/settings/database/migrate`
- `/api/settings/database/test-connection`
- `/api/settings/maintenance/clear-cache`
- `/api/settings/maintenance/clear-usage`
- `/api/settings/maintenance/factory-reset`
- `/api/settings/migration/apply`
- `/api/settings/notify/test`
- `/api/settings/system-proxy/test`
- `/api/site-announcements/:id/read`
- `/api/site-announcements/read-all`
- `/api/site-announcements/sync`
- `/api/sites`
- `/api/sites/:id/probe-now`
- `/api/sites/batch`
- `/api/sites/detect`
- `/api/sites/import`
- `/api/test/boom`
- `/api/test/chat`
- `/api/test/chat/jobs`
- `/api/test/chat/stream`
- `/api/test/proxy`
- `/api/test/proxy/jobs`
- `/api/test/proxy/stream`
- `/api/test/write`
- `/api/update-center/check`
- `/api/update-center/deploy`
- `/api/update-center/rollback`

### PUT
- `/api/account-tokens/:id`
- `/api/accounts/:id`
- `/api/accounts/:id/tags`
- `/api/announcements/:id`
- `/api/channels/:channelId`
- `/api/channels/batch`
- `/api/checkin/schedule`
- `/api/downstream-keys/:id`
- `/api/model-redirects/:id`
- `/api/models/rates`
- `/api/monitor/config`
- `/api/routes/:id`
- `/api/routes/reorder`
- `/api/settings/backup/webdav`
- `/api/settings/database/runtime`
- `/api/settings/runtime`
- `/api/sites/:id`
- `/api/sites/:id/disabled-models`
- `/api/sites/:id/tags`
- `/api/test/update`
- `/api/update-center/config`

### PATCH
- `/api/oauth/connections/:accountId/proxy`
- `/api/oauth/route-units/:routeUnitId`
- `/api/test/patch`

### DELETE
- `/api/account-tokens/:id`
- `/api/accounts/:id`
- `/api/announcements/:id`
- `/api/channels/:channelId`
- `/api/downstream-keys/:id`
- `/api/events`
- `/api/model-redirects/:id`
- `/api/monitor/session`
- `/api/oauth/connections/:accountId`
- `/api/oauth/route-units/:routeUnitId`
- `/api/routes/:id`
- `/api/site-announcements`
- `/api/sites/:id`
- `/api/test/chat/jobs/:jobId`
- `/api/test/delete`
- `/api/test/proxy/jobs/:jobId`
