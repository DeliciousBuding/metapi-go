# Architecture Overview

**Last updated**: 2026-09-03

> **Navigation**: full docs map in [`docs/README.md`](README.md) · route list in [`api/routes-inventory.md`](api/routes-inventory.md) · environment variables in [`configuration.md`](configuration.md).
>
> Every structural claim below names the package or exported symbol that owns
> it, so it can be checked with a search instead of trusted. The dependency
> rules in [Ownership and boundary map](#ownership-and-boundary-map) are
> enforced by `docs/package_boundary_test.go`; the route claims are enforced
> by `docs/api_inventory_parity_test.go`; the environment-variable claims are
> enforced by `docs/env_parity_test.go`.

Metapi Go is a ground-up rewrite of the TypeScript Metapi proxy gateway in Go. This document describes the **as-built** package layout, request paths, and key design decisions. Package dependency rules are stated in full below and machine-enforced by [`docs/package_boundary_test.go`](package_boundary_test.go).

> **Naming truth:** There is **no** `proxycore/` or `protocol/` package in this repository. The proxy engine is `proxy/` (with `proxy/profiles` and `proxy/types`). Protocol conversion is `transform/` (with `openai` [completions/embeddings/images/responses], `gemini`, and `shared`). There is **no** `transform/canonical` intermediate layer — cross-protocol conversion is native (e.g. OpenAI→Gemini) and bypasses any canonical representation. Older docs or TS-era names that say “ProxyCore package” or “protocol package” refer to these real packages.

## High-Level Architecture

```
                  ┌─────────────────┐
                  │   nginx/Caddy   │  (TLS termination)
                  └────────┬────────┘
                           │ :4000
                  ┌────────▼────────┐
                  │  router (chi)   │
                  │  /api/*  /v1/*  │
                  │  SPA + health   │
                  └───┬────────┬────┘
                      │        │
         ┌────────────▼──┐  ┌──▼──────────────┐
         │  Admin API     │  │  Proxy Handlers  │
         │  handler/admin │  │  handler/proxy   │
         │  auth.Admin    │  │  auth.Proxy      │
         └───────┬────────┘  └──┬───────────────┘
                 │              │
         ┌───────▼──────────────▼───────┐
         │           proxy/              │
         │  Coordinator · Executor       │
         │  ChannelSelection · Retry     │
         │  Profiles · Failure policy    │
         └──────────────┬───────────────┘
                        │
         ┌──────────────▼───────────────┐
         │         routing/              │
         │  TokenRouter · Selector       │
         │  Cooldown · Runtime breaker   │
         └──────────────┬───────────────┘
                        │
         ┌──────────────▼───────────────┐
         │        transform/             │
         │  OpenAI / Gemini (native)     │
         │  shared stream helpers        │
         └──────────────┬───────────────┘
                        │
         ┌──────────────▼───────────────┐
         │   platform/  +  service/      │
         │  16 adapters · domain logic   │
         └──────────────┬───────────────┘
                        │
         ┌──────────────▼───────────────┐
         │           store/              │
         │    SQLite (dev) / PG (prod)   │
         └──────────────────────────────┘
```

## Ownership and boundary map

The layering is a **denylist of import edges**, not a file inventory: each
package owns one decision, and the test named below fails the build if a
package reaches outside its decision. `docs/package_boundary_test.go` is the
authoritative, executable form of this table — when the two disagree, the test
is right and this page is stale.

| Package | Decision it owns | Must NOT import |
|:--------|:-----------------|:----------------|
| `config` | Env parsing, defaults, clamping, validation. Nothing else reads `os.Getenv` for a documented knob. | (leaf) |
| `store` | Persistence and dialect differences (SQLite / PostgreSQL), placeholder rebinding. | `handler`, `proxy`, `routing`, `service`, `scheduler`, `router`, `auth` |
| `platform` | One adapter per upstream provider family; the leaf that actually talks to upstreams. | `store`, `handler`, `proxy`, `router`, `scheduler` (`config` and `proxy/profiles` are allowed) |
| `transform` | Wire-format conversion between protocols (native OpenAI ⇄ Gemini, no intermediate canonical form). | `handler`, `store`, `proxy`, `routing`, `service`, `auth` |
| `routing` | Which channel/account serves a request: selection, cooldown, runtime breaker. | `proxy`, `handler` |
| `service` | Domain logic that spans stores and platforms. | `handler`, `router`, `proxy` |
| `scheduler` | When background work runs (cron, interval, window). | `handler`, `router`, `proxy` |
| `handler/*` | HTTP surface only: decode, authorize, call a service/proxy entrypoint, encode. | `router` (the router mounts handlers, never the reverse) |
| `router` | Mount points, middleware order, SPA fallback, static asset serving. | — |
| `cmd/server` | Composition root; may import anything. | — |

Approved exceptions are enumerated in the header comment of
`docs/package_boundary_test.go` (for example `handler/admin → scheduler` for
cron validation, `app → handler/proxy` for the upstream composition helper,
`platform → proxy/profiles` for client detection, `handler/proxy →
transform/*` for one-way protocol wiring). The rule for adding one is: move
the import to an allowed edge first; only if that is impossible, add the
exception to the test with a written justification.

`proxycore/` and `protocol/` do not exist and must not be reintroduced — the
test rejects those TypeScript-era names.

## Request paths: stages and single owners

Two independent auth surfaces share one chi router built by `router.New`, the
only composition root. Registrars take a `chi.Router` and register absolute
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
`if db := store.GetDB(); db != nil`. When the database is unavailable those
routes are simply not registered and the router logs
`router: database not initialized, P3 routes skipped`; the session manager is
`nil` and the session handlers fail closed rather than allowing traffic.
`/api/about` and the session lifecycle routes are mounted outside that guard
because every field they serve comes from the linker, the Go runtime, or a
fail-closed handler.

### Data plane (`/v1/*`)

`r.Route("/v1", ...)` owns the proxy surface, in this order:

1. `ProxyWriteDeadline`
2. `CORS()`
3. `auth.ProxyRateLimit(cfg.ProxyRateLimitRPM)` — per-IP, **before** auth so an
   unauthenticated flood is dropped without a database lookup
4. `auth.ProxyAuth()` — resolves the caller (global `PROXY_TOKEN` or a managed
   downstream key) and puts the result in the request context
5. `auth.ProxyGlobalTokenRateLimit(cfg.ProxyGlobalTokenRPM)` — **after** auth,
   because the global cap only applies once the resolved auth source is known
   to be the shared token; managed keys have their own admission
6. `proxyhandler.RegisterProxyRoutes(r)` — the surface itself
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

Streaming and non-streaming responses take different paths through the attempt
loop. A non-streaming body is buffered up to `PROXY_MAX_BUFFERED_RESPONSE_BYTES`
and judged once; a stream is relayed under a chunk-gap guard
(`PROXY_STREAM_IDLE_TIMEOUT_SEC`) and a total byte bound
(`PROXY_MAX_STREAM_RESPONSE_BYTES`), then classified by how the relay ended:
clean end of stream, idle timeout, upstream fault mid-stream, byte-limit
truncation, or downstream disconnect.

Two rules keep that classification honest:

- **One judge for content.** Whether an upstream answer counts as a failure on
  its content — a configured `PROXY_ERROR_KEYWORDS` hit, or an empty completion
  when `PROXY_EMPTY_CONTENT_FAIL` is on — is decided by a single pure function
  in `proxy`, fed by both paths: the buffered body directly, the stream through
  the bounded SSE analyser. The two paths cannot reach different verdicts for
  the same upstream content.
- **A downstream disconnect is not an upstream fault.** The channel answered
  correctly, so recording the cancel against it would poison channel health
  with user behaviour. Usage already extracted from earlier SSE events is still
  accounted; tokens are never invented for content that did not arrive.

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

`router.setupSPAFallback` serves the embedded `web/dist` tree: content-hashed
subtrees under `/assets/` and `/static/` get an immutable cache header, while
root files whose names are not hashed (`bootstrap.js`, `theme-init.js`, logos
and favicons) get `no-cache` so deploys propagate without a hard refresh. The
fallback answers `index.html` for non-API paths and a JSON 404 body for
`/api/*` and `/v1/*`.

## Package Layout (as-built)

```
metapi-go/
├── cmd/
│   ├── server/             # Main server entry point
│   └── migrate/            # Standalone SQLite→PG migration tool
├── app/                    # Lifecycle: start/shutdown, health, metrics, proxy upstream glue
├── auth/                   # Admin + proxy + downstream auth, policy, rate limit
├── config/                 # Env loading (no prefix), defaults, validation
├── router/                 # chi router mount, middleware, security headers, SPA fallback
├── handler/
│   ├── admin/              # Admin REST handlers (+ payloads/)
│   ├── proxy/              # /v1/* proxy surface handlers
│   └── shared/             # Shared API error helpers
├── proxy/                  # Proxy orchestration (NOT "proxycore/")
│   ├── profiles/           # Client/profile detection (Claude Code, Codex, Gemini CLI, …)
│   └── types/              # Shared proxy types
├── routing/                # TokenRouter: match, weights, cooldown, site runtime breaker
├── platform/               # Upstream platform adapters (16) + site proxy
├── transform/              # Protocol transformers (NOT "protocol/"; no canonical IR)
│   ├── openai/             # completions, embeddings, images, responses
│   ├── gemini/             # generate_content (native OpenAI→Gemini bridge)
│   └── shared/             # Cross-protocol helpers
├── service/                # Domain services (sites, accounts, checkin, balance, notify, oauth, backup, …)
│   ├── pricing/            # Canonical pricing normalization (ratios → $/M)
│   └── pricingcatalog/     # models.dev official catalog pricing (cold-start cost signal)
├── scheduler/              # Background cron jobs (checkin, balance, recovery, retention, …)
├── store/                  # sqlx DB open, dual dialect, schema, settings
├── web/
│   ├── embed.go            # //go:embed dist
│   └── dist/               # Built React SPA (generated; embedded into binary)
├── e2e/                    # End-to-end tests
├── docs/                   # Specs, architecture, design philosophy
├── Dockerfile
├── docker-compose.yml
├── docker-compose.prod.yml
└── Makefile
```

### Package roles (one-liners)

| Package         | Responsibility                                                                                                           |
| --------------- | ------------------------------------------------------------------------------------------------------------------------ |
| `cmd/server`    | Wire config → store → app → router; process entry                                                                        |
| `cmd/migrate`   | Offline SQLite → PostgreSQL data move                                                                                    |
| `app`           | HTTP server lifecycle, readiness, metrics, upstream executor glue                                                        |
| `router`        | Route tree, CORS/security middleware, embed SPA                                                                          |
| `auth`          | Fail-closed admin/proxy auth, downstream key policy                                                                      |
| `config`        | Env → `Config` (names match TS; no `METAPI_` prefix)                                                                     |
| `handler/admin` | Admin CRUD + settings + ops endpoints                                                                                    |
| `handler/proxy` | Protocol surfaces under `/v1/*`                                                                                          |
| `proxy`         | Coordinator + executor + channel selection + retry policy (the request loop itself lives in `handler/proxy/upstream.go`) |
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
  → auth.ProxyAuth (PROXY_TOKEN or managed downstream key; fail-closed)
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
| Config env names     | Unprefixed                    | Same unprefixed names                  |

## Key Design Decisions

### 1. Embedded frontend

React SPA is built once and embedded via `//go:embed`. Production image has no Node runtime and no separate static volume for the UI.

### 2. Dual dialect: SQLite + PostgreSQL

SQLite is default (zero-config dev/test). PostgreSQL is the production path. `store.Open(dialect, dsn)` and dialect rebinding hide `?` vs `$N` and type differences. MySQL was intentionally not ported.

### 3. camelCase JSON API parity

All admin/proxy JSON field names use camelCase tags matching the TS frontend (`externalCheckinUrl`, `useSystemProxy`, …). Do not introduce snake_case wire formats for existing APIs.

### 4. Fail-closed auth

Admin and proxy middleware deny by default on missing/invalid credentials. Managed downstream keys use deny-all-when-empty model allowlists. Public exceptions are explicit allowlists only (e.g. desktop health, OAuth callback).

### 5. Channel isolation, cooldown, circuit breaker

Routing isolates bad channels instead of cascading:

- **Per-channel cooldown** — Fibonacci backoff on failures (`routing` cooldown helpers).
- **Site runtime breaker** — streak-based open periods at site and model granularity (`SiteRuntimeBreakerLevelsMs`).
- **Selection filters** — open breakers and recent failures are removed before weighted/round-robin pick.
- **Proxy failover** — the request loop can retry the same channel, refresh auth, or failover to the next channel without taking the whole gateway down.

### 6. Config via env, TS-compatible names

`config.Load` reads the same env var names as TS Metapi (`AUTH_TOKEN`, `PROXY_TOKEN`, `DB_TYPE`, …) with **no** project prefix. Defaults and validation live in `config/`.

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
proxy → routing, service, store, config, proxy/profiles, proxy/types
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

## S.U.P.E.R. Compliance

- **S** (small): packages own one layer (HTTP, orchestration, routing, persistence, …)
- **U** (understandable): real names match code (`proxy`, `transform`, `routing`)
- **P** (pluggable): platform adapters and protocol transforms register/compose independently
- **E** (environment-agnostic): SQLite or PostgreSQL via dialect store
- **R** (replaceable): coordinator dependencies and platform adapters are injectable/replaceable

## Related docs

- [`docs/package_boundary_test.go`](package_boundary_test.go) — the executable dependency denylist and its approved exceptions; read this before moving an import
- [`docs/api.md`](api.md) — admin API reference index; [`docs/api/routes-inventory.md`](api/routes-inventory.md) is the complete registered `/api` route list, checked against the code by [`docs/api_inventory_parity_test.go`](api_inventory_parity_test.go)
- [`docs/api/conventions.md`](api/conventions.md) — auth surfaces, error envelope, pagination: read before calling any `/api` route
- [`docs/api/proxy.md`](api/proxy.md) — the `/v1` data-plane contract
- [`docs/configuration.md`](configuration.md) — every environment variable with its real default and clamp, checked against `.env.example` and `config/config.go` by [`docs/env_parity_test.go`](env_parity_test.go)
- [`docs/deployment.md`](deployment.md) — deploy guide
- [`docs/migration.md`](migration.md) — TS → Go migration notes
