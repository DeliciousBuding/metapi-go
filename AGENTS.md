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
- **Simplicity first**: 按下方「编码行为准则」实施；一个职责 owner、一条明确数据流。
  不支持的行为明确报错、返回 501 或记录边界，不用占位实现制造成功。
- **Before pushing**: `go build ./cmd/server && go vet ./... && go test ./... -count=1 -race` must pass.
  🚫 **Never skip local CI** — GitHub Actions is a verification gate, not a debug environment. The pre-push hook enforces this.

## 编码行为准则

吸收 Karpathy behavioral guidelines，适用于编写、审阅与重构。复杂度与风险决定检查深度；简单任务不增加仪式性流程。

### 动手前想清楚

- 明说影响结果的假设与取舍，不掩盖不确定性。有多种会改变结果的解释时先说明差异。
- 先查源码、调用方和现有约定；仍有会实质改变目标、权限或验收的疑点时，停下相关实现并澄清。
- 有更简单的实现就提出；不合理的要求或设计应说明具体代价，不默默增加复杂度。

### 简单优先

- 只实现本次目标所需的最少代码；不为假想需求增加功能、配置、扩展点或兼容路径。
- 单次使用的代码通常直接写在现有 owner 内；抽象须有真实复用或明确边界，不为统一形式造层。
- 校验外部输入与真实失败，不为内部不可能状态堆防御分支。能用清晰的短实现解决，就简化长实现。

### 精准修改

- 每个改动都应能对应本次请求或已核实缺陷；沿用现有风格，不顺手重构邻近代码、注释或格式。
- 清理本次改动造成的无用 import、变量和函数；无关历史死代码只报告，未经要求不删除。
- 保留他人 WIP；规则和文档优先合并已有内容，不追加平行副本。

### 以可验证结果推进

- 开始时把目标写成可检查的结果；多步任务用简短计划说明每步如何验证。
- 修 bug 先复现；加校验覆盖非法输入；重构核对改前与改后行为。测试必须保护用户可见契约。
- 检查真实生产者、存储与消费者，不能只造一个实现并不产生的 fixture 来证明功能存在。
- 持续修到相称验证通过；代码、测试、现役文档一起交付。CI 等待期间推进独立工作，合并仍遵守门禁。

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

- **Development loop**: Edit, debug, and run focused checks in a local checkout or isolated worktree; SSH-only development is not required.
  Full CI/E2E may run on an isolated test server. This does not replace the pre-push build/vet/full-race gate or required SQLite/PostgreSQL checks.
- **Evidence identity**: Record the tested commit and any uncommitted diff, build/artifact digest, commands, and relevant environment.
  A server run must trace back to those exact source inputs and artifacts. Reuse successful checks only for byte-identical tested inputs within their verified scope;
  rerun affected checks when inputs or environments change, and never label an unrun check as passed. Evidence reuse does not bypass mandatory gates.
- **Coverage claims**: Keep deterministic-fixture, real-model relay, and downstream-client E2E results distinct; none alone proves the others.
  See `docs/testing.md` for test layers and the testbed SOP.

## Release Workflow

0. 所有改动经 `fix/*` / `feature/*` 等短命分支 → PR → Squash merge 回 master（详见 [`docs/internal/git-workflow.md`](docs/internal/git-workflow.md)；master 受保护，禁止直接 push）
1. 确保本地 CI 全部通过（pre-push hook 自动检查）
2. 更新 `CHANGELOG.md`（按 Keep a Changelog 格式；**必须包含 `## [vX.Y.Z]` 节**，Release 说明从该节提取）；同步 `web/package.json` 的 version 字段
3. 发布助手：在干净且 HEAD 等于 `origin/master` 的 checkout/worktree 运行 `bash scripts/release.sh X.Y.Z`（校验 CHANGELOG 节、`web/package.json` 版本后打 annotated tag 并推送）；仅 SemVer tag 触发发布
4. Tag push 触发单一管道 `.github/workflows/main.yml`：全量 12 项检查通过 → 推送 `ghcr.io/deliciousbuding/metapi-go:X.Y.Z`（amd64+arm64，provenance+SBOM）→ 5 平台二进制附件 + checksums + 二进制冒烟 → 创建 draft GitHub Release（body 取自 CHANGELOG 对应节；维护者复核后发布）

**版本号**：`vMAJOR.MINOR.PATCH`（SemVer 2.0）；合入不等于稳定发布，默认积累修复后做候选验收，由维护者明确决定发版时机和版本号（不自动推断、不按周/月强制发布）；安全问题或严重回归可明确走 hotfix，决策与 1.0 就绪标准见 [`docs/internal/git-workflow.md` §6.1](docs/internal/git-workflow.md)。

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
**Public/private boundary:** Keep product code, public contracts, reusable tests, and sanitized fixtures in this repository.
Private deployment/runbooks, host/image pins, and raw runtime/acceptance evidence belong in private documentation, not the published tree.
Credentials belong in an external secret store, never in Git (including private repos) or `.local/`.
Local-only drafts/handoffs must be ignored and do not replace task or runtime SSOT. Public deployment notes: `docs/deployment.md`.
**Honesty:** Prefer 501 / documented residual over stub theater. Do not claim cluster-wide sticky or WS product without the matching milestone.

## Related References

- TS source: original TypeScript Metapi repository, checked out separately when parity work needs it.
- Gateway fork: private deployment, not part of this public repo.
- Ops skill: operator-local reference only. Do not publish private filesystem paths.
