# Health Checks & Build Info

> **Index**: back to [API Reference](../api.md). This file is the /health /ready /desktop/health & /about domain split out of the pre-`docs/api/` `docs/api.md`.

## Health

### GET /health

Liveness check (no auth required). It does not touch dependencies and returns `{"status":"ok"}` when the HTTP process is alive.

### GET /ready

Readiness check (no auth required). It pings the active database and returns `200 {"status":"ok","database":"ok"}` when ready, `503 {"status":"degraded","database":"error"}` when the database is unavailable, or `503 {"status":"draining","database":"ok"}` while graceful shutdown is in progress.

### GET /api/desktop/health

Desktop health check. Returns `{"status":"ok"}`.

---

## About

### GET /api/about

Build provenance of the running binary, rendered by the admin About page.

**Response**: `{ version, commit, buildTime, goVersion }`

`version` is the `-ldflags` injected build version (`dev` for local builds). `commit` is the git SHA and `buildTime` the UTC RFC3339 build timestamp, both injected by the Makefile / Dockerfile / release pipeline; they are **empty strings** for builds without injection (plain `go build`, plain `docker build`) and the UI renders an em-dash rather than a fabricated value. `goVersion` comes from `runtime.Version()` and is always populated. The endpoint reads no database, so it keeps answering while the database is unavailable.
