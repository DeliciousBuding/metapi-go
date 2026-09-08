# Git Workflow — 分支策略与协作规范

**最后更新**：2026-09-08

> 本文定义 metapi-go 的分支模型、保护规则、PR 流程、提交规范与**版本号策略**。规则已在 GitHub 仓库实际落地（见下文"已启用设置"），不依赖个人自觉。

## 1. 分支模型：GitHub Flow（单主线）

```
master (唯一长期分支，受保护，持续集成)
   └── feature/<name> / fix/<name> / chore/<name> / docs/<name>  (短命分支)
          └── 开发 → 提交 → push → 开 PR → CI 全绿 → Squash merge 回 master
```

- **master**：唯一长期分支，所有代码的最终归宿；合入表示通过合并门禁，不表示已完成候选验收或已稳定发布。
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
| 强制管理员 | ✅ 管理员同样受保护；hotfix 仍走 PR / CI，不通过关闭保护绕过 |
| 要求 approve | ❌ 不强制（个人项目，自己合并自己的 PR） |
| 合并方式 | **Squash only**（仓库级设置：关闭 merge commit 与 rebase merge） |
| 线性历史 | 由 Squash-only 保证 |
| 要求分支最新（strict） | ❌ **已关闭（2026-08-14）**——仍保留 12 项必检 + squash 线性历史；关闭后多个并行分支可各自 CI 绿了直接合入，无需每个 PR 串行 rebase 重跑全量 CI |

> 注意：必选状态检查与 CI 的 `paths-ignore` 互斥——任何 PR 都会触发全量 CI（含纯文档 PR），否则必选检查会永久 pending 卡住合并。
> 注意：关闭 strict 后，合并时仍以「12 项必检全绿」为准；squash 合并若有真实文件冲突仍会被 GitHub 拦截。并行开发时优先让各分支文件面不重叠。

## 4. PR 流程

1. 从 master 切分支：`git checkout -b fix/xxx`
2. 本地门禁全绿后提交（Conventional Commits 风格，见 §5），`git push -u origin fix/xxx`
   - push 前全局 hook-kit 链式执行 `.githooks/pre-push-project`（`go build` + `go vet` + 完整前端门禁 + WSL-backed `-race`）；`.githooks/pre-push` 仅供未安装 hook-kit 的贡献者作为兼容入口。`git push --no-verify` 只用于已确认的紧急例外；hotfix 不据此默认跳过本地门禁，远端 CI 仍必须通过。
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

1. 默认先积累修复，在 GitHub issue / PR 中关联受影响的用户任务与回归证据；维护者选定候选 commit 后，按 [`docs/testing.md`](../testing.md) 核对历史问题及关键任务。候选验收不要求先打 tag，也不为每次合入修改版本号。
2. 验收记录绑定最终源码（commit + 未提交 diff）和实际二进制或镜像摘要，分别写明 fixture、真实模型、下游客户端结果与未验证项。源码或产物改变后重跑受影响检查，不把旧构建的成功沿用到新候选。
3. 维护者审阅范围、剩余风险及证据后，明确决定是否发布和版本号。决定发布后才更新 `CHANGELOG.md`（必须包含 `## [vX.Y.Z]` 节）与 `web/package.json` version，经 PR 合入 master；该 PR 合入后，以最终 master commit 重新构建并补跑受影响的候选验收，不能沿用准备前的产物摘要。
4. 仅在明确发布决定后，从选定的 master 提交运行 `bash scripts/release.sh X.Y.Z`。助手负责一致性校验、annotated tag 与推送，不负责决定发版。tag、包版本与 CHANGELOG 节须一致。
5. SemVer tag 触发 `.github/workflows/main.yml` 的检查及镜像发布，再构建五个平台二进制、checksums 和安装脚本，执行发布二进制冒烟。**新建 GitHub Release 使用 `--draft`**；维护者复核最终附件、验收结果和公开说明（含脱敏与已知限制）后，才发布稳定 Release。

### 构建渠道与发布状态（当前 workflow）

- **master 构建**：成功的镜像发布 job 更新 `latest` 与短 commit SHA 标签，构建版本为 `dev`。这不是稳定发布批准。
- **tag 构建**：镜像标签由 `docker/metadata-action@v6` 的 `{{version}}` / `{{major}}.{{minor}}` 生成（不带 `v`）；该 action 默认 `latest=auto`，也会为符合触发格式的 SemVer tag 生成 `latest`。因此 `latest` 是可变构建标签，不能当作稳定渠道。
- **Release 草稿**：镜像在创建草稿之前已发布；草稿不会隔离或回滚镜像。已有同 tag Release 的重跑只覆盖附件、更新说明，不改变其 draft/published 状态；已发布 Release 不会因重跑重新进入审阅。
- **稳定安装**：选维护者已公开发布的非预发布 Release，并固定对应完整版本镜像标签或 digest；开发验证选具体 commit 构建并记录摘要。不要用 `latest` 或仓库 HEAD 代替验收身份。

## 6.1 版本号与发布节奏

> 本节是版本号与发布节奏的决策来源；工具只做计算或校验，不能替维护者作决定。

格式为 `vMAJOR.MINOR.PATCH`（SemVer 2.0）；1.0 前为 `v0.X.Y`。修复通常使用 PATCH，有主题的功能里程碑使用 MINOR，1.0 及之后的不兼容变更使用 MAJOR；最终选择由维护者根据兼容性和发布范围明确决定，不从合并次数、最新 tag 或脚本默认值自动推断。

**默认积累修复 → 候选验收 → 维护者决定稳定发布**。合入不是发版触发器；不要求每波 bump，不强制周/月定时发布。未通过验收或仍有阻塞时继续修复，不为追版本号而发版。安全问题或严重回归可明确走 §7 的 hotfix。

**1.0 就绪标准**：

- 生产多通道级联证据落地（公开记录只保留脱敏结论）
- API / wire 契约冻结（无计划内的破坏性演进）
- 备份 / 迁移双向路径文档化且有实测覆盖

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

安全问题或严重回归允许维护者明确决定提前发布 hotfix，不必等待其他积累中的修复。记录影响范围、最小修复、回归证据和未验证项；安全细节走私有报告渠道，公开记录需脱敏。

hotfix 不等于绕过 PR、现有 CI 或发布复核，也不自动授权关闭分支保护。依然按 §6 核对最终产物并由维护者决定发布；不能把“紧急”当成跳过验收的默认理由。

## 8. 已启用设置速查

| 设置 | 位置 | 值 |
|:-----|:-----|:---|
| 合并策略 | 仓库 Settings → General | Allow squash merging only |
| master 保护 | 仓库 Settings → Branches | 见 §3 |
| PR 模板 | `.github/pull_request_template.md` | 自动填充 |
| CI + CD + Release | `.github/workflows/main.yml` | 单一管道：PR / master push / SemVer tag 全量 12 项检查；master push 推送镜像（latest+sha）；SemVer tag：镜像（amd64+arm64）→ 多平台二进制 + 新建 draft Release；已有 Release 重跑不改 draft/published 状态 |
| 本地门禁 | `.githooks/pre-push-project` | 全局 hook-kit 在 push 前链式运行（build + vet + 完整前端 + WSL-backed race）；`.githooks/pre-push` 是 standalone 兼容入口 |

相关文档：[`deployment.md`](../deployment.md)（部署）· [`CHANGELOG.md`](../../CHANGELOG.md)（版本叙事）· `AGENTS.md`（工程规则）；当前状态以 GitHub issues/releases 为准。
