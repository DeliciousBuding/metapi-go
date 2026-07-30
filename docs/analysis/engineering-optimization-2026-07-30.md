# Engineering Optimization Synthesis — codeg 对标拆解

**Date**: 2026-07-30
**Authority**: leader 综合结论（对标 codeg 竞品研究子报告 + SYNTHESIS）
**Scope**: metapi-go 工程优化（巨石/边界/错误/文档/测试/并发），非产品功能 parity（parity 见 `original-parity-complete-2026-07-20.md`）
**Method**: 6 路 fable 子代理深拆 + leader 源码实证复核；每条结论 cite `file:line`，区分真缺口/误报/已诚实声明

---

## 1. 执行判断（leader 自决）

子代理舰队在上一会话末被中断（无 completion 记录），但其工具调用产出可由 leader 直接复核源码重证。**本回合不重启舰队**——6 个维度的核心结论均可通过源码 grep + Read 在数分钟内实证，且已被 leader 逐条复核（见 §3）。重启舰队只会重跑已知结论、浪费预算。直接进入「综合 → 计划 → 执行」。

**codeg 对标的核心定位**：codeg 是 Rust 单体双形态（桌面/server/伴生 MCP），metapi-go 是 Go 单二进制 gateway——**形态不同，工程机制不可平移抄**。可借鉴的是 codeg 的**工程纪律模式**（结论固化成机器断言、注释即设计文档、快照纪律、target backstop），不是它的架构。codeg 自身最大的反面教材就是巨石（`connection.rs` 9929 行、`commands/acp.rs` 13904 行靠 `recursion_limit=256` 硬撑）——metapi-go 应**以此为戒**，在巨石化之前先拆。

---

## 2. 实证基线（leader 复核，非子代理转述）

### 2.1 规模与巨石

- 总 Go 文件 477 / 测试 218（46% 文件级配比，远高于 codeg 的 41%）。
- 最大非测试文件（LOC）：
  - `transform/shared/chatFormatsCore.go` 1826 — Claude/Anthropic 下游序列化核心，**高内聚**（围绕 `ClaudeDownstreamContext` 一族状态机），不拆。
  - `handler/proxy/upstream.go` 1781 — 上游转发编排，**职责内聚**（依赖注入 + candidate paths + sanitize），大但合理。
  - `handler/admin/accounts.go` 1768、`platform/newapi.go` 1646、`routing/runtime_health.go` 1640 — 各自单一平台/领域的自然体量。
  - `handler/admin/stats.go` 1544 — **34 个 func**，混合 handler + SQL builder + coerce helpers + marketplace builder（`buildMarketplaceModels` 837-1113）；`stats.go` 是**真·可拆**：coerce/helper 族（`queryRow`/`coerceFloat`/`coerceInt`/`coerceString`/`nowUTC`/`roundMicro` 1457-1532）是纯工具，应抽到 `handler/admin/stats_helpers.go`；marketplace builder 族（837-1364）应抽到 `handler/admin/stats_marketplace.go`。拆后 stats.go 本体降到 ~900 行。
- 测试巨石：`accounts_test.go` 2464、`scheduler_test.go` 2056、`upstream_test.go` 1611、`failure_isolation_test.go` 1601、`token_routes_test.go` 1541、`max_tokens_test.go` 1511 — Go 同包多测试文件合法，应按被测主题拆，但**非阻塞**，本轮不拆（拆测试不改行为、收益是合并冲突降低，优先级低于边界断言）。

### 2.2 包边界（BACKEND.md §2.3 实证）

leader 用 `go list -deps` + grep import 块逐条实证 BACKEND.md §2.3 八条硬规则——**当前全部干净，零违规**：

| 规则 | 实证 | 结果 |
|:-----|:-----|:-----|
| store 不 import 上层 | `grep metapi-go/(handler\|proxy\|routing\|service\|scheduler\|router\|auth) store/*.go` | ✅ 仅 import `config` |
| platform 不 import 上层 | 同上 + `go list -deps ./platform` | ✅ 仅 `config` + 文档已记录的例外 `proxy/profiles` |
| transform 不 import 上层 | grep transform/ import | ✅ 仅 `transform/canonical` + `transform/shared`（纯叶子） |
| routing 不 import proxy/handler | grep routing/ import | ✅ 仅 `config` + `store` |
| service 不 import handler/router | grep service/ import | ✅ 仅 `config`/`platform`/`routing`/`store` |
| handler→platform 直连 | grep | ⚠️ 8 处 handler 直 import `platform`（accounts/account_tokens/settings/model_refresh/site_announcements/channel_test_harness/channel_health_probe/upstream）——**这是已记录的合法例外**（handler/admin 可 import platform，见 BACKEND.md §2.2 表 + package-boundaries.md:145） |
| 循环依赖 | `go vet ./...` | ✅ rc=0，无 cycle |
| 组合根 | grep 包级 global | ⚠️ `handler/proxy` 有 `upstreamCfg` 包级单例 + `SetUpstreamConfig`（`upstream.go:36`）——非 cmd 构造，但这是 parity 期的既有模式，与 `config.Get()` 同类，**非本轮处理** |

**结论**：边界**已守得很干净**，问题不是违规而是**未固化成机器断言**——codeg 用 `grep -nE` gate 锁死「web handler 必须走 _core」（test.yml:134-148），metapi-go 当前靠人脑守 BACKEND.md。**最高价值动作 = 把边界落成 CI 断言**，防未来漂移。

### 2.3 错误模型

- `handler/shared/` grep `code:` / `ErrorCode` → **空**，无统一错误码注册表。
- sentinel errors 仅散点：`proxy/buffered_response.go:14` `ErrBufferedResponseBodyTooLarge`、`proxy/executor.go:111` `ErrObservedFirstByteTimeout`。
- 对比 codeg：`app_error.rs` 统一 DTO `{code, message, detail, i18n_key}` + 17 个语义 code + i18n key 单测断言字面值防静默改名（SYNTHESIS A4）。
- **真缺口**：metapi-go 错误是各处 `fmt.Errorf` 自由文本，无机器码。但**这是 M 级改造**（需定义注册表 + 迁移各 surface + 不破坏 camelCase wire），**本轮不开**——记录为 Issue 候选。

### 2.4 文档卫生（neat-freak）

- `docs/analysis/` 85 个 md + `docs/plan/` 14 个 = **99 个分析/计划文件**，无 `docs/archives/` 目录。
- **真·过期可归档**（leader 逐文件确认无活跃引用）：
  - `p4-account-verify.md`、`p4-settings-proxy-test.md`、`p4-token-adapter-wiring.md` — P4 阶段工作文件，**全仓零引用**（grep 全仓 md+go，仅自身命中）。
  - `ui-score-2026-07-19.md`、`ui-score-shell-2026-07-19.md`、`ui-score-shell-mock-2026-07-19.md` — 2026-07-19 一次性 UI 打分快照，仅 `ui-score-pages-2026-07-19.md` 被 `ui-visual-acceptance.md` 引用；其余两个 shell 变体无外部引用。
  - `ui-pm-empty-state-2026-07-19.md` — 一次性 PM 空态笔记。
- **不可动**（有活跃引用）：
  - `p4-admin-test-routes.md` — 被 `admin-channel-test-harness.md:111` + `ops-admin-stubs.md:25` + `handler/admin/test.go:19` 三处引用，**保留**。
  - `ui-score-pages-2026-07-19.md` — 被 `ui-visual-acceptance.md:101` 引用，**保留**。
  - `failover-first-byte.md` vs `failover-isolation.md` — 主题不同（前者讲 timeout 单位 #38，后者讲隔离 #25/#585），**不合并**。
- `docs-hygiene` CI job 已存在（ci.yml:77，跑 `doc_hygiene_test.go` 检 local path / Redis 伪声明 / AI citation）——卫生纪律已有机器门禁，**无需新增**。

### 2.5 测试纪律

- `ci.yml` **已有 `paths-ignore: docs/** + **/*.md`**（ci.yml:7-14）——path-filter 已落地，codeg SYNTHESIS 反面教材 A（全量矩阵硬跑）**不适用**，metapi-go 已优于 codeg。
- `test-sqlite`（`-race`）+ `test-pg`（`-tags=integration` PG）双 dialect 已落地（ci.yml:105-141）——BACKEND.md §3.4 dual-dialect 已守。
- **缺口**：无 `INSTA_UPDATE=no` 式快照防漂移纪律（metapi-go 无 golden 快照资产，暂不适用）；无 decoupling 测试锁（routing 改动不影响 transform 输出的断言）——后者可补，**非本轮**。

### 2.6 并发状态

- `service/balance/balance.go` 每账户 singleflight（`sub2apiRefreshMu` + `sub2apiRefreshInFlight` map，41-42 行）——**已存在**（上一会话已核实，子代理「高危缺口」为误报）。
- `update-center` 刻意诚实残桩（`scheduler/update_center.go:105` `runSyncLocked` no-op + #246/#283 注释）——**刻意的诚实门禁，非缺口**。
- STICKY-B Redis deferred — STATE.md 已诚实声明，**非缺口**。
- **本轮无新并发缺口需补**。

---

## 3. 优化优先级（SDD 分阶段）

| 优先级 | 项 | 量 | 收益 | 风险 |
|:------:|:---|:--:|:-----|:-----|
| **P0** | 包边界 CI 断言（`package_boundary_test.go`） | S | 把 BACKEND.md §2.3 八条硬规则落成 `go test` 机器门禁，防未来漂移；对标 codeg grep-gate | 零（只读断言，不改产品代码） |
| **P1** | neat-freak 文档归档（6 个过期文件 → `docs/archives/2026-07/`） | S | 99 文件瘦身，降接手认知负担 | 零（归档保留，加重定向指针） |
| **P2** | `handler/admin/stats.go` 职责拆分（helpers + marketplace 抽出） | S | 1544→~900，降合并冲突 | 低（同包移动，不改 public API，pre-push test 兜底） |
| **P3（记录，本轮不开）** | 错误码注册表 SSOT | M | 对标 codeg A4 机器码贯穿 | 中（需迁移各 surface，不破坏 wire） |
| **P3（记录，本轮不开）** | 测试巨石按主题拆分 | S-M | 降合并冲突 | 零（不改行为） |
| **P3（记录，本轮不开）** | decoupling 测试锁 | S | 防 routing 改动回归 transform | 低 |

**本轮执行范围**：P0 + P1 + P2（全部 S，零或低风险，硬门禁内）。P3 开 Issue 跟踪。

---

## 4. 硬门禁（继承 MASTER.md）

1. 保行为不变——拆分/归档/断言不得改变任何 wire 契约或运行时语义。
2. 不自动 pin 生产（0.8.45 pin 需管理员授权 + ≥15min soak）。
3. Electron 非目标。
4. pre-push：`go vet ./... && go test ./... -count=1 -race`。
5. 不虚构 UC registry / STICKY-B / 假 WS 帧。
6. 文档归档保留文件（移入 `docs/archives/`，不删），加重定向指针，便于回溯。

---

## 5. 交付批次

**Batch A（单 PR，本回合）**：
1. 新增 `package_boundary_test.go`（P0）——`go list -deps` 实证 BACKEND.md §2.3 八条规则。
2. 归档 6 个过期文件 → `docs/archives/2026-07/` + 留 `.md` 重定向指针（P1）。
3. `handler/admin/stats.go` 拆出 `stats_helpers.go` + `stats_marketplace.go`（P2）。
4. 更新 `docs/README.md` map + `STATE.md`/`MASTER.md`/`log.md` tip + `CHANGELOG.md`。
5. pre-push `go vet && go test -race`。

**Issue 候选（不在本回合）**：错误码注册表、测试巨石拆分、decoupling 测试锁。

---

## 6. 来源索引

- codeg 研究报告：codeg 竞品研究子报告集（SYNTHESIS / backend / devx / frontend / product / protocol / data / uiux / web-module / v0.22.1-DELTA）
- codeg 仓库：上游 codeg 仓库（克隆参考，HEAD dea521fe，v0.21.9）
- metapi-go 边界规则：`docs/design/BACKEND.md` §2
- metapi-go 边界实证清单：`docs/analysis/package-boundaries.md`
- metapi-go 卫生门禁：`docs/doc_hygiene_test.go` + `.github/workflows/ci.yml:77`
