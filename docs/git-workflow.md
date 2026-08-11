# Git Workflow — 分支策略与协作规范

**最后更新**：2026-08-11

> 本文定义 metapi-go 的分支模型、保护规则、PR 流程与提交规范。规则已在 GitHub 仓库实际落地（见下文"已启用设置"），不依赖个人自觉。

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
| 必选状态检查 | CI 全部 11 个 job（lint / vet / vulncheck / mod-verify / secret-scan / docs-hygiene / frontend / test-sqlite / test-pg / build / docker-build） |
| 强制管理员 | ✅ 管理员同样受保护（防呆，紧急时可在仓库设置临时关闭） |
| 要求 approve | ❌ 不强制（个人项目，自己合并自己的 PR） |
| 合并方式 | **Squash only**（仓库级设置：关闭 merge commit 与 rebase merge） |
| 线性历史 | 由 Squash-only 保证 |

> 注意：必选状态检查与 CI 的 `paths-ignore` 互斥——任何 PR 都会触发全量 CI（含纯文档 PR），否则必选检查会永久 pending 卡住合并。

## 4. PR 流程

1. 从 master 切分支：`git checkout -b fix/xxx`
2. 本地门禁全绿后提交（Conventional Commits 风格，见 §5），`git push -u origin fix/xxx`
   - push 前 `.githooks/pre-push` 自动跑本地 CI（vet + 前端门禁 + race）；紧急跳过 `git push --no-verify`
3. 开 PR（`gh pr create`，模板自动填充），base = master
4. CI 11 job 全绿（含 PG 集成测试）后 Squash merge
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

## 6. Release 流程（不变）

1. 本地 CI 全绿 → `CHANGELOG.md` 更新 → `git tag -a vX.Y.Z` → `git push origin vX.Y.Z`
2. Tag 推送触发 `cd.yml`（release-gate 全量验证 + 构建推送 `ghcr.io/deliciousbuding/metapi-go`）与 `release.yml`（创建 GitHub Release）
3. Tag 只从 master 打（master 即发布线）

## 7. 紧急修复

- 生产问题需要绕过 PR 时：临时在 GitHub 仓库设置关闭 master 保护 → 直接修复 → 重新开启。**不得**长期保留关闭状态。
- 本地 CI 可 `git push --no-verify` 跳过（由 GitHub Actions 兜底），但这是例外不是默认。

## 8. 已启用设置速查

| 设置 | 位置 | 值 |
|:-----|:-----|:---|
| 合并策略 | 仓库 Settings → General | Allow squash merging only |
| master 保护 | 仓库 Settings → Branches | 见 §3 |
| PR 模板 | `.github/pull_request_template.md` | 自动填充 |
| CI | `.github/workflows/ci.yml` | PR + master push 全量 11 job |
| CD | `.github/workflows/cd.yml` | master push + tag 发布镜像 |
| Release | `.github/workflows/release.yml` | tag 建 Release |
| 本地门禁 | `.githooks/pre-push` | push 前自动运行 |

相关文档：[`deployment.md`](deployment.md)（部署）· [`STATE.md`](STATE.md)（当前状态）· `AGENTS.md`（工程规则）
