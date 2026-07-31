# K1 模型重定向映射（all-api-hub borrow）设计文档

**日期**: 2026-08-01 · **状态**: 设计 + K1a 实现 · **量级**: M（拆分为 K1a S-M + K1b M）

> 决策输入: `docs/analysis/competitive/all-api-hub-product-borrow-2026-07-31.md` 第 154 行。
> all-api-hub 证据: `modelRedirect` 资源映射 writer + 同步后 auto-apply。

## 1. 问题

上游模型列表同步（`refreshAccountModels` → `model_availability`）后，route 重建
（`RebuildTokenRoutesFromAvailability`）按 **实际模型名** 与 token_routes 的
`model_pattern` 做 `routing.MatchesModelPattern` 严格匹配。当上游返回带日期/区域
后缀的模型名（如 `claude-3-5-sonnet-20241022`）而路由 pattern 是标准名
（`claude-3-5-sonnet`）时：

- 通道不会挂到该路由（名字不匹配）→ 流量少一条可用路径；
- 该模型可能曾因「未匹配任何路由」被判定不可用而写入 `site_disabled_models`，
  即使上游已恢复也不会被修复。

## 2. 目标

「标准名 → 上游实际名」映射：

1. **生成**：模型同步成功后，对每个实际模型名生成映射候选（自动，可预览）；
2. **修复**：映射存在时，移除 `site_disabled_models` 中因名字不匹配产生的禁用
   （**仅限非手动、同步可归因的条目**，dry-run 预览 + 可回滚）；
3. **展示/手编**：Settings 或 Models 页查看映射、手动增删；
4. **K1b（可选后续）**：路由匹配 canonical 化（实际名按映射折算后再匹配
   pattern）——触及核心热路径，另行拍板。

## 3. 匹配规则（生成映射）

对同步得到的实际名 `actual`，逐个与「标准名候选集」比较：

- 标准名候选集 = 全局 `allowed_models`（loadGlobalAllowedModels）+ token_routes
  的 `model_pattern`（仅 exact 模式，如 `claude-3-5-sonnet`，非 `claude-*`）；
- 规则（按优先级）：
  1. 精确相等（大小写归一后）→ 无映射需要；
  2. **日期后缀**：`canonical` 是 `actual` 的前缀，且剩余部分匹配
     `-YYYYMMDD` / `-YYYYMMDD-vN`（如 `claude-3-5-sonnet-20241022`）→ 映射；
  3. **版本后缀**：`canonical` 是 `actual` 的前缀，且剩余部分以 `-` 开头
     （如 `-latest`、`-beta`）→ 映射；
  4. 大小写归一后按 2/3 再试（如 `GPT-4O` vs `gpt-4o` 精确相等已覆盖）。
- 一个 `canonical` 只保留一个映射（首个命中，稳定性优先：精确 > 日期 > 版本）；
- 显式保留：不自动删除手动建立的映射；上游移除某实际名时映射保留（记录
  `last_seen_at`，展示为「未在最近同步中出现」）。

## 4. Schema（AdditiveStep sc2_012）

```sql
model_name_redirects (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,   -- PG: SERIAL
  account_id   INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  canonical    TEXT NOT NULL,          -- 标准名（路由 pattern 侧）
  actual       TEXT NOT NULL,          -- 上游实际名
  source       TEXT NOT NULL DEFAULT 'sync',  -- sync | manual
  last_seen_at TEXT,                   -- 最近一次同步仍见该 actual
  created_at   TEXT NOT NULL,
  updated_at   TEXT NOT NULL,
  CONSTRAINT model_name_redirects_account_canonical_unique UNIQUE (account_id, canonical)
)
```

- **per-account**（同账号不同站点返回不同名；账号粒度可回滚、可归因）；
- 索引：`(account_id, actual)` 供反向查询。

## 5. 端点（K1a）

| 方法 | 路径 | 说明 |
|:--|:--|:--|
| GET | `/api/model-redirects?accountId=&source=` | 列表（含 last_seen 状态） |
| PUT | `/api/model-redirects/{id}` | 改 source=manual / 修正 actual |
| DELETE | `/api/model-redirects/{id}` | 删除映射 |
| POST | `/api/model-redirects/generate` | 手动触发生成（dry-run 预览） |
| POST | `/api/model-redirects/apply` | 应用 disabled_models 修复（dryRun 参数） |

**disabled_models 修复语义（K1a 的 auto-apply）**：
- 候选：`site_disabled_models` 中 `model_name` 是某个映射的 `canonical` 的条目；
- 前置条件：该 account 的 `model_availability` 中 `actual` 可用（available=1）；
- `apply`（非 dry-run）删除这些条目并记录 events（type=model_redirect_applied）；
- 仅删同步产生的禁用；`is_manual` 类目不动（site_disabled_models 无 manual 标记 →
  仅当条目在映射生成前已存在且上游曾实际不可用时才删——保守策略：**只在
  apply 时点 actual 可用**，天然覆盖「曾不可用现已恢复」）。

## 6. 集成点

- 生成：`handler/admin/model_refresh.go` `refreshAccountModels` 成功后调用
  `generateModelRedirects(ctx, db, accountID, clean)`（best-effort，不阻断刷新）；
- 修复触发：`POST /api/model-redirects/apply` 手动触发（**不自动删**——删除是
  破坏性写，需 operator 确认；generate 的自动部分只写映射表，不删禁用）。

## 7. K1b（拍板后再动，不静默实现）

路由匹配 canonical 化——**设计深化（2026-08-01，基于 K1a 落地后的代码勘察）**。

### 7.1 问题确认（热路径证据）

`routing/matcher.go:124 ChannelSupportsRequestedModel(source, requested)`：
- `MatchesModelPattern(requested, source)` 对无通配符的 requested（canonical）是**精确比较**
  → source=actual（`claude-3-5-sonnet-20241022`）不匹配 requested=canonical（`claude-3-5-sonnet`）
  → 通道被 eligibility 拒绝（`routing/router.go:419`），即使通道已挂在该路由下。

**结论**：仅把 actual 通道挂进 canonical 路由（rebuild 侧）不够——请求时的
eligibility 检查仍会拒绝。必须动匹配侧。

### 7.2 实现设计（三件套，全部触及核心）

**A. Eligibility 匹配（热路径，进程内注册表）**
- `routing` 包加进程内 redirect 注册表：`routing.SetModelRedirects(map[string]string)`
  （canonical→actual）+ 反向索引（actual→canonical），map 替换引用重建（读无锁）。
- `ChannelSupportsRequestedModel` 加一步：source 命中反向索引且 canonical == requested
  → 匹配。O(1) 查，无 DB。
- 注册表数据源：`model_name_redirects` 表，启动时加载 + 同步/管理变更后重建
  （K1a 已有变更点）。

**B. 转发改写（canonical → actual）**
- eligibility 通过后，upstream 请求体的 model 必须改写为 actual（上游只认实际名
  ——anthropic 的 `claude-3-5-sonnet-20241022` 类名；不改写上游 404）。
- 改写点：PrepareCtx 之后、dispatch 之前（handler/proxy 层）。
- **风险**：改写影响 proxy_logs 的 model_requested/model_actual 归因 + billing
  lookup（`EstimateBillingCostFromUsage(modelName, ...)` 用请求侧 model 查 ratio）——
  归因需以 requested（canonical）为准，改写只作用于 upstream 出站体。

**C. 计费归因确认**
- `billing_cost.go` 用传入 modelName（归因名）→ ratio lookup。改写后归因名必须
  保持 canonical，出站名才是 actual——需要拆「归因名」与「出站名」两个字段，
  贯穿 proxy 数据流（Ctx 增加 UpstreamModelOverride）。

### 7.3 风险清单（拍板输入）

| 风险 | 等级 | 缓解 |
|:--|:--|:--|
| 热路径新增 map 查（每请求） | 低 | O(1) 引用替换，无锁 |
| 改写遗漏某些出站路径（chat/completions/embeddings/gemini/codex-ws） | 高 | 统一在 PrepareCtx 产出 override，各 dispatch 消费；e2e 覆盖 5 路径 |
| 计费归因漂移（model_actual 显示改写名） | 中 | 归因名/出站名分离，测试断言 proxy_logs 两列 |
| 映射误判（前缀误匹配） | 低 | K1a 规则已验证（日期/版本后缀） |
| 进程内注册表与 DB 漂移 | 低 | 同步/管理变更后必重建 + 启动加载 |

### 7.4 验收（K1b）

- eligibility：canonical 请求选中 actual 通道（5 路径 e2e）；
- 出站体 model = actual，proxy_logs.model_requested = canonical（归因不漂移）；
- 注册表重建后热路径无 stale；误映射不产生（规则复用 K1a）；
- 性能：routing benchmark 增量 < 5%。

### 7.5 建议

K1b 是**真实但可控**的 M 级项：核心风险在 B/C（改写 + 归因分离）。若拍板执行，
建议按 A → C（数据流字段）→ B（改写）顺序，单 PR 内完成 5 路径 e2e。
不做的话，K1a 已提供 disabled_models 修复 + 映射可视化，收益已部分落地。

## 8. 验收（K1a）

- 同步后映射表生成正确（日期/版本后缀 + 大小写归一并去重、精确名不产生映射）；
- generate 幂等（重复跑不产生重复行）；
- apply dry-run 返回将删除的 disabled 条目列表，不实际删；
- apply 实际删除后 events 有记录；上游 actual 不可用时不删；
- 手动映射不被 generate 覆盖；
- 测试：生成规则单测（8 组样例）+ 端点 e2e + PG 方言奇偶。

## 9. 非目标

- 不自动修改 `route_channels.source_model`；
- 不自动删除 site_disabled_models（需 apply 确认）；
- K1b 路由匹配 canonical 化（另行拍板）；
- 不引入远程模型目录。
