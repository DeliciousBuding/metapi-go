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
| 1   | 已交付（代码） | 徽章配方收敛                 | overview/availability/checkin/accounts/routes/channels 内联状态徽章 | 全量迁移完成：#825 迁移 dashboard 3 个手写 `<span>` 徽章 → `<Badge>` 语义变体；本 PR 收口正态状态 default→success 软徽章（含 routes 通道计数 success/warning/secondary 阶梯、failure-reason network/state→info/success）；23 个 data-variant 回归用例守卫 |
| 2   | 已交付（代码） | RealtimeSparkline 图表 token | `availability-section.tsx`                                          | 已改 `bg-chart-1/70`；`useChartColors` 已随 VChart 移除                                           |
| 3   | 已交付（代码） | 行内高频操作免菜单化         | `accounts-columns.tsx`                                              | #824 行内 Enable/Disable 按钮（`Power` + `Loader2` + 每行 pending via mutation `variables`，无全局锁，复用现有 i18n）；refresh/pin/checkin/edit/delete 仍走下拉菜单 |

### P2

| #   | 状态              | 项                                        | 位置                                                 | 备注                                                                    |
| --- | ----------------- | ----------------------------------------- | ---------------------------------------------------- | ----------------------------------------------------------------------- |
| 1   | 已交付 #744/#751  | URL 状态同步两套机制统一                  | accounts/checkin/token-routes/proxy-logs             | 页面统一 `useSearch` + `navigate`，loader 只读 `location.searchStr`     |
| 2   | 已交付 #758       | 图标族统一（HugeIcons × lucide 同屏）     | ui 原语 vs feature 层                                | ui 原语用 HugeIcons 免费层，业务层沿用 lucide，约定写入 `web/AGENTS.md` |
| 3   | 已交付 #757       | 文档漂移销项                              | DESIGN.md（9 vs 10 预设）、标题 400 vs 500           | 预设数、标题字重、状态色、图表栈和 CTA 对比度已对齐实现                 |
| 4   | 已交付（代码）    | 圆角层级 / header 高度双来源              | `app-header.tsx`、`authenticated-layout.tsx`、`layout-error-boundary.tsx` | 圆角半 stale（`data-table-view` 已用 `rounded-lg` token，无双来源）；header 高度半已收口到单一 `--app-header-height` token + 删 2 处冗余 inline re-declaration + 静态守卫 `header-height-ssot.test.ts`（#824） |
| 5   | 已交付（代码）    | 导入向导校验失败聚焦首错字段              | `import-wizard-dialog.tsx`                           | e1991ef 落地 `markInvalidAndFocusFirst` + `aria-invalid` + per-field clear，9 用例覆盖（`import-wizard-dialog.test.tsx`）；其他表单（`account-form-dialog.tsx` 等）仍为观察 |
| 6   | 已交付          | 移动端首帧侧栏 / settings 375px 导航      | `use-mobile.tsx`、`settings-sidebar.tsx`           | 两半均收口：首帧闪烁 #712 已修（`useIsMobile` 改 `useSyncExternalStore`，首渲染同步取 matchMedia 真值）；settings section nav 在 <lg 降级为横向滚动 chip 条（原全宽垂直列表把内容挤出 375px 首屏），active 项补 `aria-current="page"`，6 个布局契约用例守卫 |
| 7   | 已交付（代码）    | 骨架屏 shimmer / 表格骨架差异化           | `index.css`、`skeleton.tsx`、`table-skeleton.tsx`    | 唤醒休眠 `--skeleton-highlight` token（`.animate-shimmer` 渐变 + reduced-motion gate，替换 `animate-pulse`）；`table-skeleton` 按 `column.getSize()` 取宽替换固定百分比池；`no-gradients` allowlist 加 `index.css` 例外（#824） |
| 8   | 部分交付 → MASTER | Playground 会话化 + 模板库 + 批量延迟对比 | `model-tester/`                                      | #662 已交付会话化 + 模板库；批量延迟对比由 MASTER Wave 2–3 接管         |

### 明确不做（定位边界，见 benchmark.md）

多租户计费/钱包/订阅/支付、社交 OAuth 登录、多实例 sticky 共享改造。

## 审计证据口径

- 动线审计：5 条动线（首次接入 / 客户端分发 / 日常巡检 / 故障处理 / 模型测试），
  全部引用文件经验证；核心发现「建完路由之后系统撒手不管」已收口（#657 接入对话框 + 完成动线 toast 改接 `/settings/downstream`）。
- 视觉审计：25 项问题（3 P0 / 10 P1 / 12 P2），已修复 3 P0（`text-*-foreground` 透明底误用 ×6、软徽章 success/info 变体、站点 active 徽章语义统一）+ 2 P1（站点 active 软徽章、12px 软徽章对比度 AA：浅色 success/info/warning 明度下调）；2026-08-18 完成徽章配方收敛（P1 #3）：正态状态统一 success 软徽章、dashboard 内联配方改 Badge variant。
- 交互审计：1 P0 / 8 P1；已修复倍率行内编辑安全网、页码钳位、导入脏确认和 404/错误边界。其余内容保留为审计观察。
- 功能对标：已落地客户端一键导出 #657、全局搜索 #658、首页今日快照 #659、告警富化 #660；后续承诺以 MASTER 为准，不从本审计表直接派生。

## 2026-08-18 多角度复审（PM / 程序员 / 用户）

三个只读 explore 子代理从产品经理、前端工程师、真实用户角度复审 5 条主流动线 + 边缘态 + 首次接入到首次代理请求的完整旅程。引导深链链路（site → account → route → downstream key）经核实完整。下列为**未升格为承诺的观察**（证据），立项须经 [`../progress/MASTER.md`](../progress/MASTER.md) 提升并附 owner + 验收标准。

### 本轮已收口（#828 + #832 + #835）

- **Dashboard stat 卡 drilldown**（PM #5 / User #1 partial，#828）：`StatCard` 加 `to` prop → `<Link>`，四张卡接线 `/accounts`/`/sites`/`/checkin`/`/proxy-logs`（S）。
- **接入凭证导出 → 测试请求深链**（User #5，#828）：`credential-export-dialog.tsx` footer 加 secondary 按钮 `<Link to='/model-tester'>`，闭合旅程最后一步 dead-end（S）。
- **Proxy-logs 列表过滤服务端化**（功能 bug，#832）：`latencyMin`/`latencyMax` + 原 silent no-op 的 `client`/`from`/`to` 全部移入 `statsHandler.proxyLogs` 共享 `where`/`args`（items/count/summary 一致），删客户端过滤 memo；6 后端 + 3 前端测试（M）。
- **Settings 边缘态硬化**（#832）：`keys-section` query 错误 → `SettingsSectionError`；enable `Switch` pending 禁用 + 唯一 `aria-label`（`enabledAria` i18n）；空态内联「Create」按钮；`update-center` 错误 → `SettingsSectionError`（4×S）。
- **Downstream key edit mode**（PM #4，#835）：`KeySheetForm` 加 `editingKey`，`editKeySchema = createKeySchema.omit({ key })`（secret 不可改），调 `api.updateDownstreamApiKey`（PATCH 风格 partial update，`key`/`description` 省略以保留）；Pencil 行按钮 + 3 i18n + 4 测试（M）。
- **Price-compare → routes 深链**（PM #3，#835）：`PriceRow` 加 ghost 图标按钮 `<Link to='/token-routes' search={{ q: row.model }}>`（routes 页 `q` 过滤匹配 `modelPattern`）+ 2 i18n + 4 测试（S）。

### 剩余观察（按主题）

**动线 dead-end / 缺 CTA**

- `proxy-log-detail-sheet.tsx:143-177` — ~~channel/account/route/token ID 渲染为不可点 `#NNN`~~ 2026-08-18 部分修复：route/channel ID 改为 router Link（`/token-routes?routeId=N` / `/channels?channelId=N`），两目标页各自一次性消费参数打开详情 sheet 后 strip（accounts create/siteId 同款消费模式，stale id 静默清除），9 行为用例覆盖；account/token 字段已有名字回显（仅无名时显示 `#NNN`），留作后续观察（M）。
- `channel-detail-sheet.tsx:216` — 无 `SheetFooter`，cooldown/breaker_open 通道无「清冷却 / 探测」动作（可复用 `useClearRouteCooldown` 模式）（M）。
- `channels-page.tsx:139` — 空态无 CTA（应接 `/accounts` 或触发 `rebuildMutation`）（S，但 channels 列属并行进程 badge 范围，需协调）。
- `overview-section.tsx:253` — 首次落地（0 site）无 onboarding banner/CTA（M，#828 stat-card drilldown 部分缓解）。

**行级 pending + a11y**

- `accounts/api.ts:281` — `useToggleAccountPin`/`useToggleAccountStatus`/`useToggleAccountCheckin` `onSuccess` 只 invalidate 不 toast（status 已由 #824 行内 Loader2 救场；pin/checkin 仍走下拉菜单 fire-and-forget 无反馈）（S，accounts 列属并行进程 badge 范围，需协调）。

### 旅程链路核实结论

`?siteId=…&create=1` 深链：`resolveDeepLinkPreselect` 校验 site 后预选 + 一次性消费 + `navigate(…, replace)`；`buildAccountsHref` 保留 transient 参数直至 effect strip；`showAccountCreatedToast` → `/token-routes?accountId=…&siteId=…` → `buildChannelDraftSeed(chainContext?.accountId)`。链路完整，唯一断点是 `route-form-dialog.tsx:423`：`accountOptions.length === 0` 时（未跑 Rebuild 模型发现）silently 隐藏 `channelDrafts` 段，深链 seed 的 account 不可见（M，需空态 hint + 「先 Rebuild」指引）。

### 二轮复审（model-tester / oauth / availability）—— 2026-08-18 续

三个只读 explore 子代理复审 model-tester / oauth / availability（一轮未深覆盖区）。下列为证据，未升格为承诺；标 ✅ 已收口，⚠️ 仍开放。

**model-tester** (`web/src/features/model-tester/`)

- ✅ **#840**：token 用量展示（`parseUsage` 从 upstream body 解析 prompt/completion/total，跨 OpenAI/Claude/Responses/Gemini；cost 不在 wire 上——harness 绕过计费，未伪造，记为 residual）；删 always-dead `chunks: 0` stat；failed run 抑制 empty badge 只显 error。
- ⚠️ 批量对比行 dead-end（无 re-run / channel drilldown）（M，触 channels 列）；cooldown/breaker 通道可选可静默失败（M，触 channels 列）。

**oauth** (`web/src/features/oauth/`)

- ✅ **#844**（并行进程）：start-dialog provider loading/error/empty 三态分支 + retry/settings 链。
- ✅ **#845**（并行进程）：refresh/rebind 行级 pending（`pendingActionId` 镜像 accounts `pendingStatusId`）+ per-account error toast（`skipErrorHandler` 防双 toast）。
- ⚠️ quota / lastModelSyncError / routeUnit 字段未呈现（M，加 quota 列 + 状态 tooltip 或 OAuthDetailSheet）；Start-OAuth 流 dead-end——丢 `result.state`/`instructions`，无 pending session 轮询、无 manual-callback UI（L，后端 `getOAuthSession`/`submitOAuthManualCallback` 已存在未用）；无批量 refresh + `updateOAuthConnectionProxy` PATCH 未用（proxy 改需删重建）（M+S）。

**availability** (`web/src/features/dashboard/sections/availability/`)

- ✅ **#846**：realtime sparkline 按 success-rate 健康分档着色（healthy/degraded/unhealthy/idle → `bg-success`/`bg-warning`/`bg-destructive`/`bg-muted-foreground`，原单色）+ `role="img"`/`aria-label`。Latency 不在 realtime wire 上（后端 `RealtimePoint` 无 latency）→ 未伪造，记为后端 residual。
- ✅ **#847**：attention 项 `createdAt` 渲染为相对时间（`Intl.RelativeTimeFormat` via `toBcp47`，零新 key，en/zh-CN 自动本地化）+ `<time dateTime>` + absolute `title` tooltip。
- ✅ **#850**：WS 重连 5 次永久放弃后，面板渲染「Connection lost — Reconnect」notice（dashed destructive border + TriangleAlert + outline button）替代归零 metrics（原看起来像「无流量」）；`useRealtimeOps` 重构为 `{ sample, reconnect }`，`reconnect` 是稳定 `useCallback`（reset `failsRef`/`backoffRef`/socket + 经 `connectRef` 重入 `connect`）；`min-h-[8rem]` 防面板塌缩。
- ⚠️ attention alert 点击 dead-end 到通用落地页（plain `<a href>` + 不支持的 `?accountId=` 参数）（M，触 accounts schema）；realtime 面板 success 下降时无 drill-down 到 per-channel 健康（M，触 channels）。

**已收口（续，#838 + #840 + #842 + #846 + #847）**

- **Dashboard onboarding banner**（User #1，#838）：`siteCount===0` 时显示 brand-tinted `<Card>` + 「Create site」CTA → `/sites`；`=== 0` 排除 loading 防误闪。
- **Sites `?create=1` auto-open**（#838 follow-up，#842）：`sitesSearchSchema` 加 `create`，sites-page 一次性消费 + strip（镜像 accounts `?create`），overview CTA 改 `<Link to='/sites' search={{ create: true }}>` 直达 create dialog。
- 详见上方 model-tester / availability 各 ✅ 项。

### 三轮复审（sites）—— 2026-08-18 续

一个只读 explore 子代理从 PM / 工程师 / 用户视角复审 sites feature（此前一轮/二轮未深覆盖区）。下列为证据，标 ✅ 已收口（#851），⚠️ 仍开放/暂缓。

**sites** (`web/src/features/sites/`)

- ✅ **gap-1 (P1)**（#851）：`customHeadersOverrideRequestHeaders` 在 `siteToFormValues` 硬编码 `false` → 编辑站点名等不相关字段会静默把 header-merge 从「site-wins」降级为「request-wins」（生产影响）。修复：round-trip 真实值 + 加可见 Switch FormField（label + hint）；`Site` type 加 optional 字段（原仅在 `SiteFormPayload`）。
- ✅ **gap-2 (P2)**（#851）：i18n 错误文案限额（100/50）与 Zod schema（120/64）不一致 → 用户输 110 字符名看到「最多 100 字符」。修复：en/zh-CN 对齐 120/64。
- ✅ **gap-4 (P2)**（#851）：sites 页/form dialog 零测试 → 加 `sites-page.test.tsx`（3 例：create 深链、error+retry、no-error table）+ `site-form-dialog.test.tsx`（4 例：Zod 错误、create payload、edit round-trip `customHeadersOverrideRequestHeaders`、dirty-close guard）。
- ✅ **gap-8 (P3)**（#851）：`platform` 是 free-text Input 但 placeholder 写「Select a platform」→ 改「Enter a platform (e.g. openai)」。
- ✅ **gap-10 (P3)**（#851）：error 态同时渲染空态 CTA「Add site」（误导——加站点不修 load error）→ error 态只显 error banner + Retry 按钮（`sitesQuery.refetch()`），抑制 `<DataTablePage>`。
- ✅ **gap-12 (P3)**（#851）：`SiteCreatedModal` 用 stale untyped `href` 导航（注释自承 workaround）→ 迁移到 typed `navigate({ to: '/accounts', search: { siteId, create: true } })`，匹配 `site-detail-sheet.tsx`。
- ⚠️ **gap-3 (P2)**：无 `apiEndpoints` 增删/启用/排序 UI（form preserve-only，detail sheet read-only）。后端 `SiteAPIEndpointInput` 已支持完整数组。当前行为诚实（edit 保留既有、create 空），未做 UI 属 feature gap 暂缓（按真实需求再立项，或 document 为 residual）。
- ⚠️ **gap-5 (P2)**：form `Select`/`Textarea` 未经 `id`/`htmlFor` 与 `FormLabel` 程序化关联（a11y）——跨 feature 模式问题（accounts form 同样），需在 `Form` primitive 层修，暂缓避免与并行进程 accounts 范围冲突。
- ⚠️ **gap-9 (P3)**：后端 `probe-now`/`available-models`/`disabled-models` endpoint 未在 sites UI 呈现（`lib/api/sites.ts` 已封装未用）。需与 `features/models`/`model-tester` 协调 ownership，暂缓。
- ⚠️ **gap-6 (P3)**：无 `?edit=<id>` 深链（打开 edit dialog 纯本地 state）——低优先级，除非运营者分享 edit 链接。
- ⚠️ **gap-7 (P3)**：load error 无 Retry 按钮（#851 已为 sites 加，但 accounts/channels 同模式未统一——可抽 `<QueryErrorBanner>` shared 组件）。
- ✅ **gap-11 (P3)**（#853）：`postRefreshProbeLatencyThresholdMs` 不可编辑（round-trips 但无 FormField）→ 加 number FormField inside `probeEnabled` block（2-col grid：model full-width，scope + latency 并排），镜像 `valueAsNumber` + NaN→0 绑定 + 测试。probe 配置表面完整（model + scope + threshold）。

### 四轮复审（token-routes list/columns/detail）—— 2026-08-18 续

一个只读 explore 子代理从 PM / 工程师 / 用户视角复审 token-routes 的 LIST、COLUMNS、DETAIL SHEET（避开 form dialog——并行进程 `route-form-drafts-hint` 拥有）。下列为证据，标 ✅ 已收口（#854 后端 / #855 前端），⚠️ 仍开放/暂缓。

**token-routes** (`web/src/features/token-routes/` — list/columns/detail，不含 form dialog)

- ✅ **gap-1 (P1)**（#854）：`listSummary` 硬编码 `siteNames: []string{}` → 「Sites」列 + detail 字段每行恒为 `—`，全局过滤也搜不到 site 名。修复：`listSummary` 加批量 `GROUP BY (route_id, site_name)` JOIN（route_channels→accounts→sites），dedup per route，nil→`[]string{}`；后端测试覆盖 linked（3 channel 同 site→dedup 1）+ empty。
- ✅ **gap-2 (P2)**（#855）：行「Refresh decision」dropdown 项 + detail sheet 按钮实为全局 `POST /api/routes/decision/refresh`（handler 忽略 route arg），与 header 按钮冗余且误导 → 删除行项 + detail 按钮（保留 header）。
- ✅ **gap-3 (P2)**（#855）：first-run 空态无 CTA → 加 `emptyAction`（primary「Add route」+ secondary「Auto-rebuild」outline），镜像 accounts-page。
- ✅ **gap-4 (P2)**（#855）：error banner 无 retry → 拉 `useRoutes().refetch` + Retry 按钮，error 态抑制 table（镜像 sites-page gap-10）。
- ✅ **gap-8 (P2)**（#855）：detail「Rebuild」是 fleet-wide 全局动作但标签「Auto-rebuild」不传达范围 → 改「Rebuild all routes」+ 图标 `ExternalLink`→`RefreshCw`。
- ✅ **gap-9 (P2)**（#855）：`requireChannelAllocation` 在 render path throw → refetch race 可经 layout error boundary 崩整页 → 改 `resolveChannelAllocation` graceful fallback（zeroed allocation + dev `console.warn`，永不 throw）。
- ⚠️ **gap-5 (P2)**：chain-context banner 显示 raw `#ID` 而非 account/site name（loader 已 prefetch 名称在 query cache，page 未读）——需读 `useAccounts()`/`useSites()` snapshot 解析（轻触 form dialog `chainContext` shape，需协调）。
- ⚠️ **gap-6 (P2)**：detail sheet footer 无「Edit」动作（需关闭 sheet 再用行下拉）——加 `onEdit` callback thread from page（coordinate openEdit callback signature only，不触 form dialog 内部）。
- ⚠️ **gap-7 (P2)**：toggle/clear-cooldown 无 per-row pending（镜像 accounts `pendingStatusId` pattern，触 routes-columns）。
- ⚠️ **gap-10 (P3)**：`showZeroChannel` toggle 在 table 下方而非 toolbar（viewOptions slot）——布局/视觉，低优先级。
- ⚠️ **gap-11 (P3)**：`index.ts` barrel 是 stub，consumer 直接 import feature 内部路径——项目级一致性观察，非 token-routes 专属。
