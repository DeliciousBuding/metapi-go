# Deployment Guide

**Last updated**: 2026-08-20

## Prerequisites

- Docker (for containerized deployment)
- Go 1.26.6+ (for bare-metal deployment)
- Bun 1.x (for frontend build)
- PostgreSQL 16+ (optional, for production database)
- A reverse proxy (nginx or Caddy) with TLS

## Environment Variables

完整的环境变量清单见仓库根目录 [`.env.example`](../.env.example)（约 150 项，含通知、代理、调试等高级配置）；下表仅列部署必需与常用项。

### Required

| Variable      | Description                                      |
| ------------- | ------------------------------------------------ |
| `AUTH_TOKEN`  | Admin API bearer token. Server exits if missing. |
| `PROXY_TOKEN` | Proxy endpoint API key. Server exits if missing. |

### Optional

| Variable                            | Default                       | Description                                                                                                                                                                                                                                                                                                                                                                                        |
| ----------------------------------- | ----------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ACCOUNT_CREDENTIAL_SECRET`         | fallback to `AUTH_TOKEN`      | Key used to encrypt stored account credentials. **Recommended**: a unique 32+ byte random secret (`openssl rand -hex 32`). `< 8` bytes is a critical startup error; `< 16` logs a weak-secret warning. Do not reuse `AUTH_TOKEN`.                                                                                                                                                                 |
| `PORT`                              | `4000`                        | HTTP listen port                                                                                                                                                                                                                                                                                                                                                                                    |
| `DATA_DIR`                          | `/app/data`                   | SQLite database directory                                                                                                                                                                                                                                                                                                                                                                           |
| `LOG_LEVEL`                         | `info`                        | slog threshold: `debug`/`info`/`warn`/`error`. Raise to `warn` to quiet hot-path logs.                                                                                                                                                                                                                                                                                                              |
| `CHECKIN_CRON`                      | `0 8 * * *`                   | Daily checkin cron expression                                                                                                                                                                                                                                                                                                                                                                      |
| `BALANCE_REFRESH_CRON`              | `0 * * * *`                   | Hourly balance refresh cron                                                                                                                                                                                                                                                                                                                                                                        |
| `TZ`                                | `Asia/Shanghai`               | Timezone for cron scheduling                                                                                                                                                                                                                                                                                                             |
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
| `TRUSTED_PROXY_CIDRS`               | _(empty)_                     | Comma-separated reverse-proxy CIDRs allowed to supply `X-Forwarded-For` / `X-Real-IP`; forwarded headers are ignored when empty                                                                                                                                                                                                          |
| `ADMIN_CORS_ALLOWED_ORIGINS`        | _(empty)_                     | Comma-separated exact `http(s)` browser origins allowed to call `/api/*`; empty keeps admin API same-origin only, and `*` is rejected                                                                                                                                                                                                    |
| `REDIS_URL` / `METAPI_REDIS_URL`    | _(empty)_                     | Optional Redis for multi-instance shared downstream-key **RPM/TPM admission** only (`internal/sharedcount`; fail-open). Empty = process-local counters; no Redis process required. Does **not** enable sticky session multi-instance sharing                                                                                             |
| `PRICING_CATALOG_ENABLED`           | `true`                        | Enables the models.dev official catalog pricing provider used as the cold-start cost signal for cost-aware routing (`lowest_cost` / weighted cost factor when a channel has no observed history or configured `unit_cost`). A failed fetch falls back to a small built-in preset table; the last good snapshot is kept on refresh errors |
| `PRICING_CATALOG_REFRESH_MIN`       | `60`                          | Catalog refresh period in minutes (`0` disables periodic refresh after the initial fetch)                                                                                                                                                                                                                                                |
| `PRICING_CATALOG_URL`               | `https://models.dev/api.json` | models.dev dataset URL override (self-hosted mirror supported)                                                                                                                                                                                                                                                                           |

## Docker Compose (Production)

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

### 数据目录与卷

容器以**非 root 用户（uid 1001）**运行，数据目录的可写性是 Docker 部署最常见的坑：

- **命名卷（全新部署推荐）**：Docker 首次使用时会把镜像内 `/app/data` 的内容连同属主一起拷入卷，容器用户天然可写，无需任何 chown。
- **Bind mount（迁移旧数据时必须用）**：宿主目录保持宿主属主。旧 TypeScript 版以 root 运行，留下的 `./data` 和 `hub.db` 归 root 所有，Go 版写不进去，启动即报 `attempt to write a readonly database` 或 `unable to open database file`。启动前在宿主机执行一次：

  ```bash
  sudo chown -R 1001:1001 ./data
  ```

- **镜像标签**：`latest` 便于尝鲜；生产建议固定到版本标签（如 `ghcr.io/deliciousbuding/metapi-go:v0.16.6`），保证可复现部署。

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

The script needs only bash + curl (no node on the target).
