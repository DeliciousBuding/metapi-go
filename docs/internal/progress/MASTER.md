# Roadmap

**Last verified**: 2026-08-24

**Release**: [v0.16.9](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.16.9) · released on master; production promotion follows the release and soak gate

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Current active work — 需求驱动（v0.16.9+）

历史完成冻结：Round 1 #887 → v0.16.3、Round 2 #889 → v0.16.4、#887 补遗 + E2E → v0.16.5、Round 3 修复波 → v0.16.6、Wave 4 综合质量波 → v0.16.7、Wave 5+6 开发 + 深审计波 → v0.16.8、Wave 7+8+9 前端体验/语义/设置/catalog/移动端审计波 → v0.16.9 均已发布。

### 已收口（v0.16.9 及以前，勿再当 active）

- Wave 5：功能闭环 #861（#935 apiEndpoints 编辑器）、#558（#939 探针产品化）、#926（#938 后端消息英文化收官）；测试矩阵（#937 真实上游 4/16 + 运行时矩阵 4/5）；截图证据管道 + golden 门禁（#951）；审计残留 P1（#936）；docs/api.md 74 条补齐（#940）。
- Wave 6 六维深审计（架构/动线/视觉/性能/安全，22 项坐实全修复）：P0 备份凭据植入（#941）、全失败告警（#942）、可空列同族（#943）、代理零拷贝（#944）、proxy-logs 单遍+缓存（#945）、后端卫生（#946）、路由缓存（#947）、UX 动线 4 项（#948）、对比度 6 项 + 320 对守卫（#949）、前端卫生（#950）。
- Wave 7 前端体验整修（#970）：12 域 55 项交互/视觉/移动端/无障碍修复，含侧边栏「路由」导航静默失败。
- Wave 8 模型数据源 + 设置 IA + 产品语义（#971）：llm-metadata/models.dev 双源注册表、settings 语义重组、14 条产品语义修复。

### Wave 9（#972）—— 已收口

| Lane | 内容 | 状态 |
|---|---|---|
| a | keys 页 data-table auto-reset 渲染环修复（EMPTY_COLUMN_FILTERS + shallow-equal 去重） | 已并入 integration，375×812 Chromium 5 次侧栏开合无冻结 |
| b | settings 语义重组（五子域 + 旧 URL redirect + catalog-sources 拖拽排序） | 已并入 integration，41 desktop/19 mobile smoke + redirect + 拖拽持久全过 |
| c | catalog ratio 倍率计价 + supportedEndpointTypes 目录推导（含 dialect 修复） | 已并入 integration，真实实例验证 anthropic 方言 |
| d | 375×812 全站移动端逐页真实交互深审（P0-P2 清单 + 修 P0/P1） | 已并入 integration；修 2 处 <24px 行内按钮（checkin 查看原始信息 / rates 行内编辑）→24px；其余硬信号逐条核验为误报 |

> 已交付：integration → PR #972 squash 合入（12-check 全绿）→ #973 bump CHANGELOG + web/package.json → tag v0.16.9 → Release 发布。生产滚动部署为运维面后续动作。

### 需求驱动候选（开放 issue 0，下波按需求立项）

> 对比度门禁已全量达标（0 豁免，10 preset × light/dark 全过 WCAG AA 4.5:1）。无 P0/P1 开放项，开放 issue 0。下一波按需求驱动立项（方向见 `../benchmark.md`）。

### UI/UX 深修波（进行中：自行探索 + 整理 + 优化）

> 2026-08-24 三轮只读探索（token-routes/models/model-tester · settings/sites/accounts/channels · dashboard/observability/checkin/oauth/import + 全局横切）产出证据化清单，按优先级分批落地，每批带回归测试与门禁。

**Batch 1（已合入 #975）—— 错误态一致性 + i18n + 文档漂移**
- accounts/channels 页 load error 时同时渲染 QueryErrorBanner 与 DataTablePage 空态（含误导 CTA）→ 镜像 sites 页三元分支抑制 table（+ 回归测试）。
- import stepper `aria-label` 硬编码英文 → `t('import.stepper.progressLabel')`（en/zh 词条补齐）。
- a11y-checklist §7「preset contrast」残留清单过时（Wave 9 已清零）→ 改为 0 豁免现状。

**Batch 2（本 PR）—— token-routes Edit + decision 语义 + stream i18n**
- token-routes detail sheet footer 增「Edit」（gap-6），routes-page 接线（关 sheet → 开 edit dialog）；decision snapshot selected channel 显示 account 名。
- transport.ts:151 硬编码中文流式错误 → 英文技术消息。

**Batch 3（本 PR）—— model-tester 错误态 + schedule-editor 可访问标签**
- model-tester：models/channels 查询加载失败时静默空下拉 → 接入 QueryErrorBanner + Retry。
- schedule-editor：daily/window/custom 的 time/cron 输入补 aria-label（en/zh 词条）。

**Batch 4（本 PR）—— 死代码清理**
- model-detail-sheet 移除从未传入的 `onTest` prop +「Test model」死按钮 + 图标 import + 死 i18n key。
- token-routes/index.ts 删除空占位注释块（barrel stub 收敛为真实重导出）。

**Batch 5（本 PR）—— i18n 收尾 + 装饰图标 a11y**
- catalog-sources Switch aria-label、tokens-panel group placeholder 去硬编码 → i18n（`toggleEnabled` / `groupPlaceholder`）。
- checkin 装饰性 CalendarRange 补 `aria-hidden`。

**Batch 6（本 PR）—— 死 wrapper 清理 + import 静默失败**
- lib/api/sites.ts 移除 4 个未用 API wrapper + `SiteProbeNowResponse` 类型 + 测试 mock。
- import wizard：非 axios 失败（如空响应体）补 `toast`（`import.submitFailed`），不再静默吞。

**已知残差（记录，不作为待办）**
- manual-checkin `catch {}`：用户可见失败均由 http-client 或结果分支覆盖，仅后端 schema 漂移的编程错误会静默；naive 补 toast 会对 `success:false` 二次提示。
- `$` 前缀：已收口 — `lib/format` 单一 `USD_SYMBOL`，`formatCurrency`/`formatPrice` 统一金额与每百万价，删除 `formatUsd`/`formatAmount`/`formatRoutePrice` 及本地 `$` 拼接；定价固定 USD（见 log 2026-08-24）。
- `#siteId` 回退：位于纯函数 `resolvePriceDetail`，需透传 `t` 才可本地化，低频残留。
- test-response-viewer useEffect 无依赖：对话区每次渲染滚到底为流式跟随意图，非缺陷。
- endpoints-editor URL：已有 `aria-label`，仅 WCAG 3.3.2 可见标签待补。

## Completed milestone — TS 兼容与迁移收官（2026-08-20 交付）

分析依据：[`../analysis/ts-go-migration-gap.md`](../analysis/ts-go-migration-gap.md)。全部批次已交付合并（#880 计划 / #881 后端 / #882 UI / #883 备份导入 / #884 文档）：

- CLI 诚实化：`--verify` 失败非零退出、buildSummary 真实方言、`--batch-size` 生效（#881）
- 反向检测：`__drizzle_migrations` 判龄 + 未知列扫描 → 启动警告不阻断（#881）
- 启动汇总日志：`store: converged legacy schema`（#881）
- 管理 UI 数据迁移：危险确认 + task 轮询进度（#882）+ overwrite 字段后端补解析（#881）
- TS 备份 v2.1 导入兼容（#883）
- 迁移文档三场景（SQLite/PG 直接接管、MySQL 经 TS 自带迁移）+ 版本锁定（#884）
- PG 直接接管实测：真实 TS 契约 PG 库 → Go 启动 `/ready` OK、数据可读、additive 收敛（实测，无代码改动）

## Delivery model

Metapi Go has **3 delivery mainlines**. CI, dual-dialect support, security, release automation, and documentation hygiene form one cross-cutting engineering baseline, not a fourth product line.

| Mainline              | Current state                                                                                                                                 | Remaining outcome                                                                                                                                      |
| :-------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A. 路由与成本真值** | Core proxy, retry/cooldown/breaker, routing strategies, usage pricing, models.dev cold-start catalog, half-open recovery, and route price truth are implemented. | Collect operator-gated live multi-channel cascade evidence; keep multi-instance limits explicit. |
| **B. 上游兼容与实测** | 16 adapters; real new-api/one-api CI e2e plus Sub2API/CLIProxyAPI verification chains; live-platform defects are covered by CI and focused tests. | Run optional Codex and AnyRouter probes from [#558](https://github.com/DeliciousBuding/metapi-go/issues/558). No blocking compatibility issue is open. |
| **C. 分发与管理体验** | Client export, global search, daily snapshot, enriched alerts, tester history/templates, guided onboarding, truthful channel comparison, and URL-owned table state are shipped. | No implementation slice is open; future work must be promoted here with an owner and acceptance criteria before coding. |

## Execution rules

1. Implement the smallest end-to-end slice in the existing feature owner; do not add a wizard framework, pricing facade, batch service, or job queue.
2. Reuse existing contracts. A missing UI connection is not grounds for a duplicate backend endpoint or a second source of truth.
3. Validate at trust boundaries. Inside typed flows, prefer explicit invariants over repeated fallbacks and speculative compatibility branches.
4. Unsupported behavior stays explicit. Do not turn a 501 residual, unavailable price, failed probe, or missing credential into fake success.
5. Each slice lands with its focused regression test and updates `STATE.md` only after the behavior is verified. Historical detail belongs in `CHANGELOG.md` / `log.md`, not here.

## Completed delivery outcomes

- **Guided onboarding**: site → account → route handoff, one-shot deep links, typed IDs, credential verification, redacted-credential edit behavior, and partial batch failure truth are implemented and covered by focused tests.
- **Tester truth**: forced-channel synchronous probes, explicit unsupported streaming behavior, enabled-channel filtering, bounded comparison concurrency, shared abort handling, and stopped-result rendering are implemented and covered by focused tests.
- **Route truth**: enabled-weight allocation and account/model-specific price provenance use exact concrete-model joins and are covered by focused Go and frontend tests.
- **URL state stability**: list-page table state has one URL owner, stable callbacks, latest-URL merge semantics, and real Chromium acceptance gates documented in [`../design/state-stability.md`](../design/state-stability.md).
- **Onboarding polish + team visibility (v0.17)**: platform picker (16-adapter searchable Select with manual-entry fallback), client export breadth (claude-code/codex/openwebui profiles), and downstream-key 24h usage summary (requests/tokens/cost) are implemented and covered by focused Go + frontend tests.

These outcomes are closed. New work enters this plan only with a concrete owner, scope, and acceptance test; completed waves do not remain as open checklists.

## Evidence closeout

### A. Live cascade proof — operator-gated, no coding by default

- Use [`../analysis/p0585-production-verification.md`](../analysis/p0585-production-verification.md), `scripts/verify-cascade-prod.sh`, and optionally the `staging` Go test against an operator-controlled multi-channel topology.
- Trigger one controlled retryable failure outside normal production traffic. Evidence must show one stable request ID, distinct channel IDs, increasing retry counts bounded by `PROXY_MAX_CHANNEL_ATTEMPTS`, and either sibling recovery or an explicit bounded all-fail result.
- Keep raw reports, host details, and credentials in the private operator surface. Record only the sanitized date/verdict/evidence pointer in public state.
- If the evidence reveals a defect, create a narrowly scoped fix. Do not pre-build another cascade layer before that evidence exists.

### B. Optional Codex / AnyRouter probes — #558

- AnyRouter: reuse `scripts/e2e/verify-token-import.sh` and record adapter detection, token verification, model listing, and one minimal request when credentials permit.
- Codex: use the existing OAuth/import flow and record one minimal supported request; do not introduce CI secrets or a second OAuth implementation.
- Done means [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) contains reproducible sanitized evidence, or names the exact credential/environment limitation and the command that remains to be run. “Present” without a live call is not completion.

## Engineering baseline and release gate

Every wave inherits root [`AGENTS.md`](../../../AGENTS.md) and backend simplicity rules in [`../design/BACKEND.md`](../design/BACKEND.md).

- Frontend slices: i18n parity, focused Vitest coverage, `bun run typecheck`, `bun run lint`, `bun run knip`, and production build.
- Any Go slice: SQLite + PostgreSQL-safe implementation, focused tests, then `go build ./cmd/server`, `go vet ./...`, and `go test ./... -count=1 -race` before push.
- No new dependency unless the existing stack cannot express the slice directly.
- Release cadence is **patch-first** ([`../git-workflow.md` §6.1](../git-workflow.md)): each merged wave with user-visible changes bumps the patch digit; minor only for themed milestones; major stays `0` until the 1.0 readiness criteria.
- Update `STATE.md` and this plan in the same PR that closes an outcome; do not leave completed checklists here.

## Deferred or out of scope

- Demand-gated: encrypted WebDAV sync, mobile PWA, Realtime transport.
- Explicitly out of scope: multi-tenant billing/wallet/subscription/payment and shared multi-instance sticky sessions.
- Update-center deploy/rollback remains external through GHCR and GitHub Releases; the admin API stays an honest 501 residual.
- Historical UI audit observations remain evidence in [`../analysis/ui-ux-audit-2026-08.md`](../analysis/ui-ux-audit-2026-08.md). They become commitments only when promoted here with an owner and acceptance criteria.
