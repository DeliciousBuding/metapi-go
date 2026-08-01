# MASTER.md — MetAPI Go open gates + parity program

**Last verified**: 2026-08-01

**Repo**: https://github.com/DeliciousBuding/metapi-go

**Mode**: **GITHUB_FULL** · parity core shipped · active M53 REL-HONESTY

**Project**: https://github.com/orgs/TokenDanceLab/projects/1

**Release**: v0.8.45 · master CD `ghcr.io/deliciousbuding/metapi-go` · production still pinned to legacy TokenDanceLab GHCR

**Program plan**: [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md)

> **开放项 + 硬门禁**。现状 → [`../STATE.md`](../STATE.md) · 日志 → [`../log.md`](../log.md) · shortlist → [`../analysis/high-value-next.md`](../analysis/high-value-next.md)

## Current status

| Fact | Value |
|:-----|:------|
| Active work | maintenance wave: real seeded EN/zh verification, SQLite OAuth refresh query ownership, Windows loopback/firewall hygiene, lazy OAuth callbacks |
| User decisions | WS = full TS parity · sticky = single-instance honesty · UC = external deploy |
| Ops | hk3 **0.8.45 healthy** since 2026-07-20; pool/role 1/1; restart=no |
| Board | M53 · [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) P0-585 production e2e · [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) optional runtime probes |

## Open product board

| Issue | Track | Title |
|------:|:------|:------|
| [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) | REL | Production multi-channel cascade e2e; keep P0-585 partial until live proof |
| [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) | REL | Runtime probes for Codex and AnyRouter (optional) |

## Hard gates

1. Do not invent WS frames, fake update availability, or cluster sticky behavior.
2. P0-585 remains partial until production multi-channel e2e evidence exists.
3. P0-555 remains present-with-residual; multi-instance aggregation is not exact billing.
4. Pre-push: `go build ./cmd/server && go vet ./... && go test ./... -count=1 -race`.
5. EN/zh browser verification must use seeded real data and fail closed when setup is incomplete.
6. SQLite and PostgreSQL query paths must share explicit projections; avoid `SELECT a.*, s.*` scan ambiguity.
7. Windows development binds loopback unless `HOST` is explicitly set; containers set `HOST=0.0.0.0`.
8. Ops runtime facts come only from server `projects/metapi/STATE.md`.
9. Electron remains a non-goal.

## Scheduled work

| Order | Wave | Work | Status |
|------:|:-----|:-----|:-------|
| 0 | MAINT | Seeded EN/zh verifier + translation regressions | implemented; see log for validation evidence |
| 1 | MAINT | OAuth refresh account/site query shared across connection and scheduler | implemented; see log for validation evidence |
| 2 | MAINT | Windows loopback default + targeted stale firewall-rule maintenance | implemented and locally audited |
| 3 | MAINT | Remove dormant OAuth loopback scheduler; start provider callbacks lazily | implemented; see log for validation evidence |
| 4 | MAINT | Daily metric truth: Dashboard + Accounts unknown-vs-zero, partial status, fail-closed SQL | implemented; see log for validation evidence |
| 5 | REL | P0-585 production multi-channel e2e | partial; no production write authorized |
| ops | — | hk3 v0.8.45 pin/up + soak | **done and healthy**; no deployment in this wave |

## Quick status

```bash
gh issue list --repo DeliciousBuding/metapi-go --state open --limit 20
gh pr list --repo DeliciousBuding/metapi-go --state open
gh release view v0.8.45 --repo DeliciousBuding/metapi-go
gh project view 1 --owner TokenDanceLab
```

## Next agent

1. Finish the maintenance-wave local gates and commit in small, reviewable units.
2. Keep production untouched; #557 needs explicit live authorization and its documented procedure.
3. Reconcile or close historical PR #542 after confirming the RE2 fix is already on `master`.
4. Do not introduce duplicate callback listeners, wildcard local binds, or dialect-specific scan shortcuts.
