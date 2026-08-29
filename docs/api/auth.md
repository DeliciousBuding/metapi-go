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

---
