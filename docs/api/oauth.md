# OAuth Flow & Connections

> **Index**: back to [API Reference](../api.md). This file is the OAuth providers, sessions, connections & route units domain split out of the pre-`docs/api/` `docs/api.md`.

## OAuth

### GET /api/oauth/providers

List OAuth providers.

**Response**: each provider includes `supportsDeviceFlow`. Device Flow providers
(currently GitHub Copilot and Grok) return a verification URL plus a copyable
user code; they do not use the manual callback form.

### POST /api/oauth/providers/{provider}/start

Start OAuth authorization flow for a provider.

**Response**: `{ provider, state, authorizationUrl, instructions }`. For Device
Flow providers, `instructions.deviceFlow` contains `{ userCode, verificationUri,
pollIntervalSeconds, expiresInSeconds }` and `device_code` itself is never
returned.

### GET /api/oauth/callback/{provider}

OAuth callback endpoint for a provider (acknowledges the browser redirect; session state was already issued by `start`/`rebind` and is validated on session lookup).

### GET /api/oauth/connections

Paginated list of OAuth-managed accounts (provider identity backfill runs once at startup, not per page load).

**Query params**: `limit` (1–200, default 50), `offset`.
**Response**: `{ connections/items: [{ accountId, siteId, provider, username, email, accountKey, planType?, projectId?, modelCount, modelsPreview, quota?, status, routeChannelCount, lastModelSyncAt?, lastModelSyncError?, proxyUrl?, useSystemProxy, routeParticipation?, site? }], total, limit, offset }` — `quota` is the cached `OauthQuotaSnapshot`; `routeParticipation` lists the route units this account feeds.

### GET /api/oauth/sessions/{state}, POST /api/oauth/sessions/{state}/manual-callback

Poll the flow state after `start`/`rebind`, or complete it with a manually pasted callback URL (for redirect flows without a browser callback).

**Response** (session): `{ provider, state, status, accountId?, siteId?, error?, retryAfterSeconds?, createdAt, updatedAt }` — 404 when the session/state is unknown or expired. Redirect sessions use the default 10m TTL; Device Flow sessions use the provider's device-code TTL. A pending Device Flow poll performs one upstream token request and returns `authorization_pending`/`slow_down` as a still-pending session with `retryAfterSeconds`; terminal OAuth errors settle the session as `error`.
**Body** (manual-callback): `{ "callbackUrl": "https://..." }` — required; unknown/expired state is 404, state mismatch is 400. Success: `{ "success": true }`.

### POST /api/oauth/connections/{accountId}/rebind, PATCH /api/oauth/connections/{accountId}/proxy, DELETE /api/oauth/connections/{accountId}

Rebind flow start, per-connection proxy update, and OAuth identity removal. Body (rebind): optional `{ "projectId", "proxyUrl", "useSystemProxy" }`; body (proxy PATCH): `{ "proxyUrl": "http://...", "useSystemProxy": true }` — empty body clears the proxy back to the system proxy.

**Responses**: rebind `{ provider, state, authorizationUrl, instructions, accountId }`; proxy PATCH `{ success, accountId, proxyUrl, useSystemProxy, refreshedRoutes, modelRefresh: { success, status, ... } }`; delete `{ "success": true }`. Non-OAuth accounts answer 404.

### POST /api/oauth/connections/{accountId}/quota/refresh, POST /api/oauth/connections/quota/refresh-batch

Refetch the upstream quota snapshot for one connection or a batch (batch runs 4-way concurrent). Batch body: `{ "accountIds": [1, 2] }` — omitted/empty body = all connections.
**Response** (single): `{ success, quota }` — `quota` is the persisted `OauthQuotaSnapshot` (`{ status, source, lastSyncAt?, lastError?, providerMessage?, subscription?, windows, lastLimitResetAt? }`); 404 for unknown/not-OAuth accounts.
**Response** (batch): `{ success, refreshed, failed, items: [{ accountId, success, quota?, error? }] }`.

### POST /api/oauth/import, POST /api/oauth/route-units, PATCH /api/oauth/route-units/{routeUnitId}, DELETE /api/oauth/route-units/{routeUnitId}

Reserved paths. Each returns HTTP 501 with `{ "code": "oauth_operation_not_supported", "error": "..." }` and writes no state.
Create or rebind connections through the provider authorization flow. Managed
routing reports participation on the connection list; these reserved paths do
not provide route-unit CRUD or generic connection import.

### POST /api/oauth/route-units/{routeUnitId}/members/{memberId}/cooldown/clear

清除指定路由池成员的冷却时间、连续失败次数、冷却级别和冷却原因，保留累计
成功/失败次数及选择、使用、失败时间。其他成员和路由通道的状态保持不变。
成功后选路缓存失效；未处于冷却的成员也返回成功。

**响应**：`{ "success": true, "routeUnitId": 7, "memberId": 11 }`。
非法 ID 返回 400，成员不存在或不属于该路由池返回 404，存储失败返回 500。
此接口使用管理 API 鉴权。

---
