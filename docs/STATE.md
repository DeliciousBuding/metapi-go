# STATE.md — MetAPI Go product status

**Last verified**: 2026-08-01

> **现状 SSOT**（产品仓库）。只记当前事实与指针，不写流水账。  
> 运维主机/compose/镜像 pin / PG role LIMIT 以 **server 仓** `projects/metapi/STATE.md` 为准（可能与本 tip 不同步）。  
> 进度开放项 → [`progress/MASTER.md`](progress/MASTER.md)  
> 时间线 → [`log.md`](log.md)  
> 高价值下一步 → [`analysis/high-value-next.md`](analysis/high-value-next.md)  
> 版本叙事 → 根 [`CHANGELOG.md`](../CHANGELOG.md)

## Current

| Fact | Value |
|:-----|:------|
| Latest release tag | **[v0.8.45](https://github.com/TokenDanceLab/metapi-go/releases/tag/v0.8.45)** (2026-07-20) — RE2-safe + UI tip |
| Tip | `de95c20` log 条目, `f43f004` — 门禁输出质量审计收官（**三批 534 键**：通知渠道/OAuth 管理/调试追踪/审计/重置/模型映射/倍率总览/路由高级参数/站点/账号令牌；suspicious 586→72 全为多行 JSX 折叠误报）；prior `193dd5e` — EN 验收 --with-data 11/11（数据面 + 对话框 18 键清零，CI en-verify 切 with-data）; prior `f216c73` — EN 验收进 CI（en-verify job 12 job 全绿）+ zh 回归 e2e; prior `87f9711` ci job + zh 回归, `8c70209` — EN 主界面实拍验收 11/11 全绿（单字键顺序 bug + scheduler Job id + Settings code 容器 + 8 键）；验收脚本 `web/scripts/verify-en-pages.mjs`; prior `52100b3` log 条目, `3769ac8` e2e wave（212 条对象字面量中文补键 + 门禁第四面 + EN e2e 三测全绿）; prior `3037376` log 条目, `42b895f` — TestFallbackCostPenalty 改 200 次统计断言（消除 CI 偶发 flaky，CD release-gate 复绿）; prior `3c100db` SSOT 同步, `bf8c613` embed SPA 重建 + log review wave 条目, `eab0db7` i18n review wave（156 处 EN Untranslated 清零 + tr() 入门禁 + 单字键汉字边界 + zh 回切恢复 + data-i18n-skip 用户数据豁免）; prior `15801e9` SHOT-1 视觉验收记录（程序化像素复核：NAV-1 折叠侧栏 4 项 + 空态内容 + 重录时间戳）; prior `1ddf697` B4/B5 拍板决策包（推荐关闭/保持现状）, `cc4578d` B3 拍板决策包（推荐关闭）, `3324edd` SHOT-1 空库截图重录（page/shell/gallery/login 全套 light/dark）; prior `b508b84` CI 全绿收官（Node25 localStorage 根治 + PG boolean 字面量 + linux 视觉基线）; prior `a81464f` resolveStorage 防御, `572e29a` PG route_channels TRUE + 基线流程, `dab9270` 三路 CI 根因修复, canvas 快照 EN 化（tr() + 11 条补译）; prior `ba935d8` i18n 第三面（插值碎片 4 条 + 三层门禁）, `a44cad1` JSX 硬编码中文全量补译 181 条, `1448169` i18n EN 完整性门禁 + 10 条漏译 + focus-trap jsdom 防御, `4b89fb5` DENSE-1 表格密度（默认 8px + compact 开关）; prior `d7222f8` review 11 项核实缺陷修复, `12cfdf7` NAV-1 first-run 侧栏, `84b3e60` VIS-1 主题 preset, `595a383` B2 实时运维 WS, `5eda0c2` B1 审计日志, `88828e7` A3 余额流入 vs 消费, `db7029b` K1b 路由 canonical 化, `caef603` N9b-a 倍率批量编辑, `31d162e`+`de3c926` docs SSOT sync, `c2d7cb8` N9a 倍率总览 + N8 关闭评估（架构等价）; prior `7597a07` K1a 模型重定向映射, `d9f915a` J1 快照 PNG, `5da5656` H1 风险横幅, `ba74242` I1 标签系统, `0047c72` Wave C（A2 图表画廊 / G1 批量验证）, `37d6a70` Wave B（E1 随机窗口调度 / F1 备份导入预览+契约修复 / C1 调度统一运行历史）, `6e0312b` Wave A（A1 余额历史/B1 需关注看板/D1 per-task 通知+4 渠道）, `10384ee` N7, `9c056a4` N2-N6+G1, `d4633f1` N1 IP 白名单; **deferred 清单已清空（2026-08-01）** |
| Production pin (ops) | server `projects/metapi/STATE.md` — hk3 still **0.8.44 Exited(2)** until authorized pin/up of **0.8.45** + 15min soak; pool/role **1/1**; restart=no |
| Standby us1 pin | compose **0.8.42** + image pulled (#528); cold stack not auto-started |
| Active milestone | **[53 REL-HONESTY](https://github.com/TokenDanceLab/metapi-go/milestone/53)** — #557 prod e2e + #558 runtime probes open; M52 UI-POLISH closed (0 open) |
| Open issues / PRs | M53 [#557](https://github.com/TokenDanceLab/metapi-go/issues/557) P0-585 prod e2e · [#558](https://github.com/TokenDanceLab/metapi-go/issues/558) runtime probes |
| Mode | **parity program (docs)** — v0.8.45 released; ops pin/up gated; original-complete plan SSOT [`plan/original-parity-complete-2026-07-20.md`](plan/original-parity-complete-2026-07-20.md) |
| Stack | Go 1.26.5 · React 19 · Vite 8 · dual dialect SQLite/PG |

## Honesty holds (not product yet)

| ID | Status | Note |
|:---|:-------|:-----|
| P0-585 cascade | **partial** | load-proof still required; honesty tests do not flip present |
| P0-555 usage stats | **present-with-residual** | media detail fold shipped; multi-instance lag residual; not perfect billing |
| WS-1 Responses WebSocket | **C1+C2+C3 present** | HTTP multi-turn bridge + per-msg quota + Codex upstream wss (flagged); single-instance honesty |
| STICKY-B Redis sticky | design-only **deferred** | multi-turn/WS requires single instance or LB pin |
| UC-1 update-center deploy | **hide/external present** | UI ops note + GHCR/Releases; API residual 501; no invent updateAvailable |
| OPS-PG-BUDGET | **present product** (v0.8.44 code) | profiles + lease backoff; ops still size role LIMIT |
| OPS-RE2-USERID | **fixed in v0.8.45** | was production Exited(2) on 0.8.44; ops still must pin/up 0.8.45 + soak |
| OPS-OAUTH-REFRESH | **present** (#251 / post-v0.8.45) | OAuth token auto-refresh scheduler (oauth-refresh); TS parity for codex/claude/gemini-cli/antigravity lead times |
| OPS-SUB2API-REFRESH | **present** (post-#246) | Sub2API managed session token refresh via balance.RefreshBalance (extraConfig parsed, due filter with 300s lead window, concurrency=4); residual: no standalone lightweight refresh endpoint |
| UI-REFRESH / UI-POLISH | **released v0.8.45** + **cloud-ops align** | visual family → tokendance-design `styles/cloud-ops`; see [`design/cloud-ops-alignment.md`](design/cloud-ops-alignment.md); residual optional empty-DB shot |
| UI vs 原版功能 | **parity on web surface** | 2026-07-20 inventory: routes/buttons 齐平；体感「没了」= 空库 + pin 落后 tip + 主题换肤 — [`analysis/ui-original-parity-2026-07-20.md`](analysis/ui-original-parity-2026-07-20.md) |

## Next-wave pointer

Prioritized **ours vs original** shortlist: [`analysis/high-value-next.md`](analysis/high-value-next.md)  
**Parity program (ex-Electron)**: [`plan/original-parity-complete-2026-07-20.md`](plan/original-parity-complete-2026-07-20.md)  
UI wave SSOT: [`analysis/ui-ux-refresh.md`](analysis/ui-ux-refresh.md) · **cloud-ops 对齐** [`design/cloud-ops-alignment.md`](design/cloud-ops-alignment.md) · visual harness [`analysis/ui-visual-acceptance.md`](analysis/ui-visual-acceptance.md) · 原版功能对照 [`analysis/ui-original-parity-2026-07-20.md`](analysis/ui-original-parity-2026-07-20.md)  
Full residual inventory: [`analysis/residual-next-candidates.md`](analysis/residual-next-candidates.md)  
Original parity evidence: [`analysis/original-gap-matrix.md`](analysis/original-gap-matrix.md)  
WS residual: [`analysis/responses-websocket-residual.md`](analysis/responses-websocket-residual.md)
Engineering optimization (codeg 对标): [`analysis/engineering-optimization-2026-07-30.md`](analysis/engineering-optimization-2026-07-30.md)
Product parity & New API 借鉴: [`analysis/product-parity-and-newapi-borrow-2026-07-30.md`](analysis/product-parity-and-newapi-borrow-2026-07-30.md)
UIUX/产品化借鉴（New API 前端对标）: [`analysis/uiux-newapi-borrow-2026-07-30.md`](analysis/uiux-newapi-borrow-2026-07-30.md)
all-api-hub 全面产品面借鉴（A1-J1，13 项决策输入）: [`analysis/competitive/all-api-hub-product-borrow-2026-07-31.md`](analysis/competitive/all-api-hub-product-borrow-2026-07-31.md)

## Entry points

| Need | Path |
|:-----|:-----|
| Doc map | [`README.md`](README.md) |
| Open gates / next | [`progress/MASTER.md`](progress/MASTER.md) |
| High-value next | [`analysis/high-value-next.md`](analysis/high-value-next.md) |
| Formal readiness | [`analysis/formal-readiness.md`](analysis/formal-readiness.md) — Track A 对内正式 / Track B 对外完备 |
| UI/UX refresh | [`analysis/ui-ux-refresh.md`](analysis/ui-ux-refresh.md) |
| Residual inventory | [`analysis/residual-next-candidates.md`](analysis/residual-next-candidates.md) |
| Agent rules | root [`AGENTS.md`](../AGENTS.md) |
| Deploy vars | [`deployment.md`](deployment.md) |
| Ops live host | server `projects/metapi/STATE.md` (not this file) |

## Branch hygiene (repo)

| Fact | Value |
|:-----|:------|
| Default branch | `master` |
| Worktrees | clean master; M52 feature worktrees pruned after merge |
| Stale remote feature heads | deleted after merge-PR |
| Unmerged historical branch | `origin/codex/metapi-regex-crash` (RE2 fix source; reapplied on master tip) |
