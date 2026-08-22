<p align="center">
  <img src="docs/assets/hero.png" alt="Metapi Go" width="720">
</p>

<h1 align="center">Metapi Go</h1>

<p align="center">
  <strong>The relay-of-relays — aggregate scattered AI API sites into one unified gateway</strong>
</p>

<p align="center">
  One key for every AI gateway.<br>
  Unify the New API / One API / OneHub / Sub2API sites you registered everywhere into
  <strong>a single API key and a single entrypoint</strong> — with automatic model
  discovery, smart routing and cost-optimal channel selection.
</p>

<p align="center">
  <a href="README.md">中文</a> ·
  <a href="README_EN.md"><strong>English</strong></a>
</p>

<p align="center">
  <a href="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml"><img alt="CI" src="https://github.com/DeliciousBuding/metapi-go/actions/workflows/main.yml/badge.svg?branch=master"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/DeliciousBuding/metapi-go?logo=github&label=release&color=blue"></a>
  <a href="https://github.com/DeliciousBuding/metapi-go/pkgs/container/metapi-go"><img alt="Docker" src="https://img.shields.io/badge/ghcr-latest-2496ED?logo=docker&logoColor=white"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26.6-00ADD8?logo=go">
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-3DA639?logo=opensourceinitiative&logoColor=white"></a>
</p>

<p align="center">
  <a href="#features"><strong>Features</strong></a> ·
  <a href="#screenshots">Screenshots</a> ·
  <a href="#quick-start-3-minutes"><strong>Quick Start</strong></a> ·
  <a href="#deployment--configuration">Deployment</a> ·
  <a href="#migrating-from-the-typescript-version">Migration</a> ·
  <a href="#known-limitations">Limitations</a>
</p>

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

## Features

| Capability                 | Description                                                                                                                                    |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| **16 upstream adapters**   | New API / One API / OneHub / DoneHub / Veloera / AnyRouter / Sub2API / OpenAI / Claude / Gemini / Gemini CLI / Codex / Antigravity / Grok / CLIProxyAPI / SenseTime |
| **Unified proxy**          | OpenAI and Claude protocols side by side: Chat / Responses / Messages / Embeddings / Images / Models / Files, full SSE streaming, automatic conversion |
| **Routing & fault tolerance** | Automatic model discovery builds route tables with zero config; multi-channel allocation weighted by cost / balance / usage; failed channels cool down while the request retries on the next; runtime circuit breaker with half-open probing |
| **Cost ground truth**      | Four-level cost signal (measured → account-configured → models.dev catalog → fallback); every request logged with tokens and cost            |
| **Admin UI**               | Sites / accounts / routes / models / logs / alerts in one SPA, pre-built and embedded into the binary — no separate frontend service needed  |
| **Operations automation**  | Scheduled check-ins, scheduled balance refresh, nine alert channels, batch model verification, audit log, realtime QPS panel                 |
| **Lightweight deployment** | A single binary is enough; SQLite by default, PostgreSQL optional; idempotent schema upgrades run automatically at startup                    |
| **Drop-in TS takeover**    | Same database schema, same environment variable names, same API contract — stop the old server and start this one with the same variables     |

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

## Quick start (3 minutes)

Three equivalent ways — pick any one. Starting the server needs only two tokens:
`AUTH_TOKEN` (admin UI login) and `PROXY_TOKEN` (the downstream key for `/v1/*`).

### Option 1: Release binary (recommended)

Pre-built binaries for Linux / macOS / Windows — one file, no runtime:

```bash
curl -fsSL https://github.com/DeliciousBuding/metapi-go/releases/download/v0.16.6/install.sh | bash

export AUTH_TOKEN=$(openssl rand -hex 16)      # admin UI login token
export PROXY_TOKEN=sk-$(openssl rand -hex 24)  # downstream key for /v1/* calls
metapi
```

The script verifies the SHA-256 checksum and installs to `/usr/local/bin/metapi`
(it installs the latest release by default; pin a version with `METAPI_VERSION`,
change the install location with `METAPI_INSTALL_PREFIX`). On Windows, download
`metapi-windows-amd64.exe` directly from [Releases](https://github.com/DeliciousBuding/metapi-go/releases/latest).

### Option 2: Docker (named volume, zero config)

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

A named volume inherits the image ownership on first mount — it just works.
If you prefer a bind mount (`./data:/app/data`), run `chown -R 1001:1001 ./data`
on the host first. With Compose (production-hardened config):

```bash
cp .env.example .env   # fill in AUTH_TOKEN / PROXY_TOKEN / ACCOUNT_CREDENTIAL_SECRET
docker compose -f docker-compose.prod.yml up -d
```

### Option 3: From source

Requires Go 1.26+ and Bun 1.x. The frontend must be built first — the output is
embedded into the binary via `go:embed`, and skipping it fails with
`pattern dist: no matching files found`:

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
cd web && bun install --frozen-lockfile && bun run build:web && cd ..
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

### Verify

```bash
curl http://localhost:4000/health
# {"status":"ok"}
curl http://localhost:4000/ready
# {"status":"ok","database":"ok"}
```

Open `http://localhost:4000` and sign in with `AUTH_TOKEN`. Data lives in
`./data` (SQLite) by default. If the port is taken, override with `PORT=<port>`
(default 4000).

## Your first proxied request

After adding at least one upstream site with an account in the UI and running
"Auto-rebuild routes" (full walkthrough: [getting started](docs/getting-started.md)),
call Metapi exactly like OpenAI:

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
With no routes configured, you get an honest 503:

```json
{ "error": { "message": "No available channels", "type": "server_error", "request_id": "…" } }
```

Claude native format (`/v1/messages`), Responses, Embeddings, Images,
`/v1/models` and `/v1/files` are supported the same way; full endpoint inventory:
[HTTP API](docs/api.md), client setup: [client-integration](docs/client-integration.md).

---

## Deployment & configuration

All configuration is driven by environment variables; only two are required to boot:

| Variable      | Description                                  |
| ------------- | -------------------------------------------- |
| `AUTH_TOKEN`  | Admin web UI login token                     |
| `PROXY_TOKEN` | Downstream key for `/v1/*` proxy calls       |

Common options:

| Variable                  | Default                    | Description                                    |
| ------------------------- | -------------------------- | ---------------------------------------------- |
| `ACCOUNT_CREDENTIAL_SECRET` | falls back to `AUTH_TOKEN` | Upstream credential encryption key; a 32+ byte random secret is recommended |
| `PORT`                    | `4000`                     | Listen port                                    |
| `DATA_DIR`                | `./data`                   | Data directory (SQLite database and uploads)   |
| `DATABASE_URL`            | empty                      | PostgreSQL connection string; empty = SQLite   |
| `LOG_LEVEL`               | `info`                     | Log level: `debug` / `info` / `warn` / `error` |
| `CHECKIN_CRON`            | `0 8 * * *`                | Check-in schedule                              |
| `BALANCE_REFRESH_CRON`    | `0 * * * *`                | Balance refresh schedule                       |

The complete reference (~150 variables) lives in the
[configuration reference](docs/configuration.md) and [`.env.example`](.env.example).

**Health checks**: `GET /health` (liveness), `GET /ready` (readiness, checks the
database); the Docker image ships `metapi healthcheck`, which probes `/ready`.

**Further reading**:

| Document                                                     | Purpose                                              |
| ------------------------------------------------------------ | ---------------------------------------------------- |
| [docs/getting-started.md](docs/getting-started.md)           | **Getting started**: install → first proxied request |
| [docs/deployment.md](docs/deployment.md)                     | Reverse proxy / TLS / PostgreSQL / backup / upgrades |
| [docs/configuration.md](docs/configuration.md)               | Full environment variable reference                  |
| [docs/client-integration.md](docs/client-integration.md)     | Client setup (Cursor / Claude Code / Codex / Open WebUI) |
| [docs/api.md](docs/api.md)                                   | HTTP API endpoint inventory                          |
| [docs/migration.md](docs/migration.md)                       | TS → Go migration (SQLite / PG / MySQL)              |
| [docs/faq.md](docs/faq.md)                                   | Frequently asked questions                           |
| [docs/architecture.md](docs/architecture.md)                 | Package map & request paths (developers)             |
| [CHANGELOG.md](CHANGELOG.md)                                 | Release notes                                        |

## Migrating from the TypeScript version

The database schema is identical: stop the old server and start the Go version
with the same environment variables; the idempotent migration runs at startup.
SQLite / PostgreSQL deployments are taken over directly; MySQL data must be
exported through the TypeScript version's built-in migration first. The Go image
runs as uid 1001 — a bind-mounted data directory previously written by the
root-owned TypeScript container needs `chown -R 1001:1001 ./data` first (named
volumes need no action). Full steps, the `metapi-migrate` tool and rollback:
[migration guide](docs/migration.md).

## Known limitations

- A few admin endpoints currently return an honest `501` (not implemented); see
  the "501 residual" markers in [api.md](docs/api.md).
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

## Verification notes

Every command in this documentation was verified as summarized below — stated
once here instead of scattered through the text:

| Path                                            | How it was verified                                            |
| ----------------------------------------------- | -------------------------------------------------------------- |
| Release binary: install → start → health checks | Tested end to end (v0.16.6)                                    |
| From source: frontend build → go build → start  | Tested end to end                                              |
| Docker / Compose commands                       | Cross-checked line by line against the repo `Dockerfile` / `docker-compose.prod.yml` |
| 503 without routes, healthcheck exit codes      | Tested                                                         |

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
