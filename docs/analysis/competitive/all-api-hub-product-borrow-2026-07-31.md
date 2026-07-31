# All-API-Hub 产品面全面借鉴 Synthesis — 2026-07-31

**Authority**: leader 综合（Explore 子代理对 `qixing-jk/all-api-hub` @ `a1ef3e9` 全量产品审计 + leader 源码交叉复核 metapi-go 现状）
**Scope**: 产品功能面（前端 UIUX 借鉴见 `uiux-newapi-borrow-2026-07-30.md`；后端 N1-N9 见 `product-parity-and-newapi-borrow-2026-07-30.md`；本文件**不重叠**，聚焦 all-api-hub 独有且 metapi-go 缺的产品能力）
**参考源**: all-api-hub（WXT 浏览器扩展，React + Tailwind + Radix + TanStack Table + ECharts + i18next 6 语种 + PostHog）— 路径 `D:/Code/TokenDance/reference/competitors/all-api-hub`（operator-only，不入产品文档绝对路径）
**定位边界**: all-api-hub 是**浏览器端资产管理器**，metapi-go 是**服务端聚合网关**。借鉴**产品能力**（能存在于服务端 admin UI 的），**不**借鉴扩展机制（content script / popup / sidepanel / 浏览器嗅探）。

---

## 1. 方法与边界

all-api-hub 的产品面比签到宽得多——它是「中转站账号全能管家」。Explore 审计了 23 个 feature 模块 + 60+ service 子目录，产出 15 条可借鉴产品想法。leader 用 metapi-go 源码逐条复核，剔除扩展专属项、剔除已对齐项、剔除与聚合网关定位冲突项，得到下文 A-K 能力簇 + 优先级表。

**产品形状边界（沿用 `competitive/sources.md`）**：
- 不把 metapi-go 做成浏览器扩展。
- 不做成多租户公网 SaaS 计费平台。
- 借鉴项必须在 metapi-go 的 operator/self-host 产品里可落地，且有 metapi 侧的改动路径。

**与既有借鉴文档的关系**：
| 既有文档 | 覆盖 | 本文件补充 |
|:---------|:-----|:----------|
| `product-parity-and-newapi-borrow-2026-07-30.md` | New API 借鉴 N1-N9（IP 白名单 / 价格目录 / 后缀解析 / 渠道测试 / 消费分布 / CSV / cache ratio / 多密钥 / 倍率管理） | all-api-hub 独有的产品能力（余额历史 / 需关注看板 / 任务通知 / 批量验证 / 导入预览 / 风险横幅 / 调度模式 / 快照分享 / 标签系统 / 设置搜索） |
| `uiux-newapi-borrow-2026-07-30.md` | 前端组件模式（ErrorBoundary / 状态三件套 / ConfirmDialog） | 本文件只标产品能力，UI 模式归该文档 |
| `competitive/matrix.md` L3/L10/L11/L12 | 跨站比价 / 测试台 / 导出适配器 / 热力图（已立项 learn） | 本文件细化 all-api-hub 侧证据 + 补 matrix 未覆盖项 |

---

## 2. metapi-go 现状产品面（交叉核验实证）

leader 源码核验，**已具备**：

| 能力 | 证据 |
|:-----|:-----|
| 14 平台适配器 + Checkin | `platform/registry.go:51-66` orderedPlatformNames 14 项全有 Checkin |
| 签到调度（cron + interval + lease + 分布式单实例） | `scheduler/checkin.go` + `handler/admin/checkin_routes.go`（trigger/logs/schedule）+ `runWithSchedulerLease` |
| 站点/账号/令牌 CRUD + 自动检测 + 初始化预设 | `handler/admin/sites.go:31` `/api/sites/detect` + `service.DetectSiteInitializationPreset` |
| 余额刷新 + 余额不足实时告警（G1 已发） | `service/balance/balance.go:RefreshBalance` → `service/alert/alert.go:ReportLowBalance`（24h 去抖） |
| proxy_logs + traces + events + site_announcements 表 | `store/migrate.go` events(1187) / site_announcements(1102) / checkin_logs(332) |
| 用量热力图 + 慢请求排行 | `handler/admin/stats.go:29-30` `/api/stats/usage-heatmap` + `/api/stats/slow-requests` |
| 站点**用量**趋势（非余额趋势） | `stats.go:458` siteTrend 查 `site_day_usage` 表（spend/calls，无 balance） |
| 跨站比价 + 下游可见价格目录（N2 已发） | `stats.go:33` modelPriceCompare + `/v1/pricing`（ProxyAuth 下游可见） |
| 模型市场 + 令牌候选 + 模型检测 + 探测 | `stats.go:36-40` marketplace / token-candidates / modelCheck / modelProbe |
| JSON 备份 + WebDAV | `web/pages/ImportExport.tsx` + `handler/admin/settings_backup.go` |
| 凭据导出 3 profile | `credential_export.go:83` openai-env / cherry / generic |
| OAuth 提供方（codex/claude/gemini/antigravity） | `handler/admin/oauth_routes.go:20` providers |
| 通知 5 渠道（bark/serverchan/webhook/telegram/smtp） | `service/notify/*.go` |
| 下游 key tags 字段 | `store/migrate.go:1041` downstream_api_keys.tags |
| 账号/站点 is_pinned + sort_order | `accounts.go` DDL |

**核验确认缺失**（all-api-hub 有、metapi-go 无）：

| 缺口 | 核验方法 | 结论 |
|:-----|:---------|:-----|
| **余额历史快照表** | `grep balance_history/daily_balance/BalanceHistory store/ service/ handler/` 全空；`site_day_usage` 只有 spend/calls 无 balance 列 | **真缺口**——只存当前余额，无历史趋势 |
| **需关注看板（severity 排序 + 深链）** | `grep attention/actionable/needsAttention Dashboard.tsx` 全空；events 表有 listEvents 但未聚合成 attention 面板 | **真缺口** |
| **per-task 通知开关** | `settings_notify.go` 只有全局 test；无 per-task（checkin/balance/oauth/probe）enable toggle | **真缺口** |
| feishu/dingtalk/wecom/ntfy 渠道 | `ls service/notify/` 仅 bark/serverchan/webhook/telegram/smtp | **真缺口**（4 渠道） |
| 模型成本分布 / 延迟直方图 / 延迟趋势 / topN-Other | `stats.go` 有 heatmap + slow-requests，无 cost-distribution pie / latency-histogram / latency-trend | **真缺口**（图表画廊） |
| 调度任务统一运行历史 UI | checkin_logs 有，但 balance/oauth/probe/announcement 无统一 per-job 结果表 + skip-reason + next-run | **真缺口** |
| 批量模型验证（per-row 探测 + 延迟 + 历史） | modelProbe 是异步「queued」，modelCheck 是 per-account；无批量 N 模型 per-row 结果 | **真缺口** |
| 备份导入预览（add/update/duplicate 计划） | ImportExport 有 parse + summary，无 commit 前计划预览 | **真缺口** |
| 调度模式：确定性 HH:mm OR 随机窗口 | checkin 有 cron OR intervalHours，无「随机窗口内抖动」模式 | **真缺口** |
| 账号/站点全局标签系统 | tags 字段只在 downstream_api_keys；accounts/sites 无 tags | **真缺口**（仅下游 key 有） |
| 风险横幅（severity 分级 in-app 公告） | site_announcements 是 per-site 上游公告；无产品级 severity 横幅 | **真缺口** |
| 可分享看板快照（PNG + 社交文案） | 全仓无 snapshot/PNG 生成 | **真缺口**（小众） |
| 设置跨特性搜索 + 锚点高亮 | SearchModal 有 Ctrl+K，无 deep-link anchor + highlight | **真缺口**（已在 uiux-newapi-borrow 提及，此处不重复立项） |

---

## 3. all-api-hub 能力簇（server-applicable 部分）

### A. 余额与用量分析（最高差异化）

all-api-hub 的 `BalanceHistory` + `UsageAnalytics` 是其最强产品面。**余额历史**每日快照 per-account，分 income/outcome（免费额度收入 vs 消费），支持 7/30/90/180/365d 快速区间 + 趋势图 + 账号分解饼/柱（负值自动 fallback 柱）+ JSON 导出。**用量分析**有模型-over-time 热力图、延迟直方图、延迟趋势、模型成本分布饼、topN-with-Other 桶、跨图联动（点选/缩放/图例互喂）。

> metapi-go 已有 usage 热力图 + 慢请求，但**无余额历史表**（核心缺口），且图表画廊远不及。聚合网关本就是 per-request latency/token/cost 的天然汇聚点，这一簇是最高 ROI 借鉴。

### B. 需关注看板（日常驾驶价值）

`OptionsOverview` 的 attention list：severity 排序的可操作项（不健康账号/渠道、缺配置、今日异常）每条深链到具体页面/区块；顶部 status cards（账号数/profile 数/今日用量/attention 计数，severity 染色可点）+ automation overview（签到/公告/模型同步/WebDAV 同步状态行 + last-run）。

> metapi-go Dashboard 有 stat cards + 站点分布 + 站点趋势，但**无 attention 聚合面板**。events 表已有数据底座，缺的是「聚合成 severity 排序深链列表」的视图层。

### C. 调度任务运行历史（统一模式）

all-api-hub 每个周期任务（签到 / 余额捕获 / 模型同步 / 公告轮询 / WebDAV 同步）共用一套 UI：per-item 结果表（status / skip-reason 本地化 / message）、run-now、过滤器、统计卡（total/success/failed/duration）、next-run + last-run、auto-retry 策略（interval + max attempts/day）。

> metapi-go 有 `scheduler/*.go` 11+ 周期任务（checkin/balance/oauth_refresh/sub2api_refresh/model_probe/site_announcement/usage_aggregation/daily_summary/log_cleanup/backup_webdav/file_retention），但前端只有 CheckinLog 一个统一日志页。**统一 run-history 模式**能复用 checkin_logs 表结构泛化到其他任务。

### D. 多渠道任务通知 + per-task 开关

all-api-hub: 浏览器通知 + Telegram/Feishu/DingTalk/WeCom/Ntfy/generic Webhook，per-task enable toggles（签到/WebDAV/模型同步/用量/余额/公告），per-channel config + test-send。

> metapi-go 有 5 渠道（bark/serverchan/webhook/telegram/smtp）+ 全局 test，**无 per-task 开关**，**缺 feishu/dingtalk/wecom/ntfy**。这是 LLM 网关用户高频请求特性，服务端原生实现比扩展更合适。

### E. 调度模式（确定性 OR 随机窗口）

all-api-hub 签到支持 deterministic HH:mm 或 random-within-window（windowStart/windowEnd）+ pre-trigger on UI open + 「今日已签」日历守卫 + 重试策略（独立 retry alarm）。

> metapi-go checkin 有 cron OR intervalHours，**无随机窗口抖动**。随机窗口对负载扩散 + 反指纹有用，不只签到——余额捕获/缓存预热等日任务都适用。

### F. 备份/恢复 + 导入预览

all-api-hub: 全量 JSON V2 + 部分备份（accounts-only / preferences-only）；导入时**校验 → 显示计划（新增/更新/重复）→ 确认后 commit**；加密 WebDAV 备份（AES）+ 选择性同步 + 自动同步间隔。

> metapi-go ImportExport 有 parse + summary + native backup 识别，**无 commit 前计划预览**。导入预览对「批量账号导入」场景（#534 已发）是天然补强。

### G. 批量验证 + 验证历史

all-api-hub `BatchVerifyModelsDialog` + verification suite（多协议 probe 注册表：OpenAI-compat/Anthropic/Google，per-probe 延迟/尝试/中断）+ verification-result history + CLI-support verification + web AI-API check（网页嗅探 Base URL/Key → 测试弹窗）。

> metapi-go 有 modelProbe（异步 queued）+ modelCheck（per-account）+ `/api/models/probe` + forced-channel ModelTester。**缺批量 N 模型 per-row 结果 + 延迟 + 历史保留**。网页嗅探是扩展专属，不入借鉴。

### H. 风险横幅 + 产品公告

all-api-hub `ProductAnnouncements`: 远程 JSON feed + schema 校验 + bundled fallback + semver 版本范围定向 + severity 分级（critical/warning/info）+ Overview 顶部风险横幅（「N more」计数）+ dismiss with revision tracking + list popover。

> metapi-go 有 site_announcements（per-site 上游公告），**无产品级 severity 横幅**。对聚合网关，可用此模式推「上游协议变更 / 已知坏站 / 维护通知」。

### I. 标签系统（accounts/sites）

all-api-hub 全局 tag store（CRUD，删除级联到 accounts），多选 tag 过滤，彩色 tag。

> metapi-go tags 字段只在 downstream_api_keys，**accounts/sites 无 tags**。给 accounts/sites 加全局标签 + 多选过滤，分类管理多站点时刚需。

### J. 可分享看板快照

all-api-hub `ShareSnapshots`: allowlisted 概览/账号快照 payload → 渲染 1200x1200 PNG（seeded mesh-gradient 背景 + overlay）→ 剪贴板/下载 + 自动生成本地化社交文案。`MeshGradientLab` 是 dev 调色板浏览器。

> metapi-go 无此能力。对服务端 admin，一键「分享额度/用量状态 PNG 到社区」是轻量营销闭环，价值中等、工程量中。

### K. 渠道管理表（部分适用）

all-api-hub `ManagedSiteChannels`: TanStack 表（排序 / faceted filter / 列可见性 / 分页 / 行选择 / 批量）+ 渠道 CRUD dialog（name/key/models，**name/key/models 重复警告**）+ 渠道迁移 dialog（跨上游迁移，provider-key 复用安全检查，model/status-code 映射保留）+ 模型重定向映射生成 + 同步后 auto-apply。

> metapi-go 渠道模型不同（route_channels 绑定上游账号，非独立 managed-channel-sync 表面），**不直接照搬**。但「CRUD 表 + faceted filter + 重复警告 + 批量」的**表 UX 模式**对 TokenRoutes/DownstreamKeys 列表页有借鉴价值（归 uiux 文档，此处不立项）。「模型重定向映射生成 + 同步后 auto-apply」是**真产品能力缺口**——metapi-go 有站点禁用模型，但无「上游模型名 → 标准名」的自动映射生成。

---

## 4. 借鉴优先级表（leader 复核后）

| 优先级 | # | 借鉴项 | all-api-hub 证据 | metapi-go 落点 | 量 | 价值 |
|:------:|:--|:-------|:----------------|:--------------|:--|:-----|
| **P0** | **A1** | **余额历史快照表 + 趋势图** ✅ 已发 | `BalanceHistory` 每日 per-account 快照 + income/outcome 分离 + 7/30/90/180/365d | 新 `balance_history` 表（AdditiveStep）+ `scheduler/balance.go` 刷新时写快照 + Dashboard 余额趋势卡 + Accounts 余额历史抽屉 | **M** | 高（聚合网关独有数据底座，补上即超越原版 TS 也未做的承诺） |
| **P0** | **B1** | **需关注看板（severity 排序 + 深链）** ✅ 已发 | `OptionsOverview` attention list + status cards + automation overview | Dashboard 加 attention 面板，聚合 events 表 + unhealthy accounts/channels + 缺配置，severity 排序深链 | **S-M** | 高（日常驾驶，复用现有 events 底座） |
| **P0** | **D1** | **per-task 通知开关 + feishu/dingtalk/wecom/ntfy 渠道** ✅ 已发 | notifications per-task toggles + 6 外部渠道 | `settings_notify.go` 扩 per-task toggle；新 `service/notify/feishu.go`/`dingtalk.go`/`wecom.go`/`ntfy.go`；NotificationSettings UI 扩 | **M** | 高（LLM 网关高频请求特性） |
| **P1** | **C1** | **调度任务统一运行历史 UI** ✅ 已发 | 签到/余额/模型同步/公告共用 per-item 结果表 + 统计卡 + next-run | 新 `/api/scheduler/status`（聚合 checkin_logs/accounts.last_*/probe 内存/events）+ Dashboard「调度任务状态」面板 | **M** | 中高（11+ 周期任务可观察性统一） |
| **P1** | **A2** | **模型成本分布 + 延迟直方图/趋势图表画廊** ✅ 已发 | `UsageAnalytics` 8 图 + 跨图联动 + topN-Other | `stats.go` 加 cost-distribution / latency-histogram / latency-trend 端点；Dashboard/Models 加图 | **M** | 中高（数据已齐，只差视图） |
| **P1** | **G1** | **批量模型验证 + 验证历史** ✅ 已发 | `BatchVerifyModelsDialog` + verification-result history | 复用 modelProbe 逻辑做批量 N 模型 per-row + 延迟 + history 表；Models 页批量验证 dialog | **M** | 中（运维 UX） |
| **P1** | **E1** | **调度模式：随机窗口抖动** ✅ 已发 | deterministic OR random-within-window + pre-trigger + 日历守卫 | `checkin_schedule.go` 加 windowStart/windowEnd 模式；泛化到 balance/cache-warm | **S** | 中（负载扩散 + 反指纹） |
| **P1** | **F1** | **备份导入预览（计划确认）** ✅ 已发 | ImportExport 校验 → 计划 → commit | 新 `/api/settings/backup/import/preview`（rows/toInsert/duplicates/skipped，不写行）+ ImportExport confirm 前展示计划；顺带修复前端 `{data}` 契约 bug | **S** | 中（#534 批量导入天然补强） |
| **P2** | **I1** | **accounts/sites 全局标签系统** ✅ 已发 | 全局 tag store + 多选过滤 + 彩色 | accounts/sites 加 tags 列（AdditiveStep）+ admin tag CRUD + Accounts/Sites 多选过滤 | **S-M** | 中（多站点分类管理） |
| **P2** | **H1** | **产品级风险横幅（severity 分级）** | `ProductAnnouncements` semver 定向 + severity + dismiss-revision | 新 `product_announcements` 表 + 远程 feed（可选）或 admin 手发；Dashboard 顶部横幅 | **S-M** | 中（替代邮件群发） |
| **P2** | **K1** | **模型重定向映射生成 + auto-apply** | `modelRedirect` 资源映射 writer + 同步后 auto-apply | 站点模型同步后生成「标准名 → 上游实际名」映射 + 自动写入 disabled_models/路由 | **M** | 中（路由修复自动化） |
| **P3** | **J1** | **可分享看板快照 PNG + 文案** | `ShareSnapshots` 1200x1200 + mesh-gradient + 本地化文案 | 服务端 canvas/headless 渲染 PNG + 文案；Dashboard 分享按钮 | **M** | 低-中（轻量营销） |
| **P3** | **A3** | **income vs outcome 余额分析** | BalanceHistory 分免费额度收入 vs 消费 | 依赖 A1 表，加 income/outcome 分离视图 | **S** | 低-中（依赖 A1） |

### 剔除（不立项）

| all-api-hub 能力 | 剔除原因 |
|:----------------|:--------|
| 网页嗅探（content script 选 Base URL/Key 弹窗） | 扩展专属机制，服务端无对应 |
| Popup / Sidepanel / BookmarkManagement / SiteBookmarks | 扩展入口形状，服务端 admin 不需要 |
| Permissions（浏览器可选权限 grant/revoke） | 扩展专属 |
| MeshGradientLab（dev 调色板浏览器） | 纯 dev 工具，非产品能力 |
| Redemption assist（页面嗅探兑换码自动兑换） | 扩展专属嗅探；服务端可做但 niche、定位冲突 |
| ProductAnalytics（PostHog 遥测） | metapi-go 隐私边界——不引入客户端遥测（STATE 已声明） |
| LDOH site lookup（社区站点列表缓存） | metapi-go 已有 `/monitor-proxy/ldoh`（`handler/admin/monitor.go:32`），已对齐 |
| ManagedSiteChannels 表 UX（排序/faceted/批量） | 归 `uiux-newapi-borrow` 前端模式，不立项产品能力 |
| Channel migration dialog（跨上游迁移） | metapi-go 渠道模型不同（route_channels 绑账号），形状不匹配 |

### 已对齐（all-api-hub 有，metapi-go 也有，无借鉴价值）

| 能力 | metapi-go 证据 |
|:-----|:-------------|
| 站点自动检测 + 初始化预设 | `service.DetectSite` + `DetectSiteInitializationPreset` |
| 多渠道路由 + 权重 + 冷却 | `routing/` 6 策略 + Fibonacci cooldown |
| 跨站比价 | `modelPriceCompare` + `/v1/pricing`（N2 已发） |
| 凭据导出适配器 | `credential_export.go` 3 profile（openai-env/cherry/generic） |
| 凭据库（Base URL + Key CRUD） | downstream_api_keys 即此角色 |
| 账号 is_pinned + sort_order | accounts DDL |
| OAuth 账号导入（codex/claude/gemini） | `oauth_routes.go` |
| 用量热力图 + 慢请求 | `stats.go:29-30` |
| 站点公告轮询 | `site_announcements.go` + scheduler |

---

## 5. 与既有 M-COMPETE learn backlog 的关系

`competitive/matrix.md` 已有 L1-L12 learn backlog。本文件**不重复立项**，而是细化 all-api-hub 侧证据 + 补 matrix 未覆盖项：

| matrix learn 项 | 本文件对应 | 关系 |
|:---------------|:----------|:-----|
| L3 跨站比价 | 已对齐（N2 已发） | matrix 仍 backlog-only，本文件标 present |
| L10 in-admin 测试台 | G1 批量验证 | 细化：matrix 是 forced-channel 测试，本文件加批量验证 + 历史 |
| L11 导出适配器 | 已对齐（credential_export 3 profile） | matrix 仍 backlog-only |
| L12 热力图 + 慢请求 | 已对齐（stats.go 已发） | matrix 仍 backlog-only |
| —（matrix 未覆盖） | A1 余额历史 / B1 需关注 / D1 per-task 通知 / C1 统一运行历史 / E1 随机窗口 / F1 导入预览 / I1 标签 / H1 风险横幅 / K1 模型映射 / J1 快照 PNG | **本文件新增 10 项**，建议后续纳入 matrix 或单独立项 |

---

## 6. 推荐执行序列（决策输入，非执行令）

> **硬门禁**：以下均为产品功能（非工程纪律），按 SDD + neat-freak + leader 规则，**需用户拍板 / 开 Issue 后再动**，不得静默自动实现。本文件是决策输入。

若用户确认推进，建议序列（按依赖 + ROI）：

```
Wave A（数据底座，最高 ROI，互相独立可并行）
  A1 余额历史快照表 + 趋势图        ← 新表 + scheduler 写 + Dashboard 卡
  B1 需关注看板                       ← 复用 events 底座，纯前端聚合
  D1 per-task 通知 + 4 新渠道         ← 通知面补全

Wave B（可观察性 + 验证，依赖 A1/A2 数据底座）
  C1 调度任务统一运行历史             ← 泛化 checkin_logs ✅ 已发
  A2 模型成本分布 + 延迟图表画廊      ← stats 端点 + 前端图 ✅ 已发
  G1 批量模型验证 + 验证历史           ← 复用 modelProbe ✅ 已发

Wave C（调度 + 导入 + 分类，独立小项）
  E1 随机窗口调度模式                 ← checkin_schedule 扩展
  F1 备份导入预览                     ← ImportExport 扩
  I1 accounts/sites 标签系统          ← AdditiveStep + 前端过滤

Wave D（产品化体验，低优先）
  H1 产品级风险横幅
  K1 模型重定向映射 auto-apply
  J1 可分享快照 PNG
  A3 income vs outcome 分析（依赖 A1）
```

**量级**：A1/D1 为 M（触及 schema + scheduler）；B1 为 S-M（纯前端聚合 + 少量后端聚合端点）；其余 S-M。

---

## 7. 来源

- all-api-hub：`D:/Code/TokenDance/reference/competitors/all-api-hub`（HEAD `a1ef3e9`，v3.52.0）；公开 [GitHub](https://github.com/qixing-jk/all-api-hub) + [文档站](https://all-api-hub.qixing1217.top)
- metapi-go 现状实证：`platform/registry.go:51-66` · `scheduler/*.go`（11+ 周期任务）· `handler/admin/stats.go:29-30,33,36-40,458` · `handler/admin/checkin_routes.go` · `handler/admin/settings_notify.go` · `service/notify/*.go` · `service/balance/balance.go:RefreshBalance` · `service/alert/alert.go:ReportLowBalance` · `store/migrate.go`（events 1187 / site_announcements 1102 / checkin_logs 332 / downstream_api_keys 1041 / site_day_usage 891）· `web/pages/Dashboard.tsx` · `web/pages/ImportExport.tsx` · `handler/admin/credential_export.go:83` · `handler/admin/monitor.go:32`
- 既有借鉴文档：`product-parity-and-newapi-borrow-2026-07-30.md`（N1-N9）· `uiux-newapi-borrow-2026-07-30.md`（前端模式）· `competitive/matrix.md`（L1-L12 learn backlog）

**本轮不自动实现**：以上 13 项均为产品功能，按硬门禁需先开 Issue / 用户拍板再动。本文件是决策输入，不是执行令。
