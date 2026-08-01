# sub2api / cliproxyapi 产品对标与 borrow 决策（2026-08-01）

**日期**: 2026-08-01 · **状态**: 决策输入文档（已勘察，部分项已立项/关闭）

> 上游: sub2api（订阅配额分发网关，Go+Vue+PG+Redis，本地镜像仓库）
> 与 cliproxyapi（多提供商 CLI 代理，Go，v7，本地镜像仓库）。
> 用户 /goal 最初点名的三家竞品（newapi / all-api-hub 已收口），此为本批。
> 遵循「不发明、不静默实现、不重复造轮子」硬门禁。

## 1. sub2api 画像

把上游 AI 订阅（Claude/OpenAI/Gemini/Grok/Antigravity OAuth 或 API Key）汇聚成
**可计费、可售卖的 API 网关**：账号池 → 分组（group 限额/倍率/降级）→ 用户
API key（quota + 限流窗）→ 平台级 `user_platform_quota`（日/周/月 USD 窗口，
`SELECT FOR UPDATE` + `ON CONFLICT` 滚动重置）→ 内置支付（计划/订单/退款/
webhook: EasyPay/支付宝/微信/Stripe/Airwallex）→ Ops 运维台（告警规则/实时
QPS WS/错误聚合）→ 合规（TOTP 2FA + step-up + 全量审计日志 + 风控 + prompt
审计）。

## 2. cliproxyapi 画像

本地/集群多提供商代理：订阅账户 OAuth 接入（PKCE/device flow/本地回调/自动
刷新）+ 多账户负载均衡（session-affinity + failover + 冷却黑名单）+ 协议双向
翻译（OpenAI/Gemini/Claude/Interactions）+ 推理重放 + 签名缓存 + payload 规则
引擎 + 插件宿主 + 单端口协议嗅探。

## 3. 逐项对照（metapi-go 现状）

| # | 上游能力 | metapi-go 现状 | 结论 |
|:--|:--|:--|:--|
| S1 | 上游额度/权益探测（upstream-billing-probe，403/429 语义区分） | `RefreshBalance` 周期调度 + `balance_history`（A1）+ G1 低余额告警 + G1b `ProbeBatch` 批量通道健康验证 | **接近等价**——探测维度（余额/健康）已覆盖；403/429 语义区分在 failover 已分级处理。**关闭** |
| S2 | 日/周/月三窗口平台额度（user_platform_quota） | 余额模型 + downstream key 权重/限流窗 | **待拍板（决策包见 §7）**——「自然周期限额」vs「余额」是计费语义分叉；推荐：**关闭**（B3 面向外部售卖的消费额度，metapi-go 内部工具 + 单管理员，现有 key 限流窗/权重已覆盖内部使用；售卖场景属 S5 非目标）。若未来接外部售卖再开 |
| S3a | TOTP 2FA + step-up 二次认证 | admin auth = Bearer token + IP CIDR 白名单，无登录态 | 2FA 无承载点（无用户/密码登录体系）；step-up 需先有会话。**deferred（M，需先建登录体系）** |
| S3b | 全量管理操作审计日志 | events 表（业务事件）无 admin 写操作审计 | **真实缺口（S）——本轮已实现**：`admin_audit_logs` 表 + 中间件 + 管理端点 |
| S4 | Ops 运维台（告警规则/静默/实时 QPS WS/错误聚合） | events + notify（D1 4 渠道）+ Dashboard 关注面板 + 调度状态（C1） | 告警/通知/错误聚合**已等价**；实时 QPS WS 是增量（S-M，`coder/websocket` 已就绪）。**QPS WS deferred（P2）** |
| S5 | 多 OAuth 登录源 + 邀请返利 + 兑换卡密 | 内部工具，单管理员 | 面向售卖的获客闭环，metapi-go 是内部网关。**非目标（内部工具）** |
| S6 | 渠道监控（channel_monitor daily rollup + 请求模板） | stats 体系（proxy_logs/site_day_usage/heatmap/慢请求） | **等价或更全**（数据底座在 metapi-go 侧）。**关闭** |
| S7 | 异步/批量图片 + 视频生成任务 | images/videos 转发面 | 批量任务编排是新增面（M）。**deferred** |
| C1 | 故障冷却黑窗（.cds 持久化 + 单凭据 disable-cooling） | 每通道 `cooldown_until` DB 持久化 + Fibonacci 冷却 + `disable_cooling` | **等价**（metapi-go 冷却天然持久化于 DB）。**关闭** |
| C2 | 会话粘性路由 + 自动 failover | `stable_first` 策略 + 重试 failover（#579 多凭证绑定） | **等价**。**关闭** |
| C3 | quota-exceeded 多级降级链（换项目→预览模型→credits） | — | 上游特定（antigravity）业务链。**非目标** |
| C4 | 别名池（多上游共一 client 别名） | token_routes model_pattern + 多通道 | **等价**（一个 pattern 多通道即别名池）。**关闭** |
| C5 | payload 规则引擎（gjson default/override/filter） | transform 包 + sanitizeUpstreamJSONBody + model_mapping | **接近等价**（metapi-go 更结构化）。**关闭** |
| C6 | 单端口协议嗅探复用 | 单端口多路径（/v1/* 全注册） | **等价**。**关闭** |
| C8 | 订阅 OAuth 接入（PKCE/device flow） | OAuth route units + Codex OAuth + OAuthManagement 页 | **已等价**（metapi-go OAuth 是 route-unit 模式）。**关闭** |
| C9 | 模型注册表远程更新 | refreshAccountModels + model_availability 同步 | **等价**。**关闭** |
| C10 | 签名缓存 / reasoning replay | — | 上游签名校验（claude cch 等）是伪装层技术，metapi-go 不需要（自有账号体系）。**非目标** |

## 4. 立项结论

| 项 | 量级 | 决定 |
|:--|:--|:--|
| **B1 admin 操作审计日志** | S | **已发**（表 + 中间件 + 查询端点 + 前端页） |
| **B2 实时 QPS 运维 WS** | S-M | **已发**（`/api/admin/ops/ws?token=` 每秒推流 + Dashboard 实时面板） |
| B3 自然周期平台额度（S2） | M | **待拍板**——决策包 §7：推荐关闭（售卖场景=非目标 S5，内部使用已被 key 限流窗/权重覆盖）；实现需新表 + 滚动重置 + 拒绝语义（M 级） |
| B4 批量媒体任务（S7） | M | deferred |
| B5 登录/2FA 体系（S3a） | M | deferred（需先建会话体系） |

## 5. 非目标（内部工具定位）

- 内置支付/订单/退款（metapi-go 面向 TokenDance 生态内部使用）
- 多 OAuth 登录源/邀请返利/卡密兑换（获客闭环，无外部用户）
- 上游签名伪装/推理重放（cliproxyapi 的 cloak 技术栈）

## 7. B3 拍板决策包（2026-08-01 补全）

**问题**：是否实现「自然周期平台额度」——给下游 key 设日/周/月 USD 消费窗口，周期滚动重置，超限拒绝。

**分叉本质**：上游 S2 是「**消费额度窗口**」（对下游售卖收钱），metapi-go 现有模型是「**上游账号余额**」（从账号池扣）+「**key 限流窗**」（RPM/TPM 请求数，无金额语义）。两者语义不同，B3 是新增一套金额窗口体系。

**选项**：
| 选项 | 成本 | 收益 |
|:-----|:-----|:-----|
| A. **关闭（推荐）** | 0 | 内部工具 + 单管理员：无外部售卖场景（S5 非目标）；内部使用已有 key 限流窗（请求数）+ weight（分配）覆盖；金额窗口对内部使用无实际约束意义 |
| B. 实现 | M 级（新表 user_platform_quota + 滚动重置（SELECT FOR UPDATE / ON CONFLICT）+ 超限拒绝语义 + 前端配额页） | 仅当未来向外部售卖 API 时才有价值 |

**推荐**：A 关闭，理由同 S5（获客/售卖闭环=非目标）。若 3 个月内出现外部售卖需求再重开（设计文档可复用 S2 描述）。

**拍板方式**：管理员一句话（「B3 关闭」或「B3 实现」）即可；本文档即决策记录。

## 6. 本轮实现：B1 管理操作审计日志

- 表 `admin_audit_logs`（AdditiveStep sc2_012）：id / actor（token sha256 前缀）/ method /
  path / status / request_id / remote_ip / created_at；写操作（POST/PUT/PATCH/DELETE
  且非公开路径）记录，GET 不记；
- 中间件挂在 admin auth 之后（只对鉴权通过的请求记，401 由 auth 层自身处理）；
- `GET /api/admin/audit-logs?limit=&method=&path=`（admin auth 保护）+ Settings 页表格。
