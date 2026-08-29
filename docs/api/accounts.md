# Accounts & Credentials

> **Index**: back to [API Reference](../api.md). This file is the Accounts, account tokens & rebind domain split out of the pre-`docs/api/` `docs/api.md`.

## Accounts

### GET /api/accounts, POST /api/accounts

List accounts. Create a new account.

**Query params** (GET): `page`/`pageSize` — when `page` is present the
endpoint returns the shared `{ items, total, page, pageSize, generatedAt,
 sites }` envelope (`page` is 1-based, `pageSize` clamps to 1–200; the
snapshot cache is bypassed so each page uses a bounded query); when `page` is
absent it keeps the legacy `{ generatedAt, accounts, sites }` snapshot shape.

The `sites` array is still returned on paged responses so the page's site
filter and create form keep working without a second full-fleet request.

**Body** (create, optional): `proxyUrl` — per-account egress proxy stored in `extraConfig.proxyUrl`; accepted schemes are `http://`, `https://`, `socks5://`, `socks5h://` (SOCKS5 runs natively on Go's `net/http` transport). Invalid schemes return `400`.

### GET /api/accounts/:id, PUT /api/accounts/:id, DELETE /api/accounts/:id

Get, update, delete an account.

**`proxyUrl` update semantics**: field omitted → keep the stored proxy; present with a value → replace (same scheme validation as create); present empty (`""`) → delete `extraConfig.proxyUrl`.

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
