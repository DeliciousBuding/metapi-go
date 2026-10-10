# Settings & Maintenance

> **Index**: back to [API Reference](../api.md). This file is the Runtime/database/backup/notifications/maintenance/auth settings domain split out of the pre-`docs/api/` `docs/api.md`.

## Settings

### GET /api/settings/runtime

Get all runtime settings as a flat JSON object. Sensitive values (proxyToken, tokens, passwords) are masked. The response includes branding fields (`systemName`, `logo`, `footer`, `about`, `serverAddress`) and semantic schedule mirrors (`checkinSchedule`, `balanceRefreshSchedule`, `logCleanupSchedule`).

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
| `admin_sessions` | Session credential material. An import must never plant admin session token hashes, so a restored deployment requires a fresh login instead of reviving the source deployment's cookies. |
| `admin_audit_logs` | Append-only audit trail of the source deployment's admin writes. Replaying it into another database makes that database's audit record assert operations that never happened there, and no retention job bounds its size. |
| `model_probe_results` | High-frequency background probe telemetry that route rebuild reads as its latest-per-model signal; stale rows from another deployment would steer routing. The prober regenerates them after a restore. |
| `catalog_sources` | Each row's `url` is fetched server-side by the catalog sync, and the import URL guard (`sites` / `site_api_endpoints` only) does not cover this table yet, so a crafted backup could plant an SSRF fetch target. |

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
`400 unknown table <name>`; the exclusion is enforced on import, not just omitted on export.

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

API keys are imported as native downstream keys. Their project and key active
profiles are intersected into source-channel boundaries, preserving model
restrictions, ordered mappings, IP allowlists and scope/status checks. Project
profiles are flattened into the key's channel-ID snapshot; changing the source
profile requires reimport. Imported usage logs support native period quotas,
and local usage is retained when reimporting. Missing history blocks quota
periods that overlap the gap; see [access policies and quotas](downstream-keys.md).
A management-only or unsupported key remains explicitly blocked for proxy use.

The preview reports configuration counts, skipped channels, remaining source
sections, configuration differences and removals. The following remain outside
the current executable import contract:

- Gemini Vertex, Antigravity, Anthropic GCP, GitHub Copilot, xAI subscription,
  and fake providers.
- Formats not listed in [direct upstreams](routes.md), including source Decisions
  and provider-specific compaction. OpenCode Go dynamic protocol selection,
  Cline envelopes, and additional Bailian tool/stream behavior are not fully matched.
- Active channel transform operations, channel rate limits and stream policies,
  conditional associations, unsupported proxy modes, and nonportable key-level
  load-balancing/sticky overrides or regular expressions.
- Source pricing, request history and deployment settings. Developer associations
  and quota-relevant usage are consumed as described above; this does not copy
  the source deployment's global settings or turn historical usage into live
  health measurements.

These residuals are not a claim of equivalent behavior. Their source records
remain in the original backup and the preview explains the affected scope.

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

Preview a backup import without writing anything. Same body shapes as `POST /api/settings/backup/import` (`{ "tables": {...} }`, optional `{ "data": { "tables": {...} }` wrapper, TS backup v2.1 payloads), plus Octopus v5 and AxonHub v1.4 JSON. External-source requests require `X-External-Origin-Key`; their plan is the source-specific preview described above instead of a per-table row count.

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

Get authentication settings (admin IP allowlist, proxy token config).

### POST /api/settings/auth/change

Update authentication settings.

---
