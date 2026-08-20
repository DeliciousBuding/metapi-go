# Changelog

All notable changes to Metapi-Go will be documented in this file.

格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

## [v0.16.2] — 2026-08-20

### Fixed

- **老 TS 库迁移后启动/查询崩溃**（#849 后续，hb0730 报告）：`SQL logic error: no such column: post_refresh_probe_enabled` — additive 迁移注册表只覆盖了 Go 独有新增列，漏了 TS 历史迁移加过的列；老版本 TS 的 `hub.db` 迁到 Go 后一查 `sites` 就崩。新增 sc2_017~024 共 8 个 additive 步骤，补齐 34 个 TS-heritage 列（sites probe/proxy/headers、token_routes、route_channels、proxy_logs、accounts OAuth、account_tokens、model_availability），并加回归测试防止 TS 后续迁移再加列时漏同步（#878）。老库无需手动操作，启动时自动补列。
- **Docker 数据目录权限导致启动失败**（#849）：Go 镜像以非 root 用户（uid 1001）运行，旧 TS 容器以 root 写入的 bind mount 数据目录会触发 `attempt to write a readonly database` / `unable to open database file`。现在启动前探测数据目录与既有库文件的可写性，失败时报可操作的 `chown -R 1001:1001 <dir>` / `chmod` 提示；README、docker-compose.prod、迁移与部署文档补充命名卷零配置 vs bind mount + chown 指引与 `ACCOUNT_CREDENTIAL_SECRET` 说明（#875）。
- **`metapi-migrate --verify` 校验和误报**：修复 4 处——目标侧哈希限定源列集合（列序无关）、settings 源侧过滤运行时键（db_type/db_url/db_ssl）、行哈希按规范化串排序（行序无关）、跨方言布尔规范化（SQLite 0/1 vs PG true/false）。裸 TS 源与 Go 迁移源 `--verify` 均 "All checksums match"（#875）。

## [v0.16.1] — 2026-08-19

### Fixed

- **OAuth Start-OAuth 流断链**（P1）：`onSubmit` 丢弃后端返回的 `state`/`instructions`，用户无法轮询会话或手动提交回调。修复：保留 pending state，渲染 pending 面板（2s `getOAuthSession` 轮询 + SSH 隧道命令复制 + 手动回调输入框绑 `submitOAuthManualCallback`），成功自动关闭、失败保留重试（后端 endpoint 已存在、前端 wrapper 此前未用）（#862）。
- **Token-routes chain-context banner 显 raw `#ID`**（P2）：banner 渲染 `account #7 / site #3` 而非名字。修复：读 loader 预取的 `useAccounts()`/`useSites()` cache 解析 `accountId→username` `siteId→name`，`#ID` 兜底（#862）。
- **Form Select a11y 关联缺失**（P2）：site-form 的 3 个 Select（`resinEnabled`/`useUtls`/`postRefreshProbeScope`）的 `SelectTrigger` 未包 `FormControl`，`FormLabel.htmlFor` 指向不存在的 id。修复：补 `FormControl` 包裹，恢复 label↔control 关联（#862）。
- **8 页面 load-error 无 Retry**（P2）：accounts/channels/oauth/models/proxy-logs/fix-candidates/price-compare/checkin 页面错误态只显 banner 无重试。新增共享 `<QueryErrorBanner>`（`role=alert` + 可选 Retry + spinner），8 页面统一接入获得 Retry（#862）。
- **Routes 行级操作无 pending 反馈**（P3）：toggle/clear-cooldown 走下拉菜单 fire-and-forget。修复：`useRoutesColumns` 加 `pendingToggleId`/`pendingCooldownId`（镜像 accounts `pendingStatusId`），匹配行 dropdown 项 swap `Loader2` + disabled（#862）。
- **showZeroChannel toggle 位置**（P3）：toggle 在表格下方而非 toolbar。移入 toolbar `viewToggle` slot（此前未用的 prop）（#862）。
- **Sites `?edit=<id>` 深链缺失**（P3）：编辑站点纯本地 state，无法深链直达。新增 `edit` schema 字段 + `buildHref` 保留 + 一次性消费 effect（镜像 `?create=1`，等 list 加载后开 edit dialog 并 strip 参数）（#862）。
- **Channel-detail 无 Edit 动作**（P3）：detail sheet footer 仅 cooldown 时渲染，无编辑入口。改为常驻 footer + 加「Edit route」按钮 → `navigate('/token-routes', { routeId })`（复用 routes drilldown）（#862）。

### Changed

- 收口 `docs/analysis/ui-ux-audit-2026-08.md` 剩余开放观察（⚠️ 项），全部提升为本 slice 的 owner + 验收测试交付。前端测试总数从 546 增至 632（+86），新增 OAuth session 轮询、sites `?edit` 深链、channel-detail Edit、routes per-row pending 等聚焦回归用例。本 slice 无 Go 代码改动。

## [v0.16.0] — 2026-08-18

### Fixed

- **Sites form silent data loss**（P1）：`customHeadersOverrideRequestHeaders` 在 `siteToFormValues` 硬编码 `false`，编辑站点名等不相关字段会静默把 header-merge 从「site-wins」降级为「request-wins」。修复：round-trip 真实值 + 加可见 Switch FormField（#851）。
- **Token-routes `siteNames` 死列**（P1）：`listSummary` 硬编码 `siteNames: []string{}`，「Sites」列 + detail 恒为 `—`，全局过滤也搜不到 site 名。修复：批量 `GROUP BY` JOIN 填充，dual-dialect 安全（#854）。
- **Token-routes `requireChannelAllocation` render-path throw**（P2）：refetch race 可经 layout error boundary 崩整页。改 `resolveChannelAllocation` graceful fallback（#855）。
- **Realtime WS give-up 静默死亡**（P1）：5 次重连失败后面板永久放弃，归零 metrics 看起来像「无流量」。加「Connection lost — Reconnect」notice + 稳定 `reconnect` callback（#850）。
- **Accounts 行内高频操作**：Enable/Disable 改行内 `Power` 按钮 + 每行 pending，免下拉菜单（#824）。
- **Header 高度双来源**：`h-14` → `var(--app-header-height)` 单一 token + 静态守卫（#824）。
- **Skeleton shimmer**：唤醒休眠 `--skeleton-highlight` token，`animate-shimmer` 渐变替换 `animate-pulse`，`table-skeleton` 按列宽取值（#824）。
- **Proxy-logs 列表过滤服务端化**：`latencyMin`/`latencyMax` + silent no-op 的 `client`/`from`/`to` 全部移入后端共享 `where`/`args`，删客户端过滤（#832）。
- **Settings 边缘态硬化**：keys/update-center query 错误 → `SettingsSectionError`；enable `Switch` pending 禁用 + `aria-label`；空态内联「Create」按钮（#832）。
- **Sites 表单打磨**：i18n 限额文案对齐 Zod（120/64）；error 态抑制空态 CTA + 加 Retry 按钮；`platform` placeholder 改「Enter a platform」；`SiteCreatedModal` 迁移 typed navigate（#851）。
- **Token-routes 列表/详情打磨**：删误导性行「Refresh decision」（实为全局）；first-run 空态加 CTA；error banner 加 Retry；detail「Rebuild」改「Rebuild all routes」+ 图标修正（#855）。
- **Status badge 语义收敛**：overview/availability 手写 `<span>` 徽章 → `<Badge>` 语义变体（#825）；accounts/checkin/routes/channels `variant='default'+dot` → `variant='success'` 软变体收敛（#827）。
- **Channels 空态无 CTA**：加「Manage accounts」outline 按钮接 `/accounts`（#839）。
- **Channel detail 无 cooldown 清除动作**：detail sheet 加 route cooldown clear action（#834）。
- **Keys 行内 CTA + toggle toast**：account pin/check-in toggle 加 named toast 确认（#841）。
- **Route form drafts 空态 hint**：`channelDrafts` 段在 `accountOptions.length===0` 时加 inline rebuild 指引（#837）。
- **Settings mobile 导航**：375px 视口 settings section nav 塌缩为水平滚动 chip strip，`lg` 以上保持 sticky vertical sidebar（#831）。
- **Proxy-log detail drilldown**：channel/account/route/token ID 渲染为可点深链，跳转到目标页 filtered view（#843）。
- **OAuth per-row pending + error toast**：refresh/rebind 行级 `Loader2` + per-account error toast（`skipErrorHandler` 防双 toast）（#845）。
- **a11y 残差核实**：axe-core 15 路由 0 serious/critical；菜单 Esc 行为钉死 2 个行为用例；清理 stale checklist 项（#833）。

### Added

- **Realtime WS reconnect affordance**：`useRealtimeOps` 重构为 `{ sample, reconnect }`（稳定 `useCallback`，经 `connectRef` 重入 `connect`，reset `failsRef`/`backoffRef`/socket）；`gaveUp` 态渲染 notice + Reconnect 按钮（#850）。
- **Sites `customHeadersOverrideRequestHeaders` Switch FormField**：probe/header-merge 配置可见可编辑（#851）。
- **Sites `postRefreshProbeLatencyThresholdMs` FormField**：probe 配置表面完整（model + scope + threshold）（#853）。
- **Token-routes `siteNames` 批量 JOIN**：`listSummary` 新增 `GROUP BY (route_id, site_name)` 查询填充 site 名称列表（#854）。
- **Token-routes empty-state CTA + error Retry**：first-run 空态加「Add route」+「Auto-rebuild」CTA；error 态加 Retry 按钮调 `refetch`（#855）。
- **Dashboard onboarding banner + sites `?create=1` 深链闭环**：`siteCount===0` 时 brand-tinted `<Card>` + CTA；sites-page 一次性消费 `create` param（#838/#842）。
- **Dashboard stat-card drilldown**：`StatCard` 加 `to` prop → `<Link>`，四张卡接线（#828）。
- **Credential export → model-tester 深链**：接入凭证导出 dialog footer 加「Send a test request」按钮（#828）。
- **Downstream key edit mode**：`KeySheetForm` 加 `editingKey` + `editKeySchema`（secret 不可改），PATCH partial update（#835）。
- **Price-compare → routes 深链**：`PriceRow` 加 ghost 图标按钮深链到 `/token-routes?q=<model>`（#835）。
- **Model-tester token 用量展示**：`parseUsage` 从 upstream body 解析 prompt/completion/total 跨四协议（cost 不在 wire 上，记 residual）（#840）。
- **Availability sparkline 健康分档着色**：success-rate 按 healthy/degraded/unhealthy/idle 着色 + `role="img"`/`aria-label`（#846）。
- **Attention `createdAt` 相对时间**：`Intl.RelativeTimeFormat` via `toBcp47`，零新 key（#847）。
- **Form focus-first-invalid 覆盖**：跨 RHF 表单的 focus-first-invalid 行为回归用例（#830）。
- **actions/cache 6 bump**（#818）。

### Changed

- 四轮 PM/工程师/用户多角度复审覆盖 5 feature 区（accounts/sites/channels/token-routes/proxy-logs + model-tester/oauth/availability + sites + token-routes），43+ gap 发现 → 30 PR 交付。审计证据在 `docs/analysis/ui-ux-audit-2026-08.md`。
- 测试总数从 ~340 增至 546（+206 测试），覆盖 error 态、empty CTA、表单 round-trip、a11y 行为、WS reconnect、probe 配置等。

## [v0.15.3] — 2026-08-17

### Fixed

- DialogFooter 半透明背景（`bg-muted/70 backdrop-blur-sm`）导致滚动内容穿透，改为不透明 `bg-popover`（#822）。
- AlertDialogContent 完全没有高度约束（与 DialogContent 修复前同 bug），补齐 `max-h-[calc(100dvh-2rem)]` + `overflow-y-auto` + `flex-col`；AlertDialogFooter 同步改不透明 + `sticky bottom-0`（#822）。
- DialogHeader 不 sticky，长表单滚动时标题/描述滚走；补 `sticky top-0` + `bg-popover`（#822）。
- PopoverContent 无 `max-h`，长内容溢出视界；补 `max-h-(--available-height)` + `overflow-y-auto`（#822）。
- site-form-dialog 缺 `onInvalid` handler，Zod 校验失败时静默不反馈；补 toast + i18n key `sites.form.invalid`（#822）。

### Added

- `dialog-viewport.test.ts` 加断言：DialogFooter 不得用 `backdrop-blur` 或半透明 `bg-*/<digit>`，防穿透回归（#822）。

## [v0.15.2] — 2026-08-17

### Fixed

- 修复 DialogContent 无高度约束导致长表单（如站点注册对话框 12+ 字段）溢出视界、提交按钮不可达：给 `DialogContent` 加 `max-h-[calc(100dvh-2rem)]` + `overflow-y-auto` + `flex-col`，`DialogFooter` 加 `sticky bottom-0` + `backdrop-blur` 使操作按钮在内容滚动时始终可见（#815）。

### Added

- 新增 `dialog-viewport.test.ts` 静态护栏：断言 `DialogContent` 携带 `max-h` + `overflow-y-auto` 合约、`DialogFooter` 携带 `sticky bottom-0`，防止设计系统原语静默回退（#815）。

## [v0.15.1] — 2026-08-17

### Fixed

- 修复 PostgreSQL BOOLEAN 列与整数字面量比较的 dialect 不兼容（#805）：`/api/models/token-candidates` 在生产 PG 上返回 500，根因是 `stats_marketplace.go` 等文件用了 SQLite-only 的 `COALESCE(<bool>, 0) = 1` / `<bool> = 1`，PG 报 SQLSTATE 42804。改为 `COALESCE(<bool>, false) = true` / `<bool> = true`（双 dialect 通用）。波及 `stats_marketplace.go`（15 处）、`model_redirects.go` × 2、`service/model_redirects.go` × 1、`sites.go` × 2（`use_system_proxy = 1/0`）。
- `tokenCandidates` handler 的四个 builder `err` 之前被静默丢弃（`writeError(500)` 不打日志），导致服务端日志无任何线索。现在每个 builder 失败时走 `slog.Error` 记录错误 + `writeErrorWithRequest` 带上 request_id。

### Added

- 新增 `docs/pg_boolean_gate_test.go` 静态 gate：扫描 `handler/ service/ scheduler/` 非测试 `.go` 文件，禁止 `COALESCE(<bool>, 0)` 和 `<bool> = [01]` / `<> [01]` / `!= [01]` 模式出现在全部 16 个已知 BOOLEAN 列上（大小写不敏感）。与既有 `pg_rebind_gate_test.go`（42601）互补，覆盖 42804 类 dialect drift，将"reactive per-incident gate"升级为"systematic dialect-compatibility contract"。
- 新增 `handler/admin/stats_marketplace_pg_test.go`（build tag `integration`）：在真实 PG 上种子 site + account + token + availability 行，调用全部四个 builder，确保 SQL 在 PG 的 BOOLEAN 类型系统下不再回归。
- `docs/deployment.md` nginx 反代模板补齐 WebSocket upgrade 头（`proxy_http_version 1.1` + `Upgrade`/`Connection $connection_upgrade` map 模式），防止 `wss://` 握手在代理层被掐断。

### Changed

- 产品品牌 MetAPI → Metapi 机械改名跨 62 文件（注释 / 用户文案 / docs / i18n / electron / 脚本 / 测试 / pre-push hook）；wire 标识（`modelsOwnedBy="metapi"`）、env var 名、行为零变更。
- logo 系统由圆角徽章文本 π 换成透明底 π 字形 + 蓝青渐变 SVG（亮 / 暗双主题单资产可读）；栅格化 `logo.png`(512) / `favicon.png`(32) / `favicon-64.png`(64)，重生成 `desktop-icon.png` / `desktop-tray-template.png`；`generate-icons.mjs` 去掉徽章时代的圆角裁剪以免切掉无徽章字形。
- 登录页标题升 `text-3xl font-bold`，删冗余脚注段；README 顶部加透明 hero banner（`README.md` / `README_EN.md`）。
- flat-surface 渐变护栏为品牌 logomark 开文档化 allowlist（`logo.svg` / `favicon.svg`），UI 表面仍守 OKLCH 平涂。

### Added

- 本地前端 Vite dev server（`vite.dev.config.ts` + `bun run dev:vite` script + `@tailwindcss/vite` / `@tanstack/router-vite-plugin` devDeps）；dev entry 经 `transformIndexHtml` 钩子 dev-only 注入，生产 Rsbuild 构建不带失效 `/src/main.tsx` script tag。

## [v0.15.0] — 2026-08-17

### Added

- 站点表单新增 Resin 粘性代理和 uTLS 指纹的 per-site tri-state 覆盖（继承全局 / 强制开启 / 强制关闭），前端 Select 配合后端 CreateSite/UpdateSite 写入路径（#807/#809）。

### Fixed

- 修复 Resin `resin_enabled` 和 `use_utls` 无法通过 REST API 写入的断裂：CreateSite INSERT 补齐两列、UpdateSite `jsonKeyToColumn` 补齐映射、`SiteSelectColumns` 补齐 `use_utls` 读回（#807）。
- 修复通知设置 hydration 不对称：`ApplyRuntimeSettings` 补齐 `smtp_secure`、`notify_cooldown_sec`、`telegram_api_base_url`、`telegram_use_system_proxy`、`telegram_message_thread_id`、`system_proxy_url` 六个读侧 case，持久化设置不再在重启后静默回退（#807）。
- 导入流程 UX 收口：渲染 per-item 失败原因、修复无法检测 URL 时的 toast 轰炸（`skipErrorHandler`）、加入 `aria-busy`/`aria-live` 无障碍区域、修复所有 label `htmlFor`/`id` 关联和 Switch `aria-label`、补齐 `ImportSiteItem.duplicateStrategy` 类型漂移（#808）。

### Chore

- 新增 17 个导入流程前端测试（`parseUrlLines`/`canonicalizeUrl` 12 个 + wizard 行为 5 个）和 14 个后端测试（Resin 写入路径 7 个 + settings hydration 7 个）。

## [v0.14.0] — 2026-08-17

### Added

- 账号表单增加 password credential mode 并接入真实登录链；新增真实 new-api/one-api 服务容器 e2e、Sub2API/CLIProxyAPI token-import 验证链和公开测试床模板（#770/#772/#774/#775）。
- 路由策略新增 `lowest_cost` / `least_busy` / `lowest_latency` 管理端选择，并接入 models.dev 官方价格目录作为冷启动成本信号（#783/#790）。
- 站点/模型运行时熔断器增加 half-open 探测，恢复通道可受控重新进入候选集（#791）。
- 接入向导收口：站点→账号→路由链路携带 `siteId`/`create` 深链预选、session/API key 凭证内联验证，创建路由时从链上下文预填 channelDrafts（#796）。
- 路由详情真值：每个通道展示配置权重 + 启用占比（停用通道不计入分母）与规范化输入/输出单价（按具体模型 + accountId 关联，附价格来源），纯函数拆到 `route-price-truth` 并覆盖测试（#799）。
- 引导链真值收口：站点 CTA 使用真实 TanStack 路由，账号/路由创建 ID 按后端响应解析，编辑保留脱敏凭据，批量通道部分失败显式提示（#800）。
- 测试台真值收口：解析同步 harness envelope，保留上游状态/延迟/错误，禁用通道不再进入比较，停止请求独立计数，比较结果不污染会话历史（#801）。
- URL 状态稳定性：列表页以 URL 作为分页、筛选、搜索和排序唯一所有者，稳定表格回调与最新 URL 合并，加入真实 Chromium route/a11y/mobile smoke（#802）。

### Changed

- SQLite 默认启用 `synchronous=NORMAL` 与连接级缓存调优；成功请求会衰减历史 `failCount`，避免已恢复通道长期受罚（#784/#785）。
- agent handoff 文件改为本地消耗品并加入忽略规则（#793）；已完成的 WEB-ARCH 执行计划从活跃文档面移除。

### Fixed

- 前端兼容旧 URL/search 参数并补齐剩余 `validateSearch` 与受限 localStorage 边界（#767/#780）。
- 修复 new-api v1 登录响应、one-api 识别与 session-cookie 登录，以及 CLIProxyAPI/Sub2API token 验证兼容性（#768/#769/#773/#776/#777）。
- 稳定 batch-writer shutdown 与 e2e `first_model` 空值处理（#771/#778）。
- usage 缺少缓存明细时按完整输入/输出单价计费，不再错误套用缓存折扣（#788）。
- usage aggregation flush 失败时保留 delta 与 watermark，重试不再静默丢失统计（#789）。
- 修复账号深链测试夹具未跟随 URL 状态 hook 演进的问题，确保完整前端测试在集成分支上可重复运行。

### Chore

- 将 tester channel/latency truth Waves 2–3 从开放计划毕业为已交付能力；剩余工作仅为 operator-gated 的真实 Codex/AnyRouter 探针（#558）。
- 将列表页 URL 状态稳定性规则固化到 [`docs/internal/design/state-stability.md`](docs/internal/design/state-stability.md)，并把 `ui:smoke` 纳入 CI a11y acceptance gate。

## [v0.13.0] — 2026-08-15

### Added

- **下游适配上游（epic #676，19 子任务全量）**
  - 上游识别：adapter Detect 链接入 `service.DetectSite`（修复 one-hub/done-hub/veloera/sub2api/cliproxyapi 5 平台无法自动识别，one-api 误标 new-api）＋ 防误判加固 ＋ HTTP 探测统一超时（#684/#689）；商汤 SenseTime 平台检测（#706）
  - 反机器人/指纹：出站统一浏览器 UA（替换 `Go-http-client/1.1` bot 信标）＋ 每站 `cf_clearance` 注入（新增 `sites.cf_clearance`/`browser_ua` 列，`sc2_013` 迁移）＋ 可选 utls TLS 指纹 ＋ 共享 Transport 池化（#687/#688/#701/#694）
  - 拉模型：修复静默空列表、关键适配器错误传播（#683）＋ 统一规范化/去重/排序（#690）＋ Sub2API 分组感知 + `token_model_availability` 回填（#695）
  - 自动签到：已签到幂等识别（#691）＋ 同站限速 + transient 重试（#692）＋ 边界打磨（auth 自愈/重启 catch-up/超时/lease）（#699）＋ 失败通知聚合（#700）
  - 新上游：Resin 粘性代理池（Tier 1 正向代理 + wss 反代 / Tier 2 override+lease+status）（#693/#698）；Grok/xAI OAuth 适配器（Device OAuth）（#696）；`/v1/images/generations` passthrough（#697）；Electron 桌面壳（#704）
  - Prompt 过滤（OAuth 账号池防封，可选）（#702）
- **前端功能**：客户端配置一键导出（Cherry Studio/CC Switch deep-link）（#657）、全局搜索 ⌘K 命令面板（#658）、首页今日快照（#659）、告警消息富化（#660）、模型测试台会话 + 模板库（#662）、可观测性自动刷新 + 错误态（#711）、proxy logs CSV 导出 + channels 详情面板（#713）、订阅汇总 `subscriptionSummary`（#708）、`/api/settings/database/migrate` 端点（#722）

### Changed

- **前端架构化（milestone WEB-ARCH，#732–#740 全量）**：URL 状态归属统一到路由层（4 页迁移 `useSearch`+`navigate`，loader 只读 `location.searchStr`）（#744/#751）；`_authenticated` 404 catch-all + checkIsActive 清理 + `?model=` 校验（#745）；动效/RTL/FOUC 单一来源（#756）+ 全 overlay `prefers-reduced-motion` 门禁（#760）+ 图标族收敛（ui 原语 HugeIcons 免费层，约定入 web/AGENTS.md）+ 交互原语收敛（transition-all→具体、select 移动端 portal、`--table-*` token）（#758）；设计系统文档对账（#757）
- **性能**：dashboard 聚合 10s 缓存 + proxy_log 异步批写（#710）；`/routes/summary` N+1 修复 + model upsert `ON CONFLICT`（#724）；移除 `@visactor/react-vchart`（bundle −2MB）（#718）；settings 代码分割（#712）；Dockerfile BuildKit cache mounts（#729）
- **可靠性**：admin/handler DB 错误传播修复（#746/#748/#759/#763/#716）；config 校验硬化（#725）；`LOG_LEVEL` 可配（#730）；列表端点防御性分页（#728）；SSE 错误事件 + 路由 error boundary + WS origin 限制 + scheduler shutdown 取消（#726）；admin 探测并发流式化（8 路，~8min→~60s）（#727）
- **测试**：OAuth/sharedcount/SwitchDB/balance/scheduler/cmd-migrate 覆盖率大幅提升（#715/#720/#723/#731/#717/#743/#750/#761/#762）

### Fixed

- 修复 #761+#763 交叉引入的 SSRF 测试编译回归（测试引用已被移入 `internal/ssrf` 的旧函数名）（#764）
- 模型拉取 200+畸形 body 静默返回空列表（#683）；adapter Detect 链未接生产（#684）
- 停止 admin 列表端点 `SELECT` 明文凭证（改为 LEFT/RIGHT/LENGTH 脱敏）（#719）
- dashboard fail-close → fail-open（单查询失败不再 500 全页）（#714）

### Security

- `/v1` 每 IP 限流（`PROXY_RATE_LIMIT_RPM`）+ 全局 token RPM 限额 + 请求体上限可配（#709）
- WebDAV SSRF 硬化（`internal/ssrf` 包统一守卫 + dial-time DNS 解析检查）（#741/#761/#763）
- OAuth 出站 prompt 过滤（#702）、WS origin 限制（#726）、LDOH URL 可配防误配（#741）

## [v0.12.0] — 2026-08-14

### Removed（架构简化：净删 ~21,000 行，Go 170K→150K）

- 删除 test-only 的 canonical 转换层（`transform/canonical` + `transform/openai/chat` + `transform/anthropic/messages`，~6,600 行）——生产唯一跨协议路径（OpenAI→Gemini）为原生直连，绕过 canonical（#652）
- 删除三套测试专用编排层（`proxy/conductor.go`、`SurfaceFailureToolkit`、`ExecuteEndpointFlow`）——生产实为 `handler/proxy/upstream.go` 内的手写重试循环（#653）
- 删除未激活的 lease/sticky 会话队列机制、`routing/workflow.go` stub、`routing/snapshot.go` 死实现（`SnapshotDB` 接口保留，由 `service.ProxyRoutingStore` 实现）（#653）
- 删除死 facade/符号：`app/prometheus.go` 11 个包装、`handler/shared` 半套 metrics/errors、`proxy/input_files.go`、`platform` 检测管线（`detect.go`/`InitRegistry`）、`service/adapter` 桥接、`service/proxy_util.go`、`DeployHelperToken`、`store.Migrate` 重复调用等（#650/#652/#651）
- 前端删除 14 个未使用 shadcn 组件 + ~101 个未使用 i18n key + 空 barrel（#654）

### Fixed

- **cmd/migrate 静默丢数据**：离线迁移工具自维护 schema 副本且已漂移（`sites` 漏 7 列等），改为复用 `store.AutoMigrate` 并新增运行时漂移守卫 `TestBuildersMatchStoreSchema`，漂移永不再发生（#651）
- **app 垃圾抽屉**：`app/proxy_upstream.go`（801 行 DB 访问层，违反 BACKEND.md「app 不得承载业务逻辑」）移入 `service/routing_store.go`（#651）
- **store/switch.go 运行时切换丢配置**：`SwitchDatabase` 走旧 `Open()` 丢 `DB_SSLMODE` 与连接池预算，改为透传 `cfg.PostgresSSLMode()` + `postgresPoolConfigFromRuntimeConfig`（#651）

### Changed（去重 + 标准化）

- 手写 stdlib 重实现全部替换为标准库：Hinnant 日历算法→`time.UnixMilli().Format()`、手写 `formatInt/parseInt64/jsonMarshal`→`strconv`/`encoding/json`、`stringsTrimSpace`→`strings.TrimSpace`、`min64/max64`→内建 `min`/`max`（#653）
- handler 去重：mask/coerce/coalesce 家族、`shared.WriteJSON`（删掉手写 JSON 序列化器）、`pathID`、`parseLimitOffset`、统一 `decodeJSONRequest` 与 `writeError` 错误响应（#650）
- routing 去重：cooldown 决策树 ×4 收敛为 `resolveCooldownUpdate`、eligibility 双实现收敛（修复 admin 解释缺失 OAuth/token 检查的真值 bug）、breaker 过滤器 API 收敛（#653）
- scheduler 去重：ticker 循环 ×11 收敛为 `intervalRunner`、retention ×3 收敛为 `NewRetentionScheduler`（#651）
- platform 去重：balance/checkin/model-list 解析各 ×4 收敛为 `base.go` 共享 helper（#652）
- god-file 拆分：`handler/admin/*`、`handler/proxy/upstream.go`、`routing/*`、`store/migrate.go`、`platform/newapi.go`/`sub2api.go`、前端 `lib/api.ts`（1997 行→10 个域模块 + 兼容 barrel）（#650/#652/#653/#654）

## [v0.11.0] — 2026-08-14

### Added

- 管理控制台 UI/UX 与功能全量交付（#594–#626，合并为 #633/#634）
  - UX 基础：共享格式化器、状态/空态组件、toast、CountUp、响应式 data-table + 移动端降级、无障碍、页面标题缩放（#594–#600）
  - 安全：token 列脱敏（#601–#602）
  - 可观测性工作台：Overview/Health/Proxy Logs、访问日志指标、slog panic 恢复、probe 驱动的路由重建过滤（#603–#610）
  - 导入：站点探测 + 统一导入向导 + 幂等批量导入（#611–#616）
  - 模型：定价层、价格对比、重定向修复候选 + 推荐（#617–#621）
  - 通道：只读列表 + probe 驱动的重建过滤（#622–#626）
- /api/routes/rebuild 响应新增 changed 统计（probe 过滤 no-change 短路时保持真实，含测试断言）
- 访问日志记录 status/bytes/duration_ms；statusRecorder 转发 Flush/Hijack/ReadFrom/SetWriteDeadline 保持 SSE/WebSocket 可用；slog panic 恢复带 request_id；/metrics 新增 go_goroutines / go_memstats_* / go_gc_duration_seconds（纯 stdlib，零依赖）（#593）
- docs/api.md 补全 `/api` 端点清单并修正漂移路径（#643）
- .env.example 补齐约 80 个可选配置键（#640）

### Fixed

- 调度器注册回归：`balance-refresh` / `log-cleanup` / `backup-webdav` 曾被构造但未 `Register()`，现已注册并加 16 集合回归测试（#635）

### Changed

- CI/CD 合并为单一 .github/workflows/main.yml 管道（测试 → 镜像推送 → GitHub Release），移除 cd.yml 中与 CI 重复的 release-gate；master push 推送镜像（latest+sha）；SemVer tag 额外构建 5 平台二进制 + checksums + 二进制冒烟并创建 Release
- Bun 工具链版本单一来源（workflow env.BUN_VERSION + Dockerfile BUN_VERSION build-arg）；发布前校验 tag / web/package.json / CHANGELOG 节一致
- 新增发布助手 scripts/release.sh（校验后打 annotated tag 并推送）
- 升级安全：旧 schema 增量 `ALTER TABLE` 回归测试 + 迁移文档明确 forward-only（#636）
- docker-compose.prod.yml 安全硬化（healthcheck / no-new-privileges / cap_drop / read_only / tmpfs / 资源限制）+ 部署升级清单（#639）
- 重新启用 `unused`/`gosimple` linter 并删除约 30 处死代码（#641）
- 前端 a11y 标签走 i18n + 删除孤儿 stub key + SSE 回调类型 any→unknown（#642）

### Chore

- Dependabot：actions/setup-go 5→7（#584）、upload/download-artifact + build-push-action majors（#592）、frontend-deps 5 项（#588）
- GitHub Actions Dependabot 分组 + 升级处理 SOP 文档（#591）
- state 文档 last-verified 日期更新（#590）
- 发布资产：install.sh 上传 release 资产 + sha256 校验（#637）
- 计数一致性：表 35 / 调度器 16 / i18n 1641 / 测试文件对账（#638）

## [v0.10.0] — 2026-08-12

### Added

- Versioned `ScheduleSpec` v1 for daily, interval, random-window, and custom Cron schedules, with additive preview/apply migration endpoints that preserve every legacy key.
- Unified settings form actions, load-error states, dirty navigation guards, semantic schedule controls, and responsive settings navigation.
- Server-side audit-log pagination (`limit`/`offset`) with a paginated audit table UI (page indicator, prev/next, method filter resets to page 1).
- Database settings page now uses the unified dirty-guard form (Save/Reset disabled until dirty, unsaved-changes hint, navigation guard); the in-app 501 migrate button is replaced with a CLI-only migration note.
- CJK sans-serif fallbacks for `--font-sans` (Noto Sans SC/TC/JP/KR, PingFang SC, Microsoft YaHei) so zh-CN UI no longer falls back to a non-deterministic system face.
- Mobile sidebar trigger in the app header (`md:hidden`) so the sidebar can be opened from the header on small screens.

### Fixed

- Kept legacy cron values and v1 schedule mirrors synchronized, including mixed legacy+semantic payloads.
- Persisted notification strings and task mute maps in a restart-safe format; PostgreSQL settings audit inserts now use rebound placeholders.
- Prevented partial settings edits from clearing untouched notification toggles, WebDAV fields, allowlist entries, or masked proxy-token inputs.
- Settings mutations now surface exactly one error toast (section-level `onError` only; the global interceptor toast is skipped for section-handled calls).
- `restartRequired` for runtime DB config is computed from the real diff; SQLite connection strings are path-normalized (`sqlite://`/`file://`/bare/default) and legacy string-encoded `db_ssl` values are tolerated, so an equivalent saved config no longer demands a restart.
- Update-center version rows hide dev `0.0.0.0` placeholders; program-logs section handles array/object responses, shows an error state, and replaces the disabled count input with a loaded-count label.

### Added — 品牌与国际化完善

- 品牌名统一 **Metapi**（identity-branding / locales / About / index title）
- 透明 SVG LOGO：`logo.svg`（纯色蓝圆角徽标 + 真 π 字形 U+03C0）+ `favicon.svg`，替换白底 PNG；router 根文件白名单 + 表驱动回归测试扩展 `image/svg+xml`
- 顶栏语言切换 `LanguageSwitcher`（en/zh-CN）：浏览器语言自动跟随（localStorage → navigator）+ `documentElement.lang`/`dir` 同步（`toBcp47`）；locale 各 1475 key 双向 0 缺失
- **主题定制面板**：顶栏 Palette 入口，4 轴（10 颜色预设 swatch / 字体 Auto-Sans-Serif / 圆角 6 档 / 缩放 4 档）+ 每轴独立重置 + 全局重置；**全部预设默认无衬线**（Anthropic 不再内联衬线，衬线仅显式选择）；移除遗留 FontProvider 双轨（html class → data-theme-font 单一机制）
- **侧栏导航 i18n**：导航标题改为 i18n key（sidebar.groups/items/backToHome，en + zh-CN），侧栏随语言切换完整本地化

### Fixed — URL 同步与滚动裁切

- sites/models/oauth/site-announcements 表格状态经 `useLocation()` 订阅 router location（`searchStr`），排序/分页立即在页面内生效（此前同路径 search 导航不重渲染，表格滞后）
- authenticated-layout 内容区 `overflow-hidden` 裁切超视口内容 → `overflow-y-auto`（长页面不可滚动）
- **侧栏导航点击崩溃**：TanStack `Link` 经 Base UI `render` prop 渲染时 React children 泄漏进 `router.navigate`（"Converting circular structure to JSON"）→ `SidebarNavLink` 包装器只透传 DOM-safe props
- **URL 同步表格参数噪声**：`searchParams.ts` parse/encode 分离，5 个 feature（sites/proxy-logs/models/oauth/site-announcements）search schema 以规范逗号字符串回写 URL，消除 `?sort=%5B%5D` / `?brand=%5B%5D` JSON 序列化噪声；proxy-logs schema 测试同步 encode 语义

### Changed — 文案与视觉润色

- 文案术语统一（启用/停用、额度、Check-in、通道）、内部计划编号（K1a/N9a 等）移出用户可见文案、tokenRoutes toast/链式横幅拼接 bug 修复、9 处硬编码 → t()（含移除公开设置页的 TokenDance 品牌泄漏）
- 登录页真实 logo 徽标 + 纯色背景 + lg CTA；Dashboard 统计卡骨架屏 + 纯色半透明面积图 + 空/错状态图标化 + WS 连接脉冲指示；设置页移动端响应式 drill-in + sticky 侧栏
- 全站移除渐变设计：主题色块、品牌 fallback、Logo/favicon、骨架屏、遮罩工具和流动边框统一改为纯色或删除；新增源码与 public 资产回归门禁
- 登录页与认证后顶栏统一复用 `InterfaceControls`（语言 / 外观 / 明暗），明暗切换按 resolved theme 工作；Public Sans 修正为实际变量字体族名，补齐项目级 mono 栈，移除字体与密度双轨，并在首屏挂载前恢复持久化外观轴
- 开发态 TanStack Query/Router Devtools 改为 `localStorage.metapi-devtools=1` 显式开启，默认界面与截图不再被第三方彩色浮层占用

### Chore — Git 工作流规范化（GitHub Flow）

- **分支模型**：master 唯一长期分支（受保护），短命分支（`fix/*`/`feature/*`/`chore/*`/`docs/*`）→ PR → Squash merge；规则文档 `docs/git-workflow.md`
- **master 分支保护**（GitHub 实际启用）：要求 PR + 11 个 CI 状态检查必选 + enforce admins + 禁强推/删除；不要求 approve（个人项目）
- **Squash-only 合并**：仓库级关闭 merge commit / rebase merge
- **PR 模板**：`.github/pull_request_template.md`（改动摘要/类型/测试验证/自查清单）
- **CI**：ci.yml 移除 `paths-ignore`（必选状态检查与跳过互斥，纯文档 PR 会永久 pending 卡合并）；PR + master push 全量 11 job
- **移除 update-center 幽灵前端**：`updateCenterReminder.ts`/`updateCenterPresentation.ts` 删除（update-center API 为 501 residual，前端不再渲染）

### Added — 仓库产品化（社区健康 + CI/CD 硬化 + 多平台发布）

- 社区文件补齐：CONTRIBUTING.md / SECURITY.md / CODE_OF_CONDUCT.md / issue 模板（bug + feature + config）/ dependabot（Go/npm/Actions/Docker 四通道）
- 仓库规范化：.gitattributes（LF 统一 + 二进制声明）、.editorconfig、.gitignore 补密钥/证书/通用 exe 规则
- README（中/英）修正过时数据（369 tests / 1434 i18n keys）、移除私有仓链接、下沉 Windows 防火墙细节、补 star/fork badge 与贡献/安全导航
- CI 硬化：全 job 超时上限、cd 最小权限、Go 版本单一来源（go-version-file）、bun/govulncheck 版本钉扎、gitleaks 扫全历史、前端 dist 产物跨 job 复用、tag 匹配收紧为 SemVer
- 发布产品化：Release 并入 CD 流程（镜像推送成功后才建 Release）、多平台二进制附件 + checksums、Docker 镜像 amd64+arm64 双架构、`--version` 版本注入
- 文档：docs/api.md Update Center 标注 501 residual、docs/README.md 补 responses-websocket-residual.md、CHANGELOG 链接只引用真实 tag

## [v0.9.0] — 2026-08-11

### Added — 前端整体重写（newapi 栈 100% 对齐）

- 技术栈：Bun + Rsbuild 2 + TanStack Router（文件路由 + validateSearch）+ TanStack Query + Zustand + Tailwind 4（CSS-first 无 config）+ shadcn Base UI + OKLCH 语义 token + HugeIcons/lucide + Public Sans/Lora（@fontsource 本地）+ VChart + recharts + RHF + Zod + i18next + motion/sonner/vaul/cmdk
- 设计系统：三层 CSS（theme.css + theme-presets.css + index.css）、OKLCH 语义 token、3 轴主题（preset/radius/scale）、10 套预设、圆角单点缩放、Tailwind 4 @theme inline 桥接、cookie 持久化暗色模式
- data-table 四层架构（core/layout/toolbar/static/hooks，~4910 行）：URL 状态同步三段式 + 移动端卡片降级 + 批量操作
- Dashboard 4 section（overview/traffic/models/availability）：VChart + shadcn chart 双轨 + useChartColors JS OKLCH token 取色（MutationObserver 重新采样）+ RealtimeOps WebSocket
- Sites/Accounts/Token-routes：data-table + 引导式配置动线（站点→账号→路由，SiteCreatedModal + 引导 toast）+ RHF/Zod 表单
- Settings 5 子区 drill-in（general/downstream/models/content/system-info）：createSectionRegistry 泛型工厂 + SettingsPage 通用分发器
- Checkin：嵌套 API 响应解构 + failureReason 分类着色 badge（6 分类）+ 手动签到
- ProxyLogs：data-table（manual + 服务端分页）+ 详情 Sheet（会话路径 + 计费 JSON + 前向兼容）
- Models + ModelTester：data-table + 品牌图标 + SSE 流式响应（parseAnyStreamDelta 全协议 OpenAI/Claude/Responses/Gemini）
- About/OAuth/SiteAnnouncements：完整 CRUD + 静态信息
- i18n：key-based（en + zh-CN 各 1369 key，双向 0 缺失），React 组件 useTranslation()+t()，.ts 模块 i18n.t()
- 测试：vitest + @testing-library/react + jsdom；351 tests 全绿（schema/utils/registry 就近 **tests**/）

### Fixed — 签到 ClassifyFailureReason 幽灵功能

- 后端 sc2_012 additive migration（checkin_logs.failure_reason TEXT，SQLite/PG dual dialect）+ checkin.go 调用 classifyAndMarshalFailureReason（4 个 INSERT 落库点）+ API 嵌套形状回归 {checkin_logs, accounts, sites, failureReason} + 前端 failureReason 分类着色 badge
- 四层全断修复：DB 列存在 + 后端分类调用 + API 嵌套响应 + UI 渲染

### Fixed — SPA fallback 吞掉 Rsbuild /static/* 静态资源（嵌入式白屏）

- 根因：`setupSPAFallback` 只服务 Vite 布局的 `/assets/*` 子树 + 根静态文件；Rsbuild 2 产物输出到 `/static/{js,css,font}`，落入 200 text/html SPA fallback → 浏览器 MIME 拒绝 → 白屏
- `mountStaticSubdir` helper：`fs.Sub(distFS, "static")` + `http.FileServer` 挂载 `/static/*`，content-hashed 文件名加 immutable 缓存头
- 启动子树校验：embed.FS 不实现 fs.SubFS，fs.Sub 对缺失目录静默返回损坏子树 → ReadDir 校验 + warn 日志（顺带修复从未触发的 assets 分支日志）
- 回归测试（真实 embedded dist + 动态 chunk 发现）：js→text/javascript、css→text/css、woff2→font/woff2、缺失资源→404（不再 SPA 200）、client 路由仍正常 fallback

### Removed — 旧前端代码清理

- 删除 web/App.tsx + pages/ + components/ + styles/ + e2e/ + 5 个旧 i18n 文件（MutationObserver 字典）+ ~200 个旧测试（测试已删的旧代码）：~53 个遗留文件/目录，~101664 行删除

### Changed — CI/构建迁移：npm → Bun

- ci.yml frontend job：npm → Bun（oven-sh/setup-bun@v2 + `bun install --frozen-lockfile` + typecheck/test/build:web）；cd.yml release-gate 同步 npm → Bun
- Dockerfile Stage 1：`node:25-alpine` → `oven/bun:1-alpine`（`bun install --frozen-lockfile` + `bun run build:web`，含 desktop icons；verify-dist.mjs 产物自洽校验移除）
- Makefile：`web-build` npm ci → `bun install --frozen-lockfile` + `bun run build:web`；`ui-e2e`/`ui-visual` targets 删除
- 删除 `ui-visual.yml`（Playwright 已砍）+ ci.yml `en-verify` job（Playwright + verify-en-pages.mjs 的 MutationObserver 字典验证由 key-based i18n 取代）
- 新增 i18n key 覆盖测试（`web/src/i18n/__tests__/i18n-keys.test.ts`）：扫描全量 `t()`/`i18n.t()` 调用，验证每个 key 在 en.json + zh-CN.json 均定义且两 locale key 集合完全一致（防 key 缺失回归）

### Chore — 运维整理：stash 恢复 + 死代码清理

- stash 恢复（重写前挂起）：backend `docs/design/BACKEND.md` + `a11y-checklist.md` 更新、`platform/error_classification.go` + `site_proxy.go` 注释修正；10 个 backend 文件已在 HEAD（0 diff）
- 死代码清理：`sitesEditor.ts`（313 行，重写后零引用）删除；knip cleanup 删除未使用的 `stub-section.tsx`（settings 18 个 stub 已全部实装）+ `format.ts` 3 个未用导出（formatCompactNumber/formatLatencyMs/formatCost）+ formatPercent 降级为内部函数

### Verified — 无缝迁移保证

- go test ./... -race：31/32 包通过（1 预存 baseline bug，websocket doc-pointer，与本次无关）
- SQLite dev 冒烟：health/ready/auth/SPA HTML 200 + sc2_012 failure_reason 列存在
- PostgreSQL 生产冒烟（本地真实 PG 18.2）：12 migrations 全应用 + sc2_012 在 PG 下执行 + schema_migrations 版本记录
- 前后端集成：bun build + go:embed 单二进制 + SPA HTML 200
- sc2_012 dual-dialect 幂等三重保护（schema_migrations 簿记 + columnExists 预检 + ALTER 后再检）
- 静态门禁：tsgo 0 error + oxlint 0 error + bun run build pass（4462.4 kB / 1385.8 kB gzip）+ vitest 351/351

### Added — 品牌与国际化完善

- 品牌名统一 **Metapi**（identity-branding / locales / About / index title）
- 透明 SVG LOGO：`logo.svg`（渐变圆角徽标 + 真 π 字形 U+03C0）+ `favicon.svg`，替换白底 PNG；router 根文件白名单 + 表驱动回归测试扩展 `image/svg+xml`
- 顶栏语言切换 `LanguageSwitcher`（en/zh-CN）：浏览器语言自动跟随（localStorage → navigator）+ `documentElement.lang`/`dir` 同步（`toBcp47`）；locale 各 1381 key 双向 0 缺失
- **主题定制面板**：顶栏 Palette 入口，4 轴（10 颜色预设 swatch / 字体 Auto-Sans-Serif / 圆角 6 档 / 缩放 4 档）+ 每轴独立重置 + 全局重置；**全部预设默认无衬线**（Anthropic 不再内联衬线，衬线仅显式选择）；移除遗留 FontProvider 双轨（html class → data-theme-font 单一机制）
- **侧栏导航 i18n**：导航标题改为 i18n key（sidebar.groups/items/backToHome，en + zh-CN），侧栏随语言切换完整本地化

### Fixed — URL 同步与滚动裁切

- sites/models/oauth/site-announcements 表格状态经 `useLocation()` 订阅 router location（`searchStr`），排序/分页立即在页面内生效（此前同路径 search 导航不重渲染，表格滞后）
- authenticated-layout 内容区 `overflow-hidden` 裁切超视口内容 → `overflow-y-auto`（长页面不可滚动）
- **侧栏导航点击崩溃**：TanStack `Link` 经 Base UI `render` prop 渲染时 React children 泄漏进 `router.navigate`（"Converting circular structure to JSON"）→ `SidebarNavLink` 包装器只透传 DOM-safe props
- **URL 同步表格参数噪声**：`searchParams.ts` parse/encode 分离，5 个 feature（sites/proxy-logs/models/oauth/site-announcements）search schema 以规范逗号字符串回写 URL，消除 `?sort=%5B%5D` / `?brand=%5B%5D` JSON 序列化噪声；proxy-logs schema 测试同步 encode 语义

### Changed — 文案与视觉润色

- 文案术语统一（启用/停用、额度、Check-in、通道）、内部计划编号（K1a/N9a 等）移出用户可见文案、tokenRoutes toast/链式横幅拼接 bug 修复、9 处硬编码 → t()（含移除公开设置页的 TokenDance 品牌泄漏）
- 登录页真实 logo 徽标 + 品牌光晕 + lg CTA；Dashboard 统计卡骨架屏 + 渐变 id 唯一化 + 空/错状态图标化 + WS 连接脉冲指示；设置页移动端响应式 drill-in + sticky 侧栏

### Chore — Git 工作流规范化（GitHub Flow）

- **分支模型**：master 唯一长期分支（受保护），短命分支（`fix/*`/`feature/*`/`chore/*`/`docs/*`）→ PR → Squash merge；规则文档 `docs/git-workflow.md`
- **master 分支保护**（GitHub 实际启用）：要求 PR + 11 个 CI 状态检查必选 + enforce admins + 禁强推/删除；不要求 approve（个人项目）
- **Squash-only 合并**：仓库级关闭 merge commit / rebase merge
- **PR 模板**：`.github/pull_request_template.md`（改动摘要/类型/测试验证/自查清单）
- **CI**：ci.yml 移除 `paths-ignore`（必选状态检查与跳过互斥，纯文档 PR 会永久 pending 卡合并）；PR + master push 全量 11 job
- **移除 update-center 幽灵前端**：`updateCenterReminder.ts`/`updateCenterPresentation.ts` 删除（update-center API 为 501 residual，前端不再渲染）

## [v0.8.54] — 2026-08-11

### Fixed — Model redirect SQL placeholder binding

- `loadRedirectCandidates` 在直连数据库时未对 PostgreSQL 占位符做 Rebind，导致模型刷新后 redirect 同步失败（主链路不受影响）。修复：显式 Rebind。
- 新增回归测试覆盖 `ExecContext/GetContext/SelectContext/QueryContext` 等 Context 变体的占位符校验。

## [v0.8.53] — 2026-08-11

### Fixed — New-Api-User header 401 降级

- `VerifyToken` 原先只在 HTTP 200 含 `New-Api-User` 时触发降级；部分上游站点直接返回 401，导致 session token 导入失败。修复：401 含 `New-Api-User` 时同样进入带 `New-Api-User`/`Veloera-User` 等头的重试路径。

### Fixed — 模型刷新持久化类型不匹配

- `persistAccountModelAvailability` 更新非手动行时布尔列类型不匹配，导致 `/api/models/check/:id` 写库失败。修复：改用 `NOT is_manual`（PostgreSQL/SQLite 双方言兼容）。

## [v0.8.52] — 2026-08-02

### Fixed — SQL placeholder binding

- 修复 4 处绕过 store 包装直连 PostgreSQL 的裸占位符调用（admin audit 插入、auth token 轮换、scheduler 查询、账号过期标记），全部显式 Rebind。
- 新增回归测试扫描 handler/service/scheduler 中的裸占位符调用。

### Changed — 飞书通知聚合防轰炸

- `SendNotification` 在带 TaskTag 时按 `tag:任务类型:级别` 签名聚合，同类告警冷却窗口内合并为 1 条并附合并计数，不再每账号刷屏。无 TaskTag 调用保持原签名向后兼容。

### Fixed — Resource integrity checks

- Dockerfile 构建后 `verify-dist.mjs` 产物自洽校验（入口 + 懒加载 chunk 存在才出镜像）；CI dist 完整性测试同一校验；部署后 `verify-live-assets.sh` 运行实例资源图重放（200 + 非 text/html）。

## [v0.8.51] — 2026-08-02

### Fixed — 登录页 logo/favicon 静态服务缺失

- 静态路由只注册 `/assets/*`，`/logo.png` 等根静态文件被 SPA fallback 以 200 text/html 应答，登录页 `<img>` 空白。修复：router 增加根静态文件白名单服务（logo/favicon/desktop icons，正确 Content-Type + immutable 缓存）。

### Changed — 深色登录页去渐变

- 深色模式下登录页 3 层 color-mix 径向渐变改为纯色 token，暗底不再显脏；浅色保留柔和渐变。

## [v0.8.50] — 2026-08-02

### Changed — 登录页单卡片化

- 移除左侧品牌大卡片与三行能力列表，品牌（logo + 名称 + 副标题）收敛进居中单卡片顶部；桌面/移动均一屏内零滚动；GitHub/部署文档链接收进卡片底部。
- 登录页 CSS 440 → 330 行。

### Changed — 登录页比例重构

- surface 总高 842 → 676px（一屏内）；修复移动端 640px 断点下登录卡被挤压的既有 bug（补 `flex-direction: column`）。

## [v0.8.49] — 2026-08-02

### Fixed — CI PostgreSQL 集成测试串行化

- PostgreSQL integration 测试多包并行共享单库时表状态竞态导致构建阻断。test-pg 命令加 `-p 1`（包串行）。

## [v0.8.48] — 2026-08-02

### Fixed — balance_history 快照 SQL placeholder binding

- `recordBalanceSnapshot` 用 SQLite 占位符直连 PostgreSQL，快照静默不落库。修复 Rebind；新增 PostgreSQL integration 测试。

## [v0.8.47] — 2026-08-02

### Fixed — PostgreSQL 旧库升级 boolean 列默认值类型不匹配

- `ALTER TABLE ... ADD COLUMN ... BOOLEAN DEFAULT 0` 在 PostgreSQL 报类型不匹配；SQLite 宽松接受、CI 全新库跳过了该迁移路径。修复：`DEFAULT FALSE`（双方言兼容）；新增 PostgreSQL integration 测试锁定旧 schema 升级路径。

## [v0.8.46] — 2026-08-02

### Added — Dashboard and analytics

- **余额历史快照 + 趋势图**：新 `balance_history` 表（per UTC day per account，同日 UPSERT）；`RefreshBalance` 成功路径自动写快照；`GET /api/stats/balance-history`；Dashboard「余额趋势」卡。
- **模型成本分布 + 延迟图表**：`GET /api/stats/model-cost-distribution`、`/latency-histogram`、`/latency-trend`（每日 avg/max/first-byte + 成功率 + p95）；Dashboard 三卡。
- **批量模型验证 + 验证历史**：新 `model_verify_history` 表；`POST /api/models/verify-batch` + `GET /api/models/verify-history`；Models 页批量验证 dialog。
- **余额流入 vs 消费**：`GET /api/stats/balance-income-outcome`（会计恒等式 income - outcome = Δbalance 推导，首日视为初始入账，退款如实反映为负 outcome）；Dashboard 分组柱卡。
- **需关注看板**：`GET /api/stats/attention` severity 排序深链项（过期账号/低余额/禁用站点/近 24h 告警事件）；Dashboard 顶部面板点击直达。
- **实时 QPS 运维面板**：1s×300 环形缓冲 + `GET /api/admin/ops/ws` 每秒推流 + Dashboard 实时流量面板（QPS/成功率/sparkline/指数退避重连）。

### Added — Tags, banners, and notifications

- **全局标签系统**：accounts/sites 支持彩色标签（JSON 数组列），`GET /api/tags` 全局索引（按使用量排序），`PUT /api/accounts/{id}/tags` / `PUT /api/sites/{id}/tags`；Accounts/Sites 页标签 chips 点击即过滤 + 共享 TagEditorDialog。
- **产品级风险横幅**：`product_announcements` + `announcement_dismissals` 表（severity info/warning/critical）；Dashboard 顶部配色横幅（dismiss + 详情链接）；Settings「产品公告」CRUD 区。
- **可分享看板快照 PNG**：Dashboard「导出快照」原生 canvas 绘制 1200×630 摘要卡（总余额/今日消耗/24h 请求/成功率/Token/活跃账号 + 站点消耗 Top5），下载 PNG，零新依赖。
- **per-task 通知 + 4 新渠道**：feishu/dingtalk（HMAC-SHA256 加签）/wecom/ntfy 四专用 channel（SSRF 校验）；`notify_task_toggles` 按告警类型静音（缺省全开向后兼容）。
- **实时低余额告警**：余额刷新发现 balance < 1.0 时触发 `ReportLowBalance`（per account per 24h 去重）。
- **管理操作审计日志**：`admin_audit_logs` 表 + AuditMiddleware 记录 POST/PUT/PATCH/DELETE（actor = token 哈希前缀，永不存原文）；Settings「审计日志」区（方法/路径/actor/IP/状态过滤 + 分页）。

### Added — Scheduling, backup, and observability

- **随机窗口调度模式**：checkin 支持 `window` 模式，在 `CHECKIN_WINDOW_START`~`END`（HH:mm）内随机生成每日 cron（负载扩散 + 反指纹）。
- **签到 catch-up**：window/cron 模式下实例在当天触发时刻后重启 → 启动时检测「今日触发已过 + 今日未跑 + 存在启用账号」→ 立即补跑（租约保护、幂等无双签）。
- **备份导入预览**：`POST /api/settings/backup/import/preview` 返回 per-table rows/toInsert/duplicates/skipped 计划且不写行；ImportExport confirm 前展示计划。
- **调度任务统一运行历史**：`GET /api/scheduler/status` 聚合 checkin/balance-refresh/model-probe/site-announcements/daily-summary/log-cleanup/usage-aggregation 的 last-run + 24h 活动；Dashboard 面板。
- **OAuth token auto-refresh**：60s 间隔，per-provider lead times（codex=5d, claude=4h, gemini-cli/antigravity=5min），singleflight dedup。

### Added — Routing, downstream keys, and WebSocket

- **per-downstream-key 权重 / 自定义 header / allow-list**：下游 key 权重、站点自定义 header 覆盖优先级、allow-list 绑定 sites/credentials。
- **IP allowlist/blocklist**：downstream key 支持 IP allowlist/blocklist（blocklist 优先，allowlist 非空要求匹配，两者空则不限）；DownstreamKey editor 文本区。
- **downstream-key pricing catalog**：`/v1/pricing`（+ `/v1/models/price-compare` alias）— 持有 managed key 的下游消费者可查询跨站点模型定价（非匿名公开）。
- **reasoning suffix**：`ParseReasoningSuffix` 剥离 `-thinking`/`-high`/`-medium`/`-low` 以匹配基础模型路由；OpenAI 注入 `reasoning_effort`。
- **Responses WebSocket**：Responses WebSocket（upgrade + HTTP SSE bridge + multi-turn/quota + Codex upstream wss 运行时，dial→HTTP fallback）。
- **multi-tier context**：同模型不同 `context_length` 路由按请求估算选最紧 fit；`LoadEnabledRoutes` 遵循 `sort_order`。
- **model redirect canonicalization**：per-account redirect 注册表（canonical→actual + 字典序确定性反向索引）；转发改写出站体 + 计费归因名。
- **rate overview + 倍率批量编辑**：`PUT /api/models/rates` 批量更新 unit_cost + weight（校验 ≥0、写后路由缓存失效）+ 总览页行内编辑。
- **admin-configurable prompt-cache ratio**：`DefaultCacheRatio`/`ClaudeCacheRatio` 运行时可覆盖（config → boot apply → Settings 暴露/持久化生效）。

### Added — UI and a11y improvements

- **RouteErrorBoundary**：所有 lazy 路由包裹错误边界，单个 lazy 页渲染错误不再白屏整个 SPA；导航离开自动清除。
- **SearchModal 键盘导航**：实现 ↑↓/Enter 真实键盘导航（combobox + `aria-activedescendant`/`aria-expanded`，results 为 `role="listbox"`）。
- **Toast a11y**：container `role="status" aria-live="polite"`；error toast `role="alert"`。
- **design-system triad**：新 `ErrorState`（+ auto Retry）+ `LoadingState`（skeleton/spinner），补齐 Empty/Loading/Error 三态。
- **Models → Playground quick-launch**：Models 卡片/行操作按钮 → `/playground?model=<name>` 预填模型选择器。
- **ProxyLogs date-range presets**：15m / 1h / Today / 7d 预设按钮。
- **visual motion**：Models 卡片网格 `animate-slide-up stagger` + hover lift 微交互。
- **表格密度切换**：默认行高 10px → 8px；主题菜单新增「表格密度」舒适/紧凑切换（localStorage 持久化）。
- **first-run 侧栏**：无站点时侧栏只显示核心 onboarding 路径，其余折叠「更多功能」。
- **主题 preset**：`data-accent` 3 预设（blue/indigo/teal）× light/dark 双套 --color-primary 族；主题菜单切换 + FOUC 同步。
- **Update Center**：Settings ops note + GHCR/Releases 链接。
- **Sites 内联测速**：Sites 列表行内测速按钮（client-side fetch，无需打开编辑器）。
- **downstream-key 消耗分布**：top-10 跨 key breakdown（usedCost/usedRequests 切换）组件。
- **CSV 导出**：CheckinLog / ProgramLogs / DownstreamKeys 支持导出 CSV（DownstreamKeys 不导出原始 key）。
- **GCP Console design alignment**（palette/shell density）。

### Added — i18n: English mode full coverage

- **EN 模式全链路可读**：消除所有「EN 界面显示中文/Untranslated」——t() 字面量、裸 JSX 硬编码中文、插值文本碎片、canvas 快照 PNG、VChart 图表 spec（系列名/图例/tooltip）、toast/confirm/alert 全覆盖。字典 zhToEn 扩至 200+ 条（含产品名官方名 Feishu/DingTalk/WeCom）。
- **四层 i18n 回归测试**：t() 字面量 / 裸 JSX 属性+文本 / 插值片段 / chart spec 对象字面量——任何新中文文案漏补字典即 CI 红。

### Fixed — i18n: object literal translation gaps

- EN 模式 e2e 测试（真实浏览器 MutationObserver 全链路）覆盖登录页、design gallery 组件面、会话内切换语言。
- 对象字面量值侧中文（`label: '站点公告'`、option/状态映射）此前 attr/text/表达式三面全扫不到——补键 212 条（字典达 ~700 键），回归测试新增对象字面量值侧收集。

### Fixed — i18n: review pass

- `tr()` 调用盲区清零：649 处 tr() 此前从不被扫描，76 处文案在 EN 显示 Untranslated；回归测试同扫 t()/tr()，字典补 103 键。
- 单字键不再拆碎词（汉字边界匹配，孤立才替换）；en→zh 回切不再滞留英文；用户数据豁免（站点名/账号名/模型名/公告正文新增 `data-i18n-skip`）；chart tooltip key 强制 tr()；插值 JSX 片段逐段校验；表达式 placeholder 入回归测试。

### Fixed — i18n: 534 translation keys added

- 精确键之外的短语替换/strict fallback 输出（'Startverify'/'AllEnabled'/'RemoveTag' 等）碎英文垃圾清零——三批 534 键补译（通知渠道、OAuth 管理、调试追踪、公告/审计日志、模型映射/倍率总览、路由高级参数、站点校验、账号令牌等）。

### Fixed — i18n: interpolation fragment quality

- 插值 JSX 片段输出质量清零（碎英文/缺词/粘词），51 键补译（统计/迁移/同步行、JSON 导入提示、OAuth 维护说明等）。
- 中文标点归一化（短语替换后无汉字残留时也归一化中文标点）；EN 值无汉字静态校验。

### Fixed — Today's metrics and checkin rewards

- Dashboard 与每日总结复用同一本地日界线聚合，真实返回 `todayCheckin`/`todayReward`，不再固定伪造 `todayReward=0`。
- nullable DB `*string` 签到奖励无法解析修复；奖励源不完整时标记 `partial`，Dashboard 显示 `—`。
- `/api/accounts` 每行返回 per-account 今日奖励/支出真值，无行账号为真实 0；Accounts 页不再渲染 `+0.00/-0.00` 假零。
- 修复站点可用性 LEFT JOIN 空行被计为失败，以及本地日结束边界丢失最后 1 秒。

### Fixed — Reliability, correctness, and security

- **GHCR 镜像所有权**：自动构建镜像改为 `ghcr.io/deliciousbuding/metapi-go`，与源码仓所有者一致。
- **Windows 本地运行**：未设置 `HOST` 时默认监听 `127.0.0.1`，避免临时构建路径触发入站防火墙提示；Linux/macOS 保持 `0.0.0.0`。
- **SQLite OAuth refresh**：OAuth connection list / refresh scheduler 移除 `SELECT a.*, s.*` 嵌套扫描依赖。
- **Charts dark mode**：VChart canvas 不解析 CSS `var()` → 轴/图例回退默认深色；`useChartColors()` JS 取色，7 图表轴色 + 4 图例 label 全部解析具体色值，对比度达 WCAG AA（light 6.05:1 / dark 6.09:1）。
- **Chart animations**：canvas 动画遵循 `prefers-reduced-motion`（WCAG 2.3.3）。
- **通知可观测性**：dispatch 每次派发留日志（含每失败渠道 + 错误截断）；Settings 通知渠道保存校验：启用但凭据空 → 拦截 + 列出缺失项。
- **dual-dialect encapsulation**：`store.DB` 暴露 `ExecContext/QueryxContext/QueryRowxContext/GetContext/SelectContext`（内部为 PostgreSQL 重绑占位符），业务层移除 4 处手动方言分支。
- **Review-driven fixes**：upstream success 计费改归因名、redirect 索引字典序确定性、退款保留负 outcome、audit log 提交态守卫、realtime panel 重连退避。
- **Security upgrade**：升级 `golang.org/x/text`（安全修复）。

### Changed — Bundle and tooling

- manualChunks 拆 react-vendor ——**index 461KB → 240KB**（-48%）；vchart-vendor 确认异步-only（图表全 React.lazy，不阻塞首屏）。
- 新增 `metapi-migrate` PG→SQLite 反向迁移能力（方向判定 + SQLite 方言 DDL 转换 + 占位符插入）。

## [v0.8.45] — 2026-07-20

### Fixed — 用户 id 提取正则崩溃

- NewAPI user-id discovery regex 改为 RE2-safe（移除 PCRE lookahead，Go regexp 下会 panic）。
- User-id probe 循环尊重 context cancel；不可达主机 adapter 测试改用 closed-listener URL。

### Changed — UI polish

- 控制台密度 / 系统字体 / hi-res 内容列 / gallery baseline 对齐。

## [v0.8.44] — 2026-07-19

### Fixed — PostgreSQL connection pool profiles

- configurable connection pool profiles via `DB_PROFILE`/`METAPI_DB_PROFILE`（`shared-tiny`/`normal` default/`dedicated`）；explicit `DB_MAX_*` always override.
- Inject `application_name=metapi-<hostname>` when DSN omits it for `pg_stat_activity` attribution.
- Scheduler advisory-lease: MaxOpen≤2 uses process-local lease; too-many-connections exponential backoff + log rate-limit + force-local after repeated pressure.
- Metrics: `metapi_db_connections_in_use`, `metapi_db_conn_errors_total`.

### Changed — Docs

- Pool budget design + operator recipes; deployment/README/.env.example/compose aligned.

## [v0.8.43] — 2026-07-19

### Fixed — Reliability

- multi-channel load-proof: 5xx storm channel-scoped exclude + MaxAttempts bound; 429 same-channel budget policy documented.
- Gemini stream usageMetadata later-wins + empty/zero usage does not invent tokens.

## [v0.8.42] — 2026-07-18

### Fixed

- Config validation accepts default 5-field cron expressions by auto-normalising to 6-field before parse.

## [v0.8.41] — 2026-07-18

### Fixed

- Move `proxy_logs_request_id_created_at_idx` out of base bootstrap indexes so upgrades from pre-request_id schemas do not fail before additive `sc2_004` runs.

## [v0.8.40] — 2026-07-18

### Fixed

- Explicit PostgreSQL pool budget: configurable `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / lifetime/idle-time via env.

## [v0.8.39] — 2026-07-18

### Fixed

- Round-robin `consecutiveFailCount` no longer double-increments (threshold 3 restored).
- Managed-key `used_requests` not burned on RPM/TPM admission 429 (`Allow` before consume).
- Optional Redis RPM/TPM admission rolls back window counters on deny (fail-open preserved).
- Wire `RecordManagedKeyCostUsage` on proxy success so `max_cost` advances.
- Gemini path model when body omits model; `streamGenerateContent` forces stream.
- Retention cutoffs use RFC3339 comparable to `created_at` (same-day prune fixed).

## [v0.8.38] — 2026-07-18

### Changed — Docs

- Public docs clarified: optional `REDIS_URL` for multi-instance RPM/TPM admission (sharedcount, fail-open); sticky session affinity is process-local.
- ghcr public badge bumped to v0.8.37 series.

## [v0.8.37] — 2026-07-18

### Changed — Docs

- Align README/README_EN stack badges to Go 1.26.5 + React 19 + Vite 8.

### Fixed — Reliability

- Best-effort TPM admission estimate when maxTPM is set (no invent; empty body skips).

### Fixed — Tests

- credential usage-limit multi-channel regression tests.

## [v0.8.36] — 2026-07-18

### Security

- Clear meta_monitor_auth cookie on successful admin AuthToken change (defense-in-depth).

### Changed — UI

- Migrate monitor-hint / route-enable-disabled / stat-summary / topbar brand hex to design tokens.

### Fixed — Tests

- Claude/Anthropic stream message_delta usage merge regression tests (never invents tokens).

## [v0.8.35] — 2026-07-18

### Added — UI

- Wire DownstreamKeys maxRpm/maxTpm create/edit/list.
- Tokenize login-shell hard-coded hex to design tokens.

### Fixed — Tests

- empty-filter global full-set fallback regression tests.

## [v0.8.34] — 2026-07-18

### Added — UI

- Wire DownstreamKeys proxyUrl create/edit/list.
- Wire TokenRoutes contextLength create/edit/list badge.
- Migrate hard-coded CSS hex clusters (checkin-toggle, route-enable, info-tip, model-tag-_, status-dot-_) to design tokens.

## [v0.8.33] — 2026-07-18

### Added — UI

- Migrate hard-coded .stat-icon-* colors to design tokens for light/dark.
- Wire Sites maxConcurrency in admin create/edit/list.

### Fixed

- Gemini generateContent/streamGenerateContent: reject maxOutputTokens above positive route context_length with honest 400.

## [v0.8.32] — 2026-07-18

### Security

- system-proxy/test rejects non-empty targetUrl that fails IsValidHTTPURL / IsForbiddenSiteTargetURL before probe.

### Fixed

- OpenAI /v1/responses (+ /compact): reject max_output_tokens or max_tokens above positive route context_length with honest 400 (no silent clamp).

## [v0.8.31] — 2026-07-18

### Security

- ProxyAwareHTTPClient shares RejectCrossOriginRedirect (HTTPGet/Post helpers inherit; Telegram patch idempotent).
- SiteProxy buildClients + doWithExplicitProxy share RejectCrossOriginRedirect.
- Downstream-keys update + reset-usage redact plaintext key (keyMasked only).

## [v0.8.30] — 2026-07-18

### Security

- Share RejectCrossOriginRedirect on OAuth Codex HTTP client + Telegram notify clients; public-origin 302 to different host rejected.

### Fixed

- loadRouteMatch applies source route model_pattern as SourceModel fallback when channel SourceModel blank/nil.

## [v0.8.29] — 2026-07-18

### Fixed

- Preferred/sticky channel selection respects open site/model breaker and falls through when healthy siblings exist.
- CooldownUntil eligibility parses timestamps via `IsCooldownActive`.
- Proxy conductor hard max attempt budget across same-channel + refresh + failover; cap RefreshAuth successes; nil/error RefreshAuth → sibling failover with channel-scoped exclude.

## [v0.8.28] — 2026-07-18

### Security

- Share `RejectCrossOriginRedirect` on bare clients: channel health probe, channel test harness, and `defaultUpstreamClient` (no longer follow public-origin 302 → private/metadata).
- Admin logout / session clear sets `Max-Age=0` for `meta_monitor_auth` with matching `Path=/monitor-proxy/`.

### Fixed

- Bare HTTP clients no longer inherit Go default redirect policy when site proxy is absent.

## [v0.8.27] — 2026-07-18

### Security

- Monitor session cookie is opaque HMAC (never embeds live AUTH_TOKEN); constant-time compare; cookie scoped to `Path=/monitor-proxy/`.
- Admin auth token change: constant-time `OldToken` compare.

### Fixed

- Claude `/v1/messages`: reject `max_tokens` above positive selected-route `context_length` with honest 400 (no silent clamp).

## [v0.8.26] — 2026-07-18

### Security

- `IsValidAPIEndpointURL` rejects cloud metadata / link-local targets.

### Fixed

- OpenAI chat/completions (and legacy completions): reject `max_tokens` above positive route `context_length` with honest 400 (no silent clamp).
- OpenAI chat/completions stream: warn once when stream ends without usable usage after `stream_options.include_usage`; never invent token counts.

## [v0.8.25] — 2026-07-17

### Security

- `IsValidHTTPURL` rejects cloud metadata / link-local targets; site externalCheckin URL uses the hardened check.

### Fixed

- Admin `GET /api/routes` batch-loads route channels in one query and groups in memory (kills per-route N+1).

## [v0.8.24] — 2026-07-17

### Security

- Admin routes channel list/get + `POST /api/search` redact plaintext `accessToken`/`apiToken`/`token` (masked only).
- Site create/update + API endpoint upsert reject cloud metadata / link-local URLs; RFC1918 + localhost still allowed.

## [v0.8.23] — 2026-07-17

### Security

- Admin account list/overview redacts `accessToken`/`apiToken` (masked only) and strips `passwordCipher` from list `extraConfig`; account-token list drops join credential fields.
- Credential export remains intentional product path (create/update may still echo once outside list enrichment).

### Fixed

- Round-robin / stable_first / least_* soft-filter priority demotion: soft-empty higher priority tries next layer.

## [v0.8.22] — 2026-07-17

### Security

- Redact plaintext `key` from admin downstream-keys list/summary/overview (`keyMasked` only).
- Deny-list sensitive site `custom_headers` (Authorization/Host/Cookie/hop-by-hop/Proxy-*/Content-Type); Bearer set after custom so identity cannot be overridden.
- RuntimeExecutor `CheckRedirect` rejects cross-origin and private/metadata redirect targets.

### Fixed

- Weighted routing: when soft-filter empties a priority layer, try the next priority instead of reselecting the unfiltered broken layer.

## [v0.8.21] — 2026-07-17

### Fixed

- OpenAI legacy `/v1/completions` stream: same `stream_options.include_usage=true` inject as chat.

## [v0.8.20] — 2026-07-17

### Fixed

- OpenAI-compatible chat/completions stream: inject/merge `stream_options.include_usage=true` on upstream body for final SSE usage chunks; skip codex/sub2api and non-chat paths.

## [v0.8.19] — 2026-07-17

### Fixed

- Race-harden `scheduleSiteRuntimeHealthPersistence` / `persistSiteRuntimeHealthState`: timer + in-flight flags under `healthStateMu`; concurrent success/failure regression.

## [v0.8.18] — 2026-07-17

### Added

- OpenAI `/v1/models` prefers positive `token_routes.context_length` (max per exposed model id) over `knownModelContextLength` heuristics.

### Fixed

- Admin test isolation: stop reassigning `globalAccountsCache` pointer; drain background health-refresh runners before registry reset (data race under full `-race` suite).
- Race-safe `healthPersistTimer` clear under `healthStateMu`.

## [v0.8.17] — 2026-07-17

### Added

- Admin `token_routes.contextLength` create/update + list/summary/lite surfaces (metadata-only; no proxy max-token enforcement).

### Fixed

- Usage aggregation projects `proxy_logs.status=failed` tokens into `failed_calls` + `total_tokens`.

## [v0.8.16] — 2026-07-17

### Fixed

- Wire Gemini official tool-history `thought_signature` inject/preserve on generateContent / gemini-cli paths.
- Harden multi-turn Responses reasoning content sanitize.
- Persist failed upstream attempts to proxy_logs with best-effort usage from error bodies.

## [v0.8.15] — 2026-07-17

### Fixed

- Gate `ReportTokenExpired` / checkin-balance mark paths with `ShouldMarkAccountExpired` (no bare/generic 401 over-expiry).
- Channel-scoped cascade isolation: 429 fails over, same-channel timeout budget, multi-channel same-site isolation.
- Preserve stream/partial usage on client disconnect when usage was already extracted.

## [v0.8.13] — 2026-07-17

### Added

- token_routes.sort_order + PUT /api/routes/reorder bulk drag reorder.

## [v0.8.12] — 2026-07-17

### Fixed

- Admin BackgroundTask snapshot under mutex (data race on get/list vs runner Result write).

### Added

- Site-announcement scheduler wires to real `SyncSiteAnnouncements` via SyncFunc.
- Channel recovery active candidates via optional `ProxyChannelCoordinator` provider hook.

## [v0.8.11] — 2026-07-17

### Added

- DB-backed durable admin BackgroundTask store (cross-instance list/get).

### Fixed

- Frontend CI EnvironmentTeardownError flake hardening.

## [v0.8.10] — 2026-07-17

### Added

- Sub2API refresh scheduler wires to RefreshBalance.
- Proxy video task age-based retention scheduler (config-gated, default off).

## [v0.8.9] — 2026-07-17

### Added

- Videos GET/DELETE sticky pin via ForcedChannelID from mapping ChannelID.

## [v0.8.8] — 2026-07-17

### Added

- Durable `proxy_video_tasks` dual-write for video publicId mapping (multi-instance / restart).
- TPM multi-instance sharing via optional Redis sharedcount (fail-open, mirrors RPM).

## [v0.8.7] — 2026-07-17

### Added

- Videos create: process-local publicId mapping + response `id` rewrite on successful POST /v1/videos.

### Fixed

- ResolveInputFile returns explicit error (no silent vault).

## [v0.8.6] — 2026-07-17

### Fixed

- Videos GET/DELETE honest upstream passthrough without empty local-store 404 theater.

### Added

- Downstream key maxCost/maxRequests clear-to-NULL.
- ParseInputFiles extracts OpenAI input_file/file body refs.

## [v0.8.5] — 2026-07-17

### Added

- Site initialization preset registry + create/detect validation.
- Gemini `/v1beta/models` from owned model catalog.
- Site proxy cache invalidation hooks (routing + admin accounts snapshot).
- Responses WebSocket boot wiring.

### Fixed

- Shared PG CI: prefer `SiteSelectColumns` over `SELECT * FROM sites`.

## [v0.8.4] — 2026-07-17

### Fixed

- PostgreSQL CreateSite: RETURNING id + explicit sites column select.
- Multipart `/v1/images/edits` forwards via dispatchUpstream.

### Added

- Expired API-key account recovery on credential update (allowInactive model refresh + reactivate).
- Account token groups via platform.GetUserGroups with local fallback.

## [v0.8.3] — 2026-07-17

### Added — Admin features

- sub2api managed auth merge on account update/rebind.
- Real account health-refresh via balance probe.
- OAuth start/rebind CSRF state tokens (server-stored, TTL).
- Honest update-center deploy/rollback + real clear-cache invalidation.

## [v0.8.2] — 2026-07-17

### Added

- Account token create/delete/sync via platform adapters + SyncTokensFromUpstream.
- Account create fail-closed VerifyToken / GetModels.
- Real system-proxy probe + brand list from platform registry.
- `/api/test/proxy` + `/api/test/chat` wired to forced-channel harness; stream/jobs 501.

### Known limitations

- sub2api managed auth on update, expired API-key recovery model refresh, async health-refresh job, OAuth state stubs.

## [v0.8.1] — 2026-07-17

### Fixed

- Go 1.26.5 toolchain; vulncheck green.

### Added

- Live /v1/models listing via TokenRouter.GetAvailableModels.
- Boot-wired ModelProbeScheduler probe executor + health recorder.
- Route decision admin APIs wired to ExplainSelection.

## [v0.8.0] — 2026-07-17

### Added

- Request trace IDs across retries/failovers.
- Per-request cost attribution + cache token types.
- TTFT/first-byte signals in routing health.
- Cross-site model price comparison APIs.
- Background channel health probing.
- Pluggable routing strategies: least_busy / lowest_latency / lowest_cost.
- Downstream-key RPM/TPM soft admission + Retry-After.
- Richer Prometheus histograms/labels + MetricsObserver export hook.
- Optional Redis-backed shared RPM admission (fail-open).
- Admin forced-channel test harness.
- Client credential export adapters (openai/cherry/generic).
- Usage heatmaps + slow-request ranking stats.

### Known limitations

- `vulncheck` may still flag Go 1.26.4 stdlib advisory until a Go patch is available; CI continues with continue-on-error.

## [v0.7.0] — 2026-07-17

### Added

- Enterprise modernization program (stack TS7/React19/Vite8, UI tokens/a11y, backend boundaries, schema additive migrations, reliability source of truth).
- Feature completeness from original metapi gap matrix: site max concurrency, per-key proxy, group route rebuild, `/v1/rerank`, usage/token accounting, failover/first-byte, protocol pack (Gemini thought_signature, Minimax thinking, models shape, previous_response_id, skill-call, responses multi-turn reasoning, responses-only sites), Codex OAuth gpt-5.5 + discovery soft-retry.

### Fixed

- Admin correctness: key refresh name/enable preserve, quota clear, model whitelist non-destructive parse, in-route model config preserve, expired account health.
- Frontend CI flake: dashboard site-observability EnvironmentTeardownError hardening.

### Known limitations

- `vulncheck` may still flag Go 1.26.4 stdlib advisories; CI keeps continue-on-error until Go patch available.

## [v0.6.5] — 2026-07-10

### Fixed

- 修复 Content-Security-Policy 缺少 `frame-src` 导致第三方 iframe 被拦截。

## [v0.6.4] — 2026-07-10

### Fixed

- 修复 Content-Security-Policy 过紧导致 dicebear 头像图片和 Cloudflare Insights 脚本被浏览器拦截。
- 新增 `img-src 'self' https://api.dicebear.com`、`connect-src 'self'` 和 `script-src https://static.cloudflareinsights.com` 指令。

## [v0.6.3] — 2026-07-07

### Fixed

- 修复后台 Admin API 被重复挂载成 `/api/api/*` 的生产路由问题，恢复管理接口正常访问。
- 登录页增加登录前明暗/跟随系统主题切换，并修复深色模式下品牌面板、链接和图标对比度。

## [v0.6.2] — 2026-07-07

### Fixed

- 修复根路径 WebUI 被非 `/v1` 代理别名鉴权拦截的问题，确保嵌入式 SPA fallback 正常返回前端页面。
- 修复嵌入式前端文件系统路径兼容性，支持 `web/dist` 作为 embed 根。
- 稳定 routing golden 与加权随机测试，避免 Windows CRLF checkout 和单次随机抽样导致 CI 偶发失败。

## [v0.6.1] — 2026-07-07

### Fixed

- CI/CD secret scan 改用开源 gitleaks CLI。

## [v0.6.0] — 2026-07-07

### Security

- CI/CD 发布流程加入 gitleaks、Go module 校验、race 测试、PostgreSQL integration 测试、前端 typecheck/test/build 和生产依赖 audit。
- CD 镜像发布前执行 Docker smoke test；发布镜像启用 provenance 和 SBOM。
- 测试和文档中的 PostgreSQL DSN 改为运行时拼接，减少 secret scanner 噪声。
- 站点自定义 headers 过滤保留头，避免覆盖运行时认证语义。

### Fixed

- `/v1/*` 数据面接入数据库路由和真实上游选择，不再停留在未配置 stub 行为。
- 上游代理支持站点/账号代理、自定义 headers、失败记录和非流式可重试 failover。
- AnyRouter 禁用 NewAPI 风格 token 管理端点，避免错误调用 `/api/token`。
- API-key/proxy-only 账号不再执行签到或余额上游调用，禁用状态判断改为大小写不敏感。
- 账号 session rebind、manual models、account token 默认值维护补齐事务和错误处理，失败路径回滚。

### Added

- 覆盖 SQLite 和 PostgreSQL 的回归测试。
- 运行时说明明确支持 SQLite 单节点与 PostgreSQL 部署；Redis 尚未集成。

## [v0.5.0] — 2026-07-05

### Security

- Admin/proxy token 比较改用 `crypto/subtle.ConstantTimeCompare`（防时序攻击）。
- CI 启用 `errcheck`、`staticcheck`、`ineffassign` linter。
- CI 测试启用 `-race`（data race 检测）。
- `/debug/vars` 移至 admin auth 保护后。
- 安全响应头中间件：`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `CSP`。
- AES 密钥派生不再 fallback 到 `AUTH_TOKEN`（独立默认值）。

### Fixed

- 代理出口 `http.DefaultClient`（零超时）→ 接入 `RuntimeExecutor`（90s 超时 fallback）。
- 6 处 OAuth `panic()` → `return error`。
- SSE 流式响应 `WriteTimeout: 60s` 截断问题 → `SetWriteDeadline` 禁用。
- 13 处 `log.Printf` → `slog.Warn/Error`。
- DB 连接池补充 `ConnMaxLifetime`(5min) + `ConnMaxIdleTime`(2min)。
- `usage_aggregation` goroutine re-panic 修复。
- `CheckinScheduler.Stop()` data race 修复。
- CI：`golangci-lint-action` Go 1.25 不兼容 → `go install` 最新版；全项目 zero warning。

### Added

- `/metrics` Prometheus 端点（零依赖 text format）。
- `RequestID` 中间件（`X-Request-Id` header + 日志关联）。
- `handler/shared/errors.go`：`APIError` 结构化错误类型。

### Tests

- 8 个零覆盖包全部补齐测试（最低 50%，最高 100%）。
- 新增 3 个 e2e 场景：并发代理、auth 时序安全、rate limit 拒绝。
- `e2e` 测试包总数：4 → 5 文件。

## [v0.4.0] — 2026-07-05

### Fixed

- PG 兼容：`INSERT OR IGNORE` → `ON CONFLICT DO NOTHING`。
- Cron 5 字段 → 6 字段自动转换。
- `sqlx.BindDriver` 时序修复（`?` → `$N` 占位符重绑定）。

## [v0.3.0] — 2026-07-04

### Fixed

- goroutine 泄漏修复。

### Changed

- JSON 性能优化；包命名规范化；`config.Validate()` 10 项启动校验。

## [v0.2.0] — 2026-07-04

### Added

- 限流中间件（admin 100rps, OAuth 10rps）。
- RWMutex 假桩替换为真实 `sync.RWMutex`。
- DB 事务包裹 usage aggregation batch。
- `store.Close()` 优雅关机。

## [v0.1.0] — 2026-07-03

### Added

- Metapi TypeScript → Go 完整重写初始发布。
- 27 表双数据库（SQLite + PostgreSQL）。
- 14 平台适配器。
- 4 协议流式转换。
- 15 后台调度任务。
- 单二进制 + Docker 部署。

[v0.9.0]: https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.9.0
