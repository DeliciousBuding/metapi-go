# Diagnostics & Observability

> **Index**: back to [API Reference](../api.md). This file is the Resin/scheduler/rates/audit-logs/ops-ws, search, tasks & test surfaces domain split out of the pre-`docs/api/` `docs/api.md`.

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

Live ops WebSocket (B2): one JSON frame per second over the current proxy-traffic window. **Auth**: one-time ticket (#1034) — the SPA mints a 60s single-use ticket via `POST /api/auth/ws-ticket` (session-authenticated) and dials with `?ticket=<one-time ticket>`. The legacy `?token=<master token>` query path is removed: the master token never appears in URLs (server logs, proxy logs, browser history). The endpoint is mounted outside the header-auth group; CORS origins follow the `ADMIN_CORS_ALLOWED_ORIGINS` configuration (same-origin when unset).

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

### POST /api/test/chat

Sync forced-channel probe: delegates to the same forced-channel harness as `POST /api/admin/test-channel` when `channelId`/`siteId`/`forcedChannelId` is given; without a forced channel the full path/multipart matrix is a known limitation and the endpoint answers an honest `501` residual rather than inventing a successful probe.

**Body**: `{ "channelId": 12, "siteId": 3, "forcedChannelId": 12, "model": "gpt-4o-mini", "prompt": "ping", "mode": "chat", "timeoutMs": 15000, "path": "...", "jsonBody": {...}, "messages": [{ "role": "user", "content": "..." }] }` — `jsonBody`/`messages` are read for model/prompt extraction when the flat fields are absent. `mode` defaults to `chat` (or `models` when `path` contains `/models` and not `/chat`).
**Response**: same shape as `POST /api/admin/test-channel` (see above).

### POST /api/debug/channel-probe

Alias of `POST /api/admin/test-channel` (same handler).

---
