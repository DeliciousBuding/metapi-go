# Metapi Go — Engineering Rules

Go rewrite of [Metapi](https://github.com/cita-777/metapi). Feature parity with the original TypeScript version.

## Golden Rules

- **Feature parity**: 客户端可见行为与 TS 版保持兼容（协议、JSON 字段 camelCase、环境变量同名）；
  内部实现以 Go 简化架构为准（as-built 见 `docs/architecture.md`），不复刻 TS 内部结构。
  Keep the TS reference checkout outside this public repo, and do not document local checkout paths.
- **Single binary**: The React SPA is pre-built and embedded via `go:embed`. Do not add `npm`/`node` to the
  production image.
- **Dual dialect**: SQLite (dev/test) and PostgreSQL (production). Use `store.Open(dialect, dsn)`. Never
  assume SQLite-only features.
- **API compatibility**: All JSON responses must use camelCase field names matching the TS frontend.
  All env var names are identical to the TS version (no prefix).
- **Simplicity first**: Design one owner and one data flow, then implement the smallest direct change. Do not add
  speculative abstractions, parity scaffolds, or defensive fallback layers for behavior that has no real caller.
  Unsupported behavior stays explicit (error/501/documented residual) instead of looking implemented.
- **Before pushing**: `go build ./cmd/server && go vet ./... && go test ./... -count=1 -race` must pass.
  🚫 **Never skip local CI** — GitHub Actions is a verification gate, not a debug environment. The pre-push hook enforces this.

## Project Structure

```
cmd/server/main.go      Entry point
cmd/migrate/main.go     SQLite→PG migration tool
config/                 env map → Config（完整变量清单见 docs/deployment.md + .env.example）
store/                  DB layer (sqlx; table registry = store/tablesets.go, DDL = store/schema_ddl.go)
auth/                   Admin + proxy auth + rate limiting
routing/                TokenRouter (weighted random + Fibonacci cooldown + runtime breaker)
proxy/                  转发编排（coordinator / executor / channel selection / retry policy）
platform/               upstream adapters (registry = platform/registry.go)
transform/              协议转换（openai completions/embeddings/images/responses + gemini + shared）
service/                Domain workflows (sites/accounts/checkin/balance/notify/oauth/backup/pricing)
scheduler/              background jobs (registered in app/services.go)
handler/admin/          admin REST registrars（端点清单见 docs/api.md）
handler/proxy/          proxy routes (OpenAI, Gemini, Claude, Codex, Files)
web/dist/               Pre-built React SPA (embedded)
```

## Key Dependencies

| Package                     | Purpose                 |
| --------------------------- | ----------------------- |
| `github.com/go-chi/chi/v5`  | HTTP router             |
| `github.com/jmoiron/sqlx`   | DB access               |
| `modernc.org/sqlite`        | Pure-Go SQLite (no CGO) |
| `github.com/jackc/pgx/v5`   | PostgreSQL driver       |
| `github.com/robfig/cron/v3` | Cron scheduler          |

## Build & Test

```bash
go build -o metapi ./cmd/server       # Build server
go build -o metapi-migrate ./cmd/migrate  # Build migration tool
go test ./... -count=1 -race          # Run all tests with race detector
go vet ./...                          # Static analysis
golangci-lint run --timeout=3m        # Lint check
```

## Release Workflow

0. 所有改动经 `fix/*` / `feature/*` 等短命分支 → PR → Squash merge 回 master（详见 [`docs/internal/git-workflow.md`](docs/internal/git-workflow.md)；master 受保护，禁止直接 push）
1. 确保本地 CI 全部通过（pre-push hook 自动检查）
2. 更新 `CHANGELOG.md`（按 Keep a Changelog 格式；**必须包含 `## [vX.Y.Z]` 节**，Release 说明从该节提取）；同步 `web/package.json` 的 version 字段
3. 发布助手：`bash scripts/release.sh X.Y.Z`（校验 CHANGELOG 节、`web/package.json` 版本、master 与远端同步后打 annotated tag 并推送）；或手动 `git tag -a vX.Y.Z` → `git push origin vX.Y.Z`（仅 SemVer tag 触发发布）
4. Tag push 触发单一管道 `.github/workflows/main.yml`：全量 12 项检查通过 → 推送 `ghcr.io/deliciousbuding/metapi-go:vX.Y.Z`（amd64+arm64，provenance+SBOM）→ 5 平台二进制附件 + checksums + 二进制冒烟 → 自动创建 GitHub Release（body 取自 CHANGELOG 对应节）

**版本号**：`vMAJOR.MINOR.PATCH`（SemVer 2.0）。**节奏为 patch-first —— 1.0 前最后一位持续迭代**（每波合入即 bump 最后一位并发布；中间位留给成体系的里程碑；第一位留到 1.0）。决策权威与 1.0 就绪标准见 [`docs/internal/git-workflow.md` §6.1](docs/internal/git-workflow.md)。拿不准下一版用 `bash scripts/next-version.sh`。

## CI Discipline

- **Do not push if local CI fails**: all pushes must pass `go vet ./... && go test ./... -count=1 -race` first
- The global hook-kit chains `.githooks/pre-push-project` and automatically blocks pushes that fail local CI; `.githooks/pre-push` is only the standalone compatibility entrypoint
- Emergency skip: `git push --no-verify`. Know what it costs: on a host that uses it, **CI is the only gate that ran**, so CI's own integrity is load-bearing rather than somebody else's problem.
- GitHub Actions is the final verification gate, not a debug environment
- **A gate must be proven to gate.** Before trusting a check — new or long-standing — delete the fix it is supposed to protect (or feed it a known-bad input), confirm it goes red, then restore byte-identically and confirm with `sha256sum -c`. A check that cannot fail is worse than no check, because it spends trust while looking like coverage. This repo has shipped two: `test-sqlite-shard` selected zero packages and reported success for **30 tags** while `-race` never ran (#1175), and a verification script's "which build is running" line was printing `{"error":"Missing Authorization header"}` and being read as an answer.
- **"Reruns green" proves a fault is transient, not that it needs no fix.** Four npm-CDN tarball-integrity failures across three jobs in one day were first filed as environmental noise and shelved; the missing retry was the actual defect (#1181).
- Same rule for prose: a comment that describes what a mechanism does is a claim. Check it. `--mount=type=cache,target=/root/.bun/install-cache` claimed to persist bun's install cache while bun uses `install/cache`, so every image build re-downloaded everything (#1181).

## Specs & Docs

**Map (start here):** [`docs/README.md`](docs/README.md)

| Path                                                       | Role                                                                           |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------ |
| `docs/architecture.md`                                     | As-built package map (proxy/transform/routing; not proxycore/protocol)         |
| `docs/internal/design/BACKEND.md`                          | Backend principles + cross-cutting contracts (import edges: `docs/architecture.md` + `docs/package_boundary_test.go`) |
| `docs/internal/design/DESIGN.md`                           | UI design system source of truth                                               |
| `docs/testing.md`                                          | Test layers + sanitized real-platform testbed SOP                              |
| `docs/api.md` / `docs/deployment.md` / `docs/migration.md` | API · deploy · migration                                                       |
| `CHANGELOG.md`                                             | Version narrative                                                              |

**Task/state SSOT:** Open work is tracked in **GitHub issues**; product state and the version narrative live in **GitHub releases** and `CHANGELOG.md`. Maintainer process/history/research moved out of this public repo. Temporary session summaries are **not** source of truth — archive or delete after use.
**Ops host/image pin** lives outside this repository (private deployment surface). Public deployment notes: `docs/deployment.md`.
**Honesty:** Prefer 501 / documented residual over stub theater. Do not claim cluster-wide sticky or WS product without the matching milestone.

## Related References

- TS source: original TypeScript Metapi repository, checked out separately when parity work needs it.
- Gateway fork: private deployment, not part of this public repo.
- Ops skill: operator-local reference only. Do not publish private filesystem paths.
