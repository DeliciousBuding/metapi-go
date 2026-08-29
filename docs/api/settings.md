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

## Auth Settings

### GET /api/settings/auth/info

Get authentication settings (admin IP allowlist, proxy token config).

### POST /api/settings/auth/change

Update authentication settings.

---
