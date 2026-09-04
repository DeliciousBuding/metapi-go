# Getting Started

**Last updated**: 2026-09-04

A single guided path from zero to your first proxied request, in about five
minutes. If you already know your way around, the [README quick start](../README.md)
and [`deployment.md`](deployment.md) are the fast lane.

**You will end with**: a running Metapi, one upstream site connected, one
account verified, one route serving models, and one `/v1` request answered
through the unified gateway.

**Prerequisites**: Docker (any recent version) and one upstream account on a
supported platform (New API, One API, OneHub, DoneHub, Veloera, AnyRouter,
Sub2API, or an OpenAI/Claude/Gemini compatible endpoint).

Prefer a single binary over Docker? Install it with the published installer and
run it with the same two tokens — the [README quick start](../README.md) covers
all three install paths. Everything below is identical either way; the commands
shown use Docker.

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

> `ACCOUNT_CREDENTIAL_SECRET` is the key that encrypts stored account
> credentials. Unset, it falls back to `AUTH_TOKEN`; below 8 bytes the server
> refuses to start, below 16 it warns. Set a 32+ byte random secret
> (`openssl rand -hex 32`) for anything that is not a throwaway.

> All data lives in `./data` on your host. Upgrading = pull a new image and
> `docker compose up -d`; the schema migrates automatically at startup.
> Coming from the TypeScript version? [migration.md](migration.md) covers the
> three takeover paths: SQLite and PostgreSQL data are taken over directly
> after stopping the old server; MySQL data moves via the TS admin's database
> migration first.

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

> **Which credential mode?** For a New API-family panel (New API, AnyRouter)
> use **password login**: Metapi signs in, exchanges the short-lived dashboard
> session for that site's durable personal access token, revokes the transient
> session, and syncs the account's models immediately — no waiting for the
> nightly cron. A session token pasted by hand from such a panel *is* that
> short-lived dashboard JWT: it verifies, it saves, and it stops working within
> minutes, which later shows up as an empty model list and
> `503 No available channels`. For every other platform, use the credential the
> site itself issues for API access (usually an `sk-` key).
>
> **Credentials age out.** Re-bind by choosing **添加账号 (Add account)** again
> with the *same site and the same username*: the backend upserts that account
> with the fresh credential and re-syncs its models. There is no separate
> "re-login" button for password/session accounts.

The account row shows balance and health once the first refresh lands
(balance refresh runs hourly by default; use the row action to refresh now).

## 4. Build a route

Navigate to **路由 (Token Routes)** → **添加路由 (Add route)**.

1. Set the model pattern: an exact model name (`gpt-4o-mini`), a glob
   (`gpt-4*`), or a regex.
2. Save. Channels bind themselves from the models your accounts already expose —
   no channel picking, no weights to configure.

One route per model you want to expose is enough to start; a glob or regex route
covers a whole family in one row. Open a route's detail to inspect:

- configured weight + enabled share per channel,
- normalized input/output price per concrete model, with the price source.

> **What 自动重建 (Auto-rebuild) does, and does not, do.** It re-scans your
> accounts' models and recomposes the channels of the routes you *already have*.
> **It does not create routes.** On a fresh install with no routes it completes
> with `routesConsidered: 0` and changes nothing, and the UI says exactly that.
> Run it after binding or re-binding an account, or when an upstream's model
> list changes — not as the step that makes your first request work. The step
> that does that is **Add route** above.

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
- **API key**: a downstream key from **API 密钥 (API Keys)** in the sidebar, or
  the `$PROXY_TOKEN` you started the server with

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
| `503 No available channels: 未匹配到启用的路由` | No **enabled route** matches that model: add one (§4). Auto-rebuild does not create routes |
| Models were listed, then went empty / relay started failing | The stored credential aged out: re-bind with **Add account**, same site + username (§3) |
| Account shows unhealthy          | Row action → refresh; verify credential mode matches the platform |
| Page not loading behind nginx    | WebSocket upgrade headers in the reverse proxy ([deployment.md](deployment.md)) |

More answers in [`faq.md`](faq.md); report defects via
[SECURITY.md](../SECURITY.md) / GitHub issues.
