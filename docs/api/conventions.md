# Standard Conventions & Patterns

> **Index**: back to [API Reference](../api.md). This file is the Conventions, auth model, response formats, billing, security, CORS & trusted IPs domain split out of the pre-`docs/api/` `docs/api.md`.


**Last updated**: 2026-08-29
**Coverage note**: every `/api` route registered by the Go server gets a `### METHOD /path` detail section — the authoritative route list is the [Admin Route Inventory](#admin-route-inventory) at the bottom. Backward-compat aliases (e.g. `GET /api/stats/model-prices`) are documented as aliases rather than duplicating the canonical handler.

Base URL: `http://localhost:4000/api`

All admin endpoints authenticate via the **#1034 session model** (dual track):

1. **Session track (UI)**: `POST /api/auth/login` exchanges the master token
   for a server-side session carried in the HttpOnly `metapi_session` cookie
   (SameSite=Strict; Secure follows the request protocol unless
   `ADMIN_SESSION_COOKIE_SECURE` overrides). Sessions slide on activity
   (`ADMIN_SESSION_TTL_MINUTES`, default 720), die on logout, and are all
   revoked when the master token rotates.
2. **Bearer track (automation)**: `Authorization: Bearer <AUTH_TOKEN>` keeps
   working for external scripts, exactly as before:

```
Authorization: Bearer <AUTH_TOKEN>
```

Failed authentication (401/403) is rate limited — the per-IP limiter runs
**before** auth, and `/api/auth/*` additionally sits behind a strict bucket
(`AUTH_RATE_LIMIT_RPS`/`AUTH_RATE_LIMIT_BURST`, default 10/20).

### Sensitive operations (master-token re-confirmation)

Backup export, downstream-key export and master-token rotation additionally
require the master token in the `X-Admin-Confirm-Token` header, even with a
live session. Without it they answer `403 {"error":"...","reauthRequired":true}`
and the UI prompts for the token:

- `GET /api/settings/backup/export`
- `POST /api/settings/backup/webdav/export`
- `GET /api/downstream-keys/:id/export`
- `POST /api/settings/auth/change`

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
  "errorCode": "optional machine-readable identifier",
  "request_id": "optional correlation id"
}
```

- `error` is the human-readable message. It is for display only — clients
  must never branch on its text when a registered `errorCode` exists.
- `errorCode` is an **optional, additive** machine-readable identifier for
  the failure class (see the registry below). It is present only on
  endpoints that have registered a code; every other error body is
  byte-identical to the pre-existing shape (no `errorCode` key at all).
  Clients that ignore `errorCode` observe no change.
- `request_id` is additive and omitted when no request ID is present.

A few legacy endpoints (some batch/500 paths under `/api/accounts`, sites
import) still answer with the TS-era `{ "message": "Error description" }`
shape; the frontend reads both keys, so both forms surface identically.

HTTP status codes: 200 (OK), 201 (Created), 202 (Accepted), 400 (Bad Request), 401 (Unauthorized), 404 (Not Found), 500 (Internal Server Error).

#### errorCode convention and registry

`errorCode` values are stable **camelCase** identifiers (matching the
project-wide camelCase JSON rule). Codes are only introduced for real call
sites; this table is the registry and grows deliberately. Constants live in
`handler/admin/error_codes.go` (admin handlers) and `auth/admin.go` +
`auth/reauth.go` (auth middleware) and are pinned by the respective
`*_test.go`.

| errorCode               | Status | Where                                          | Meaning                                                        |
| ----------------------- | ------ | ---------------------------------------------- | -------------------------------------------------------------- |
| `invalidId`             | 400    | any `/api/.../{id}` route (pathID helper)      | path ID missing, non-numeric or non-positive                   |
| `invalidDatabaseType`   | 400    | `/api/settings/database/*`                     | runtime-database dialect is neither `sqlite` nor `postgres`    |
| `emptyMigrationTarget`  | 400    | `POST /api/settings/database/migrate`          | target connection string is blank                              |
| `sameMigrationTarget`   | 400    | `POST /api/settings/database/migrate`          | target resolves to the currently-running database (rejected)   |
| `authSessionExpired`    | 401    | all admin routes (auth middleware)             | session cookie presented but unknown/expired                   |
| `authMissingCredential` | 401    | all admin routes (auth middleware)             | no Authorization header and no session cookie                  |
| `authInvalidToken`      | 403    | all admin routes (auth middleware)             | Bearer master-token mismatch                                   |
| `authIpBlocked`         | 403    | all admin routes (auth middleware)             | client IP not on the admin allowlist                           |
| `authReauthRequired`    | 403    | sensitive admin routes (reauth gate)           | sensitive op needs master-token confirmation (`reauthRequired`) |

Frontend note: the admin UI historically detected the same-target migration
rejection by substring-matching the message text; it should migrate to
`errorCode === "sameMigrationTarget"`.

Frontend note (auth): the interceptor keeps its load-bearing substring match
on `"invalid token"` (clears session) and the `reauthRequired:true` flag
(prompts for master token); the auth `errorCode` values above only feed the
localized toast copy via the `errors.auth.*` i18n keys.

## Request Body Rules

Request bodies are capped at 20 MiB by the HTTP router. Admin JSON handlers also apply the same cap when decoding, so direct route handlers still read bounded input.

Admin JSON requests must contain one JSON value. Duplicate object keys and trailing JSON values are rejected with `400`.

---

## Billing & Currency

All money fields are denominated in **US dollars (USD)** — `balance`, `quota`, `balanceUsed`, `todayIncome`, `todayQuotaConsumption`, `unitCost`, `estimatedCost`, and the per-million pricing rates. There is no RMB path or multi-tenant wallet; the admin console renders `$` as a fixed currency symbol (not translatable interface copy).

Upstream balance semantics: NewAPI/OneAPI-family platforms expose integer `quota` where **1 USD = 500000 quota**; the `veloera` adapter uses **1 USD = 1000000 quota**. Platform adapters normalize to USD at the boundary (`quota / divisor`), so the API and frontend never carry raw upstream quota.

Pricing is ratio-based: `modelRatio` / `completionRatio` / `groupRatio` are multipliers over a base rate. At ratio 1, input costs **$2 per 1M tokens** (the NewAPI `0.002`/1K convention); output uses `modelRatio × completionRatio`, and prompt-cache tiers use `cacheRatio` / `cacheCreationRatio` (Claude defaults 0.10 / 1.25). Per-million rates and `estimatedCost` are estimates, never account-priced; `unitCost` is a display/planning field.

---

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
