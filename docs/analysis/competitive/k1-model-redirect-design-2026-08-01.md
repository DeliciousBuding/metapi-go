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

路由匹配 canonical 化：`routing.MatchesModelPattern` 增加映射感知——需要
（a）映射缓存（避免热路径 DB 查询）；（b）语义确认：实际名 `claude-3-5-sonnet-20241022`
挂到 pattern `claude-3-5-sonnet` 的路由后，upstream 发送时用哪个名？（现有
`source_model` 语义是「该通道对应上游哪个模型」，改名会影响计费归因/日志）——
**此点在 K1a 落地后基于真实数据再评估**。

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
