# Deployment Guide

**Last updated**: 2026-08-22

> **How to read this page**: just getting started? The README Quick Start covers
> the 3-minute paths — this page is the full reference behind them. Start with
> [Prerequisites](#prerequisites) and [Quick paths](#quick-paths), then jump to
> the production topics you need: reverse proxy and TLS, PostgreSQL, backup,
> upgrades and rollback. Every environment variable is listed in
> [`.env.example`](../.env.example); the grouped reference with explanations is
> [configuration.md](configuration.md).

## Prerequisites

Pick the prerequisites for the path you deploy with:

- **Release binary (fastest, recommended for first-time users)** — none.
  Pre-built binaries ship with every [GitHub Release](https://github.com/DeliciousBuding/metapi-go/releases/latest);
  `install.sh` only needs `curl` + `sha256sum`.
- **Docker** — any recent Docker Engine with Compose v2 (for containerized deployment).
- **From source** — Go 1.26.6+ **and** Bun 1.x. Bun is only needed to build the
  embedded frontend (`cd web && bun install --frozen-lockfile && bun run build:web`
  must run before `go build`); release binaries and GHCR images already include it.
- PostgreSQL 16+ (optional, for a production database)
- A reverse proxy (nginx or Caddy) with TLS (optional, for public exposure)

## Quick paths

Three equivalent ways to a running server. All commands below were executed
against a real release/source build during documentation review; default port
is 4000.

### 1. Release binary (3 minutes)

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/latest/download/install.sh | bash
```

The script downloads the matching platform binary from GitHub Releases,
verifies its SHA-256 against `checksums.txt`, and installs to
`/usr/local/bin/metapi` (override with `METAPI_INSTALL_PREFIX`; pin a version
with `METAPI_VERSION=v0.16.20`). Windows: download `metapi-windows-amd64.exe`
from the release page instead. Then start with the two required tokens:

```bash
export AUTH_TOKEN=$(openssl rand -hex 16)      # admin UI login token
export PROXY_TOKEN=sk-$(openssl rand -hex 24)  # downstream key for /v1/* calls
metapi
```

Data lands in `./data` (SQLite, code default) unless `DATA_DIR` / `DATABASE_URL` say otherwise.

### 2. Docker

```bash
docker run -d --name metapi \
  -p 4000:4000 \
  -e AUTH_TOKEN=your-admin-token \
  -e PROXY_TOKEN=your-proxy-sk-token \
  -e ACCOUNT_CREDENTIAL_SECRET=$(openssl rand -hex 32) \
  -e TZ=Asia/Shanghai \
  -v metapi_data:/app/data \
  --restart unless-stopped \
  ghcr.io/deliciousbuding/metapi-go:latest
```

Named volume is the zero-config default; bind mounts need a one-time
`chown -R 1001:1001` — see [Data directory & volumes](#data-directory--volumes).
Compose variants: [Docker Compose (Production)](#docker-compose-production).

### 3. From source

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

The frontend build step is mandatory: `web/dist/` is gitignored and `go build`
fails with `pattern dist: no matching files found` without it.

### Verify

```bash
curl http://localhost:4000/health
# {"status":"ok"}
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

Open `http://localhost:4000` and sign in with `AUTH_TOKEN`.

> **First proxied request**: `/v1/*` needs at least one upstream site with a
> verified account and a route rebuild first — without them the proxy answers
> an honest `503 {"error":{"message":"No available channels","type":"server_error"}}`
> (tested). The full walkthrough lives in
> [getting-started.md](getting-started.md) (§2–§5).

## Environment Variables

The complete variable inventory (~150 entries, including notifications, proxy
and debugging options) lives in [`.env.example`](../.env.example) at the repo
root; the tables below list only what a deployment needs plus common options.

### Required

| Variable      | Description                                      |
| ------------- | ------------------------------------------------ |
| `AUTH_TOKEN`  | Admin API bearer token. Server exits if missing. |
| `PROXY_TOKEN` | Proxy endpoint API key. Server exits if missing. |

### Optional

| Variable                            | Default                       | Description                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------------------- | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ACCOUNT_CREDENTIAL_SECRET`         | fallback to `AUTH_TOKEN`      | Key used to encrypt stored account credentials. **Recommended**: a unique 32+ byte random secret (`openssl rand -hex 32`). `< 8` bytes is a critical startup error; `< 16` logs a weak-secret warning. Do not reuse `AUTH_TOKEN`.                                                                                                                                                                 |
| `ADMIN_SESSION_TTL_MINUTES`         | `720`                         | Sliding lifetime of the admin UI session (#1034 session model). Every authenticated request refreshes it; logout/rotation revokes server-side. Values < 1 clamp to 1 |
| `ADMIN_SESSION_COOKIE_SECURE`       | `auto`                        | Secure flag of the `metapi_session` cookie: `auto` (follow request protocol), `true` (always Secure; use behind a TLS terminator), `false` (plain-HTTP dev only) |
| `AUTH_RATE_LIMIT_RPS`               | `10`                          | Strict per-IP rate limit for `/api/auth/*` (login is the only surface accepting the master token, #1034) |
| `AUTH_RATE_LIMIT_BURST`             | `20`                          | Burst size of the `/api/auth/*` bucket |
| `PORT`                              | `4000`                        | HTTP listen port                                                                                                                                                                                                                                                                                                                                                                                    |
| `DATA_DIR`                          | `./data`                      | Data directory (SQLite database and uploaded files). Code default is `./data` relative to the working directory; the container image sets `DATA_DIR=/app/data` via its `ENV`, and compose files pass it through explicitly |
| `LOG_LEVEL`                         | `info`                        | slog threshold: `debug`/`info`/`warn`/`error`. Raise to `warn` to quiet hot-path logs.                                                                                                                                                                                                                                                                                                              |
| `CHECKIN_CRON`                      | `0 8 * * *`                   | Daily checkin cron expression                                                                                                                                                                                                                                                                                                                                                                      |
| `CHECKIN_ENABLED`                   | `true`                        | Global kill switch for the automatic check-in scheduler: `false` stops scheduled check-in requests to upstream sites. Runtime setting `checkinEnabled` wins when set (#1027)                                                                                                                                                                                                                       |
| `BALANCE_REFRESH_CRON`              | `0 * * * *`                   | Hourly balance refresh cron                                                                                                                                                                                                                                                                                                                                                                        |
| `BALANCE_REFRESH_ENABLED`           | `true`                        | Global kill switch for the balance-refresh scheduler: `false` stops scheduled upstream balance queries. Runtime setting `balanceRefreshEnabled` wins when set (#1027)                                                                                                                                                                                                                              |
| `MODEL_SYNC_CRON`                   | `0 4 * * *`                   | Daily upstream model-list sync cron; DB setting `model_sync_cron` overrides                                                                                                                                                                                                                                                                                                                        |
| `TZ`                                | system local time             | Timezone for cron scheduling. No code-level default; `.env.example` and the compose files preset `Asia/Shanghai`                                                                                                                                                                                                                                                                                   |
| `DB_TYPE`                           | `sqlite`                      | Database type: `sqlite` or `postgres`; inferred as `postgres` when a PostgreSQL URL is provided                                                                                                                                                                                                                                          |
| `DATABASE_URL`                      | _(empty)_                     | PostgreSQL connection string; alias of `DB_URL`; when set to `postgres://` or `postgresql://`, uses PG instead of SQLite                                                                                                                                                                                                                 |
| `DB_URL`                            | _(empty)_                     | Database URL or SQLite file path; takes precedence over `DATABASE_URL`                                                                                                                                                                                                                                                                   |
| `DB_SSLMODE`                        | _(empty)_                     | PostgreSQL TLS mode. Supports `disable`, `allow`, `prefer`, `require`, `verify-ca`, and `verify-full`; non-empty values override `sslmode` in the connection string                                                                                                                                                                      |
| `DB_PROFILE` / `METAPI_DB_PROFILE`  | `normal`                      | Pool preset: `shared-tiny` (2/1), `normal` (10/3), `dedicated` (20/5). Explicit `DB_MAX_*` always override                                                                                                                                                                                                                               |
| `DB_MAX_OPEN_CONNS`                 | profile default               | PostgreSQL application pool ceiling; **must not exceed the database role CONNECTION LIMIT**                                                                                                                                                                                                                                              |
| `DB_MAX_IDLE_CONNS`                 | profile default               | PostgreSQL idle pool ceiling; must not exceed `DB_MAX_OPEN_CONNS`                                                                                                                                                                                                                                                                        |
| `DB_CONN_MAX_LIFETIME_SEC`          | `1800`                        | Maximum PostgreSQL connection lifetime; `0` disables rotation                                                                                                                                                                                                                                                                            |
| `DB_CONN_MAX_IDLE_TIME_SEC`         | `300`                         | Maximum PostgreSQL idle time; `0` disables idle rotation                                                                                                                                                                                                                                                                                 |
| `DB_APPLICATION_NAME`               | `metapi-<hostname>`           | Injected as PostgreSQL `application_name` when absent from DSN                                                                                                                                                                                                                                                                           |
| `PROXY_MAX_BUFFERED_RESPONSE_BYTES` | `20971520`                    | Maximum buffered non-streaming upstream response size; responses above the limit return 502                                                                                                                                                                                                                                              |
| `METAPI_ENABLE_PROXY_STUB`          | _(empty)_                     | Test/demo-only local proxy stub. Leave empty in production; unconfigured upstream forwarding returns 503                                                                                                                                                                                                                                 |
| `PROXY_CONNECT_TIMEOUT_SEC`         | `2`                           | Outbound TCP dial (connect) timeout in seconds for site-proxy/upstream requests; `0`/negative/invalid falls back to the default. (#1009)                                                                                                                                                                                                 |
| `PROXY_TLS_HANDSHAKE_TIMEOUT_SEC`   | `10`                          | TLS handshake timeout in seconds for outbound site-proxy transports; `0`/negative/invalid falls back to the default. (#1009)                                                                                                                                                                                                             |
| `PROXY_RESPONSE_HEADER_TIMEOUT_SEC` | `30`                          | Seconds to wait for upstream response headers on outbound site-proxy requests; `0`/negative/invalid falls back to the default. (#1009)                                                                                                                                                                                                   |
| `PROXY_IDLE_CONN_TIMEOUT_SEC`       | `90`                          | Idle keep-alive connection TTL in seconds for pooled outbound transports; `0`/negative/invalid falls back to the default. (#1009)                                                                                                                                                                                                        |
| `PROXY_REQUEST_TIMEOUT_SEC`         | `30`                          | Whole-request timeout in seconds for outbound site-proxy HTTP clients; `0`/negative/invalid falls back to the default. (#1009)                                                                                                                                                                                                           |
| `PROXY_STREAM_IDLE_TIMEOUT_SEC`     | `300`                         | Max seconds between chunks on a flowing SSE stream; each relayed chunk resets the window and expiry aborts the stalled stream as an upstream timeout fault. Bounds chunk gaps only — never total stream duration; `0`/negative/invalid falls back to the default        |
| `TRUSTED_PROXY_CIDRS`               | _(empty)_                     | Comma-separated reverse-proxy CIDRs allowed to supply `X-Forwarded-For` / `X-Real-IP`; forwarded headers are ignored when empty. A trusted peer's chain is resolved right-to-left, skipping trusted hops, so a client-injected left-most value cannot forge identity                                                                                                                                                                                                          |
| `ADMIN_CORS_ALLOWED_ORIGINS`        | _(empty)_                     | Comma-separated exact `http(s)` browser origins allowed to call `/api/*`; empty keeps admin API same-origin only, and `*` is rejected                                                                                                                                                                                                    |
| `REDIS_URL` / `METAPI_REDIS_URL`    | _(empty)_                     | Optional Redis for multi-instance shared downstream-key **RPM/TPM admission** only (`internal/sharedcount`; fail-open). Empty = process-local counters; no Redis process required. Does **not** enable sticky session multi-instance sharing                                                                                             |
| `PRICING_CATALOG_ENABLED`           | `true`                        | Enables the models.dev official catalog pricing provider used as the cold-start cost signal for cost-aware routing (`lowest_cost` / weighted cost factor when a channel has no observed history or configured `unit_cost`). A failed fetch falls back to a small built-in preset table; the last good snapshot is kept on refresh errors |
| `PRICING_CATALOG_REFRESH_MIN`       | `60`                          | Catalog refresh period in minutes (`0` disables periodic refresh after the initial fetch)                                                                                                                                                                                                                                                |
| `PRICING_CATALOG_URL`               | `https://models.dev/api.json` | models.dev dataset URL override (self-hosted mirror supported)                                                                                                                                                                                                                                                                           |

## Docker Compose (Production)

Both compose files mount a **named volume** (`metapi_data:/app/data`) by
default — zero configuration. To use a bind mount instead, follow the chown
instructions in the compose file comments and in
[Data directory & volumes](#data-directory--volumes).

1. Create a `.env` file:

```bash
AUTH_TOKEN=<your-admin-token>
PROXY_TOKEN=<your-proxy-token>
ACCOUNT_CREDENTIAL_SECRET=$(openssl rand -hex 32)   # recommended
```

2. Start the service:

```bash
docker compose -f docker-compose.prod.yml up -d
```

3. Check health:

```bash
curl http://localhost:4000/health
# {"status":"ok"}
```

## Data directory & volumes

The container runs as a **non-root user (uid 1001)**, and writability of the
data directory is the most common Docker deployment pitfall:

- **Named volume (recommended for fresh deployments)**: on first use Docker
  copies the image's `/app/data` contents — including ownership — into the
  volume, so the container user can write immediately. No `chown` needed.
- **Bind mount (required when migrating existing data)**: a host directory
  keeps its host ownership. The old TypeScript version ran as root, so a
  leftover `./data` / `hub.db` is root-owned and the Go container cannot write
  it — startup fails with `attempt to write a readonly database` or `unable to
  open database file`. Fix it once on the host before starting:

  ```bash
  sudo chown -R 1001:1001 ./data
  ```

- **Image tags**: `latest` is convenient for trying things out; for production,
  pin a version tag (e.g. `ghcr.io/deliciousbuding/metapi-go:v0.16.20`) so
  deployments stay reproducible.

## Nginx Reverse Proxy

```nginx
# Map for WebSocket Connection header: only send "upgrade" when the client
# actually requested an upgrade; otherwise close (standard nginx WS pattern).
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl http2;
    server_name metapi.example.com;

    ssl_certificate     /etc/letsencrypt/live/metapi.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/metapi.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:4000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # SSE/streaming support
        proxy_buffering off;
        proxy_read_timeout 600s;
    }
}
```

If this service is reachable only through that proxy and admin IP allowlists depend on the client IP, set `TRUSTED_PROXY_CIDRS` to the proxy source CIDR. Leave it empty when clients can reach Metapi directly.

## TLS with Let's Encrypt

```bash
# Install certbot
sudo apt install certbot python3-certbot-nginx

# Obtain certificate
sudo certbot --nginx -d metapi.example.com

# Auto-renewal (certbot timer is enabled by default)
sudo certbot renew --dry-run
```

## Bare-Metal Deployment

```bash
# 1. Build frontend
cd web && bun install --frozen-lockfile && bun run build:web && cd ..

# 2. Build server
go build -ldflags="-s -w" -o metapi ./cmd/server

# 3. Configure environment
export AUTH_TOKEN=<your-admin-token>
export PROXY_TOKEN=<your-proxy-token>
export DATABASE_URL='postgres://<user>:<password>@<host>:5432/<db>?sslmode=require'  # optional

# 4. Start server
./metapi
```

For container releases, the Dockerfile builds the React frontend inside a Bun stage and copies `web/dist` into the Go build stage before `go:embed` runs. A clean checkout should pass `docker build -t metapi-go:ci .` without a pre-existing local `web/dist`.

## Database

### SQLite (default)

Data is stored in `$DATA_DIR/hub.db`. The directory is created automatically on first start. Auto-migration runs at startup. SQLite is intended for a single Metapi process.

### PostgreSQL (production)

Set `DATABASE_URL` to switch to PostgreSQL:

```
DATABASE_URL='postgres://<user>:<password>@<host>:5432/metapi?sslmode=require'
```

For certificate validation, set `DB_SSLMODE=verify-full` and configure the PostgreSQL client certificate roots in the runtime environment.

Schema migrations run automatically at startup. Use `metapi-migrate` to transfer data from an existing SQLite database.

Use PostgreSQL for multi-instance deployments. Side-effecting schedulers use PostgreSQL advisory locks, so only one replica runs each job batch at a time. `admin-snapshot` remains process-local cache warming; `usage-aggregation` uses its own checkpoint lease.

Optional Redis (`REDIS_URL` / `METAPI_REDIS_URL`) is used only for multi-instance shared downstream-key **RPM/TPM admission** via `auth.ConfigureSharedAdmissionFromRedisURL` and `internal/sharedcount`. Admission is fail-open: if Redis is unreachable, counters fall back to process-local windows. Leave empty for single-node deployments — no Redis process is required. Sticky session bindings remain process-local; Redis does **not** share sticky maps across instances (sticky sessions are process-local, not cluster-wide).

### Proxy Forwarding

Proxy routes require routing and upstream forwarding dependencies at runtime. If they are not wired, requests return HTTP 503 instead of a synthetic success response. `METAPI_ENABLE_PROXY_STUB=1` is limited to tests and demos.

## Backup Strategy

### SQLite

```bash
# Simple file copy (while server is running -- WAL mode safe)
cp data/hub.db "backups/hub-$(date +%Y%m%d-%H%M%S).db"

# With sqlite3 backup command
sqlite3 data/hub.db ".backup 'backups/hub-$(date +%Y%m%d-%H%M%S).db'"
```

### PostgreSQL

```bash
pg_dump -Fc 'postgres://<user>:<password>@<host>:5432/metapi?sslmode=require' > "backups/metapi-$(date +%Y%m%d-%H%M%S).dump"
```

## Monitoring

- Liveness check: `GET /health` returns `{"status":"ok"}` when the HTTP process is alive
- Readiness check: `GET /ready` returns `{"status":"ok","database":"ok"}` or HTTP 503 when the database is unavailable or the process is draining for shutdown
- Docker healthcheck runs `metapi healthcheck`, which polls `/ready` every 30 seconds by default
- Override the healthcheck target with `METAPI_HEALTHCHECK_URL` or `METAPI_HEALTHCHECK_PATH`
- Startup exits before binding the HTTP port when database bootstrap, runtime settings load, or runtime schema migration fails.
- Admin events are logged in the database `events` table
- Proxy request logs are stored in `proxy_logs`

## Disabling Upstream Account Health Monitoring

Metapi runs three background jobs that actively contact upstream accounts
("health monitoring"). Each one can be switched off independently; defaults
keep the historical always-on behavior for check-in and balance refresh.

| Job | What it does | How to turn it off |
| --- | --- | --- |
| Automatic check-in | Sends scheduled check-in requests to upstream accounts (cron or interval mode) | Settings -> Operations -> Scheduled Tasks: uncheck **Enable daily check-in** (global); per-account switches are on the Accounts page; env `CHECKIN_ENABLED=false` |
| Balance refresh | Queries upstream account balances on a schedule | Settings -> Operations -> Scheduled Tasks: uncheck **Enable scheduled balance refresh**; env `BALANCE_REFRESH_ENABLED=false` |
| Model availability probe | Actively probes channel models (off by default) | Settings -> Proxy & Models -> Proxy Transport: `modelAvailabilityProbeEnabled`; env `MODEL_AVAILABILITY_PROBE_ENABLED` |

Notes:

- The two global checkboxes apply hot (no container restart required) and are
  persisted to the database, so they survive restarts and upgrades.
- Environment variables only set the startup default; a persisted runtime
  setting wins when present.
- Status badges and probe-health columns on the Accounts page are passive
  displays of past traffic; they never issue upstream requests themselves.

Docker Compose example -- stop every automatic upstream request by adding the
kill switches to your `.env` and recreating the container:

```bash
CHECKIN_ENABLED=false
BALANCE_REFRESH_ENABLED=false
```

```bash
docker compose -f docker-compose.prod.yml up -d
```

Flipping the same two checkboxes in the admin UI achieves the identical result
without any restart.

## Browser CORS

- `/api/*` admin routes do not allow cross-origin browser access unless `ADMIN_CORS_ALLOWED_ORIGINS` is set.
- `/v1/*`, non-`/v1` proxy aliases, `/health`, `/ready`, and `/metrics` retain wildcard CORS for operational and downstream-client compatibility.
- Keep `ADMIN_CORS_ALLOWED_ORIGINS` to exact trusted origins, for example `https://admin.example.com`; wildcard origins, paths, query strings, and fragments are rejected at startup.

## Request Limits

- HTTP request bodies are capped at 20 MiB.
- Admin JSON decoders enforce the same cap, reject duplicate object keys, and reject trailing JSON values.
- WebDAV backup import downloads are capped at 64 MiB.

## Trusted Proxies

- `X-Forwarded-For` and `X-Real-IP` are ignored by default.
- Set `TRUSTED_PROXY_CIDRS` only to reverse-proxy source ranges you control.
- Admin IP allowlists and rate limits use the direct peer IP unless the peer matches `TRUSTED_PROXY_CIDRS`.
- When the peer is trusted, the forwarded chain is resolved from the **right**: every `X-Forwarded-For` header is joined in order, the direct peer is appended as the last hop, and hops inside `TRUSTED_PROXY_CIDRS` are skipped until the first address outside them is found. That address is the client. A value the client injected at the left of the chain is therefore never trusted, which matches the append semantics of `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for` in the nginx example above.
- List **every** proxy layer in `TRUSTED_PROXY_CIDRS` (edge plus internal load balancers). A layer missing from the list becomes the resolved "client" for all traffic behind it: allowlist entries stop matching and those callers share one rate-limit bucket. The failure mode is closed, not open.
- If every hop in the chain is trusted (internal proxies in front of an internal client), the left-most entry is used so those callers keep distinct rate-limit keys.
- `X-Real-IP` is consulted only when `X-Forwarded-For` carries no parsable address.

## Upgrading

```bash
# Pull new image
docker pull ghcr.io/deliciousbuding/metapi-go:latest

# Restart with new image
docker compose -f docker-compose.prod.yml up -d
```

Docker Compose will automatically recreate the container with the new image. Auto-migration runs at startup to apply any new schema changes.

Upgrade checklist:

1. Back up the database first (cp data/hub.db ... or pg_dump -Fc ...).
2. Prefer pinning the release selected for your deployment (for example `ghcr.io/deliciousbuding/metapi-go:vX.Y.Z`) over `latest` so rollback is reproducible.
3. After upgrade, verify GET /ready returns 200 and run bash web/scripts/verify-live-assets.sh http://127.0.0.1:4000.
4. Rollback = restore the pre-upgrade backup and re-run the previous pinned image tag. Schema changes are forward-only; do not downgrade the schema by deleting rows from schema_migrations.

## Post-deploy asset smoke (mandatory)

After every deploy, replay the browser asset graph against the running
instance — this catches SPA-fallback-swallowed assets (200 text/html instead
of the real file) and stale-cache 404s:

```bash
bash web/scripts/verify-live-assets.sh http://127.0.0.1:4000
# verify-live-assets OK: N referenced assets all 200
```

The script discovers entry assets from index.html (both the rsbuild `/static/`
and legacy Vite `/assets/` prefixes) plus the lazy async chunks listed in the
runtime chunk map, and fails closed (exit 1) if it discovers zero assets or any
asset is missing/swallowed — an empty set never passes.

The script needs only bash + curl (no node on the target).
