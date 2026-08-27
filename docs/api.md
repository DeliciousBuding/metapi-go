# Admin API Reference

**Last updated**: 2026-08-23
**Coverage note**: every `/api` route registered by the Go server gets a `### METHOD /path` detail section — the authoritative route list is the [Admin Route Inventory](#admin-route-inventory) at the bottom. Backward-compat aliases (e.g. `GET /api/stats/model-prices`) are documented as aliases rather than duplicating the canonical handler.

Base URL: `http://localhost:4000/api`

All admin endpoints require authentication via Bearer token:

```
Authorization: Bearer <AUTH_TOKEN>
```

## Response Format

All responses are JSON (`Content-Type: application/json`) with camelCase
field names. There is no single global envelope; the shape depends on the
endpoint family (the frontend consumes all of the shapes below):

### Success (2xx)

- **Resource lists (legacy shape)**: a bare JSON array of objects
  (`GET /api/sites`, `GET /api/routes` without `?page`, `GET /api/account-tokens`).
- **Paginated lists**: `{ "items": [...], "total": N, "page": N, "pageSize": N }`
  (`GET /api/channels`, `GET /api/accounts?page=`, `GET /api/routes?page=`,
  `GET /api/downstream-keys?page=`, `GET /api/checkin/logs`).
  `total` is the true filtered row count, not the page size.
- **Snapshot lists**: `{ "generatedAt": "...", "accounts": [...], "sites": [...] }`
  (`GET /api/accounts` without `?page`).
- **Operation results**: `{ "success": true, ... }` with operation-specific
  fields (`POST /api/routes/rebuild`, downstream-keys summary, batch
  endpoints). Batch endpoints report partial failures per item.

### Error (non-2xx)

Failures always use a non-2xx status code (never HTTP 200 with an error
body). The unified error body is:

```json
{
  "error": "Error description",
  "request_id": "optional correlation id"
}
```

`request_id` is additive and omitted when no request ID is present. A few
legacy endpoints (some batch/500 paths under `/api/accounts`, sites import)
still answer with the TS-era `{ "message": "Error description" }` shape; the
frontend reads both keys, so both forms surface identically.

HTTP status codes: 200 (OK), 201 (Created), 202 (Accepted), 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 500 (Internal Server Error).

## Request Body Rules

Request bodies are capped at 20 MiB by the HTTP router. Admin JSON handlers also apply the same cap when decoding, so direct route handlers still read bounded input.

Admin JSON requests must contain one JSON value. Duplicate object keys and trailing JSON values are rejected with `400`.

---

## Billing & Currency

All money fields are denominated in **US dollars (USD)** — `balance`, `quota`, `balanceUsed`, `todayIncome`, `todayQuotaConsumption`, `unitCost`, `estimatedCost`, and the per-million pricing rates. There is no RMB path or multi-tenant wallet; the admin console renders `$` as a fixed currency symbol (not translatable interface copy).

Upstream balance semantics: NewAPI/OneAPI-family platforms expose integer `quota` where **1 USD = 500000 quota**; the `veloera` adapter uses **1 USD = 1000000 quota**. Platform adapters normalize to USD at the boundary (`quota / divisor`), so the API and frontend never carry raw upstream quota.

Pricing is ratio-based: `modelRatio` / `completionRatio` / `groupRatio` are multipliers over a base rate. At ratio 1, input costs **$2 per 1M tokens** (the NewAPI `0.002`/1K convention); output uses `modelRatio × completionRatio`, and prompt-cache tiers use `cacheRatio` / `cacheCreationRatio` (Claude defaults 0.10 / 1.25). Per-million rates and `estimatedCost` are estimates, never account-priced; `unitCost` is a display/planning field.

---

## Stats & Dashboard

### GET /api/stats/dashboard

Returns the admin dashboard snapshot.

**Response**: Site/account/token counts, total cost, usage summaries.

### GET /api/stats/proxy-logs

Query proxy request logs (server-side filtered and paginated).

**Query params**: `view` (`full`|`query`|`meta`, default `full`), `limit` (1–100, default 50), `offset` (≥0), `status` (`success`|`failed`), `search` (substring on requested/actual model), `client` (exact client family), `siteId`, `channelId`, `from`/`to` (RFC3339 bounds on createdAt), `latencyMin`/`latencyMax` (ms bounds).

**Response** (`view=query`/`full`): `{ items, total, page, pageSize }` where `total` respects all active filters; `view=meta`/`full` adds `summary` (totalCount/successCount/failedCount/totalCost/totalTokensAll), `sites`, `clientOptions`.

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

### GET /api/stats/attention

Severity-ranked actionable items for the dashboard ("what needs my eyes"): expired accounts (critical), low-balance accounts with `balance < 1.0` (warning), disabled sites (warning), and the last 24h of warning/error events.

**Query params**: `limit` (default 20, max 100).
**Response** (200): `{ "items": [ { "severity": "critical", "category": "expired_account", "label": "账号已过期：svc-1", "target": "/accounts?accountId=5", "createdAt": "2026-08-22T00:00:00Z" } ], "total": 1 }`. `severity` is `critical` | `warning` | `info`; `category` is `expired_account` | `low_balance` | `disabled_site` | `event`; `target` is a frontend deep link.

### GET /api/stats/balance-history, GET /api/stats/balance-income-outcome

Daily balance snapshots per account (`balance-history`) and the derived income/outcome accounting (`balance-income-outcome`).

**Response** (`balance-history`): `{ series: [{ accountId, points: [{ day, balance, balanceUsed, quota, capturedAt }] }], days }` — only days with actual snapshots are emitted.
**Response** (`balance-income-outcome`): `{ generatedAt, days, points: [{ day, income, outcome, net }], summary: { totalIncome, totalOutcome, net, accounts } }` — derived from the snapshots via `income - outcome = Δbalance`; an account's first snapshot day counts as initial income.
**Query params** (both): `days` (1–365, default 30), `accountId` (optional).

### GET /api/stats/latency-histogram, GET /api/stats/latency-trend

Request-count histogram over `latency_ms` buckets (`latency-histogram`) and the per-day latency/throughput series (`latency-trend`).

**Query params** (both): `days` (1–90, default 7); histogram adds `bucketMs` (100–60000, default 500).
**Response** (`latency-histogram`): `{ days, since, bucketMs, total, buckets: [{ bucketStartMs, bucketEndMs, label, count, percent }] }` — zero-count buckets are omitted.
**Response** (`latency-trend`): `{ days, points: [{ date, requests, avgLatencyMs, maxLatencyMs, avgFirstByteMs, p95LatencyMs, successRate }], truncatedDays }` — `truncatedDays` flags days whose p95 sample exceeded the 10000-row cap (honest under-reporting).

### GET /api/stats/model-cost-distribution

Top-N model cost concentration aggregated from `proxy_logs`; everything below top-N is grouped as `other`.

**Query params**: `days` (1–90, default 30), `topN` (1–20, default 8).
**Response** (200): `{ "days": 30, "since": "2026-07-24T00:00:00Z", "topN": 8, "items": [ { "model": "gpt-4o", "label": "gpt-4o", "cost": 1.23, "calls": 1200, "tokens": 450000 } ], "totals": { "cost": 4.56, "calls": 3000, "tokens": 1000000 } }`

### GET /api/stats/model-prices

Alias of `GET /api/models/price-compare` (same handler) — kept for frontend/back-compat.

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

### GET /api/channels

Full route-channel list (5-way JOIN) with a 30s snapshot cache; `?refresh=true` bypasses it.

**Query params**: `page`/`pageSize` (when present the response is paginated; without them the bare full shape is returned and `pageSize` reports the real row count), `refresh`.

**Response** (200): `{ "items": [ { "id": 12, "routeId": 1, "name": "svc-1", "site": { "id": 3, "name": "anthropic" }, "type": "account", "status": "enabled", "models": "gpt-4o", "priority": 10, "weight": 20, "responseMs": 842, "cooldownUntil": null, "cooldownReasonCode": null, "cooldownReason": null, "cooldownReasonAt": null, "enabled": true, "manualOverride": false } ], "total": 1, "page": 1, "pageSize": 1 }`

`type` is `account` | `token` | `oauth_unit`; `status` is `enabled` | `cooldown` | `breaker_open` | `manually_disabled`.

**Cooldown reason fields**: `cooldownReasonCode` / `cooldownReason` / `cooldownReasonAt` describe why the channel entered cooldown. All three are `null` when no reason was recorded (rows cooled before the structured-reason schema existed). Codes are a stable, append-only vocabulary: `usage_limit` | `rate_limited` | `auth_error` | `upstream_error` | `client_error` | `timeout` | `network_error` | `probe_failure` | `unknown`. `cooldownReason` is a sanitized error summary truncated to 200 runes; `cooldownReasonAt` is the ISO-8601 UTC time the triggering failure was recorded.

### GET /api/channels/probe-history

Recent background model-probe history per channel — the data behind the row-level probe health bars on the channels page. One bounded query covers every channel that has history (windowed to the newest `limit` results each); channels without probes are omitted from `items`.

**Query params**: `limit` — results per channel, clamped to 1–50 (default 20).

**Response** (200): `{ "limit": 20, "items": [ { "channelId": 12, "results": [ { "id": 401, "status": "success", "latencyMs": 842.5, "httpStatus": 200, "errorText": null, "modelName": "gpt-4o", "createdAt": "2026-08-28T02:00:00Z" } ] } ] }`

`status` shares the probe vocabulary: `success` | `failure` | `inconclusive` | `skipped`. `latencyMs`/`httpStatus`/`errorText` are null when the probe produced no such signal. Results are ordered newest-first within each channel.

### POST /api/routes

Create a new route.

### PUT /api/routes/:id

Update an existing route.

### DELETE /api/routes/:id

Delete a route and its channels.

### POST /api/routes/batch

Batch enable/disable/delete routes. Body: `{ "ids": [1, 2, 3], "action": "enable" }`.

### PUT /api/routes/reorder

Persist route sort order. Body: `{ "items": [{ "id": 1, "sortOrder": 10 }] }` — max 1000 items; `sortOrder >= 0`; duplicate ids in the payload are rejected per item.

**Response**: `{ success, successIds, failedItems }` — `success` is `false` when any item failed.

### POST /api/routes/rebuild

Trigger route rebuild. Body: `{ "refreshModels": true }`.

### POST /api/routes/:id/cooldown/clear

Clear cooldown state for all channels on a route: resets `cooldown_until`, `consecutive_fail_count`, `cooldown_level`, and the structured reason fields (`cooldown_reason_code` / `cooldown_reason` / `cooldown_reason_at`) back to their neutral values.

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

### GET /api/models/token-candidates

Available token candidates for route configuration.

### POST /api/models/check/:accountId

Check model availability for a specific account. Returns `{ "success": true, "models": [...] }`.

### POST /api/models/probe

Trigger a model probe. Body: `{ "models": ["gpt-4o"], "wait": false }`. Returns 202 with probe job.

### POST /api/models/verify-batch, GET /api/models/verify-history

Operator-initiated verification of route-channel model targets (same lightweight probe the background scheduler uses) with durable per-row history, and the recent verification rows.

**Body** (verify-batch): `{ "models": ["gpt-5"], "accountId": 0, "limit": 50 }` — empty `models` = all enabled route channels; `accountId > 0` narrows to one account; `limit` 1–200 (default 50). 503 when the probe scheduler is not running.
**Response** (verify-batch): `{ "success": true, "batchId": "vb-...", "probed": 2, "summary": { "success": 1, "failure": 0, "inconclusive": 1, "skipped": 0 }, "items": [ { "model": "gpt-4o", "channelId": 12, "accountId": 5, "siteId": 3, "status": "success", "latencyMs": 842, "httpStatus": 200, "errorText": "", "healthApplied": true } ] }` — no matching targets answers with `probed: 0` and a `note`.
**Query params** (verify-history): `limit` (default 50, max 200), `model` (exact match). Response: `{ items: [{ id, batchId, model, channelId, accountId, siteId, siteName, status, latencyMs, httpStatus, errorText, createdAt }] }`.

---

## Model Redirects

Canonical→actual model name mappings, created by availability sync (`source: sync`) or operator edits (`source: manual`). Rows live in `model_name_redirects` and feed the in-process redirect registry (`service.ReloadRedirectRegistry`), so every write below reloads it immediately.

### GET /api/model-redirects, PUT /api/model-redirects/{id}, DELETE /api/model-redirects/{id}

List, correct, and delete redirect mappings.

**Query params** (list): `accountId`, `source` (`sync`|`manual`).
**Body** (update): `{ "actual": "gpt-4o", "source": "manual" }` — partial update; empty `actual` or an unknown `source` is 400, "nothing to update" is 400, missing row is 404.

**Response** (list): `{ items: [{ id, accountId, username, siteName, canonical, actual, source, lastSeenAt, createdAt, updatedAt }] }` (updatedAt descending). Update/delete: `{ "success": true }`.

### POST /api/model-redirects/generate, POST /api/model-redirects/apply

Generate sync mappings idempotently (per account or all accounts), or apply redirect fixes to models disabled by availability drift.

**Body** (generate): `{ "accountId": 0, "models": ["gpt-4o"] }` — `accountId: 0` = all accounts; omitted `models` = every available model of the target account; manual mappings are never overwritten. Response: `{ success, created }` (all-accounts mode adds `accounts`).
**Body** (apply): `{ "dryRun": false }` (default `true` = report only). Dry run: `{ success, dryRun: true, candidates, count }`; applied: `{ success, dryRun: false, removed, count }` — each removal writes a `model_redirect_applied` event.

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

### POST /api/sites/batch

Batch enable/disable/delete sites and toggle their system-proxy usage. Body: `{ "ids": [1, 2], "action": "enable|disable|delete|enableSystemProxy|disableSystemProxy" }`.

**Response**: `{ success, successIds, failedItems }` — site/routing caches are invalidated after any mutating action.

### POST /api/sites/{id}/probe-now, GET /api/sites/{id}/probe-stream

Probe the site's active channels (bounded to 32 targets, 8-way parallel, 30s wall clock): synchronously for `probe-now`, as server-sent events for `probe-stream`.

**Body** (probe-now, optional): `{ "modelName": "gpt-4o" }` — filters which results are surfaced.
**Response** (probe-now): `{ success, totalModels, available, unavailable, results: [{ channelId, accountId, model, status, latencyMs, error? }], complete }` — `complete: false` adds `truncated` and `reason`. `status` is `success` | `failure`.
**Stream** (probe-stream): `text/event-stream`; events are `probe-start` (`{ startedAt, streaming, modelFilter }`), one `probe-result` per probed model (same row shape), `complete` (`{ totalModels, available, unavailable }`), then a `data: [DONE]` sentinel. `?modelName=` filters streamed results. The stream clears the server WriteTimeout so long probe passes are not cut off.

### GET /api/sites/{id}/available-models, GET /api/sites/{id}/disabled-models, PUT /api/sites/{id}/disabled-models

Available (account+token merged, case-insensitive sorted) and disabled model lists for a site.

**Response** (GETs): `{ siteId, models: ["gpt-4o", ...] }`.
**Body** (PUT disabled): `{ "models": ["gpt-4o"] }` — deduped/trimmed, full replace; returns `{ siteId, models }` and invalidates the routing cache.

### PUT /api/sites/{id}/tags, PUT /api/accounts/{id}/tags, GET /api/tags

Global tag system: per-row writes and the aggregated index driving the Accounts/Sites filter chips.

**Body** (PUT): `{ "tags": ["prod", "priority"] }` (a raw JSON array is also accepted). Tags are trimmed/deduped and stored as JSON-array text; returns `{ success, tags }` (400 for a malformed body, 404 for unknown rows).
**Response** (GET): `{ items: [{ name, accounts, sites, total }] }` — union of all account + site tags, sorted by total usage descending (`total` = accounts + sites).

---

## Accounts

### GET /api/accounts, POST /api/accounts

List all accounts. Create a new account.

### GET /api/accounts/:id, PUT /api/accounts/:id, DELETE /api/accounts/:id

Get, update, delete an account.

### GET /api/accounts/probe-history

Recent background model-probe history per account — the data behind the row-level probe health bars on the accounts page. One bounded query covers every account that has history (windowed to the newest `limit` results each); accounts without probes are omitted from `items`. Rows recorded without a channel (account-level probes) are included here.

**Query params**: `limit` — results per account, clamped to 1–50 (default 20).

**Response** (200): `{ "limit": 20, "items": [ { "accountId": 5, "results": [ { "id": 402, "status": "failure", "latencyMs": null, "httpStatus": 401, "errorText": "unauthorized", "modelName": "gpt-4o", "createdAt": "2026-08-28T02:00:00Z" } ] } ] }`

`status` shares the probe vocabulary: `success` | `failure` | `inconclusive` | `skipped`. Results are ordered newest-first within each account.

### POST /api/accounts/login

Log in against the site with username/password and create the account — or update the existing one for `(siteId, username)` — with the session token and an encrypted `autoRelogin` credential (`extraConfig.credentialMode = session`).

**Body**: `{ "siteId": 3, "username": "me@example.com", "password": "..." }` — `siteId`/`username`/`password` are required; unsupported platforms are 400, failed logins are 401 with the platform message.
**Response** (200): `{ "success": true, "account": { "id": 12, "siteId": 3, "username": "me@example.com", "accessToken": "sk-...", "apiToken": null, "balance": 9.5, "status": "active", "isPinned": false, "sortOrder": 1, "checkinEnabled": true, "extraConfig": "{}", "createdAt": "...", "updatedAt": "..." }, "apiTokenFound": false, "tokenCount": 1, "reusedAccount": false }`

### POST /api/accounts/verify-token

Verify an access token against the site (token type, models, user info, balance) without saving it.

**Body**: `{ "siteId": 3, "accessToken": "sk-...", "platformUserId": 123, "credentialMode": "session" }` — `accessToken` is required.
**Response**: `{ success, tokenType, modelCount, models, userInfo, balance, apiToken, apiTokenFound }` — 400 `token verification failed` when the token type is unknown.

### POST /api/accounts/batch, POST /api/accounts/health/refresh

Batch enable/disable/delete/balance-refresh accounts, or refresh runtime health (balance probe + persisted runtime health) for all accounts or one account.

**Body** (batch): `{ "ids": [1, 2], "action": "enable|disable|delete|refreshBalance" }` — deletes trigger a route rebuild; enable/disable invalidate the routing cache. Response: `{ success, successIds, failedItems, skippedItems }` (`skippedItems` only for `refreshBalance`).
**Body** (health): `{ "accountId": 5, "wait": true }` (default `wait: false` = background task).
**Response** (health, wait): `{ success, summary: { total, healthy, unhealthy, degraded, disabled, unknown, success, failed, skipped }, results: [{ accountId, status, state, reason, message, proxyOnly? }], message }` — `status` is `success` | `failed` | `skipped`; `state` is the persisted runtime-health state. Queued: `202 { success, queued, reused, jobId, taskId, status, message }`; progress via `GET /api/tasks/{id}`.

### POST /api/accounts/{id}/balance

Refresh one account's balance/quota now. Disabled accounts/sites and API-key connections do not probe — they return the stored values with `skipped: true`.

**Response**: `{ balance, balanceUsed, quota, skipped, reason }` — `reason` is `account_disabled` | `site_disabled` | `proxy_only` when skipped; 404 for unknown account or unsupported platform, 502 on upstream failure.

### GET /api/accounts/{id}/models, POST /api/accounts/{id}/models/manual

The account's proven model list (with site-disabled and operator-pinned flags) and the pinned manual-model upsert.

**Response** (GET): `{ siteId, siteName, models: [{ name, latencyMs, disabled, isManual }], totalCount, disabledCount }`.
**Body** (manual): `{ "models": ["gpt-4o"] }` — upserts `model_availability` rows with `is_manual = true`, invalidates the routing cache. `{ "success": true }`; empty model list is 400.

### POST /api/accounts/{id}/rebind-session

Replace a session account's access token (session-credential rebind; Sub2API auth merges `refreshToken`/`tokenExpiresAt` into `extraConfig.sub2apiAuth`).

**Body**: `{ "accessToken": "sk-...", "refreshToken": "...", "tokenExpiresAt": 1710000000 }` — `accessToken` is required (400 otherwise).
**Response**: `{ success, account, tokenType: "session", credentialMode: "session", capabilities, apiTokenFound: false }`.

---

## Account Tokens

### GET /api/account-tokens, POST /api/account-tokens

List all account tokens. Create a new account token.

### GET /api/account-tokens/:id, PUT /api/account-tokens/:id, DELETE /api/account-tokens/:id

Get, update, delete an account token.

### GET /api/account-tokens/{id}/value

Reveal one token for copy, in display and masked forms. API-key connections are refused (400).

**Response**: `{ success, id, name, token, tokenMasked }` — `409` when only a masked placeholder is stored ("待补全令牌").

### GET /api/account-tokens/groups/{accountId}, GET /api/account-tokens/account/{accountId}/default

Upstream token groups of an account, and the account's current default local token (masked).

**Response** (groups): `{ success, groups }`; (default): `{ success, token }` — `token: null` when the account has no default token or is an API-key connection. The default token object uses the masked list shape `{ id, accountId, name, tokenGroup, valueStatus, source, enabled, isDefault, tokenMasked, createdAt, updatedAt }`.

### POST /api/account-tokens/batch, POST /api/account-tokens/{id}/default

Batch enable/disable/delete tokens, or mark one token as the account default (clears the previous default).

**Body** (batch): `{ "ids": [1, 2], "action": "enable|disable|delete" }` — deletion also removes the token upstream; disabling the default token repairs the account default. Response: `{ success, successIds, failedItems }`.
**Response** (default): `{ "success": true }` — 400 for masked-pending tokens and API-key connections.

### POST /api/account-tokens/sync/{accountId}, POST /api/account-tokens/sync-all

Sync one account's tokens from upstream (session credential token list), or all active accounts.

**Response** (one): `{ "success": true, "accountId": 12, "status": "synced", "synced": true, "created": 1, "updated": 0, "total": 3, "maskedPending": 0, "defaultTokenId": 7, "message": "..." }` — `status: "skipped"` with `reason` `site_disabled` | `apikey_connection` | `no_access_token` | `no_upstream_tokens` | `unsupported_platform` keeps `synced: false`; sync failures write a `token_sync` error event and answer 502.
**Body** (sync-all): `{ "wait": true }` (default `false` = background task). Wait mode returns `{ success, summary: { total, synced, skipped, failed, created, updated }, results: [...] }`; queued mode returns `202 { success, queued, reused, jobId, taskId, status, message }`.

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

## Announcements

Operator-authored severity-ranked product banners (替代邮件群发). The dashboard renders the `active` list; editing content resets the dismissal so a new revision surfaces again (dismiss-revision semantics).

### GET /api/announcements, GET /api/announcements/active

Admin list (all announcements with dismissal state) and dashboard list (enabled and not dismissed).

**Response**: `{ items: [{ id, title, message, severity, link, enabled, dismissed, dismissedAt, createdAt, updatedAt }] }` — `active` filters `enabled = TRUE` in SQL and drops dismissed rows in Go; both sort by severity (`critical` first) then updatedAt descending.

### POST /api/announcements, PUT /api/announcements/{id}, DELETE /api/announcements/{id}

Create, update, delete an announcement.

**Body**: `{ "title": "...", "message": "...", "severity": "warning", "link": "https://...", "enabled": true }` — `title`/`message` required; `severity` is `info` (default) | `warning` | `critical`.
**Response**: create `{ success, items }` (reloaded list); update `{ success, revision }` where `revision: true` when the content changed (dismissal reset); delete `{ "success": true }` (404 for unknown id).

### POST /api/announcements/{id}/dismiss

Record a dismissal for the current operator (upsert). `{ "success": true }`; 404 for unknown id.

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

### DELETE /api/events

Delete all system events. `{ "success": true }`.

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

### POST /api/downstream-keys/batch

Batch enable/disable/delete/reset-usage/updateMetadata on downstream keys. Body: `{ "ids": [1, 2], "action": "enable|disable|delete|resetUsage|updateMetadata", "groupName": "prod", "groupOperation": "set|clear" }` — `groupOperation`/`groupName` only apply to `updateMetadata`.

**Response**: `{ success, successIds, failedItems }`.

---

## Settings

### GET /api/settings/runtime

Get all runtime settings as a flat JSON object. Sensitive values (proxyToken, tokens, passwords) are masked. The response includes branding fields (`systemName`, `logo`, `footer`, `about`, `serverAddress`) and semantic schedule mirrors (`checkinSchedule`, `balanceRefreshSchedule`, `logCleanupSchedule`).

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

Queues a migration of the live runtime database onto an external target as an
admin background task and returns `202` with the task ID:
`{ "success": true, "taskId": "...", "task": {...} }`. Progress is observed by
polling `GET /api/tasks/{id}` until the task reaches `succeeded` (result
carries per-table row counts) or `failed` (error carries the reason). The
migration runs the same `store.RunMigration` code path as the `metapi-migrate`
CLI; it is not wired to any remote helper.

Returns `400` when the dialect is unsupported, the connection string is empty,
or the target resolves to the live runtime database (migrating onto the
database currently in use is refused).

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

### POST /api/settings/backup/import/preview

Preview a backup import without writing anything (F1). Same body shapes as `POST /api/settings/backup/import` (`{ "tables": {...} }`, optional `{ "data": { "tables": {...} } }` wrapper, TS backup v2.1 payloads).

**Response**: `{ success, plan: { "<table>": { rows, toInsert, duplicates, skippedRows } } }` — `duplicates` are rows whose PK already exists in the target DB (they would be dropped by `ON CONFLICT DO NOTHING`); `skippedRows` are runtime-local settings skipped by policy. No rows are written.

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

Local-only residual surface. Version updates ship via GitHub Releases / GHCR
image tags; the admin UI renders this section read-only with external links
and does not depend on in-app update discovery. Honest residuals: the
status/check payloads always report `updateAvailable: false` and expose a
`residual` string describing the limitation; deploy/rollback/task-stream
return `501` rather than inventing task ids or fake SSE streams.

### GET /api/update-center/status

Local status only. Returns `200` with
`{ "currentVersion": "0.0.0", "latestVersion": "0.0.0", "updateAvailable": false, "lastCheckedAt": null, "mode": "external", "residual": "..." }`.
Never invents `updateAvailable: true` or a fake `lastCheckedAt`.

### POST /api/update-center/check

Same local-only payload as status (no remote registry polling, no deploy
tasks started).

### PUT /api/update-center/config

Echo-only: accepts the config body and returns it with `success: true` plus a
`residual` note. Not persisted as an update-center product config; deploy and
rollback remain residual regardless of the echoed values.

### POST /api/update-center/deploy

Returns `501` (`{ "success": false, "message": "Update-center deploy is not implemented in Go", "residual": "..." }`).
Remote binary/Helm deploy via a helper service is out of scope; use external
deploy tooling (CI/CD or helper).

### POST /api/update-center/rollback

Returns `501` — same honest residual shape as deploy.

### GET /api/update-center/tasks/:id/stream

Returns `501`. No deploy/rollback task registry exists, so there is no SSE log
stream to serve.

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

### GET /api/admin/audit-logs

List recent authenticated admin write operations (B1), newest first. GET/HEAD/OPTIONS are not recorded; the middleware records best-effort and never blocks a request.

**Query params**: `limit` (default 50, max 200), `offset`, `method` (exact `POST`/`PUT`/`PATCH`/`DELETE`), `path` (substring).
**Response**: `{ items: [{ id, actor, method, path, status, requestId, remoteIp, createdAt }], total, limit, offset }`. `actor` is the first 8 hex chars of the bearer token's SHA-256 — stable, non-reversible, never the token itself.

### GET /api/debug/vars

Runtime debug snapshot (Go memory/goroutine stats, `/debug/vars` parity for the TS-era operator surface).

**Response**: `{ goroutines, mem: { alloc, totalAlloc, sys, heapAlloc, heapSys, heapIdle, heapInuse, heapReleased, heapObjects, stackInuse, stackSys, numGC, numForcedGC, gcPauseTotalNs, lastGCUnixNs } }`.

### GET /api/admin/ops/ws

Live ops WebSocket (B2): one JSON frame per second over the current proxy-traffic window. **Auth**: `?token=<admin token>` — browser WebSocket cannot set the Authorization header, and the endpoint is mounted outside the header-auth group; the token is compared in constant time. CORS origins follow the `ADMIN_CORS_ALLOWED_ORIGINS` configuration (same-origin when unset).

**Frame**: `{ lifetime, points: [{ ts, total, success }] }` — points cover the last 300s (zero-filled), this instance's traffic only; no cross-instance aggregation. Invalid token: `403 {"message":"invalid token"}`.

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

Developer/QA test harness surfaces. The legacy TS-era `/api/test/read|write|update|patch|delete|boom` endpoints are **not registered** by the Go server; the inventory below only lists what the router actually mounts.

### POST /api/test/proxy, POST /api/test/chat

Sync forced-channel probe: delegates to the same forced-channel harness as `POST /api/admin/test-channel` when `channelId`/`siteId`/`forcedChannelId` is given; without a forced channel the full path/multipart matrix is a known limitation and the endpoint answers an honest `501` residual rather than inventing a successful probe.

**Body**: `{ "channelId": 12, "siteId": 3, "forcedChannelId": 12, "model": "gpt-4o-mini", "prompt": "ping", "mode": "chat", "timeoutMs": 15000, "path": "...", "jsonBody": {...}, "messages": [{ "role": "user", "content": "..." }] }` — `jsonBody`/`messages` are read for model/prompt extraction when the flat fields are absent. `mode` defaults to `chat` (or `models` when `path` contains `/models` and not `/chat`).
**Response**: same shape as `POST /api/admin/test-channel` (see above).

### POST /api/test/chat/stream, POST /api/test/proxy/stream, POST /api/test/chat/jobs, POST /api/test/proxy/jobs, GET /api/test/chat/jobs/{jobId}, GET /api/test/proxy/jobs/{jobId}, DELETE /api/test/chat/jobs/{jobId}, DELETE /api/test/proxy/jobs/{jobId}

Stream and async-job test surfaces — honest residuals (see `handler/admin/test.go`): stream and job-create answer `501` with a `residual` note (no fake SSE chunks, no stub job ids); job status/cancel answer `404` (no in-process `/api/test` job registry). Use the sync endpoints above or `POST /api/admin/test-channel` instead.

### POST /api/debug/channel-probe

Alias of `POST /api/admin/test-channel` (same handler).

---

## OAuth

### GET /api/oauth/providers

List OAuth providers.

### POST /api/oauth/providers/{provider}/start

Start OAuth authorization flow for a provider.

### GET /api/oauth/callback/{provider}

OAuth callback endpoint for a provider (acknowledges the browser redirect; session state was already issued by `start`/`rebind` and is validated on session lookup).

### GET /api/oauth/connections

Paginated list of OAuth-managed accounts (provider identity backfill runs once at startup, not per page load).

**Query params**: `limit` (1–200, default 50), `offset`.
**Response**: `{ connections/items: [{ accountId, siteId, provider, username, email, accountKey, planType?, projectId?, modelCount, modelsPreview, quota?, status, routeChannelCount, lastModelSyncAt?, lastModelSyncError?, proxyUrl?, useSystemProxy, routeParticipation?, site? }], total, limit, offset }` — `quota` is the cached `OauthQuotaSnapshot`; `routeParticipation` lists the route units this account feeds.

### GET /api/oauth/sessions/{state}, POST /api/oauth/sessions/{state}/manual-callback

Poll the flow state after `start`/`rebind`, or complete it with a manually pasted callback URL (for flows without a browser redirect).

**Response** (session): `{ provider, state, status, accountId?, siteId?, error?, createdAt, updatedAt }` — 404 when the session/state is unknown or expired (10m TTL).
**Body** (manual-callback): `{ "callbackUrl": "https://..." }` — required; unknown/expired state is 404, state mismatch is 400. Success: `{ "success": true }`.

### POST /api/oauth/connections/{accountId}/rebind, PATCH /api/oauth/connections/{accountId}/proxy, DELETE /api/oauth/connections/{accountId}

Rebind flow start, per-connection proxy update, and OAuth identity removal. Body (rebind): optional `{ "projectId", "proxyUrl", "useSystemProxy" }`; body (proxy PATCH): `{ "proxyUrl": "http://...", "useSystemProxy": true }` — empty body clears the proxy back to the system proxy.

**Responses**: rebind `{ provider, state, authorizationUrl, instructions, accountId }`; proxy PATCH `{ success, accountId, proxyUrl, useSystemProxy, refreshedRoutes, modelRefresh: { success, status, ... } }`; delete `{ "success": true }`. Non-OAuth accounts answer 404.

### POST /api/oauth/connections/{accountId}/quota/refresh, POST /api/oauth/connections/quota/refresh-batch

Refetch the upstream quota snapshot for one connection or a batch (batch runs 4-way concurrent). Batch body: `{ "accountIds": [1, 2] }` — omitted/empty body = all connections.
**Response** (single): `{ success, quota }` — `quota` is the persisted `OauthQuotaSnapshot` (`{ status, source, lastSyncAt?, lastError?, providerMessage?, subscription?, windows, lastLimitResetAt? }`); 404 for unknown/not-OAuth accounts.
**Response** (batch): `{ success, refreshed, failed, items: [{ accountId, success, quota?, error? }] }`.

### POST /api/oauth/import, POST /api/oauth/route-units, PATCH /api/oauth/route-units/{routeUnitId}, DELETE /api/oauth/route-units/{routeUnitId}

Stubs — each answers `{ "success": true }`. Connection import and route-unit state are orchestrated inside `service/oauth` (the connection list reports `routeParticipation`); these endpoints exist for UI path parity only.

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

---

## About

### GET /api/about

Build provenance of the running binary, rendered by the admin About page.

**Response**: `{ version, commit, buildTime, goVersion }`

`version` is the `-ldflags` injected build version (`dev` for local builds). `commit` is the git SHA and `buildTime` the UTC RFC3339 build timestamp, both injected by the Makefile / Dockerfile / release pipeline; they are **empty strings** for builds without injection (plain `go build`, plain `docker build`) and the UI renders an em-dash rather than a fabricated value. `goVersion` comes from `runtime.Version()` and is always populated. The endpoint reads no database, so it keeps answering while the database is unavailable.

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

Export a downstream key's full secret as one-click credentials profiles. **Query params**: `profile` (`all` default, or one profile id such as `openai` | `cherry` | `claude` | `codex` | `generic`).

**Response** (200): `{ "success": true, "formatVersion": "1.0.0", "keyId": 1, "keyName": "prod-key", "baseUrl": "http://localhost:4000", "profiles": [ { "id": "openai", "label": "...", "description": "...", "contentType": "text/plain", "content": "..." } ], "notes": ["..."] }`

The full secret is intentionally returned by this endpoint only (see `handler/admin/credential_export.go`); 400 for unknown profile ids.

---

## Proxy files (`/v1/files`)

OpenAI-compatible Files surface (proxy auth required). Forwards to the selected upstream channel; does not persist customer file bytes on Metapi disk.

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
- `/api/about`
- `/api/account-tokens`
- `/api/account-tokens/:id/value`
- `/api/account-tokens/account/:accountId/default`
- `/api/account-tokens/groups/:accountId`
- `/api/accounts`
- `/api/accounts/:id/models`
- `/api/admin/audit-logs`
- `/api/admin/ops/ws`
- `/api/admin/resin/status`
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
- `/api/models/token-candidates`
- `/api/models/verify-history`
- `/api/monitor/config`
- `/api/monitor/health`
- `/api/oauth/callback/:provider`
- `/api/oauth/connections`
- `/api/oauth/providers`
- `/api/oauth/sessions/:state`
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
- `/api/models/verify-batch`
- `/api/monitor/session`
- `/api/oauth/connections/:accountId/quota/refresh`
- `/api/oauth/connections/:accountId/rebind`
- `/api/oauth/connections/quota/refresh-batch`
- `/api/oauth/import`
- `/api/oauth/providers/:provider/start`
- `/api/oauth/route-units`
- `/api/oauth/sessions/:state/manual-callback`
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
- `/api/test/chat`
- `/api/test/chat/jobs`
- `/api/test/chat/stream`
- `/api/test/proxy`
- `/api/test/proxy/jobs`
- `/api/test/proxy/stream`
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
- `/api/update-center/config`

### PATCH
- `/api/oauth/connections/:accountId/proxy`
- `/api/oauth/route-units/:routeUnitId`

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
- `/api/test/proxy/jobs/:jobId`

