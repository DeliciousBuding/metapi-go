# Task Breakdown — 前端架构化（路由 + 设计系统）

**Last updated**: 2026-08-16

> Spec-driven 运行产物。范围：架构层（路由 + 设计系统架构债务），不含功能 feature P1。
> 跟踪模式：GITHUB_STANDARD（Issue + Milestone + Label + worktree + 分批 PR）。
> 上游分析：`docs/analysis/ui-ux-audit-2026-08.md` + 两份运行时审计（路由 / 设计）。
>
> **执行状态（2026-08-16 对账）**：全部落地。T1（4 页 URL 状态）在 PR #782；
> T2/T3 已合入 master；Phase 2 T4–T8 已合入 master；T9（文档销项）= 本次对账。
> 详见 [`milestones.md`](milestones.md)。

## Overview
- **Total Phases**: 2
- **Total Tasks**: 9
- **Planned Delivery Batches / PRs**: 4
- **Estimated Total Effort**: L

## S.U.P.E.R Design Constraints

- **S (Single Purpose)**: 每个改动解决一个问题；URL 状态归属、动效、方向、图标各自独立。
- **U (Unidirectional Flow)**: URL → search state → URL 单向；loader 只读 `location.searchStr`，不反向读 `window.location`。
- **P (Ports over Implementation)**: `validateSearch` schema 是路由层的契约端口，页面只消费类型化 search。
- **E (Environment-Agnostic)**: 不硬编码主题/方向；FOUC bootstrap 与 TS 常量单一来源。
- **R (Replaceable Parts)**: 徽章/图标/动效统一后，替换单一实现不产生级联改动。

## Testing and Governance Constraints

- 路由/URL/解析/行为变更必须带测试；文档/配置任务标注 N/A 并给最接近的校验命令。
- 稳定约定/坑 → 更新 `docs/`（架构/SOP）或 `AGENTS.md`（规则红线）；本仓库无独立 agent memory，沿用 `docs/progress/MASTER.md` + `docs/analysis/`。

---

## Phase 1: 路由架构硬化
**Goal**: URL 状态归属统一到路由层，关闭 404 无壳与校验缺口。
**Prerequisite**: 无（#685 已落地 pending/splitting/scroll/active 基础）。
**S.U.P.E.R Focus**: U（单向 URL 流）、P（schema 契约端口）。

| # | Task | Pri | Effort | Depends | Lane | Batch | S.U.P.E.R | Test Expectation | Memory Impact | Acceptance |
|:--|:-----|:----|:-------|:--------|:-----|:------|:----------|:-----------------|:--------------|:-----------|
| 1 | URL 状态统一：accounts/checkin/token-routes/proxy-logs 从 `history.replaceState`+手动读迁移到 `navigate({search,replace:true})`+`useSearch`；proxy-logs/checkin loader 改用 `location.searchStr` | P0 | XL | — | A-D | P1-B1 | U, P | 补/改 URL 状态 + loader search 解析测试 | 记录「页面不直接碰 history」约定到 analysis | 4 页过滤/分页 URL 往返无损；深链可恢复；意图预加载用目标参数 |
| 2 | `_authenticated` 404 catch-all（404 落入侧栏壳内，容器相对高度） | P1 | S | — | E | P1-B2 | R | N/A（纯布局，跑 build+typecheck） | 无 | 未知路径渲染壳内 404 |
| 3 | `checkIsActive` 死代码/hash 清理 + `?model=` deep-link 校验 | P1 | S | — | E | P1-B2 | S | 改 hash 匹配补单测；model 校验补 schema 测试 | 无 | hash 不丢 active 态；非法 model 回退 |

### Parallel Lanes
| Lane | Tasks | Combined Effort | Merge Risk | Key Files |
|:-----|:------|:----------------|:-----------|:----------|
| A | 1(accounts) | M | Low | `accounts-page.tsx` + `accounts.tsx` route |
| B | 1(checkin) | M | Low | `checkin-page.tsx` + `checkin.tsx` route |
| C | 1(token-routes) | M | Low | `routes-page.tsx` + `token-routes.tsx` |
| D | 1(proxy-logs) | M | Low | `proxy-logs-page.tsx` + `proxy-logs.tsx` |
| E | 2, 3 | S | Low | `routes/_authenticated/$`、`url-utils.ts`、`model-tester.tsx` |

> T1 拆 4 条 lane（4 页互不相干，文件集合不相交）；T2/T3 一条 lane。lane 间无依赖。

### Delivery Batches
| Batch | Tasks / Issues | Waves | Rationale | Integration Branch | Combined Validation | Depends | Split Rationale |
|:------|:---------------|:------|:----------|:-------------------|:--------------------|:--------|:-----------------|
| P1-B1 | 1 | W1: Lane A-D | 高风险 URL 迁移隔离成单一可审 PR | `batch/p1-b1-url-state` | 4 页 URL 往返测试 + typecheck/lint/test/build | — | 高风险隔离 |
| P1-B2 | 2, 3 | W1: Lane E | 低风险路由收口 | `batch/p1-b2-routing-polish` | typecheck/lint/test/build | — | 默认批 |

---

## Phase 2: 设计系统架构化
**Goal**: 动效/方向/主题单一来源，图标与交互原语收敛，文档销项。
**Prerequisite**: Phase 1（不含强依赖，仅顺序隔离）。
**S.U.P.E.R Focus**: S（单一职责）、R（可替换）、E（环境无关）。

| # | Task | Pri | Effort | Depends | Lane | Batch | S.U.P.E.R | Test Expectation | Memory Impact | Acceptance |
|:--|:-----|:----|:-------|:--------|:-----|:------|:----------|:-----------------|:--------------|:-----------|
| 4 | 动效统一：tw-animate-css vs Base UI starting-style 二选一 + 全量 `motion-reduce` 覆盖 | P1 | M | — | F | P2-B1 | S, R | N/A（视觉，跑 build + 手测） | 记录单一动效体系约定 | 弹层/抽屉同动效规格；reduced-motion 下无入场动画 |
| 5 | RTL：`DirectionProvider` 单一归属，i18n `syncDocumentLanguage` 不再钉死 `ltr` | P1 | M | — | F | P2-B1 | E, S | dir 同步补单测 | 记录 dir 单一来源约定 | 语言切换不覆盖 dir cookie |
| 6 | FOUC bootstrap 与 `THEME_PRESETS`/`resolveThemeFont` 对齐（测试断言一致） | P1 | M | — | G | P2-B1 | S, E | 加「bootstrap 列表 == 常量」测试 | 记录「bootstrap 不能手写」约定 | 新预设不再漂移 |
| 7 | 图标族统一（HugeIcons × lucide 同屏收敛） | P1 | M | — | G | P2-B2 | R | N/A（纯替换，build+typecheck） | 记录图标选型约定 | 单一图标源 |
| 8 | 交互原语收敛：`transition-all` 收敛、select 移动端 portal、table 行 token 化 | P2 | M | — | G | P2-B2 | S, R | 改 token 补样式存在性校验 | 记录 `--table-*` token 约定 | 无 `transition-all` 散落；移动端 select 不被裁切 |
| 9 | 文档漂移 + 死代码：DESIGN.md 预设数、a11y 对比度债务、标题 400vs500、死 motion 变体/chart fallback 重复 | P2 | M | — | H | P2-B2 | S | N/A（文档，跑 doc 链接/尺寸自检） | 更新 analysis 销项 | 文档与实现一致 |

### Parallel Lanes
| Lane | Tasks | Combined Effort | Merge Risk | Key Files |
|:-----|:------|:----------------|:-----------|:----------|
| F | 4, 5 | M | Medium | `ui/dialog|sheet|select|popover|dropdown|tooltip`、`i18n/config.ts`、`direction-provider.tsx` |
| G | 6, 7, 8 | M | Low | `index.html`、`theme-customization.ts`、图标、`ui/*` |
| H | 9 | M | Low | `DESIGN.md`、`a11y`、`lib/motion.ts`、`use-chart-colors.ts` |

> F 与 G/H 文件集合不相交（F 碰弹层/i18n/方向，G 碰 bootstrap/图标/原语，H 碰文档）。G 内 6/7/8 顺序执行。

### Delivery Batches
| Batch | Tasks / Issues | Rationale | Integration Branch | Combined Validation |
|:------|:---------------|:----------|:-------------------|:--------------------|
| P2-B1 | 4, 5, 6 | 动效/方向/主题单一来源（一处评审） | `batch/p2-b1-motion-theme` | typecheck/lint/test/build |
| P2-B2 | 7, 8, 9 | 图标/原语/文档 polish | `batch/p2-b2-polish` | typecheck/lint/test/build |

> 拆 P2-B1/P2-B2：动效+方向+主题属「行为/架构」评审面，图标+原语+文档属「收敛/polish」评审面，分开更易 review 与回滚。
