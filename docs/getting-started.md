# Getting Started

**Last updated**: 2026-10-17

A single guided path from zero to your first proxied request, in about five
minutes. If you already know your way around, the [README quick start](../README.md)
and [`deployment.md`](deployment.md) are the fast lane.

**You will end with**: a running Metapi, one upstream connected, a public
model route, a downstream key, and one `/v1` request answered through the gateway.

**Prerequisites**: one install path from
[deployment.md](deployment.md#prerequisites) — Docker Compose 2.24+ for the
container path, or a release binary — plus an API key or supported account for
an upstream platform. A direct API-key connection does not require a site
account. Use the account path below when you also need balance,
token-management or check-in features supported by that platform.

Prefer a single binary over Docker? Install it with the published installer and
run it with your admin token. The [README quick start](../README.md) covers
all three install paths. Everything below is identical either way.

## 1. Start the server

Install and start Metapi with one of the paths in
[deployment.md](deployment.md#quick-paths). The Compose path is the default for
a long-lived install: copy `.env.example` to `.env`, replace the admin token and encryption key,
pin `METAPI_IMAGE` to a release tag or digest, and run `docker compose up -d`.

Verify before continuing:

```bash
curl -fsS http://127.0.0.1:4000/ready
# {"status":"ok","database":"ok"}
```

Open `http://127.0.0.1:4000` and sign in with `AUTH_TOKEN`. Data layout,
upgrades, PostgreSQL and rollback are owned by [deployment.md](deployment.md)
and are not repeated here. Coming from the TypeScript version? Follow
[migration.md](migration.md) first.

## 2. Connect an upstream

For an API key, open **通道 (Channels)** at `/channels` and choose **接入平台
(Connect platform)**. Select the product, paste its key, and supply a Base URL
if it is self-hosted. Product variants such as pay-as-you-go and Coding Plan
have different endpoints and model catalogs; select the one your key belongs to.

Quick connect creates the connection, credential, model grants and public routes
as one operation. Review the resulting models and public route names in the
connection's detail. Discovered models and preset lists describe configuration;
test a request before treating a model as working. If routes are already
created, continue at §5 without creating a site or account.

For OAuth subscriptions, use **上游 → OAuth** and the provider's authorization
flow. The following site/account path remains available for supported management
platforms.

### Optional: add a managed site

Navigate to **站点 (Sites)** → **添加站点**.

1. Enter the site URL, e.g. `https://api.example.com`.
2. Pick the platform from the searchable list (or let auto-detect do it).
3. **创建**.

Metapi stores the site and marks it active. You can add as many sites as you
have accounts scattered across platforms.

## 3. Optional: bind a site account

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
> **Credentials age out — and which fix applies depends on how the account was
> bound.** The health reason on the account row tells you in about thirty
> seconds, so you do not have to know how Metapi stores credentials:
>
> | The row says | What died | What to do |
> | :--- | :--- | :--- |
> | `Access token expired` | A **session / password** credential. A hand-copied New API dashboard JWT lives only minutes; a PAT issued by password login can instead be revoked, expire, or lose the needed scopes | Re-bind through **添加账号 (Add account)** with the *same site and username*. Metapi reuses a valid scoped PAT; if its grant is insufficient, adjust it in New API or import a suitable PAT. There is no separate "re-login" button |
> | `Connection expired; update the API key` | An **API key** credential — the `sk-…` key the site itself issued — was rotated or revoked upstream | Replace it in place: row menu → **编辑 (Edit)** → paste the new key → **保存修改 (Save changes)** |
> | `Credentials expired; update the credentials` | An **OAuth** connection | Row menu → **重新绑定 (Rebind)** on the OAuth connections page. OAuth accounts otherwise renew themselves from their refresh token |
> | Nothing: models still list, and balance refresh does not report `upstream credential expired` | Nothing | Nothing to do. A durable `sk-` API key does not expire on a fifteen-minute clock, so an account bound with one survives an upgrade untouched |
>
> Password re-login recovery belongs to the check-in workflow, not to a failed
> proxy request. Repair or rebind a failed management credential explicitly.
> A failed model refresh retains the last observed availability; it is not an
> empty-model answer and does not necessarily invalidate a separate relay key.

The account row shows balance and health once the first refresh lands
(balance refresh runs hourly by default; use the row action to refresh now).

## 4. Review or build a public route

Quick connect normally creates the public routes. Use the public model name
shown in the route, not an upstream alias or a display-only label. A Key's
model mapping can intentionally map that client name to another routed model.
Account connections or custom routing can be configured as follows.

Navigate to **路由 (Token Routes)** → **添加路由 (Add route)**.

1. Set the model pattern: an exact model name (`gpt-4o-mini`), a glob
   (`gpt-4*`), or a regex.
2. Save. Channels bind themselves from the models your accounts already expose —
   no channel picking, no weights to configure.

One route per model you want to expose is enough to start; a glob or regex route
covers a whole family in one row. Open a route's detail to inspect:

- configured weight + enabled share per channel,
- normalized input/output price per concrete model, with the price source.

**For a fleet of accounts**, you can instead enable **自动创建模型路由
(Automatically create model routes)** in **设置 → 运维 → 任务调度
(Settings → Operations → Scheduling)**, then run **自动重建 (Rebuild)**. The
setting is off by default. It creates one exact route for each usable model not
covered by an existing route; the display name is the model name. Existing
wildcards, groups, disabled routes and manual bindings are kept.

Rebuild runs as an observable background task: you can leave the page and come
back to its status and counts. Check partial model-refresh failures rather than
treating a completed local rebuild as proof every upstream works. Subsequent
model syncs add missing routes while the setting is on and detach automatic
channels for delisted models. Turning the setting off stops new route creation,
not synchronization of existing automatic channels; retained route IDs keep
downstream route grants stable.

**If the upstream blocks model discovery:** in the API-key account form,
explicitly enable **跳过模型获取 (Skip model fetch)** to save a key without
verifying its model list. Then add an exact route using a model name you know
the upstream supports and select the account in **manual channel bindings**
(the list is independent of discovery, and supports Select all). No model
availability is invented by skipping the fetch: verify the first real relay
request before treating the connection as usable. API-key-only connections do
not gain balance/check-in management from this option.

## 5. Make your first proxied request

Open **访问 → API 密钥** at `/downstream-keys`, create a downstream key and
grant it the intended models or routes. The **连接 (Connect)** dialog opens
after creation; you can reopen it from the key's row action. Enter your admin
token to unlock it, check the model list, select a conversational model, then
send a test chat in **验证调用 (Test request)**. Use **配置客户端 (Client setup)**
for client imports and configuration exports.
The generation uses real quota. This verifies one non-streaming Chat request;
test your client's protocol, streaming and tools separately.

The model tester uses an administrator test context. It does not establish that
this downstream key is allowed to call the same model. Use the key's permission
check or the route workspace when investigating admission or routing.

Set `DOWNSTREAM_API_KEY` to the key you created, then send a request:

```bash
curl http://localhost:4000/v1/chat/completions \
  -H "Authorization: Bearer $DOWNSTREAM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<any routed model>",
    "messages": [{ "role": "user", "content": "hello" }]
  }'
```

The router selects an eligible candidate according to the route's strategy.
Check **请求与诊断 → 代理日志 (Proxy logs)** for status, latency, tokens and
estimated cost. The default **请求记录 (Requests)** view shows one row
per request, including denials before upstream dispatch. Open its detail for
recorded upstream attempts, or open **尝试记录 (Attempt records)** to
inspect individual attempts. Older logs without a request record remain in
the attempts view.

For a current check, open the route workspace at `/token-routes`, enter the
client model and protocol, and select the downstream key. It evaluates admission
and candidate eligibility without calling the upstream. Its result describes
the current configuration, not the decision made by an earlier request.

## 6. Point your tools at Metapi

Everything that speaks the OpenAI protocol now only needs:

- **Base URL**: `http://<your-host>:4000/v1`
- **API key**: a downstream access token from **API 密钥 (API Keys)** in the sidebar

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
| `401` on `/v1`                   | `Authorization: Bearer` uses an access token from API Keys |
| `503` from the proxy             | No route/channel configured for the model, or upstream unconfigured |
| `503 No available channels: 未匹配到启用的路由` | No **enabled route** matches that model: add one or enable optional automatic creation and rebuild (§4). Existing disabled routes are not re-enabled |
| Models were listed, then went empty / relay started failing | The stored credential aged out. Session/password account: re-bind with **Add account**, same site + username. API-key account: **Edit** the account and paste the new key (§3) |
| Account shows unhealthy          | Row action → refresh; verify credential mode matches the platform |
| Page not loading behind nginx    | WebSocket upgrade headers in the reverse proxy ([deployment.md](deployment.md)) |

More answers in [`faq.md`](faq.md); report defects via
[SECURITY.md](../SECURITY.md) / GitHub issues.
