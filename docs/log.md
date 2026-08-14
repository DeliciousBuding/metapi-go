# log.md — MetAPI Go product milestones

**Last updated**: 2026-08-14

> Product milestone timeline (grouped by version). Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../CHANGELOG.md)

## 2026-08-14 — v0.12.0 架构简化（净删 ~21K 行）

- **死代码大清扫**（#650–#653，5 个 PR）：删 test-only 的 canonical 转换层（~6.6K）、三套测试专用编排层（conductor/surface/endpoint_flow）、未激活的 lease/sticky 队列、死 facade（app/prometheus、shared 半套 metrics/errors、input_files、检测管线、service/adapter 桥接）；Go 170K→150K，无生产行为变化
- **数据完整性修复**：cmd/migrate 自维护 schema 副本已漂移（sites 漏 7 列）→ 复用 store.AutoMigrate + 运行时漂移守卫；app/proxy_upstream.go 垃圾抽屉移入 service/routing_store.go
- **去重 + 标准化**：手写 stdlib 重实现（Hinnant 日历/JSON/int/TrimSpace）→ 标准库；handler/routing/scheduler/platform 各去重 3-11 份复制；god-file 按自然接缝拆分（含前端 api.ts 1997→10 模块）
- **文档对账**：architecture/BACKEND/package-boundaries 同步为 as-built 真相（无 canonical、无 Conductor 编排）

## 2026-08-14 — v0.11.0 管理控制台 UI/UX 全量 + 开源打磨

- **管理控制台 UI/UX 与功能全量交付**（#594–#626，合并为 #633/#634）：共享格式化器、状态/空态组件、toast、CountUp、响应式 data-table + 移动端降级、无障碍、token 列脱敏、可观测性工作台（Overview/Health/Proxy Logs）、站点探测 + 统一导入向导、模型定价/价格对比/重定向修复、通道只读列表 + probe 驱动重建过滤
- **可观测性**：访问日志 status/bytes/duration_ms + statusRecorder 转发 SSE/WebSocket + slog panic 恢复带 request_id + /metrics 纯 stdlib go runtime 指标（#593）；/api/routes/rebuild 响应新增 changed 统计
- **调度器注册回归修复**：balance-refresh / log-cleanup / backup-webdav 曾构造未 Register()，已注册并加 16 集合回归测试（#635）
- **CI/CD 单一管道**：test → 镜像推送 → GitHub Release 合一（main.yml）；master push 推 latest+sha 镜像；SemVer tag 另建 5 平台二进制 + checksums + 二进制冒烟 + Release（#637）
- **升级与打磨**：旧 schema 增量 ALTER 回归测试 + forward-only 迁移文档（#636）；docker-compose.prod 安全硬化（#639）；重新启用 unused/gosimple + 删死代码（#641）；a11y 标签 i18n + SSE any→unknown（#642）；docs/api.md 端点清单（#643）；.env.example 补齐（#640）
- **类型/卫生收尾**：api.ts payload/response any→unknown（#646）；Go 1.26.6 升级（#647）；清理过期子代理拆分注释（#648）

## 2026-08-12 — v0.10.0 设置中心 + 调度规格 v1

- **ScheduleSpec v1**：daily / interval / random-window / custom cron 四种调度规格，additive preview/apply 迁移端点保留所有 legacy key
- **设置中心**：统一表单 actions + load-error 态 + dirty 导航守卫 + 语义化调度控件 + 响应式导航；DB 设置页统一 dirty-guard，应用内 501 migrate 按钮改为 CLI-only 迁移提示；审计日志服务端分页 + 分页表 UI
- **CJK 字体回退**：--font-sans 加 Noto Sans SC/TC/JP/KR + PingFang SC + Microsoft YaHei；移动端侧栏 header 触发器（md:hidden）

## 2026-08-11 — post-v0.9.0 UI completion batch

- **Git workflow（GitHub Flow 落地，已实际启用）**：master 唯一长期分支 + 分支保护（要求 PR + 11 CI 检查必选 + enforce admins + 禁强推/删除）+ 仓库级 Squash-only 合并 + PR 模板；规则文档 `docs/git-workflow.md`；ci.yml 移除 paths-ignore（必选检查与跳过互斥）
- **v0.9.0 发布补推（此前仅本地）**：本地 master 18+ commit（v0.9.0 重写 + UI completion）推上 GitHub；`v0.9.0` tag + Release 创建；CD 双跑成功 → `ghcr.io/deliciousbuding/metapi-go` 发布 `latest`/`0.9.0`/`0.9`/sha 镜像（首跑暴露 CI 盲点并修复：go:embed 需 web/dist，测试 job 构建真实前端；responses-websocket doc 指针 `.md` → 真实文档）
- **前端修复收尾**：Base UI render prop + TanStack Link 冲突（侧栏点击 JSON circular 崩溃）→ SidebarNavLink 只透传 DOM-safe props；searchParams parse/encode 分离消除 `?sort=%5B%5D` URL 噪声；updateCenterReminder/updateCenterPresentation（501 residual 幽灵前端）删除
- **品牌 rename → MetAPI**：display name unified (identity-branding / locales / About / index title); transparent SVG badge `logo.svg` (gradient rounded-square + real π glyph) + `favicon.svg` replace the white-background PNG; router root-file whitelist + table-driven regression test extended to `image/svg+xml`
- **i18n language switcher**: header `LanguageSwitcher` dropdown (en/zh-CN) + browser auto-follow (localStorage → navigator) + `documentElement.lang`/`dir` sync via `toBcp47`; locale parity now 1381 keys each, bidirectional 0 missing
- **URL-synced tables fix**: sites/models/oauth/site-announcements read the router location (`useLocation` + `searchStr`) so sort/pagination now update the table in place instead of waiting for an unrelated re-render
- **Copy audit**: terminology unification (启用/停用, 额度, Check-in, 通道), internal plan codes (K1a/N9a) removed from user-visible copy, tokenRoutes toast/chain-banner concatenation bugs fixed, 9 hardcoded strings → t() (incl. TokenDance brand leak removed from the public settings copy)
- **Visual polish**: sign-in real logo mark + brand glow + lg CTA; dashboard StatCard skeletons + useId-unique gradients + iconized empty/error states + pulsing WS indicator; settings responsive drill-in + sticky sidebar; authenticated-layout scroll-clip fix (content taller than viewport was unscrollable)
- **Theme customizer**: header Palette panel with 4 axes (10 color presets / font Auto-Sans-Serif / radius 6 / scale 4) + per-axis & global reset; **default font is sans for every preset** (Anthropic no longer inlines serif; serif is an explicit choice); legacy FontProvider dual-track removed
- **Sidebar i18n**: nav titles moved to keys (sidebar.groups/items/backToHome, en + zh-CN), sidebar renders fully in the active language

## 2026-08-11 — v0.9.0 frontend rewrite

- **Frontend rewrite**: newapi stack 100% alignment — Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH tokens (details in root `CHANGELOG.md`)
- **i18n key-based**: i18next + en/zh-CN locales each 1369 keys, bidirectional 0 missing; MutationObserver dictionary + `tr()` sweep retired, replaced by vitest `i18n-keys.test.ts` gate (1151 keys scanned)
- **Tooling**: npm → Bun (CI/Dockerfile/Makefile); Playwright + `ui-visual` removed; old frontend tree (`App.tsx`/pages/components/styles/e2e) deleted

## 2026-08-02 — v0.8.46 → v0.8.52

- **PG dialect hardening**: 4 bare `*sqlx.DB` `?` placeholders hitting PG directly (SQLSTATE 42601) all wrapped with `db.Rebind`; added static check `pg_rebind_gate_test.go` to prevent regressions
- **v0.8.49+**: `sc2_008` migration `BOOLEAN DEFAULT` PG 42804 and balance_history UPSERT 42601 — two PG dialect blind spots fixed; CI PG integration tests serialized (`-p 1`)
- **Frontend i18n sweep**: 414 bare Chinese JSX text nodes wrapped with `tr()`; added `i18n.gate.test.tsx` static check
- **Chart series color token wiring**: VChart canvas series colors — 27 `var(--color-chart-N)` silent fallbacks → `useChartColors()` JS lookup + chart-colors check
- **Design token unification**: RealtimeOpsPanel real-time traffic badge hardcoded rgba → `--color-*-soft` token
- **Feishu notifications**: TaskTag signature aggregation anti-spam (same-class alerts merged within cooldown window) + notification channel save pre-validation
- **Resource integrity three layers**: dist self-check + CD build verification + deploy live smoke test
- **Login page refactor**: single-card layout + dark de-gradient; root static files (logo/favicon) SPA fallback image-swallow fix

## 2026-07-31 — Feature batch

- **All features shipped**: model redirect mapping, snapshot PNG, risk banner, tag system, batch validation, chart gallery, randomized window scheduling, backup import preview, scheduler task run history
- **Downstream key hardening**: downstream key IP allowlist/blocklist (security gap), public price endpoint, inference suffix parsing, spend distribution dashboard, CSV export, configurable cache multiplier; remaining items honestly deferred
- **Frontend UX pass**: RouteErrorBoundary, SearchModal real keyboard navigation, Toast a11y, design-system state trio (Empty/Loading/Error), Models→Playground quick jump, ProxyLogs date presets

## 2026-07-30 — Engineering optimization + parity review

- **Package boundary enforcement**: `docs/package_boundary_test.go` turns BACKEND.md §2.3 eight hard rules into a `go test` check; caught and remediated `scheduler/lease.go` stale exception on the spot
- **Product parity review**: Go rewrite lost no TS README product features; 14 platform adapters TS=Go aligned; multi-user/payments/redemption-codes/invitations/subscriptions explicitly not applicable
- **Dual-dialect encapsulation**: `store.DB` gained Context methods (rebind `?`→`$N`), removed 4 manual dialect branches

## 2026-07-20 — v0.8.45 RE2-safe

- **RE2 panic fix** (production crash root cause): NewAPI user-id extract compiled PCRE lookahead `(?!\d)` → Go RE2 panic; switched to pre-compiled regex + 8-digit length cap
- **Four-track original feature alignment**: frontend 18 routes + 14 sidebar 100% parity; 14 platform adapters fully aligned; 16 scheduler tasks covered; WS/Sticky/UC explicitly residual
- **Original parity plan** (ex-Electron): WS full TS parity, sticky single-instance honesty, UC hide/external

## 2026-07-19 — UI polish milestone

- **UI polish batch**: Traffic sparkline, real page scoring, axe a11y smoke, Dashboard getting-started, Sites banner
- **UI parity inventory**: sidebar 18 routes at parity, Sites/Accounts/Tokens/Routes/Settings button counts at parity
- **Focus management shared**: `useFocusTrap` wired into SearchModal / CenteredModal / MobileDrawer / NotificationPanel; skip-link accessibility jump
