# Roadmap

**Last verified**: 2026-09-03

**Release**: [v0.17.1](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.17.1) · **published = repo Latest**, 2026-09-03 (tag `v0.17.1` → master `292208fd`; artifacts sampled, sha256-verified against `checksums.txt`, and re-verified end-to-end on the real-upstream testbed). Previous Latest [v0.17.0](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.17.0), published the same day

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Current active work — 波次 6 待编排（2026-09-03）

v0.17.0 收口了波次 4 的全部十条（#1159–#1170）：转发链客户端 IP 从右往左解析（`TRUSTED_PROXY_CIDRS` 下 admin IP 白名单 / 每 IP 限流 / 审计 IP 三处可伪造）· 账号写路径不再回显明文凭据（策略收口到 `service.RedactAccountSecrets`）· 流式中断/截断/空内容不再记成功、内容级失败判定与协议回落各只有一个所有者 · 三份手抄表清单（20/28/28 项 vs 37 张表）收敛为单一 schema 注册表，`cmd/migrate` 不再静默丢 17 张表、恢复出厂不再漏 9 张（含 `admin_sessions`，此前重置后旧 cookie 仍有效）· 上游压缩编码诚实性（`gzip`/`deflate` 解码后判定与计费，解不了的原样转发 + 用量记 `unknown` + 稳定文案 WARN，`Accept-Encoding` 不再是站点可配头）· 夹在两次上游调用之间的等待可取消（三个 OAuth onboard 轮询 28.3s → 0.16s）· Redis 补偿回滚按摘要协商（`EVALSHA` 优先，`NOPERM` 降级而非失败）· 对外文档成为可对账参考（env parity + 路由清单两个门禁）· smoke 站点解析按后端去重键 · PG 门禁在复用库上可重复。时间线：[`../log.md`](../log.md)。

**波次 5 已全部收口（#1172 / #1173 / #1175 / #1178，无 lane 在飞）**：备份表集从 schema 注册表派生（第三份手抄清单消灭；`type=all` 导出此前静默丢 9 张表，其中 5 张用户可见状态，而导入端回 200；4 张改为**显式**排除，理由随 `metadata.excluded_tables` 与文档一起交付并被门禁钉住）· `ApplyRuntimeSettings` 的破坏性静默丢弃清零（坏行不再清空已配置值，改为保留 + WARN 描述形状；显式 `null` / `[]` 仍按清空生效；两条机械门禁 R4/R5 让这一族回不来）· **CI 竞态门复活**（`test-sqlite-shard` 的分片算术缺 `${{ }}`，四个分片与聚合出的必选检查 `test-sqlite` 从 v0.14.0 到 v0.17.0 共 30 个 tag 全绿而执行 0 个测试；该窗口唯一真跑 Go 套件的 `test-pg` 不带 `-race`，所以竞态检测三周内从 CI 完全消失。修复后实测四片 13/12/12/12、并集 == 全量 49 包、四守卫逐一挑发全部非零退出、复活后全量 `-race` 0 失败 0 竞态）。顺带抓出仓库里**第四份**手抄表清单（`e2e` 备份用例的字面量 28）并改为向注册表取。**启动日志诚实性 + 前端布局门禁（#1178）**：`setupSPAFallback` 里 Vite 时代的 `/assets/*` 挂载声称「兼容仍然产出 `dist/assets` 的旧构建」，但前端是 `//go:embed` 编译期烘进二进制的 ⇒ 一个二进制只有一份由本提交构建的 dist，**「更旧的构建」在构造上不存在**；该挂载自 Rsbuild 迁移起永不可达，守卫却在每个部署每次启动打一条 `serving disabled` 的 WARN（在已发布的 v0.17.0 二进制上实测复现），占用的还是**真 `/static` 故障会用的同一句话**。删挂载，改由 `router/spa_layout_test.go` 钉住真契约：`index.html` 每条 root-relative 引用必须 200 **且不得为 `text/html`**（SPA fallback 对未挂载路径一律答 `200 text/html`，这正是 `730bb9c2` 把白屏藏在绿色状态码后面的方式）· 挂载本提交交付的 dist 必须 0 WARN · **零引用判失败而非通过，门禁不许空转**。**发布产物活体复验**：把已 publish 的 `metapi-linux-amd64`（sha256 对过 `checksums.txt`）换进真实上游测试床，带鉴权核对 `/api/about` = `v0.17.0` / `commit ec1043b4`（正是 tag 指向的提交），四平台上游链 + F5 双语视觉全绿、五服务全 RUNNING；该产物 `index.html` 引用的 14 条路径逐条核对，主包 200 / `text/javascript` / 141242 B ⇒ **发布产物的 SPA 健康**。

**下一波候选**（写集互斥，均不阻塞发布；来自 #1172 / #1173 / #1175 的已知遗留）：`catalog_sources` 的导入 URL 闸扩展（`importURLColumns` 加 `catalog_sources.url`，之后删掉那条备份排除即自动纳入）· PostgreSQL 导入不重置序列（`importBackupTablesWithConn` 缺 `setval` / `sqlite_sequence`）· `PayloadRules` 无 Go 运行时消费者、`OpenAiServiceTierRules` 零读者但 `docs/configuration.md` 写成 "Per-model payload rewrite rules"（**裁决项**：改文档说实情，还是落地规则引擎）· `notify_task_toggles` admin 写路径应返 400 · 约 20 个不可解析**数值**分支「静默保留 fallback」建议在解析器失败路径做一次聚合 WARN · 4 条数值键 floor 偏差 · `proxy_retry_status_ranges` / `proxy_disable_status_ranges` 透传不可解析原文 · CI 给 `bun install` 加重试（本轮 `visual-regression` 与 `docker-push` 各挂一次 registry 完整性抖动）+ 增 `actionlint`：**已实测否证它不是 #1175 那一类的守卫**（对修复前的 workflow 同样零 findings，因为它只校验 `${{ }}` 表达式，而那条 bug 是 bash 算术里的裸字面量；对照实验里它连 `if; then` 都没报，只报 `${{ matrix.nope }}` 这类未知属性）。它对现存 workflow 零 findings ⇒ 加进去无噪声成本、值得加，但定位是表达式层拼写守卫；本类故障只能由 step 内的数量断言守住。

**开放项**（均不阻塞发布，按优先级择机；每项都要有「摘掉修复必红」的门禁）：

| # | 内容 | 建议验收 |
|---|---|---|
| D1 | 5 张高频写入表无保留期（`model_probe_results` ≈2.9 万行/天）；usage 聚合无纠偏路径 | 每表一个 `*_RETENTION_DAYS`（默认 7–14）+ pruner + 启动日志报体制归属，照 `PROXY_LOG_RETENTION_DAYS` 的形状；纠偏路径要有「重算一次并对账」的入口 |
| D2 | `POST /api/accounts/verify-token` 在适配器返回空/`unknown` tokenType 时只回 400 `token verification failed`，**服务端零日志**；`channel selection failed err=<nil>` 同类（说了但没说原因） | 失败路径一条 WARN 带 site/platform/tokenType/上游状态（不含凭据本身）；e2e 断言「失败可从日志定位根因」 |
| D3 | env parity 门禁缺「过期白名单」检查（`detectDrift` 只在遍历 `.env.example` 时查 allowlist，孤儿条目永不可达） | 孤儿条目 → 门禁红，照 `docProseTokens` 的 shadow 检查形状 + 自证伪用例 |
| D4 | `sleepCtx` 在 `service/checkin` 与 `service/oauth` 各一份（12 行×2）；签到 per-account ctx 传播（`CheckinAccountContext` + 单账号触发接线）；OAuth 同链 6 个无 ctx helper（`postGeminiToken` / `fetchGcpProjects` / `fetchGeminiUserEmail` / `checkCloudAIAPIEnabled` / `postAntigravityToken` / `fetchAntigravityUserEmail`） | 单一实现落 `internal/**`；每条贯通都要有「已取消 ctx 不得产生上游往返」的用例 |
| D5 | 41 个 TS 默认时间戳列（接管库省略 `created_at` 的 INSERT 写出空格形状，新装库写 NULL）· SQLite TEXT PK 可空 + `textPK()` 死码 · 可空带默认 bool 的拷贝覆盖显式 NULL | 先定「重建表 vs 写入侧强制显式赋值」再动；每步都要有老库/接管库跑一遍不死的测试 |
| D6 | 配置面 fail-loud + 生效值回显（未知键硬错误、`GET /api/settings` 回 `{values,overrides,read_only}`）；最贵路径是 `DB_TYPE` typo 静默起空 SQLite | 未知键启动即失败并列出候选；生效值回显三段式（值 / 来源 / 是否只读） |
| D7 | 计费回执（Receipt）+ 票据后结算（503 不退账）· 前端幂等写 + 结果分类 | 参考项目已有样板（拆解报告在维护者工作区，含反模式清单：不要抄的部分同样明确） |
| D8 | 不可解码 error body 写进 `proxy_logs.error_message` 的二进制未清洗；`br` / `zstd` 不解码（需新依赖） | 不可读时用占位文案（例如 `upstream body undecodable (content-encoding: br)`）；解码支持要先定依赖策略 |
| F5 | **events 结构化批次 2**：alert 族（token expired / low balance / all proxies failed 等）迁 `WriteEvent`（批次 1 checkin 族已交付 #1128） | 同批次 1 验收：双侧键集门禁 + 双语视觉 e2e + 历史行原文 fallback |
| W1 | web 侦察 F5–F12：同端点两套 key/两种 shape、channels 两套 key、`downstreamKeysQueryKeys` 三抄且 staleTime 矛盾、英文复数三套机制、52 处内联 `defaultValue`、knip 跑默认 `include: []`、手写字面量 key、`?page=` 0/1-based 分裂 | 每项一次收敛或一条门禁，附前后对账数字 |
| W2 | 视觉 P2×3（`overview-section.tsx:292` soft-red、`column-header.tsx` 缺 `aria-sort`、`observability-error-banner.tsx:41-42` 对比度）· 术语统一批 · downstream-keys count 视图 · reset-usage UI 入口 · 视觉基线龄期治理 | 视觉基线只能从 CI `visual-regression-diffs` 产物的 actuals 重建 ⇒ 任何视觉改动都要一次 CI 往返，别在没有基线通道时开视觉 lane |
| W3 | axonhub / gpt-load 平台适配器（两个参考平台已在测试床长驻，`detect` 因无适配器失败） | 适配器 + fixture 测试 + 测试床实测链（照 sub2api 的 verify-token-import 姿势） |
| C4 | ~~#1035 结构性专题 S1–S10~~ **全部交付，issue 已关闭**（S7 收官 #1097） | — |

挑选条件：需求驱动或维护者确认；选定后按既有模式开短命分支、定验收门槛（本地全门禁 → 12-check CI → squash merge）。**并行推送教训**：8+ lane 并发 pre-push 时 `handler/admin` -race 曾撞 300s 默认预算（已升至 900s，#1063），建议错峰推送或 ≤4 并发；写集必须互斥，且以 `git merge-base` 而非 `master` 核验（master 会在 lane 脚下前进）。

### 已收口（Wave 18/17，勿再当 active）

- **Wave 18 → v0.16.19**：#1057 会话模型重构（#1034：服务端会话/HttpOnly cookie/WS ticket/限速前置/敏感操作重确认）、#1052 路由懒加载竞态修复（+config 单例竞态升级项）、#1058 出站 HTTP 基线 + AST 门禁、#1054 管理读路径索引 `sc2_027` + N+1 修复、#1061 调度器健壮性（panic recover/in-flight 竞态/错误显性化）、#1060 PG 方言陷阱清扫（零迁移）、#1053 构建收敛 + vendor 块拆分、#1055 UX 残留（focus-ring/autoComplete/图表 sr-only）、#1056 健康监测全局开关（#1027）、#1059 SOCKS5 代理 + 清空即清除（#1009）。#1009/#1027/#1034 关闭；#1026 站点维度已交付、凭证维度留 open。
- **Wave 17 → v0.16.18**：竞品研究 P1×4 全部落地——#1046 SSE 空闲超时、#1049 重试/禁用状态码策略、#1047 批量测试闭环、#1048 错误横幅一键过滤；前端审计快赢 #1040-#1045、#1050 下游密钥站点限制（#1026 站点维度）。

### 已收口（Wave 16/15/14，勿再当 active）

- **Wave 16 → v0.16.17**：竞品研究 P0×3 全部落地——PR #1018（transform golden 快照 46 份，零生产改动）、#1020（行级探测健康条 + `/api/{channels,accounts}/probe-history` 只读端点）、#1019（结构化冷却原因三列 + 根因弹窗）+ #1021（发布节）+ #1022（加权选择测试去 flake：单次抽签断言改 200 抽统计）squash 合入 master。
- **Wave 15 → v0.16.16**：PR #1013（#1009 出站代理超时五变量）/ #1014（竞品研究）/ #1015（#1005 定时模型同步）+ #1016（发布节）squash 合入 master；#1005 关闭，#1009 配置已交付、保持 open 等待报告者补充 reset 证据；验收（版本注入、调度器热更新、设置往返与 400 校验、真实上游 e2e smoke、SPA 资产）通过。
- **Wave 14 → v0.16.15**：PR #1010 squash 合入 `master`（`840d930`），#1007/#1008 关闭；发布前 required CI、Docker、a11y、视觉回归、SQLite/PG、E2E 与本地 pre-push 门禁均通过。
- Wave 12A/12B/13/14 均已合入并发布 v0.16.12 / v0.16.13 / v0.16.14 / v0.16.15。

历史完成冻结：Round 1 #887 → v0.16.3、Round 2 #889 → v0.16.4、#887 补遗 + E2E → v0.16.5、Round 3 修复波 → v0.16.6、Wave 4 综合质量波 → v0.16.7、Wave 5+6 开发 + 深审计波 → v0.16.8、Wave 7+8+9 前端体验/语义/设置/catalog/移动端审计波 → v0.16.9、Wave 10 Sites demand batch → v0.16.10 、Wave 11 UX 真值波 → v0.16.11 均已发布。

### Wave 11 已收口说明

- 四 lane 全部落地（batch 分支 `batch/w11-ux-truth`）：A 路由页视图持久 + 横幅语义测试补齐（#862 行为核验已在基线），B accounts 行级 pending + 对比行 re-run，C OAuth start 流闭环（有界轮询 + 手动回调），D vitest ignore flag 移除 + a11y 41 路由单一来源 + golden 10 页。

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
- **CI is the race gate of record on hosts that push with `--no-verify`** (small nodes cannot finish the pre-push suite inside their budget). `test-sqlite-shard` therefore asserts its own shard selection instead of trusting it — matrix values bound through `env`, the round-robin slot computed in a standalone `$(( ))` so a malformed selector aborts under `set -e` rather than hiding in an `if` condition, and the per-shard package count checked against `ceil((N - S) / T)`, with an empty shard treated as an error. See [`../../testing.md`](../../testing.md). Do not "simplify" those guards: a required check that runs nothing still reports green, and that is precisely what happened for 30 tags.

## Deferred or out of scope

- Demand-gated: encrypted WebDAV sync, mobile PWA, Realtime transport.
- Explicitly out of scope: multi-tenant billing/wallet/subscription/payment and shared multi-instance sticky sessions.
- Update-center deploy/rollback remains external through GHCR and GitHub Releases; the admin API stays an honest 501 residual.
- Historical UI audit observations remain evidence in [`../analysis/ui-ux-audit-2026-08.md`](../analysis/ui-ux-audit-2026-08.md). They become commitments only when promoted here with an owner and acceptance criteria.
