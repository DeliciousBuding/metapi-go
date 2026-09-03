# Admin API Reference

**Last updated**: 2026-09-03
**Index**: the API reference is split by domain into [`api/`](api/) (one file per domain, table below); detailed endpoint listings live in each domain page.

Base URL: `http://localhost:4000/api`. Authentication model, response formats, request-body rules, billing, security notes, CORS and trusted-IP conventions: [docs/api/conventions.md](api/conventions.md).

## Admin API Domains

| Domain | File |
| :--- | :--- |
| Conventions, auth model, response formats, billing, security, CORS & trusted IPs | [`conventions.md`](api/conventions.md) |
| Stats & dashboard | [`stats.md`](api/stats.md) |
| Routes, channels & route decision | [`routes.md`](api/routes.md) |
| Marketplace, price compare, probes & model redirects | [`models.md`](api/models.md) |
| Sites CRUD, detect/import/batch, probe & tags | [`sites.md`](api/sites.md) |
| Accounts, account tokens & rebind | [`accounts.md`](api/accounts.md) |
| Site announcements, operator banners & system events | [`announcements.md`](api/announcements.md) |
| Downstream API keys, scope contract & export | [`downstream-keys.md`](api/downstream-keys.md) |
| Runtime/database/backup/notifications/maintenance/auth settings | [`settings.md`](api/settings.md) |
| Check-in trigger, logs & schedule | [`checkin.md`](api/checkin.md) |
| Monitor session/config + update center (residual) | [`monitor.md`](api/monitor.md) |
| Resin/scheduler/rates/audit-logs/ops-ws, search, tasks & test surfaces | [`diagnostics.md`](api/diagnostics.md) |
| Admin session model (login/logout/session/ws-ticket) | [`auth.md`](api/auth.md) |
| OAuth providers, sessions, connections & route units | [`oauth.md`](api/oauth.md) |
| /health /ready & /about | [`health.md`](api/health.md) |
| Full registered /api route inventory | [`routes-inventory.md`](api/routes-inventory.md) |
| /v1/files & /v1/pricing | [`proxy.md`](api/proxy.md) |
