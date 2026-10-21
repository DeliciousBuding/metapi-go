# Auth & Sessions

> **Index**: back to [API Reference](../api.md). This file is the Admin session model (login/logout/session/ws-ticket) domain split out of the pre-`docs/api/` `docs/api.md`.

## Admin Session (#1034 session model)

Server-side admin sessions. `login`/`logout`/`session` are public (login is
the place the master token is presented and is strictly rate limited;
logout/session are idempotent and never 401); `ws-ticket` requires a live
session or Bearer master token.

### POST /api/auth/login

Exchange the master token for a session cookie.

**Request**: `{ "token": "<AUTH_TOKEN>" }`.
**Response (200)**: `{ "authenticated": true, "expiresAt": "<RFC3339 UTC>", "ttlMinutes": N }` + `Set-Cookie: metapi_session=...; HttpOnly; SameSite=Strict; Path=/`.
**Errors**: `400` empty body/token, `403 {"error":"Invalid token"}` wrong token, `503` session store unavailable.

### GET /api/auth/session

Bootstrap probe for the SPA; always answers 200.

**Response**: `{ "authenticated": true, "source": "session"|"token", "expiresAt"?: "<RFC3339 UTC>" }` or `{ "authenticated": false }`.

### POST /api/auth/logout

Revoke the session behind the cookie and clear it. Idempotent.

**Response**: `{ "success": true }`.

### POST /api/auth/ws-ticket

Mint a one-time WebSocket ticket (requires a live session or Bearer token).

**Response**: `{ "ticket": "<one-time>", "expiresInSeconds": 60 }`. The ticket is single-use; the ops WebSocket consumes it on upgrade.

## System users and groups

The deployment administrator manages ordinary users for this single gateway.
Each user has one current group; groups grant access to existing system routes.
There are no workspaces, team memberships or ordinary-user administrator roles.
This API establishes metadata and authorization relationships only: user login,
user-owned Key issuance, proxy enforcement and private usage views are not enabled.

All endpoints require the existing administrator session or master bearer token.
All writes additionally require `X-Admin-Confirm-Token` and join the admin audit
trail. Responses are `no-store`. Ordinary-user principals cannot use this surface.

| Endpoint | Request / response |
| --- | --- |
| `GET /api/admin/users` | `{items: GatewayUser[], limit: 1000}`, ordered by ID, includes disabled users |
| `POST /api/admin/users` | `{username, displayName, groupId?, enabled}` → user (`201`); omitted or zero groupId selects the default group |
| `GET /api/admin/users/{userId}` | user metadata, no credentials |
| `PUT /api/admin/users/{userId}` | `{displayName, groupId, enabled}`; positive groupId required, username immutable |
| `GET /api/admin/user-groups` | `{items: GatewayUserGroup[], limit: 1000}`, ordered by ID, includes disabled groups |
| `POST /api/admin/user-groups` | `{slug, name, enabled}` → group (`201`) |
| `GET /api/admin/user-groups/{groupId}` | group metadata |
| `PUT /api/admin/user-groups/{groupId}` | `{name, enabled}`; slug immutable |
| `GET /api/admin/user-groups/{groupId}/routes` | `{items: [{groupId, routeId, createdAt}]}`, all configured grants ordered by route ID |
| `POST /api/admin/user-groups/{groupId}/routes` | `{routeId}`; existing group and route required |
| `DELETE /api/admin/user-groups/{groupId}/routes/{routeId}` | revoke the grant |

The first default-group user creation creates the group with slug `default`, name
`默认分组`, enabled state and no route grants, in the same transaction as the user.
An existing default group's name, enabled state and grants are preserved. Schema
migration creates no users or groups and does not adopt any existing Key or route.
Changing a user's group changes the next effective-route lookup; disabled users,
groups or routes grant no callable routes. Empty effective grants deny all.
These lookups are not yet connected to downstream proxy admission.

Payloads require exact camelCase fields, explicit boolean `enabled` where listed,
and positive IDs except the documented create-user default selector. Unknown or
immutable fields, duplicate keys, empty/null objects and trailing JSON return
`400`. Missing objects return `404`; duplicate usernames/slugs/grants or invalid
group relationships return `409`; unavailable storage returns `503` or a generic
`500`. Updates and grant mutations return `{success: true}`. No password, role,
session issuance or physical user/group deletion is exposed here.

The three user authorization tables are excluded from portable backups; explicit
administrator-controlled database migration still copies them. Legacy Key policy
and the existing administrator authentication behavior remain unchanged.

---
