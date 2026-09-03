# Settings & Maintenance

> **Index**: back to [API Reference](../api.md). This file is the Runtime/database/backup/notifications/maintenance/auth settings domain split out of the pre-`docs/api/` `docs/api.md`.

## Settings

### GET /api/settings/runtime

Get all runtime settings as a flat JSON object. Sensitive values (proxyToken, tokens, passwords) are masked. The response includes branding fields (`systemName`, `logo`, `footer`, `about`, `serverAddress`) and semantic schedule mirrors (`checkinSchedule`, `balanceRefreshSchedule`, `logCleanupSchedule`).

`ScheduleSpec` v1 uses `{ "version": 1, "kind": "daily|interval|window|custom", ... }`. The legacy `*_cron` fields remain available and remain the runtime compatibility source of truth.

### PUT /api/settings/runtime

Update runtime settings. Partial update -- only send fields you want to change. Schedule updates atomically write both the legacy cron key and the corresponding v1 semantic mirror.

**Upstream account health-monitoring kill switches** (#1027): `checkinEnabled` / `balanceRefreshEnabled` (boolean, default `true`) globally stop the two always-on jobs that contact upstream accounts -- automatic check-in and scheduled balance refresh. Changes persist to the `checkin_enabled` / `balance_refresh_enabled` settings and hot-apply to the running schedulers without a restart. Env equivalents: `CHECKIN_ENABLED` / `BALANCE_REFRESH_ENABLED` (read at startup). The per-account check-in switch (`checkinEnabled` on account rows) and `modelAvailabilityProbeEnabled` (Proxy & Models -> Proxy Transport) remain independent controls. Non-boolean values are rejected with `400`.

**Status-code verdict policy** (P1-2): `proxyRetryStatusRanges` / `proxyDisableStatusRanges` take a comma-separated spec of single codes (`401`) and inclusive ranges (`500-599`); adjacent/overlapping ranges merge. Blank restores the defaults. Malformed specs are rejected with `400` and the config is left untouched.

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
sync state are skipped.

Tables absent from the payload are skipped, so a backup written by an older build (which carried fewer tables) still
imports. A payload naming a table the backup set excludes — or any table outside the schema registry — is rejected with
`400 unknown table <name>`; the exclusion is enforced on import, not just omitted on export.

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
