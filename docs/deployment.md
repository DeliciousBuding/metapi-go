# Deployment Guide

**Last updated**: 2026-10-21

> **How to read this page**: just getting started? The README Quick Start covers
> the short path — this page owns the full install, Compose, upgrade and
> rollback commands behind it. Start with
> [Prerequisites](#prerequisites) and [Quick paths](#quick-paths), then jump to
> the production topics you need: reverse proxy and TLS, PostgreSQL, backup,
> upgrades and rollback. Every environment variable is listed in
> [`.env.example`](../.env.example); the grouped reference with explanations is
> [configuration.md](configuration.md).

## Prerequisites

Pick the prerequisites for the path you deploy with:

- **GHCR Docker image (recommended)** — Docker Engine; add Compose v2.24+
  when using the repository Compose file (it uses optional `env_file`). The
  pre-built image needs no Go, Bun, or source compilation on the host.
- **Release binary** — no language runtime.
  Pre-built binaries ship with every [GitHub Release](https://github.com/DeliciousBuding/metapi-go/releases/latest);
  `install.sh` only needs `curl` + `sha256sum`.
- **From source** — Go 1.26.9+ **and** Bun 1.x. Bun is only needed to build the
  embedded frontend (`cd web && bun install --frozen-lockfile && bun run build:web`
  must run before `go build`); release binaries and GHCR images already include it.
- PostgreSQL 16+ (optional, for a production database)
- A reverse proxy (nginx or Caddy) with TLS (optional, for public exposure)

## Quick paths

Three ways to a running server. CI parses the Compose file with its example
values; release and source paths were exercised against real builds. The
default port is 4000.

### 1. GHCR Docker image (recommended)

Pull the published `ghcr.io/deliciousbuding/metapi-go` image rather than building
from source. Pin a release tag or digest; `edge` is for development.

**Compose (recommended for anything long-lived).** From a checkout:

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cp .env.example .env
# Edit .env: replace the admin token and encryption key, and point METAPI_IMAGE at the
# release tag or digest you intend to run.
docker compose pull
docker compose up -d
```

The shipped Compose file binds the host side to `127.0.0.1:4000` by default
and stores SQLite in the `metapi_data` volume. The full lifecycle is in
[Docker Compose](#docker-compose).

**One-off container.** If you do not want a checkout:

```bash
export AUTH_TOKEN=$(openssl rand -hex 32)
export ACCOUNT_CREDENTIAL_SECRET=$(openssl rand -hex 32)

docker pull ghcr.io/deliciousbuding/metapi-go:0.26.0
docker run -d --name metapi \
  -p 127.0.0.1:4000:4000 \
  -e AUTH_TOKEN \
  -e ACCOUNT_CREDENTIAL_SECRET \
  -e TZ=Asia/Shanghai \
  -v metapi_data:/app/data \
  --restart unless-stopped \
  ghcr.io/deliciousbuding/metapi-go:0.26.0
```

The named volume is the zero-config default; bind mounts need a one-time
`chown -R 1001:1001` — see [Data directory & volumes](#data-directory--volumes).

### 2. Release binary

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/latest/download/install.sh | bash
```

The script downloads the matching platform binary from GitHub Releases,
verifies its SHA-256 against `checksums.txt`, and installs to
`/usr/local/bin/metapi` (override with `METAPI_INSTALL_PREFIX`; pin a version
with `METAPI_VERSION=v0.26.0`). Windows: download `metapi-windows-amd64.exe`
from the release page instead. Set the admin token and start the server:

```bash
export AUTH_TOKEN=$(openssl rand -hex 16)      # admin UI login token
metapi
```

Data lands in `./data` (SQLite, code default) unless `DATA_DIR` / `DATABASE_URL` say otherwise.

### 3. From source

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token ./metapi
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
> verified account and one route covering the model (channels bind themselves
> from account model availability). Create the route manually or enable the
> opt-in automatic model-route setting, then rebuild — without usable routes the proxy answers
> an honest `503 {"error":{"message":"No available channels","type":"server_error"}}`
> (tested). The full walkthrough lives in
> [getting-started.md](getting-started.md) (§2–§5).

## Environment Variables

[`configuration.md`](configuration.md) owns the complete environment-variable
reference: names, aliases, precedence, defaults and clamps. [`.env.example`](../.env.example)
is the template the parity test checks against `config.Load`; do not maintain a
second default table here.

For deployment, only these decisions need attention:

- `AUTH_TOKEN` is optional at first start. If it is unset or still the shipped
  placeholder, startup generates a 32-byte random token and writes it once to
  `DATA_DIR/admin-token` (0600). An explicit value shorter than 8 bytes refuses
  startup; shorter than 16 bytes logs a warning.
- Set a unique 32+ byte `ACCOUNT_CREDENTIAL_SECRET` before storing upstream
  credentials; values shorter than 8 bytes are critical, and an unset value
  falls back to the admin token with a warning.
- `PROXY_TOKEN` is only a one-time legacy import. Create downstream keys in
  **API Keys**; changes to `PROXY_TOKEN` do not restore a migrated key.
- `METAPI_IMAGE`, `METAPI_BIND_ADDRESS` and `METAPI_PORT` are Compose-only and
  are listed under
  [Docker Compose only](configuration.md#docker-compose-only).

## Docker Compose

`docker-compose.yml` is the only runtime Compose entry in the repository. It
requires Compose >= 2.24 for optional `env_file`.

1. Copy the template and replace the required values:

```bash
cp .env.example .env
```

```dotenv
METAPI_IMAGE=ghcr.io/deliciousbuding/metapi-go:0.26.0
AUTH_TOKEN=<new random admin token>
ACCOUNT_CREDENTIAL_SECRET=<new random 32+ byte secret>
```

2. Start the service and verify readiness:

```bash
docker compose up -d
docker compose ps
curl -fsS http://127.0.0.1:4000/ready
# {"status":"ok","database":"ok"}
```

3. Follow logs when needed:

```bash
docker compose logs -f metapi
```

The host side is `METAPI_BIND_ADDRESS` (default `127.0.0.1`) and
`METAPI_PORT` (default `4000`); the container port stays `4000`. Set
`METAPI_BIND_ADDRESS=0.0.0.0` only when another host must reach the port
directly — otherwise keep the reverse proxy on loopback.

The service runs read-only with all Linux capabilities dropped, a bounded
`/tmp` and bounded JSON logs. The named volume is
`metapi_data:/app/data`; the image owns the health check. The Compose project
is named `metapi-go`; set `COMPOSE_PROJECT_NAME` to run a second stack on the
same host.

### Image tags

| Tag | Published by | Use |
|:----|:-------------|:----|
| `vX.Y.Z` / `X.Y.Z` | stable tag | Same digest. Pin this or a digest for production. |
| `X.Y` | stable tag | Moving alias to the newest patch in that minor. |
| `latest` | stable tag | Newest stable release; updated only by stable tags. |
| `edge` | `master` push | Tip of master. Development only. |
| `sha-<short>` | `master` push | Per-commit build. Re-running the same commit produces a new digest. |

The published image is a multi-architecture manifest for `linux/amd64` and
`linux/arm64`; Docker selects the matching architecture. Architecture-specific
intermediate images are addressed by digest during publication and are not
published as user-facing tags.

`vX.Y.Z` and `X.Y.Z` are immutable once published: rerunning a failed release
must bump the patch version rather than replace bytes a deployment may have
pinned. If the manifest is published but the anonymous pull or signature check
fails, the release is blocked and the next attempt also needs a new patch
version. `X.Y` and `latest` remain moving aliases by design.

### Verify the image signature

Every published architecture image and the final multi-architecture manifest
from this pipeline onward are signed with keyless cosign through GitHub OIDC.
Releases published before the change (`v0.25.0` and earlier) have no signature.
To verify a release, set `VERSION` to the release you are checking:

```bash
VERSION=0.26.0  # replace with the exact release, without the leading v
cosign verify "ghcr.io/deliciousbuding/metapi-go:${VERSION}" \
  --certificate-identity "https://github.com/DeliciousBuding/metapi-go/.github/workflows/main.yml@refs/tags/v${VERSION}" \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

For an `edge` image, use the same command with
`@refs/heads/master` in place of `@refs/tags/v${VERSION}`.

Pin an explicit release tag or digest when the exact bytes matter; do not rely
on `latest` or `edge` for production.

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

- **Image tags**: pin a release tag or digest (for example
  `ghcr.io/deliciousbuding/metapi-go:0.26.0`). `edge` and `sha-*` are master
  builds; `latest` follows stable releases but is still a moving alias.

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

`TRUSTED_PROXY_CIDRS` also gates the other forwarding hints: `X-Forwarded-Proto` is honored for `Secure` session/monitor cookies and for exported key URLs only when the direct peer is listed, and `X-Forwarded-Host` is only a fallback for exports when the proxy rewrote `Host` to an empty value. Direct clients cannot spoof these headers. The nginx example above preserves `Host $host`, so exported URLs use the public hostname; alternatively set `SERVER_ADDRESS` explicitly.

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

Use PostgreSQL for multi-instance deployments. Side-effecting schedulers use PostgreSQL advisory locks, so only one replica runs each job batch at a time. `usage-aggregation` uses its own checkpoint lease.

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
- Metrics: `GET /metrics` needs no token and serves Prometheus text directly; see [API health](api/health.md#get-metrics) for the series contract. Scrape it from a network you control, or allowlist the path in your reverse proxy.
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
docker compose up -d
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

Change `METAPI_IMAGE` in `.env` to the new release tag or digest, then:

```bash
docker compose pull
docker compose up -d
docker compose ps
curl -fsS http://127.0.0.1:4000/ready
```

Compose recreates the container when the resolved image or configuration
changed. Auto-migration runs at startup. To roll back, restore the backup, set
`METAPI_IMAGE` back to the previous tag or digest, and run
`docker compose up -d`.

Upgrade checklist:

1. Back up the database first (cp data/hub.db ... or pg_dump -Fc ...).
2. Pin the exact release tag or digest selected for your deployment (for example `ghcr.io/deliciousbuding/metapi-go:vX.Y.Z` or `@sha256:...`). Do not use `edge` or a moving alias.
3. After upgrade, verify GET /ready returns 200 and run bash web/scripts/verify-live-assets.sh http://127.0.0.1:4000.
4. **Upgrading from v0.17.x or older? Re-bind each upstream account once.**
   Those versions stored the upstream *dashboard session* as the account
   credential — on New API-family sites that is a JWT which expires in about
   fifteen minutes — and no schema migration can turn it into the durable
   credential v0.18.0+ uses. The upgrade keeps the account row, so everything
   looks bound; then model refresh and relay start failing once that session is
   gone (`503 No available channels`, an empty model list, `balance refresh
   failed: upstream credential expired`). Re-bind from **添加账号 (Add account)**
   with the *same site and the same username*: the backend upserts that account,
   exchanges the login for the site's durable credential, revokes the transient
   session, and re-syncs its models immediately. There is no separate
   "re-login" button for password/session accounts. Accounts bound with a
   *durable API key* instead need nothing here: that credential does not expire
   on a fifteen-minute clock, so it survives the upgrade as-is — and if the site
   later rotates it, replace it in place (row menu → **编辑 (Edit)** → new key →
   **保存修改 (Save changes)**). This was verified on a real
   v0.16.23 → v0.19.0 in-place upgrade of a live SQLite database: 28 additive
   migrations, site / account / downstream key / route all preserved, and the
   chain only served a real completion again after this step.
5. Rollback = restore the pre-upgrade backup and set `METAPI_IMAGE` back to the previous pinned tag or digest. Schema changes are forward-only; do not downgrade the schema by deleting rows from schema_migrations.

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
