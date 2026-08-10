<h1 align="center">MetAPI Go</h1>

<p align="center">
  <strong>The proxy for proxies — aggregate all your AI API resellers into one unified gateway</strong>
</p>

<p align="center">
  Go rewrite of <a href="https://github.com/cita-777/metapi">MetAPI</a> · single binary · full feature parity with the TypeScript version
</p>

<p align="center">
  <a href="README.md"><strong>中文</strong></a> ·
  <a href="README_EN.md">English</a>
</p>

<p align="center">
  <a href="https://github.com/DeliciousBuding/metapi-go/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/DeliciousBuding/metapi-go/actions/workflows/ci.yml/badge.svg?branch=master"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DeliciousBuding/metapi-go?logo=github&label=release&color=blue"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26.5-00ADD8?logo=go">
  <img alt="React" src="https://img.shields.io/badge/React-19-61DAFB?logo=react">
  <a href="https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go"><img alt="Docker" src="https://img.shields.io/badge/ghcr-latest-2496ED?logo=docker&logoColor=white"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-3DA639?logo=opensourceinitiative&logoColor=white"></a>
</p>

<p align="center">
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-14-blueviolet">
  <img alt="Notifications" src="https://img.shields.io/badge/notifications-9%20channels-success">
  <img alt="DB" src="https://img.shields.io/badge/DB-SQLite%20%7C%20PostgreSQL-informational">
  <img alt="Image" src="https://img.shields.io/badge/image-15MB-orange">
  <img alt="Memory" src="https://img.shields.io/badge/memory-~20MB-9cf">
</p>

---

## Quick Start

```bash
docker run -d -p 4000:4000 \
  -v ./data:/app/data \
  -e AUTH_TOKEN=your-token \
  -e PROXY_TOKEN=sk-your-token \
  ghcr.io/deliciousbuding/metapi-go:latest
```

Open `http://localhost:4000`.

## Features

- **Protocol proxy**: OpenAI, Anthropic, Gemini, Codex — with real-time format conversion
- **Routing engine**: Weighted random, round-robin, stable-first. Fibonacci backoff cooldown. Circuit breaker.
- **Account management**: 14 platform adapters, auto check-in, balance tracking, OAuth PKCE
- **Operations**: 9-channel notifications (Webhook/Bark/ServerChan/Telegram/SMTP/Feishu/DingTalk/WeCom/ntfy), audit log, realtime ops panel, backup/restore, rate limiting, 16 background schedulers
- **Performance**: 20MB memory, 15MB Docker image, <0.1s startup

## Why Go?

| | Node.js | Go |
|---|---|---|
| Memory | 85 MB | ~20 MB |
| Image | ~250 MB | ~15 MB |
| Startup | 5-10 s | <0.1 s |

## Proxy Usage

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer proxy-token" \
  -d '{"model":"gpt-4o","messages":[{"role":"user","content":"Hello"}]}'
```

## Configuration

All env vars are identical to the TypeScript version.

| Variable | Default |
|----------|---------|
| `AUTH_TOKEN` | `change-me-admin-token` |
| `PROXY_TOKEN` | `change-me-proxy-sk-token` |
| `PROXY_MAX_BUFFERED_RESPONSE_BYTES` | `20971520`; maximum buffered non-streaming upstream response size |
| `METAPI_ENABLE_PROXY_STUB` | empty; test/demo-only local proxy stub. Keep empty in production so unconfigured upstream forwarding returns 503. |
| `PORT` | `4000` |
| `HOST` | `127.0.0.1` on Windows when unset; `0.0.0.0` elsewhere. Explicit values always win; containers set `0.0.0.0`. |
| `DB_TYPE` | `sqlite`; `postgres` is inferred when a PostgreSQL URL is provided |
| `DATABASE_URL` / `DB_URL` | empty; PostgreSQL connection string or SQLite file path. `DB_URL` takes precedence. |
| `DB_SSLMODE` | empty; PostgreSQL TLS mode. Supports `disable`, `allow`, `prefer`, `require`, `verify-ca`, and `verify-full`; non-empty values override `sslmode` in the connection string. |
| `DB_PROFILE` | `normal`; pool preset `shared-tiny` (2/1), `normal` (10/3), `dedicated` (20/5). Explicit `DB_MAX_*` override. |
| `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` | profile defaults; PostgreSQL application pool budget, must not exceed the database role connection limit. |
| `DB_CONN_MAX_LIFETIME_SEC` / `DB_CONN_MAX_IDLE_TIME_SEC` | `1800` / `300`; PostgreSQL connection lifetime and idle rotation in seconds. |
| `TRUSTED_PROXY_CIDRS` | empty; comma-separated reverse-proxy CIDRs allowed to supply `X-Forwarded-For` / `X-Real-IP`; forwarded headers are ignored by default |
| `ADMIN_CORS_ALLOWED_ORIGINS` | empty; comma-separated exact `http(s)` admin browser origins allowed to call `/api/*`; `*` is rejected |

Full list: [`.env.example`](.env.example).

On Windows, the loopback default avoids repeated inbound-firewall prompts from changing `go run` or temporary build paths. Set `HOST=0.0.0.0` only for intentional LAN exposure. Audit and precisely remove stale MetAPI executable rules with:

```powershell
.\scripts\windows-firewall-maintenance.ps1 -Mode Audit
.\scripts\windows-firewall-maintenance.ps1 -Mode Cleanup -Elevate
```

The runtime supports two database modes: single-process SQLite and PostgreSQL for production deployments. In PostgreSQL mode, side-effecting schedulers such as external requests, notifications, uploads, cleanup, and sync jobs use PG advisory locks so multiple replicas do not run the same job batch at the same time. Optional `REDIS_URL` / `METAPI_REDIS_URL` enables multi-instance shared **RPM/TPM admission** counters only (`auth.ConfigureSharedAdmissionFromRedisURL` + `internal/sharedcount`; fail-open to process-local windows if Redis is unreachable). Leave empty for single-node — no Redis process required. Sticky sessions remain process-local and are **not** shared across instances via Redis today (STICKY-B is residual, not product). See [`docs/analysis/redis-shared-state.md`](docs/analysis/redis-shared-state.md).

Proxy forwarding returns HTTP 503 when routing and upstream dependencies are not configured. `METAPI_ENABLE_PROXY_STUB=1` is for tests and demos only.

## Operations Health Checks

- `GET /health` is liveness only.
- `GET /ready` is readiness and returns HTTP 503 when the database is unavailable or the process is draining for shutdown.
- Docker runs `metapi healthcheck`, which probes `http://127.0.0.1:${PORT}/ready` by default.
- Override with `METAPI_HEALTHCHECK_URL` or `METAPI_HEALTHCHECK_PATH`.

## CORS Policy

Admin API CORS is closed by default for cross-origin browser requests. Set `ADMIN_CORS_ALLOWED_ORIGINS=https://admin.example.com` when the admin frontend is hosted on a different origin. Proxy and health endpoints keep wildcard CORS for client compatibility.

Forwarded client IP headers are ignored unless `TRUSTED_PROXY_CIDRS` contains the direct reverse proxy address range. Set it only for proxies you control.

## Migration from TypeScript

```bash
# Stop old server, start Go version with same env vars
./metapi
```

Database schema is identical. Auto-migration runs on startup.

## Related Projects

- [MetAPI (TypeScript)](https://github.com/cita-777/metapi) — Original Node.js implementation
- [TokenDance Gateway](https://github.com/DeliciousBuding/tokendance-gateway) — Production NewAPI fork

## License

MIT
