# BLOCKED

**状态：无**

无阻塞项，全部按研究结论自主落地。

## 已落地的自主决策（无需裁决，仅备查）

1. **catalog 价单位换算**：models.dev 为 USD/1M tokens；`EffectiveUnitCost` 的 observed/configured 信号是每请求美元。为可比，catalog 价按 `routing.DefaultPriceCompareSampleUsage` 同款参考样本（1k input + 1k output）换算为每请求成本：`(inputPerM + outputPerM) / 1000`。实现见 `CatalogEntry.ReferenceUnitCost`。
2. **诚实标注**：官方厂商站点（platform + host 白名单命中）标 `catalog`；第三方中转站点一律标 `catalog_estimate`，绝不冒充真实支付价。白名单未知/DB 查询失败默认按 relay 处理（诚实兜底）。routing 侧经新接口 `CatalogPricingResolver` 透传 provenance（`CostSignal.Source` 支持 `catalog_estimate`）。
3. **官方 host 白名单**：openai=api.openai.com、claude=api.anthropic.com、gemini(+cli)=generativelanguage.googleapis.com、codex=api.openai.com/chatgpt.com、grok=api.x.ai；其余平台一律 relay。
4. **刷新策略**：启动即拉（后台，不阻塞启动）+ 可配周期（`PRICING_CATALOG_REFRESH_MIN`，默认 60）；拉取失败保留上一份快照，从未成功时用编译期 presets（11 个常用模型，models.dev 2026-08-16 快照，源码标注日期）。
5. **Claude 别名**：无日期后缀的 family 名（如 `claude-3-5-sonnet`）解析到最新 dated 快照（按 release_date，其次按 id 字典序）；`claude-3.7-sonnet` 点号写法归一为 `claude-3-7-sonnet`；provider 前缀（`anthropic/...`）剥离。dots 仅对 claude 家族归一，`gpt-4.1` 等真实点号模型不受影响。
6. **models.dev tier 价**（`context_over_200k`/`tiers`）暂忽略，仅用 base tier；缺失 input/output 或负价的条目直接跳过。
7. **reconfigure 安全**：provider 为进程级单例，重复 `ConfigureProxyUpstream` 复用既有 provider 与刷新循环；`PRICING_CATALOG_ENABLED=false` 时 resolver 为 nil，路由行为与改造前完全一致。

## 非阻塞跟进项（不在本 PR 范围）

- admin price-compare 界面尚未消费 catalog provider（其 Source 仍是 billing/observed/configured/fallback 四档）；catalog/catalog_estimate 当前只流入 routing `CostSignal`。后续可把 catalog 作为第五档并入 `BuildPriceCompareRow`。
- presets 表为快照，扩表需人工同步 models.dev；刷新失败超过 24h 时建议告警（当前仅 warn 日志）。
