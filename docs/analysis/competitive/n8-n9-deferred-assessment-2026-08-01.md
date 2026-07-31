# N8/N9 deferred 项评估（New API borrow）

**日期**: 2026-08-01 · **状态**: N8 = 架构已等价（建议关闭）；N9 = 真实缺口（S 级读取面可做，M 级写入面待拍板）

> 上游: New API 的渠道倍率/密钥模型。评估基于当前 tip `7597a07` 的代码证据，
> 遵循「不发明、不静默实现、不重复造轮子」硬门禁。

## N8 单渠道多密钥轮询 — 架构已等价，建议关闭

New API 模型：`channel`（渠道实体）下挂 N 个 `key`（凭证子资源），请求在 key 间
轮询、失败 key 冷却。metapi-go 模型：凭证即 `account`，绑定进 `route_channels`
（一 route 多 channel 行），路由在 channel 间选择。逐项对照：

| New API 能力 | metapi-go 等价 | 证据 |
|:--|:--|:--|
| 多 key 轮询 | 多 channel + `round_robin` 策略 | `routing/ports.go:145` StrategyRoundRobin；`routing/route_units.go:61` 默认 round_robin |
| 加权轮询 | `weighted` 策略 + channel.weight × key_weight | `routing/ports.go:144`；#547 `downstream_api_keys.key_weight` |
| 失败 key 冷却 | 每通道独立冷却 | `routing/router.go` RecordFailure/RecordProbeFailure → `ApplyRoundRobinCooldown`/Fibonacci + `cooldown_until` |
| 健康探测 | model-probe 每通道轻量探测 + 路由健康记录 | `scheduler/model_probe.go` ProbeBatch + recorder |
| 多凭证组轮询（正式形态） | OAuth route unit 多成员 | `oauth_route_units`/`oauth_route_unit_members` + 成员级 failCount/cooldown |
| 一键多 key 绑定 | Accounts 批量 API Key 创建 + 通道重建 | `accounts.go` batch API Key 流 |

**结论**: N8 在 metapi-go 架构中是**重复模型**——channel 即「渠道」，account 即
key，多 key 轮询 = 多 channel 挂同一 route。再实现「渠道内多 key」会产生两套
平行的凭证模型（维护/迁移成本高、门禁冲突）。**建议关闭 N8**，不立项。

真正的 UX 增量（可选，非 N8）：tokenRoutes 页把同一 route 的多通道按
「轮询组」视角展示健康/冷却/权重——现有页面已有通道行 + N4 行内测速，此视角
价值有限，不建议单独立项。

## N9 倍率管理后台可视化 — 真实缺口，拆两期

New API 的「倍率」= 计费倍率配置（model price × multiplier），有集中管理界面。
metapi-go 的计费/倍率事实分散在多处：

| 倍率面 | 位置 | 现状 |
|:--|:--|:--|
| 账号单价 | `accounts.unit_cost`（可选） | 仅账户表单可见 |
| 通道权重 | `route_channels.weight` | tokenRoutes 通道行可见 |
| 站点全局权重 | `sites.global_weight` | Sites 表单可见 |
| 下游 key 权重 | `downstream_api_keys.key_weight`（#547） | DownstreamKeys 表单可见 |
| 模型价格 | model_day_usage 观测 / 账单明细 | `model_price_compare` 聚合 |

**缺口**: 没有一个「倍率总览」——operator 无法一眼看到「哪个账号单位成本最高、
哪个通道权重最大、倍率配置是否一致」。这是真实的管理盲区。

**N9a（S，只读，低风险，建议直接做）**: `GET /api/models/rates` 聚合视图——
每账号 unit_cost + 每通道 weight + 每站点 global_weight + 每 key key_weight +
模型观测成本，一张表 + 排序 + 汇总（不改任何计费/路由逻辑，纯读）。

**N9b（M，写入面，待拍板）**: 倍率配置的集中编辑（改 unit_cost/weight 的批量
入口）——触及计费语义（unit_cost 如何参与 estimated_cost 计算需先确认），
需设计文档 + 拍板。

## 建议

1. N8: **关闭**（架构等价，证据如上）；MASTER 标记 `n8_closed_architecture_equivalent`。
2. N9a: 直接实现（S 级只读聚合 + Settings/Models 页展示）。
3. N9b: 设计文档 → 拍板 → 实现（M 级触及计费）。
