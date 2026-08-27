# 竞品研究 — new-api × axonhub × sub2api（实现级借鉴清单）

**Date**: 2026-08-28
**Status**: 研究结论（Wave 15 Lane C）· 只读研究，不含任何代码改动
**基线**: metapi-go v0.16.15（master）· 能力清单以 `STATE.md` 为准

> 本文聚焦**实现级设计模式**。产品定位与能力矩阵见 [`benchmark.md`](../benchmark.md)；
> 凭证体系对比见 [`credential-mental-model-and-ia.md`](credential-mental-model-and-ia.md)。
> 两文已有结论不再重复。

## 研究对象与证据基线

| 对象 | 来源 | 基线 | 形态 |
|---|---|---|---|
| new-api | GitHub QuantumNous/new-api 浅克隆（任务书所指 `calciumion/new-api` 已迁入 QuantumNous org，原地址 404） | 2026-08-28 master | Go 单体 + React 19 SPA + Electron 壳 |
| axonhub | looplj/axonhub 本地检出（unstable 分支） | HEAD `9dfd6ac`（2026-08-10） | Go + React SPA（GraphQL 管理面） |
| sub2api | GitHub Wei-Shaw/sub2api 浅克隆 | 2026-08-28 master | Go + Vue 3 SPA |

**证据约定**：仓库相对路径 + 行号/函数名；【推断】= 间接证据推测；【未验证】= 本次阅读范围内无法确认。
三产品均为静态阅读；未运行服务、未访问真实上游。

## 0. 概览

| 维度 | new-api | axonhub | sub2api | metapi-go（对照） |
|---|---|---|---|---|
| 定位 | 多租户计费网关（卖 API 的站长） | All-in-one 开发平台（企业 RBAC/多项目） | 订阅配额池化分发（拼车） | 自用/小团队聚合网关 |
| 管理面 API | REST | GraphQL | REST | REST |
| 前端 | React 19 + TanStack + Tailwind 4 + base-ui | React 19 + TanStack + Tailwind 4 + shadcn | Vue 3 + Vite 5 + Tailwind 3 | React 19 + TanStack + Tailwind 4 + shadcn |
| 首启 | 4 步 Setup 向导 | 单表单初始化 | 带连接测试的 4 步向导 | env AUTH_TOKEN 零配置（定向，无向导） |
| 首屏→第一个请求 | ~5 屏 / ~10 次交互 | ~5 次页面操作 | ~6 步（AutoSetup 可压到 4） | 向导已交付（站点→账号→路由） |
| 后端测试规模 | 158 个 `_test.go` / 1038 个 Test | ~440 个 `_test.go`（含独立 llm 模块，约 15.6 万行） | 1206 个 `_test.go` + 270 个迁移 | 见 STATE（routing golden + 双 dialect 门禁等） |
| 浏览器 E2E | 无 | 16 个 Playwright spec（不进 CI） | 无（组件级 246 个 spec） | CI 内视觉回归 + 浏览器 E2E |
| 真实上游测试 | 无 | 独立 `integration_test/` module（手工） | `//go:build e2e`（手工） | 4/16 进 CI |

一句话总结：**new-api 强在协议正确性与策略可解释，axonhub 强在故障闭环与可观测工程，
sub2api 强在账号池状态管理与工程密度；metapi-go 在 CI E2E 与门禁密度上四者最强。**

## 1. 轴1 产品动线

### 1.1 new-api — Setup 向导 + Playground
- 首启 4 步向导：数据库自检 → 管理员账号 → 使用模式（对外运营/自用/演示，写入 option）→ 确认初始化
  （`web/src/features/setup/setup-wizard.tsx:50` STEPS；后端 `controller/setup.go`；端点免鉴权
  `router/api-router.go:22-23`，完成后 `constant.Setup=true` 锁死）。
- **数据库自检步**：第一步即提示「当前 sqlite（warning 色），生产应换库」——把部署质量风险前置到
  onboarding（`features/setup/components/complete-step.tsx` DATABASE_VARIANT）。
- 初始化后不自动登录（【推断】：PostSetup 无 session 下发，`setup-wizard.tsx:108-111` 跳登录页）。
- 动线：向导 → 登录 → 渠道抽屉（类型/Key/模型/分组；模型可从上游拉取 `dialogs/fetch-models-dialog.tsx`）
  → 建令牌（仅 name 必填，`features/keys/lib/api-key-form.ts:37`）→ 请求或内置 Playground
  （`POST /pg/chat/completions`，`router/relay-router.go:62-67`）。
- 计价：倍率体系 `setting/ratio_setting/`；支持从上游同步价格（`controller/ratio_sync.go`）；
  公开价格页 `/pricing` 免登录（`controller/pricing.go`）。
- 登录后无任务式引导；渠道编辑抽屉用分区完成态（complete/configured/error/idle）部分补偿
  （`drawers/channel-mutate-drawer.tsx:210`）。

### 1.2 axonhub — 计价环节默认自动化、可跳过
- 单表单初始化（姓名/邮箱/密码≥8位/品牌名）→ 强制跳登录页；`frontend/src/routes/(auth)/initialization.tsx`，
  `POST /admin/system/initialize` 免鉴权（`internal/server/routes.go` unSecureAdminGroup）。
- 建渠道后模型同步（默认 1h）与探测（默认 5min）自动运行（`internal/server/biz/system_default.go`
  defaultChannelSetting）；价格按渠道模型版本化存储（`biz/channel_price.go`）——计价步骤默认可跳过。
- Key 创建成功后**自动弹「查看」对话框**展示一次性明文（`features/apikeys/components/apikeys-create-dialog.tsx`）。
- 发现两处文档与实现漂移（引以为戒）：quick-start 仍写默认凭据（与初始化向导矛盾，疑过期）；
  README 写密码≥6位而代码强制≥8位（`initialization-form.tsx` zod `min(8)`）。

### 1.3 sub2api — 部署全链路产品化
- 4 种部署形态：一键脚本（systemd、自动生成密钥）、Docker Compose（自动写密钥进 .env）、
  Apple Container、源码单二进制（前端内嵌）（`deploy/install.sh`、`deploy/docker-compose.yml`、
  `backend/internal/web/embed_on.go`）。
- 向导 4 步（数据库→Redis→管理员→完成），**每步带连接测试按钮**
  （`frontend/src/views/setup/SetupWizardView.vue:267` TestRedisConnection）；完成后写
  config.yaml + `.installed` 锁（`backend/internal/setup/setup.go:300-462`）；
  另有 env AutoSetup 通道可跳过向导（【推断】：`setup.go:567` AutoSetupFromEnv）。
- 管理台内一键升级 + 回滚（`backend/internal/service/update_service.go`）。
- 计价挂在分组上（按模型定价/倍率，migrations 217–228）；Simple Mode（`RUN_MODE=simple`）隐藏 SaaS 面。
- 加分项：仓库内置面向 agent 的管理 skill（`skills/sub2api-admin/SKILL.md`，先读后写/写后复核工作流）。

### 1.4 与 metapi-go 对照

| 方面 | 结论 |
|---|---|
| 零配置启动 | metapi-go 最优（env + SQLite 默认）；三竞品都需初始化向导 |
| 连接自检 | sub2api 最强（逐步连接测试）；metapi-go 站点向导有内联凭证验证（已交付），无 DB 级自检（单二进制，DB 失败面小，定位合理） |
| 产品内更新入口 | sub2api 有；metapi-go update-center 为诚实 501 残留（见 STATE） |
| Key 一次性明文 | axonhub 自动弹窗；metapi-go 客户端导出对话框已交付（等价能力，入口不同） |
| 公开价格页 | new-api `/pricing` 免登录；metapi-go price-compare 面向运营者（定位差异，无需补） |

## 2. 轴2 前端 / UIUX

### 2.1 列表页状态呈现（含 metapi-go 现状对照）

| 模式 | 证据 | metapi-go 现状 |
|---|---|---|
| 自动禁用与手动禁用分色 | new-api `features/channels/constants.ts:135-152`（auto=warning 色） | 未对照（状态语义细节需查） |
| 多 key 渠道 per-key 独立状态 | new-api MULTI_KEY_STATUS_CONFIG + `dialogs/multi-key-manage-dialog.tsx` | 已等价（channel≈account_token，天然 per-token） |
| 骨架行数=当前 pageSize | axonhub `channels-table.tsx:319` | 未对照 |
| **状态徽章可点击→根因弹窗** | sub2api `components/account/AccountStatusIndicator.vue` + `TempUnschedStatusModal.vue`（触发码/关键词/阈值/计数窗口/恢复时间 + 一键重置） | **缺口**：cooldown 仅存计数器与时间戳 |
| **健康历史条**（15 根竖条 + 成功率/TTFT/TPS tooltip） | axonhub `features/channels/components/channel-health-cell.tsx` | **缺口**（数据层已有 `model_probe_results`） |
| 错误横幅=筛选入口 | axonhub `features/channels/components/channels-error-banner.tsx`（「N 个渠道异常」一键过滤/退出） | 错误横幅已有 Retry（sites/routes）；「计数→过滤视图」缺 |

### 2.2 错误处理与反馈
- new-api：后端错误消息**映射为 i18n key 再翻译**弹 toast，不透传原文（`web/src/lib/handle-server-error.ts`）；
  业务信封 + HTTP 双层拦截（`lib/http-client.ts:80-92`）。
- axonhub：GraphQL `extensions.code` → i18n 映射（`lib/error-parser.ts`）；401 清会话跳登录、
  403 只报错不清会话（`src/gql/graphql.ts`）；QueryClient retry 排除 401/403/422。
- sub2api：axios 拦截器 401 自动 refresh 重试一次（`frontend/src/api/client.ts:163-165`）；
  流式错误由后端注入 stream error event（`backend/internal/handler/stream_error_event.go`）。

### 2.3 引导与导航
- **情境化 driver.js tour**：axonhub 3 套（重试策略/自动禁用/模型页，`features/onboarding/*-onboarding-flow.tsx`，
  完成写 onboarded 标记）；sub2api 亦有（`composables/useOnboardingTour.ts`）。metapi-go：无
  （已有 guided onboarding 深链 + ⌘K，tour 属增量）。
- 路由预取 + 导航加载态：sub2api `composables/useRoutePrefetch.ts` / `useNavigationLoading.ts`
  缓解懒加载白屏；metapi-go【未验证】。
- 表格 composable 组：sub2api `usePersistedPageSize`/`useSwipeSelect`/`useKeyedDebouncedSearch`
  （各带 spec）；metapi-go 已有 URL 单一所有者的表格状态（STATE 已交付），不重复。

### 2.4 i18n / 主题 / 移动端
- new-api：7 语言 + `i18n:sync` 翻译同步审计产物（`web/src/i18n/locales/_reports/_sync-report.json`）；
  **后端通知文案也 i18n**（`i18n/i18n.go`，通知/邮件多语言）。metapi-go 前端 parity gate 已交付、
  后端消息英文化已收口；7 语言不需要，「后端通知 i18n」留作将来多语言通知的参考模式。
- axonhub：2 语言 + `check_translation_keys.sh` 校验脚本（metapi-go vitest i18n gate 已等价）；
  9 套配色 × 明暗（`context/theme-context.tsx`）（metapi-go 5 轴 10 preset 更强，不借鉴）。
- sub2api：locale 按模块拆分 + key 冲突检测测试（`i18n/__tests__/localesNoKeyCollision.spec.ts`）。
- 移动端：axonhub 有专门 mobile-layout E2E（`frontend/tests/mobile-layout.spec.ts`）；sub2api 抽屉侧栏
  + 社区 RN 移动控制台。metapi-go 移动端已经 Wave 7/9 深审（已交付），等价。

## 3. 轴3 后端稳定性

### 3.1 new-api — 策略可解释
- 重试主循环 `controller/relay.go:194`：**RetryTimes 默认 0**（`common/constants.go:133`，已验证）——
  默认不重试，避免默认重放放大上游计费；重试时换渠道（重试序号=优先级阶梯，同级加权随机，
  `model/channel_cache.go:114`、`service/channel_select.go:83`）。
- **状态码区间运营者可调**：`setting/operation_setting/status_code_ranges.go`（已验证）——
  retry 区间与 disable 区间独立；默认自动禁用区间仅 401；504/524 硬编码永不重试
  （上游网关超时不盲目重放）。
- 自动禁用：上游错误关键词匹配（7 条，如余额不足，`setting/operation_setting/operation_setting.go:8`）
  + 状态码；**自动恢复**：自动禁用态请求成功即重新启用（`service/channel.go:68-79`），
  手动禁用不自动恢复（语义正确）。
- 超时：`RELAY_TIMEOUT` 默认 0（交环境自定）、IdleConnTimeout 90s（`service/http_client.go:93`）；
  **流 chunk 间隔超时 STREAMING_TIMEOUT 默认 300s，每收到一块重置 ticker**
  （`relay/helper/stream_scanner.go:88-250`）——区分「总时长」与「流卡死」；SSE 保活 ping
  （`relay/channel/api_request.go:405`）。
- 限流分层：全局 360 次/180s + 敏感操作 20 次/20min + **模型级滑窗**
  （`middleware/model-rate-limit.go`；Redis Lua 固定窗口，无 Redis 回落内存）。
- 优雅停机 120s（`main.go:222-232`）。可观测：`/api/status`、Uptime Kuma 面板 `/api/uptime/status`、
  `/api/perf-metrics`、可选 pprof/Pyroscope；Prometheus 端点【未验证】。

### 3.2 axonhub — 故障闭环 + 可组合均衡
- 负载均衡：4 策略可切（adaptive 复合打分/failover/circuit-breaker/round-robin），统一 `Score()` 接口
  + partial-sort top-k（`internal/server/orchestrator/load_balancer.go`、`orchestrator.go:46-71`）。
- **渠道×模型熔断**：连续 3 失败→half_open、5→open、失败计数 TTL 30min、open 期每 5min 探测、
  half-open 权重 0.3、探测租约防并发穿透（`biz/model_circuit_breaker.go`）。
- 自动禁用：阈值制；多 key 渠道禁用**单 key** 而非整渠道 + Webhook 通知；Key 恢复后
  **自动重新启用**（`biz/channel_apikey.go:354` applyRecoveredChannelStatus）。
- 主动探测：每分钟 cron 按渠道频率（默认 5min）采集成功率/TTFT/TPS，落 `channel_probe` 表，
  **同一份数据喂前端健康条与均衡器**（`biz/channel_probe.go`）。
- 限流三层：渠道并发信号量 + FIFO 等待队列（队列满/超时错误）→ RPM 准入（队列拒绝不消耗 RPM）
  → 配置面校验（`orchestrator/channel_limiter.go`、`rate_limit_admission.go`）。
- 重试：默认跨渠道 3 / 单渠道 2 / 间隔 1s；**流内容已提交后不再重试**（防重复输出，
  `orchestrator/retry.go` 注释）；「流首事件超时」与「非流响应超时」独立旋钮；渠道级自定义
  重试状态码/错误正则。
- HTTP 客户端：dial 30s / TLS 握手 10s / MaxIdleConns 100 / IdleConnTimeout 90s / 强制 HTTP2；
  **上游错误体截断 1MB 上限**（防大 body 打爆内存，`llm/httpclient/client.go` MaxErrorBodySize）。
- 生命周期：fx StopTimeout 30s；启动时清理上次崩溃遗留的 processing 记录；SIGHUP 热加载部分配置
  （`reload_unix.go`）。
- 可观测：zap JSON + lumberjack；**access log 只记 ≥400 或带错误的请求**（降噪，
  `middleware/access_log.go`）；OTel 指标（prometheus/otlphttp/console exporter，默认关闭，
  `internal/metrics/metrics.go`）。

### 3.3 sub2api — 账号池状态机 + 计费安全
- failover 循环（`backend/internal/handler/failover_loop.go`）：同账号重试≤3 次/间隔 500ms；
  瞬时错误指数退避封顶 8s；**利润否决封顶 10 次防活锁**；重试耗尽→临时封禁账号
  （TempUnscheduleRetryableError）。
- 错误策略中枢（`service/ratelimit_service.go:257`）：429 冷却 5s~7200s，优先读 `Retry-After`；
  OpenAI 403 冷却 10min、180min 窗口命中 3 次禁用账号；封禁态多形态检测
  （ChatGPT deactivated、402 deactivated_workspace）。
- 临时不可调度状态**结构化双写**：触发码/关键词/规则索引/阈值/计数窗口，Redis + DB
  （`service/temp_unsched.go:8-25`）——前端根因弹窗直接消费。
- 并发槽：Redis ZSET per account/user，时间戳清过期槽 + 启动清旧进程残留（`service/concurrency_service.go:22-50`）；
  WS 独立租约（TTL 60s / 20s 刷新）。
- 限流：Redis Lua `INCR+PEXPIRE` 原子脚本 + TTL 自愈（检测无 TTL 即修复并记日志）+ 可选
  fail-open/close（`middleware/rate_limiter.go:17-100`）。
- **计费写入解耦**：用量记账 worker pool 自动扩缩 128→512，队列溢出**默认同步兜底不丢账**，
  区分 dropped / dropped_stopped / sync_fallback 语义（`service/usage_record_worker_pool.go:16-55`）。
- 超时全部配置化：响应头超时 / 首个语义输出超时（高推理档位独立更长阈值）/ 流数据间隔超时 /
  会话空闲超时（`backend/internal/config/config.go:949-1049`）；关闭时透传客户端超时头
  （如 x-stainless-timeout）防上游提前断流。
- 优雅停机：5s 上下文（`backend/cmd/server/main.go:181-191`）。【推断】对流式长连接偏短；
  流排空逻辑【未验证】。
- 可观测：zap + lumberjack 运行时可调级；ops_error_logger 归因；audit_logs 表；管理台 ops 仪表盘
  15+ 卡片（错误分布/延迟/切换率/吞吐）。

### 3.4 与 metapi-go 对照（已有能力，避免重复建议）
- Fibonacci backoff 冷却（`routing/cooldown.go` 已验证：base 15s × fib(n)，封顶 30 天）+
  round-robin 分级 [0/10min/1h/24h]；site×model 运行时熔断（STATE）。
- 首字节超时 `PROXY_FIRST_BYTE_TIMEOUT_SEC`、流/缓冲字节上限（`PROXY_MAX_STREAM_RESPONSE_BYTES` 等）、
  WS 读空闲 10min（`proxy/responses_ws.go:52`）。
- `PROXY_ERROR_KEYWORDS` 关键词故障判定（`proxy/failure_judge.go`）；异步日志批量写（ProxyLogAsync）。
- 会话级 sticky + 并发租约（ProxySessionChannel* 配置族），与 axonhub channel_limiter 语义等价。
- **确认的缺口**：① cooldown/breaker 不记录结构化触发原因——schema 只有 fail_count /
  consecutive_fail_count / cooldown_level / cooldown_until（`store/schema_ddl.go:370-399`，已验证）；
  ② 重试状态判定为硬编码区间（`proxy/retry_policy.go:146-165`，已验证）；③ 未发现 SSE 流
  chunk 间隔超时（仅 first-byte + WS idle）【未验证：热路径或有其他机制】。

## 4. 轴4 测试方法

| 维度 | new-api | axonhub | sub2api | metapi-go |
|---|---|---|---|---|
| 单元规模 | 158 文件 / 1038 Test | ~440 文件（约 15.6 万行，含独立 llm 模块） | 1206 文件 + 每个迁移带回归测试 | 见 STATE（规模不是短板） |
| 快照/黄金文件 | **36 份协议转换 golden**（`relaykit/relayconvert/testdata/golden/`，已验证） | `llm/simulator` 离线转换链 | 核心路径专项测试 | `routing/golden_test.go`（选择行为）；**transform 层无签入快照**（已验证无 testdata） |
| 真实 DB | 纯 Go SQLite 内存库真跑 | enttest 内存 SQLite | testcontainers PG/Redis + miniredis | SQLite/PG 双方言门禁 + CI 真 PG service（`.github/workflows/main.yml:244`） |
| 浏览器 E2E | 无 | 16 spec，**不进 CI**（`scripts/e2e/e2e-test.sh` 手工，DB 方言可切） | 无（组件级止） | **进 CI**（视觉回归 10 页 + 浏览器 E2E） |
| 真实上游 | 无 | `integration_test/` 独立 module（手工、需真实 key） | `//go:build e2e` 手工（注释明确防凭证入史） | 4/16 进 CI（容器链） |
| Lint/安全 | 仅 vet | golangci-lint 进 CI | golangci-lint v2.13 + govulncheck + pnpm audit 每周 cron + 例外白名单脚本 | golangci-lint + govulncheck（`main.yml:64`） |
| 特色 | 合并后对 base 分支再跑一次回归；actions 全 SHA pin | LB 仿真测试 4 份（`lb_simulation_adaptive/failover` + `lb_simuation_rr/cb`，含拼写不一致）+ 一键 e2e harness | 部署脚本也进 CI 检查（macOS shell job） | 12-check 单管道 + 运行时 4/5 平台冒烟 |

**诚实评价**：
- new-api：协议正确性层最强（golden + 真实 DB + 边界用例），端到端最弱（无浏览器、无上游测试床）。
- axonhub：后端仿真/行为测试是标杆（确定性仿真每种均衡组合）；E2E 存在但不作 CI 门禁
  （【推断】质量信心押在手工跑）。
- sub2api：工程密度同类罕见（迁移带回归、部署脚本进 CI、安全扫描例外有白名单治理），
  但真实上游回归完全人工（【推断】上游协议漂移只能发布后暴露）。
- **metapi-go 是四者中唯一浏览器 E2E + 真实上游链进 CI 的**——这是现有护城河，不是借鉴对象。

## 5. 轴5 借鉴点汇总

分级标准：P0=小改动、大收益、不与现有架构冲突；P1=收益明确、中等投入；P2=需求驱动或涉及定位取舍。
已剔除与现有能力等价的项（⌘K、主题 preset/对比度 gate、i18n gate、移动端深审、视觉回归、
PG 真库 CI、govulncheck、routing golden、会话 sticky、客户端导出——见 STATE 能力表）。

### P0（3 项）

| # | 做什么 | 为什么 | 建议验收 | 证据 |
|---|---|---|---|---|
| P0-1 | transform/ 协议转换 golden 快照套件（签入 request/response/stream testdata + update 环境变量） | metapi-go transform 层只有单元/roundtrip 测试；多协议互转是最易回归面，new-api 用 36 份 golden 锁死同层 | `transform/{openai,gemini}/testdata/golden` + golden test 进 CI；后续协议改动被快照机械拦截 | new-api `relaykit/relayconvert/testdata/golden/`（36 份，已验证）；metapi-go `transform/` 无 testdata（已验证） |
| P0-2 | 渠道/账号行级探测历史健康条（近 N 次竖条 + tooltip 成功率/延迟） | 数据层已有 `model_probe_results`，缺的只是呈现层；axonhub 证明该交互让运维 3 秒定位坏渠道 | 行级健康条组件 + tooltip，vitest + 视觉回归基线更新 | axonhub `features/channels/components/channel-health-cell.tsx`（已验证存在）；metapi-go `store/schema_ddl.go:1133` model_probe_results |
| P0-3 | cooldown/breaker 记录结构化原因 + 状态徽章可点击→根因弹窗（触发码/错误摘要/剩余时间/一键清除） | 现只存计数器+时间戳，无法回答「这个渠道为什么在冷却」；sub2api 根因弹窗是该模式的成熟形态，且 clear-cooldown 操作已存在只差呈现 | schema 加原因列（双方言）+ 徽章弹窗 + 复用既有清除操作 | sub2api `AccountStatusIndicator.vue` / `TempUnschedStatusModal.vue`（已验证存在）；metapi-go `store/schema_ddl.go:370-399` 无原因列（已验证） |

### P1（4 项）

| # | 做什么 | 为什么 | 建议验收 | 证据 |
|---|---|---|---|---|
| P1-1 | SSE 流 chunk 间隔超时（流卡死检测，每收到一块重置） | 现只有 first-byte 超时 + WS idle；流中途卡死会长期占连接【未验证：热路径或有其他机制，立项前先确认】 | `PROXY_STREAM_IDLE_TIMEOUT_SEC` env + 执行器测试；卡死流限时中断 | new-api `relay/helper/stream_scanner.go:88-250`（STREAMING_TIMEOUT 300s ticker） |
| P1-2 | 重试/禁用判定运营者可调（状态码区间运行时设置），并吸收「默认禁用仅 401、504/524 永不重试」语义 | 现硬编码（`proxy/retry_policy.go:146-165`）；new-api 区间可解释可调，策略语义完整 | 设置项 + 解析测试；默认行为与现状一致 | new-api `setting/operation_setting/status_code_ranges.go`（已验证，默认禁用区间 `{401,401}`） |
| P1-3 | 批量测试闭环：失败清单 + 一键禁用（人工确认） | model-tester 已有批量验证，缺「测完之后的动作」；new-api 把测试=运维闭环在界面内 | 批量测试后对失败渠道提供禁用动作 + 审计记录 | new-api `controller/channel-test.go`（performChannelTests 自动摘坏渠道） |
| P1-4 | 错误计数横幅→一键过滤视图 | 错误横幅已有 Retry；axonhub 把「N 个异常」进一步变成过滤入口，错误态与操作态合一 | channels/accounts 页横幅组件 + 过滤参数 URL 化 | axonhub `features/channels/components/channels-error-banner.tsx`（已验证存在） |

### P2（6 项，需求驱动）

| # | 做什么 | 为什么不建议现在做 | 证据 |
|---|---|---|---|
| P2-1 | 首启 Setup 向导 | 三竞品都有；但 metapi-go 的 env 配置是 TS 兼容冻结契约 + 零配置定位。若将来做，参考 sub2api 逐步连接测试 + `.installed` 锁 | sub2api `backend/internal/setup/setup.go:300-462` |
| P2-2 | driver.js 情境化 tour | 已有 guided onboarding + ⌘K；无用户困惑证据 | axonhub `features/onboarding/*-onboarding-flow.tsx`；sub2api `composables/useOnboardingTour.ts` |
| P2-3 | Thread/Trace 三层追踪 + trace 级粘性 | 会话级 sticky 已有；单实例定位，Redis sticky 为已知缓议项 | axonhub `internal/server/middleware/thread.go`（AH-Thread-Id，已验证）、`biz/system_default.go` TraceStickyPreferPreviousChannel |
| P2-4 | OTel/Prometheus 指标导出 | /health、/ready 与 observability 页已有；标准导出暂无明确消费方 | axonhub `internal/metrics/metrics.go`（默认关闭）；new-api `/api/perf-metrics` |
| P2-5 | 用量记账异步 worker pool（溢出同步兜底） | ProxyLogAsync 批量写已有；等出现高负载丢用量证据再评估 | sub2api `service/usage_record_worker_pool.go:16-55` |
| P2-6 | 平台 OAuth 刷新策略表化 | 重构项；仅当新增刷新失败语义不同的平台时值得做 | sub2api `service/refresh_policy.go`（已验证存在） |

### 明确不借鉴
- new-api 的多租户计费/钱包/支付、Electron 桌面版——定位边界（见 benchmark.md「明确不做」）。
- axonhub 的 GraphQL 管理面 / 多项目 RBAC——REST + 单实例定位，复杂度不匹配收益。
- sub2api 的拼车 SaaS 面（订阅/兑换/返佣/支付）——定位边界。
- E2E harness 与 LB 仿真测试——metapi-go 已有等价或更强（浏览器 E2E 进 CI + routing 行为测试套件）。

## 6. 未验证与边界声明
1. 三产品均为静态阅读（浅克隆/本地检出），未运行、未发真实上游请求；运行时行为结论是代码阅读推断。
2. axonhub quick-start「默认凭据」与 README 密码位数和代码矛盾——只作为文档漂移教训引用，未仲裁对错。
3. new-api 的 Prometheus 端点、setup 后自动登录；sub2api 的优雅停机流排空、仓库外 e2e 工程——均【未验证】。
4. 版本漂移：new-api / sub2api 为当日 master 浅克隆，axonhub 为本地 unstable HEAD（2026-08-10）；
   后续版本可能与本文证据不同。
5. 本文不含任何部署面凭据、主机名、内部域名；示例配置值一律省略。
