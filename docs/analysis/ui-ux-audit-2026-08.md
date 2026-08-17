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
| 1   | 部分交付（代码）| 徽章配方收敛                 | overview-section.tsx、availability-section.tsx       | #825 机械迁移：3 个手写 `<span>` 徽章 → `<Badge>` 语义变体（`SCHEDULER_STATUS_BADGE`/`SEVERITY_TONE` map `className`→`variant`）。剩余 7 处 `variant='default'+dot` → `variant='success'` 跨 accounts/checkin/routes/channels 需设计决策，按 feature 分批；`failure-reason-badge` 的 network/state 保留（文档化意图） |
| 2   | 已交付（代码） | RealtimeSparkline 图表 token | `availability-section.tsx`                                          | 已改 `bg-chart-1/70`；`useChartColors` 已随 VChart 移除                                           |
| 3   | 已交付（代码）  | 行内高频操作免菜单化         | `accounts-columns.tsx`                              | #824 行内 Enable/Disable 按钮（`Power` + `Loader2` + 每行 pending via mutation `variables`，无全局锁，复用现有 i18n）；refresh/pin/checkin/edit/delete 仍走下拉菜单 |

### P2

| #   | 状态              | 项                                        | 位置                                                 | 备注                                                                    |
| --- | ----------------- | ----------------------------------------- | ---------------------------------------------------- | ----------------------------------------------------------------------- |
| 1   | 已交付 #744/#751  | URL 状态同步两套机制统一                  | accounts/checkin/token-routes/proxy-logs             | 页面统一 `useSearch` + `navigate`，loader 只读 `location.searchStr`     |
| 2   | 已交付 #758       | 图标族统一（HugeIcons × lucide 同屏）     | ui 原语 vs feature 层                                | ui 原语用 HugeIcons 免费层，业务层沿用 lucide，约定写入 `web/AGENTS.md` |
| 3   | 已交付 #757       | 文档漂移销项                              | DESIGN.md（9 vs 10 预设）、标题 400 vs 500           | 预设数、标题字重、状态色、图表栈和 CTA 对比度已对齐实现                 |
| 4   | 已交付（代码）    | 圆角层级 / header 高度双来源              | `app-header.tsx`、`authenticated-layout.tsx`、`layout-error-boundary.tsx` | 圆角半 stale（`data-table-view` 已用 `rounded-lg` token，无双来源）；header 高度半已收口到单一 `--app-header-height` token + 删 2 处冗余 inline re-declaration + 静态守卫 `header-height-ssot.test.ts`（#824） |
| 5   | 已交付（代码）    | 导入向导校验失败聚焦首错字段              | `import-wizard-dialog.tsx`                           | e1991ef 落地 `markInvalidAndFocusFirst` + `aria-invalid` + per-field clear，9 用例覆盖（`import-wizard-dialog.test.tsx`）；其他表单（`account-form-dialog.tsx` 等）仍为观察 |
| 6   | 未承诺            | 移动端首帧侧栏 / settings 375px 导航      | `use-mobile.tsx` / `settings-sidebar.tsx`            | 审计观察，按复现证据再立项                                              |
| 7   | 已交付（代码）    | 骨架屏 shimmer / 表格骨架差异化           | `index.css`、`skeleton.tsx`、`table-skeleton.tsx`    | 唤醒休眠 `--skeleton-highlight` token（`.animate-shimmer` 渐变 + reduced-motion gate，替换 `animate-pulse`）；`table-skeleton` 按 `column.getSize()` 取宽替换固定百分比池；`no-gradients` allowlist 加 `index.css` 例外（#824） |
| 8   | 部分交付 → MASTER | Playground 会话化 + 模板库 + 批量延迟对比 | `model-tester/`                                      | #662 已交付会话化 + 模板库；批量延迟对比由 MASTER Wave 2–3 接管         |

### 明确不做（定位边界，见 benchmark.md）

多租户计费/钱包/订阅/支付、社交 OAuth 登录、多实例 sticky 共享改造。

## 审计证据口径

- 动线审计：5 条动线（首次接入 / 客户端分发 / 日常巡检 / 故障处理 / 模型测试），
  全部引用文件经验证；核心发现「建完路由之后系统撒手不管」已收口（#657 接入对话框 + 完成动线 toast 改接 `/settings/downstream`）。
- 视觉审计：25 项问题（3 P0 / 10 P1 / 12 P2），已修复 3 P0（`text-*-foreground` 透明底误用 ×6、软徽章 success/info 变体、站点 active 徽章语义统一）+ 2 P1（站点 active 软徽章、12px 软徽章对比度 AA：浅色 success/info/warning 明度下调）。
- 交互审计：1 P0 / 8 P1；已修复倍率行内编辑安全网、页码钳位、导入脏确认和 404/错误边界。其余内容保留为审计观察。
- 功能对标：已落地客户端一键导出 #657、全局搜索 #658、首页今日快照 #659、告警富化 #660；后续承诺以 MASTER 为准，不从本审计表直接派生。

## 2026-08-18 多角度复审（PM / 程序员 / 用户）

三个只读 explore 子代理从产品经理、前端工程师、真实用户角度复审 5 条主流动线 + 边缘态 + 首次接入到首次代理请求的完整旅程。引导深链链路（site → account → route → downstream key）经核实完整。下列为**未升格为承诺的观察**（证据），立项须经 [`../progress/MASTER.md`](../progress/MASTER.md) 提升并附 owner + 验收标准。

### 本轮已收口（#828，待合并）

- **Dashboard stat 卡 drilldown**（PM #5 / User #1 partial）：`StatCard` 加 `to` prop → `<Link>`，四张卡接线 `/accounts`/`/sites`/`/checkin`/`/proxy-logs`（S）。
- **接入凭证导出 → 测试请求深链**（User #5）：`credential-export-dialog.tsx` footer 加 secondary 按钮 `<Link to='/model-tester'>`，闭合旅程最后一步 dead-end（S）。

### 剩余观察（按主题）

**动线 dead-end / 缺 CTA**

- `proxy-log-detail-sheet.tsx:143-177` — channel/account/route/token ID 渲染为不可点 `#NNN`；要做 drilldown 需目标页支持 `?channelId=` 等 deep-link 过滤（M）。
- `channel-detail-sheet.tsx:216` — 无 `SheetFooter`，cooldown/breaker_open 通道无「清冷却 / 探测」动作（可复用 `useClearRouteCooldown` 模式）（M）。
- `price-compare-page.tsx:184` — `PriceRow` 无动作列，比价结果不接 route weight 编辑（`/token-routes?model=` deep-link）（S）。
- `keys-section.tsx:296` — downstream key 仅 create/toggle/delete/export，无 edit；改名/调 `maxCost`/`allowedIps` 需删重建（轮换 key 值，破坏所有客户端）（M）。
- `channels-page.tsx:139` — 空态无 CTA（应接 `/accounts` 或触发 `rebuildMutation`）（S）。
- `keys-section.tsx:221` — 空态仅一行文字，无内联「Create」按钮（仅 header 有）（S）。
- `overview-section.tsx:253` — 首次落地（0 site）无 onboarding banner/CTA（M，#828 stat-card drilldown 部分缓解）。

**错误 / 空态处理**

- `keys-section.tsx:190` — `keysQuery` 失败时 `items=[]` 渲染为「无 key」空态而非错误/重试 UI（S）。
- `update-center-section.tsx:85` — `statusQuery` 失败渲染「version: unknown」无可区分错误提示/重试（S）。

**行级 pending + a11y**

- `keys-section.tsx:268` — enable `Switch` 共享一个 mutation，pending 时不禁用（可连点重复 mutate）+ 全行 `aria-label="Enabled"` 非唯一（S）。
- `accounts/api.ts:281` — `useToggleAccountPin`/`useToggleAccountStatus`/`useToggleAccountCheckin` `onSuccess` 只 invalidate 不 toast（status 已由 #824 行内 Loader2 救场；pin/checkin 仍走下拉菜单 fire-and-forget 无反馈）（S）。

**功能 bug（非 polish）**

- `proxy-logs-page.tsx:200` — 「Slow only」`latencyMin`/`latencyMax` 仅客户端过滤当前页，而 `total`/`manualPagination` 用服务端未过滤计数 → 分页与总数不一致，过滤结果页间漂移；`ProxyLogsQuery`（`lib/api/types.ts:479`）无 latency 字段，需后端支持或移除该过滤（M）。

### 旅程链路核实结论

`?siteId=…&create=1` 深链：`resolveDeepLinkPreselect` 校验 site 后预选 + 一次性消费 + `navigate(…, replace)`；`buildAccountsHref` 保留 transient 参数直至 effect strip；`showAccountCreatedToast` → `/token-routes?accountId=…&siteId=…` → `buildChannelDraftSeed(chainContext?.accountId)`。~~链路完整，唯一断点是 `route-form-dialog.tsx:423`：`accountOptions.length === 0` 时（未跑 Rebuild 模型发现）silently 隐藏 `channelDrafts` 段，深链 seed 的 account 不可见~~ 2026-08-18 已修复：channelDrafts 段恒渲染，空态显示「模型扫描后发现账号」提示 + 内联 Auto-rebuild 按钮（rebuild invalidate candidates 查询前缀，列表原地刷新），seed 账号可见性由行为用例覆盖。
