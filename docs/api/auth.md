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
| `POST /api/user/keys` | `{name,supportedModels?,expiresAt?,maxCost?,maxRequests?}`; omit supportedModels to inherit the group; explicit empty array is rejected. Generate cryptographically random Key, bind authenticated user, return `{item,key}` once; users cannot choose owner, group, site or credentials |
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

### User-wide token quota

User quota is token credit, **not a USD wallet or upstream invoice**. One unit
means one million reported total tokens at group multiplier 1. Input and output
use the same multiplier. The selected user, group and multiplier are snapshotted
when the request is admitted; regrouping, repricing, adding/deleting Keys and
editing limits never reset used quota. New users have finite zero quota until
the administrator assigns a limit or explicitly enables unlimited mode.

| Endpoint | Contract |
| --- | --- |
| `GET /api/admin/users/{userId}/budget` | `{limit,used,reserved,unlimited,frozen,unknownRequests}` |
| `PUT /api/admin/users/{userId}/budget` | Explicit `{limit,unlimited}` → full budget DTO; no used-counter reset |
| `GET /api/admin/user-groups/{groupId}/pricing` | `{multiplier}`; default 1 |
| `PUT /api/admin/user-groups/{groupId}/pricing` | `{multiplier}` → same DTO; positive only |
| `GET /api/admin/users/{userId}/budget/holds?offset=0` | `{items:[{requestId,state,reserved,totalTokens,createdAt,multiplier}],limit:100,offset}`; pending/unknown holds only, administrator-only |
| `POST /api/admin/users/{userId}/budget/reconcile` | `{requestId,totalTokens,confirmTerminated:true}` → budget DTO; verified total across all attempts, not an increment |
| `GET /api/user/budget` | Current authenticated user's safe aggregate budget DTO |

Amounts have at most six decimals; limits are 0..1e9 units, multipliers
1e-6..1e9. Zero is not unlimited. Shared-pool reservations are atomic across
all user Keys, including concurrent connections. Arithmetic uses integer micro
units and rounds each request's total charge upward. A server-generated request
ID or WS turn ID owns one settlement; retry attempts contribute their observed
usage without charging the same attempt twice. This ledger is synchronous and
independent of best-effort log queues and retention.

Supported user requests are bounded text Chat, Messages, Completions, Responses,
Embeddings and Gemini generateContent/streamGenerateContent. An omitted output
cap becomes 4096 tokens; existing valid caps remain. Admission reserves twice
the serialized UTF-8 JSON byte count plus the maximum output and 1024 tokens,
multiplied by the snapshotted group rate. Chat SSE requests include usage.
This is conservative estimation, **not a guarantee that a provider cannot
report more**: actual reported usage is charged even above the hold/limit,
and subsequent finite requests are rejected. User media, asynchronous video,
file resources, built-in search/tools, compact and opaque continuation bodies
(including previous_response_id/cachedContent) are rejected until their
accounting is supported; unlimited users have the same protocol boundary.
System Key protocol behavior is unchanged.

No dispatch releases a hold. Dispatched requests with missing/invalid/overflowed
usage retain the reservation and freeze further admission rather than charging
zero. Crashed processes leave durable pending holds without an automatic refund
TTL. Administrators inspect holds and provider/gateway evidence, verify the
request has terminated on **all instances**, then explicitly reconcile total
tokens at the original multiplier. Local active requests return `409`; this is
not a distributed termination detector. Repeating the same reconciliation is
idempotent, a different total conflicts. Raising quota alone does not resolve
unknown usage. All management writes require confirmation and audit.

---
