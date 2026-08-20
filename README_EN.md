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
      <div><b>Smart routing</b> — probabilistic multi-channel allocation</div>
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

## Introduction

The AI ecosystem is full of aggregation relays built on New API / One API and
friends. Managing balances, model lists and API keys across many of them is
scattered and slow.

**Metapi** is a **meta-aggregation layer** above those relays: it unifies your
sites behind **one entrypoint (with per-project downstream keys)**, so every
downstream tool — Cursor, Claude Code, Codex, Open WebUI and anything that
speaks OpenAI — reaches all of your models transparently. Supported upstreams:

- **Aggregation panels**: New API, One API, OneHub, DoneHub, Veloera, AnyRouter, Sub2API
- **Compatible endpoints**: OpenAI / Claude / Gemini compatible APIs, plus `cliproxyapi` / CPA
- **OAuth connections**: Codex, Claude, Gemini CLI, Antigravity

| Pain point                                    | How Metapi solves it                                        |
| --------------------------------------------- | ----------------------------------------------------------- |
| One key per site, a pile of client configs    | **Unified proxy entry + downstream keys**, models aggregated on `/v1/*` |
| No idea which site is cheapest for a model    | **Smart routing** picks the best channel by cost, balance and usage |
| A site goes down, manual switching            | **Automatic failover** with per-channel cooldown            |
| Balances scattered everywhere                 | **Central dashboard** with low-balance alerts               |
| Daily check-ins on every site                 | **Scheduled auto check-in** with reward tracking            |
| No idea which site has which model            | **Auto model discovery** — new upstream models appear with zero config |

### How the Go version differs

A ground-up Go rewrite of [Metapi (TypeScript)](https://github.com/cita-777/metapi)
with client-visible compatibility, in a much lighter runtime:

|                | Node.js (original) | Go (this repo)   |
| -------------- | ------------------ | ---------------- |
| Memory         | ~85 MB             | ~20 MB           |
| Docker image   | ~250 MB            | ~15 MB           |
| Startup        | 5-10 s             | instant          |
| Deployment     | Node runtime       | single binary    |

---

## Quick start

### Docker (recommended)

```bash
docker run -d --name metapi \
  -p 4000:4000 \
  -e AUTH_TOKEN=your-admin-token \
  -e PROXY_TOKEN=your-proxy-sk-token \
  -e TZ=Asia/Shanghai \
  -v ./data:/app/data \
  --restart unless-stopped \
  ghcr.io/deliciousbuding/metapi-go:latest
```

Open `http://localhost:4000` and sign in with `AUTH_TOKEN`.

> [!IMPORTANT]
> Change `AUTH_TOKEN` and `PROXY_TOKEN` — never ship the defaults. Data lives
> in `./data` and survives upgrades.

### Docker Compose

```bash
mkdir metapi && cd metapi

cat > docker-compose.yml << 'EOF'
services:
  metapi:
    image: ghcr.io/deliciousbuding/metapi-go:latest
    ports:
      - "4000:4000"
    volumes:
      - ./data:/app/data
    environment:
      AUTH_TOKEN: ${AUTH_TOKEN:?AUTH_TOKEN is required}
      PROXY_TOKEN: ${PROXY_TOKEN:?PROXY_TOKEN is required}
      CHECKIN_CRON: "0 8 * * *"
      BALANCE_REFRESH_CRON: "0 * * * *"
      PORT: ${PORT:-4000}
      DATA_DIR: /app/data
      TZ: ${TZ:-Asia/Shanghai}
    restart: unless-stopped
EOF

export AUTH_TOKEN=your-admin-token
export PROXY_TOKEN=your-proxy-sk-token
docker compose up -d
```

Reverse proxy, PostgreSQL and hardening details: [deployment guide](docs/deployment.md).
A full install-to-first-request walkthrough: [getting started](docs/getting-started.md).

### From source

```bash
git clone https://github.com/DeliciousBuding/metapi-go.git
cd metapi-go
go build -o metapi ./cmd/server
AUTH_TOKEN=your-admin-token PROXY_TOKEN=your-proxy-sk-token ./metapi
```

On Windows, an empty `HOST` binds `127.0.0.1` by default to avoid recurring
firewall prompts; set `HOST=0.0.0.0` for LAN access.

---

## Your first proxied request

Create a downstream key in the UI (or use `PROXY_TOKEN`), then call Metapi
exactly like OpenAI:

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

Metapi selects the cheapest healthy channel across all your sites; failed
channels cool down and the request retries on the next one. Claude native
(`/v1/messages`), Responses, Embeddings, Images, `/v1/models` and `/v1/files`
work the same way. Full endpoint inventory: [HTTP API](docs/api.md); client
setup for Cursor / Claude Code / Codex / Open WebUI:
[client integration](docs/client-integration.md).

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
- Failure cooldown + half-open probing so recovering channels re-enter safely

### Multi-platform aggregation

**16 adapters**: `new-api`, `one-api`, `onehub`, `done-hub`, `veloera`,
`anyrouter`, `sub2api`, `openai`, `claude`, `gemini` and more — covering model
enumeration, balance queries, token management and proxying; login/check-in
capabilities per platform.

### Accounts & tokens

Multi-site, multi-account, multiple API tokens per account. Four-state health
machine; credentials encrypted at rest; automatic re-login on expiry;
disabling a site cascades to its accounts.

### Model marketplace & tester

Cross-site model coverage, per-site price comparison, latency and success
metrics; an interactive tester that forces specific channels and preserves
honest status/latency/errors.

### Check-in & balances

Scheduled check-in (default 08:00 daily) with reward parsing and failure
alerts; hourly balance refresh; daily/cumulative spend trends.

### Alerts

Nine channels: Webhook, Bark, ServerChan, Telegram Bot, SMTP, Feishu (HMAC),
DingTalk (HMAC), WeCom, ntfy. Low balance, site/account incidents, check-in
failures, proxy failures, token expiry, daily digests — with per-type muting
and cooldowns.

### Operations & audit

Admin audit log for mutating operations; realtime QPS/success-rate panel
(WebSocket with reconnect); batch model verification; model rate overview
with inline editing; redirect mapping; tags; dashboard snapshot PNG export.

### Lightweight deployment

One container plus a local data directory, or external PostgreSQL. SQLite
and PostgreSQL dual dialect with automatic additive schema upgrades at
startup. ~15 MB image, instant start, full import/export.

---

## Configuration

Two required variables to boot:

| Variable      | Description                                  |
| ------------- | -------------------------------------------- |
| `AUTH_TOKEN`  | Admin web UI login token                     |
| `PROXY_TOKEN` | Downstream key for `/v1/*` proxy calls       |

Common options:

| Variable           | Default     | Description                              |
| ------------------ | ----------- | ---------------------------------------- |
| `PORT`             | `4000`      | Listen port                              |
| `HOST`             | per-platform | Windows defaults `127.0.0.1`, servers `0.0.0.0`; containers fixed `0.0.0.0` |
| `DATABASE_URL`     | empty       | PostgreSQL DSN; empty = SQLite           |
| `CHECKIN_CRON`     | `0 8 * * *` | Check-in schedule                        |
| `BALANCE_REFRESH_CRON` | `0 * * * *` | Balance refresh schedule             |

The complete reference (PostgreSQL pool presets, proxy limits, CORS, trusted
proxy CIDRs, notification providers, …) lives in
[configuration reference](docs/configuration.md) and [`.env.example`](.env.example).

### Health checks

- `GET /health` — liveness
- `GET /ready` — readiness incl. database; 503 while unavailable or draining
- Docker runs `metapi healthcheck`, equivalent to probing `/ready`

---

## Migrating from the TypeScript version

The schema is identical: stop the old server and start Metapi Go with the
same environment — `./data` is reused and the schema migrates itself.
MySQL-based deployments convert to PostgreSQL with the standalone
`metapi-migrate` tool. Steps and rollback: [migration guide](docs/migration.md).

---

## Documentation

| Document                                                       | Purpose                          |
| -------------------------------------------------------------- | -------------------------------- |
| [docs/getting-started.md](docs/getting-started.md)             | Install → first proxied request  |
| [docs/deployment.md](docs/deployment.md)                       | Deployment / reverse proxy / PG  |
| [docs/configuration.md](docs/configuration.md)                 | Full environment reference       |
| [docs/client-integration.md](docs/client-integration.md)       | Cursor / Claude Code / Codex / Open WebUI |
| [docs/api.md](docs/api.md)                                     | HTTP API inventory               |
| [docs/migration.md](docs/migration.md)                         | TS → Go / SQLite → PG            |
| [docs/faq.md](docs/faq.md)                                     | Frequently asked questions       |
| [docs/architecture.md](docs/architecture.md)                   | Package map & request paths      |
| [docs/README.md](docs/README.md)                               | Docs map (incl. maintainer docs) |
| [CHANGELOG.md](CHANGELOG.md)                                   | Release notes                    |

---

## Development

```bash
# backend
make build && make test && make vet && make lint && make vuln

# frontend (web/, Bun)
cd web && bun install && bun run dev     # /api /v1 proxy to :4000
bun run typecheck && bun run test && bun run build
```

Contribution flow (branch model, PR gates): [CONTRIBUTING.md](CONTRIBUTING.md).

---

## Contributing & security

- [CONTRIBUTING.md](CONTRIBUTING.md) — branch model, PR flow, local gates
- [SECURITY.md](SECURITY.md) — responsible disclosure
- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — community conduct

---

## Related projects

- [Metapi (TypeScript)](https://github.com/cita-777/metapi) — the original Node.js implementation this repo rewrites in Go
- [New API](https://github.com/QuantumNous/new-api) — a primary upstream
- [One API](https://github.com/songquanpeng/one-api) — the classic OpenAI-interface aggregator

---

## Privacy

Metapi is fully self-hosted: all data (accounts, tokens, routes, logs) stays
in your deployment and nothing is reported anywhere; proxy traffic flows only
between your server and your upstream sites.

---

## License

[MIT](LICENSE)
