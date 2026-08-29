# Roadmap

**Last verified**: 2026-08-29

**Release**: [v0.16.19](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.16.19) · released on master; production promotion follows the release and soak gate

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Current active work — 无执行中 wave（Wave 19 候选待挑选）

Wave 18 已收口并发布 v0.16.19（见下方已收口节）。下一波候选（后端优先）：

| # | 内容 | 建议验收 |
|---|---|---|
| C1 | **config 单例运行时竞态里程碑**（#1052 审计升级项 D2）：约 25 个运行时写字段（含数据面 `ProxyToken`/`ProxyRetryStatusRanges`）× 热路径无锁读，撕裂读可致 401 抖动；快照交换或守卫访问器重设计 | -race 全绿 + 并发读改写测试；热更新语义不变 |
| C2 | **#1026 凭证维度 UI 树形选择器**：后端契约已在 W20 加固（解析往返 snake/camel 修复、畸形引用显式 400、选择器/验证/悬空引用契约测试、`docs/api.md` 契约文档；API-only，PR 待合）；剩余仅 `allowedCredentialRefs` UI 树形选择器 | UI 选择器与 API 往返一致，复用 #1050 allowedSiteIds 选择器模式 |
| C3 | **race 门禁预算**：`handler/admin` -race 实测 250-360s 临界于默认 300s（本波 6 lane 撞线，均用 `METAPI_RACE_TIMEOUT_SECONDS` 官方旋钮）；上调默认或拆包 | 门禁默认预算下全绿 |
| C4 | **#1035 剩余专题**：S2 CSP（独立 sub-issue）、S4 API 契约层（后端 400 加 errorCode）、S5-S8、S9 后半（六张大表服务端分页）、S10 双语 CI | 按各专题验收 |

挑选条件：需求驱动或维护者确认；选定后按 Wave 18 模式拆 worktree 分支、定验收门槛（本地全门禁 → 12-check CI → squash merge → 发布）。**并行推送教训**：本波 8+ lane 并发 pre-push 门禁时 `handler/admin` -race 频繁超 300s 包超时（环境性），建议错峰推送或 ≤4 并发。

### 已收口（Wave 18/17，勿再当 active）

- **Wave 18 → v0.16.19**：十线并行（10 worktree + 10 subagent）——#1057 会话模型重构（#1034：服务端会话/HttpOnly cookie/WS ticket/限速前置/敏感操作重确认 + 六个浏览器门禁 harness 迁移会话登录）、#1052 路由懒加载竞态修复（+config 单例竞态升级项）、#1058 出站 HTTP 基线 + AST 门禁、#1054 管理读路径索引 `sc2_027` + N+1 修复、#1061 调度器健壮性（panic recover/in-flight 竞态/错误显性化）、#1060 PG 方言陷阱清扫（零迁移）、#1053 构建收敛 + vendor 块拆分（S1+S9 前半）、#1055 UX 残留（focus-ring/autoComplete/图表 sr-only）、#1056 健康监测全局开关（#1027）、#1059 SOCKS5 代理 + 清空即清除（#1009）。#1009/#1027/#1034 关闭；#1026 站点维度已交付、凭证维度留 open。
- **Wave 17 → v0.16.18**：竞品研究 P1×4 全部落地——#1046 SSE 空闲超时、#1049 重试/禁用状态码策略、#1047 批量测试闭环、#1048 错误横幅一键过滤；前端审计快赢 #1040-#1045、#1050 下游密钥站点限制（#1026 站点维度）。

### 已收口（Wave 16/15/14，勿再当 active）

- **Wave 16 → v0.16.17**：竞品研究 P0×3 三线并行（3 worktree + 3 subagent）全部落地——PR #1018（transform golden 快照 46 份，零生产改动）、#1020（行级探测健康条 + `/api/{channels,accounts}/probe-history` 只读端点）、#1019（结构化冷却原因三列 + 根因弹窗）+ #1021（发布节）+ #1022（加权选择测试去 flake：单次抽签断言改 200 抽统计）squash 合入 master；私有面 testbed 验收见 log；原始证据留私有面。
- **Wave 15 → v0.16.16**：PR #1013（#1009 出站代理超时五变量）/ #1014（竞品研究）/ #1015（#1005 定时模型同步）+ #1016（发布节）squash 合入 master；#1005 关闭，#1009 配置已交付、保持 open 等待报告者补充 reset 证据；私有面运维 testbed 综合验收通过（版本注入、调度器启动与热更新、设置往返与 400 校验、真实上游 e2e smoke 13 PASS / 0 FAIL、SPA 资产）；原始证据留私有面。
- **Wave 14 → v0.16.15**：PR #1010 squash 合入 `master`（`840d930`），#1007/#1008 关闭；发布前 required CI、Docker、a11y、视觉回归、SQLite/PG、E2E 与本地 pre-push 门禁均通过。
- Wave 12A/12B/13/14 均已合入并发布 v0.16.12 / v0.16.13 / v0.16.14 / v0.16.15。

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
