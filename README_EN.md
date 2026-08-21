<p align="center">
  <img src="docs/assets/hero.png" alt="Metapi Go" width="720">
</p>

<h1 align="center">Metapi Go</h1>

<p align="center">
  <strong>The relay-of-relays — aggregate scattered AI API sites into one unified gateway</strong>
</p>

<p align="center">
  One key for every AI gateway. <br>
  Unify the New API / One API / OneHub / Sub2API sites you registered everywhere into
  <strong>a single API key and a single entrypoint</strong> — with automatic model
  discovery, smart routing and cost-optimal channel selection.
</p>

<p align="center">
  <a href="README.md"><strong>中文</strong></a> ·
  <a href="README_EN.md">English</a>
</p>

<p align="center">
  <a href="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml"><img alt="CI" src="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml/badge.svg?branch=master"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DeliciousBuding/metapi-go?logo=github&label=release&color=blue"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go"><img alt="Docker" src="https://img.shields.io/badge/ghcr-latest-2496ED?logo=docker&logoColor=white"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-3DA639?logo=opensourceinitiative&logoColor=white"></a>
</p>

---

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/dashboard.webp" alt="Dashboard" style="width:100%;height:auto;"/>
      <div><b>Dashboard</b> — balance snapshot, check-ins, scheduler health</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/models.webp" alt="Model marketplace" style="width:100%;height:auto;"/>
      <div><b>Model marketplace</b> — cross-site coverage, brands, live metrics</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/routes.webp" alt="Smart routing" style="width:100%;height:auto;"/>
      <div><b>Smart routing</b> — probabilistic multi-channel allocation, cost-first</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/accounts.webp" alt="Accounts" style="width:100%;height:auto;"/>
      <div><b>Accounts</b> — multi-site accounts with health tracking</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/sites.webp" alt="Sites" style="width:100%;height:auto;"/>
      <div><b>Sites</b> — upstream site configuration at a glance</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/proxy-logs.webp" alt="Proxy logs" style="width:100%;height:auto;"/>
      <div><b>Proxy logs</b> — request logs with latency, tokens and cost</div>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs/assets/screenshots/model-tester.webp" alt="Model tester" style="width:100%;height:auto;"/>
      <div><b>Model tester</b> — compare channel outputs side by side</div>
    </td>
    <td align="center">
      <img src="docs/assets/screenshots/settings.webp" alt="Settings" style="width:100%;height:auto;"/>
      <div><b>Settings</b> — global parameters, theming and security</div>
    </td>
  </tr>
</table>

---

## What is Metapi

The AI ecosystem is full of aggregation relays built on New API / One API and
friends, and balances, model lists and API keys end up scattered across many
of them. **Metapi** is the **meta-aggregation layer** above those relays: it
unifies your sites behind **one entrypoint**, so every downstream tool —
Cursor, Claude Code, Codex, Open WebUI and anything that speaks OpenAI —
reaches all of your models transparently.

Supported upstreams:

- **Aggregation panels**: New API, One API, OneHub, DoneHub, Veloera, AnyRouter, Sub2API
- **Compatible endpoints**: OpenAI / Claude / Gemini compatible APIs, plus `cliproxyapi` / CPA
- **OAuth connections**: Codex, Claude, Gemini CLI, Antigravity

| Pain point                                 | How Metapi solves it                                                    |
| ------------------------------------------ | ----------------------------------------------------------------------- |
| One key per site, a pile of client configs | **Unified proxy entry + multiple downstream keys**, models aggregated on `/v1/*` |
| No idea which site is cheapest for a model | **Smart routing** picks the best channel by cost, balance and usage     |
| A site goes down, manual switching         | **Automatic failover**: failed channels cool down, request retries on the next |
| Balances scattered everywhere              | **Central dashboard** at a glance, with low-balance alerts              |
| Daily check-ins on every site              | **Scheduled auto check-in** with reward tracking                        |
| No idea which site has which model         | **Auto model discovery** — new upstream models enter routing with zero config |

This project is a Go rewrite of [Metapi (TypeScript)](https://github.com/cita-777/metapi)
with client-visible compatibility:

|                | Node.js (original) | Go (this repo)   |
| -------------- | ------------------ | ---------------- |
| Memory         | ~85 MB             | ~20 MB           |
| Docker image   | ~250 MB            | ~15 MB           |
| Startup        | 5-10 s             | instant          |
| Deployment     | Node runtime       | single binary    |

---

## Quick start (3 minutes)

The release page ships pre-built binaries for Linux / macOS / Windows — one file, no runtime.

**1. Install** (Linux / macOS; on Windows download `metapi-windows-amd64.exe` directly from [Releases](https://github.com/DeliciousBuding/metapi-go/releases/latest)):

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/latest/download/install.sh | bash
```

The script verifies the SHA-256 checksum and installs to `/usr/local/bin/metapi`.
If `/usr/local` is not writable, point `METAPI_INSTALL_PREFIX` at a directory of
your choice (e.g. `METAPI_INSTALL_PREFIX=~/.local` installs to `~/.local/bin/metapi`).

**2. Start** (only two tokens are required):

```bash
export AUTH_TOKEN=$(openssl rand -hex 16)      # admin UI login token
export PROXY_TOKEN=sk-$(openssl rand -hex 24)  # downstream key for /v1/* calls
metapi
```

**3. Verify**:

```bash
curl http://localhost:4000/health
# {"status":"ok"}
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

Open `http://localhost:4000` and sign in with `AUTH_TOKEN`. Data lives in `./data`
(SQLite) by default and survives removing the token environment variables.

> The path above was tested end to end. If the port is taken, override with
> `PORT=<port>` (default 4000). Then add sites and accounts in the UI, run the
> automatic route rebuild, and make your first proxied request — full walkthrough:
> [getting started](docs/getting-started.md).

## Deployment

### Docker

> The commands below were cross-checked line by line against the repo `Dockerfile` /
> `docker-compose.prod.yml` (no Docker available in the test environment; not executed).

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

Key points:

- The image runs as a non-root user (uid 1001). A **named volume** (`metapi_data`
  above) inherits the image ownership on first mount — zero configuration. If you
  use a bind mount such as `./data:/app/data` instead, run
  `chown -R 1001:1001 ./data` on the host first; see the
  [deployment guide](docs/deployment.md).
- `ACCOUNT_CREDENTIAL_SECRET` encrypts the upstream credentials stored in the
  database. Generate an independent 32+ byte random secret; when unset it falls
  back to `AUTH_TOKEN`.
- For production, pin a version tag (e.g. `:v0.16.6`) instead of `latest`.

### Docker Compose

`docker-compose.prod.yml` (GHCR image + production hardening) or
`docker-compose.yml` (local build):

```bash
cp .env.example .env   # fill in AUTH_TOKEN / PROXY_TOKEN / ACCOUNT_CREDENTIAL_SECRET
docker compose -f docker-compose.prod.yml up -d
```

### From source

Requires Go 1.26+ and Bun 1.x (the frontend must be built first; the output is
embedded into the binary via `go:embed`):

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

Reverse proxy, PostgreSQL, upgrades and rollback: [deployment guide](docs/deployment.md).

---

## Your first proxied request

After adding at least one upstream site with an account in the UI and running
"Auto-rebuild routes" (see [getting started](docs/getting-started.md)), call
Metapi exactly like OpenAI:

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<any routed model>",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

Metapi picks the cheapest healthy channel across all of your upstream sites;
failed channels cool down automatically and the request retries on the next one.
Claude native format (`/v1/messages`), Responses, Embeddings, Images,
`/v1/models` and `/v1/files` are supported the same way; full endpoint inventory:
[HTTP API](docs/api.md), client setup: [client-integration](docs/client-integration.md).

With no routes configured, proxy requests return an honest 503 (tested):

```json
{ "error": { "message": "No available channels", "type": "server_error", "request_id": "…" } }
```

---

## Core features

### Unified proxy gateway

OpenAI **and** Claude downstream formats for every mainstream client. Chat
Completions, Responses, Messages, Completions (legacy), Embeddings, Images,
Models and the standard `/v1/files` endpoint, with full SSE streaming and
automatic OpenAI ⇄ Claude conversion.

### Smart routing engine

- Zero-config route tables from automatic model discovery
- Four-level cost truth: measured → configured → models.dev catalog → fallback
- Probabilistic multi-channel allocation weighted by cost, balance and usage
- Automatic cooldown and avoidance of failed channels; requests retry on other channels
- Runtime circuit breaker + half-open probing so recovering channels re-enter under control

### Multi-platform aggregation

**16** adapters in total (`platform/`): `new-api`, `one-api`, `one-hub`,
`done-hub`, `veloera`, `anyrouter`, `sub2api`, `openai`, `claude`, `gemini`,
`gemini-cli`, `codex`, `antigravity`, `grok`, `cliproxyapi`, `sensetime`.
Model enumeration, balance queries, token management and proxying are generic;
login/check-in capabilities vary per platform.

### Accounts & tokens

Multi-site, multi-account, multiple API tokens per account. Four-state health
machine (`healthy` / `unhealthy` / `degraded` / `disabled`); credentials
encrypted at rest; automatic re-login on token expiry; disabling a site cascades
to its accounts.

### Model marketplace & tester

Cross-site model coverage, per-site price comparison, latency and success-rate
metrics; an interactive tester that forces specific channels and compares outputs.

### Check-in & balances

Scheduled check-in (default daily at 08:00) with reward parsing, failure
notifications and a concurrency lock against duplicates; scheduled balance
refresh (default hourly) with income tracking and spend trend analysis.

### Alerts

Nine channels: Webhook, Bark, ServerChan, Telegram Bot, SMTP email, Feishu,
DingTalk, WeCom, ntfy. Covers low balance, site/account incidents, check-in
failures, proxy failures, token expiry and daily digests — with per-type muting
and cooldowns against duplicates.

### Operations & audit

Audit log for admin operations; realtime QPS / success-rate panel (WebSocket
stream); batch model verification, model rate editing, model redirect mappings,
tags; dashboard snapshot PNG export.

### Lightweight deployment

A single binary plus a local data directory is enough to run, or attach an
external PostgreSQL; SQLite / PostgreSQL dual dialect with idempotent schema
upgrades at startup; full data import/export.

---

## Configuration

All configuration is driven by environment variables; only two are required to boot:

| Variable      | Description                                  |
| ------------- | -------------------------------------------- |
| `AUTH_TOKEN`  | Admin web UI login token                     |
| `PROXY_TOKEN` | Downstream key for `/v1/*` proxy calls       |

Common options:

| Variable                  | Default              | Description                                          |
| ------------------------- | -------------------- | ---------------------------------------------------- |
| `ACCOUNT_CREDENTIAL_SECRET` | falls back to `AUTH_TOKEN` | Upstream credential encryption key; a 32+ byte random secret is recommended |
| `PORT`                    | `4000`               | Listen port                                          |
| `HOST`                    | platform-dependent   | Windows defaults to `127.0.0.1`, other platforms `0.0.0.0` |
| `DATA_DIR`                | `./data`             | Data directory (SQLite database and uploaded files)  |
| `DATABASE_URL`            | empty                | PostgreSQL connection string; empty = SQLite         |
| `LOG_LEVEL`               | `info`               | Log level: `debug` / `info` / `warn` / `error`       |
| `CHECKIN_CRON`            | `0 8 * * *`          | Check-in schedule                                    |
| `BALANCE_REFRESH_CRON`    | `0 * * * *`          | Balance refresh schedule                             |

The complete reference (~150 variables: PostgreSQL pool presets, proxy limits,
rate limiting, CORS, trusted proxies, notification channels, …) lives in the
[configuration reference](docs/configuration.md) and [`.env.example`](.env.example).

### Health checks

- `GET /health` — liveness; 200 as long as the HTTP process is alive
- `GET /ready` — readiness; checks the database; 503 while unavailable or draining
- The Docker image ships `metapi healthcheck` (equivalent to probing `/ready`; exit codes tested)

---

## Documentation

| Document                                                     | Purpose                          |
| ------------------------------------------------------------ | -------------------------------- |
| [docs/getting-started.md](docs/getting-started.md)           | **Getting started**: install → first proxied request |
| [docs/deployment.md](docs/deployment.md)                     | Deployment / reverse proxy / PostgreSQL / upgrades |
| [docs/configuration.md](docs/configuration.md)               | Full environment variable reference |
| [docs/client-integration.md](docs/client-integration.md)     | Client setup (Cursor / Claude Code / Codex / Open WebUI) |
| [docs/api.md](docs/api.md)                                   | HTTP API endpoint inventory      |
| [docs/migration.md](docs/migration.md)                       | TS → Go migration (SQLite / PG / MySQL) |
| [docs/faq.md](docs/faq.md)                                   | Frequently asked questions       |
| [docs/architecture.md](docs/architecture.md)                 | Package map & request paths (developers) |
| [docs/README.md](docs/README.md)                             | Documentation map                |
| [CHANGELOG.md](CHANGELOG.md)                                 | Release notes                    |

## Migrating from the TypeScript version

The database schema is identical: stop the old server and start the Go version
with the same environment variables; the idempotent migration runs at startup.
The Go image runs as uid 1001 — a bind-mounted data directory previously written
by the root-owned TypeScript container needs `chown -R 1001:1001 ./data` first
(named volumes need no action). SQLite / PostgreSQL deployments are taken over
directly; MySQL data must be exported through the TypeScript version's built-in
migration first. Full steps, the `metapi-migrate` tool and rollback:
[migration guide](docs/migration.md).

## Known limitations

- A few admin endpoints currently return an honest `501` (not implemented); see
  the "501 residual" markers in [api.md](docs/api.md). For version updates use
  GitHub Releases / GHCR image tags.
- With no upstream configured, proxy requests return 503 — never a synthetic success.
- Single-process semantics are the primary target; the shared semantics for
  multi-instance deployments (Redis-shared RPM/TPM admission, PostgreSQL
  advisory locks) are described in the [FAQ](docs/faq.md).

---

## Development

### Backend (Go)

```bash
make build    # build
make test     # all tests (incl. -race)
make vet      # go vet
make lint     # golangci-lint
make vuln     # govulncheck vulnerability scan
```

### Frontend (`web/`, Bun)

```bash
cd web
bun install
bun run dev         # local dev (/api /v1 proxied to the backend on :4000)
bun run typecheck   # tsgo type check
bun run test        # vitest full suite
bun run build       # rsbuild build (output embedded into the Go binary via go:embed)
```

Contribution flow (branch model, PR gates): [CONTRIBUTING.md](CONTRIBUTING.md).

## Contributing & security

- [CONTRIBUTING.md](CONTRIBUTING.md) — branch model, PR flow, local gates
- [SECURITY.md](SECURITY.md) — vulnerability disclosure (Security Advisory)
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — community conduct

## Related projects

- [Metapi (TypeScript)](https://github.com/cita-777/metapi) — the original Node.js implementation this repo rewrites in Go
- [New API](https://github.com/QuantumNous/new-api) — a primary upstream
- [One API](https://github.com/songquanpeng/one-api) — the classic OpenAI-interface aggregator

## License

[MIT](LICENSE). Metapi is fully self-hosted: all data stays in your own
deployment, and proxy traffic flows directly between your server and your
upstream sites.
