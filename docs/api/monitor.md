# Monitoring & Updates

> **Index**: back to [API Reference](../api.md). This file is the Monitor session/config + update center (residual) domain split out of the pre-`docs/api/` `docs/api.md`.

## Update Center

Local-only status surface. Version updates ship via GitHub Releases / GHCR
image tags; the admin UI renders this section read-only with external links
and does not depend on in-app update discovery. The status payload always
reports `updateAvailable: false` and exposes a `residual` string describing
the limitation.

### GET /api/update-center/status

Local status only. Returns `200` with
`{ "currentVersion": "0.0.0", "latestVersion": "0.0.0", "updateAvailable": false, "lastCheckedAt": null, "mode": "external", "residual": "..." }`.
Never invents `updateAvailable: true` or a fake `lastCheckedAt`.

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
