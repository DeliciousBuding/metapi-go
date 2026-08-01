# STATE.md — MetAPI Go product status

**Last verified**: 2026-08-02

> **现状 SSOT**（产品仓库）。只记当前事实与指针，不写流水账。
> 运维主机、compose、镜像 pin 与 PG role LIMIT 以 server 仓 `projects/metapi/STATE.md` 为准。
> 开放项 → [`progress/MASTER.md`](progress/MASTER.md) · 时间线 → [`log.md`](log.md) · 版本叙事 → 根 [`CHANGELOG.md`](../CHANGELOG.md)

## Current

| Fact | Value |
|:-----|:------|
| Source | **[DeliciousBuding/metapi-go](https://github.com/DeliciousBuding/metapi-go)** · default branch `master` |
| Latest release | **[v0.8.45](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.8.45)** (2026-07-20); master CD publishes `ghcr.io/deliciousbuding/metapi-go` |
| Product tip | current maintenance wave hardens seeded EN/zh verification, SQLite OAuth refresh, Windows listen/firewall behavior, OAuth callback ownership, GHCR ownership, and closes daily-metric truth on Dashboard + Accounts (unknown vs zero, partial status) |
| Production pin (ops) | hk3 `td-metapi` **0.8.45 Up healthy** since 2026-07-20 on legacy `ghcr.io/tokendancelab/metapi-go`; Azure PG pool/role **1/1**; `restart=no` |
| Standby | us1 cold stack; must not connect to the production PG concurrently with hk3 |
| Active milestone | **[53 REL-HONESTY](https://github.com/DeliciousBuding/metapi-go/milestone/53)** — #557 production e2e + #558 optional runtime probes open |
| Open issues / PRs | [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) · [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) |
| Stack | Go 1.26.5 · React 19 · Vite 8 · dual dialect SQLite/PostgreSQL |
| Runtime shape | single embedded SPA binary · **16** background schedulers · OAuth callback listeners start only with an active flow |

## Product honesty

| ID | Status | Note |
|:---|:-------|:-----|
| P0-585 cascade | **partial** | HTTP load proof exists; production multi-channel proof still required |
| P0-555 usage stats | **present-with-residual** | media detail fold shipped; multi-instance lag remains |
| WS-1 Responses WebSocket | **C1+C2+C3 present** | single-instance honesty; no cluster-wide sticky claim |
| STICKY-B Redis sticky | **deferred** | use a single instance or load-balancer pinning |
| UC-1 update-center deploy | **hide/external present** | releases/GHCR are external; API residual remains 501 |
| OPS-PG-BUDGET | **present** | profiles + lease backoff; deployment must still respect role LIMIT |
| OPS-RE2-USERID | **fixed and deployed** | v0.8.45 is healthy on hk3; the 0.8.44 crash state is historical |
| OPS-OAUTH-REFRESH | **present** | shared explicit account/site projection supports SQLite and PostgreSQL |
| UI i18n verification | **present** | EN and zh cover 18 routes with seeded real data-state assertions |
| Daily metric truth | **Dashboard + Accounts closed in master** | shared local-day aggregation, real reward/check-in, partial truth metadata, query errors fail closed; per-account today reward/spend with status gate on Accounts rows (no fake zeros) |
| Windows local development | **loopback by default** | empty `HOST` binds `127.0.0.1`; containers/server platforms explicitly retain `0.0.0.0` |

## Current pointers

- High-value next: [`analysis/high-value-next.md`](analysis/high-value-next.md)
- Parity plan: [`plan/original-parity-complete-2026-07-20.md`](plan/original-parity-complete-2026-07-20.md)
- UI acceptance: [`analysis/ui-visual-acceptance.md`](analysis/ui-visual-acceptance.md)
- Residual inventory: [`analysis/residual-next-candidates.md`](analysis/residual-next-candidates.md)
- Engineering optimization: [`analysis/engineering-optimization-2026-07-30.md`](analysis/engineering-optimization-2026-07-30.md)
- Deployment vars: [`deployment.md`](deployment.md)
- Live operations: server `projects/metapi/STATE.md`

## Branch hygiene

| Fact | Value |
|:-----|:------|
| Default branch | `master` |
| Current maintenance wave | master-only changes; no auxiliary worktree introduced; preserve small-commit discipline |
| Historical branch | `origin/codex/metapi-regex-crash`; superseded by `af2749c` on master, PR #542 closed 2026-08-02 |
