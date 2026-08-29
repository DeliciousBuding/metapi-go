# OAuth Flow & Connections

> **Index**: back to [API Reference](../api.md). This file is the OAuth providers, sessions, connections & route units domain split out of the pre-`docs/api/` `docs/api.md`.

## OAuth

### GET /api/oauth/providers

List OAuth providers.

### POST /api/oauth/providers/{provider}/start

Start OAuth authorization flow for a provider.

### GET /api/oauth/callback/{provider}

OAuth callback endpoint for a provider (acknowledges the browser redirect; session state was already issued by `start`/`rebind` and is validated on session lookup).

### GET /api/oauth/connections

Paginated list of OAuth-managed accounts (provider identity backfill runs once at startup, not per page load).

**Query params**: `limit` (1–200, default 50), `offset`.
**Response**: `{ connections/items: [{ accountId, siteId, provider, username, email, accountKey, planType?, projectId?, modelCount, modelsPreview, quota?, status, routeChannelCount, lastModelSyncAt?, lastModelSyncError?, proxyUrl?, useSystemProxy, routeParticipation?, site? }], total, limit, offset }` — `quota` is the cached `OauthQuotaSnapshot`; `routeParticipation` lists the route units this account feeds.

### GET /api/oauth/sessions/{state}, POST /api/oauth/sessions/{state}/manual-callback

Poll the flow state after `start`/`rebind`, or complete it with a manually pasted callback URL (for flows without a browser redirect).

**Response** (session): `{ provider, state, status, accountId?, siteId?, error?, createdAt, updatedAt }` — 404 when the session/state is unknown or expired (10m TTL).
**Body** (manual-callback): `{ "callbackUrl": "https://..." }` — required; unknown/expired state is 404, state mismatch is 400. Success: `{ "success": true }`.

### POST /api/oauth/connections/{accountId}/rebind, PATCH /api/oauth/connections/{accountId}/proxy, DELETE /api/oauth/connections/{accountId}

Rebind flow start, per-connection proxy update, and OAuth identity removal. Body (rebind): optional `{ "projectId", "proxyUrl", "useSystemProxy" }`; body (proxy PATCH): `{ "proxyUrl": "http://...", "useSystemProxy": true }` — empty body clears the proxy back to the system proxy.

**Responses**: rebind `{ provider, state, authorizationUrl, instructions, accountId }`; proxy PATCH `{ success, accountId, proxyUrl, useSystemProxy, refreshedRoutes, modelRefresh: { success, status, ... } }`; delete `{ "success": true }`. Non-OAuth accounts answer 404.

### POST /api/oauth/connections/{accountId}/quota/refresh, POST /api/oauth/connections/quota/refresh-batch

Refetch the upstream quota snapshot for one connection or a batch (batch runs 4-way concurrent). Batch body: `{ "accountIds": [1, 2] }` — omitted/empty body = all connections.
**Response** (single): `{ success, quota }` — `quota` is the persisted `OauthQuotaSnapshot` (`{ status, source, lastSyncAt?, lastError?, providerMessage?, subscription?, windows, lastLimitResetAt? }`); 404 for unknown/not-OAuth accounts.
**Response** (batch): `{ success, refreshed, failed, items: [{ accountId, success, quota?, error? }] }`.

### POST /api/oauth/import, POST /api/oauth/route-units, PATCH /api/oauth/route-units/{routeUnitId}, DELETE /api/oauth/route-units/{routeUnitId}

Stubs — each answers `{ "success": true }`. Connection import and route-unit state are orchestrated inside `service/oauth` (the connection list reports `routeParticipation`); these endpoints exist for UI path parity only.

---
