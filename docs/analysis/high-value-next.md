# High-value next candidates (ours vs original)

**Date**: 2026-08-01
**Scope**: planning inventory — **no product code in this file**.  
**Mode**: **parity program** after v0.8.45; plan SSOT [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md).  
**Ops pin**: hk3 remains v0.8.45; master is materially ahead and must not be described as deployed (read server `projects/metapi/STATE.md`).

> **两套问题，不要混：**  
> - **Ours** = residual / ops / engineering（[`residual-next-candidates.md`](./residual-next-candidates.md) + [`../STATE.md`](../STATE.md)）  
> - **Original** = 上游 parity（[`original-gap-matrix.md`](./original-gap-matrix.md)；sources 2026-07-16 snapshot + 2026-07-20 reverify）

## How to read

| Surface | Role |
|:--------|:-----|
| This file | **Next-wave shortlist** |
| [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md) | **Parity program plan** (ex-Electron) |
| [`residual-next-candidates.md`](./residual-next-candidates.md) | Full honesty inventory |
| [`original-gap-matrix.md`](./original-gap-matrix.md) | Evidence table (may lag code) |
| [`../progress/MASTER.md`](../progress/MASTER.md) | Open gates + schedule |

---

## A. Ours — residual / ops / engineering

| Rank | ID | Title | Status | Next | Risk if skip |
|-----:|:---|:------|:-------|:-----|:-------------|
| 1 | **METRIC-TRUTH** | Daily metric truth | **Dashboard + Accounts closed in master** | — | Silent fake-zero decisions |
| 2 | **WS-1** | Responses WebSocket Codex | **C1+C2+C3 present** | Full TS parity shipped (C3 Codex upstream wss flagged); sticky single-instance honesty | multi-instance pin only |
| 3 | **#547** | Per-downstream-key weight | **present** | shipped key_weight + selector + UI | — |
| 4 | **#584** | Site header override priority | **present** | shipped override flag + ApplyCustomHeadersWithOptions + Sites UI | — |
| 5 | **#579** | Multi-credential on one key | **present** | allow-list sites/credentials shipped | — |
| 6 | **#514** | Multi-tier ctx routing | **present** | estimate + tightest-fit among same-model routes | residual: tokenizer accuracy |
| 7 | **P0-585** | Channel cascade | partial | HTTP e2e present; live procedure #557 + `scripts/p0585_cascade_probe.py` | Silent cascade claim |
| 8 | **P0-555** | Billing accuracy | present-with-residual | media detail fold + orphan/missing-usage observability; multi-instance lag residual | not perfect billing |
| 9 | **UC-1** | Update-center deploy | **hide/external present** | UI ops note + 501 residual | — |
| 10 | **STICKY-B** | Redis sticky | design-only | **Deferred** — single-instance / LB pin honesty | Multi-instance multi-turn |
| 11 | **OPS-PIN** | Prod 0.8.45 | **present / healthy** | Keep production pin distinct from master candidates | False deployed claims |
| 12 | **REL-RE2** | RE2 user-id | **present** v0.8.45 | Ops pin | Was Exited(2) |
| 13 | **OAUTH-REFRESH** | OAuth token scheduler | **present in master, not v0.8.45 runtime** #251 | Release + soak before ops claim | Expired sessions remain manual in production |
| 14 | **SUB2API-REFRESH** | Managed token due window | **present** | — | Was always-true due |

### Explicit non-goals (without reopening)

- Electron desktop  
- Fake WS frames / Hijack-silent-close  
- STICKY-B unless multi-instance product reopen  
- Invent UC registry client  
- Flip P0-585 present from unit tests alone  
- Claim perfect billing  

---

## B. Original matrix leftovers (reverified)

| Upstream# | Title | Our status | Next |
| ---: | --- | --- | --- |
| **585** | Channel cascade | partial | Prod e2e |
| **555** | Token stats | present-with-residual | Media / multi-instance |
| **579** | Multi-key binding | **present** | allow-list bind |
| **547** | Per-key weight | **present** | — |
| **584** | Header priority | **present** | — |
| **514** | Multi-tier ctx | **present** | — |
| **534** | Bulk account import | **present** (matrix stale if still missing) | Docs only |
| **520** | context_length | **present-with-residual** | Dialects residual only |
| **577** | AnyRouter check-in | partial | Live runtime |
| **571** | Codex OAuth gpt-5.5 | unknown-needs-runtime | Live probe only (static allowlist+tests present) |

Out-of-product: Electron · MySQL · k3s · noise issues.

---

## C. Competitive refresh (2026-08-01)

| Rank | Pattern | Peer signal | Decision |
| ---: | --- | --- | --- |
| 1 | Daily metric `complete / partial / unavailable` truth | all-api-hub #1195 | **Dashboard slice implemented**; extend to Accounts without collapsing unknown to zero |
| 2 | Cache read/create and hit-rate by downstream key/day | New API | Next observability candidate; typed columns + dual-dialect queries, admin-only |
| 3 | Declarative provider capabilities | all-api-hub native adapters | Next adapter-foundation candidate; separate check-in, balance, token and mutation support |
| 4 | State-aware onboarding with recovery deep links | all-api-hub #1237 | UX candidate after metric truth; do not add another static checklist |
| 5 | Partial streaming usage survives abnormal termination | Sub2API #5154 | Audit MetAPI stream accounting first; retry/failover must prevent double charge |
| 6 | Grok CLI / Hermes export profiles | all-api-hub #1204 | Small P1/P2 export extension after capability truth |
| 7 | Typed announcement CTA | all-api-hub #1241 | Validate internal SPA routes separately from external HTTPS URLs |

Rejected: browser protection bypass, extension-only credential scraping, referral/wallet growth loops, default IP/Geo recording, and SaaS billing sprawl.

---

## D. Recommended sequencing (authoritative)

1. Parity core **shipped**: KEYS · WS C1–C3 · #514 · UC-1 · C4 docs.  
2. **P0-585** production e2e only (partial until then).  
3. **P0-555** remains present-with-residual (media fold present; multi-instance lag).  
4. Dashboard daily truth **present in master**; close Accounts unknown-vs-zero before adding more charts.
5. Production remains **0.8.45** until an explicit release + image + deployment + soak wave.
6. Then choose one low-coupling competitive slice: cache efficiency or provider capability declarations.

---

## E. Doc ownership

| Question | Read |
|:---------|:-----|
| Production now? | [`../STATE.md`](../STATE.md) + server metapi STATE |
| Parity program? | [`../plan/original-parity-complete-2026-07-20.md`](../plan/original-parity-complete-2026-07-20.md) |
| Open gates? | [`../progress/MASTER.md`](../progress/MASTER.md) |
| Residual ours? | [`residual-next-candidates.md`](./residual-next-candidates.md) |
| Matrix evidence? | [`original-gap-matrix.md`](./original-gap-matrix.md) |
| WS residual? | [`responses-websocket-residual.md`](./responses-websocket-residual.md) |
| 正式可用? | [`formal-readiness.md`](./formal-readiness.md) |
| UI 对照? | [`ui-original-parity-2026-07-20.md`](./ui-original-parity-2026-07-20.md) |
