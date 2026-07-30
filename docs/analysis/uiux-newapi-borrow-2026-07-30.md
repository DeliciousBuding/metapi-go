# UIUX/产品化借鉴综合（New API 前端对标）— 2026-07-30

**Authority**: leader 综合（metapi-go web 审计 + New API 前端深度调研，两路 Explore 产出经 leader 交叉读源确认）
**Scope**: 前端 UIUX/产品化（后端产品功能 N1-N9 见 `product-parity-and-newapi-borrow-2026-07-30.md`，本轮不重叠）
**参考源**: New API `reference/competitors/new-api/web/default/src`（QuantumNous/new-api @ `a63364d`，TanStack Router/Table/Query + base-ui + VChart + i18next + cmdk + oxlint + knip）

---

## 1. 定位

本轮聚焦**前端 UIUX 与产品化体验**，以 New API 前端为主要参考（用户指定）。metapi-go web 是 React 19 + Vite 8 + 手写 design-system + 巨石页面；New API 是组件库化 + 特性模块化 + 标准化状态/表格系统。借鉴**模式与组件抽象**（不引入 TanStack 全家桶，保持手写栈）。

---

## 2. metapi-go web 现状短板（审计实证）

| 短板 | 证据 | 严重度 |
|:-----|:-----|:------:|
| **无 ErrorBoundary** | 全仓 0 处；lazy 路由抛错=白屏 | **P0** |
| **SearchModal 假键盘导航** | `components/SearchModal.tsx:321-325` 显示键盘提示但无 activeIndex/Arrow/Enter 实现 | **P0（诚实性 bug）** |
| **Toast 无 aria-live** | `components/Toast.tsx:66` 无 `role="alert"`，屏幕阅读器无反馈 | P0（a11y） |
| **i18n 严重缺失** | `pages/ImportExport.tsx` 884 行仅 1 个 `tr()`；Settings/Monitors 大量硬编码中文 | P1 |
| **列表页无 CSV 导出** | ProxyLogs/CheckinLog/ProgramLogs/DownstreamKeys 均无导出 | P1 |
| **filter 状态不进 URL** | 仅 ProxyLogs siteId 进 URL；其余页跳转后丢失筛选/排序/滚动 | P1 |
| **design-system 几乎未采用** | `design-system/Button.tsx` 全 pages/ 仅 1 处（Dashboard:68）；Card/Input/Stack 近零采用，两套 CSS 并存 | P1 |
| **无标准化 Loading/Error 组件** | 仅 EmptyState 被广泛采用（41 处/16 文件）；每页自造 skeleton | P1 |
| **列表行无内联 test 按钮** | Sites/Accounts 必须 开编辑→滚到底→test；Models 无 test 跳 Playground | P1 |
| **无日期预设** | ProxyLogs 需手敲 datetime | P1 |
| **无页面级键盘快捷键** | 仅 Ctrl+K（搜索）+ Esc | P3 |
| **无实时刷新** | 仅 navbar 15s 轮询事件；Dashboard/ProxyLogs/TokenRoutes 全手动 | P2 |
| **巨石页面** | ProxyLogs 3605 / Accounts 3462 / ModelTester 3113 / Settings 2616 / OAuthManagement 2605 / Sites 2434 / Dashboard 1675 | P2（工程） |

---

## 3. New API 可借鉴模式（调研实证，leader 筛选可直接迁移到手写栈）

| # | 模式 | New API 证据 | 解决问题 | metapi-go 落点 |
|:--|:-----|:-------------|:---------|:----------------|
| B1 | **标准化 Loading/Error/Empty 三件套** | `components/loading-state.tsx`+`error-state.tsx`+`empty-state.tsx`（共用 `ui/empty` 复合） | 消除每页自造状态 UI | 补 `design-system/ErrorState`+`LoadingState`（已有 EmptyState） |
| B2 | **受控 ConfirmDialog** | `components/confirm-dialog.tsx`（`open/onOpenChange/destructive/isLoading/handleConfirm`） | trigger 与 dialog 分离，破坏性操作统一确认 | 替换散落的 `confirm()`/自造删除弹窗 |
| B3 | **可复制 StatusBadge + 溢出列表** | `components/status-badge.tsx`（badge/text/underline 三型 + `StatusBadgeList` `+N`） | 状态徽章统一 + 点击复制 | 各列表 status/model 徽章 |
| B4 | **stringToColor 确定性配色** | `lib/colors.ts:178-185`（char code sum mod palette） | model/group/user 零配置稳定配色 | BrandIcon 之外的 tag/group 配色 |
| B5 | **IME 感知 debounce 输入** | `components/data-table/hooks/use-debounced-column-filter.ts`（`isComposing`+onCompositionStart/End） | CJK 输入法 composing 期间不过滤 | 所有搜索/筛选输入 |
| B6 | **URL 同步表状态** | `hooks/use-table-url-state.ts`（pagination/globalFilter/columnFilters 进 searchParams） | 表格视图可分享/书签/回退不丢 | 各列表页 |
| B7 | **浮动批量操作栏** | `components/data-table/toolbar/bulk-actions.tsx`（bottom-center glass + Escape 区分 dropdown + a11y live region） | 选择 UX 统一 + a11y | 现有 `ResponsiveBatchActionBar` 可增强 |
| B8 | **全局 Cmd+K 命令面板** | `context/search-provider.tsx`+`command-menu.tsx`（nav 从 sidebar data 注入） | 键盘导航/页跳转一等公民 | 现有 SearchModal 已有 Ctrl+K，补真键盘导航即可 |
| B9 | **扁平 JSON i18n + static-keys 清单** | `i18n/config.ts`+`static-keys.ts`+`languages.ts`（English key 为源，`STATIC_I18N_KEYS` 防死键剪枝） | i18n 死键预防 + BCP-47 桥接 | metapi-go 用 `tr()`+`i18n.supplement.ts`，可借鉴 static-keys 思路 |
| B10 | **motion variants + reduced-motion 守卫** | `lib/motion.ts`+`page-transition.tsx`（`useReducedMotion()` 落 plain div；`AnimatedOutlet` 按 routeId 而非 pathname key） | 动画统一 + 无障碍降级 | 全局过渡/列表 stagger |
| B11 | **Section Registry** | `features/system-settings/utils/section-registry.ts`+`features/dashboard/section-registry.tsx`（`createSectionRegistry`） | 标准化 tab 子页，类型安全 | Settings/Dashboard 子 tab |
| B12 | **masked-value reveal + copy** | `components/masked-value-display.tsx`（Popover reveal + CopyButton） | 敏感值单元格统一交互 | DownstreamKeys/Tokens key 列 |

---

## 4. 合并优先级 + 执行批次

按硬门禁区分：**A 类=低风险纯 UIUX 修复/增强（可直接执行，小步提交）**；**B 类=需 Issue 讨论/拍板的产品功能（本轮不自动实现）**。

### Wave 1 — 防御性 + 诚实性 + a11y（A 类，立即执行）

| 项 | 类型 | 量 | 关联 |
|:---|:-----|:--|:-----|
| ErrorBoundary 组件 + 包裹 lazy 路由 | P0 防御 | S | B1 前置 |
| SearchModal 真键盘导航（修假提示） | P0 诚实 bug | S | B8 |
| Toast `aria-live`/`role="alert"` | P0 a11y | S | — |

### Wave 2 — design-system 状态三件套 + i18n（A 类）

| 项 | 类型 | 量 | 关联 |
|:---|:-----|:--|:-----|
| `design-system/ErrorState`+`LoadingState` | P1 基础 | S | B1 |
| ImportExport i18n 补齐（884 行 1→全） | P1 i18n | M | B9 |
| Settings/Monitors 硬编码中文补 `tr()` | P1 i18n | M | B9 |

### Wave 3 — 列表体验（A 类）

| 项 | 类型 | 量 | 关联 |
|:---|:-----|:--|:-----|
| ProxyLogs 日期预设（15m/1h/Today/7d） | P1 UX | S | — |
| 列表页 CSV 导出（ProxyLogs/CheckinLog/DownstreamKeys） | P1 UX | S-M | — |
| URL 同步 filter 状态（CheckinLog/ProgramLogs/DownstreamKeys） | P1 UX | S-M | B6 |
| Sites/Accounts 列表行内联 test/verify 按钮 | P1 UX | S | 复用现有 API |
| Models→Playground `?model=` 快跳 | P1 UX | S | — |

### Wave 4 — 借鉴优化（A 类，按容量选做）

| 项 | 类型 | 量 | 关联 |
|:---|:-----|:--|:-----|
| stringToColor 确定性配色（Badge 增强） | P2 | S | B4 |
| IME 感知 useDebouncedInput hook | P2 | S | B5 |
| 受控 ConfirmDialog + 推广替换 | P2 | M | B2 |
| 可复制 StatusBadge + 溢出列表 | P2 | M | B3 |
| motion variants + reduced-motion 守卫 | P3 | M | B10 |

### 不在本轮（B 类，需拍板）

后端产品功能 N1-N9 + G1（下游 key IP 白名单 / 公开价格页 / 余额告警等）→ 见 `product-parity-and-newapi-borrow-2026-07-30.md` §4，硬门禁需先开 Issue/用户拍板。

---

## 5. 硬门禁

1. **A 类可直接执行**：纯前端、行为中性或纯增强/修 bug，非新产品功能决策；遵循小步提交 + pre-push `npm run typecheck && npm test`。
2. **B 类不自动实现**：产品功能（后端 schema/API/产品决策）需 Issue 讨论/用户拍板。
3. **不引入 TanStack 全家桶**：借鉴模式与组件抽象，保持手写 React 栈（metapi-go 现有 `react-router-dom` + 手写组件）。
4. **不伪造**：SearchModal 键盘导航要么真实现要么移除假提示，不留中间态。
5. **reduced-motion**：任何新动画必须 `prefers-reduced-motion` 降级。

---

## 6. 来源

- metapi-go web 审计：`web/App.tsx` · `web/components/SearchModal.tsx` · `web/components/Toast.tsx` · `web/design-system/` · `web/pages/{ProxyLogs,Settings,ImportExport,Monitors,...}.tsx`
- New API 前端参考：`reference/competitors/new-api/web/default/src/{components,features,hooks,i18n,lib,stores,styles,routes}`
- 产品功能借鉴（后端）：`analysis/product-parity-and-newapi-borrow-2026-07-30.md`
- 工程优化（codeg 对标）：`analysis/engineering-optimization-2026-07-30.md`
