# UI/UX 与动线审计 — 多 Agent 对照（New API × All API Hub）

**Last updated**: 2026-08-14

> 4 个审计 agent 的对照结论聚合（动线 / 视觉 / 交互 / 功能对标）+ 实施进展。
> 对标基线：New API v1.0.0-rc.24、All API Hub 上游 #1290、MetAPI-Go v0.12.0。
> 产品级结论见 [`../benchmark.md`](../benchmark.md)；本文件是执行层清单。

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

## 剩余 backlog（已定级，未实施）

### P1

| 项 | 位置 | 备注 |
|----|------|------|
| 全局搜索（⌘K） | 后端 `/api/search` + `searchApi` 已完备，前端 CommandDialog 闲置 | 新建 SearchModal + header 入口，搜站点/账号/Token/模型/日志并跳转 |
| 今日快照横条 | `dashboard/sections/overview/overview-section.tsx` | 余额+环比 / 今日签到收益 / 未读告警 / 可用性红绿点；`todayCheckin`/`getAttention` API 已就绪 |
| attention 上提首页 | `availability-section.tsx` | 最有价值的待办列表目前藏在第 4 个 Tab |
| 告警富化 | `service/alert/alert.go` 3 处 | 消息加受影响路由 + 替代站点 + 直达链接；program-logs 支持 ?related 过滤 |
| 批量操作反馈 | `accounts-page.tsx` / `accounts/api.ts` | 成功/部分失败 toast（failedItems 现被丢弃） |
| checkin/oauth 死复选框 | `checkin-page.tsx` / `oauth-page.tsx` | 有选择列无批量操作 → 补操作或删列 |
| 排序清除 | `data-table/core/column-header.tsx` | 菜单加「默认顺序」或表头三态循环 |
| 徽章配方收敛 | overview/availability/checkin/accounts/routes/channels 内联状态徽章 | 迁移到 badge success/info 变体（本轮只加了变体，未全量迁移） |
| Stat 卡升级 | `dashboard/components/stat-card.tsx` | IconBadge + 三档 tone + 明细子格（New API C1/C2 借鉴） |
| RealtimeSparkline 图表 token | `availability-section.tsx` | `bg-primary/70` → `--chart-1`/useChartColors |
| 行内高频操作免菜单化 | `accounts-columns.tsx` | New API 渠道行操作模式（行级 pending + 成功 toast） |

### P2

| 项 | 位置 |
|----|------|
| URL 状态同步两套机制统一 | `use-url-table-state.ts` vs 一次性 read + replaceState（accounts/checkin/token-routes/proxy-logs） |
| 图标族统一（HugeIcons × lucide 同屏） | ui 原语 vs feature 层 |
| 文档漂移销项 | DESIGN.md（9 vs 10 预设）、a11y-checklist §4.1 对比度债务已过时、标题 400 vs 500 |
| 圆角层级 / header 高度双来源 | `data-table-view.tsx`（rounded-lg vs 卡片 rounded-xl）、`app-header.tsx` + `theme.css` token |
| 导入向导校验失败聚焦首错字段 | `account-form-dialog.tsx` 等 |
| 移动端：首帧侧栏闪烁 / settings 375px 导航 | `use-mobile.tsx` / `settings-sidebar.tsx` |
| 骨架屏 shimmer / 表格骨架行宽差异化 | `index.css` / `table-skeleton.tsx` |
| Playground 会话化 + 模板库 + 批量延迟对比 | `model-tester/`（对标 New API Playground，benchmark P1-7） |

### 明确不做（定位边界，见 benchmark.md）

多租户计费/钱包/订阅/支付、Electron 桌面版、社交 OAuth 登录、多实例 sticky 共享改造。

## 审计证据口径

- 动线审计：5 条动线（首次接入 / 客户端分发 / 日常巡检 / 故障处理 / 模型测试），
  全部引用文件经验证；核心发现「建完路由之后系统撒手不管」已由本轮 P0 修复收口。
- 视觉审计：25 项问题（3 P0 / 10 P1 / 12 P2），本轮修复 3 P0 + 1 P1（站点徽章）。
- 交互审计：1 P0 / 8 P1，本轮修复 P0 + 页码钳位 + 导入脏确认 + 404 边界。
- 功能对标：8 项差距，本轮落地「客户端一键导出」；6/8 项后端数据已完备、只需前端。
