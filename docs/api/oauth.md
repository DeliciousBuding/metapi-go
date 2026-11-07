# OAuth Flow & Connections

> **Index**: back to [API Reference](../api.md). This file is the OAuth providers, sessions, connections & route units domain split out of the pre-`docs/api/` `docs/api.md`.

## OAuth

### 刷新并发与凭据替换

托管账号和 direct OAuth 凭据刷新使用数据库级记录 lease，不仅依赖进程内
singleflight。同一数据库的多个连接池或进程刷新同一记录时，等待者重新读取
当前状态，并复用已落库且仍有效的结果；不能继续消耗旧 refresh token。
禁用、删除、身份/端点变更或并发替换不会被刷新覆盖，也不能发布已被替换的快照。
上游错误只保留固定 OAuth 分类，不回传可能包含 token 的错误正文。

lease 使用数据库时钟，等待最多 30 秒，所有权有效期 180 秒，等待支持 context
取消；兑换和落库临界区与调用方取消解耦，最多 2 分钟。发布必须同时通过当前
lease 的事务 fence 和凭据 CAS；共享存储失败不降级为本地锁。释放失败只记录
不含凭据的告警，由 TTL 恢复，不能重新交换已轮换 token。

`credential_refresh_leases` 是临时所有权，不进入便携备份或 SQLite/PostgreSQL
方言迁移，恢复出厂设置会清除。迁移前须停止所有旧实例；旧版本不参与此
协调，不能用新版本的 lease 推断混合版本部署安全。

签发供应商与 OAuth client 相同的复制 token 由持久 family 协调，跨账号/direct
复用已发布结果。数据库仅存不可逆指纹、generation、退休 alias 和结果记录引用，
不在协调表复制 token。消费意图在发请求前提交；连接后传输结果未知、成功响应
无法解析或错误响应带不同的新 token 均停止旧 token 重试。lease 到期/进程退出
不解除保护；需要重新授权或替换凭据。明确未发送或明确拒绝仍按既有有界策略重试。

兑换成功后仅重试落库，提交确认丢失时检查本次发布 nonce，不重新兑换；每次
落库仍校验记录/family fence 和原身份 CAS。旧 alias 不重新绑定，不用较旧备份
降低消费事实。HTTP 与 WS 在解析后重验当前模型、grant 和请求配置，不重复预算
准入。Copilot 的持久 GitHub token 是可重复使用的交换输入，允许中断后再次获取
短期 token；此供应商语义由服务端固定，不是可关闭 OAuth 保护的渠道选项。

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
