# UI/UX 与动线审计 — 多 Agent 对照（New API × All API Hub）

**Last updated**: 2026-08-18

> 4 个审计 agent 的对照结论聚合（动线 / 视觉 / 交互 / 功能对标）+ 实施进展。
> 对标基线：New API v1.0.0-rc.24、All API Hub 上游 #1290、Metapi-Go v0.13.0。
> 本文件保留 2026-08 审计证据，不是当前 backlog；开放结果只在 [`../progress/MASTER.md`](../progress/MASTER.md) 维护。

## 本轮已实施（2026-08-14）

### P0 — 分发端（最大产品缺口）

- **客户端接入导出对话框**：`web/src/components/common/credential-export-dialog.tsx`（新）
  - 接入地址 + Key 复制（复用后端已有 `GET /api/downstream-keys/{id}/export`）
  - Cherry Studio / CC Switch（codex/claude/gemini/cursor）一键深链导入，附 JSON 复制兜底
  - env 变量 / 通用 JSON profile 复制
  - 入口：Settings → Downstream → API Keys 每行「接入」按钮（`keys-section.tsx`）
- **完成动线断链**：建完 token 路由的完成 toast CTA 从「回 dashboard」改为「接入客户端」→
  直达 `/settings/downstream`（`route-completion-toast.tsx`）

### P0 — 视觉（审计 A1/A2/A3/A4）

- 修复 6 处 `text-*-foreground` 在 `/10` 透明底上的误用（双主题下文字不可读）：
  announcement-banner ×2、availability ×2、overview ×1、database-section ×1
- `ui/badge.tsx` 增加 `success` / `info` 软徽章变体（对齐 warning 配方）
- 站点 active 状态徽章从实心主色块改为 success 软徽章（语义统一）
- 浅色主题 success/info/warning 明度下调（0.596→0.53 / 0.588→0.53 / 0.681→0.62），
  修复 12px 软徽章文字对比度 ≈3:1 低于 AA 的问题

### P0 — 交互（倍率行内编辑安全网）

- `rates-section.tsx`：移除 refetch 静默丢 draft 的 effect；提交防重（pending 守卫）；
  空值/非数字拦截（不再静默写 0）；编辑中 pending 禁用输入；编辑按钮 aria-label

### P1 快赢

- **404 / 错误边界**：`not-found-page.tsx` + `error-page.tsx`，挂到 router
  `defaultNotFoundComponent` / `defaultErrorComponent`（此前白屏）
- **深链过期页码钳位**：`use-url-table-state.ts` 新增 `ensurePageInRange`，接线
  models / sites / channels / oauth 四页（`?page=20` 深链不再渲染空表 + 误导空态）
- **导入向导脏确认**：`import-wizard-dialog.tsx` 有输入时关闭先确认（全站最后一个无守卫表单）
- **站点状态徽章**、**对比度 token** 见上

## 审计快照（截至 2026-08-18）

> “未承诺”表示审计时未实施，仅保留为历史观察，不是待办。下表只有“批量延迟对比”的剩余部分已进入 [`../progress/MASTER.md`](../progress/MASTER.md)；其他观察需要新的用户需求或缺陷证据才会重新立项。
>
> **2026-08-18 知识卫生收口**：原 P1 #1（checkin/oauth 死复选框）、#2（排序清除）、#4（Stat 卡升级）三项经核实已交付，从下表移除。证据：`checkin-page.tsx`/`oauth-page.tsx` 均为 `enableRowSelection: false`；`data-table/core/column-header.tsx` 已有「Default order」菜单调 `clearSorting()`；`dashboard/components/stat-card.tsx` 已有 IconBadge + 三档 tone + 明细子格（注释标注 audit upgrade）。原编号 #3/#5/#6 重排为 #1/#2/#3。P2 #5（导入向导 focus-first-invalid）经核实 e1991ef 已落地 `markInvalidAndFocusFirst` + `aria-invalid` + per-field clear，本日补 4 个回归用例（`import-wizard-dialog.test.tsx` 共 9 例），状态改「已交付（代码）」；`account-form-dialog.tsx` 等其他表单仍未做，保留为观察。

### P1

| #   | 状态           | 项                           | 位置                                                                | 备注                                                                                              |
| --- | -------------- | ---------------------------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| —   | 已交付 #658    | 全局搜索（⌘K）               | 后端 `/api/search` + `searchApi`                                    | Command Palette 已接入 header，搜站点/账号/Token/模型/日志并跳转                                  |
| —   | 已交付 #659    | 今日快照横条                 | `dashboard/sections/overview/overview-section.tsx`                  | 快照横条上线（余额+环比 / 签到 / 未读告警 / 可用性红绿点），`todayCheckin`/`getAttention` 已接    |
| —   | 已交付 #659    | attention 上提首页           | `availability-section.tsx`                                          | 首页快照横条提供 attention 直达，原「藏在第 4 个 Tab」已收口                                      |
| —   | 已交付 #660    | 告警富化                     | `service/alert/alert.go`                                            | 3 条核心告警消息富化（受影响路由 + 替代站点 + 直达链接）                                          |
| —   | 已交付（代码） | 批量操作反馈                 | `accounts-page.tsx` / `accounts/api.ts`                             | `useBatchUpdateAccounts` 已 toast `bulkPartial`（success/failed/items）；证据见 `accounts/api.ts` |
| 1   | 未承诺         | 徽章配方收敛                 | overview/availability/checkin/accounts/routes/channels 内联状态徽章 | success/info 变体已存在，但未做全量迁移                                                           |
| 2   | 已交付（代码） | RealtimeSparkline 图表 token | `availability-section.tsx`                                          | 已改 `bg-chart-1/70`；`useChartColors` 已随 VChart 移除                                           |
| 3   | 未承诺         | 行内高频操作免菜单化         | `accounts-columns.tsx`                                              | 审计建议：高频动作采用行级 pending + 成功反馈                                                     |

### P2

| #   | 状态              | 项                                        | 位置                                                 | 备注                                                                    |
| --- | ----------------- | ----------------------------------------- | ---------------------------------------------------- | ----------------------------------------------------------------------- |
| 1   | 已交付 #744/#751  | URL 状态同步两套机制统一                  | accounts/checkin/token-routes/proxy-logs             | 页面统一 `useSearch` + `navigate`，loader 只读 `location.searchStr`     |
| 2   | 已交付 #758       | 图标族统一（HugeIcons × lucide 同屏）     | ui 原语 vs feature 层                                | ui 原语用 HugeIcons 免费层，业务层沿用 lucide，约定写入 `web/AGENTS.md` |
| 3   | 已交付 #757       | 文档漂移销项                              | DESIGN.md（9 vs 10 预设）、标题 400 vs 500           | 预设数、标题字重、状态色、图表栈和 CTA 对比度已对齐实现                 |
| 4   | 未承诺            | 圆角层级 / header 高度双来源              | `data-table-view.tsx`、`app-header.tsx`、`theme.css` | 审计观察，未升格为重构任务                                              |
| 5   | 已交付（代码）    | 导入向导校验失败聚焦首错字段              | `import-wizard-dialog.tsx`                           | e1991ef 落地 `markInvalidAndFocusFirst` + `aria-invalid` + per-field clear，9 用例覆盖（`import-wizard-dialog.test.tsx`）；其他表单（`account-form-dialog.tsx` 等）仍为观察 |
| 6   | 未承诺            | 移动端首帧侧栏 / settings 375px 导航      | `use-mobile.tsx` / `settings-sidebar.tsx`            | 审计观察，按复现证据再立项                                              |
| 7   | 未承诺            | 骨架屏 shimmer / 表格骨架差异化           | `index.css` / `table-skeleton.tsx`                   | 纯 polish，不进入当前计划                                               |
| 8   | 部分交付 → MASTER | Playground 会话化 + 模板库 + 批量延迟对比 | `model-tester/`                                      | #662 已交付会话化 + 模板库；批量延迟对比由 MASTER Wave 2–3 接管         |

### 明确不做（定位边界，见 benchmark.md）

多租户计费/钱包/订阅/支付、社交 OAuth 登录、多实例 sticky 共享改造。

## 审计证据口径

- 动线审计：5 条动线（首次接入 / 客户端分发 / 日常巡检 / 故障处理 / 模型测试），
  全部引用文件经验证；核心发现「建完路由之后系统撒手不管」已收口（#657 接入对话框 + 完成动线 toast 改接 `/settings/downstream`）。
- 视觉审计：25 项问题（3 P0 / 10 P1 / 12 P2），已修复 3 P0（`text-*-foreground` 透明底误用 ×6、软徽章 success/info 变体、站点 active 徽章语义统一）+ 2 P1（站点 active 软徽章、12px 软徽章对比度 AA：浅色 success/info/warning 明度下调）。
- 交互审计：1 P0 / 8 P1；已修复倍率行内编辑安全网、页码钳位、导入脏确认和 404/错误边界。其余内容保留为审计观察。
- 功能对标：已落地客户端一键导出 #657、全局搜索 #658、首页今日快照 #659、告警富化 #660；后续承诺以 MASTER 为准，不从本审计表直接派生。
