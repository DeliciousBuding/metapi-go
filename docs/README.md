# docs/ — Metapi Go documentation map

**Last updated**: 2026-09-12
**Purpose**: one-screen orientation for users and contributors.

This directory is split into **public docs** (written for users and
contributors) and **[`internal/`](internal/)** (contributor design notes and
residual caveats). Maintainer process/history/research — product state,
roadmap, progress log, audits, benchmarks — live outside this public repo
(open work in GitHub issues; version narrative in `CHANGELOG.md`). User-facing
documents never deep-link into `internal/` — they state the fact directly.
`docs/doc_hygiene_test.go` enforces these rules in CI.

## Public docs

| If you need…                        | Read                                                                                     |
| :---------------------------------- | :--------------------------------------------------------------------------------------- |
| Install → first proxied request     | [`getting-started.md`](getting-started.md)                                               |
| Deploy / ops vars / reverse proxy   | [`deployment.md`](deployment.md)                                                         |
| Environment variable reference      | [`configuration.md`](configuration.md)                                                   |
| Client wiring (Cursor etc.)         | [`client-integration.md`](client-integration.md)                                         |
| Common questions                    | [`faq.md`](faq.md)                                                                       |
| HTTP API surface (index + by-domain) | [`api.md`](api.md) · [`api/`](api/)                                                        |
| Package architecture & request path | [`architecture.md`](architecture.md)                                                     |
| TS→Go migration (SQLite / PG / MySQL) | [`migration.md`](migration.md)                                                           |
| Test layers / real-platform testbed | [`testing.md`](testing.md) |
| UI screenshot evidence & golden visual regression | [`visual-regression.md`](visual-regression.md) |
| Version history                     | root [`CHANGELOG.md`](../CHANGELOG.md)                                                   |
| Contribute / report                 | root [`CONTRIBUTING.md`](../CONTRIBUTING.md) · [`SECURITY.md`](../SECURITY.md)           |

## Internal docs (contributor design & residual notes)

| Path                                             | Role                                                        |
| :----------------------------------------------- | :---------------------------------------------------------- |
| [`internal/git-workflow.md`](internal/git-workflow.md) | Branch model, PR flow, protection rules, release cadence |
| [`internal/design/`](internal/design/)           | Design source of truth (BACKEND / DESIGN / a11y / components / state-stability) |
| [`internal/web-package-boundaries.md`](internal/web-package-boundaries.md) | Frontend layering contract (`web/scripts/check-boundaries.mjs` enforces it) |
| [`internal/responses-websocket-residual.md`](internal/responses-websocket-residual.md) | Why the Responses WebSocket surface answers 501 |

## Layout

```
docs/
  README.md                 ← this map
  getting-started.md        ← tutorial: install → first proxied request
  api.md                    ← public API notes (index + anchor stubs)
  api/                      ← by-domain API files (stats / sites / settings / ...)
  architecture.md           ← as-built package & request path
  client-integration.md     ← client wiring (Cursor / Claude Code / Codex / Open WebUI)
  configuration.md          ← environment variable reference
  deployment.md             ← run / Docker / ops vars
  faq.md                    ← common questions
  migration.md              ← TS→Go takeover paths (SQLite / PG / MySQL) + version pinning
  testing.md                ← test layers + public real-platform testbed SOP
  visual-regression.md      ← screenshot evidence + golden visual regression SOP
  assets/                   ← public images (hero, screenshots)
  internal/                 ← contributor design notes & residual caveats (never linked from README)
    design/                   living design source of truth
    git-workflow.md           branch model / PR / protection rules
    web-package-boundaries.md frontend layering gate contract
    responses-websocket-residual.md
```

## Hygiene rules

- Prefer **merge/update** over new parallel analysis files.
- Absolute dates, not "recently".
- Public docs state facts for users. `internal/` holds contributor *design*
  notes and residual caveats — it is tracked and public, so maintainer
  process context (product state, roadmap, progress log, audits,
  benchmarks) belongs outside this repository entirely: open work lives in
  GitHub issues, the version narrative in `CHANGELOG.md`.
- Work-programme vocabulary does not belong in the published tree either:
  the batch, priority and audit labels of an internal plan, or a citation of
  a plan file that was never published. A comment states what the code does,
  not which plan asked for it — the label is unresolvable for anyone outside
  that plan and rots the day the plan is archived.
- `docs/doc_hygiene_test.go` enforces all of the above in CI: public markdown
  hygiene (no local paths / no credential DSNs / no false Redis sticky claims
  / no AI citation artifacts), the README → `internal/` link boundary, and
  the work-programme vocabulary ban.
