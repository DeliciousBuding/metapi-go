# Product Parity & New API 借鉴 Synthesis — 2026-07-30

**Authority**: leader 综合（metapi-ts 原版对标 + New API 上游借鉴调研，子代理产出经 leader 源码交叉复核）
**Scope**: 产品功能面（非工程纪律，工程优化见 `engineering-optimization-2026-07-30.md`）
**Repos**: `D:/Code/aihub/metapi-ts`（原版 TS）· `D:/Code/aihub/metapi-go`（Go 重写）· New API（QuantumNous/new-api，web 调研）

---

## 1. 定位

metapi-go 是「中转站的中转站」——聚合 New API/One API/Sub2API 等上游中转站成统一网关。故：
- **原版对标**：metapi-ts 的产品功能在 Go 重写中是否漏了。
- **New API 借鉴**：同领域成熟实现里，哪些产品功能 metapi-go 自己可补强（不是抄成另一个 New API；多用户/支付/兑换码/邀请/订阅等终端用户功能与聚合网关定位冲突，明确不适用）。

---

## 2. 原版 metapi-ts 对标结论：基本无回退

子代理逐条核查 TS README 11 项核心功能，leader 复核其 4 个报告缺口：

| 报告缺口 | leader 复核 | 裁决 |
|:---------|:----------|:-----|
| G3 代理调试 trace 端点缺失 | **误报**。Go 已实现 `handler/admin/stats.go:23-24` `/api/stats/proxy-debug/traces` + `/{id}`，`debugTraces`/`debugTraceDetail` 含 attempts 关联 | 剔除 |
| G2 路由自动发现 workflow 是 stub | 部分误报。`routing/workflow.go` 是 thin 委托，但发现循环由 `scheduler/balance` 链路驱动（功能能跑）。是叙事/可观察性问题，非功能缺口 | 降级为文档 |
| G1 余额不足 + 站点账号异常告警 | **真缺口，但 TS 也没做实**。Go `service/alert/alert.go` 仅 `ReportTokenExpired`+`ReportProxyAllFailed`；TS 同样只在每日摘要 count `lowBalanceAccounts`（`balance<1`），无独立实时告警触发器。README 写了但两边都未落地 | 可补（做得比原版更好） |
| G4 WS/Videos residual | 已诚实声明（STATE.md），非新缺口 | 不动 |

**平台适配器**：子代理核实 TS 与 Go **都是 14 个**且集合一致（`platform/registry.go:51-66`），多平台聚合完全对齐，无缺口。

**结论**：Go 重写**没有丢掉任何 TS README 承诺的头部产品功能**，且在慢请求/热力图/品牌分类/跨站比价上略超出 TS。唯一可补的是 G1 余额告警——但这是「原版也未落地的产品承诺」，metapi-go 可借这次做得比原版更好。

---

## 3. New API 借鉴结论（leader 复核后）

子代理给出 10 项高 ROI 借鉴，leader 复核剔除 1 项误报、确认 9 项：

### 3.1 直接适用 / 真缺口

| # | 借鉴项 | leader 复核证据 | 量 | 价值 |
|:--|:-------|:---------------|:--|:-----|
| **N1** | **下游 key IP 白名单/黑名单** | `admin_ip_allowlist` 仅用于 admin auth（`auth/admin.go:45`）；`downstream_api_keys` DDL（`store/migrate.go:1033-1090`）**无 IP 字段**。聚合网关对外暴露 key 却无 IP 限制 = 安全短板 | **S** | 高（基础防护，低成本） |
| **N2** | **公开/下游可见价格目录页** | `/api/stats/model-prices`（`stats.go:33`）已存在但仅 admin 侧。做成下游 key 可见/可选公开 = 跨站比价，聚合网关独有价值（New API 单站 /pricing 的升级） | **S** | 高（聚合网关卖点） |

### 3.2 借鉴优化（metapi-go 已有，New API 更成熟）

| # | 借鉴项 | leader 复核证据 | 量 | 价值 |
|:--|:-------|:---------------|:--|:-----|
| **N3** | 推理参数后缀（-high/-medium/-low/-thinking）+ thinking_to_content | 路由层无后缀解析；reasoning_content 无归一化。聚合层做格式归一是天然位置 | S | 中（New API 差异化便捷特性） |
| **N4** | 渠道测试按钮（Test This Channel） | `/api/models/probe`（`stats.go:40`）是模型级；无 per-route-channel 测试端点。复用 model_probe 逻辑即可 | S | 中（运维 UX） |
| **N5** | 下游 key 级消费分布看板 | `proxy_logs` 已有 `downstream_api_key_id`（`schema.go:219,251`），数据齐备，只差 group-by 视图 | S | 中（为下游消费者服务） |
| **N6** | 日志查询结果 CSV 导出 | `/api/stats/proxy-logs` 无 `format=csv`。审计合规常用 | S | 低-中 |
| **N7** | Prompt Cache Ratio 配置化 | cache_ratio 是成本估算常量，未做成 admin 可配倍率 | S | 低-中 |
| **N8** | 单渠道多密钥轮询 | 一个 route_channel 绑一个上游账号；单渠道多 key 列表可减配置膨胀（1 站点 N key 从 N channel→1 channel） | **M** | 中（schema 改 + 调度器） |
| **N9** | 模型倍率/分组倍率管理后台可视化 | 倍率散落代码常量+配置文件；收口为 admin 可配倍率表 | **M** | 中（跨站成本透明化） |

### 3.3 剔除

| 子代理建议 | leader 复核 | 裁决 |
|:----------|:----------|:-----|
| #8 Playground 在线测试 | **误报**。metapi-go 已有 `web/pages/ModelTester.tsx` + `/api/test/chat` + `/api/test/proxy`（`handler/admin/test.go:26-37`）含 forced-channel | 剔除（已对齐） |

### 3.4 明确不适用（与聚合网关定位冲突）

多用户注册登录体系 · 预充值/支付 · 兑换码 · 邀请奖励 · 订阅套餐 —— 这些是 New API 面向终端用户的业务功能，metapi-go 下游消费者是中转站而非终端用户，引入会冲突。

### 3.5 metapi-go 已超越 New API 的（无借鉴价值）

负载均衡策略（6 种 vs 2 种）· 渠道健康机制（指数退避+恢复探测 vs 连续失败禁用）· 平台适配器（14 个）· 通知渠道（7 个）· 日志审计（proxy_logs+traces+events+backup）· 看板（跨站比价独有）· 自动签到（metapi-go 独有）。

---

## 4. 合并优化优先级（原版对齐 + New API 借鉴）

| 优先级 | 项 | 来源 | 量 | 类型 |
|:------:|:---|:-----|:--|:-----|
| **P0** | N1 下游 key IP 白名单/黑名单 | New API | S | 安全缺口（直接适用） |
| **P1** | G1 余额不足实时告警 + 站点账号异常聚合告警 | metapi-ts | S-M | 原版未落地产品承诺，做得比原版好 |
| **P1** | N2 公开/下游可见价格目录页 | New API | S | 聚合网关独有价值 |
| **P2** | N3 推理参数后缀 + thinking_to_content | New API | S | 借鉴优化 |
| **P2** | N4 渠道测试按钮 | New API | S | 借鉴优化 |
| **P2** | N5 下游 key 消费分布看板 | New API | S | 借鉴优化 |
| **P3** | N6 日志 CSV 导出 | New API | S | 借鉴优化 |
| **P3** | N7 Prompt Cache Ratio 配置化 | New API | S | 借鉴优化 |
| **P3** | N8 单渠道多密钥轮询 | New API | M | 借鉴优化 |
| **P3** | N9 倍率管理后台可视化 | New API | M | 借鉴优化 |

**本轮不自动实现**：以上均为产品功能（非工程纪律），按硬门禁需先开 Issue 讨论 / 用户拍板再动，不得静默自动修。本文件是决策输入，不是执行令。

---

## 5. 来源

- 原版 metapi-ts：`D:/Code/aihub/metapi-ts`（HEAD `41767a6`）
- New API docs：[docs.newapi.pro](https://docs.newapi.pro/en/docs/guide/wiki/basic-concepts/features-introduction) · [channel](https://docs.newapi.pro/en/docs/guide/feature-guide/admin/channel) · [token](https://docs.newapi.pro/en/docs/guide/feature-guide/user/token) · [pricing](https://docs.newapi.pro/en/docs/guide/feature-guide/user/pricing)
- metapi-go 现状实证：`auth/admin.go` · `store/migrate.go:1033` · `handler/admin/stats.go` · `handler/admin/test.go` · `store/schema.go:219,251` · `service/alert/alert.go`
