# Git Workflow — 分支策略与协作规范

**最后更新**：2026-08-20

> 本文定义 metapi-go 的分支模型、保护规则、PR 流程、提交规范与**版本号策略**。规则已在 GitHub 仓库实际落地（见下文"已启用设置"），不依赖个人自觉。

## 1. 分支模型：GitHub Flow（单主线）

```
master (唯一长期分支，受保护，随时可发布)
   └── feature/<name> / fix/<name> / chore/<name> / docs/<name>  (短命分支)
          └── 开发 → 提交 → push → 开 PR → CI 全绿 → Squash merge 回 master
```

- **master**：唯一长期分支，所有代码的最终归宿；master 的 HEAD 永远处于"可发布"状态。
- **功能分支**：所有工作（无论大小）在短命分支上完成，命名 `feature/`、`fix/`、`chore/`、`docs/` 前缀 + kebab-case 描述。
- **不做的事**：不引入 `dev`/`develop`/`release` 长期分支（单人多仓不需要 GitFlow 的复杂度）；不在 master 直接提交（保护规则禁止）。

## 2. 分支命名

| 前缀 | 用途 | 示例 |
|:-----|:-----|:-----|
| `feature/` | 新功能 | `feature/model-tester` |
| `fix/` | bug 修复 | `fix/url-sync-nav` |
| `perf/` | 性能优化 | `perf/dashboard-lazy-load` |
| `refactor/` | 重构（行为不变） | `refactor/search-params` |
| `chore/` | 工程维护（CI/依赖/清理） | `chore/ci-git-workflow` |
| `docs/` | 文档 | `docs/git-workflow` |

## 3. master 分支保护（标准强度）

已在 GitHub 启用（`gh api` 设置）：

| 规则 | 值 |
|:-----|:---|
| 要求 PR | ✅ 禁止直接 push，任何改动必须经 PR 合入 |
| 必选状态检查 | CI 全部 12 个 job（lint / vet / vulncheck / mod-verify / secret-scan / docs-hygiene / frontend / a11y / test-sqlite / test-pg / build / docker-build） |
| 强制管理员 | ✅ 管理员同样受保护（防呆，紧急时可在仓库设置临时关闭） |
| 要求 approve | ❌ 不强制（个人项目，自己合并自己的 PR） |
| 合并方式 | **Squash only**（仓库级设置：关闭 merge commit 与 rebase merge） |
| 线性历史 | 由 Squash-only 保证 |
| 要求分支最新（strict） | ❌ **已关闭（2026-08-14）**——仍保留 12 项必检 + squash 线性历史；关闭后多个并行分支可各自 CI 绿了直接合入，无需每个 PR 串行 rebase 重跑全量 CI |

> 注意：必选状态检查与 CI 的 `paths-ignore` 互斥——任何 PR 都会触发全量 CI（含纯文档 PR），否则必选检查会永久 pending 卡住合并。
> 注意：关闭 strict 后，合并时仍以「12 项必检全绿」为准；squash 合并若有真实文件冲突仍会被 GitHub 拦截。并行开发时优先让各分支文件面不重叠。

## 4. PR 流程

1. 从 master 切分支：`git checkout -b fix/xxx`
2. 本地门禁全绿后提交（Conventional Commits 风格，见 §5），`git push -u origin fix/xxx`
   - push 前全局 hook-kit 链式执行 `.githooks/pre-push-project`（`go build` + `go vet` + 完整前端门禁 + WSL-backed `-race`）；`.githooks/pre-push` 仅供未安装 hook-kit 的贡献者作为兼容入口；紧急跳过 `git push --no-verify`
3. 开 PR（`gh pr create`，模板自动填充），base = master
4. CI 12 job 全绿（含 PG 集成测试）后 Squash merge
5. 合并时把 PR 标题改写为最终提交信息（符合 Conventional Commits）
6. 删除已合并分支（`gh pr merge --delete-branch` 自动处理；若远端分支残留——如合并时本地有未提交改动——手动 `git push origin --delete <branch>`）

## 5. 提交信息规范（Conventional Commits）

```
<type>(<scope>): <subject>
```

- **type**：`feat` / `fix` / `perf` / `refactor` / `chore` / `docs` / `polish` / `state`
- **scope**（可选）：`web`（前端）、`store`（DB 层）、`proxy`、`handler`、`scheduler`、`platform`、`service`、`auth`、`config`、`docs`、`release` 等
- **subject**：一句话描述变更原因（why），中文或英文均可；Squash 时沿用 PR 标题
- 示例：`fix(web): URL-synced tables round-trip sort params without JSON noise`

## 6. Release 流程

1. 更新 `CHANGELOG.md`（**必须包含 `## [vX.Y.Z]` 节**——Release 说明从该节提取，缺节会失败）并同步 `web/package.json` 的 version 字段，经 PR 合入 master
2. 运行发布助手 `bash scripts/release.sh X.Y.Z`（校验 CHANGELOG 节、`web/package.json` 版本、master 与远端同步后打 annotated tag 并推送）；等价手动流程：`git tag -a vX.Y.Z` → `git push origin vX.Y.Z`
3. Tag 推送触发单一管道 `.github/workflows/main.yml`：全量 12 项检查通过 → 推送 `ghcr.io/deliciousbuding/metapi-go`（**amd64 + arm64**，provenance + SBOM）→ 构建 5 平台二进制附件（linux/darwin/windows × amd64/arm64）+ `checksums.txt` → 冒烟 `metapi-linux-amd64 --version` → 创建 GitHub Release（body 取自 CHANGELOG 对应节）
4. Tag 只从 master 打（master 即发布线）；SemVer 格式 `vX.Y.Z`（其他 tag 不触发发布）
5. 版本号经 `-ldflags -X .../internal/version.Version` 注入二进制（`metapi --version` 可查）；tag 与 `web/package.json` 版本不一致会在发布前失败
6. 拿不准下一个版本号时：`bash scripts/next-version.sh`（只读，从最新 tag 打印 patch/minor/major 三个候选，默认走 patch，见 §6.1）

## 6.1 版本号策略（SemVer · patch-first 节奏）

> **本节是版本号决策的唯一权威来源**。AGENTS.md / CONTRIBUTING.md 只做引用，不重复定义。

格式 `0.MAJOR.MINOR.PATCH`（SemVer 2.0）。1.0 之前主版本号恒为 `0`；下文「中间位」指 `0.X.Y` 的 `X`，「最后一位」指 `Y`。

**节奏：默认 patch-first —— 最后一位持续迭代。**

| 段 | 何时 bump | 说明 |
|:---|:---|:---|
| **最后一位（PATCH）** | **默认，每波都动** | 每波合入 master 且含用户可见变更 → 立即 bump 最后一位并发布。`0.16.2 → 0.16.3 → 0.16.4 …` 小步、高频，最后一位一直往前走。 |
| **中间位（MINOR）** | 克制，里程碑才动 | 仅当一次交付构成一个有主题的里程碑（新子系统 / 新界面大类 / 成体系的交付波）时 bump 中间位并把最后一位归零。不为单点改动 bump。 |
| **第一位（MAJOR）** | 1.0 及之后 | 1.0 前恒 `0`；只有 1.0 落地与其后的不兼容 API 变更才动用。 |

**为什么 patch-first**：1.0 前的软件高频交付，最后一位持续递增表达「master 随时可发布」，同时避免中间位被琐碎改动快速吃满、稀释「里程碑」语义。发版频率由变更驱动，不由日历驱动。

**1.0 就绪标准**（到齐才把第一位从 0 提走）：

- 生产多通道级联证据落地（Evidence closeout · A）
- API / wire 契约冻结（无计划内的破坏性演进）
- 备份 / 迁移双向路径文档化且有实测覆盖

**与发布流程的关系**：选定版本号后，仍走 §6 的 CHANGELOG + `web/package.json` + tag 一致性校验（`scripts/release.sh` / CI release job 强制三者一致）。

## 6.5 处理 Dependabot 升级（别人追升级时的 SOP）

Dependabot 每周一自动开升级 PR（Go / npm / GitHub Actions / Docker），全部走同一套 12 项 CI。处理顺序：

1. **Go 组（patch/minor）**：CI 全绿 → 直接 squash merge。breaking major 自动关闭，需手动迁移 PR。
2. **GitHub Actions**：patch/minor 已分组为一个 PR；major（如 `setup-go` 5→7）单独开 PR。CI 全绿即可合并；若显示 `BEHIND`，先 `gh pr update-branch <n>` 等 CI 重跑再合并。
3. **npm 组（patch/minor）**：⚠️ **dependabot 不更新 `web/bun.lock`**，`bun install --frozen-lockfile` 必然失败。合并前必须补 lockfile：
   ```bash
   git worktree add .worktrees/fix-frontend-deps -b fix/frontend-deps dependabot/<npm-branch>
   cd .worktrees/fix-frontend-deps/web && bun install      # 重新生成 bun.lock
   bun run typecheck && bun run lint && bun run format:check
   git add web/bun.lock && git commit -m "chore(deps): regenerate bun.lock"
   git push --force origin HEAD:dependabot/<npm-branch>
   ```
   再等 CI 全绿后合并。`oxfmt` 升级可能顺带改格式（`format:check` 失败时跑 `bun run format` 一并提交）。
4. **前端库 major（如 `@tanstack/react-table` 8→9）**：关闭 PR，注明需手动迁移 PR（API 改动 + 重新生成 lockfile），不自动合并。
5. 合并后无需手动重推镜像：master push 会触发单一管道自动推送 `latest` + sha。

## 7. 紧急修复

- 生产问题需要绕过 PR 时：临时在 GitHub 仓库设置关闭 master 保护 → 直接修复 → 重新开启。**不得**长期保留关闭状态。
- 本地 CI 可 `git push --no-verify` 跳过（由 GitHub Actions 兜底），但这是例外不是默认。

## 8. 已启用设置速查

| 设置 | 位置 | 值 |
|:-----|:-----|:---|
| 合并策略 | 仓库 Settings → General | Allow squash merging only |
| master 保护 | 仓库 Settings → Branches | 见 §3 |
| PR 模板 | `.github/pull_request_template.md` | 自动填充 |
| CI + CD + Release | `.github/workflows/main.yml` | 单一管道：PR / master push / SemVer tag 全量 12 项检查；master push 推送镜像（latest+sha）；SemVer tag：镜像（amd64+arm64）→ 多平台二进制 + GitHub Release |
| 本地门禁 | `.githooks/pre-push-project` | 全局 hook-kit 在 push 前链式运行（build + vet + 完整前端 + WSL-backed race）；`.githooks/pre-push` 是 standalone 兼容入口 |

相关文档：[`deployment.md`](../deployment.md)（部署）· [`CHANGELOG.md`](../../CHANGELOG.md)（版本叙事）· `AGENTS.md`（工程规则）；当前状态以 GitHub issues/releases 为准。
