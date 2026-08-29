# Route & Channel Management

> **Index**: back to [API Reference](../api.md). This file is the Routes, channels & route decision domain split out of the pre-`docs/api/` `docs/api.md`.

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

Full route-channel list (5-way JOIN) with a 10s snapshot cache; `?refresh=true` bypasses it.

**Query params**: `page`/`pageSize` (when present the response is paginated; without them the bare full shape is returned and `pageSize` reports the real row count), `refresh`, `status` (optional comma-separated subset of the four `status` values below; filtering loads the full row set to read in-memory routing/breaker state, then pages the filtered result).

**Response** (200): `{ "items": [ { "id": 12, "routeId": 1, "name": "svc-1", "site": { "id": 3, "name": "anthropic" }, "type": "account", "status": "enabled", "models": "gpt-4o", "priority": 10, "weight": 20, "responseMs": 842, "cooldownUntil": null, "cooldownReasonCode": null, "cooldownReason": null, "cooldownReasonAt": null, "enabled": true, "manualOverride": false } ], "total": 1, "page": 1, "pageSize": 1 }`

`type` is `account` | `token` | `oauth_unit`; `status` is `enabled` | `cooldown` | `breaker_open` | `manually_disabled`.

**Cooldown reason fields**: `cooldownReasonCode` / `cooldownReason` / `cooldownReasonAt` describe why the channel entered cooldown. All three are `null` when no reason was recorded (rows cooled before the structured-reason schema existed). Codes are a stable, append-only vocabulary: `usage_limit` | `rate_limited` | `auth_error` | `upstream_error` | `client_error` | `timeout` | `network_error` | `probe_failure` | `unknown`. `cooldownReason` is a sanitized error summary truncated to 200 runes; `cooldownReasonAt` is the ISO-8601 UTC time the triggering failure was recorded.

### GET /api/channels/error-summary

Fleet-wide runtime status counts that cannot be derived from a SQL aggregate because circuit-breaker state lives in the routing in-memory health maps. `?refresh=true` bypasses the 10s cache; any `route_channels` mutation invalidates both this summary and the channel-list snapshot.

**Response** (200): `{ "total": 16, "errorCount": 3, "byStatus": { "enabled": 10, "cooldown": 2, "breaker_open": 1, "manually_disabled": 3 } }` — `errorCount` counts only `cooldown` and `breaker_open`; `manually_disabled` is operator intent and is excluded.

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

Rebuild the automatic channels of every existing pattern route from current model availability. The endpoint never creates routes; it only recomposes `route_channels` rows for routes that already exist and invalidates the routing cache.

**Body** (both fields optional; defaults shown): `{ "refreshModels": true, "wait": true }`. With `refreshModels: true` the upstream model list of every active account is refreshed first (periodic model-sync batch semantics: per-account failures are counted, never fatal), then channels are rebuilt from the refreshed availability. With `refreshModels: false` the rebuild runs against the availability rows already stored. `wait` is accepted for client compatibility; the handler is synchronous, so `queued` is always `false`.

**Response** (200): `{ "success": true, "queued": false, "reused": false, "status": "completed", "message": "route channels rebuilt and cache refreshed", "routesConsidered": 3, "patternRoutes": 2, "groupRoutes": 1, "channelsInserted": 4, "channelsRemoved": 1, "channelsKept": 2, "changed": true }`. `routesConsidered` counts every route row; explicit-group routes are counted but their membership is not rewritten here. All counts are `0` when nothing matched — callers should treat `routesConsidered: 0` as "no routes exist yet" and `changed: false` as "no channel movement" 

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
