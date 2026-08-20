# Getting Started

**Last updated**: 2026-08-20

A single guided path from zero to your first proxied request, in about five
minutes. If you already know your way around, the [README quick start](../README.md)
and [`deployment.md`](deployment.md) are the fast lane.

**You will end with**: a running Metapi, one upstream site connected, one
account verified, one route serving models, and one `/v1` request answered
through the unified gateway.

**Prerequisites**: Docker (any recent version) and one upstream account on a
supported platform (New API, One API, OneHub, DoneHub, Veloera, AnyRouter,
Sub2API, or an OpenAI/Claude/Gemini compatible endpoint).

## 1. Start the server

```bash
mkdir metapi && cd metapi

export AUTH_TOKEN=$(openssl rand -hex 16)     # admin login token
export PROXY_TOKEN=sk-$(openssl rand -hex 24) # downstream proxy key

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
      DATA_DIR: /app/data
      TZ: ${TZ:-Asia/Shanghai}
    restart: unless-stopped
EOF

docker compose up -d
```

**Verify**: `curl http://localhost:4000/health` prints `{"status":"ok"}`.
Open `http://localhost:4000` and log in with `$AUTH_TOKEN`.

> All data lives in `./data` on your host. Upgrading = pull a new image and
> `docker compose up -d`; the schema migrates automatically at startup.

## 2. Add your first site

Navigate to **站点 (Sites)** → **添加站点**.

1. Enter the site URL, e.g. `https://api.example.com`.
2. Pick the platform from the searchable list (or let auto-detect do it).
3. **创建**.

Metapi stores the site and marks it active. You can add as many sites as you
have accounts scattered across platforms.

## 3. Add an account and verify credentials

Navigate to **账号 (Accounts)** → **添加账号**.

1. Select the site you just created.
2. Choose a credential mode: **Session token / API key / password login**,
   and paste the credential.
3. Press **验证** — Metapi performs a real verification call against the site
   before saving, so a bad token fails here instead of later.
4. **添加账号**.

The account row shows balance and health once the first refresh lands
(balance refresh runs hourly by default; use the row action to refresh now).

## 4. Build a route

Navigate to **路由 (Token Routes)** → **重建全部路由 (Auto-rebuild)**.

Auto-rebuild discovers every model available on your accounts and creates one
route per model with weighted channels across the accounts that serve it —
zero manual configuration. Open a route's detail to inspect:

- configured weight + enabled share per channel,
- normalized input/output price per concrete model, with the price source.

Prefer manual control? Create a route yourself and pick exact-match,
glob/wildcard, or regex model patterns, then attach channels.

## 5. Make your first proxied request

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $PROXY_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<any routed model>",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

The response comes back from whichever channel the router selected (cost,
balance and usage weighted). Check **使用日志 (Proxy Logs)** to see the row
with status, latency, tokens and estimated cost.

## 6. Point your tools at Metapi

Everything that speaks the OpenAI protocol now only needs:

- **Base URL**: `http://<your-host>:4000/v1`
- **API key**: a downstream key (see **下游密钥**, or `$PROXY_TOKEN`)

Per-client walkthroughs (Cursor, Claude Code, Codex, Open WebUI, client
config export) live in [`client-integration.md`](client-integration.md).

## What to explore next

- **签到 (Check-in)** — enable per-account auto check-in to collect daily rewards.
- **模型广场 (Models)** — cross-site coverage, price comparison, live tester.
- **告警 (Settings → Notifications)** — balance-low and failure alerts via
  Telegram / Bark / Webhook / SMTP and more.
- **可观测性 (Observability)** — realtime QPS / success-rate panel.
- [`deployment.md`](deployment.md) — reverse proxy, PostgreSQL, hardening.
- [`configuration.md`](configuration.md) — every environment variable.

## When something goes wrong

| Symptom                          | First check                                            |
| :------------------------------- | :----------------------------------------------------- |
| `401` on `/v1`                   | `Authorization: Bearer` uses a downstream key or `PROXY_TOKEN` |
| `503` from the proxy             | No route/channel configured for the model, or upstream unconfigured |
| Account shows unhealthy          | Row action → refresh; verify credential mode matches the platform |
| Page not loading behind nginx    | WebSocket upgrade headers in the reverse proxy ([deployment.md](deployment.md)) |

More answers in [`faq.md`](faq.md); report defects via
[SECURITY.md](../SECURITY.md) / GitHub issues.
