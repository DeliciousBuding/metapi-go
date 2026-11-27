# Architecture Overview

**Last updated**: 2026-11-26

> **Navigation**: full docs map in [`docs/README.md`](README.md) · route list in [`api/routes-inventory.md`](api/routes-inventory.md) · environment variables in [`configuration.md`](configuration.md).
>
> Every structural claim below names the package or exported symbol that owns
> it, so it can be checked with a search instead of trusted. The dependency
> rules in [Ownership and boundary map](#ownership-and-boundary-map) are
> enforced by `docs/package_boundary_test.go`; the route claims are enforced
> by `docs/api_inventory_parity_test.go`; the environment-variable claims are
> enforced by `docs/env_parity_test.go`.

Metapi Go is a ground-up rewrite of the TypeScript Metapi proxy gateway in Go. This document describes the **as-built** package layout, request paths, and key design decisions. Package dependency rules are stated in full below and machine-enforced by [`docs/package_boundary_test.go`](package_boundary_test.go).

> **Naming truth:** There is **no** `proxycore/` or `protocol/` package in this repository. The proxy engine is `proxy/` (with `proxy/profiles` and `proxy/types`). Protocol conversion is `transform/` (with `anthropic/messages` for the Messages-to-Chat request/return bridge, `openai` [completions/embeddings/images/responses], `gemini`, and `shared`). There is **no** `transform/canonical` intermediate layer — cross-protocol conversion is native (e.g. OpenAI→Gemini) and bypasses any canonical representation. Older docs or TS-era names that say “ProxyCore package” or “protocol package” refer to these real packages.

## 产品主线与实现入口

这是一个网关，不另建聊天产品或通用任务平台。默认体验围绕接入、调用、
诊断和恢复；先复用现有入口，只有当前任务缺少可执行路径时才新增能力。

| 用户任务 | 现有入口 | 责任边界 |
| --- | --- | --- |
| 接入上游、确认模型与授权 | 站点/账号/独立上游、渠道编辑 | `service/upstream`、`service/oauth`、`store`；目录发现不等于授权 |
| 配置下游客户端并实际调用 | API Key、客户端接入、原生 `/v1/*` | `auth` 负责 Key 与预算，`routing` 选路，`handler/proxy` 执行并结算 |
| 查清一次调用为什么失败 | 使用日志的 request/attempt 详情 | 正常代理账本保留尝试、用量和安全原因；审计日志记录管理/API 操作 |
| 比较或验证指定渠道 | “渠道诊断”（保留 `/model-tester`） | `handler/admin` 同步探针；记审计但不进普通用量账本，不替代下游端到端验收 |
| 恢复冷却渠道 | `scheduler.ChannelRecoveryScheduler` | 只处理符合资格的冷却候选；健康模型探测仍归 `ModelProbeScheduler` |

`cmd/server`、`app` 组合依赖，`router` 挂载协议/管理入口及预构建 SPA。
请求循环归 `handler/proxy/upstream.go`；`proxy.RuntimeExecutor` 只拥有有界
HTTP/SSE transport，接收原生 `http.Request` 并返回原生 `http.Response`，
不另做请求模型、缓冲结果或账本。协议转换归 `transform`，持久化归 `store`。
它们不是一条所有请求都必须依次经过的抽象流水线。

当前没有 session-channel 租约协调器或等待队列。`app.proxyLoadProvider` 是
无状态兼容信号：session-scoped 渠道的配置 limit 为正时，`least_busy` 保持
历史零负载排序；活动/等待计数为零，不能理解为真实容量观测。实际限流由站点/
direct credential 的 `proxy.SiteConcurrencyLimiter` 与渠道
`internal/channellimit` 拥有，见[配置说明](configuration.md#实际生效的并发上限)。
恢复任务不再经过空租约 ID 钩子，也不会因此开启健康渠道 SQL 探测。

参考取舍：采用 [Magpie 的 transport/逐次尝试边界](https://github.com/yetone/magpie/blob/e513cba54a47eabfb59b75b362b63c30968c213c/internal/gateway/upstream_transport.go) 与
[AxonHub 的指定渠道测试](https://github.com/looplj/axonhub/blob/a47f4a56b63b957d94aaa15c1381095818ec9d14/frontend/src/features/channels/components/channels-test-dialog.tsx)、
[渠道/网关选择区分](https://github.com/looplj/axonhub/blob/a47f4a56b63b957d94aaa15c1381095818ec9d14/frontend/src/features/playground/selection.ts)。
不因此导入额外重放、并行探测或设置；现有授权、协议、取消和计费边界不变。

## Ownership and boundary map

The layering is a **denylist of import edges**, not a file inventory: each
package owns one decision, and the test named below fails the build if a
package reaches outside its decision. `docs/package_boundary_test.go` is the
authoritative, executable form of this table — when the two disagree, the test
is right and this page is stale.

| Package | Decision it owns | Must NOT import |
|:--------|:-----------------|:----------------|
| `config` | Env parsing, defaults, clamping, validation. Nothing else reads `os.Getenv` for a documented knob. | (leaf) |
| `store` | Persistence and dialect differences (SQLite / PostgreSQL), placeholder rebinding. | `handler`, `proxy`, `routing`, `service`, `scheduler`, `router`, `auth`, unapproved `internal/*` |
| `platform` | One adapter per upstream provider family; the leaf that actually talks to upstreams. | `store`, `handler`, `proxy`, `router`, `scheduler` (`config` and `proxy/profiles` are allowed) |
| `transform` | Wire-format conversion between protocols (native OpenAI ⇄ Gemini, no intermediate canonical form). | `handler`, `store`, `proxy`, `routing`, `service`, `auth` |
| `routing` | Which channel/account serves a request: selection, cooldown, runtime breaker. | `proxy`, `handler` |
| `service` | Domain logic that spans stores and platforms. | `handler`, `router`, `proxy`, `scheduler` |
| `scheduler` | When background work runs (cron, interval, window). | `handler`, `router`, `proxy` |
| `handler/*` | HTTP surface only: decode, authorize, call a service/proxy entrypoint, encode. | `router` (`handler/proxy ↛ scheduler`, `handler/admin ↛ handler/proxy`) |
| `router` | Mount points, middleware order, SPA fallback, static asset serving. | — |
| `cmd/server` | Composition root; may import anything. | — |
| `cmd/channel-plugin-maintain` | 显式离线维护入口；计划/环境变量选择与输出，领域事务交给 `service/channelplugins` | 不启动网关、不自动迁移，不挂载在线回收入口 |

Approved exceptions are enumerated in [`docs/package_boundary_test.go`](package_boundary_test.go)
(for example `handler/admin → scheduler` for cron validation, `app → handler/proxy`
for the upstream composition helper, `platform → proxy/profiles` for client
detection, `handler/proxy → transform/*` for one-way protocol wiring, and the
`storeAllowedInternalImports` allowlist).
Both `docs/package_boundary_test.go` and [`docs/coupling_ratchet_test.go`](coupling_ratchet_test.go)
enforce stale-exemption failure: once an allowed store import, direct `sqlx` admin handler,
or global `store.GetDB()` lookup is removed, the gate fails until its baseline
entry is deleted. Production files must not import `testing` or `net/http/httptest`.
The rule for adding an exception is: move the import to an allowed edge first;
only if that is impossible, add a scoped exception with written justification.

`proxycore/` and `protocol/` do not exist and must not be reintroduced — the
test rejects those TypeScript-era names.

The test also names what it does **not** police: standalone `cmd` tools, `e2e`, `docs`,
`web` and most of `internal/` are out of scope, because those are repo-private
helpers and fixtures rather than a layer (`store → internal/*` is allowlisted
explicitly so persistence stays free of domain packages).

## Request paths: stages and single owners

Two independent auth surfaces share one chi router built by `router.New`, the
only place the route tree is composed — `cmd/server` stays the composition root
for dependencies. Registrars take a `chi.Router` and register absolute
`/api/...` paths, so each can also be exercised on a standalone router in
tests.

### Middleware applied to every request (`router.New`, in order)

`WithRequestID` → `SecurityHeaders` → `TrustedRealIP(cfg)` → `RequestLogger`
→ `Recoverer` → `BodyLimitPathAware(cfg.RequestBodyLimit, cfg.FileUploadLimitBytes)`.

`TrustedRealIP` runs before the logger so the logged client IP is already the
policy-approved one, and it only honours `X-Forwarded-For` / `X-Real-IP` from
peers inside `TRUSTED_PROXY_CIDRS`. `BodyLimitPathAware` takes two limits
because the multipart upload surfaces (`/v1/files`, `/v1/images/*`) need a
larger cap than the JSON surfaces.

### `/health`, `/ready`, `/metrics`

Registered with `r.With(CORS())` **before** the admin group, so they never
pass through admin auth — a container orchestrator or a load balancer cannot
present a token. Handlers live in `app` (`app.Health`, `app.Ready`,
`app.PrometheusHandler`).

### Admin surface (`/api/*`)

One `r.Group` owns the whole admin surface, and the order inside it is
load-bearing:

1. `AdminCORS(cfg)`
2. `auth.AdminRateLimit` — per-IP bucket for all `/api/*`
3. `auth.AuthRateLimit` — stricter bucket, `/api/auth/*` only, because login is
   the only surface that accepts the master token
4. `auth.OAuthRateLimit` — stricter bucket, `/api/oauth/*` only
5. `auth.AdminAuth(sessions)` — dual track: server-side session cookie (UI) or
   bearer master token (scripts)
6. `auth.RequireReauth()` — sensitive operations must re-present the master
   token in `X-Admin-Confirm-Token`
7. `admin.AuditMiddleware(db.DB)` — audit trail for admin writes

Rate limiting deliberately runs **before** authentication: a rejected login
consumes the per-IP bucket instead of bypassing it, so credential brute force
is capped like any other admin traffic.

Registrars that need a database are mounted inside
`if db != nil` using the database captured when the router is constructed. When the database is unavailable those
routes are simply not registered and the router logs
`router: database not initialized, P3 routes skipped`; the session manager is
`nil` and the session handlers fail closed rather than allowing traffic.
`/api/about` and the session lifecycle routes are mounted outside that guard
because every field they serve comes from the linker, the Go runtime, or a
fail-closed handler.

### Ordinary-user surface (`/api/user/*`)

`handler/user.RegisterRoutes` is mounted outside the admin group and owns its
same-origin and rate-limit guards. `auth.UserSessionAuth` accepts only the
independent user cookie, reloads current state, and sets a typed user principal
with no administrator authority. `store.UserAuthStore` owns bcrypt credential
digests/versioned session references; `store.GatewayUserStore` owns scoped
personal Keys and safe request/usage reads. Administrator activation remains in
`handler/admin.RegisterUserActivationRoutes` behind confirmation and audit.
The `/user` SPA page uses an independent session query and does not bootstrap
the administrator session or invoke its authentication-error redirect.

`handler/proxy/user_budget.go` attaches ordinary-user admission/settlement to the
existing ingress lifecycle through a narrow response-writer capability. HTTP
requests and WS turns share the server request owner; internal bridge calls do
not create a second hold. `store.GatewayUserStore` owns the durable integer
token-credit ledger and snapshot rates, independent of upstream-cost accounting
and asynchronous logs. Missing usage retains a hold, while administrator-only
reconciliation is exposed by `admin.RegisterUserBudgetRoutes`. The personal
budget registrar is inside user session auth. Supported protocol and estimation
limits are specified in [user authentication/quota](api/auth.md#user-wide-token-quota).

### Data plane (`/v1/*`)

`r.Route("/v1", ...)` owns the proxy surface, in this order:

1. `ProxyWriteDeadline`
2. `CORS()`
3. `auth.ProxyRateLimit(cfg.ProxyRateLimitRPM)` — per-IP, **before** auth so an
   unauthenticated flood is dropped without a database lookup
4. `auth.ProxyAuth()` — resolves the managed downstream key, applies its
   admission limits and puts the result in the request context
5. `proxyhandler.RegisterProxyRoutes(r)` — the surface itself
7. `admin.RegisterDownstreamPricingRoutes(r, db.DB)` — the downstream-key-visible
   price catalog, mounted here so it inherits proxy auth instead of admin auth

A request then moves through stages that each have exactly one owner:

| Stage | Owner | Decision made here |
|:------|:------|:-------------------|
| HTTP surface | `handler/proxy` | Decode the inbound protocol shape, resolve the downstream key context, relay the upstream body (buffered or SSE), encode the response or the protocol-shaped error envelope. |
| Attempt loop | `handler/proxy` | Which endpoint candidate is tried next, how a stream relay ended, and how that ending is reported (recorded status, reason text, terminal metric outcome). |
| Failure and fallback policy | `proxy` (with `proxy/profiles`, `proxy/types`) | Which client profile this is, how many channel attempts are allowed, and every reusable verdict: content-level failure judgement, same-site abort, downgrade to the next endpoint, retry the same channel, refresh its auth. |
| Channel selection | `routing` | Which channel/account is eligible right now: weighting, cooldown, runtime breaker state. |
| Protocol conversion | `transform` (`openai`, `gemini`, `shared`) | Rewriting between wire formats. Native conversion — there is no canonical intermediate representation. |
| Upstream I/O | `platform` + `service` | Speaking to the actual provider, including outbound proxy and TLS behaviour. |
| Persistence | `store` | Request/usage logging and every read the stages above need, with dialect differences hidden here. |

请求结果与上游尝试分别记录。`handler/proxy/request_lifecycle.go` 在代理鉴权之前分配内部请求身份，
观察实际 HTTP 提交与写出失败，并消费派发/协议处理器的结束结果；每个 HTTP 调用或 Responses
WebSocket turn 由一个 owner 写入 `gateway_requests`。WS 升级只建立连接，不以 101 表示 turn 完成。
`proxy_logs.gateway_request_id` 关联尝试，外部追踪 ID 可重复；历史不回填终态。
请求保存观测到的尝试次数，详情与异步落库行数不一致时标记不完整，不能把丢失记录当作零派发。
失败请求的请求级 `reason_code` 保留首个上游失败的安全分类，如 `upstream_auth_rejected` 或
`upstream_first_byte_timeout`；后续恢复成功时也保留该恢复前原因。原始错误正文仍只在对应
attempt 中，不复制到请求摘要。最终 HTTP 结果与首个原因分别呈现，避免最后的泛化失败覆盖首因。
写入失败仅输出带内部 ID 的 WARN；取消后的记录使用有界、与客户端取消解耦的写入 context。
请求与尝试共用既有留存策略，不引入正文存储、外部队列或新的调度服务。
两者也复用 `PROXY_LOG_ASYNC` 控制的有界批写队列、刷新周期和停机排空 owner；
队列满时同步回退，批量失败时逐行隔离。异步模式不承诺进程崩溃前已入队数据持久化，
列表短暂缺少请求/尝试不等于零派发；需要即时可见时使用既有同步模式。

Streaming and non-streaming responses take different paths through the attempt
loop. A non-streaming body is buffered up to `PROXY_MAX_BUFFERED_RESPONSE_BYTES`
and judged once; a stream is relayed under a chunk-gap guard
(`PROXY_STREAM_IDLE_TIMEOUT_SEC`) and a total byte bound
(`PROXY_MAX_STREAM_RESPONSE_BYTES`), then classified by how the relay ended:
clean end of stream, idle timeout, upstream fault mid-stream, byte-limit
truncation, or downstream disconnect.

Two rules keep that classification honest:

- **One judge for content.** A single pure function in `proxy` applies the
  optional `PROXY_ERROR_KEYWORDS` and `PROXY_EMPTY_CONTENT_FAIL` heuristics to
  facts supplied by the buffered body or bounded SSE analyser. Explicit SSE
  error events also fail without either heuristic being enabled; prior output,
  usage, or a later `[DONE]` cannot turn an observed error into success.
- **A downstream disconnect alone is not an upstream fault.** A client cancel
  does not penalize a healthy channel, but cannot erase an upstream error
  already parsed before the disconnect. Usage extracted from earlier SSE
  events is still accounted; tokens are never invented for missing content.

Every other ending — an idle upstream, a mid-stream reset, a truncated relay —
goes through the failure path, whose status, reason text and terminal metric
outcome come from one owner so channel health, request logs and metrics cannot
disagree about the same request. Which candidate is tried next is likewise a
single decision function in the attempt loop: HTTP statuses, content failures
and transport errors all ask it, instead of each call site re-deriving the
policy. `transform` owns none of this.

### Non-`/v1` proxy aliases

Codex native paths (`/chat/completions`, `/responses`, `/responses/*`) and the
Gemini surface (`/v1beta/models`, `/gemini/{geminiApiVersion}/models`,
`/v1internal::*`) are mounted in an `r.Group` carrying the same middleware
stack as `/v1`. A group is used instead of `Route("/")` so proxy auth applies
only to the exact registered paths and never shadows the SPA fallback.

### Two surfaces that are intentionally outside the admin group

- **`/monitor-proxy/ldoh*`** — registered via `r.HandleFunc` outside the bearer
  group, because an iframe's sub-resource requests cannot carry an
  `Authorization` header. The handler enforces its own HttpOnly cookie auth
  scoped to `/monitor-proxy/` and rejects `..` traversal.
- **`/api/admin/ops/ws`** (`admin.RegisterOpsWSRoutes`) — mounted after the
  admin group for the same browser limitation: a WebSocket handshake cannot
  set headers, so it redeems a one-time ticket minted by
  `POST /api/auth/ws-ticket` instead of ever seeing the master token.

### Static assets and SPA fallback

`router.setupSPAFallback` serves the embedded `web/dist` tree. One rule sets
every cache header: **a content-hashed name is `immutable`, anything else
revalidates**. `/static/*` is the only asset subtree mounted and every name in
it is hashed, so it gets `public, max-age=31536000, immutable`. The files the
build copies into the dist root are not hashed, so they get `no-cache` —
`bootstrap.js`, `theme-init.js`, the logos and favicons, and the `index.html`
the fallback serves — which is what lets a deploy propagate to an
already-visited client without a hard refresh.

There is no `/assets/*` mount. One sat here for a legacy Vite layout until it
was removed: `//go:embed` bakes exactly one dist into a binary, so there is no
older build to stay compatible with, and the embedded tree has had no `assets/`
directory since the Rsbuild migration. `router/spa_layout_test.go` pins the
replacement contract — every root-relative asset the built `index.html`
references must be served as that asset and never as the fallback HTML, and
mounting the shipped dist must not warn — while
`router/static_cache_policy_test.go` pins the cache header of each class above.
The fallback answers `index.html` for non-API paths and a JSON 404 body for
`/api/*` and `/v1/*`.

## Package Layout (as-built)

```
metapi-go/
├── cmd/
│   ├── server/             # Main server entry point
│   ├── migrate/            # Standalone SQLite→PG migration tool
│   └── channel-plugin-maintain/ # Explicit offline archive/capture maintenance
├── app/                    # Lifecycle: start/shutdown, health, metrics, proxy upstream glue
├── auth/                   # Admin + proxy + downstream auth, policy, rate limit
├── config/                 # Env loading, defaults, validation (naming rule: BACKEND.md §1.6)
├── router/                 # chi router mount, middleware, security headers, SPA fallback
├── handler/
│   ├── admin/              # Admin REST handlers (+ payloads/)
│   ├── proxy/              # /v1/* proxy surface handlers + the non-/v1 aliases
│   └── shared/             # Shared API error helpers
├── proxy/                  # Proxy orchestration (NOT "proxycore/")
│   ├── profiles/           # Client/profile detection (Claude Code, Codex, Gemini CLI, …)
│   └── types/              # Shared proxy types
├── routing/                # TokenRouter: match, weights, cooldown, site runtime breaker
├── platform/               # Upstream platform adapters (16) + site proxy
├── transform/              # Protocol transformers (NOT "protocol/"; no canonical IR)
│   ├── anthropic/          # messages: the Messages ⇄ Chat request/return bridge
│   ├── openai/             # completions, embeddings, images, responses
│   ├── gemini/             # generate_content (native OpenAI→Gemini bridge)
│   ├── relaykitbridge/     # Request-scoped New API RelayKit integration
│   └── shared/             # Cross-protocol helpers
├── service/                # Domain services (sites, accounts, checkin, balance, notify, oauth, backup, …)
│   ├── pricing/            # Canonical pricing normalization (ratios → $/M)
│   └── pricingcatalog/     # models.dev official catalog pricing (cold-start cost signal)
├── scheduler/              # Background cron jobs (checkin, balance, recovery, retention, …)
├── store/                  # sqlx DB open, dual dialect, schema, settings
├── internal/               # Repo-private helpers, out of the boundary test's scope:
│                           #   ssrf · sharedcount · httpclient · version · golden · pgtest
├── web/
│   ├── embed.go            # //go:embed dist
│   └── dist/               # Built React SPA (generated; embedded into binary)
├── e2e/                    # End-to-end tests
├── scripts/                # Env-driven release/verify/e2e scripts (no host or credential baked in)
├── testbed/                # Sanitized compose template for a local upstream testbed
├── licenses/               # Preserved third-party and prior license notices
├── docs/                   # Specs, architecture, design philosophy
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

### Package roles (one-liners)

| Package         | Responsibility                                                                                                           |
| --------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `cmd/server`    | Wire config → store → app → router; process entry                                                                        |
| `cmd/migrate`   | Offline SQLite → PostgreSQL data move                                                                                    |
| `cmd/channel-plugin-maintain` | 精确选择/检查/执行离线程序档案与遗留捕获维护；停机确认由操作者负责 |
| `app`           | HTTP server lifecycle, readiness, metrics, upstream executor glue                                                        |
| `router`        | Route tree, CORS/security middleware, embed SPA                                                                          |
| `auth`          | Fail-closed admin/proxy auth, downstream key policy                                                                      |
| `config`        | 环境变量 → `Config`；保留既有名称、alias、优先级和默认值，见 `configuration.md` |
| `handler/admin` | Admin CRUD + settings + ops endpoints                                                                                    |
| `handler/proxy` | Protocol surfaces under `/v1/*` and the non-`/v1` aliases                                                                                          |
| `proxy`         | Bounded HTTP/SSE executor + channel selection + retry policy (the request loop itself lives in `handler/proxy/upstream.go`) |
| `routing`       | Model/route match, weighted selection, Fibonacci cooldown, site breaker                                                  |
| `platform`      | Per-upstream adapter behavior (detect, auth headers, admin APIs)                                                         |
| `transform`     | Request/response (+ SSE) conversion, native per protocol (no canonical intermediate)                                     |
| `service`       | Business workflows used by handlers and schedulers                                                                       |
| `scheduler`     | Cron-driven background work                                                                                              |
| `store`         | Dual-dialect persistence                                                                                                 |
| `web`           | Embedded frontend assets                                                                                                 |

## Data Flow

### Proxy request flow

```
Client → /v1/chat/completions (or messages / generateContent / …)
  → router middleware (RequestID, security headers, body limits where applied)
  → auth.ProxyAuth (managed downstream key; fail-closed)
  → handler/proxy surface
  → routing.TokenRouter.Match(requestedModel) + policy filters
  → routing selector (weights / round-robin / stable-first)
      · skip cooldown channels
      · skip open site/model runtime breakers
      · cost signal: observed avg → configured unit_cost → catalog (models.dev,
        official sites "catalog", relay sites "catalog_estimate") → fallback
  → handler/proxy upstream loop (per-attempt retry in `upstream.go`)
      → profile detect (proxy/profiles)
      → endpoint dispatch + retry policy (proxy.retry_policy)
          → transform (client format ↔ provider format, native per protocol)
          → HTTP to upstream (platform-aware headers / site proxy)
          → transform response / SSE
      → failure judge (content + policy)
      → surface format to client
  → response
```

### Admin request flow

```
Browser SPA (embedded) → /api/*
  → auth.AdminAuth (optional IP allowlist + Bearer AUTH_TOKEN; fail-closed)
  → handler/admin → service/* → store
```

### Background work

```
scheduler (robfig/cron)
  → service (checkin / balance / notify / backup / oauth / …)
  → store + platform adapters
  → routing health / channel recovery feedback
```

## TS vs Go Comparison

| Aspect               | TypeScript                    | Go                                     |
| -------------------- | ----------------------------- | -------------------------------------- |
| Runtime              | Node.js                       | Single static binary (`CGO_ENABLED=0`) |
| Frontend             | Static files from Node server | `web` + `//go:embed dist`              |
| Database             | Drizzle (SQLite/MySQL/PG)     | sqlx (SQLite + PostgreSQL only)        |
| MySQL support        | Yes                           | Dropped                                |
| Proxy engine package | `proxy-core` (TS)             | `proxy/`                               |
| Transformers package | `transformers` (TS)           | `transform/`                           |
| Routing              | `tokenRouter` service         | `routing/`                             |
| Background jobs      | timers + node-cron            | `scheduler/` + robfig/cron             |
| Image size           | ~80MB+ node base              | Alpine + static binary                 |
| Config env names     | Unprefixed                    | 保留既有名称与 alias；优先级见 `config/` 和 `configuration.md` |

## Key Design Decisions

### 1. Embedded frontend

React SPA is built once and embedded via `//go:embed`. Production image has no Node runtime and no separate static volume for the UI.

### 2. Dual dialect: SQLite + PostgreSQL

SQLite is default (zero-config dev/test). PostgreSQL is the production path. `store.Open(dialect, dsn, sslMode)` and dialect rebinding hide `?` vs `$N` and type differences. MySQL was intentionally not ported.

### 3. 管理 API 与原生代理协议

管理 API 保持与 TS 前端兼容的 camelCase 字段（`externalCheckinUrl`、`useSystemProxy` 等）。
代理 API 保留各原生协议的字段、错误和流式语义，包括协议定义的 snake_case 字段；
不能为统一代码风格重命名 wire 字段。

### 4. Fail-closed auth

Admin and proxy middleware deny by default on missing/invalid credentials. Managed downstream keys use deny-all-when-empty model allowlists. Public exceptions are explicit allowlists only (e.g. desktop health, OAuth callback).

### 5. Channel isolation, cooldown, circuit breaker

Routing isolates bad channels instead of cascading:

- **Per-channel cooldown** — Fibonacci backoff on failures (`routing` cooldown helpers).
- **Site runtime breaker** — streak-based open periods at site and model granularity (`SiteRuntimeBreakerLevelsMs`).
- **Selection filters** — open breakers and recent failures are removed before weighted/round-robin pick.
- **Proxy failover** — the request loop can retry the same channel, refresh auth, or failover to the next channel without taking the whole gateway down.

### 6. 配置名称与优先级

`config.Load` 保留已有环境变量名称及 alias。名称、优先级、默认值和校验由 `config/`
实现，[配置文档](configuration.md) 与 `.env.example` 同步说明。
例如 `DB_PROFILE`、`DB_APPLICATION_NAME` 优先于对应 `METAPI_` alias；
不通过一条新命名前缀规则改写现有用户配置。

### 7. Pure Go, no CGO

`modernc.org/sqlite` keeps the binary fully static and portable.

## Package dependency overview

Allowed direction (forbidden edges are the denylist in
[Ownership and boundary map](#ownership-and-boundary-map), enforced by
[`docs/package_boundary_test.go`](package_boundary_test.go)):

```
cmd → app, router, config, store, …
router → handler/*, auth, web, app, config, store
handler → service, proxy, routing, platform, auth, store, config
proxy → auth, routing, service, store, config, proxy/profiles, proxy/types
routing → store, config
service → platform, store, config, service/*
scheduler → service/*, store, config
platform → (leaf; no store/handler imports)
transform/* → transform/shared (leaf protocol layer)
store → config
config, web, handler/shared → leaves
```

**Where new code belongs:** decide from the ownership table above, then prove
it by running `go test ./docs -run TestPackageBoundaries`. That test is the
specification; a new edge either passes it or needs a written exception in its
header comment.

## 渠道类型与多协议适配

Direct 渠道由多个独立配置维度组成，不将供应商名称当作完整能力，也不将添加设置视作扩大权限。

### 渠道配置的职责与执行

| 配置维度 | 实现归属 | 不能隐含改变的边界 |
| --- | --- | --- |
| 供应商预设与协议端点 | `service/upstream`、`store.DirectEndpoints`、`proxy/profiles`、`internal/upstreamadapter` | 预设只是初始配置；原生 HTTP、compact、WebSocket 分别配置与授权 |
| 凭据与身份生命周期 | `service/oauth`、`store` | API Key/OAuth/GCP 由对应 resolver 与持久 refresh family 处理，不由请求 hook 接管 |
| 模型与访问授权 | `upstream_models`、`upstream_grants`、`upstream_group_items` | 模型发现、新增端点或插件安装不自动授予访问 |
| 选路与流式策略 | `routing`、`handler/proxy`、`transform/streamaggregate`、`transform/relaykitbridge` | 硬授权先于偏好；流式聚合与协议桥接不扩大授权或用户预算 |
| 渠道流量与队列 | `internal/channellimit`、`handler/proxy` | 本进程并发/RPM 硬限制和实测 TPM 偏好分开；不宣称集群配额 |
| 请求自定义与插件/驱动 | `internal/requestoverride`、`internal/channelplugin`、`service/channelplugins` | 变换/解码执行当前渠道尝试，不选择路由、不获得凭据或额外网络能力 |

### External backup import ownership

外部备份导入由 `service/backup` 统一承载（TS v2.1、Octopus v5、AxonHub v1.4、Magpie）：Octopus v5、AxonHub v1.4 与 Magpie 的预览与提交共享同一份校验/编译计划，不支持的可执行项显式阻塞或具名跳过，并复用 `external_source_ids` 生命周期管理重复导入与来源替换；TS v2.1 的预览与提交共享 `ParseTSV21` 解析校验，预览按主键估算待插入量，提交按表主键跳过重复行并过滤不允许导入的站点行。

上述子系统的细粒度契约、锁顺序、视频任务归档、请求内图片任务与外部导入编译规则已下沉到开发者子系统参考文档（见 [`docs/README.md`](README.md) 索引）；对外接口契约见 [渠道插件 API](api/channel-plugins.md) 与 [渠道管理 API](api/routes.md#patch-apiimported-upstreamsid)。

## Related docs

- [`docs/package_boundary_test.go`](package_boundary_test.go) — the executable dependency denylist, scoped legacy exemptions, and stale-exemption check; read this before moving an import
- [`docs/coupling_ratchet_test.go`](coupling_ratchet_test.go) — monotonic coupling ratchets (`handler/admin/...` direct `sqlx` import baseline and per-file `store.GetDB` occurrence ceilings)
- [`docs/api.md`](api.md) — admin API reference index; [`docs/api/routes-inventory.md`](api/routes-inventory.md) is the complete registered `/api` route list, checked against the code by [`docs/api_inventory_parity_test.go`](api_inventory_parity_test.go)
- [`docs/api/conventions.md`](api/conventions.md) — auth surfaces, error envelope, pagination: read before calling any `/api` route
- [`docs/api/proxy.md`](api/proxy.md) — the `/v1` data-plane contract
- [`docs/configuration.md`](configuration.md) — every environment variable with its real default and clamp, checked against `.env.example` and `config/config.go` by [`docs/env_parity_test.go`](env_parity_test.go)
- [`docs/deployment.md`](deployment.md) — deploy guide
- [`docs/migration.md`](migration.md) — TS → Go migration notes
