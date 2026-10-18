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

List recent authenticated admin write operations, newest first. GET/HEAD/OPTIONS are not recorded; the middleware records best-effort and never blocks a request.

**Query params**: `limit` (default 50, max 200), `offset`, `method` (exact `POST`/`PUT`/`PATCH`/`DELETE`), `path` (substring).
**Response**: `{ items: [{ id, actor, method, path, status, requestId, remoteIp, createdAt }], total, limit, offset }`. `actor` is the first 8 hex chars of the bearer token's SHA-256 — stable, non-reversible, never the token itself.

### GET /api/debug/vars

Runtime debug snapshot (Go memory/goroutine stats, `/debug/vars` parity for the TS-era operator surface).

**Response**: `{ goroutines, mem: { alloc, totalAlloc, sys, heapAlloc, heapSys, heapIdle, heapInuse, heapReleased, heapObjects, stackInuse, stackSys, numGC, numForcedGC, gcPauseTotalNs, lastGCUnixNs } }`.

### GET /api/admin/ops/ws

Live ops WebSocket: one JSON frame per second over the current proxy-traffic window. **Auth**: one-time ticket (#1034) — the SPA mints a 60s single-use ticket via `POST /api/auth/ws-ticket` (session-authenticated) and dials with `?ticket=<one-time ticket>`. The legacy `?token=<master token>` query path is removed: the master token never appears in URLs (server logs, proxy logs, browser history). The endpoint is mounted outside the header-auth group; CORS origins follow the `ADMIN_CORS_ALLOWED_ORIGINS` configuration (same-origin when unset).

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

聊天测试使用指定渠道的凭据、站点代理与取消机制，完成后返回同步响应。
须提供 `channelId`、`siteId` 或 `forcedChannelId`；缺少时返回 `501`。

```json
{
  "channelId": 12,
  "model": "example-model",
  "targetFormat": "openai",
  "messages": [
    { "role": "system", "content": "简明回答。" },
    { "role": "user", "content": "你好" },
    { "role": "assistant", "content": "你好！" },
    { "role": "user", "content": "继续" }
  ],
  "temperature": 0.7,
  "top_p": 1,
  "max_tokens": 1024
}
```

`messages` 保留完整文本上下文，支持 `system`、`user`、`assistant`。
`targetFormat` 支持 `openai`（默认）、`claude`、`responses`、`gemini`，
通过现有协议转换器生成对应原生请求。采样值直接传入转换器；未指定输出上限时
使用 1024 tokens。默认等待 60 秒，`timeoutMs` 可在 1000–60000 毫秒内调整。

旧的 `prompt`、`path`、`mode` 和 `jsonBody` 输入仍可使用。
`mode` 默认 `chat`；`path` 指向模型列表时为 `models`。
响应沿用渠道测试 envelope，`truncatedBody` 最多保留 1 MiB，并用
`bodyTruncated` 标明截断。请求和回复不写入数据库。独立的
`POST /api/admin/test-channel` 连通性探针保持原有短提示词、8-token 与 2 KiB 限额。

### POST /api/debug/channel-probe

Alias of `POST /api/admin/test-channel` (same handler).

---
