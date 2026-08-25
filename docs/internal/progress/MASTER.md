# Roadmap

**Last verified**: 2026-08-25

**Release**: [v0.16.13](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.16.13) · released on master; production promotion follows the release and soak gate

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Current active work — Wave 12 demand truth

权威分析与拆分：[`../analysis/wave12-demand-truth.md`](../analysis/wave12-demand-truth.md)。执行分两次 patch-first 发布：

- **Wave 12A → v0.16.12**：#996 Sites 分页、#999 API key 模型策略 UI、#1001 账号表单站点搜索、#1000 公告进入 attention + #997 待处理语义澄清、截图数据 profile 真值门禁（已合入）。
- **Wave 12B → v0.16.13**：#1002 session 账号创建后 service-owned token sync（失败不回滚账号）→ #998 上游模型刷新/手工模型管理/创建后持久化；周期 scheduler 延后到明确需求（已合入）。
- **共享写点唯一归属**：locale、CHANGELOG、package version、STATE/MASTER/log 仅 integration lane 修改；实现 lane 不并发改共享文件。
- **当前阶段**：Wave 12A/12B 均已合入（→ v0.16.12 / v0.16.13）；#996-#1002 全部关闭；无执行中波次，下一波待需求驱动。

历史完成冻结：Round 1 #887 → v0.16.3、Round 2 #889 → v0.16.4、#887 补遗 + E2E → v0.16.5、Round 3 修复波 → v0.16.6、Wave 4 综合质量波 → v0.16.7、Wave 5+6 开发 + 深审计波 → v0.16.8、Wave 7+8+9 前端体验/语义/设置/catalog/移动端审计波 → v0.16.9、Wave 10 Sites demand batch → v0.16.10 、Wave 11 UX 真值波 → v0.16.11 均已发布。

### Wave 11 已收口说明

- 四 lane 全部落地（batch 分支 `batch/w11-ux-truth`）：A 路由页视图持久 + 横幅语义测试补齐（#862 行为核验已在基线），B accounts 行级 pending + 对比行 re-run，C OAuth start 流闭环（有界轮询 + 手动回调），D vitest ignore flag 移除 + a11y 41 路由单一来源 + golden 10 页。
- a11y residual（moderate/minor，不强修）：region / landmark 双 main 布局 / image-redundant-alt / catalog-sources empty-table-header。

### 已收口（v0.16.9 及以前，勿再当 active）

- Wave 11 UX 真值波（→ v0.16.11）：见上节收口说明。
- Wave 10 Sites demand batch（#985+#986 → v0.16.10）：站点快捷跳转链接、`/site-announcements` 独立 SPA、公告 API camelCase 契约与诚实同步错误、SSRF dial-time 守卫（internal/ssrf）、newapi/donehub 信封校验；配套 #991（pre-push 链 + release freshness 门禁）、#992（产品公告前端真值）。


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

### 已收口的 UI/UX 深修（#975–#984）

2026-08-24 的错误态、i18n、a11y、死代码、USD 格式、pagination/search/navigation 与 credential terminology 批次均已合入；详细时间线见 [`../log.md`](../log.md)，不得再作为 active work 重做。

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
- **Onboarding polish + team visibility (shipped by v0.16.2)**: platform picker (16-adapter searchable Select with manual-entry fallback), client export breadth (claude-code/codex/openwebui profiles), and downstream-key 24h usage summary (requests/tokens/cost) are implemented and covered by focused Go + frontend tests.

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
