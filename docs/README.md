# docs/ — MetAPI Go documentation map

**Last updated**: 2026-08-14
**Purpose**: one-screen orientation for humans and contributors.

## Progress source of truth roles

| File | Role | Not for |
|:-----|:-----|:--------|
| [`STATE.md`](STATE.md) | **现状** — verified product facts | Session diary, open TODO lists |
| [`progress/MASTER.md`](progress/MASTER.md) | **Open items + requirements** | Full changelog, ops host pins |
| [`log.md`](log.md) | **进度日志** (append-only) | Overriding STATE |

## Start here

| If you need… | Read |
|:-------------|:-----|
| Current product status | [`STATE.md`](STATE.md) |
| Open gates | [`progress/MASTER.md`](progress/MASTER.md) |
| Progress timeline | [`log.md`](log.md) |
| Product benchmark / roadmap | [`benchmark.md`](benchmark.md) |
| Package architecture | [`architecture.md`](architecture.md) |
| Backend design rules | [`design/BACKEND.md`](design/BACKEND.md) |
| UI design system | [`design/DESIGN.md`](design/DESIGN.md) |
| Version history | root [`CHANGELOG.md`](../CHANGELOG.md) |
| Deploy / ops vars | [`deployment.md`](deployment.md) |
| HTTP API | [`api.md`](api.md) |
| Git branch & PR workflow | [`git-workflow.md`](git-workflow.md) |
| Contribute / report | root [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`SECURITY.md`](../SECURITY.md) |

## Layout

```
docs/
  README.md                 ← this map
  STATE.md                  ← 现状 source of truth (keep slim)
  log.md                    ← progress log (append-only)
  benchmark.md              ← product benchmark (New API × All API Hub) + roadmap
  architecture.md           ← as-built package & request path
  api.md                    ← public API notes
  deployment.md             ← run / Docker / ops vars
  git-workflow.md           ← branch model / PR / protection rules
  migration.md              ← SQLite → PG / schema upgrade
  responses-websocket-residual.md ← Responses API WS transport (501 residual)
  design/                   ← living design source of truth
    BACKEND.md                backend design philosophy, dependency rules
    DESIGN.md                 design system, tokens, visual language
    a11y-checklist.md         accessibility checklist
    components.md             component library docs
  analysis/                 ← evidence-based design docs
    db-pool-budget.md         PG pool profiles
    package-boundaries.md     B1 package ownership inventory
    redis-shared-state.md     Redis shared state design
    ui-ux-audit-2026-08.md    multi-agent UI/UX audit + backlog
  progress/                 ← MASTER only (open gates)
```

## Hygiene rules

- Prefer **merge/update** over new parallel analysis files.
- Absolute dates, not "recently".
- `docs/doc_hygiene_test.go` enforces public markdown hygiene (no local paths / false Redis sticky claims).
