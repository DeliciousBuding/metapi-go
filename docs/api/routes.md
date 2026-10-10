# Route & Channel Management

> **Index**: back to [API Reference](../api.md). This file is the Routes, channels & route decision domain split out of the pre-`docs/api/` `docs/api.md`.

## Models & Routes

### GET /api/imported-upstreams

Returns `{items, members}` for imported and locally created direct-upstream channels. `items` contains
channel ID, name, origin key, dialect, base URL, availability, protocol paths and
model/credential counts. `members` connects group items to route, model and
credential names and authorized outbound protocol bits listed below.
Each member's `protocolOrder` restricts and orders that
route item's outbound protocols; an empty list retains the grant's protocols.
`items[].endpointConfig` contains optional entries with resolved `url`, `auth`
(`bearer`, `x-api-key`, or `x-goog-api-key`), and optional Gemini `modelPath`.
When `modelPath` is true, the URL is a models prefix and the selected model and action are
appended at execution; otherwise it is the exact upstream endpoint URL.
The optional endpoint `profile` selects provider-specific request handling.
The channel's `provider` identifies its source provider;
it is separate from its wire protocol and credential kind.
This is configuration inventory, not a protocol-health probe. Credential values,
custom headers and parameter overrides are never included.
Channel and credential summaries include `ownership` (`native` or `imported`).
Imported channels can contain locally created models and credentials; ownership
describes the individual record, not its parent or its routing eligibility.
Members also expose persisted grant success/failure counts and nullable cooldown
time/reason code. Imported grant failures use the gateway's bounded Fibonacci
backoff and are excluded from subsequent selection until that cooldown expires;
success clears cooldown. Health is per model/credential grant, shared across its
group memberships, and does not change unrelated grants. Imported historical
statistics are retained separately and are not treated as live health evidence.

| Endpoint key | Persisted protocol bit | Operation |
| --- | ---: | --- |
| `chat` | 2 | Chat Completions |
| `responses` | 4 | Responses |
| `messages` | 8 | Anthropic Messages |
| `gemini` | 16 | Gemini generation |
| `completions` | 32 | Legacy text completions |
| `embeddings` | 64 | OpenAI embeddings |
| `rerank` | 128 | Reranking |
| `imageGeneration` | 256 | Image generation |
| `imageEdit` | 512 | Image editing |
| `imageVariation` | 1024 | Image variations |
| `audioSpeech` | 2048 | Speech generation |
| `audioTranscription` | 4096 | Audio transcription |
| `audioTranslation` | 8192 | Audio translation |
| `moderations` | 16384 | Content moderation |
| `video` | 32768 | OpenAI-compatible video tasks |
| `geminiEmbeddings` | 65536 | Native Gemini embeddings |
| `jinaEmbeddings` | 131072 | Jina embeddings |
| `modelscopeImageGeneration` | 262144 | ModelScope image generation |

Non-generation requests require their configured capability and never fall back
to a conversation endpoint. OpenAI/Jina embeddings and OpenAI/ModelScope image
generation share a downstream operation but keep separate endpoint fields and
permission bits. A member's order selects among its authorized formats without
widening the grant. Gemini embeddings use native Gemini paths; converting an
OpenAI embeddings body to Gemini is not implemented.

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
and configured overrides, with provider thinking translation when an explicit
domestic profile is selected. Cross-protocol requests with nonportable continuity,
signed reasoning or unsupported tools fail explicitly instead of losing fields.

`deepseek` and `zai` translate Chat reasoning effort into the provider's thinking
settings; `zai` also serves GLM and MiMo. Default native AxonHub endpoints receive
the matching profile on import. Custom endpoints and existing empty profiles
remain generic until explicitly edited or re-imported. Native sites apply the
same translation only when their URL and platform match a supported preset.

Responses/Chat conversion preserves plain reasoning preferences, text, stream
events and tool history. It does not generate a reasoning summary. Encrypted or
signed reasoning and `previous_response_id` cannot cross this bridge. For
Messages-to-Chat tools, hidden reasoning is retained in the bounded process-local
replay cache, scoped to the client, route, model, credential and wire settings.
The next tool turn must reach that process; expired state, changed credentials or
permissions fail explicitly instead of silently dropping the tool history.

### GET /api/imported-upstreams/:id

Returns a flat editable connection object: `id`, `originKey`, `ownership`, `provider`, `dialect`,
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
`modelPath` is only valid for Gemini generation and embeddings. `codex` and `claudecode` profiles require
their matching provider, protocol, and Bearer authentication; `deepseek` and `zai`
profiles require Chat and Bearer. Empty profile keeps the generic wire contract.
Media profiles require Bearer authentication:

- `jina-embeddings` is required for `jinaEmbeddings`; omitted `task` defaults to
  `text-matching`. It cannot be attached to the generic embeddings field.
- `minimax-image` applies to image generation and translates the native MiniMax
  request and result shapes, including application errors in HTTP 200 responses.
- `modelscope-image` applies to image generation/editing and is required for
  `modelscopeImageGeneration`. It submits and polls the task with the configured
  credential; result downloads do not receive that credential.
- `codex-image` applies to image generation/editing on Codex/Fenno. Its required
  `requestModel` is the Responses model (maximum 255 bytes); the granted model
  remains the image tool model. Other profiles cannot set `requestModel`.

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
`protocolOrder` (unique protocol bits from the table above). A non-empty order must be a subset
of the shared grant; an explicit empty array restores inheritance from
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
`expiresAt` uses Unix milliseconds. The management form accepts a browser-local
date and time and converts it to milliseconds before submission.
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

## Upstream catalog management

The `/api/imported-upstreams` API manages both imports and local records. It does
not create shadow sites or accounts. Missing entities return 404; identity or
relationship conflicts return 409. Secrets are accepted only in write requests.

### Platform presets

`GET /api/imported-upstreams/presets` returns `{items}`. Presets reuse the site
catalog with `id`, `name`, `label`, `provider`, `platform`, `group`, `defaultUrl`
`protocols` (executable protocol names), and `recommendedModels`. New API comes first; domestic providers and Coding Plan
presets precede other providers. Only presets with an executable endpoint
contract appear in this list.

`POST /api/imported-upstreams/presets/resolve` accepts `{presetId,baseUrl}` and
returns `{provider,endpointConfig}`. It resolves configuration locally without
contacting an upstream. Resolve again after changing the base URL; do not reuse
the previous host's endpoints. Domestic Chat profiles are applied only on their
known hosts. A selected media preset retains its required wire adapter on custom
hosts. New API excludes image variations, which its router does not implement;
Coding Plan presets do not inherit a provider's media capabilities. Invalid targets or unknown
presets return 400.

### Catalog operations

| Method and path | Request / response |
| --- | --- |
| `POST /api/imported-upstreams` | Create with `name`, `provider`, `baseUrl`, explicit `endpointConfig`, optional `enabled` and connection/request settings. `dialect` is `generic`. Returns `{id,name,enabled,ownership}`. |
| `POST /api/imported-upstreams/:id/credentials` | Create with `name`, optional `enabled`, and either `apiKey` or the OAuth object described above. Returns the secret-free credential summary. |
| `GET /api/imported-upstreams/:id/models` | `{items:[{id,name,enabled,ownership,grants}]}`. Each grant includes `id`, `modelId`, `credentialId`, `credentialName`, `enabled`, protocol bitmask `protocols`, `memberCount`, and `ownership`. |
| `POST /api/imported-upstreams/:id/models` | `{name,enabled?}` or `{names:[...],enabled?}` (mutually exclusive; 1–500 unique names). Always returns `{items:[model...]}`. |
| `PATCH /api/imported-upstreams/models/:id` | Change `name` or `enabled`. Returns `{success,id,affectedRouteIds}`. This changes the upstream model name, not public route aliases. |
| `POST /api/imported-upstreams/grants` | `{modelId,credentialId,protocols:[2,8],enabled?}`. The model and credential must belong to the same channel. |
| `PATCH /api/imported-upstreams/grants/:id` | Change `protocols` (array) or `enabled`; model/credential identity is fixed. Returns the grant summary. |
| `GET /api/imported-upstreams/groups` | `{items:[group...]}` with route identity and all group members. |
| `POST /api/imported-upstreams/groups` | `{name,mode?,enabled?,route:{modelPattern,displayName?,routingStrategy?},members:[{grantId,priority?,weight?,protocolOrder?}],activeGrantId?}`. Atomically creates one group and its paired exact public-model route. |
| `PATCH /api/imported-upstreams/groups/:id` | Change `name`, `enabled`, `mode`, or `activeMemberId`. The selected member must belong to this group. |
| `POST /api/imported-upstreams/groups/:id/members` | Add `{grantId,priority?,weight?,protocolOrder?}`. Existing member preferences use the PATCH endpoint above. |

Channels, credentials and groups default to disabled; models and grants default
to enabled. Creating a model or credential does not implicitly create grants.
Group mode defaults to `failover`; an enabled `manual` group needs a selected
member. Members default to priority 0, weight 1 and inherited protocol order `[]`.
Group summaries include `id`, `name`, `mode`, `enabled`, `activeMemberId`,
`ownership`, `routeId`, `modelPattern`, `displayName`, and `members`. Members include
`id`, `groupId`, `grantId`, `priority`, `weight`, `protocolOrder`, `ownership`,
`modelName`, `credentialName`, and `channelId`.

Grant protocol arrays must be non-empty, unique and executable by the channel's
endpoints. An explicit member order must be a subset of that grant. Removing a
protocol still used by a member returns 409 with `conflictingMemberIds`; the
server does not silently clear the member order. Public aliases live on routes;
multiple aliases use separate groups/members sharing the same grants.

## Upstream deletion

Append `/deletion-preview` to any of these resource paths, then GET before DELETE:

- `/api/imported-upstreams/:id`
- `/api/imported-upstreams/models/:id`
- `/api/imported-upstreams/credentials/:id`
- `/api/imported-upstreams/grants/:id`
- `/api/imported-upstreams/groups/:id`
- `/api/imported-upstreams/members/:id`
- `/api/routes/:id`

Preview returns `{kind,id,counts,affectedRouteIds,revision,requiresCascade}`.
Counts cover actual dependent records regardless of origin: `channels`, `models`,
`credentials`, `grants`, `groups`, `members`, `routes`, `routeChannels`,
`routeGroupSources`, `downstreamKeys` and `sourceMappings`. `downstreamKeys`
means route permissions are affected; those keys are not deleted by this API.

DELETE accepts `expectedRevision` and `cascade=true`. Related-record deletion
requires both; any supplied revision is checked even for a leaf. Missing
confirmation or changed impact returns 409 with `{error,preview}` and changes
nothing. Successful deletion returns `{success:true,...preview}` and optionally
`downstreamKeysWithOnlyDeletedRoutes`. Deleting the last authorized route keeps
its dead ID in the key scope so the key cannot become unrestricted.

Deletion removes all associated source mappings, clears a removed manual active
member without selecting a replacement, and invalidates routing caches. Deleting
a group deletes its paired route and vice versa. Re-import can recreate deleted
source records; stale source mappings cannot silently update zero rows.

## Account routes

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

Delete a route and its channels. A route paired with a direct-upstream group uses
the same deletion preview and confirmation contract as that group, described below.
Ordinary account-backed routes retain their existing DELETE contract.

### POST /api/routes/batch

Batch enable/disable routes. Body: `{ "ids": [1, 2, 3], "action": "enable" }`.

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
