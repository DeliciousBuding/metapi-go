# Monitoring & Updates

> **Index**: back to [API Reference](../api.md). This file is the Monitor session/config + update center (residual) domain split out of the pre-`docs/api/` `docs/api.md`.

## Update Center

Local-only residual surface. Version updates ship via GitHub Releases / GHCR
image tags; the admin UI renders this section read-only with external links
and does not depend on in-app update discovery. Honest residuals: the
status/check payloads always report `updateAvailable: false` and expose a
`residual` string describing the limitation; deploy/rollback/task-stream
return `501` rather than inventing task ids or fake SSE streams.

### GET /api/update-center/status

Local status only. Returns `200` with
`{ "currentVersion": "0.0.0", "latestVersion": "0.0.0", "updateAvailable": false, "lastCheckedAt": null, "mode": "external", "residual": "..." }`.
Never invents `updateAvailable: true` or a fake `lastCheckedAt`.

### POST /api/update-center/check

Same local-only payload as status (no remote registry polling, no deploy
tasks started).

### PUT /api/update-center/config

Echo-only: accepts the config body and returns it with `success: true` plus a
`residual` note. Not persisted as an update-center product config; deploy and
rollback remain residual regardless of the echoed values.

### POST /api/update-center/deploy

Returns `501` (`{ "success": false, "message": "Update-center deploy is not implemented in Go", "residual": "..." }`).
Remote binary/Helm deploy via a helper service is out of scope; use external
deploy tooling (CI/CD or helper).

### POST /api/update-center/rollback

Returns `501` — same honest residual shape as deploy.

### GET /api/update-center/tasks/:id/stream

Returns `501`. No deploy/rollback task registry exists, so there is no SSE log
stream to serve.

---

## Monitor

### GET /api/monitor/health

Monitoring health snapshot.

### GET /api/monitor/config

Get monitoring configuration.

### PUT /api/monitor/config

Save monitoring configuration.

### POST /api/monitor/session

Start a monitoring session.

### DELETE /api/monitor/session

Clear the monitoring session.

---
