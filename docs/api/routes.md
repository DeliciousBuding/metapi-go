# Route & Channel Management

> **Index**: back to [API Reference](../api.md). This file is the Routes, channels & route decision domain split out of the pre-`docs/api/` `docs/api.md`.

## Models & Routes

### GET /api/imported-upstreams

Returns `{items, members}` for imported direct-upstream channels. `items` contains
channel ID, name, origin key, dialect, base URL, availability, protocol paths and
model/credential counts. `members` connects group items to route, model and
credential names and authorized outbound protocol bits (Chat 2, Responses 4,
Messages 8, Gemini 16). Each member's `protocolOrder` restricts and orders that
route item's outbound protocols; an empty list retains the grant's protocols.
`items[].endpointConfig` contains optional `chat`, `responses`, `messages` and
`gemini` entries with resolved `url`, `auth` (`bearer`, `x-api-key`, or
`x-goog-api-key`), and optional Gemini `modelPath`. When `modelPath` is true,
the URL is a models prefix and the selected model and generation action are
appended at execution; otherwise it is the exact upstream endpoint URL.
The optional endpoint `profile` selects the audited `codex` or `claudecode`
request contract. The channel's `provider` identifies its source provider;
it is separate from its wire protocol and credential kind.
This is configuration inventory, not a protocol-health probe. Credential values,
custom headers and parameter overrides are never included.
Members also expose persisted grant success/failure counts and nullable cooldown
time/reason code. Imported grant failures use the gateway's bounded Fibonacci
backoff and are excluded from subsequent selection until that cooldown expires;
success clears cooldown. Health is per model/credential grant, shared across its
group memberships, and does not change unrelated grants. Imported historical
statistics are retained separately and are not treated as live health evidence.

Each member includes `grantId`, `channelEnabled`, `modelEnabled`,
`credentialEnabled`, `grantEnabled`, `groupEnabled`, `routeEnabled`, and
`selectedByGroup` (false for an unselected manual-group member). `effectiveEnabled`
is their conjunction; cooldown remains a separate temporary state. A shared grant
may appear in more than one group, so changing its credential or clearing its
cooldown affects every membership.

Channels with an explicit `endpointConfig` can translate generation requests
from any of the four client protocols. Legacy base/path grants retain their
original protocol permission checks. Selection prefers
the client's protocol when it is in the route item's allowed list, otherwise the
first allowed outbound protocol is converted using the existing transform
packages. Native requests retain their original body apart from model mapping
and configured overrides. Cross-protocol requests with nonportable continuity,
signed reasoning or unsupported tools fail explicitly instead of losing fields.

### GET /api/imported-upstreams/:id

Returns a flat editable connection object: `id`, `originKey`, `provider`, `dialect`,
`name`, `enabled`, `baseUrl`, `endpointConfig`, `openaiChatCompletionPath`,
`openaiResponsePath`, `anthropicMessagePath`, and `useSystemProxy`. The three
request-configuration values below are omitted; only `hasChannelProxy`,
`channelProxyDisplay` (without user information), `hasCustomHeaders`, and
`hasParamOverride` are returned. The response is `Cache-Control: no-store`.

### GET /api/imported-upstreams/:id/request-config

Explicitly retrieves `{channelProxy, customHeaders, paramOverride}` for editing.
These values can contain sensitive operator configuration. Fetch only when the
operator opens this editor, do not put the response in persistent or query caches,
and clear editor state on close. The response is `Cache-Control: no-store`.
Upstream credential secrets and OAuth state are never returned by either GET.

### PATCH /api/imported-upstreams/:id

Accepts a non-empty JSON object containing any editable connection fields above,
plus `channelProxy`, `customHeaders`, and `paramOverride`. Identity fields
`id`, `originKey`, `provider`, and `dialect` are read-only. Omitted fields retain
their values. Explicit empty strings clear proxy/header/parameter configuration.
`customHeaders` is a JSON string containing an array of
`{header_key, header_value}` objects; `paramOverride` is a JSON object string.
Only supported client-header templates are accepted. `useSystemProxy` selects the
gateway's configured system proxy when `channelProxy` is empty.

`endpointConfig` replaces the whole object. Endpoint URLs are exact, credential-free
HTTP(S) URLs; authentication is `bearer`, `x-api-key`, or `x-goog-api-key`.
`modelPath` is only valid for Gemini. `codex` and `claudecode` profiles require
their matching provider, protocol, and Bearer authentication; `deepseek` and `zai`
profiles require Chat and Bearer. Empty profile keeps the generic wire contract.
Endpoints still referenced by a grant and required provider profiles cannot be
removed. Configured endpoints cannot silently revert to legacy base/path routing.
Changing `baseUrl` does not rewrite exact URLs in `endpointConfig`.

Updates are atomic. Unknown fields, nulls, duplicate JSON keys, trailing JSON,
invalid types, URLs, templates, and protocol/profile combinations return 400.
Missing records return 404, conflicting names/updates return 409, and storage
failures return 500. Success returns `{success, id, enabled}` and invalidates routing
caches. Re-import remains the owner of the source graph: importing the same origin
again can overwrite local connection, credential, and membership edits.

### PATCH /api/imported-upstreams/members/:id

Updates any of `priority` (signed int32), `weight` (positive int32), and
`protocolOrder` (unique protocol bits 2/4/8/16). A non-empty order must be a subset
of the immutable shared grant; an explicit empty array restores inheritance from
that grant. Omission retains the current restriction. Returns `{success, id}`.

### POST /api/imported-upstreams/members/:id/cooldown/clear

Clears the member's shared grant cooldown and reason, preserving health counters.
Returns `{success, id, grantId}`; all memberships of this grant are affected.
Routing cache invalidation takes effect on the next selection.

### GET /api/imported-upstreams/:id/credentials

Authenticated administrators receive `{items}` with each credential's `id`,
`name`, `enabled`, `kind` (`api_key` or `oauth`), optional `expiresAt` in Unix
milliseconds, and `canRefresh`. Neither access nor refresh tokens are returned.

### PATCH /api/imported-upstreams/credentials/:id

Authenticated administrators can change `name`, `enabled`, or replace a credential.
Supply either `apiKey` or an `oauth` object, never both. An OAuth replacement
requires `accessToken` and may include `refreshToken`, `clientId`, `expiresAt`,
`idToken`, and `accountId`. This is a complete credential replacement; omitted
replacement fields are not inherited from the previous credential.
Unknown fields and invalid combinations return 400; missing IDs return 404,
duplicate names within the channel return 409, and storage failures return 500.
Success returns `{success: true, id}` and invalidates routing caches.

Codex/Fenno and Claude Code OAuth credentials are read immediately before
dispatch and refreshed when needed through their existing OAuth providers.
Concurrent requests share a refresh, and a compare-and-swap update prevents
an in-flight refresh from overwriting a replaced credential. Codex/Fenno use
upstream streaming even for a JSON client; a complete terminal response is
required before producing JSON. Native Claude Code tool names are restored
before any downstream protocol conversion.

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

`enabled` is the channel's own configured switch; `status` is its effective runtime state. A disabled parent site
or account yields `manually_disabled` without rewriting the channel switch or disabling a model route shared
with other sites. The nested `site.status` identifies the parent state. Site updates/deletes invalidate the
channel list and error-summary snapshots immediately, as well as the selector cache.

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

Recompose automatic channels from model availability. With the runtime setting `autoCreateModelRoutes: true` (default `false`), the pass also creates missing exact-model routes. Existing patterns, explicit groups, disabled routes, manual bindings and downstream-key grants remain operator-owned. Delisted models lose automatic channels; their route IDs remain stable for future recovery.

**Body**: `{ "refreshModels": true, "wait": false }`. `refreshModels` defaults to `true`: refresh upstream models first, then rebuild once for the batch. `false` uses stored availability. Explicit `wait: false` queues a background task; `wait: true` or omission preserves synchronous behavior for existing callers.

**Queued response** (202): `{ "success": true, "queued": true, "reused": false, "jobId": "…", "taskId": "…", "status": "pending" }`. Observe `GET /api/tasks/{taskId}` until `succeeded` or `failed`; its `result` carries the same statistics as a synchronous response. Identical in-flight requests in the same running process reuse the same task. A task requiring upstream refresh is not reused for a local-only rebuild. Execution is process-local and survives browser navigation/disconnection, not a server restart; normal task persistence/retention governs later visibility.

**Synchronous/completed result**: `{ "success": true, "queued": false, "reused": false, "status": "completed", "routesCreated": 1, "unsafeModelsSkipped": 0, "routesConsidered": 3, "patternRoutes": 2, "groupRoutes": 1, "channelsInserted": 4, "channelsRemoved": 1, "channelsKept": 2, "changed": true }`. When refreshing models, `modelRefresh` additionally contains `{ total, success, failed, notProcessed }`; nonzero `failed` or `notProcessed` means the upstream refresh was partial, even if local channel recomposition finished.

`routesCreated` counts new route rows; `changed` covers route creation and channel changes. `unsafeModelsSkipped` counts upstream names containing glob/regex syntax that cannot safely become literal routes. `routesConsidered` counts all routes, including explicit groups whose membership is not rewritten. Zero routes after completion means no route was configured/generated, not successful model forwarding.

Automatic creation requires observed model availability and a usable relay token or account/OAuth credential. Existing route patterns (including disabled ones) take precedence, so automatic creation cannot silently override a manual routing policy. With creation disabled, existing automatic channels still synchronize. A successful empty upstream list retires obsolete automatic availability; an authorization/network failure retains the last observed snapshot and is counted as a refresh failure.

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
