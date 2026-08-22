# docs/ — Metapi Go documentation map

**Last updated**: 2026-08-23
**Purpose**: one-screen orientation for users and contributors.

This directory is split into **public docs** (written for users and
contributors) and **[`internal/`](internal/)** (maintainer process docs:
product state, roadmap, progress log, audits, design notes). User-facing
documents never deep-link into `internal/` — they state the fact directly.
`docs/doc_hygiene_test.go` enforces both rules in CI.

## Public docs

| If you need…                        | Read                                                                                     |
| :---------------------------------- | :--------------------------------------------------------------------------------------- |
| Install → first proxied request     | [`getting-started.md`](getting-started.md)                                               |
| Deploy / ops vars / reverse proxy   | [`deployment.md`](deployment.md)                                                         |
| Environment variable reference      | [`configuration.md`](configuration.md)                                                   |
| Client wiring (Cursor etc.)         | [`client-integration.md`](client-integration.md)                                         |
| Common questions                    | [`faq.md`](faq.md)                                                                       |
| HTTP API surface                    | [`api.md`](api.md)                                                                       |
| Package architecture & request path | [`architecture.md`](architecture.md)                                                     |
| TS→Go migration (SQLite / PG / MySQL) | [`migration.md`](migration.md)                                                           |
| Test layers / real-platform testbed | [`testing.md`](testing.md) |
| UI screenshot evidence & golden visual regression | [`visual-regression.md`](visual-regression.md) |
| Version history                     | root [`CHANGELOG.md`](../CHANGELOG.md)                                                   |
| Contribute / report                 | root [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`SECURITY.md`](../SECURITY.md)           |

## Internal docs (maintainer process)

| Path                                             | Role                                                        |
| :----------------------------------------------- | :---------------------------------------------------------- |
| [`internal/STATE.md`](internal/STATE.md)         | **现状** — verified product facts (keep slim)               |
| [`internal/progress/MASTER.md`](internal/progress/MASTER.md) | **3 条交付主线 + 唯一执行计划**（不是 changelog） |
| [`internal/log.md`](internal/log.md)             | **进度日志** (append-only，不覆盖 STATE)                    |
| [`internal/benchmark.md`](internal/benchmark.md) | 产品对标（New API × All API Hub）+ direction                |
| [`internal/git-workflow.md`](internal/git-workflow.md) | 分支模型 / PR / 保护规则                              |
| [`internal/design/`](internal/design/)           | 设计系统 SSOT（BACKEND / DESIGN / a11y / components）       |
| [`internal/analysis/`](internal/analysis/)       | 证据型分析（pool budget / package boundaries / audit 证据） |
| [`internal/responses-websocket-residual.md`](internal/responses-websocket-residual.md) | Responses WS 501 residual 说明 |

## Layout

```
docs/
  README.md                 ← this map
  getting-started.md        ← tutorial: install → first proxied request
  api.md                    ← public API notes
  architecture.md           ← as-built package & request path
  client-integration.md     ← client wiring (Cursor / Claude Code / Codex / Open WebUI)
  configuration.md          ← environment variable reference
  deployment.md             ← run / Docker / ops vars
  faq.md                    ← common questions
  migration.md              ← TS→Go takeover paths (SQLite / PG / MySQL) + version pinning
  testing.md                ← test layers + public real-platform testbed SOP
  assets/                   ← public images (hero, screenshots)
  internal/                 ← maintainer process docs (never linked from README)
    STATE.md                  现状 source of truth (keep slim)
    log.md                    progress log (append-only)
    benchmark.md              product benchmark + direction
    git-workflow.md           branch model / PR / protection rules
    responses-websocket-residual.md
    progress/                 MASTER only (mainlines + executable open plan)
    design/                   living design source of truth
    analysis/                 evidence-based analysis docs
```

## Hygiene rules

- Prefer **merge/update** over new parallel analysis files.
- Absolute dates, not "recently".
- Public docs state facts for users; process context (waves, gaps, audit
  rounds) belongs in `internal/`.
- `docs/doc_hygiene_test.go` enforces public markdown hygiene (no local
  paths / no credential DSNs / no false Redis sticky claims) and the
  README → `internal/` link boundary.
