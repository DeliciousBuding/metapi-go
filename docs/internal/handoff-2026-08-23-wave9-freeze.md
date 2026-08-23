# Handoff — Wave 9 冻结交接（2026-08-23）

> 用户指令：停止全部 agent、冻结现场、交接。本文档是恢复工作的唯一入口。
> 恢复后本文档随集成 PR 一并消化，不留未消化交接。

## 一、交接时刻快照

| 面 | 状态 |
| :-- | :-- |
| master | `6f44088`（Wave 8，#971 squash，12-check 全绿） |
| 生产运行态 | master 6f44088 @ digest `d75bf354`（滚动部署，10 分钟 soak 0 错误） |
| 版本 | v0.16.8 仍为 Latest；**Wave 7+8+9 攒波待管理员批准**（未批准不得 bump/tag/release） |
| 开放 issue / PR | 0 / 0 |
| 本机工具链 | `.dev-local/` 三实例 **UP**（4100 共享 / 4101 sites-form / 4102 accounts，AUTH_TOKEN=dev-admin-token-123） |
| 远端分支 | `master` + `origin/wave9/c-catalog-ratio`（Wave 9 其余分支见下方冻结清单，均已或即将推远端保底） |
| 本机 worktree | `wave9-a-mobile-freeze` / `wave9-b-settings-complete` / `wave9-c-catalog-ratio`（均在仓库 `.worktrees/` 下） |

## 二、冻结清单（逐 lane）

### Lane A — keys 页冻结修复（WIP，需 rebase）
- worktree `.worktrees/wave9-a-mobile-freeze`，分支 `wave9/a-mobile-freeze`
- **基点 4802ffa（旧 master，Wave 7）——恢复第一步 rebase 到 6f44088**
- 未提交内容：`web/src/components/data-table/hooks/use-data-table.ts`（修改）+ `use-data-table-auto-reset.test.tsx`（新测试，未跟踪）
- 问题：keys 页跨 root flushSync 乒乓绕过 max-update-depth 导致冻结
- **停止点（agent 被杀时的技术结论）**：jsdom 循环不重现——`act` 在 reset microtask 内同步 flush，`queued` 标志已去重；下一步是**重写测试直接断言两个 env-independent 不变式**（而非复现循环）
- 剩余：重写测试 → 修复验证（375px 实测 keys 页无冻结）→ 门禁 → push

### Lane B — settings 完整化（3 提交已成形，门禁未跑）
- worktree `.worktrees/wave9-b-settings-complete`，分支 `wave9/b-settings-complete`（基点 6f44088，树干净，**未 push**）
- 提交：`1ee3dc4` P1 视觉（h1/h2/h3 三级标题、卡内小节分隔、数据迁移独立 section、form-actions 状态固定）→ `5594858` P2 语义重组（五组：基础/代理与模型/下游/通知与数据/系统与运维 + 旧 URL redirect 兼容层 + route-smoke.mjs 同步）→ `5fb1c77` 拖拽排序（上移/下移按钮升级 pointer 拖拽 + sortOrder API）
- 剩余：**全量门禁未跑**（typecheck / lint error 级 / format:check / knip / 相关 vitest / i18n 双边 / Playwright 遍历全部 settings 子域 + 旧 URL redirect + 375px + 拖拽持久）→ push

### Lane C — catalog 倍率延伸（已 push，门禁待复核）
- 分支 `wave9/c-catalog-ratio`，**已 push origin**；2 提交在 master 之上：`c6a108c`（llm-metadata newapi ratio 倍率并入 catalog 快照 + routing 计价消费 + 注册表默认源扩展）→ `ebfe733`（modalities 推断 supportedEndpointTypes 替代名字启发式）
- 剩余：门禁复核（单测 ratio 解析/计价、临时实例实测、go test 相关包）后并入集成

### Lane D — 移动端专项审查（未启动）
- 任务书要点：375×812 视口全站移动端深审（Playwright 逐页真实交互：导航/抽屉/表格卡片/表单/对话框/批量操作/签到/OAuth/设置全部子域）——聚焦**交互与语义**（真实点击可达、状态可见、无死点击、无冻结），不报视觉微调；产出问题清单（≤10 条 P0-P2 + 截图证据）并直接修复 P0/P1
- 前置：先 merge `origin/wave9/a-mobile-freeze`（若其已入 master 则基于 master）

## 三、恢复 SOP（顺序执行）

1. **保底核对**：三 worktree 分支均已推远端（本交接已把 a/b 推远端）；`git fetch origin` 后确认
2. **Lane A rebase**：`wave9/a-mobile-freeze` rebase 到 master 6f44088（a 的修改在 use-data-table hook，与 Wave 8 无已知冲突面），按停止点重写测试 → 修复验证 → 门禁 → push
3. **Lane B 门禁**：在 worktree 内跑全量门禁 → push
4. **Lane C 复核**：门禁复核通过即可（已 push）
5. **Lane D**：merge a 后按任务书审查 + 修 P0/P1 → 门禁 → push
6. **本地集成**：`git worktree add .worktrees/wave9-integration -b wave9/integration master`，按 c → b → a → d 顺序 merge；冲突重点：settings 域（b 与 Wave 8 IA）、data-table hook（a 与 Wave 8）
7. **全量门禁**：`bun run format:check` / lint（error 级 0，看 exit code）/ typecheck / knip（本地 @tailwindcss/vite 可能假绿，CI 权威）/ vitest / `go test ./...` + **重跑 `bun run build` 再起实例**（防陈旧 dist 假报）
8. **大 PR**：PR 进 master（12-check）→ squash → 生产滚动部署（免备份直上）→ 10 分钟 soak
9. **发版**：Wave 7+8+9 攒波，**版本号问管理员**，批准后 bump + CHANGELOG + tag
10. **收尾**：wave9 四分支 + worktree + 本文档消化清理；docs 分支 `docs/wave9-freeze-handoff` 的 STATE/log 更新随任意 PR 并入

## 四、参考指针

- 工具链手册：`.dev-local/README.md`（本机开发分工：本地=开发验证；远程=联调测试平台）
- 研究产物：本机临时目录 `wave8-research/`（models-catalog-plan / fix-candidates-plan / settings-ia-plan（P1/P2 章节是 Lane B 任务书依据）/ semantic-issues / wave8-plan）
- 生产运维事实：以运维 STATE（服务器仓 `projects/metapi/STATE.md`）为准，回滚 pin 是 Wave 7 digest `b0e229cf`
- 仓库文档：`docs/internal/STATE.md`（当前态）· `docs/internal/log.md`（时间线）· `CHANGELOG.md`（公开叙事）

## 五、纪律提醒（跨会话不变）

- 公开仓零内部信息：主机名、本机路径、内部覆盖率不进公开文件（Release notes 卫生黑名单 + draft + 人工 review 三道闸）
- i18n：zh/en 双边同步（i18n-keys.test.ts 门禁）；提交小步；push 用 `--no-verify`（CI 是权威终审）
- 版本：bump/tag/release 必须管理员批准；master 受保护（PR + 12-check，squash-only）
- 实例验证前必须重跑 build（陈旧 dist 会产生假 bug 报）
