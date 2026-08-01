# log.md — MetAPI Go progress log

> **进度日志**（append-only）。不是现状 SSOT。  
> 现状 → [`STATE.md`](STATE.md) · 开放项 → [`progress/MASTER.md`](progress/MASTER.md)

## [2026-08-01] New API deferred 项评估 + N9a 交付

- **评估文档** `n8-n9-deferred-assessment-2026-08-01.md`:
  - **N8 关闭**: metapi-go 架构下「渠道」= route 的 channel 行、「密钥」= account，多 key 轮询已由 `round_robin`/`weighted` 策略 + 每通道独立冷却（ApplyRoundRobinCooldown/Fibonacci + cooldown_until）+ model-probe 健康记录 + OAuth route unit 多成员轮询完整覆盖。实现「渠道内多 key」= 平行凭证模型（维护成本 + 门禁冲突）——诚实评估后**不立项**。
  - **N9 拆两期**: N9a（S 只读）直接做；N9b（M 写入面，改 unit_cost/weight 批量入口）触及计费语义需设计 + 拍板。
- **N9a 倍率总览**（`c2d7cb8`）: `GET /api/models/rates` 聚合 5 个倍率面（账号 unit_cost+通道足迹、通道 weight、站点 global_weight、下游 key_weight、模型 30 天观测成本）+ summary（有单价账号数/通道启用数）；Settings「倍率与权重总览」区只读表格。纯读零风险。
- **验证**: `go vet` + `go test` 全绿；`npm test`（553 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `c2d7cb8` / MASTER / CHANGELOG / 评估文档。

## [2026-08-01] all-api-hub borrow 收官（K1a 模型重定向映射）

- **K1a 模型重定向映射**（`7597a07`）: 先写设计文档 `k1-model-redirect-design-2026-08-01.md`（问题/规则/边界/验收/K1b 风险面）再实现——
  - `model_name_redirects` 表（Table 33: account_id/canonical/actual/source(sync|manual)/last_seen_at，UNIQUE(account_id, canonical) + (account_id, actual) 索引）。
  - `service.CanonicalFromActual`：精确（大小写归一）→ 日期后缀 `-YYYYMMDD`/`-YYYYMMDD-vN` → 版本后缀 `-latest` 等；候选 = global allowed models + token_routes 精确 pattern（无通配符）。
  - **稳定性优先**：同一 canonical 首个命中的 actual 固定，后续不同 actual 只 touch last_seen 不覆盖；manual 永不被同步覆盖；幂等（ON CONFLICT 只刷 last_seen）。
  - `refreshAccountModels` 同步后 best-effort 生成（不阻断刷新，结果进 redirects.generated）。
  - 端点: GET（join 账号/站点名 + 过滤）/ PUT（转 manual / 修 actual）/ DELETE / POST generate（单账号或全量，ORDER BY id ASC 保证确定性）/ POST apply {dryRun}——dry-run 列「canonical 在 disabled_models 且 actual 可用」的候选不删除；确认后删 + events(type=model_redirect_applied)。
  - 前端: Settings「模型重定向映射」区（表格 + 生成映射 + 预览可修复项 + 确认修复 + 转手动/删除）。
  - 测试: 生成规则矩阵（8 用例：精确不映射/日期/日期+revision/版本/大小写折叠合并/稳定性）+ e2e（generate→list→幂等→转手动不被覆盖→dry-run 不删→apply 删+event→delete）+ 400/404；前端 section 2 用例。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（551 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `7597a07` / MASTER / CHANGELOG / borrow doc K1 行（K1a ✅ K1b deferred）。**borrow 清单 13/13 立项全发；K1b/N8/N9 为 M 级 deferred，需拍板。**

## [2026-08-01] all-api-hub borrow Wave D 收官（J1 快照 PNG）

- **J1 可分享看板快照 PNG**（`d9f915a`）:
  - Dashboard header「导出快照」按钮（lazy `SnapshotExportButton`）：并行拉 `getDashboardSnapshot` + `getSiteDistribution` → 原生 canvas 绘制 1200x630（@2x 2400x1260）摘要卡——品牌蓝顶部条 + 标题/生成时间 + 6 指标卡（总余额/今日消耗/24h 请求/成功率（<90% 红色强调）/Token/活跃账号）+ 站点消耗 Top5 条形 + 底部来源注；`toBlob` → object URL 下载 `metapi-snapshot-YYYYMMDD.png`。
  - 关键决策: **canvas 无 CSS 变量** → 固定品牌 hex 调色板（primary #1a73e8 取自 tokens.css）；零新依赖（不引 html2canvas）；toBlob 缺失/API 失败 → toast 诚实报错。
  - 测试: 组件 2 用例（stub canvas：导出流 + toBlob 缺失回退）；dashboard 测试不受影响（lazy 挂载）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（549 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `d9f915a` / MASTER / CHANGELOG / borrow doc J1 行 ✅。**borrow 清单 12/13 全发，余 K1（M 级）待拍板。**

## [2026-08-01] all-api-hub borrow Wave D 交付（H1 风险横幅）

- **H1 产品级风险横幅**（`5da5656`）:
  - Schema: `product_announcements`（Table 31: title/message/severity/link/enabled + 时间戳）+ `announcement_dismissals`（Table 32: announcement_id PK + ON DELETE CASCADE）。
  - 端点: `GET /api/announcements`（管理视图带 dismissed 状态）/ `GET /api/announcements/active`（enabled + 未关闭，critical→warning→info 排序）；`POST`（校验 title/message/severity）/ `PUT`（**内容变更重置 dismissal** = dismiss-revision 语义，事务内删除关闭记录；severity/enabled-only 变更不重置）/ `DELETE` / `POST {id}/dismiss`（ON CONFLICT upsert 双方言）。
  - 前端: `AnnouncementBanner`（Dashboard 顶部，severity 配色 critical 红/warning 黄/info 蓝 + dismiss × + 详情外链，API 失败静默降级）；Settings「产品公告」区（AnnouncementsSection：列表 + 新建/编辑表单 + 删除 + 已关闭徽标）。
  - 测试: 后端 e2e（CRUD + active 排序 + dismiss + 内容编辑重置 + 400/404）；banner 组件 3 用例（渲染/dismiss 移除/空不渲染）；section 组件 2 用例（创建表单提交/删除）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（547 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `5da5656` / MASTER / CHANGELOG / borrow doc H1 行 ✅。

## [2026-08-01] all-api-hub borrow Wave D 交付（I1 标签系统）

- **I1 accounts/sites 全局标签系统**（`ba74242`）:
  - Schema: `accounts.tags` / `sites.tags` TEXT（JSON 数组文本，NULL=无）；AdditiveStep `sc2_011_account_site_tags`（EnsureColumn 双方言）；SiteSelectColumns + 两个 struct 加字段；列数断言 +2。
  - 端点: `GET /api/tags`（union 聚合 + account/site 计数 + total 降序稳定排序）；`PUT /api/accounts/{id}/tags` / `PUT /api/sites/{id}/tags`（body 校验 400 / 不存在 404 / 去重 + trim / 空列表=清空）。
  - 前端: `helpers/tags.ts`（parseTags 容错 JSON 文本/数组/逗号、tagColor 稳定哈希到 chart 调色板、collectTags 频次排序 union）；共享 `TagEditorDialog`（chips + 删除 × + 自由输入 + 常用标签快捷添加 + Enter 保存）；Accounts/Sites 双页：行内彩色 chips（点击切换过滤）、列表上方过滤 chips 行（清除过滤）、行操作「标签」按钮 → dialog → PUT → 重载。
  - 测试: 后端 e2e（聚合计数/去重/清空后索引消失/400/404/坏 id）；helpers 单测；dialog 组件 2 用例（快捷添加+保存、× 删除）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（542 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `ba74242` / MASTER / CHANGELOG / borrow doc I1 行 ✅。

## [2026-08-01] all-api-hub borrow Wave C 交付（G1 批量验证）

- **G1 批量模型验证 + 验证历史**（`0047c72`）: 
  - 新表 `model_verify_history`（Table 30，batch_id/model_name/channel/account/site/status/latency/http_status/error_text + 2 索引）。
  - `scheduler.ProbeBatch(ctx, targets, timeoutMs)`：一次性操作者验证——复用注入的 `ChannelHealthProbe` 执行器 + 有 recorder 时经 `ApplyProbeOutcome` 同步路由健康；**不碰账号租约**（区别于后台 pass）；无 executor 时诚实 skipped。
  - `POST /api/models/verify-batch`：body `{models?, accountId?, limit?}`（models 空 = 全部启用 route_channels，IN 过滤 + account 过滤，limit 默认 50 cap 200）；scheduler 未启动 → 503；空匹配 → probed 0 + note。逐行写 history（best-effort，不阻断验证）。
  - `GET /api/models/verify-history?limit=&model=`：join sites 出 siteName，newest-first。
  - 前端：Models 页 page-actions「批量验证」按钮（data-testid=open-model-verify）→ `ModelVerifyDialog`（CenteredModal）：验证 tab（说明 + 开始验证 + summary badges + per-row 表格）+ 验证历史 tab；api.ts `verifyModelsBatch`/`getModelVerifyHistory` + 类型。
  - 测试：后端 e2e（503 / 双模型行结果 + summary / history 持久化 + siteName / account 过滤 / 空匹配 / 行数 4）；前端 dialog 2 用例（验证运行 + 历史 tab 切换）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（534 测试）全绿；SPA rebuild。
- SSOT 同步: STATE tip `0047c72` / MASTER / CHANGELOG / borrow doc G1 行 + Wave B 全绿。

## [2026-08-01] all-api-hub borrow Wave C 交付（A2 图表画廊）

- **A2 模型成本分布 + 延迟图表画廊**（`439b6dd`）: 数据全在 proxy_logs，只差视图——
  - `GET /api/stats/model-cost-distribution?days=&topN=`: model_actual>model_requested>unknown 归组，topN 按成本降序 + 余量折叠「其他模型」桶，附 totals；无 LIMIT 全量分组后 Go 端折叠（模型数有限）。
  - `GET /api/stats/latency-histogram?days=&bucketMs=`: `(latency_ms / ?) * ?` 整数除法双方言同语义，桶含 count + percent；空桶省略。
  - `GET /api/stats/latency-trend?days=`: 每日 avg/max/first-byte + successRate；p95 用 `ORDER BY latency_ms DESC LIMIT 10000` + `COUNT(*) OVER ()` 拿真实行数，`floor(0.05*n)` 落采样内（n<200k 安全），超限天进 truncatedDays 诚实标记；新增 `dayBucketSQLExpr` 双方言日期桶。
  - 前端: `CostDistributionChart`（环形图 + 总成本头）/ `LatencyHistogramChart`（柱状）/ `LatencyTrendChart`（avg+P95 双线 + 截断提示）三卡挂 Dashboard（lazy）；api.ts 类型 + 3 方法；4 个 dashboard 测试补 mock。
  - 测试: stats_gallery_test.go 3 组 SQLite 用例（topN-Other 折叠、桶边界、per-day profile + p95 值）+ 1 组 PG 方言奇偶（PG_TEST_DSN 缺省跳过）；3 个前端图表组件测试（spec 断言 + header 文本）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `npm test`（162 文件/532 测试）全绿；SPA rebuild（注意：vitest 必须从 `web/` 下以 `npm test` 跑，从仓库根 `npx vitest run` 会误收集 `web/e2e/*.spec.ts`）。
- SSOT 同步: STATE tip `439b6dd` / MASTER / CHANGELOG / borrow doc A2 行 + Wave B 余项标注。

## [2026-07-31] all-api-hub borrow Wave B 交付（E1/F1/C1）

- **E1 随机窗口调度**（`ea8991f`）: checkin 新 `window` 模式——`RandomCronInWindow(start,end)` 解析 HH:mm 边界、窗口内均匀随机生成 "m h * * *" 每日 cron（每次启动/设置变更重 roll = 负载扩散 + 反指纹）；scheduler window 分支 + 校验（start<=end、坏边界降级 cron）；config `CHECKIN_WINDOW_START/END`（默认 00:00-23:59）；store/settings 水合；`PUT /api/checkin/schedule` 接受 windowStart/windowEnd。测试：5 边界组 × 50 次滚动全落窗口 + cron 合法、重 roll 变化性、7 组坏边界、parseHHMM 容错。
- **F1 备份导入预览**（`d197c76`）: 新 `POST /api/settings/backup/import/preview`——decodeBackupImportBody 同时接受 `{tables}` 与 `{data:{tables}}`（**顺带修复前端 {data} 包装契约 bug**：手动 JSON 粘贴导入此前恒 400）；per-table plan rows/toInsert/duplicates（PK 已存在）/skippedRows（runtime-local settings），不写任何行；ImportExport confirm 弹窗展示「导入计划预览」。测试：preview 报告 + 不写入验证 + {data} 回归。
- **C1 调度任务统一运行历史**（`37d6a70`）: 新 `GET /api/scheduler/status` 聚合 7 个周期任务的 last-run/24h 活动/成功数（checkin_logs、accounts.last_balance_refresh、model-probe 内存 summary、site_announcements、events type 计数）；Dashboard「调度任务状态」面板（severity dot + 相对时间）。零 scheduler 代码改动，纯聚合。测试：checkin 近/远日志 → lastStatus/runs24h/success24h。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`vitest` 159 文件/526 测试全绿；SPA rebuild。
- SSOT 同步: STATE tip `37d6a70` / MASTER / CHANGELOG / borrow doc P1 三行标 ✅。

## [2026-07-31] all-api-hub borrow Wave A 交付（A1/B1/D1）

- **A1 余额历史快照表 + 趋势图**（`34d3371`）: 新 `balance_history` 表（Table 29，per (local_day, account_id) UPSERT 同日覆盖）+ `RefreshBalance` 成功路径 `recordBalanceSnapshot`（dialect-unified ON CONFLICT DO UPDATE，best-effort 不阻断刷新）+ `/api/stats/balance-history?accountId=&days=`（per-account per-day 系列）+ Dashboard `BalanceHistoryChart`（跨账号聚合 per-day 总余额趋势）。测试：RefreshBalance 两次→1 行 UPSERT；端点两天快照 + ASC 排序。
- **B1 需关注看板**（`9d0c5ab`）: `/api/stats/attention?limit=` 聚合 expired accounts（critical）→ low-balance <1.0（warning）→ disabled sites（warning）→ 近 24h warning/error events，每项 category/label/target（深链）/createdAt。Dashboard 顶部 `AttentionPanel`（severity dot + navigate 深链 + EmptyState）。只查 plain columns（runtime health 已由 alert.go 写 events，避免 json_extract 跨方言）。4 个 dashboard 测试补 mock；site-speed-button 测试补 MemoryRouter。
- **D1 per-task 通知 + 4 新渠道**（`6e0312b`）: 新 `service/notify/{feishu,dingtalk,wecom,ntfy}.go`——飞书 HMAC-SHA256 签名（key=ts+"\n"+secret, msg=""→base64）、钉钉 HMAC-SHA256（key=secret, msg=ts+"\n"+secret→base64）+ URL query 追加、企业微信 text msgtype、ntfy POST + Title/Priority/Tags/Bearer；全部走共享 `doNotifyRequest`。`SendNotificationOptions.TaskTag` + `cfg.NotifyTaskToggles` 静音门（空 TaskTag=不门控）；alert 3 调用带 tag（token_expired/low_balance/proxy_all_failed）；config/store/settings/handler 全链持久化；NotificationSettings 扩展渠道卡 + 按告警类型静音行。测试：签名确定性 + 4 渠道 send + 静音门 3 场景（10 个新测试）。
- **验证**: `go vet ./...` + `go test ./...` 全绿；`npm run typecheck` + `vitest` 159 文件/526 测试全绿；SPA rebuild 三次。
- SSOT 同步: STATE tip `6e0312b` / MASTER 未发布清单 / log 本条目；borrow doc P0 三行标 ✅。

## [2026-07-31] all-api-hub 全面产品面借鉴 synthesis

- **决策输入文档**（未提交，待 commit）: 新 `docs/analysis/competitive/all-api-hub-product-borrow-2026-07-31.md`（241 行）。Explore 子代理全量审计 all-api-hub @ `a1ef3e9`（v3.52.0）23 feature + 60+ service，leader 用 metapi-go 源码逐条复核，得 13 项可借鉴产品能力（A1-J1）。
- **最高 ROI 缺口**（P0）: A1 余额历史快照表 + 趋势图（metapi-go 只存当前余额，无历史；`site_day_usage` 只有 spend/calls 无 balance 列——核验确认真缺口）/ B1 需关注看板（events 表有底座，缺 severity 排序深链聚合面板）/ D1 per-task 通知开关 + feishu/dingtalk/wecom/ntfy 4 渠道（现有 5 渠道无 per-task toggle）。
- **与既有借鉴文档不重叠**: New API N1-N9 归 `product-parity-and-newapi-borrow`；前端 UI 模式归 `uiux-newapi-borrow`；本文件聚焦 all-api-hub 独有且 metapi-go 缺的产品能力。
- **硬门禁遵守**: 本文件是决策输入非执行令——13 项均为产品功能，需用户拍板/开 Issue 再动，不静默自动实现。已对齐项（签到/检测/比价/导出/OAuth/热力图/公告）不重复立项；扩展专属项（网页嗅探/popup/permissions/PostHog 遥测）剔除。
- SSOT 同步: STATE/MASTER/competitive/README 指针更新；tip `97c54b1`。

## [2026-07-31] N7 admin 可配 prompt-cache 倍率 + N8/N9 deferred

- **N7**（`10384ee`）: `routing.DefaultCacheRatio`/`ClaudeCacheRatio`（及 creation 对应项）改成 runtime-overridable（`atomic.Pointer` + `SetCacheRatioDefaults`，非正/NaN/Inf 重置回代码默认）。端到端：config 字段 → `store.ApplyRuntimeSettings`（cache_ratio_default/claude）→ `app.ApplyCacheRatioOverrides` 启动 apply → admin settings getRuntime 暴露/updateRuntime 持久化+即时 apply。测试：override 生效、坏值重置、显式 per-row 倍率仍优先（ResolveCacheRatio）。
- **N8/N9 deferred**（M 级）: N8 单渠道多密钥轮询需改 `routing/selector.go` 令牌解析算法（从列表 round-robin 选 key）+ candidate 加载——触及负载均衡选路核心，须专项会话+完整算法测试，不仓促尾段。N9 倍率管理后台需新倍率表+CRUD+UI。两项仍记于 `product-parity-and-newapi-borrow-2026-07-30.md` §4（P3）。
- CHANGELOG `N7` + `N8/N9 deferred` 条目；STATE tip `10384ee`。

## [2026-07-31] 产品化批次 N2-N6 + G1（New API borrow）

- **N2 下游 key 可见价格端点**（`d22b808`）: 新 `/v1/pricing`（+ `/v1/models/price-compare` 别名）mount 在 /v1 ProxyAuth 组——持 managed key 的下游消费者可查跨站模型有效价格（复用 admin.modelPriceCompare，无独立 catalog 漂移）。非世界公开（匿名不泄露成本）。router 测试 401-without-key。
- **N3 推理后缀解析**（`9c056a4`）: `ParseReasoningSuffix` 剥离 `-thinking`/`-high`/`-medium`/`-low` 使路由命中 base model；OpenAI 表面注入 `reasoning_effort`（客户端已设不覆盖）+ 重序列化 RawBody。非 OpenAI 方言仅 strip 用于路由（跨方言注入 deferred）。
- **N4 Sites 行内测速**（`6ed798d`）: Sites 列表行加内联测速按钮（client-side `fetch(site.url/v1/models)` no-cors，同 Dashboard），免开编辑器即测。
- **N5 下游 key 消费分布看板**（`05e10a7`）: 新 `ConsumptionDistribution` 组件——按 usedCost/usedRequests 切换的 top-10 跨 key 分布（bar+% 占比），聚合自可见 key，DownstreamKeys 顶部可折叠面板。纯前端。
- **N6 列表 CSV 导出**（`18e9066`）: CheckinLog/ProgramLogs/DownstreamKeys 加导出 CSV（复用 csvExport helper；DownstreamKeys 仅导出 keyMasked+计费字段，不导出原 key）。
- **G1 余额不足实时告警**（`09a619e`）: `alert.ReportLowBalance` 在 balance 刷新观察到 <1.0 时触发（TS parity 阈值），per account per 24h events 去重防刷。hook 在 `balance.RefreshBalance` 成功路径——定时+手动刷新都实时落地。TS 仅每日摘要 count（承诺未落地），metapi-go 做得更好。
- CHANGELOG `N2/N3/N4/N5/N6/G1` 条目；STATE tip `9c056a4`。

## [2026-07-31] N1 下游 key IP 白名单/黑名单 + §5.11 包边界 leaf 抽取

- **N1 安全缺口（New API borrow）**（backend `9e9cad1` + UI `d4633f1`）: 聚合网关公开暴露 managed 下游 key 却无 per-key IP 限制。新增 `downstream_api_keys.ip_allowlist`/`ip_blocklist` TEXT NULL（base DDL + AdditiveStep `sc2_010`）；`auth.CheckDownstreamKeyIP` 在 `ProxyAuth` 边缘 managed-key 鉴权后拦截——blocklist 优先、allowlist 非空须命中、皆空=不限；复用 `auth/admin.go` `parseAllowlist`/`isIPAllowed`（IPv4-mapped-IPv6 + `::1` 归一、非法条目静默跳过）。`AuthorizeDownstreamToken` 签名不变（无 test ripple）。admin CRUD（create + 部分更新，空串清为 NULL）+ DownstreamKey 编辑器 textarea。测试：`auth/downstream_ip_test`（split/check/block-wins/CIDR/IPv4-mapped/loopback）、`handler/admin TestDownstreamKeysIPAllowBlockListCRUD`、`DownstreamKeys.test` IP 字段渲染+保存。闭合 `product-parity-and-newapi-borrow-2026-07-30.md` §N1（P0）。
- **§5.11 包边界例外移除**（`0879652`）: `scheduler/lease.go` 曾 import `handler/shared`（唯一包边界例外）。抽 `app/observability` **叶子子包**（stdlib-only，≠ app 包故无环——scheduler 在依赖链 app→handler/proxy→scheduler 之下，无法 import app）承载 `dbConnErrorsTotal` 计数器；`handler/shared` 委托 Record/Read/Reset；scheduler 改 import leaf。boundary gate 删除 §5.11 例外，scheduler→handler/* 现硬拒绝。`package-boundaries.md` §5.11+§6 标 RESOLVED。
- CHANGELOG `N1` + `UIUX wave 2/3` 条目；STATE tip `d4633f1`。

## [2026-07-31] UIUX wave 2/3 + 视觉质感（New API 前端对标）

- **Wave 2 design-system 三件套**（`28ce05a`）: 新增 `design-system/ErrorState.tsx`（EmptyState tone=danger + 默认告警 icon + 自动 Retry，role=alert）+ `LoadingState.tsx`（block 变体 N 行 skeleton + inline 变体 spinner+label，均 role=status aria-live=polite），补齐 Empty/Loading/Error 三件套（对齐 New API B1）。ProgramLogs 落地 LoadingState 替换 bare skeleton。测试 `design-system/states.test.tsx` 4 例。
- **Wave 2 i18n**（`3cd2525`）: ImportExport 884 行仅 1 个 tr() → 全量用户可见字符串包 tr()（toast/confirm/动态拼接 + 模块级 option label）；i18n.supplement.ts 补 EN 映射。typecheck + ImportExport 9/9 通过。
- **Wave 3 Models→Playground 快跳**（`df94e72`）: Models 卡片/表格行加「在 Playground 测试」按钮 → `/playground?model=`；ModelTester 读 `?model=` query 预填模型选择器（优先于 restored session）。i18n 补「在 Playground 测试」。修 ModelTester 测试缺 MemoryRouter（useSearchParams 需 Router 上下文）。
- **Wave 3 ProxyLogs 日期预设**（`2da7401`）: filter 区加 15m/1h/今天/7d 预设按钮（setRangePreset 调 endTime=now+startTime 回填 datetime 输入 + 触发 load）。i18n 补预设 EN 映射。
- **视觉质感**（`a352ab4`）: Models 卡片网格加 `animate-slide-up stagger-N`（复刻 New API CARD_STAGGER_VARIANTS，capped stagger-8=0.32s）+ `model-card:hover` translateY(-2px) 上浮微交互。page-enter 保持 opacity-only（transform 祖先会破坏后代 sticky 表头，故不加 y/blur）。reduced-motion 全局守卫已覆盖。
- Pre-push: `npm run typecheck` clean; 各批 vitest 通过（design-system 4/4、ImportExport 9/9、Models 11/11、ProgramLogs 3/3、ModelTester forced-channel 修复后通过）。
- **硬门禁**: 以上均为 A 类（低风险 UIUX/视觉/修 bug），非产品功能决策；后端产品功能 N1-N9 仍需拍板（见 `product-parity-and-newapi-borrow-2026-07-30.md`）。

## [2026-07-30] UIUX wave 1 — 防御性 + 诚实性 + a11y（New API 前端对标）

- **RouteErrorBoundary**（`web/components/RouteErrorBoundary.tsx`，新建）：class 边界包裹 `App.tsx` 全部 lazy `<Routes>`。此前全仓 0 处 error boundary，任一 lazy 路由 render 抛错=白屏。keyed on `location.pathname` 离开即清；fallback 含 message+重试。对齐 New API per-route `errorComponent`（不引入 TanStack）。测试 `RouteErrorBoundary.test.tsx`。
- **SearchModal 真键盘导航**（`web/components/SearchModal.tsx`）：footer 一直显示 `↑↓`/`Enter` 提示但 0 实现（诚实性 bug）。重构 6 section 为单一有序 `flat` 列表 + 全局 `activeIndex`；ArrowDown/Up 移动+wrap、Enter 打开活动项、`scrollIntoView` 跟随。input 升级 `combobox`+`aria-activedescendant`/`aria-expanded`，结果 `role="listbox"`，项 `aria-selected`。`search-modal.results.test.tsx` 增键盘用例。
- **Toast a11y**（`web/components/Toast.tsx`）：容器 `role="status" aria-live="polite" aria-atomic`；error toast `role="alert"`（assertive），success/info 保持 status。测试 `toast.a11y.test.tsx`。
- Pre-push: `npm run typecheck` clean；vitest 515/515（`tokens.edit-and-select.test.tsx` 有 1 个非失败 jsdom focus-trap teardown flake，预先存在，`useFocusTrap.ts` 未动）。
- CHANGELOG `UIUX wave 1` 条目；STATE tip `0c10296`。

## [2026-07-30] UIUX/产品化借鉴综合（New API 前端对标）

- **Synthesis**: `docs/analysis/uiux-newapi-borrow-2026-07-30.md` — 两路 Explore（metapi-go web 审计 + New API 前端深度调研）综合。本轮聚焦前端 UIUX/产品化，参考 New API `reference/competitors/new-api/web/default/src`（TanStack+base-ui+VChart+cmdk+oxlint），借鉴**模式与组件抽象**（不引入 TanStack 全家桶，保持手写 React 栈）。
- **metapi-go web 短板（审计实证）**: 无 ErrorBoundary（lazy 抛错=白屏）；`SearchModal.tsx:321-325` 显示键盘提示但 0 实现（诚实性 bug）；`Toast.tsx:66` 无 aria-live；ImportExport 884 行仅 1 个 tr()；列表页无 CSV 导出；filter 不进 URL；design-system Button 全 pages/ 仅 1 处采用；巨石页面（ProxyLogs 3605 等）。
- **New API 可借鉴 12 项**: B1 标准化 Loading/Error/Empty · B2 受控 ConfirmDialog · B3 可复制 StatusBadge · B4 stringToColor · B5 IME 感知 debounce · B6 URL 同步表状态 · B7 浮动批量栏 · B8 Cmd+K 面板 · B9 扁平 JSON i18n+static-keys · B10 motion+reduced-motion · B11 Section Registry · B12 masked-value reveal+copy。
- **优先级**: Wave 1 防御性+诚实性+a11y（ErrorBoundary/SearchModal 真键盘导航/Toast aria-live，A 类直接执行）；Wave 2 design-system 状态三件套+i18n；Wave 3 列表体验（CSV 导出/URL 状态/内联 test/日期预设）；Wave 4 借鉴优化。后端产品功能 N1-N9 不重叠（见 product-parity 文档，B 类需拍板）。
- docs SSOT: STATE.md Next-wave + docs/README.md layout + log 指针；`doc_hygiene_test` 通过。

## [2026-07-30] product parity & New API 借鉴综合

- **Synthesis**: `docs/analysis/product-parity-and-newapi-borrow-2026-07-30.md` — 合并 metapi-ts 原版对标 + New API 上游借鉴调研（子代理产出经 leader 源码交叉复核）。
- **原版对标结论**: Go 重写未丢任何 TS README 头部产品功能；14 平台适配器 TS=Go 对齐；慢请求/热力图/品牌分类/跨站比价 Go 超出 TS。子代理 4 报告缺口经复核：G3 代理调试 trace = 误报（`stats.go:23-24` 已实现）；G2 workflow stub = 降级为文档叙事问题；G1 余额告警 = 真缺口但 TS 也只在每日摘要 count 未实时落地（README 写了双边都未做）→ 可做得比原版好；G4 WS residual 已诚实声明。
- **New API 借鉴**: 子代理 10 项 → leader 复核剔除 1 误报（#8 Playground 已有 `ModelTester.tsx`+`test.go:26-37`），确认 9 项：N1 下游 key IP 白名单/黑名单（P0 安全缺口）、N2 公开价格目录页（P1 聚合网关卖点）、N3-N9 借鉴优化（推理参数后缀/渠道测试/下游 key 看板/CSV 导出/cache ratio/单渠道多 key/倍率可视化）。明确不适用：多用户/支付/兑换码/邀请/订阅（与聚合网关定位冲突）。
- **硬门禁**: 以上均为产品功能（非工程纪律），按门禁需先开 Issue 讨论 / 用户拍板再动，**本轮不自动实现**。本文件是决策输入，不是执行令。
- docs SSOT: STATE.md Next-wave + docs/README.md layout 增 product-parity 指针；`doc_hygiene_test` 通过。

## [2026-07-30] engineering optimization wave (codeg 对标)

- **Synthesis**: `docs/analysis/engineering-optimization-2026-07-30.md` — 6-dim fable fleet (中断后 leader 源码重证) 对标 codeg 工程纪律（非产品功能 parity）。结论：边界已守干净（BACKEND.md §2.3 八条 0 违规，`go vet` rc=0 无 cycle），缺口在**未固化成机器断言**。
- **P0 包边界 CI 断言**: 新增 `docs/package_boundary_test.go`（`docs_test` 包，`go list -deps`/AST 解析）把 BACKEND.md §2.3 八条硬规则落成 `go test` 机器门禁，对标 codeg `test.yml:134-148` grep-gate。运行中**当场发现**一处 stale 例外：`scheduler/lease.go:17` import `handler/shared`（B1 §6 "scheduler Pass" 过时）→ 记录为 §5.11 例外 + 治理 follow-up（metrics 抽到 `app/observability` leaf），非静默放行。
- **P1 neat-freak 归档**: 7 个过期一次性文件 → `docs/archives/2026-07/`（p4-account-verify / p4-settings-proxy-test / p4-token-adapter-wiring / ui-score-2026-07-19 / ui-score-shell-2026-07-19 / ui-score-shell-mock-2026-07-19 / ui-pm-empty-state-2026-07-19）；逐文件确认全仓 0 活跃引用（`p4-admin-test-routes.md` + `ui-score-pages-2026-07-19.md` 因被引用而保留）。新增 `docs/archives/2026-07/README.md` 记录归档策略与「未归档」清单。
- **P2 巨石拆分**: `handler/admin/stats.go` 1544→826 行，抽出 `stats_helpers.go`（modelAllowed/parseTruthyQuery/queryRow/nowUTC/roundMicro/coerce* 纯工具）+ `stats_marketplace.go`（marketplace/token-candidate/without-token/missing-group/endpoint-type builder + infer/resolve helpers）。行为中性（同包、同导出面），`model_price_compare.go` 的 coerce* 调用不受影响。
- **docs SSOT**: `package-boundaries.md` §6 标注机器化 + §5.11 新例外 + §7#8 DONE；`engineering-optimization-2026-07-30.md` 路径脱敏（去掉本地临时目录路径，过 `doc_hygiene_test`）。
- Pre-push: `go vet ./...` rc=0 · `go test ./... -count=1 -race` 全绿 · `docs` hygiene+boundary 测试通过。
- **非目标（硬门禁内）**: 不动 wire 契约；不自动 pin 0.8.45（需管理员授权）；错误码注册表/测试巨石拆分/decoupling 测试锁记录为 Issue 候选，本轮不开。

## [2026-07-30] fable fleet review + CI unblock + dual-dialect encapsulation

- **Fable 6-dimension audit fleet** (Workflow): backend-correctness/arch, frontend-parity/quality, docs-ssot, security-config → 14 raw findings → adversarial verify → 9 confirmed, 3 rejected. 4 reviewer agents + 2 verifiers hit fable 429 rate-limit (incomplete dims: backend-correctness, frontend-parity/quality, security-config) — those dims not landed; re-run later if needed.
- **CI unblock** (`146c538`): `DownstreamKeyEditorModal` `item.title/subtitle`→`item.label/detail` (TS2339, #579 regression, CI `frontend` job red); `dispatchUpstream` dropped redundant `if ctx != nil` (staticcheck SA5011, CI `lint` job red). CI was failing since 2026-07-20 across `lint`+`frontend`.
- **dual-dialect encapsulation**: `store.DB` gains `ExecContext/QueryxContext/QueryRowxContext/GetContext/SelectContext` (rebind `?`→`$N` for PG); `app/proxy_upstream.go` + `handler/proxy/proxy_log.go` delegate, removing 4 manual dialect branches. Semantic splits (RETURNING id, advisory lock, ON CONFLICT) kept.
- **docs SSOT sweep**: docs/README residual board v0.8.43/M50→v0.8.45/M53; STATE active milestone M52→M53; deployment/migration Go 1.26.4+→1.26.5+; project-overview tagged historical; residual-next release links +v0.8.44/45; log.md header moved to top; MASTER "this session"→date.
- Pre-push: go vet ./..., golangci-lint, go test ./... -race, tsc web/test, vitest 155/155, docs-hygiene all green.

## [2026-07-21] rebuild embed SPA for About honesty

- Rebuild `web/dist` via `npm run build:web` so embed About shows v0.8.45, Go stack, TokenDanceLab links (drop Fastify 1.3.0 theater).
- `web/embed.go` build comment points at in-repo `web/` (not legacy metapi TS monorepo copy path).
- `web/package.json` version 0.8.45; `README_EN.md` GHCR badge 0.8.45; About privacy dual-dialect storage.

## [2026-07-21] tip after #557 procedure

- Tip `d0288f3`: p0585 production e2e procedure + probe harness on master.

## [2026-07-21] #557 P0-585 production e2e procedure + probe script

- `docs/analysis/p0585-production-e2e-procedure.md` — pass/fail, staging inject, evidence harvest.
- `scripts/p0585_cascade_probe.py` — dry-run default; `METAPI_P0585_LIVE=1` for authorized live probes.
- Does not flip P0-585 present; does not pin production.

## [2026-07-21] tip after P0-585 HTTP e2e

- Tip `0027db6`: e2e cascade load-proof + M53 board #557/#558.

## [2026-07-21] SDD REL board: M53 + #557/#558

- Milestone **53 REL-HONESTY**; issues #557 (P0-585 prod e2e) · #558 (runtime probes optional).
- HTTP cascade e2e shipped same session; board tracks remaining prod/live residual.

## [2026-07-21] REL: P0-585 HTTP cascade e2e (partial remains)

- `e2e/e2e_p0585_cascade_test.go`: multi-channel 5xx storm channel-scoped exclude + recover healthy sibling.
- mockRouter `excludeSnapshots` for SelectNextChannel honesty.
- Does **not** flip P0-585 present (prod/live storm still required).

## [2026-07-21] tip after stream missing-usage metric

- Tip `56eca67`: `metapi_stream_missing_usage_total` + OrphanLogs; README unreleased tip honesty.

## [2026-07-21] P0-555 stream missing-usage metric

- `metapi_stream_missing_usage_total` + `RecordStreamMissingUsage` when include_usage stream ends without usable tokens.
- Wire from `warnMissingStreamUsageAfterIncludeUsage`; tests for metric + warn path.

## [2026-07-21] matrix #571 static evidence note

- #571 remains unknown-needs-runtime for live OAuth; static allowlist/seed/probe + `codex_models_test.go` documented in matrix.

## [2026-07-21] tip after OrphanLogs observability

- Tip `c9ca3e7`: usage projection OrphanLogs + slog; P0-555 still present-with-residual.

## [2026-07-21] P0-555 orphan projection observability

- `ProjectionPassResult.OrphanLogs` + slog when proxy_logs skip site buckets; watermark still advances (no double count).
- Honesty tests assert orphan count; docs residual updated. P0-555 remains present-with-residual.

## [2026-07-21] routing estimate envelope tests + #577 evidence

- `routing` EstimateRequestContextTokens tests for Claude system + Gemini contents.
- matrix #577 notes unit anyrouter checkin coverage; live model-list residual.

## [2026-07-21] tip after TPM estimate polish

- Tip `e408fdf`: auth TPM counts Claude system + Gemini contents; pre-push race clean.

## [2026-07-21] TPM estimate Claude/Gemini envelope parity

- auth `estimateAdmissionTokens` counts `system` + `contents` string leaves (align routing #514 estimate).
- Tests: Claude system increases estimate; Gemini contents-only path.

## [2026-07-21] plan: parity core status banner

- `original-parity-complete` waves A/C/D marked present; B REL open; E runtime optional; ops pin gated.

## [2026-07-21] tip 530fab6

- Final tip after residual/matrix/formal sync push (pre-push race clean).

## [2026-07-21] tip after residual/matrix/formal sync

- Tip `a87ef7e`: residual Active wave = parity core shipped; matrix #520/#555/#489 honesty; formal-readiness product tip headers.

## [2026-07-21] neat-freak: residual/matrix/formal after parity ship

- residual-next **Active wave** → parity core shipped (not "when coding starts").
- formal-readiness table headers → product tip ≥ v0.8.45 (not v0.8.44 column label).
- matrix: #520 present-with-residual (stale missing-field claim); #555 present-with-residual; #489 discovery timeout present; #571 evidence refresh (still runtime).

## [2026-07-21] tip after embed SPA rebuild

- Tip `c1daeab`: web/dist embed matches About source honesty; pushed master (pre-push race clean).

## [2026-07-21] neat-freak: formal-readiness + About honesty

- formal-readiness Track B gates refresh: WS C1–C3 present · UC-1 hide/external present · P0-585/555 residual honesty; A2.1 ops pin lag noted.
- About page: version placeholder **0.8.45**; tech stack Go/React/Vite/SQLite+PG (drop Fastify/Drizzle theater); links → TokenDanceLab/metapi-go + GHCR/Releases.
- README GHCR badge → v0.8.45.

## [2026-07-21] neat-freak: CHANGELOG unreleased + #514 UI hint

- CHANGELOG [Unreleased] captures KEYS / WS C1–C3 / #514 / UC-1 / cloud-ops / P0-555 media fold.
- TokenRoutes context_length help: multi-tier same-model routes (#514).
- high-value matrix leftover #547 → present (was stale partial).

## [2026-07-21] P0-555 media usage details + route sort_order load

- Usage: fold OpenAI `input_tokens_details` / `output_tokens_details` / `*_tokens_details` text/image/audio leaves into prompt/completion **only when top-level is missing** (no double-count).
- Honesty tests: media fill, no double-count, zero details no invent; P0-555 remains present-with-residual (multi-instance lag / orphans).
- Routing load: `LoadEnabledRoutes` SELECT/ORDER BY `sort_order ASC, id ASC` (admin list order parity for multi-route match bucket).

## [2026-07-21] UC-1 hide/external Update Center honesty

- User decision: no invent registry; deploy via GHCR/ops.
- Backend: status/check `mode=external` + residual; deploy/rollback/SSE remain honest 501.
- UI: Settings `UpdateCenterSection` → short ops card (Releases/GHCR links); hide check/deploy/rollback controls.
- About: no "发现新版本" theater from local stub; link to settings ops note.
- Tests: admin mode assert + vitest honesty cards.

## [2026-07-21] #514 multi-tier context routing

- Product: multiple same-model `token_routes` with different `context_length` → pick tightest ceiling that fits estimated request ctx.
- Estimate: `routing.EstimateRequestContextTokens` (messages/input chars÷4 + max_tokens/max_output_tokens); 0 = first-match honesty.
- Pick: `PickContextTierRoute` among match-bucket routes; wired in `findRoute` + dispatchUpstream / WS C3 policy.
- Tests: unit + SelectChannel multi-tier integration.
- Residual: estimate is best-effort (not a tokenizer); no new schema — reuses CTX-520 `context_length` + multi-route config.

## [2026-07-21] WS C3: Codex upstream wss runtime

- Status: `c3_codex_upstream_wss` (was `c2_multi_turn_http_bridge`).
- Runtime: `handler/proxy/codex_ws_runtime.go` — dial/reuse upstream wss, wait terminal events, process-local previous_response_id store + tool-output infer/recovery strip.
- Capability probe: platform=codex + `CodexUpstreamWebsocketEnabled` + optional account extraConfig `websockets`.
- Wire: `tryCodexUpstreamWSS` before HTTP SSE bridge; dial/empty-event failures fall back to bridge (no fake terminals).
- Tests: URL/headers/body/store/continuation helpers + residual status C3.
- Residual: multi-instance pin honesty only (no STICKY-B).

## [2026-07-21] UI cloud-ops 对齐（tokendance-design）

- 参考 `TokenDance/tokendance-design/styles/cloud-ops/` 全面收紧管理台视觉。
- Tokens：canvas `#f8f9fa`/`#202124`、GCP 语义色、radius 4/8/12、topbar 48 / sidebar 232、Material e-1 阴影、可选 `data-density=compact`。
- Shell：实色侧栏 + 轻 blur 顶栏；去掉重 glass/卡片抬升；chip/table/page-title 按 StatusChip/DataTable/PageHeader 密度。
- FOUC：`index.html` + `themeBootstrap` 与 tokens 同色；单测期望同步。
- 说明：[`design/cloud-ops-alignment.md`](design/cloud-ops-alignment.md)。不 pin 生产。

## [2026-07-21] #579 multi-credential / multi-site allow-list bind

- Schema additive `sc2_009_downstream_key_allow_lists`: `allowed_site_ids` + `allowed_credential_refs` (empty = unrestricted).
- Auth policy + routing eligibility: allow-list gates before exclusions; both can compose.
- Admin create/update/validate + DownstreamKeys form types + editor allow-list panels.
- Tests: `routing/allowlist_579_test.go` site/credential allow + exclude still wins.
- Product AC: one downstream key can **specify** sites/credentials (not only exclude). Not a rename of exclusions.

## [2026-07-21] WS C2: multi-turn + per-message quota

- Status string: `c2_multi_turn_http_bridge` (was `c1_http_bridge`).
- Multi-turn merge: last input + last output + new input (non-incremental).
- Incremental: client `previous_response_id` on `response.create` keeps id (no history force-merge); mode header `x-metapi-responses-websocket-mode: incremental` on bridge.
- Per-message: `auth.ConsumeManagedKeyRequest` after normalize for managed keys; ProxyAuth skips used_requests on WebSocket upgrade handshake (TS parity).
- Model gate: `IsModelAllowedByPolicy` each turn before bill/bridge.
- Prewarm still local only on first create with `generate=false` and non-incremental.
- Tests: merge / incremental / prewarm+incremental / residual status; auth upgrade detector unit.
- Residual: C3 Codex upstream wss + channel capability probe; multi-instance sticky still single-instance honesty.

## [2026-07-21] WS C1: Responses WebSocket HTTP bridge

- Dep: `github.com/coder/websocket` (single WS library).
- Upgrade path: GET `/v1/responses` (+ alias) → real Accept after ProxyAuth; 401 without auth; plain GET still 426.
- Session: `response.create` single-turn + local prewarm (`generate=false`); in-process `HandleResponses` SSE→WS bridge.
- Tests: upgrade auth guard, normalize helpers, prewarm dial integration; no fake completions on real turns.
- Residual: C2 multi-turn incremental · C3 Codex upstream wss · single-instance honesty (no STICKY-B).

## [2026-07-21] #584 site custom header override priority

- Schema `custom_headers_override_request_headers` + additive `sc2_008_site_custom_headers_override_request_headers`.
- `platform.ApplyCustomHeadersWithOptions`: default **request-wins** (only fill missing); opt-in **site-wins** (`OverrideRequest` / site flag).
- Wired: ProxyConfig flag ← site · BuildPlatformProxyConfig · upstream apply · site_proxy Do/DoWithProxy · channel health probe.
- Admin create/update + Sites UI checkbox「站点请求头覆盖客户端同名头」; deny-list unchanged.
- Tests: request-wins / site-wins / sensitive still denied; store column count Site 21.

## [2026-07-21] #547 per-downstream-key weight

- Schema `key_weight` + additive `sc2_007_downstream_key_weight`.
- Auth policy + routing: `KeyWeight` multiplies `channel.Weight` in weighted selection (NULL/≤0 → 1.0).
- Admin create/update + DownstreamKeys UI "密钥权重".
- Tests: normalize helper + weighted amplification; schema column count 24.


## [2026-07-20] neat-freak + SDD: original parity program (ex-Electron)

- Plan SSOT: `docs/plan/original-parity-complete-2026-07-20.md`.
- User decisions: WS-1 **full TS parity** (C1–C3); sticky **single-instance honesty** (no STICKY-B now); UC **hide/external deploy**.
- MASTER + high-value-next + STATE rewritten for parity program schedule.
- Truth: #534 bulk import **present** (matrix row + summary; was stale missing); #520 CTX present-with-residual; OAuth/Sub2API refresh present.
- Residual-next + responses-websocket-residual: WS scheduled C1–C3; STICKY-B deferred; UC hide/external; residual 426/501 until C1.
- Next: open Issues or start Wave KEYS (#547/#584) + WS C1 when coding resumes. No product code this entry.

## [2026-07-20] 四路并行原版功能对齐研究

- 4 路 sonnet 代理：后端路由 · 平台/调度 · 前端 · gap 矩阵对抗复核。
- 前端 18 路由 + 14 侧栏 **100% 齐平**；14 平台适配器完整对齐；调度 16 任务全覆盖。
- 明确缺口：**Responses WebSocket**（501 residual）· Sub2API 托管刷新仅扫描 · Update Center 纯占位（UC-1）· OAuth 定期 token 刷新无独立 scheduler。
- gap 矩阵漂移：**#513 model_mapping** → present（`ResolveMappedModel` + routing wire 完整）；其余 backlog=yes 均 CONFIRMED。
- 结论：**Track A 对内可用（是）· Track B 对外「完全完备」（否）** — WS/Sticky/UC/级联e2e/计费residual 仍在。

## [2026-07-20] Release v0.8.45 — RE2-safe + UI tip

- Tag **v0.8.45**: RE2-safe NewAPI user-id extract (blocks production restart) + M51–M52/console density UI + fail-fast probe tests.
- Ops: still **must not** auto-start; pin/up **0.8.45** only after GHCR image + ≥15min background soak authorization.
- Residual: empty-DB AUTH recapture; VIS-1/NAV-1 optional; P0-585 prod e2e; P0-555 residual.

## [2026-07-20] RE2-safe NewAPI user-id extract (production crash root cause)

- Ops: hk3 **0.8.44 Exited(2)** after balance refresh compiled PCRE lookahead `_(\d{4,8})(?!\d)` (Go RE2 panic).
- Fix on tip: `platform/newapi.go` package-level `underscoreUserIDRE` / `namedUserIDRE` without `(?!\d)`; length >8 rejected in Go.
- Tests: `TestNewApiAdapter_ExtractLikelyUserIDs_RE2Boundaries`.
- Historical branch `codex/metapi-regex-crash` (`f1c629d`) was **not** on master; reapplied onto current tip.
- Also: user-id probe loops honor `ctx.Done()`; adapter unreachable tests use closed local listener (`unreachableBaseURL`) instead of `:1` blackhole; SiteProxy dial timeout 2s; pre-push race timeout 300s.
- Residual: tag/release (candidate **v0.8.45** = RE2 + unreleased UI tip) → GHCR → **15min background soak** → authorized ops pin/up only. Do not auto-start.

## [2026-07-20] Linux gallery baselines = GHA actuals (not Docker)

- ui-visual failed: console density changed full-page height; Docker jammy snapshots still drift vs GHA fonts (light 3919 vs 3953).
- SSOT: copy CI `design-gallery-*-actual.png` → `*-chromium-linux.png`; drop serial so dark actuals also upload.
- light `016ec80` + dark `4f05736` → **ui-visual success** (run 29701482781).
- Residual: UI release decision; empty-DB AUTH page recapture.

## [2026-07-20] Linux gallery baselines after console density (partial)

- First attempt: Docker Playwright v1.61.1 jammy regenerate — insufficient vs GHA; superseded by GHA actuals entry above.

## [2026-07-20] Console density + hi-res type polish

- System font stack (drop Google Fonts Inter CDN); letter-spacing / line-height tokens; page-title + KPI weight 400; tabular nums.
- Pill sidebar/topbar active nav; calmer card hover (no translateY lift).
- `.main-content` max-width ladder 1680→1920→2280→2600 + centered; larger pad on 2560+.
- Docs: DESIGN.md + ui-ux-refresh abstract only (no private portal facts).
- Residual: linux baselines (fixed next entry); UI release decision; empty-DB AUTH recapture.

## [2026-07-20] Shell mock sidebar full parity (14 items)

- User saw truncated left nav → root cause was `/__design__` Shell chrome mock (3–4 items), not product `sidebarGroups`.
- `DesignSystemGallery` shell mock now lists full production labels (控制台 10 + 系统 4); topbar adds 模型操练场.
- Unit guard `designSystemGallery.shell-nav.test.ts`; shell-*.png recaptured; win32 gallery visual baselines updated; `web/dist` rebuilt for embed.
- Residual: linux gallery baselines may need CI actuals if pixel-diff; empty-DB real page shots still need AUTH token; UI release decision.

## [2026-07-20] UI 原版对照 inventory（功能未删）

- 用户反馈「丑 + 原版功能和按钮全没了」→ 对照 `TokenDance/metapi` web vs metapi-go tip。
- 结论：侧栏 18 路由齐平；Sites/Accounts/Tokens/Routes/Settings 按钮计数齐平；`/tokens` 两边均 redirect 到连接管理。
- 体感来源：空库稀疏 + ops pin 0.8.44 未含 tip first-run/glass + 主题 indigo→GCP blue + 仓库空库截图仍 pre-#553/#554。
- 文档：`docs/analysis/ui-original-parity-2026-07-20.md`；STATE/MASTER residual 指针更新。无产品代码。

## [2026-07-19] M52 Wave2 first-run closed — epic #548 done

- Merged #554 Sites banner defer (PR #555 `68ff46e`) · #553 Dashboard getting-started (PR #556 `479f52c`).
- #553 fixup: Dashboard unit tests wrap `MemoryRouter` (Link context); frontend CI green.
- Closed epic #548; Milestone 52 residual = optional shot recapture + **UI release decision**.
- Tip `479f52c`; ops pin still **v0.8.44** unreleased.
- Board empty; mode → maintenance.

## [2026-07-19] M52 Wave1 merged — screenshot residual polish

- Milestone **52 UI-POLISH** + epic #548; Project items Todo.
- Wave1 closed: #543 Traffic sparkline · #544 real page score honesty · #545 hex soft pass · #546 axe smoke (PRs #549–#552).
- Unblocked CI frontend: dual-CTA EmptyState tests (`8bd9ec1`).
- First-run product backlog: #553 Dashboard zeros · #554 Sites weight banner.
- Tip `9092a4b`+; ops pin still **v0.8.44** unreleased.

## [2026-07-19] UI polish: focus-trap + EmptyState residual + skip-link

- Shared `useFocusTrap` wired into SearchModal / CenteredModal / MobileDrawer / NotificationPanel.
- Skip link → `#main-content`; sidebar `:focus-visible`; chrome i18n for nav/skip.
- EmptyState: Accounts, Tokens panel, ModelTester conversation empty.
- typecheck + related vitest pass; web dist rebuilt. Still **unreleased** (ops pin v0.8.44).
- Residual: optional live authed shots, hex hygiene, axe CI, UI patch release decision.

## [2026-07-19] M51 UI-REFRESH epic closed (unreleased)

- Closed #532 epic + #538 (mock track). All M51 children closed.
- Tip `168e8ee`; ui-visual CI green; ops pin remains v0.8.44.
- Residual only: optional live authed shots, focus-trap/hex, Accounts/ModelTester empty, UI patch release.

## [2026-07-19] M51 closeout: foundation issues + Linux CI green + more EmptyState

- Pushed linux gallery baselines; `ui-visual.yml` **success**.
- Closed #533–#536 · #539 (with #537/#540/#541 already closed).
- EmptyState: Sites / ProxyLogs / OAuth / TokenRoutes; residual Accounts/ModelTester/Tokens panel.
- Epic #532 open for #538 real authed shots + optional UI release decision.

## [2026-07-19] #539 Linux gallery baselines + more EmptyState pages

- Committed `design-gallery-*-chromium-linux.png` from CI failure actuals (ubuntu Playwright).
- EmptyState adoption: ProgramLogs + SiteAnnouncements.
- Residual: #538 real authed page shots; Accounts/OAuth/ProxyLogs empty migration; focus-trap/hex.

## [2026-07-19] #541 EmptyState page adoption (DownstreamKeys/CheckinLog/Models)

- Migrated empty surfaces to design-system `EmptyState` + primary action.
- Residual: remaining pages (Accounts/Sites/Logs/OAuth/…); Tokens panel is redirect-only.

## [2026-07-19] UI-REFRESH Phase 4/5/shell **source** merge + EmptyState

- Fixed incomplete prior tip: worktree source for #537/#540/#538 actually merged to product tree (forms/a11y/shell mock).
- #541: `EmptyState` ds primitive + gallery samples; legacy `.empty-state` retokenized.
- Playwright e2e 7/7; gallery score axes 5/5; win32 baselines refreshed.
- Residual: #539 Linux CI baselines; real authed Dashboard/Sites/Settings; focus-trap/hex.

## [2026-07-19] UI-REFRESH Phase 4/5 + shell mock integrated

- Phase 4 (#537): form/drawer/modal Apple-detail density (36px controls, glass chrome, gallery samples).
- Phase 5 (#540): prefers-reduced-motion hard-cut + reduced-transparency solid glass fallbacks.
- #538: auth-free shell chrome mock (Dashboard/Sites/Settings) + capture SOP + METAPI_PW_FORCE_SERVER.
- Security: TokenRoutes escapeHtml also escapes apostrophe for dialog HTML.
- e2e 7/7 green; gallery score axes 5/5 after shell mock height growth.

## [2026-07-19] docs: M51 Phase 4–5 board + worktree lanes

- Open issues #537–#541 on Milestone 51 (forms · shell shots · linux baselines · a11y · empty/error).
- Worktree lanes: `ui/phase4-forms` · `ui/shell-page-shots` · `ui/phase5-a11y` under `.worktrees/*`.
- MASTER/STATE board lists #532–#541; Phase 1–3 remains on master (`af3a4d2`); Phase 4–5 not yet merged.

## [2026-07-19] UI-REFRESH Phase 3 data surfaces + scored gallery

- Token-only polish: dual-theme semantic `*-ink`, purple badge family; tables/filters/pagination/toasts/badges retokenized in `index.css`.
- Gallery sample: filter chips + pill tabs + data-table + pagination; win32 visual baselines refreshed; `npm run test:e2e` 7/7 green.
- Capture script `web/scripts/capture-ui-shots.mjs` + `docs/analysis/ui-shots/*` (login/gallery light+dark).
- Residual: Linux baselines, Dashboard/Sites/Settings shell screenshots, Phase 4 forms/drawers.

## [2026-07-19] UI-REFRESH GCP/Apple token + card density

- Primary remapped to GCP blue (`#1a73e8` / dark `#8ab4f8`); cool gray accent; FOUC canvas retained.
- New semantic radius (`control`/`card`/`shell`), dual soft shadows, `motion-swift`/`motion-soft`.
- Cards/stat-cards/page-header + design-system primitives consume new tokens; DESIGN.md rewritten.

## [2026-07-19] UI-REFRESH Phase 2 shell glass

- CSS-only glass chrome in `web/index.css`: topbar, sidebar, user-dropdown, mobile drawer, login surfaces.
- Token-only (`--glass-*`); `@supports` + `prefers-reduced-transparency` solid fallbacks.
- Login/sidebar unit tests green; typecheck green.

## [2026-07-19] UI-REFRESH Phase 1 foundation in tree

- FOUC #535: `themeBootstrap` + head script theme_mode-first; canvas #0b0f14/#f4f6f8; unit + e2e contracts.
- Design system #533: `web/design-system/*` primitives (ds-*) + `/__design__` gallery (auth-free when DEV or `metapi_design_gallery=1`); glass tokens.
- Visual/e2e #534/#536: Playwright harness (`web/e2e/*`, Makefile `ui-e2e`/`ui-visual`, CI `ui-visual.yml`); vitest excludes e2e.
- Residual: gallery snapshot baselines, DESIGN.md full rewrite, shell glass Phase 2.

## [2026-07-19] UI-REFRESH M51 opened + multi-lane kickoff

- Milestone 51 UI-REFRESH; issues #532 epic, #535 FOUC, #533 design-system, #534 visual, #536 e2e.
- Session loop every 10m; lanes: FOUC / design-system / visual+e2e harness.

## [2026-07-19] design: formal readiness + UI-REFRESH

- Added `docs/analysis/formal-readiness.md` — Track A 对内正式可用（已达标）vs Track B 对外完备（未达标）；T0–T4 运行档位；Redis 可选。
- Added `docs/analysis/ui-ux-refresh.md` — GCP IA + 白磨砂玻璃 + 苹果细节；FOUC/夜间闪光弹 P0；分 Phase 落地，未改 web 实现。

## [2026-07-19] ops: hk3 deploy v0.8.44 shared-tiny

- Pin + up `td-metapi` 0.8.44; compose force `DB_PROFILE=shared-tiny` + MaxOpen/Idle 1/1 + `application_name=metapi-hk3`; restart=no.
- Verified healthy/ready; metrics open=1 errors=0; Azure backends=1; no 53300; NewAPI ok.

## [2026-07-19] #531 PostgreSQL pool budget profiles + lease pressure

- Product: `DB_PROFILE` shared-tiny/normal/dedicated; default normal 10/3 (dedicated still 20/5 for large DBs).
- Inject `application_name=metapi-<host>`; startup banner logs profile + pool.
- Scheduler lease: MaxOpen≤2 → local; 53300 backoff + log denoise + force-local.
- Metrics: db_connections_in_use + db_conn_errors_total.
- Docs: `docs/analysis/db-pool-budget.md`; CHANGELOG v0.8.44 pending tag/deploy.

## [2026-07-19] M50 v0.8.43 residual honesty + us1 pin

- GitHub Milestone 50 + Project items #527–#530.
- P0-585 unit load-proof tests (5xx storm + 429 same-channel policy); P0-585 stays partial.
- P0-555 Gemini SSE usageMetadata honesty tests; stays present-with-residual.
- us1 `/opt/tokendance-us1` compose pin 0.8.42 + pull; cold no auto-start.
- Docs: residual / high-value-next / MASTER / STATE / CHANGELOG.

## [2026-07-18] docs: high-value-next shortlist (ours vs original)

- Add `docs/analysis/high-value-next.md` separating metapi-go residual from cita-777/metapi parity leftovers.
- Banner matrix/sources as historical; residual header → post v0.8.42; wire README/STATE/MASTER entry points.
- No product board opened; maintenance default remains.

## [2026-07-18] v0.8.42 cron validation + prod roll-forward

- Fix: config `validateCronExpr` accepts default 5-field crons (parity with scheduler normalize).
- Ship/tag v0.8.42; deploy hk3 pin 0.8.42; generate `ACCOUNT_CREDENTIAL_SECRET` when missing (no OAuth client invent).
- Residual: OAuth client placeholders remain intentional until real client IDs are configured.

## [2026-07-18] deploy v0.8.41 to hk3 (0.6.5 → 0.8.41)

- Tags: v0.8.40 (PG pool + docs) · **v0.8.41** (request_id index upgrade fix for old DBs).
- Prod: Azure PG `tokendance-pg` / role `metapi`; container `td-metapi` healthy; migrations sc2_001–006 applied.
- Ops fix: role CONNECTION LIMIT 2→15; app pool max_open=5 idle=2.
- Evidence: `/health` `/ready database=ok`; admin auth OK; 103 sites; public 302 to ID.

## [2026-07-18] neat-freak: STATE/MASTER/LOG roles + branch hygiene

- Closed M49 / shipped **v0.8.39**; board empty.
- Post-tag **#526** landed on master: explicit PostgreSQL pool budget (config + store + docs).
- Progress docs split: **STATE** = 现状, **MASTER** = 开放门禁, **LOG** = 本文件; no HANDOFF SSOT.
- Pruned ~255 agent worktrees → main only; deleted merged-PR remote heads (~200+) and abandoned leftovers; local non-master cleaned.
- Memory pointer updated for metapi-go docs map.

## [2026-07-18] v0.8.39 / M49 adversarial bugfix residual

- Product: RR fail-count, used_requests 429 order, Redis admit rollback, max_cost wire, Gemini path/stream, retention RFC3339 (#511–#516).
- Docs honesty #517; release docs #525; tag + GitHub Release published; Milestone 49 closed.

## Earlier residual train

- v0.8.18–v0.8.38 narrative: root `CHANGELOG.md` + GitHub Releases (do not duplicate here).

## [2026-08-01] N9b-a: 倍率批量编辑入口（New API borrow 收口）

- N9b-a shipped: `PUT /api/models/rates` 批量更新 accounts.unit_cost + route_channels.weight（校验 >= 0，空 body 400；写后 `routing.InvalidateCache` 立即生效）。
- Rates 总览页行内编辑：账号单价 / 通道权重 ✎ → input → 保存（Enter 提交 / Esc 取消），负值拒绝。
- 测试：后端 SQLite 3 例 + PG 奇偶 1 例；前端 vitest 4 例（编辑保存 / Enter 提交 / 负值拒绝 / Esc 取消）；全量 557 vitest + go vet/test 绿。
- N9b-b 正式关闭（unit_cost 不参与 estimated_cost，ratio-based 计费口径不变）；N9 系列全部收口。

## [2026-08-01] K1b: 路由匹配 canonical 化（all-api-hub borrow 收口）

- A eligibility: `routing/redirects.go` 进程内 per-account 注册表（canonical→actual + 反向索引，原子替换）；`ChannelSupportsRequestedModelWithRedirects` 在两个 eligibility 点启用——actual 名通道对 canonical 请求开放。
- B 转发改写: `ResolveActualModelForSelectedChannel` + redirect 步（路由 model_mapping 优先）→ `selected.ActualModel` → `swapModelInJSON` 出站体改写（既有归因/出站分离骨架）。
- C 计费归因: `upstream.go` 两处 `EstimateBillingCostFromUsage` 改归因名（canonical）；`proxy_logs.model_requested` 保持 canonical、`model_actual` 如实记录 actual——ratio 计费口径不变。
- 数据流: `service.ReloadRedirectRegistry` 启动加载（router.New）+ K1a 全部变更点（PUT/DELETE/generate/apply/同步生成）后重建。
- 测试: routing 3 例（注册表/eligibility/selector 集成）+ service 重载 1 例；全量 go vet/test 绿。
- K1 系列全部收口；deferred 清单清空（N9b-b 关闭 + K1b 落地）。

## [2026-08-01] A3: income vs outcome 余额分析（all-api-hub borrow 收官）

- `GET /api/stats/balance-income-outcome?days=&accountId=`：基于 A1 快照按会计恒等式 income - outcome = Δbalance 推导——outcome = max(0, Δbalance_used)，income = Δbalance + Δused；首日快照视为初始入账；只输出有快照的日（缺失日 ≠ 零活动）。
- Dashboard 新卡「余额流入 vs 消费」：VChart 分组柱（收入/消费 per day）+ 汇总（总收入/总消费/净，净值颜色区分）。
- 测试：后端恒等式/多账号聚合/PG 奇偶 3 例 + 前端 3 例；全量 560 vitest + go vet/test 绿。
- **all-api-hub borrow A1-J1 全部 14/14 立项项收官**（A3 为最后一项）；MASTER 第 5 条 backlog 状态同步。

## [2026-08-01] B1: 管理操作审计日志（sub2api/cliproxyapi borrow）

- 竞品对标收官：sub2api + cliproxyapi 系统勘察（决策文档 `sub2api-cliproxyapi-borrow-2026-08-01.md`）——17 项对照，10 项等价/非目标关闭（S1 额度探测≈RefreshBalance+G1、C1 冷却黑窗≈DB cooldown_until、C2 粘性≈stable_first、C4 别名池≈pattern+多通道 等），4 项 deferred（QPS WS / 周期额度 / 批量媒体 / 登录 2FA），1 项立项。
- B1 落地：`admin_audit_logs` 表（Table 34，actor=token sha256 前缀 8 位，永不存明文）+ `AuditMiddleware`（admin auth 后，POST/PUT/PATCH/DELETE 记录，GET 不记，best-effort 不阻断）+ `GET /api/admin/audit-logs`（method/path 过滤 + limit）+ Settings「管理操作审计」页（方法 badge/状态着色/actor/IP/request_id）。
- 测试：中间件 3 例（写记录/读跳过/status 捕获）+ 端点过滤 1 例 + nil-db noop 1 例 + 前端 2 例；全量 562 vitest + go vet/test + docs 卫生绿。

## [2026-08-01] B2: 实时 QPS 运维 WS + Dashboard 实时面板（sub2api borrow）

- `handler/shared/realtime.go`：1s 粒度 300s 环形缓冲（总/成功两个 atomic 数组），
  `ObserveProxyOutcome`（统一终态观测点）接入，热路径两个原子加，零锁
- `GET /api/admin/ops/ws?token=`：browser WS 无法带 header，token 走 query +
  常量时间校验（与 AdminAuth 同一 AuthToken）；`coder/websocket` upgrade +
  1s tick 推 `{lifetime, points[300]}`；挂在 admin auth 组外
- Dashboard「实时流量」面板：当前 QPS + 近 1s 成功率（≥95% 绿）+ 累计请求 +
  60s 零依赖柱状 sparkline；断线指数退避自动重连（2s→15s 封顶）；
  多实例诚实声明「本实例流量」
- 测试：ring buffer 3 例（记录/零填充/并发）+ WS 端点 1 例（403 无/错 token、
  upgrade + 首帧）+ 前端 3 例（token query/空 token 不连/断线重连）；
  全量 565 vitest + go vet/test 绿

## [2026-08-01] VIS-1: 主题 preset（可选主色）+ 补齐测试

- `data-accent` 属性（blue 默认 / indigo 原版亲和 Material Indigo / teal 冷静运维 Material Teal）× light/dark 双套 `--color-primary` 族覆盖（tokens.css，衍生 token 全联动：info/focus-ring/gradient）
- themeBootstrap `resolveInitialAccent` + `THEME_ACCENT_KEY`；index.html FOUC inline 同步（防 flash）；App 主题菜单加 3 色点切换（localStorage + 立即应用）
- 修复：Dashboard 新卡（IncomeOutcomeChart）导致 dashboard.site-speed-button 测试 api mock 缺失 → 补 mock；App 测试环境 documentElement mock 缺 removeAttribute → 组件防御
- 测试：themeBootstrap 2 例 + 全量 567 vitest 绿

## [2026-08-01] NAV-1: first-run 侧栏渐进披露（ui-original-parity 收官）

- 无站点（first-run，App 挂载轻量 getSites 判定）时侧栏只强调核心导航（仪表盘/站点管理/连接管理/设置），其余折叠到「更多功能」区（desktop 点击展开/收起，mobile 归组）——降 onboarding 噪音；API 失败或已有站点时保持全量导航（不误折叠）
- 修复：App 渲染类测试 api mock 缺 getSites → 补 mock（mobile-layout/sidebar-mobile）
- 测试：NAV-1 折叠断言 1 例 + 全量 568 vitest 绿
- **ui-original-parity 全部 UI 待办收官**（VIS-1 + NAV-1 + 此前 CONSOLE-1/MOCK-NAV）

## [2026-08-01] 综合 review 修复：11 项核实缺陷（双 agent 对抗审查）

**后端 5 项**：
1. K1b 计费口径统一——`recordUpstreamSuccess` 第三处计费路径改归因名（此前 channel 累计成本按 actual 名查倍率，与 proxy_logs 分叉）
2. redirects 反向索引确定性——Go map 迭代随机导致「首个命中」不稳定，改 canonical 字典序最小
3. ReloadRedirectRegistry 补 `rows.Err()`——中途断连不再换入截断注册表
4. A3 退款场景——used 回退保留负 outcome（钳 0 破坏恒等式 income-outcome=Δbalance），新增退款恒等式测试
5. B1 审计——panic 路径补记（中间件内 recover 记 500 后 re-panic）+ statusRecorder 首次写入为准 + Write 隐式 200

**前端 6 项**：
6. AuditLogsSection 提交态触发（Enter/查询按钮）+ 请求序号丢弃过期响应（原来每 keystroke 一发）
7. RealtimeOpsPanel 连续失败 5 次停止重连（403/token 轮换不再无限重试）
8. NAV-1 expanded 时隐藏更多功能区（防侧栏项重复）
9. firstRun 探测加 authed 门控（登录前 401 → false 导致整个首次会话折叠失效）
10. Dashboard 挂载去重（load + poll 首轮双请求）
11. SnapshotExportButton 主色随 data-accent

**验证**：新增后端 2 测试（退款恒等式/panic 审计）+ 前端相关 9 例；全量 568 vitest + go vet/test 绿

## [2026-08-01] DENSE-1：有数据页表格密度（ui-original-parity 收官项）

- 默认表格密度 10px → 8px（tokens `--table-pad-y`，运营态「满」感；compact 6px 不变）
- 接通此前死开关 `html[data-density="compact"]`：themeBootstrap 加 `DENSITY_STORAGE_KEY`/`DensityMode`/`resolveInitialDensity`（未知回退 comfortable）
- 主题菜单加「表格密度」舒适/紧凑切换（与 VIS-1 主题色同构：data-density 属性 + localStorage 持久化，documentElement 双 guard 兼容测试渲染器）
- 测试：resolveInitialDensity 2 例 + App 集成 1 例（切换闭环 + 持久化断言，fixture 补 removeAttribute）

## [2026-08-01] i18n 完整性门禁 + focus-trap 测试健壮性

- 工程巡检发现 10 条 t() 文案在 EN 严格模式下显示 `Untranslated`（VIS-1 主题色 3 条 / DENSE-1 密度 5 条 / NAV-1 更多功能 2 条）——补 zhToEn 精确翻译
- 新增 `web/i18n.coverage.test.ts` 门禁：扫描全部 `t('中文')` 字面量，断言 EN 翻译无中文残留/非 Untranslated（防后续 wave 漏补字典）
- useFocusTrap 两处 jsdom 防御（listFocusable 缺 querySelectorAll / focusInitial 缺 hasAttribute 时跳过）——消除全量测试偶发 Uncaught Exception（tokens.edit-and-select 时序竞争）

**验证**：572 vitest（174 文件）零 Unhandled · typecheck exit=0 · SPA rebuild

## [2026-08-01] JSX 硬编码中文全量补译（181 条）+ 门禁盲区修复

- 扩展审计发现 i18n 门禁盲区：t() 门禁只覆盖包裹调用，**未包 t() 的 JSX 可见中文**（属性 + 文本节点）靠运行时 MutationObserver 兜底，其中 **181 条 EN 模式显示 `Untranslated`**（Dashboard 指标卡/设置表单/下游密钥/调试追踪/迁移工具/通知渠道等全 UI 面）
- zhToEn 主字典补 181 条精确翻译（含产品名官方名：飞书→Feishu、钉钉→DingTalk、企业微信→WeCom）
- `i18n.coverage.test.ts` 门禁升级：新增第二用例「raw JSX Chinese is covered by the dictionary」——扫描所有 .tsx 的属性/文本节点中文，断言 EN 无残留（防未来硬编码中文漏补字典）

**验证**：573 vitest（174 文件）零 Unhandled · typecheck exit=0 · SPA rebuild

## [2026-08-01] i18n 第三面：JSX 插值碎片补译 + toast 面验证

- 插值行文本节点（React 拆分为独立片段，如「确认删除 {n} 个连接吗？」拆出「个连接吗？」）补 4 条碎片翻译（个连接吗？/获取。/访问/之类的参数。）——此前 EN 下残留中文
- toast/confirm/alert 直接调用的 91 条中文审计：全部已覆盖（无 Untranslated）——此面干净
- `i18n.coverage.test.ts` 加第三用例「interpolated JSX text fragments」——插值片段纳入门禁

**验证**：574 vitest（174 文件）零 Unhandled · typecheck exit=0 · SPA rebuild

## [2026-08-01] canvas 快照 EN 化（快照 PNG 不再硬编码中文）

- `drawSnapshotCanvas` 7 处 canvas 文案包 `tr()`（canvas 文本不在 DOM，MutationObserver 无法到达）：MetAPI 网关快照 / 生成时间：/ 总余额 / 今日消耗 / 24h 请求 / 24h 成功率 / 24h Token / 活跃账号 / 站点消耗 Top / 暂无站点消耗数据 / footer
- zhToEn 补 11 条 canvas 文案（此前全 MISSING）
- 效果：EN 模式下导出的快照 PNG 全英文；zh 模式 tr() 原样返回，行为不变

**验证**：574 vitest（174 文件）零 Unhandled · typecheck exit=0 · SPA rebuild

## [2026-08-01] VChart 图表 EN 化（canvas 图例/tooltip + 第四门禁）

- 8 个图表组件审计：VChart spec 中的系列名/图例/tooltip key 渲染到 canvas（MutationObserver 够不着）——6 组件 canvas 字符串包 `tr()`（收入/消费/余额/成本/请求/占比/平均延迟/延迟/请求数）；DownstreamKeyTrendChart 的 tooltip key 惰性求值（模块级常量不能模块加载时 tr()）
- 模块级 `METRIC_OPTIONS` 常量（SiteTrend/DownstreamKeyTrend）label 仅进 JSX DOM——MutationObserver 兜底，无需改
- zhToEn 补 36 条（chart 系列/空态/说明句/碎片：总成本/次请求/余额分布 + 单位词：秒/分钟/小时/天 + About/监控 spec 字面量 12 条）
- `i18n.coverage.test.ts` 第四用例「chart spec literals」（key/type/label/metric/title 对象字面量）——canvas 面纳入门禁

**验证**：575 vitest（174 文件）零 Unhandled · typecheck exit=0 · SPA rebuild

## [2026-08-01] README_EN 门面同步 + vitest 噪音清零

- README_EN.md 三处过时修正：unreleased 提示覆盖 14 项 borrow + i18n 收官（此前停在 parity KEYS/WS）；「5-channel notifications」→ 9 渠道（含 Feishu/DingTalk/WeCom/ntfy + 审计 + 实时面板）；标题「AI API中转站」→ resellers
- vitest.setup.ts stub window.scrollTo/scrollBy——全量跑 18 条 jsdom "Not implemented" stderr 噪音清零（输出干净，失败易查）

**验证**：575 vitest（174 文件）零 Unhandled + 零 Not implemented 噪音

## [2026-08-01] CI 红修复：三路失败全根因修复

**背景**：健康巡检发现 GitHub Actions 最近 5 次 push 全红（CI frontend / UI visual / CD release-gate）——本地全绿远程红的经典环境差异。

**① CI frontend（Node 25）**：4 个 Dashboard 测试失败——根因双因素：
- Node 25 实验性全局 `localStorage`（`--localstorage-file` 无效路径时存在但缺 getItem）与 jsdom 冲突 → RealtimeOpsPanel 的 `getAuthToken(localStorage)` 崩；vitest.setup.ts 加兼容（globalThis.localStorage 缺 getItem 时替换为 window.localStorage）
- React.lazy 图表组件在测试中**时序性**加载完成（Node 25 模块解析更快）→ IncomeOutcomeChart 挂载调 `api.getBalanceIncomeOutcome` 而 apiMock 缺该方法；testApiCompat 补 getBalanceIncomeOutcome 空 fixture + performance-card/dashboardHookOrder 的 hoisted mock 补字段（本地 Node 24 lazy 从不完成所以一直没暴露——预存 flaky）

**② CD release-gate（PG 集成）**：`TestModelRates_UpdateBatchPostgres` + `TestStats_PostgresBalanceIncomeOutcome` 失败——测试 fixture 给 boolean 列 checkin_enabled 插整数 0/1（SQLite 宽松接受、PG 严格 42804 拒绝）；seedRateFixture/seedIncomeOutcomeFixture 改 SQL 标准 FALSE/TRUE；stats_test.go 5 处字面量 0 防御性改 FALSE；其余 PG 变体（usage/checkin/downstream_keys）核实已做方言处理或 Go bool 绑定

**③ UI visual（Playwright 基线）**：toHaveScreenshot diff——DENSE-1 表格密度（10px→8px）是预期视觉变化，待更新基线（--update-snapshots）

**验证**：go vet + handler/admin + scheduler 全绿 · 575 vitest 零 Unhandled · typecheck exit=0

## [2026-08-01] CI 修复第三轮：resolveStorage 最底层防御（Node 25 坏 localStorage 根治）

- 前两轮修复后 Dashboard 3 文件与 RealtimeOpsPanel 单测交替失败——根因：`resolveStorage` 在 storage 为 null 时 fallback 回裸 `localStorage`（Node 25 实验性全局、--localstorage-file 无效时 getItem 缺失）——RealtimeOpsPanel 防御传 null 反而把坏对象捞回 getAuthToken
- 治本：`resolveStorage` 验证 `localStorage.getItem` 是函数才 fallback——getAuthToken/persistAuthSession/clearAuthSession 全链路对坏全局免疫
- 双保险保留：RealtimeOpsPanel 防御（坏对象传 null）+ resolveStorage 验证
- 验证：12 相关测试全过 · typecheck exit=0 · SPA rebuild

## [2026-08-01] CI 全绿收官（三轮迭代）+ CD ghcr 权限问题留待管理员

**最终状态**：
- ✅ **CI**（frontend）：success——Node 25 三层防御（vitest.setup global 替换 + RealtimeOpsPanel 坏对象传 null + **resolveStorage 验证 getItem**——最后这层是根治：storage=null fallback 裸 localStorage 会把坏全局捞回 getAuthToken）+ lazy 时序 mock 补齐
- ✅ **UI visual + UX e2e**：success——linux 基线更新（DENSE-1 密度变化：页高 3919→3874px；流程：test:e2e 命令内 --update-snapshots → artifact 下载 → commit → 恢复普通命令）
- ⚠️ **CD**：release-gate（PG 集成测试）通过 ✓；失败在 build-and-push `ghcr.io/tokendancelab/metapi-go:latest denied: permission_denied: The requested installation does not exist`——仓库/组织 token 对 ghcr 的权限配置问题，**非代码缺陷，需管理员处理**

## [2026-08-01] SHOT-1 空库截图重录（ui-original-parity 100% 收官）

- 空库环境 = 本地单进程（后端 embed SPA 同源）——无需外部环境，`METAPI_UI_AUTH_TOKEN` + `METAPI_UI_SHOT_BASE` 直拍
- 全套重录：page-dashboard/sites/settings + shell-* + gallery + login（light/dark，1440x900）
- 截图反映当前 tip：first-run 折叠侧栏（NAV-1）、密度 8px（DENSE-1）、主题 preset（VIS-1）
- ui-original-parity 文档 SHOT-1 标 done + 头部「已过期」标注修正——**全部待办收官**
