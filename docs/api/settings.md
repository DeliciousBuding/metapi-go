# Settings & Maintenance

> **Index**: back to [API Reference](../api.md). This file is the Runtime/database/backup/notifications/maintenance/auth settings domain split out of the pre-`docs/api/` `docs/api.md`.

## Settings

### GET /api/settings/runtime

Get all runtime settings as a flat JSON object. Sensitive values (tokens, passwords) are masked. The response includes branding fields (`systemName`, `logo`, `footer`, `about`, `serverAddress`) and semantic schedule mirrors (`checkinSchedule`, `balanceRefreshSchedule`, `logCleanupSchedule`).

根下游密钥已迁入 API 密钥管理。此接口不再返回 `proxyTokenMasked`；发送
`proxyToken` 更新返回 `410 Gone`，并引导至 `/downstream-keys`。

`ScheduleSpec` v1 uses `{ "version": 1, "kind": "daily|interval|window|custom", ... }`. The legacy `*_cron` fields remain available and remain the runtime compatibility source of truth.

### PUT /api/settings/runtime

Update runtime settings. Partial update -- only send fields you want to change. Schedule updates atomically write both the legacy cron key and the corresponding v1 semantic mirror.

**Upstream account health-monitoring kill switches** (#1027): `checkinEnabled` / `balanceRefreshEnabled` (boolean, default `true`) globally stop the two always-on jobs that contact upstream accounts -- automatic check-in and scheduled balance refresh. Changes persist to the `checkin_enabled` / `balance_refresh_enabled` settings and hot-apply to the running schedulers without a restart. Env equivalents: `CHECKIN_ENABLED` / `BALANCE_REFRESH_ENABLED` (read at startup). The per-account check-in switch (`checkinEnabled` on account rows) and `modelAvailabilityProbeEnabled` (Proxy & Models -> Proxy Transport) remain independent controls. Non-boolean values are rejected with `400`.

**Automatic model routes**: `autoCreateModelRoutes` (boolean, default `false`) enables creation of missing exact-model routes after model synchronization or route rebuild. It persists as `auto_create_model_routes`, takes effect on the next rebuild and survives restarts; this is a runtime setting, not a new environment variable. Set it in **Settings → Operations → Scheduling**. Existing route patterns (including disabled routes), manual bindings and downstream grants are preserved. Turning it off stops new route creation; existing automatic channels still synchronize. See [route rebuild](routes.md#post-apiroutesrebuild) for task and result semantics.

**Status-code verdict policy**: `proxyRetryStatusRanges` / `proxyDisableStatusRanges` take a comma-separated spec of single codes (`401`) and inclusive ranges (`500-599`); adjacent/overlapping ranges merge. Blank restores the defaults. Malformed specs are rejected with `400` and the config is left untouched.

- `proxyRetryStatusRanges` — upstream statuses that count as retryable channel faults. Default (blank) reproduces the historical hardcoded verdicts: `401,403,408,409,425,429,500-599`.
- `proxyDisableStatusRanges` — statuses that disable the failing channel outright (enabled=false + manual_override, on top of the cooldown escalation). Default (blank) = no auto-disable, matching historical behavior; the new-api-style preset is `401`.

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

**Response**: `{ metadata: { exported_at, version, excluded_tables }, type, tables: { "<table>": [rows] } }`.

`type=all` carries every table the schema registry creates except the ones excluded by name in
`store.BackupExcludedTables()`; the set is derived (`store.BackupTableNames()`), not hand-copied, so a table added to
the schema ships in every full backup until somebody excludes it with a recorded reason. Rows are ordered
parents-before-children so an import can replay them in one pass. `metadata.excluded_tables` maps every registry table
this payload does **not** carry to the reason, so a backup file states its own gaps:

| Table | Why a backup does not carry it |
| --- | --- |
| `credential_refresh_leases` | 临时 OAuth 刷新所有权，恢复实例必须重新获取 lease；同样不复制到方言迁移目标。恢复出厂设置仍会清除它。 |
| `admin_sessions` | Session credential material. An import must never plant admin session token hashes, so a restored deployment requires a fresh login instead of reviving the source deployment's cookies. |
| `admin_audit_logs` | Append-only audit trail of the source deployment's admin writes. Replaying it into another database makes that database's audit record assert operations that never happened there, and no retention job bounds its size. |
| `model_probe_results` | High-frequency background probe telemetry that route rebuild reads as its latest-per-model signal; stale rows from another deployment would steer routing. The prober regenerates them after a restore. |
| `catalog_sources` | Each row's `url` is fetched server-side by the catalog sync, and the import URL guard (`sites` / `site_api_endpoints` only) does not cover this table yet, so a crafted backup could plant an SSRF fetch target. |
| `downstream_quota_reservations` | In-flight access-policy quota holds with a 30-minute TTL. They are settled by the source process, so replaying one would reserve quota on the target for a request that never ran there; settled `downstream_quota_usage` history is still carried. |
| `gateway_users`, `gateway_user_groups`, `gateway_user_group_route_grants` | Local ordinary-user identities and route permissions. Portable backups must not plant users or widen their group grants; explicit administrator-controlled database migration preserves these relationships. |
| `gateway_user_credentials`, `gateway_user_sessions` | Local password/activation/session digests, never replayed through portable imports. |
| `gateway_user_budgets`, `gateway_user_group_pricing`, `gateway_user_budget_ledger` | Local user quota balances, group rates and durable settlement history; importing them without the identity graph could plant charges or release holds. Whole-database migrations/snapshots preserve them. |

Row-level exclusions are stated in `metadata.excluded_rows`: user-owned
`downstream_api_keys`, their quota usage, files/video tasks, and user-attributed
`gateway_requests` and `proxy_logs` are not portable without their identity/authorization graph.
Imports reject user Keys and user-attributed request/attempt rows rather than
dropping ownership. Use a consistent whole-database snapshot or the explicit
database migration tool to preserve these users and their Keys.

`type=accounts` and `type=preferences` are scoped exports; `metadata.excluded_tables` additionally lists the tables
outside that scope. Limits: 50,000 rows per table, 4 MiB per cell, 64 MiB per payload — exceeding one fails the export
with `413` instead of truncating silently.

### POST /api/settings/backup/import

Import settings and data from JSON. Runtime-local settings such as `auth_token`, database connection settings, and WebDAV
sync state are skipped. Octopus v5 channel exports are also detected by their top-level `version: 5` envelope and imported into
separate direct-upstream tables, never converted into native `sites`/`accounts` rows. Supply a stable `X-External-Origin-Key`
header on both preview and commit; it scopes source-ID mappings so repeat imports update the same source identity while
different imports of channels with the same URL remain distinct.

Re-import is a source snapshot replacement, not an append-only merge. Preview returns `removals` counts for
source-owned records absent from the new snapshot and their dependent channels, credentials, models, grants, groups, members, and routes.
When removals exist, commit requires `X-Octopus-Replace-Origin: true` after reviewing the removal notice and final
confirmation; otherwise it returns 409 without changes. Replacement and upserts share one transaction.
Dependents can include local children of imported channels or members from another origin.
Preview includes `removalImpact` with `kind: "source"`, `id: 0`, `counts`, `affectedRouteIds`,
`requiresCascade`, and `revision`, using the [deletion preview contract](routes.md#upstream-deletion).
Send the reviewed revision in `X-External-Replacement-Revision`; a supplied stale revision always returns 409.
This header is required in addition to the replacement flag when the closure includes local or other-origin records.
Existing callers replacing only their own source records remain compatible with the boolean flag.
On 409, fetch a new preview and ask the operator to review it before resubmitting.
Empty channel exports remain invalid, not a delete-all command.

The Octopus v5 path currently imports channels, named channel keys, models, grants, groups, and group items atomically.
Imported grants enter the normal token-route selector and proxy executor as typed direct upstream candidates; they do not
create synthetic sites or accounts. Chat Completions, Responses, and Anthropic Messages are dispatched only when the
selected grant authorizes the matching protocol, using each imported protocol path. Messages credentials use the
Anthropic `x-api-key` header; OpenAI-compatible protocols use Bearer authorization. Direct requests preserve native
protocol bodies, including Responses reasoning items and tool continuations, except for the selected model mapping
and explicitly configured parameter overrides. Legacy site compatibility cleanup is not applied to direct grants.
Downstream model, route, and
direct-grant credential allow-lists still apply. Direct requests have no native site identity, so a non-empty downstream
site allow-list fails closed for these grants.

Octopus `channel_proxy` is honored only when its channel's `proxy` flag is enabled; `proxy_url` is the fallback only
for enabled-proxy channels without their own proxy URL, and never changes Metapi's global proxy setting. A channel
marked `proxy: true` without either effective source proxy is blocking (it never silently falls back to direct egress).
Other Octopus settings, API keys, and pricing records are not activated or imported. Preview lists these omissions. When any
are present, commit requires the explicit `X-Octopus-Import-Mode: channels-only` header after the operator has reviewed
and acknowledged the partial import; the original export remains unchanged. Allowlisted `{client_header:...}` templates
(`Idempotency-Key`, `OpenAI-Beta`, `X-Request-ID`, `X-Correlation-ID`, `traceparent`, and `tracestate`) are expanded
from the downstream request; other template names are blocking. Octopus's exact six-field default group `relay_config`
is listed as a routing-policy adaptation and requires `channels-only` acknowledgement: channel/group relationships are
preserved, but default member retries, timeouts, cooldown, and affinity are replaced by Metapi's routing rules. Custom
or extended group relay settings are blocking because their semantics cannot be represented by Metapi direct grants;
preview identifies them and commit rejects them even in channels-only mode.
Official `group_items` denormalized display fields (`channel_id`, channel/model/key names, protocols, and availability)
are accepted only in their exported zero-value form and ignored; the group/grant foreign keys remain authoritative.

Preview counts inline and aggregate historical statistics, and commit preserves those records as imported JSON data;
they remain source history and are not merged into Metapi's live usage totals. The existing native and TypeScript backup
formats retain their existing behavior. Backup import HTTP requests (including WebDAV imports) are limited to 20 MiB;
backup exports remain subject to the separate export limits described above.

Tables absent from the payload are skipped, so a backup written by an older build (which carried fewer tables) still
imports. A payload naming a table the backup set excludes — or any table outside the schema registry — is rejected with
`400 unknown table <name>`; the exclusion is enforced on import, not just omitted on export. The one compatibility
exception is `downstream_quota_reservations`: a genuine export (one carrying export provenance in `metadata`, such as
`exported_at` or `version`) written before the exclusion may still contain the transient table. Preview and import both
skip it and list it in `ignoredTables` instead of restoring it. A hand-written payload naming it, and every other
excluded table, is still rejected.

### AxonHub v1.4 import

An AxonHub backup (`version: "1.4"` with `timestamp`, `channels`, and `models`)
uses the same import and preview endpoints and `X-External-Origin-Key` scoping
as Octopus. One compiled plan drives both the preview and the transaction.
Repeated imports update source-owned records; removals require
`X-AxonHub-Replace-Origin: true`. Preview never writes, and a failed import
rolls back the entire graph and its downstream keys.
The same `removalImpact` and replacement-revision contract applies. Its
`counts.downstreamKeys` counts keys whose route permissions are affected, not keys
being deleted. AxonHub's `removals.downstream_api_keys` separately counts source
keys that will be deleted. The revision also covers a keys-only replacement.

The audited source revision is `e863c6fe1942deddd0f6e471fa003c430e5314f0`.
The parser accepts its generated `edges` metadata and default settings, rejects
ambiguous duplicate JSON keys and invalid source identities, and keeps secrets
out of the preview. Unknown executable fields remain explicit incompatibilities.

Key/model routing policy was additionally audited against source `d96748f1`.
Source system `retry_policy` routing defaults are flattened into each imported
route, then explicit model settings override them; the active Key profile
overrides the route at request time. `default` and `system_default` inherit.
Source `round-robin` uses gateway `round_robin` rotation and reports
`source_round_robin_replaced_by_gateway_rotation`: the source's decayed historical
request scoring is not reproduced. Key `adaptive`, `failover` and
`circuit-breaker` overrides remain blocked with
`source_key_strategy_not_equivalent:<strategy>`; their multi-dimensional source
scores are not silently mapped to a different strategy.

Source sticky `disabled` and `prefer_previous_channel` import. The latter
reports `source_trace_affinity_replaced_by_success_only_key_scope`: the source
remembers selection immediately, whereas this gateway remembers only completed
successes and isolates preference by Key, route and model. Default `AH-Trace-Id`
and explicit Codex/OpenCode session headers are accepted; custom source header
or body extraction and thread-derived identity are not imported and remain
`source_custom_trace_extraction_not_imported`. Anonymous requests without an
explicit stable identity gain no cross-request preference.

Model mappings retain first-match, whole-string matching and fixed destination
semantics. Go RE2 supports the documented ordinary regex subset; source
lookaround/backreferences and other unsupported syntax keep the Key blocked as
`source_key_model_mapping_unsupported`, without broadening its channel/model or
quota boundary.

The compiler resolves all six model-association kinds against supported models,
prefixes, aliases and hide/lowercase settings. Developer-level associations are
inherited unless the model opts out; local rules win ties, overlapping candidates
retain the highest priority. Per-model `modelProtocols` becomes an ordered list
on each route item, so two aliases sharing a credential do not widen each
other's outbound protocol choices.

Provider defaults and custom endpoints are merged by API format. Each endpoint
keeps its actual URL and authentication, including distinct hosts for Chat,
Responses and Messages. Explicit endpoint configurations support native Gemini
and generation-protocol conversion, with JSON, streaming and function tools.
Media formats retain independent permissions and exact URLs: completions,
OpenAI/Jina/native Gemini embeddings, rerank, image generation/editing/variations,
audio, moderation and OpenAI-compatible video tasks. Jina, MiniMax, ModelScope
and Codex image profiles preserve their source wire behavior. Media model types
only become routable through an associated channel's configured format; a model
type never creates a missing endpoint or grants unrelated credentials access.
Native bodies preserve provider-specific reasoning and continuation data;
nonportable cross-protocol fields fail explicitly. Codex/Fenno and Claude Code
support static and structured OAuth credentials, request-time refresh and their
provider-specific request/stream contracts. See [direct upstreams](routes.md)
for endpoint and credential management.

Native Ollama, Ollama Messages, Bedrock Messages, Seedance/ZenMux video,
TypeSafe System One and Alpha Search retain their independent formats and
authentication. Ollama channels with no configured API keys can create an
anonymous credential; a configured but disabled key never becomes anonymous.
Bedrock custom Messages endpoints follow the source's generic `x-api-key`
contract rather than its default Bedrock invoke adapter. Source combinations
that cannot construct an outbound adapter (custom `ollama/chat`, custom
`seedance/video`, or no-key Ollama with custom endpoints) remain named skips.
A channel that also carries a provider management key used only for console
quota checks still imports: that key is never attached to inference requests,
so it is omitted with the `management_api_key_not_imported` residual instead of
skipping the channel.

Bailian default Chat imports its tool/stream adapter; custom Chat retains the
source's generic format. Cline default and custom Chat normalize its request
and response envelope. OpenCode Go default Chat preserves model-selected wire
destinations inside that logical capability. Custom endpoints remain fixed to
their configured format and cannot replace internal addresses. OpenCode Go
Anthropic with no endpoint configuration uses the source's primary Messages
adapter; configured endpoints replace that fallback. The default model-dispatch
adapter does not support a `##` raw-URL base, since the source's constituent
adapters do not agree on that URL mode.

Moonshot, LongCat, OpenRouter, Cerebras and NanoGPT default Chat endpoints use
their platform adapters. Explicit custom Chat endpoints retain the source's
generic OpenAI contract. OpenRouter's dedicated image adapter is available to
local presets; importing a generic custom image endpoint does not add it or
expand the source grant. Signed HTTP(S) image references remain URLs forwarded
to the selected upstream, without a gateway-side download.

API keys are imported as native downstream keys. Their project and key active
profiles are intersected into source-channel boundaries, preserving model
restrictions, ordered mappings, IP allowlists and scope/status checks. Project
profiles are flattened into the key's channel-ID snapshot; changing the source
profile requires reimport. Imported usage logs support native period quotas,
and local usage is retained when reimporting. Missing history blocks quota
periods that overlap the gap; see [access policies and quotas](downstream-keys.md).
A management-only or unsupported key remains explicitly blocked for proxy use.

Model and inherited developer associations with disabled conditions or no
`when.condition` predicate retain their unconditional behavior, including
`when.enabled=true` with an omitted or null predicate. Active predicates preserve
their validated group tree on the imported membership. Matching happens on every
request before source associations for the same channel/model are aggregated:
the first matching association in source priority order supplies the effective
priority and protocol order. Local associations precede inherited ones at equal
priority. A condition never expands a grant, user-group permission or Key scope.
Unknown `when` fields are reported as
`model_association_field_unsupported:when.<field>` and exclude that association.
Invalid predicate fields, operators, types, group nesting or sensitive header
references are reported as `model_association_field_unsupported:when.condition`.
Enabled predicates use nonempty AND/OR groups up to three group levels and
the audited `prompt_tokens`, `stream`, `request_format`, content-feature,
non-sensitive `request_header.*` and `daily_time` conditions. Prompt token
thresholds use the source request heuristic, not billing tokens; daily times
use the gateway process timezone and inclusive-start/exclusive-end minute
boundaries, including intervals spanning midnight.
Within one Responses WebSocket turn, native transport selection and HTTP
fallback share the original handshake headers, normalized request body and one
clock snapshot for conditions. Each later turn gets a new descriptor and uses
its freshly authorized Key policy.

Missing request context cannot satisfy a conditional membership. Route
descriptors cover audited Chat, Responses, Messages, Gemini and supported
non-generation input shapes. Unknown tool/content shapes and other native
media protocols remain unevaluated rather than supplying an invented zero
token count or false content feature.
Route
explanations expose `associationConditions` as not evaluated, with
`associationContextRequired` on the affected candidate. Model catalogs retain
statically authorized conditional models without claiming that a particular
request will match. Imported member summaries expose `associationRuleCount`,
not predicate values. A conditional member's static priority cannot be changed
locally; reimport source associations to change their precedence. Weight and
protocol restrictions remain editable and cannot erase conditions.

Channel `policies.stream` imports as `streamPolicy`: `unlimited` (also the
default), `require`, or `forbid`. Streaming requests exclude `forbid` channels.
Non-streaming requests first use eligible channels that do not require streaming,
regardless of their relative priority; only when none remain may a `require`
channel stream upstream and return an aggregated JSON response. This applies to
Chat, Responses, Messages, Gemini and legacy Completions. Ollama upstreams use
their existing native NDJSON adapter and client protocol bridge.

Conditions always inspect the original downstream stream mode. Aggregation
does not widen model/protocol grants, Key scope, or user budgets. Embeddings,
media and Messages `count_tokens` cannot use a required-stream fallback.
Queue waits preserve the original request's stream mode and condition snapshot;
dispatch rechecks the current channel policy after waiting. Gateway-added
streaming controls do not count as prompt expansion in the ordered-operation
budget check; the original user reservation, output ceiling and content
restrictions still apply.
Native response fields, tools, reasoning/signatures and observed usage are
retained; explicit stream errors, missing terminals, cancellation and
configured byte/idle limits return failure before any partial JSON is sent.
An explicit Responses `response.incomplete` with `max_output_tokens` or
`content_filter`, and Gemini `promptFeedback.blockReason`, retain their native
JSON outcome and usage. These are complete protocol outcomes, distinct from
transport truncation. Codex endpoints retain their existing completed-only
collector. Existing configured content-failure heuristics still apply.
Unknown future native delta types may fail aggregation rather than silently
lose their content. A catalog without a request keeps static authorized
visibility and marks the streaming dimension as unevaluated.

Channel `rateLimit` settings with absent, null or nonpositive `rpm`, `tpm` and
`maxConcurrent` values retain the source's unlimited behavior. `queueSize` and
`queueTimeoutMs` do not activate a limit without positive concurrency. Positive
limits import into the channel's `rateLimit` policy:

- RPM is an atomic admission limit in fixed natural-minute buckets. It is
  consumed after concurrency admission and request preparation, immediately
  before a logical upstream attempt. Queue rejection/cancellation and invalid
  requests do not consume RPM; an authorized endpoint fallback is another attempt.
- TPM records the final measured usage once per attempt, including partial or
  error responses. It lowers routing preference when exhausted, but does not
  reserve an estimated token amount or reject the last eligible candidate.
  Missing usage is counted separately as unknown.
- Positive `maxConcurrent` is a hard cap shared by every credential and model
  of that channel. Excess requests wait FIFO. Missing/nonpositive `queueSize`
  means an unbounded waiting queue; positive values cap waiting requests.
  Positive `queueTimeoutMs` bounds either queue mode; client cancellation also
  removes the waiter. The selected credential's separate safety cap still applies.
- Counters and queues belong to one gateway process, reset on restart, and are
  not cluster-wide limits. Policy changes retain active permits; reducing
  capacity stops new grants until existing work finishes. Reimport replaces
  policy without resetting runtime counts.

Local admission failure tries another authorized candidate and never cools down
the provider. If none can dispatch, the request returns 503; a canceled waiter
returns 499. After waiting, dispatch reloads the Key/user authorization and the
selected grant/model/member graph; revoked or changed access is rejected before
upstream I/O, without consuming ingress admission again. A changed channel policy
also rejects the stale permit. The source's current limiter implementation defines this hard-cap
behavior even where older field comments describe soft counting. Unknown
rate-limit fields or unrepresentable timeouts still skip the channel with
`channel_rate_limit_unsupported`; malformed known field types reject the backup.

The preview reports configuration counts, skipped channels, remaining source
sections, configuration differences and removals. The following remain outside
the current executable import contract:

- Source `openai_fake` and `anthropic_fake` are fixed test-data executors, not
  production suppliers. Preview reports `provider_test_executor_not_imported`;
  apply creates no channels or grants for them. Their source records remain in
  the backup; synthetic response testing belongs to explicit test fixtures.
- Antigravity's source multi-origin quota fallback remains
  `source_endpoint_fallback_not_imported`. Its `refreshToken|project` credential
  imports as refresh-only OAuth state and exchanges lazily through the issuing
  installed-application client. The deployment must supply
  `ANTIGRAVITY_CLIENT_SECRET`; missing configuration prevents exchange.
  A channel selects one of the source's three
  official origins (daily by default); runtime never silently tries another
  origin. Custom source endpoints, missing projects and unrelated origins are
  skipped. Native Gemini and configured protocol bridges share the Antigravity
  envelope/schema adapter; no API-key fallback is allowed.
- GitHub Copilot, xAI subscription, Gemini
  Vertex and Anthropic GCP are executable through their audited credential and
  wire contracts; real upstream acceptance is still tracked separately.
- xAI subscription imports require structured OAuth with a refresh token and the official client ID, and resolve to the fixed `https://cli-chat-proxy.grok.com/v1/responses` endpoint with Bearer authentication. Static keys, custom base URLs and custom endpoints are skipped rather than imported as a generic channel.
- Formats not listed in [direct upstreams](routes.md), including source Decisions
  and unverified provider-specific compaction variants.
- Declared `openai/responses_compact` imports as a separate native HTTP grant.
  Its source constructor appends `/responses/compact` after base normalization;
  `##` retains an exact URL. It does not inherit the Codex generation wrapper
  or unsupported custom OAuth credentials.
- `openai/responses_websocket`, and `openai/responses` with
  `transport=websocket`, import as explicit WebSocket grants. Replacing a
  Responses endpoint's transport removes its HTTP permission. API-key native
  Responses and the Codex credential/profile are supported; other OAuth wire
  profiles remain named residuals. Compaction over WebSocket is not imported.
- Request operations outside the supported JSON/profile or authority boundaries below,
  stream policies,
  unsupported proxy modes, and the non-equivalent Key strategies, custom trace
  extraction or unsupported regular expressions described above.
- Source pricing, request history and deployment settings. Developer associations
  and quota-relevant usage are consumed as described above; this does not copy
  the source deployment's global settings or turn historical usage into live
  health measurements.

These residuals are not a claim of equivalent behavior. Their source records
remain in the original backup and the preview explains the affected scope.

#### Ordered channel request operations

AxonHub body/header operation arrays are retained in `upstream_channels.request_config`
(JSON text, additive migration `direct_request_config_v1`, default `{}`). Reimport
replaces the source-owned configuration without changing another origin. Portable
backups retain it; malformed stored configurations fail closed.

Body operations execute in order after provider serialization: `set`,
`set_if_absent`, `delete`, `rename`, `copy`, `array_append`, `array_prepend`,
`array_insert`, and `array_remove`. Header operations support the source's
`set`, `delete`, `rename`, and `copy`. A non-null operation array takes precedence
over its legacy field, including `[]`; a null array retains legacy behavior.
Operation arrays encoded in legacy `overrideParameters` are also recognized.
Legacy header entries are converted to ordered set/delete operations.

Each operation uses `{op,path?,from?,to?,value?,condition?,match?:{path,eq},index?,splat?}`.
Dotted/escaped paths and numeric array indexes are supported. Destination query,
modifier and projection paths remain unsupported; `array_remove.match.path`
is a read-only gjson expression. Empty remove-match paths/equality values are
invalid, as in the source. Insert indexes can be negative and are clamped to
the array bounds; `splat` defaults to true. `set_if_absent` keeps an explicit
null. `__AXONHUB_CLEAR__` clears a set destination.

Values and conditions use Go `text/template` with `toJSON` and the public
`RequestModel`, `Model`, `Metadata`, `RequestHeader`, `PromptCacheKey`,
`ReasoningEffort` fields. Authentication and private gateway-control headers are
absent. Conditions run only when their trimmed rendering is `true`. Rendered
objects, arrays, numbers, booleans and null become JSON values; other output
stays text. Large integer lexemes are preserved for untouched/copied values
and inserted JSON numbers rather than rounded through float64.
Unknown operations/fields, invalid templates, more than 128 operations or more
than 64 KiB per value/condition are refused at import. Template output is bounded
to 1 MiB; the configured request body limit also bounds the transformed body.
Runtime template errors retain the original literal, and individual
body path/type errors leave the previous body intact and continue, matching
the source's best-effort behavior; logs expose only operation index and reason.
Rendered header CR/LF/NUL fails the request before dispatch.

Operations cannot alter model, stream, stream-usage configuration, output limits,
authentication, framing or protocol identity headers, or copy secret headers.
Known provider-owned multi-request/envelope profiles (`opencode-go`, `systemone`,
`nanogpt`, `antigravity`, and dedicated image/video profiles) remain residual
when they carry these operations. Body operations on multipart/non-JSON
endpoints also remain residual. Imported endpoint edits must retain this
compatibility. Ordinary user Keys permit header-only and nonexpanding body
operations; a transformation that expands the conservative token estimate,
adds media/hosted tools or opaque continuation receives 403 before dispatch
and consumes no user usage. It does not enlarge the existing request hold.

Serializer options have a separate phase. Developer roles become system roles
before the existing protocol bridge; force-array instructions affects
Messages `system`, and force-array inputs affects Responses `input`. Other
serializers retain their source behavior. Reasoning mappings use the first
matching entry in source primary Chat serializers. The Claude CLI fallback
maps Chat/Responses effort only when a provider adapter has not already changed
it. These options do not change routing, stream selection or response conversion.

### Magpie backup import

Magpie `.magpie-backup` files are recognized by their encrypted
`format: "magpie-backup"` envelope. The envelope carries the bundle version
and PBKDF2/AES-GCM parameters; the passphrase is supplied separately in the
request body, never in a URL or header:

```json
{ "data": { "format": "magpie-backup", "...": "..." }, "passphrase": "..." }
```

An unwrapped envelope without a passphrase is rejected. Preview and commit
use the same decryption, validation and compile path. The default origin is
`magpie-backup`; a stable `X-External-Origin-Key` may be supplied instead.
Repeated imports update source-owned records. When a snapshot removes
previously imported records, commit requires `X-Magpie-Replace-Origin: true`;
the same `removalImpact` and `X-External-Replacement-Revision` contract as the
other external sources applies.

Static provider keys become direct channels, credentials, models and grants;
`provider/model` and `group/<id>` routes keep their Magpie-visible names.
Models a routing group names are imported even when the provider left them
out of its exposed list, because Magpie still serves such members through the
group; they also get a direct route and are reported with
`provider_models_completed_from_groups`, since Magpie did not list them to
agents.
Provider and group key order, weights, manual picks, static gateway-key model
scopes and convertible token/cost quotas are preserved. A gateway key whose
model or account scope cannot be represented is imported blocked, not widened
to unrestricted access. Magpie agents, library, settings, profiles, icons,
search APIs, dynamic rules/classifiers, fixed reasoning effort, affinity,
first-token/patience timeouts, Vertex/OAuth accounts, control-plane secrets
and per-key proxies have no direct equivalent; preview lists each residual
and the original backup remains the source of those records. A backup with
provider keys omitted imports channel configuration without usable grants.

Magpie and the other external importers share the 20 MiB request limit.

### GET /api/settings/backup/webdav

Get WebDAV backup configuration and last sync state. Passwords are never returned; use `hasPassword` and `passwordMasked` to show saved credential status.

The returned `state.lastSyncAt` is the last successful WebDAV import/export time. `state.lastAttemptAt` is the most recent attempt time, including failed attempts. `state.lastError` is set only when the latest attempt failed.

### PUT /api/settings/backup/webdav

Update WebDAV backup configuration. `fileUrl` must be an `http` or `https` URL without embedded userinfo. `exportType` supports `all`, `accounts`, or `preferences`.

### POST /api/settings/backup/webdav/export

Export a restorable backup payload to `fileUrl` with HTTP `PUT`. The payload uses the same `tables` structure as `GET /api/settings/backup/export`.

### POST /api/settings/backup/webdav/import

Download a backup payload from `fileUrl` with HTTP `GET` and import its `tables`. Runtime-local settings are skipped. The response includes imported row counts and updated sync state. The maximum downloaded backup size is 20 MiB.

### POST /api/settings/backup/import/preview

Preview a backup import without writing anything. Same body shapes as `POST /api/settings/backup/import` (`{ "tables": {...} }`, optional `{ "data": { "tables": {...} }` wrapper, TS backup v2.1 payloads), plus Octopus v5, AxonHub v1.4 and Magpie `.magpie-backup` envelopes. Octopus and AxonHub require `X-External-Origin-Key`; Magpie defaults to `magpie-backup` and carries its passphrase in `{ "data": ..., "passphrase": ... }`. External-source plans are the source-specific previews described above instead of per-table row counts.

**Response**: `{ success, plan: { "<table>": { rows, toInsert, duplicates, skippedRows } } }` — `duplicates` are rows whose PK already exists in the target DB (they would be dropped by `ON CONFLICT DO NOTHING`); `skippedRows` are runtime-local settings skipped by policy. No rows are written.

---

## Settings - Notifications

### POST /api/settings/notify/test

Send a test notification.

---

## Settings - Maintenance

### POST /api/settings/maintenance/clear-cache

Invalidate this process's in-memory caches (routing + accounts snapshot) and queue a real background route rebuild. Returns `202` with `{ success, queued, reused, jobId, taskId, status, message }`; poll `GET /api/tasks/:jobId` for the rebuild outcome.

**It deletes no rows.** Route definitions (`token_routes`), discovered models (`model_availability`) and channel attachments (`route_channels`, including the manual ones a rebuild never removes) are operator and upstream state, not cache — an earlier version of this endpoint wiped all three and then queued a rebuild that recomposes channels *from* them, so the promised rebuild had nothing to work with and every account's model list had to be re-fetched from upstream first. Use [factory reset](#post-apisettingsmaintenancefactory-reset) when you actually mean to wipe business rows.

Multi-instance note: only the process that served the request drops its in-memory caches; peers keep theirs until TTL expiry.

### POST /api/settings/maintenance/clear-usage

Clear all proxy usage data (proxy_logs, route_channel stats, account balanceUsed).

### POST /api/settings/maintenance/factory-reset

Restore the clean-install state: wipe every business table and restart the auto-increment sequences.

**Request**: `{ "confirm": true }` — required; any other body is rejected with `400`.

**Response**: `{ success, message, deleted: { "<table>": <rows deleted> } }`.

The table set is derived from the schema registry (`store.FactoryResetTableNames()`), not a hand-copied list, so a table added to the schema is wiped by a factory reset until it is explicitly excluded with a recorded reason. The single exclusion is the additive-migration journal (`schema_migrations`): wiping it would replay every migration step against an already-converged schema. Deletion runs in one transaction in FK-safe order (children before parents), so a failure leaves the database untouched rather than half wiped.

`admin_sessions` is part of the set on purpose — session validation reads that table on every authenticated request, so emptying it revokes every cookie issued before the reset. **Sign in again after a successful factory reset.** Audit history (`admin_audit_logs`) and probe history (`model_probe_results`) are wiped as well; that is what "restore factory settings" means here. A backup export does **not** preserve them — both are excluded by name with a recorded reason (see [Settings - Backup](#settings---backup)), so dump them with your own database tooling before resetting if you need to keep them.

---

## Auth Settings

### GET /api/settings/auth/info

Get the current admin token state without disclosing the token. The response is
`{ customized, configured, length }`: `customized` is true when the token
differs from the built-in default, `configured` reports whether a non-empty
token is active, and `length` is its character count. No prefix, suffix, or
masked token is returned. The response is `Cache-Control: no-store`.

### POST /api/settings/auth/change

Update authentication settings.

---
