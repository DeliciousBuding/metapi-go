# Roadmap

**Last updated**: 2026-08-16

**Repo**: https://github.com/DeliciousBuding/metapi-go

**Release**: v0.13.0 · master CD `ghcr.io/deliciousbuding/metapi-go`

> Current status → [`../STATE.md`](../STATE.md) · Timeline → [`../log.md`](../log.md) ·
> 产品对标 → [`../benchmark.md`](../benchmark.md)

## Open items

| Issue | Title |
|------:|:------|
| [#557](https://github.com/DeliciousBuding/metapi-go/issues/557) | Production multi-channel cascade e2e |
| [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) | Runtime probes for Codex and AnyRouter (optional) |
| [#562](https://github.com/DeliciousBuding/metapi-go/issues/562) | 商汤端口兼容性（自动判失效） |

## Product roadmap（对标 New API × All API Hub，2026-08-14）

> 完整对标与"明确不做"清单 → [`../benchmark.md`](../benchmark.md)；
> 执行层清单与剩余 backlog → [`../analysis/ui-ux-audit-2026-08.md`](../analysis/ui-ux-audit-2026-08.md)。

| Pri | Item | Status |
|-----|------|--------|
| P0 | 客户端配置一键导出（Cherry Studio / CC Switch / env / JSON） | **done**（#657，接入对话框 + keys 行内入口） |
| P0 | 接入向导（平台 → 凭证 → 连通性测试 → 引导建 token 路由） | partial（完成 toast 已改接 `/settings/downstream`；onboarding checklist 与表单连通性测试待做） |
| P1 | 全局搜索（跨站点/账号/模型/路由/告警） | **done**（#658，⌘K Command Palette，后端 `/api/search`） |
| P1 | 首页今日快照（签到/余额/告警/可用性聚合） | **done**（#659，快照横条 + attention 直达） |
| P1 | 价格对比增强（路由分配权重 vs 价格对照） | planned |
| P1 | 告警富化（受影响路由 + 替代站点 + 直达链接） | **done**（#660，3 条核心告警消息富化） |
| P1 | 测试台增强（模板库 + 批量延迟对比） | partial（#662 会话化 + 模板库；批量延迟对比待做） |
| P2 | WebDAV 加密同步 / 移动端 PWA / Realtime/Rerank 转发 | deferred（需需求信号） |

## Engineering backlog

| Item | Note |
|------|------|
| 前端架构化（spec-driven run 2026-08-14） | **done**（2026-08-16 对账）：Phase 1/2 全落地——T1 在 PR #782，T2–T8 在 master，T9 文档销项；计划 `docs/plan/`，Issues #732–#740（milestone WEB-ARCH） |
| CI 旧 schema 升级 job | 并入 test-sqlite/test-pg 或独立 job |
| web api.ts any → Zod | 类型收窄（自 api.ts 拆分后剩余面） |
| a11y 门禁增强 | moderate/minor + 375px + zh-CN + 键盘 + reduced-transparency |
| 测试盲区补测 | scheduler / service / oauth / proxy_log |

## Product honesty

- Sticky sessions: process-local only (single-instance or LB pinning required for multi-instance)
- WebSocket Responses: present (single-instance honesty)
- Update center: external (GHCR/releases); API residual remains 501
- Usage stats: present with residual (multi-instance aggregation is not exact billing)

## Pre-commit checks

```bash
go build ./cmd/server && go vet ./... && go test ./... -count=1 -race
```
