# STATE.md — MetAPI Go product status

**Last verified**: 2026-08-11

> **Current state** (product repository). Only current facts and pointers, no narrative.
> Deployment facts live in the deployment guide; open items → [`progress/MASTER.md`](progress/MASTER.md) · timeline → [`log.md`](log.md) · version narrative → root [`CHANGELOG.md`](../CHANGELOG.md)

## Current

| Fact | Value |
|:-----|:------|
| Source | **[DeliciousBuding/metapi-go](https://github.com/DeliciousBuding/metapi-go)** · default branch `master` |
| Latest release | **[v0.9.0](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.9.0)** (2026-08-11); master CD publishes `ghcr.io/deliciousbuding/metapi-go` |

| Production pin (ops) | **v0.9.0** image on `ghcr.io/deliciousbuding/metapi-go`; runtime deployment facts live in the deployment docs |
| Current focus | production hardening |
| Open issues / PRs | [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) · [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) |
| Stack | Go 1.26.5 · React 19 + Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH + i18next · dual dialect SQLite/PostgreSQL |
| Runtime shape | single embedded SPA binary · **16** background schedulers · OAuth callback listeners start only with an active flow |

## Product honesty

| ID | Status | Note |
|:---|:-------|:-----|
| cascade | **partial** | HTTP load proof exists; production multi-channel proof still required |
| usage stats | **present-with-residual** | media detail fold shipped; multi-instance lag remains |
| Responses WebSocket | **present** | single-instance honesty; no cluster-wide sticky claim |
| Redis sticky sessions | **deferred** | use a single instance or load-balancer pinning |
| Update-center deploy | **external present** | releases/GHCR are external; API residual remains 501 |
| PostgreSQL pool budget | **present** | profiles + lease backoff; deployment must still respect role LIMIT |
| RE2 user-id extraction | **fixed and deployed** | v0.8.45 shipped; the 0.8.44 crash state is historical |
| OAuth token refresh | **present** | shared explicit account/site projection supports SQLite and PostgreSQL |
| UI i18n verification | **present** | EN and zh cover 18 routes with seeded real data-state assertions; static gate rejects unwrapped han text nodes |
| EN label coverage | **swept 2026-08-02** | 414 JSX text nodes wrapped in tr(); 22 keys added; live status badge colors tokenized |
| Theme accent presets | **chart-synced 2026-08-02** | blue/indigo/teal recolor UI chrome AND chart series (chart-1 follows primary; FOUC-safe) |
| Daily metric truth | **Dashboard + Accounts closed in master** | shared local-day aggregation, real reward/check-in, partial truth metadata, query errors fail closed; per-account today reward/spend with status gate on Accounts rows (no fake zeros) |
| Windows local development | **loopback by default** | empty `HOST` binds `127.0.0.1`; containers/server platforms explicitly retain `0.0.0.0` |

## Current pointers


- Deployment vars: [`deployment.md`](deployment.md)

## Branch hygiene

| Fact | Value |
|:-----|:------|
| Default branch | `master` |
| Current maintenance | master-only changes; no auxiliary worktree introduced; preserve small-commit discipline |
