# MASTER.md — MetAPI Go open gates + parity program

**Last verified**: 2026-08-01  
**Repo**: https://github.com/TokenDanceLab/metapi-go  
**Mode**: **GITHUB_FULL** · product **parity core shipped**; active M53 REL-HONESTY  
**Project**: https://github.com/orgs/TokenDanceLab/projects/1  
**Tip**: `b508b84` · tag **v0.8.45** · unreleased: parity core + P0-555 obs + P0-585 HTTP e2e + #557 procedure + CI unblock + dual-dialect Context helpers + engineering optimization wave + N2-N7/G1 productization batch + all-api-hub borrow Wave A–D + K1a/K1b + N9a/N9b-a + A3 + B1 审计 + B2 实时运维 WS + VIS-1/NAV-1/DENSE-1 + review 修复 + **i18n 全面收官（t() 门禁 / 181 条裸 JSX 补译 / 插值碎片 / canvas 快照 EN 化）**
**Program plan**: [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md)

> **开放项 + 硬门禁**。现状 → [`../STATE.md`](../STATE.md) · 日志 → [`../log.md`](../log.md) · shortlist → [`../analysis/high-value-next.md`](../analysis/high-value-next.md)

## Current status

| Fact | Value |
|:-----|:------|
| Active work | UI 待办收官（VIS-1/NAV-1/DENSE-1）+ 双 agent 对抗 review 修复；#557 live soak pending; ops pin gated |
| User decisions | WS = **full TS parity**; sticky = **single-instance honesty**; UC = **hide/external deploy** |
| Ops | hk3 pin still **0.8.44 Exited** until authorized **0.8.45** soak (server STATE) |
| Board | M53 REL-HONESTY · open [#557](https://github.com/TokenDanceLab/metapi-go/issues/557) P0-585 prod e2e · [#558](https://github.com/TokenDanceLab/metapi-go/issues/558) runtime probes |

## Open product board

| Issue | Track | Title |
|------:|:------|:------|
| [#557](https://github.com/TokenDanceLab/metapi-go/issues/557) | REL | P0-585 production multi-channel cascade e2e (keep partial until live) |
| [#558](https://github.com/TokenDanceLab/metapi-go/issues/558) | REL | Runtime probes #571/#577 live (optional) |

## Hard gates

1. **No invent** WS frames / Hijack-silent-close / fake updateAvailable / cluster sticky without AC.  
2. **WS-1 C1–C3 present** — C4 single-instance honesty only; no STICKY-B without reopen.  
3. **STICKY-B Redis deferred** (single-instance / LB pin honesty only).  
4. **UC-1**: hide or external ops deploy — **not** invent registry.  
5. **P0-585 stays partial** until production e2e.  
6. **P0-555 stays present-with-residual** (media detail fold present; multi-instance lag residual).  
7. Pre-push: `go vet ./... && go test ./... -count=1 -race`.  
8. Ops pin SSOT: server `projects/metapi/STATE.md`.  
9. Electron = **non-goal**.

## Scheduled next (from parity plan)

| Order | Wave | Work | Status |
|------:|:-----|:-----|:-------|
| 0 | DOC | Truth: #534/#520 present; matrix/MASTER/high-value | done (2026-07-21) |
| 1 | KEYS | **#547** present · **#584** present · **#579** present | allow-list bind shipped |
| 2 | WS | **C1** upgrade+HTTP bridge → **C2** multi-turn → **C3** upstream wss | **C1+C2+C3 present** |
| 3 | ROUTE | **#514** multi-tier ctx | **present** |
| 4 | REL | P0-585 HTTP e2e load-proof **present**; prod e2e still pending · P0-555 residual | partial |
| 5 | UC | Hide/external Update Center honesty | **present** (UI external card + API residual) |
| ops | — | Pin/up **0.8.45** + ≥15min soak | needs admin auth |

## Optional / not blockers

| Priority | Candidate |
|:---------|:----------|
| Docs/visual | Empty-DB page shot recapture (`METAPI_UI_AUTH_TOKEN`) — SHOT-1 |
| Runtime | #571 Codex OAuth gpt-5.5 · #577 AnyRouter live |

## Quick status

```bash
gh issue list --state open --limit 20
gh pr list --state open
gh release view v0.8.45
gh project view 1 --owner TokenDanceLab
```

## Next agent

1. Read [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md) + [`../STATE.md`](../STATE.md).  
2. REL: P0-585 needs **production/live e2e** per [`../analysis/p0585-production-e2e-procedure.md`](../analysis/p0585-production-e2e-procedure.md) (#557); HTTP e2e already on tip.  
3. Ops pin **0.8.45** only with admin auth + ≥15min soak.  
4. Do **not** invent UC registry / STICKY-B / fake WS terminals.  
5. Product borrow backlog（决策输入，未立项）: [`../analysis/competitive/all-api-hub-product-borrow-2026-07-31.md`](../analysis/competitive/all-api-hub-product-borrow-2026-07-31.md) — **A1-J1 全部立项项已发（14/14，含 A3 收官；K1 拆分 K1a + K1b 均已发）**；N8 关闭（架构等价，[评估文档](../analysis/competitive/n8-n9-deferred-assessment-2026-08-01.md)）；**N9a + N9b-a 已发，N9b-b 关闭**；deferred 清单已清空（2026-08-01）。
