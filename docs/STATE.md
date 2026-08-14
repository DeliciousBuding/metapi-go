# STATE.md — MetAPI Go product status

**Last verified**: 2026-08-14

> **Current state** (product repository). Only current facts and pointers, no narrative.
> Deployment facts live in the deployment guide; open items → [`progress/MASTER.md`](progress/MASTER.md) · timeline → [`log.md`](log.md) · version narrative → root [`CHANGELOG.md`](../CHANGELOG.md)

## Current

| Fact | Value |
|:-----|:------|
| Source | **[DeliciousBuding/metapi-go](https://github.com/DeliciousBuding/metapi-go)** · default branch `master` |
| Latest release | **[v0.12.0](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.12.0)** (2026-08-14); master pipeline publishes `ghcr.io/deliciousbuding/metapi-go` — verified live: tags `latest`/`0.12.0`/`0.12`/sha |
| Release pipeline | Single pipeline (`.github/workflows/main.yml`): PR / master push / SemVer tag all run the same 12-check suite; master push pushes the **linux/amd64 + linux/arm64** image (provenance + SBOM, tags latest+sha); SemVer-only tags (`vX.Y.Z`) additionally build **5-platform binaries + checksums.txt** (linux/darwin/windows × amd64/arm64), smoke `metapi-linux-amd64 --version`, and create the GitHub Release with notes extracted from the matching `CHANGELOG.md` section; tag must match `web/package.json` version or the release fails |
| Versioning | `metapi --version` reports the build version injected via `-ldflags -X .../internal/version.Version` (`dev` for local builds); Docker build arg `VERSION`; Makefile `VERSION` variable |
| Dependency updates | Dependabot active (Go / npm / GitHub Actions / Docker) — weekly group PRs through the same 12-check CI; breaking major bumps are closed and need a manual migration PR |

| Production pin (ops) | **v0.12.0** image on `ghcr.io/deliciousbuding/metapi-go` (deployed 2026-08-14); runtime deployment facts live in the deployment docs |
| Current focus | distribution UX（P0：客户端配置一键导出 + 接入向导；roadmap → [`benchmark.md`](benchmark.md)） |
| Open issues / PRs | [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) · [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) · [#562](https://github.com/DeliciousBuding/metapi-go/issues/562) |
| Stack | Go 1.26.6 · React 19 + Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH + i18next · dual dialect SQLite/PostgreSQL |
| Runtime shape | single embedded SPA binary · **16** background schedulers · OAuth callback listeners start only with an active flow |
| Brand | **MetAPI** · transparent solid-blue SVG badge `web/public/logo.svg` (real U+03C0 π glyph) + `favicon.svg` · served from the embedded SPA root by the router whitelist |

## Product honesty

| ID | Status | Note |
|:---|:-------|:-----|
| cascade | **partial** | HTTP load proof exists; production multi-channel proof still required |
| usage stats | **present-with-residual** | media detail fold shipped; multi-instance lag remains |
| Responses WebSocket | **present** | single-instance honesty; no cluster-wide sticky claim |
| Redis sticky sessions | **deferred** | use a single instance or load-balancer pinning |
| Update-center deploy | **external present** | releases/GHCR are external; API residual remains 501 |
| PostgreSQL pool budget | **present** | profiles + lease backoff; deployment must still respect role LIMIT |
| RE2 user-id extraction | **present** | pre-compiled RE2-safe regex; the 0.8.44 crash state is historical |
| OAuth token refresh | **present** | shared explicit account/site projection supports SQLite and PostgreSQL |
| UI i18n verification | **present (v0.9.0 rewrite)** | key-based i18n (i18next): en + zh-CN identical key sets (vitest i18n-keys gate, bidirectional 0 missing); header `LanguageSwitcher` (en/zh-CN) + browser-language auto-follow (localStorage → navigator) + `document.documentElement.lang` sync via `toBcp47`; sidebar nav fully key-based (sidebar.groups/items) |
| EN label coverage | **present (v0.9.0 rewrite)** | all UI copy via t() (i18next); vitest i18n-keys gate scans t() sites with en/zh-CN identical; live status badge colors tokenized |
| Theme accent presets | **chart-synced (v0.9.0 rewrite)** | 4-axis theme (preset/font/radius/scale) + 10 presets; useChartColors() syncs chart series with OKLCH tokens (FOUC-safe); header ThemeCustomizer panel (preset swatches / font Auto-Sans-Serif / radius / scale + per-axis & global reset); **all presets default to sans-serif** (serif is an explicit font-axis choice, no preset inlines it) |
| Daily metric truth | **Dashboard + Accounts closed in master** | shared local-day aggregation, real reward/check-in, partial truth metadata, query errors fail closed; per-account today reward/spend with status gate on Accounts rows (no fake zeros) |
| Windows local development | **loopback by default** | empty `HOST` binds `127.0.0.1`; containers/server platforms explicitly retain `0.0.0.0` |

## Current pointers


- Deployment vars: [`deployment.md`](deployment.md)

## Branch hygiene

| Fact | Value |
|:-----|:------|
| Model | GitHub Flow — master 唯一长期分支（受保护），短命分支（`fix/*`/`feature/*`/`chore/*`/`docs/*`）→ PR → Squash merge |
| Default branch | `master` |
| Master protection | PR required + 12 CI status checks required + enforce admins; no approve requirement; squash-only merge (repo-level) |
| Workflow doc | [`git-workflow.md`](git-workflow.md) |
