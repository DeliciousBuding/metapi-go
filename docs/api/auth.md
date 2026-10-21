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
Administrators open accounts and provide a one-time activation/reset code.
Ordinary users enter `/user` directly, set their own password and use their
personal console. They never select a workspace or enter the administrator UI.
Administrators can also issue user Keys through the existing Key API using
`ownerUserId`; proxy admission and routing enforce the user's current group.

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
HTTP requests and each Responses WebSocket turn reload user/group state and
route grants. User/group disablement or removal rejects the next admission;
group changes apply to already-issued Keys. Key-specific limits only narrow
this boundary. Composite routes authorize their published outer entrypoint,
not independent access to their internal source routes. The legacy aggregate
pricing endpoints currently reject user Keys rather than exposing platform
observations as personal tariffs.

Payloads require exact camelCase fields, explicit boolean `enabled` where listed,
and positive IDs except the documented create-user default selector. Unknown or
immutable fields, duplicate keys, empty/null objects and trailing JSON return
`400`. Missing objects return `404`; duplicate usernames/slugs/grants or invalid
group relationships return `409`; unavailable storage returns `503` or a generic
`500`. Updates and grant mutations return `{success: true}`. No password, role,
session issuance or physical user/group deletion is exposed by these metadata APIs.

The three user authorization tables, user Keys and user-attributed records are
excluded from portable backups; exports describe this row-level exclusion.
Identity-preserving database migration/snapshots retain their ownership.
Portable imports reject user Keys or user-attributed records. Legacy system
Key policy and existing administrator authentication remain unchanged.

### Activation, password and personal session

`POST /api/admin/users/{userId}/activation` requires administrator authority,
master-token confirmation and audit, and returns `{code, expiresAt}` once.
It immediately resets the login credential and revokes all existing sessions.
The code expires after 30 minutes and can be consumed once. Provide it through
a private channel with the username and `/user` URL; do not put it in a URL.
Public registration and an email dependency are not introduced.

User passwords contain 12–72 UTF-8 bytes and use bcrypt cost 12. Only credential
hashes persist. `metapi_user_session` is an independent random server-side
session cookie, `HttpOnly`, `SameSite=Strict`, `Path=/api/user`, with Secure
behavior using the same configured transport policy as administrator cookies.
Only its digest persists. Sessions have a 24-hour absolute and two-hour idle
expiry. Password changes and resets invalidate all user sessions; disabled
users/groups cannot sign in or continue using a session. A concurrent password
change cannot mint a session verified against the previous credential version.

The user API is registered outside administrator authentication. User cookies,
downstream Keys and confirmation headers do not grant administrator authority;
an administrator cookie/bearer does not substitute for a user session. Mutating
user requests enforce exact same-origin checks and login/password paths are
rate-limited. Responses are `no-store`. Credential/session tables are excluded
from portable backup. API fields use exact camelCase, reject duplicate, unknown,
null or case-aliased fields and oversized/trailing JSON bodies.

| Endpoint | Contract |
| --- | --- |
| `POST /api/user/auth/activate` | `{username, code, password}`; consumes code, sets password and signs in; `{success:true}` |
| `POST /api/user/auth/login` | `{username, password}`; sets user cookie; `{success:true}` |
| `GET /api/user/session` | `{user:{id,username,displayName,groupId},group:{id,name},expiresAt}` |
| `POST /api/user/auth/logout` | Revoke current session and clear cookie; `{success:true}` |
| `POST /api/user/auth/password` | `{currentPassword,newPassword}`; revoke all sessions and clear cookie; sign in again |
| `GET /api/user/models` | `{items:[{id,routeId}]}`; concrete public names with eligible sources under current group, no upstream details or live availability promise |
| `GET /api/user/keys` | `{items:[{id,name,keyMasked,enabled,expiresAt,supportedModels,maxCost,maxRequests,usedCost,usedRequests,createdAt}]}`; only caller-owned Keys, never plaintext |
| `POST /api/user/keys` | `{name,supportedModels?,expiresAt?,maxCost?,maxRequests?}`; generate cryptographically random Key, bind authenticated user, return `{item,key}` once; users cannot choose owner, group, site or credentials |
| `DELETE /api/user/keys/{id}` | Delete only caller-owned Key; unknown or another user's ID is `404` |
| `GET /api/user/requests?page=1&pageSize=20` | `{items,total,page,pageSize}`; rows and total use the same caller-owner predicate |
| `GET /api/user/requests/{id}` | One safe request DTO, owner-scoped or `404` |
| `GET /api/user/usage` | `{usedRequests,keyCount,usedCost}` for caller-owned current Keys; `usedCost` is the legacy upstream cost estimate, not personal token quota |

Request DTO fields are `id`, `modelRequested` (nullable), `terminalStatus`,
`latencyMs`, `startedAt`, `endedAt`, `attemptCount`; no account/credential IDs,
raw error bodies, provider costs, arbitrary log payloads or other users' rows.
Old unowned logs stay administrator-only; renaming, regrouping and Key deletion
do not change historical request ownership. Invalid/expired credentials return
`401`, invalid input `400`, unavailable storage `503`. The personal UI renders
failed reads as errors rather than empty successful data.

---
