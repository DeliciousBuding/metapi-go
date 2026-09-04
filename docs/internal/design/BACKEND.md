# Backend Design Philosophy

**Last updated**: 2026-09-04

**Status**: contributor principles — how backend code should be written
**Not the authority on import edges**: the boundary contract is
[`docs/architecture.md`](../../architecture.md) §"Ownership and boundary map", and its
executable form is [`docs/package_boundary_test.go`](../../package_boundary_test.go). When this
file and those two disagree, those two are right and this file is stale.

This document states the **non-negotiable backend principles** for metapi-go. Implementation work under backend architecture (package boundaries, concurrency, unified errors) must not violate these rules without an explicit design revision.

---

## 1. Principles

### 1.1 Single binary + `go:embed` SPA

- Ship **one** server binary from `cmd/server`.
- Frontend is pre-built into `web/dist` and embedded by `web` (`//go:embed dist`).
- Production images must not require Node/npm at runtime.
- SPA fallback and `/api` + `/v1` share the same process and port.

### 1.2 Dual dialect: SQLite + PostgreSQL

- Persistence goes through `store` only.
- Supported dialects: **SQLite** (dev/test/default) and **PostgreSQL** (production).
- Open via `store.Open(dialect, dsn)` (or equivalent store entrypoints); never hard-code SQLite-only SQL in callers.
- Placeholders and type differences are handled inside `store` (rebind / dialect helpers).
- MySQL is **out of scope** (dropped from TS).

### 1.3 camelCase JSON API parity with TS frontend

- Wire JSON uses **camelCase** field names identical to the original Metapi frontend expectations.
- Struct tags on API models and payloads follow `json:"fooBar"`, not snake_case, for public admin/proxy contracts.
- Breaking renames require a versioned migration plan; silent format drift is a defect.

### 1.4 Fail-closed auth

- Unauthenticated or invalid credentials **deny** (401/403). There is no “open if misconfigured” path for protected routes.
- **Admin** (`auth.AdminAuth`): IP allowlist (when configured) + Bearer `AUTH_TOKEN`; public API routes are an explicit allowlist only.
- **Proxy** (`auth.ProxyAuth`): global `PROXY_TOKEN` or managed downstream keys; unknown/missing tokens fail closed.
- **Managed downstream keys**: empty model/route allowlists mean **deny all** (`DenyAllWhenEmpty=true`). Global proxy token remains the broad default only when intentionally used.
- Constant-time compare for secrets; do not log full tokens.

### 1.5 Channel isolation / cooldown / circuit breaker

Gateway resilience is **per channel / per site**, not process-wide suicide:

| Mechanism                  | Package                                     | Intent                                                               |
| -------------------------- | ------------------------------------------- | -------------------------------------------------------------------- |
| Fibonacci failure cooldown | `routing`                                   | Back off a bad channel without blackholing the fleet                 |
| Recent-failure filters     | `routing` selector                          | Prefer healthy candidates in the current pick                        |
| Site/model runtime breaker | `routing` runtime health                    | Open breaker after streak; tiered open windows                       |
| Channel failover           | `handler/proxy` loop + `proxy` retry policy | Retry same channel, refresh auth, or move to next channel            |
| Content failure judge      | `proxy`                                     | Detect “HTTP 200 but empty/error body” without trusting status alone |

Rules of thumb:

- A single upstream outage must not disable unrelated sites/channels.
- Breaker open ⇒ filter out of selection, do not crash the process.
- Recovery jobs (`scheduler` channel recovery, etc.) may heal state; they must not bypass auth or dialect boundaries.

### 1.6 Config via env (no prefix), matching TS names

- Configuration is environment-driven (`config.Load` / `config.Get`).
- Env var names match TS Metapi **without** a `METAPI_` (or similar) prefix: e.g. `AUTH_TOKEN`, `PROXY_TOKEN`, `DB_TYPE`, `DB_URL`, `PORT`.
- Defaults live in `config`; production unsafety (default tokens, weak secrets) is warned/validated, not silently accepted as secure.
- Runtime settings that belong in DB stay in `store` settings; do not invent a second config language.

### 1.7 Feature parity with controlled evolution

- Behavioral parity with original Metapi remains the default for existing APIs.
- Enterprise upgrades (backend architecture and later) may clarify structure and fix CRITICAL defects without changing wire contracts unless the issue explicitly allows it.

### 1.8 Simplicity before abstraction

- Start with the owning package and the shortest explicit data flow. Add an interface, facade, registry, or shared helper only for an existing architectural boundary or multiple real consumers.
- Do not retain scaffolds for endpoints, refresh flows, protocols, or deployment modes that do not exist. Unsupported behavior should fail explicitly or remain a documented residual.
- Validate untrusted input at HTTP, config, storage, and upstream boundaries. Inside a validated flow, rely on typed invariants instead of repeating normalization and fallback logic at every layer.
- A replacement removes the superseded path in the same change. Parallel implementations require a time-bounded migration plan and an explicit owner.
- Tests protect observable contracts and high-risk boundaries; they should not freeze speculative implementation structure.

---

## 2. Package dependency rules

### 2.1 Layer diagram (allowed direction)

```
                    ┌──────────── cmd/server, cmd/migrate ────────────┐
                    │                     │                            │
                    ▼                     ▼                            │
                 config ◄──────────── store                            │
                    ▲                     ▲                            │
                    │                     │                            │
        ┌───────────┴───────────┬─────────┴──────────┬─────────────────┤
        │                       │                    │                 │
        ▼                       ▼                    ▼                 │
      auth                   platform             transform/*          │
        ▲                       ▲                    ▲                 │
        │                       │                    │                 │
        │                    service ───────────────┘ (prefer not;     │
        │                       ▲                     transform is     │
        │                       │                     leaf for now)    │
        │                  scheduler                                   │
        │                       ▲                                      │
        │                       │                                      │
      router ──► handler/* ──► proxy ──► routing ──────────────────────┘
        │            │           │
        │            │           └── proxy/profiles, proxy/types
        │            └── handler/shared
        └── web (embed only)
                 app (lifecycle + health/metrics glue)
```

Arrows mean **“may import”**. Edges not shown are forbidden unless listed under exceptions.

### 2.2 The forbidden-import rules have one owner, and it is not this file

The diagram above is the *intent* — which direction is down. The contract is
[`docs/architecture.md`](../../architecture.md) §"Ownership and boundary map": one row per
package, the decision it owns, and what it must not import. Its executable form is
[`docs/package_boundary_test.go`](../../package_boundary_test.go) — rules 1–8, every approved
exception with the architecture section that justifies it, and an assertion that all twelve
domains were really scanned, because a boundary gate that scans nothing and reports no
violations is not a lenient gate but an absent one. That is not a hypothetical: it is the shape
that let roughly thirty release tags stay green while their required shards ran zero tests.

This section used to carry a dependency table and the next one a list of hard rules. Both were
copies and both had drifted, in opposite directions: the table forbade `routing → router` and
`auth → handler/proxy/router`, which nothing enforces, while omitting `store ↛ routing` and
`transform ↛ auth`, which are enforced; the rule list omitted `service ↛ proxy` and
`scheduler ↛ handler/router/proxy` altogether. Two copies of one rule in a single file — three
counting the gate — is how a contributor gets a stale answer to "may I import this?". Read the
gate; its failure message names the rule number.

Two items from the old list are not the gate's business and stay here as principles:

- **No import cycles.** The compiler already refuses them; if one appears, extract a small
  types package rather than merging two layers.
- **Prefer a service facade over `handler → scheduler`** for anything on a request path. The one
  approved exception is admin-ops cron validation, and it is listed in the gate's header comment.

### 2.3 Composition root

Only `cmd/server` (and tests/e2e helpers) should construct the full graph: load config → open store → build services/router/schedulers → `app.Start`. Libraries under packages should accept dependencies via constructors/parameters, not hidden global grabs—except the existing `config.Get()` singleton pattern used for parity.

**As-built inventory:** package ownership, public entrypoints, and documented exception edges (e.g. admin checkin schedule → `app`/`scheduler`, `app.ConfigureProxyUpstream` → `handler/proxy`) are recorded in the as-built package map [`docs/architecture.md`](../../architecture.md). New exceptions must be listed there and in the gate's header comment, with a reason, rather than introduced silently.

---

## 3. Cross-cutting behavioral contracts

### 3.1 Errors

- Prefer typed/sentinel errors at package boundaries; map to HTTP in `handler` / `handler/shared`.
- Do not return HTTP 200 for auth or validation failure.
- Proxy surfaces should preserve upstream-compatible error shapes where TS parity requires it.

### 3.2 Concurrency

- Channel/session state mutations must be safe under concurrent proxy traffic.
- Cooldown and breaker updates must not race into “always open” or “never open” corruption.
- CRITICAL concurrency fixes under backend architecture stay inside owning packages (`routing`, `proxy`, …) without smuggling locks into `store` SQL.

### 3.3 Observability

- Use `log/slog` with request IDs from router middleware.
- Metrics/health live under `app` (`/health`, `/ready`, `/metrics` patterns already established).
- Do not log secrets, full `Authorization` headers, or raw account credentials.

### 3.4 Testing

- Dual dialect: store tests cover SQLite and PG where SQL diverges.
- Package tests stay inside the package; e2e covers full binary behavior.
- Pre-commit verification: `go vet ./...` and `go test ./... -count=1 -race` (see `AGENTS.md`).

---

## 4. Naming glossary (TS → Go)

| TS / informal name      | Actual Go package       |
| ----------------------- | ----------------------- |
| proxy-core / ProxyCore  | `proxy`                 |
| transformers / protocol | `transform` (+ subdirs) |
| tokenRouter             | `routing`               |
| platform adapters       | `platform`              |
| admin routes            | `handler/admin`         |
| proxy routes            | `handler/proxy`         |
| embed web               | `web`                   |

---

## 5. Change routing

- Open outcomes and priorities are tracked as GitHub issues.
- Package ownership and approved exception edges live in [`docs/architecture.md`](../../architecture.md).
- When principles change, revise this file. When only layout or request-flow facts change, update [`docs/architecture.md`](../../architecture.md).

---

## 6. Checklist for new backend code

- [ ] Belongs in an existing package (no drive-by top-level package)
- [ ] Uses a direct implementation; any new abstraction has a real boundary or multiple consumers
- [ ] Replaces old paths instead of adding speculative parallel scaffolding
- [ ] Imports only allowed edges (§2)
- [ ] JSON tags camelCase for public API
- [ ] Auth path fail-closed
- [ ] Channel failure isolated (cooldown/breaker/failover) when touching routing/proxy
- [ ] Dialect-safe SQL via `store`
- [ ] Env names match TS, no new prefix scheme
- [ ] Tests with `-race` for concurrent paths
