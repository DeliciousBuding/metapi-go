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

## Monitor iframe proxy

The monitor page embeds an upstream LDOH dashboard in an iframe. Iframe
sub-resource requests cannot carry an `Authorization` header, so this surface
authenticates with the HttpOnly `meta_monitor_auth` cookie minted by
`POST /api/monitor/session` instead of the admin Bearer token, and it is
deliberately mounted outside the Bearer-gated group. That cookie is scoped to
`Path=/monitor-proxy/` — so a stolen value cannot be presented to other
same-origin endpoints — and lives 7200 seconds.

| Route | Behaviour |
| :--- | :--- |
| `ANY /monitor-proxy/ldoh` | proxies the LDOH base URL root |
| `ANY /monitor-proxy/ldoh/` | the same, trailing-slash form |
| `ANY /monitor-proxy/ldoh/{wildcard}` | proxies `<LDOH_BASE_URL>/<wildcard>` |

`ANY` means every method: the surface is registered method-agnostically so the
embedded dashboard's own asset and XHR requests work unchanged.

Upstream and limits:

- `LDOH_BASE_URL` (default `https://ldoh.105117.xyz`) — point the iframe at a
  self-hosted LDOH instance.
- `LDOH_PROXY_TIMEOUT_SEC` (default `30`) — per-request upstream timeout, parsed
  once at startup.
- The upstream LDOH session cookie the proxy presents is the
  `monitor_ldoh_cookie` setting: written through `PUT /api/monitor/config`, and
  reported only as `ldohCookieConfigured` + `ldohCookieMasked` by
  `GET /api/monitor/config`.

Failures are explicit rather than empty: `401 {"error":"Missing or invalid
monitor session"}` without a valid monitor cookie; `400` with the plain-text
body `LDOH cookie not configured` when the upstream cookie is unset; `400
{"error":"invalid proxy path"}` for any path containing `..`. The traversal
check runs on the percent-decoded path, so `%2e%2e` is caught too — without it a
monitor-session holder could normalize outside the LDOH base subpath on the
upstream host.

**Security note.** `monitor_ldoh_cookie` is stored in plaintext in the settings
table, so read access to the database — a leaked backup, an injection in another
handler, a snapshot on a shared host — is equivalent to disclosing the LDOH
session until it expires upstream. Backup imports are forbidden from setting
this key (it is listed in `service/backup.RuntimeLocalSettingKeys`, which both
import paths enforce), because a planted value would pin the monitored session
to an attacker-controlled credential. Encrypting it at rest is the known
improvement; until then, treat database access as LDOH credential disclosure.
