# Changelog

All notable changes to MetAPI-Go will be documented in this file.

格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

## [Unreleased]

### Fixed — 今日指标真值与签到奖励（2026-08-01）
- Dashboard 与每日总结复用同一本地日界线聚合，真实返回 `todayCheckin` / `todayReward`，不再固定伪造 `todayReward=0`
- 修复 nullable DB `*string` 签到奖励无法解析；奖励源不完整时标记 `partial/source_partial`，Dashboard 显示 `—`，每日通知标注「部分可观测」
- Dashboard 核心 SQL 失败改为 HTTP 500 + 结构化日志，不再以 HTTP 200 和零值假绿；禁用站点不再污染余额/流量聚合
- 修复站点可用性 LEFT JOIN 空行被计为失败，以及本地日结束边界丢失最后 1 秒
- `/api/accounts` 每行返回 per-account 今日奖励/支出真值（`todayReward` / `todaySpend` / `todayRewardStatus` 等），无行账号为真实 0；Accounts 页不再渲染 `+0.00 / -0.00` 假零，指标不可观测时显示 `—`
- 新增 `service/daily.CollectPerAccountTodayMetrics`，与 Dashboard 同一日界线与奖励解析语义；legacy TEXT 时间规范化提取为公共 SQL 片段，消除查询间重复

### Fixed — GHCR 所有权与 CD 发布（2026-08-01）
- 自动构建镜像改为 `ghcr.io/deliciousbuding/metapi-go`，与源码仓所有者一致，避免个人仓 `GITHUB_TOKEN` 跨所有者写入未关联的 TokenDanceLab 包时失败
- 生产 hk3 仍保持旧 TokenDanceLab GHCR v0.8.45 pin，本变更仅修复后续镜像发布所有权，不触发生产迁移

### Changed — 本地运行与 OAuth 维护成本收敛（2026-08-01）
- Windows 未设置 `HOST` 时默认监听 `127.0.0.1`，避免临时构建路径反复触发入站防火墙提示；Linux/macOS 保持 `0.0.0.0`，容器显式固定 `HOST=0.0.0.0`
- 新增 `scripts/windows-firewall-maintenance.ps1`，只审计/清理已丢失、临时目录或旧 checkout 下的 MetAPI 入站规则
- Windows pre-push race 门禁自动转入 WSL 执行，避免原生 ThreadSanitizer 地址空间分配失败导致误报或人工 `--no-verify`
- 删除永久占用 9844–9846 的旧 OAuth loopback scheduler；provider callback server 改为按 flow 懒启动、由 app shutdown 统一回收，端口不可用时保留手工 callback/SSH tunnel 路径

### Fixed — SQLite OAuth refresh 与真实数据态验收（2026-08-01）
- OAuth connection list / refresh scheduler 共享显式 account-site projection，移除 `SELECT a.*, s.*` 对 SQLite/sqlx 嵌套扫描的依赖
- EN/zh 18 路由验收改为严格 seeded data-state：账号、余额历史、日用量和代理日志缺一即失败，不再以 warning 继续产生假绿
- 补齐数据态暴露的 `Latency trend (last …)` 与 `Non-streaming` 翻译回归

### Changed — Charts 动画遵循 prefers-reduced-motion（2026-08-01）
- canvas 动画无法用 CSS 关闭 → `animation: !prefersReducedMotion()` 门控 8 图表（WCAG 2.3.3）；门禁防硬编码回归

### Fixed — Charts 轴/图例色 dark 模式不可读（2026-08-01）
- 根因：VChart canvas 不解析 CSS `var()` → 轴标签/图例静默回退默认深色，dark 主题深字深底
- `useChartColors()` JS 取色（getComputedStyle + data-theme 监听）——7 图表轴色 + 4 图例 label 全部解析具体色值
- 对比度达 WCAG AA：light 6.05:1 / dark 6.09:1；静态门禁防回归（fill/stroke 无 var() 残留）

### Changed — 通知可观测性与保存校验（2026-08-01）
- dispatch 每次派发留日志（`notify: dispatch ok/partial/failed`，含每失败渠道 + 错误截断 100 字符）——生产可回答「通知为什么没发」
- Settings 通知渠道保存校验：**启用但凭据空 → 拦截 + 列出缺失项**（9 渠道）——防配置缺陷静默丢失告警

### Added — 签到可靠性：错过窗口 catch-up（E1b，2026-08-01）
- **重启不漏签**：window/cron 模式下实例在当天触发时刻后重启 → 启动时检测「今日触发已过 + 今日未跑 + 存在启用账号」→ 立即补跑（租约保护、幂等无双签）——签到系统核心可靠性
- `scheduler/checkin_catchup.go` 纯函数判定 + 8 测试

### Changed — bundle 拆分（2026-08-01）
- manualChunks 拆 react-vendor（react/react-dom/router 独立缓存 chunk）——**index 461KB → 240KB**（-48%）
- vchart-vendor（2MB）确认异步-only（图表全 React.lazy，不阻塞首屏）——chunkSizeWarningLimit 2100 消除警告

### Added — metapi-migrate PG→SQLite 反向迁移（2026-08-01）
- 方向判定 + SQLite 方言 DDL 转换（BIGSERIAL→AUTOINCREMENT、BOOLEAN→INTEGER+DEFAULT 1/0、JSONB→TEXT 等）+ `?` 占位插入——备用能力（部署定案为保持 Azure PG，勿增实体）
- 单测 +3；sqlite→sqlite 全链路验证（18 表 + 数据 + checksum）

### Fixed — EN 验收扩 18 路由 + 中文标点断言（2026-08-01）
- **verify-en-pages.mjs 11 → 18 路由**：新增 /oauth /playground /tokens /site-announcements /about /settings/notify /settings/import-export（质量审计覆盖的深层面此前无 CI 验证）+ **中文标点断言**（EN 输出零 `：，。（）` 残留，质量审计成果持久化）
- **扩面抓 3 处真问题**：① NotificationSettings `当前配置: {key || '未设置'}` 在 code 容器内被 SKIP 豁免（Settings '当前：' 案例重演）——标签与 fallback 移出容器 + tr() 包裹；② `或 '/'或`/`无`/`当前配置: ` 4 键此前落在 zhToEn 对象外（顶层垃圾，translateText 查不到）——移回对象；③ 纯中文标点节点（`（`）被 shouldTranslateTextNode 无汉字过滤跳过——加 CJK_PUNCT_RE 检查
- 回归测试 +1（8 断言）；本地 18/18 clean

### Fixed — i18n 反向审计 + 插值片段质量（2026-08-01）
- **插值 JSX 片段输出质量**：`>text {expr} text<` 运行期片段的碎英文/缺词/粘词（'个，已禁用'→'items，Disabled'、'推荐模型：'→'RecommendedModel：'、'回退到 revision'→'revision' 等）清零——**51 键补译**（统计/迁移/同步行、JSON 导入提示、OAuth 维护说明、下次刷新/已选模型/目标 Session ID 等）
- **中文标点归一化**：短语替换后无汉字残留时也归一化中文标点（此前 '，'/'：' 泄漏进 EN 输出）——normalizePunctuationOnly 提取复用
- **EN 值无汉字静态门禁**：字典值含中文即 CI 红（反向审计 HAN-VALUE=0 固化）
- 回归测试 +3（片段精确键 14 断言 / 标点归一化 / EN 值静态扫描）

### Fixed — 门禁输出质量审计（2026-08-01）
- **门禁「无汉字」标准放行的碎英文垃圾清零**：精确键之外的短语替换/strict fallback 输出（'Startverify'/'AllEnabled'/'RemoveTag'/'Sites AddSuccess'/'Save Retry'/'Sign In( )' 等）经探针审计——**三批 534 键补译**（通知渠道 Webhook/Bark/ServerChan/Telegram/SMTP、OAuth 管理 JSON 导入/SSH 隧道/路由池、调试追踪、公告/审计日志/重置系统/批量测活门槛、模型映射/倍率总览/路由高级参数 Codex WS/会话并发/首字超时/冷却上限、站点主站点 URL 校验/API 地址池/延迟阈值/品牌屏蔽/白名单、账号令牌创建/绑定/默认令牌/同步站点令牌等）
- 收敛：suspicious **586 → 72**（剩余全为多行 JSX 折叠探针误报，运行期单行已覆盖；真实浏览器验收 verify-en-pages/e2e 为最终裁决）

### Fixed — i18n EN 主界面实拍验收 11/11（2026-08-01）
- **单字键替换顺序 bug**：'启用'→'Enabled' 后 '中' 的汉字邻居变 'd'，边界检查误判 → 'EnabledZH'——单字键重构为基于原文邻界判断（先单字后多字）
- **调度任务面板**：后端 Job 用英文 eventType id（原把中文显示名当日 id 用）；model-probe note 中文改英文
- **Settings 代理令牌区**：'当前：' 移出 code 容器（SKIP 豁免面误伤 UI 标签）
- 补键：启用中/从未运行/可见密钥/筛选状态/已开启/暂无/进程内版本占位：/OPS_NOTE；修正 '签到'→'Check-in'
- 新增 `web/scripts/verify-en-pages.mjs`：11 条主路由 EN 模式无 Untranslated/无汉字残留验收

### Fixed — i18n e2e wave：对象字面量盲区（2026-08-01）
- **EN 模式 e2e 测试**（`web/e2e/i18n-en.spec.ts`，真实浏览器 MutationObserver 全链路）：登录页无汉字/无 Untranslated + design gallery 组件面 + 会话内切换语言——首跑即抓真 bug
- **第四类门禁盲区清零**：对象字面量值侧中文（`label: '站点公告'`、`site_notice: '…'`、option/状态映射）此前 attr/text/表达式三面全扫不到——生产侧栏「连接管理/OAuth 管理」等在 EN 显示 Untranslated；补键 212 条（字典达 ~700 键），门禁新增对象字面量值侧收集

### Fixed — i18n review wave（2026-08-01）
- **tr() 门禁盲区清零**：649 处 tr() 调用此前从不被扫描——SnapshotExportButton 按钮/toast 5 串 + 76 处 tr() 文案在 EN 模式显示 `Untranslated`；门禁同扫 t()/tr()，字典补 103 键
- **单字键不再拆碎词**：`'中'/'天'/'净'` 等单字键做子串替换曾把 `'导出中...'` 变 `'ZH...'`——改汉字边界匹配（孤立才替换），`'登录中...'/'模型同步中'/'立即导出到 WebDAV'` 等补精确键
- **en→zh 回切不再滞留英文**：zh 模式此前把英文值写回原文 map（WeakMap 污染、中文永久丢失）——现只恢复原文、不更新 map
- **用户数据豁免**：站点名/账号名/模型名/公告正文等中文数据在 EN 模式不再被剥离——新增 `data-i18n-skip` 豁免机制（Sites/Accounts/Models/公告容器 4 处标记）
- **chart tooltip key 强制 tr()**：SiteDistributionChart 补包；门禁升级为 raw `key: '中文'` 直接报错
- **插值 JSX 片段门禁**：按运行期片段逐个校验（此前按整串，运行期 React 拆片段后逐段翻译）——60 条片段键 + translateText trim-exact 查找（覆盖 JSX 空格节点）
- **表达式 placeholder 入门禁**：`placeholder={cond ? '中文' : …}` 此前完全逃逸扫描——现 tr() 包裹 + 门禁表达式分支
- **词典错误修正**：跳过→Skipped、重试→Retry、豆包→Doubao、路由不存在→Route not found、通道→Channel、错误→Error、代理→Proxy
- **门禁扫描器修复**：stripComments 不再截断 `https://` URL 字符串（行注释正则要求行首或前置空白）
- 全量 580 vitest + typecheck 双配置 + build:web 绿

### Added — i18n 全面收官（2026-08-01）
- **EN 模式全链路可读**：六波 wave 消除所有「EN 界面显示中文/Untranslated」——t() 字面量 10 条漏译补齐、裸 JSX 硬编码中文 181 条全量补译、插值文本碎片 4 条、canvas 快照 PNG 11 条 tr() 化、VChart 图表 spec（系列名/图例/tooltip）tr() 化 + 36 条补译、toast/confirm/alert 面审计全覆盖
- **四层 i18n 门禁**（`web/i18n.coverage.test.ts`）：t() 字面量 / 裸 JSX 属性+文本 / 插值片段 / chart spec 对象字面量——任何新中文文案漏补字典即 CI 红
- 字典 zhToEn 扩至 200+ 条（含产品名官方名 Feishu/DingTalk/WeCom、单位词、图表说明句）

### Added — UI 收官 + review 修复 + 密度（2026-08-01）
- **DENSE-1 表格密度**: 默认表格行高 10px → 8px（运营态「满」感）；主题菜单新增「表格密度」舒适/紧凑切换（接通既有 `html[data-density="compact"]` 开关：data-density 属性 + localStorage 持久化）
- **Review 11 项核实缺陷修复**（双 agent 对抗审查）: 后端——`recordUpstreamSuccess` 计费改归因名（K1b 口径统一，channel 成本不再按 actual 名查倍率）、redirects 反向索引字典序确定性（map 迭代随机不再影响 eligibility）、`ReloadRedirectRegistry` 补 `rows.Err()`、A3 退款保留负 outcome（会计恒等式始终成立）、B1 panic 路径补记 500 + statusRecorder 首次写入为准；前端——AuditLogsSection 提交态触发 + 请求序号丢弃过期响应、RealtimeOpsPanel 连续失败 5 次停止重连、NAV-1 expanded 隐藏更多功能区、firstRun 探测加 authed 门控、Dashboard 挂载去重、快照 PNG 主色随 data-accent
- **NAV-1 first-run 侧栏**: 无站点时侧栏只显示核心 onboarding 路径（仪表盘/站点管理/连接管理/设置），其余折叠「更多功能」（desktop 展开 toggle + mobile 归组）
- **VIS-1 主题 preset**: `data-accent` 3 预设（blue 默认 GCP / indigo 原版亲和 / teal 冷静）× light/dark 双套 --color-primary 族覆盖；主题菜单 3 色点切换 + FOUC 同步
- **B2 实时 QPS 运维面板**: 1s×300 环形缓冲（total/success）+ `GET /api/admin/ops/ws?token=` 每秒推流（browser WS 无法带 header，token 走 query 常量时间校验）+ Dashboard 实时流量面板（QPS/成功率/sparkline/指数退避重连）
- **B1 管理操作审计日志**: `admin_audit_logs` 表 + AuditMiddleware 记录 POST/PUT/PATCH/DELETE（actor = token sha256 前缀 8 位，永不存原文；panic 也补记 500）；Settings「审计日志」区（方法/路径/actor/IP/状态过滤 + 分页）
- **A3 余额流入 vs 消费**: `GET /api/stats/balance-income-outcome` — 会计恒等式 income - outcome = Δbalance 推导（首日视为初始入账；退款如实反映为负 outcome）；Dashboard「余额流入 vs 消费」分组柱卡
- **K1b 路由匹配 canonical 化**: per-account 进程内 redirect 注册表（canonical→actual + 字典序确定性反向索引）；actual 通道对 canonical 请求开放 eligibility；转发改写（swapModelInJSON 出站体）+ 计费归因名（proxy_logs model_requested=canonical / model_actual=actual）
- **N9b-a 倍率批量编辑**: `PUT /api/models/rates` 批量更新 accounts.unit_cost + route_channels.weight（校验 ≥0、写后路由缓存失效）+ 总览页行内编辑（✎ → input → 保存，Enter/Esc）；N9b-b 关闭（unit_cost 不参与 estimated_cost，ratio 计费口径不变）

### Added — New API borrow N9a (rate overview) + N8 assessment
- **N9a 倍率与权重总览**: `GET /api/models/rates` 只读聚合全部倍率面——账号 unit_cost + 通道权重足迹、通道 weight、站点 global_weight、下游 key_weight、模型 30 天观测成本 + 汇总；Settings「倍率与权重总览」区（只读表格）。评估文档 `docs/analysis/competitive/n8-n9-deferred-assessment-2026-08-01.md`
- **N8 关闭（架构等价）**: 多密钥轮询已由 route_channels 多行 + round_robin/weighted + 每通道冷却 + OAuth route unit 原生覆盖；实现渠道内多 key 会重复凭证模型——不立项

### Added — all-api-hub borrow K1a (model redirects)
- **K1a 模型重定向映射**: `model_name_redirects` 表（per-account 标准名 → 上游实际名，UNIQUE(account_id, canonical)）；同步后自动生成（匹配规则：精确 → 日期后缀 `-YYYYMMDD(-vN)` → 版本后缀，首个命中的实际名稳定保留、手动映射不被覆盖、幂等）；`GET/PUT/DELETE /api/model-redirects` + `POST generate`（单账号/全量）+ `POST apply {dryRun}`——dry-run 预览可修复的 `site_disabled_models`（canonical 被禁用但 actual 可用），确认后删除并记录 events；Settings「模型重定向映射」区（列表/生成/预览/确认修复/转手动/删除）。设计文档 `docs/analysis/competitive/k1-model-redirect-design-2026-08-01.md`；**K1b 路由匹配 canonical 化 deferred（M 级触及核心热路径，待拍板）**

### Added — all-api-hub borrow Wave D (tags + banners + snapshot)
- **I1 accounts/sites 全局标签系统**: `accounts.tags` / `sites.tags` JSON 数组列（AdditiveStep sc2_011）；`GET /api/tags` 全局索引（按使用量排序 + account/site 计数）；`PUT /api/accounts/{id}/tags` / `PUT /api/sites/{id}/tags`（去重校验写入）；Accounts/Sites 页彩色标签 chips（点击即过滤）、过滤 chips 行、共享 TagEditorDialog（快捷添加/删除/Enter 保存）
- **H1 产品级风险横幅**: `product_announcements` + `announcement_dismissals` 表（severity info/warning/critical + enabled + link；内容编辑重置 dismiss = 新 revision 重新展示）；`GET /api/announcements`（管理视图）/ `GET /api/announcements/active`（未关闭，critical 优先）；POST/PUT/DELETE + dismiss 端点；Dashboard 顶部 severity 配色横幅（dismiss × + 详情链接）；Settings「产品公告」手发 CRUD 区
- **J1 可分享看板快照 PNG**: Dashboard「导出快照」— 原生 canvas 绘制 1200x630 摘要卡（总余额/今日消耗/24h 请求/成功率/Token/活跃账号 + 站点消耗 Top5 + 生成时间戳）下载 `metapi-snapshot-YYYYMMDD.png`，零新依赖（toBlob 缺失时诚实报错）

### Added — all-api-hub borrow Wave C (analytics + verification)
- **A2 模型成本分布 + 延迟图表画廊**: `GET /api/stats/model-cost-distribution`（topN-with-Other 成本桶 + totals）、`GET /api/stats/latency-histogram`（双方言整数除法延迟桶）、`GET /api/stats/latency-trend`（每日 avg/max/first-byte + 成功率 + 有界降序采样 p95，超采样上限天数以 truncatedDays 诚实标记）；Dashboard「模型成本分布 / 延迟直方图 / 延迟趋势」三卡
- **G1 批量模型验证 + 验证历史**: 新 `model_verify_history` 表（per-row batch/status/latency/http_status/error_text）；`scheduler.ProbeBatch` 一次性验证（复用注入 probe executor + 路由健康记录，不碰账号租约）；`POST /api/models/verify-batch`（models/accountId 过滤 + limit）+ `GET /api/models/verify-history`；Models 页「批量验证」dialog（per-row 结果表 + 验证历史 tab）

### Added — all-api-hub borrow Wave B (scheduling / backup / observability)
- **E1 随机窗口调度模式**: checkin 支持 `window` 模式 — 启动/设置变更时在 `CHECKIN_WINDOW_START`~`END`（HH:mm）内随机生成每日 cron（负载扩散 + 反指纹）；`PUT /api/checkin/schedule` 接受 windowStart/windowEnd
- **F1 备份导入预览**: `POST /api/settings/backup/import/preview` 返回 per-table rows/toInsert/duplicates/skipped 计划且不写行；ImportExport confirm 前展示计划；顺带修复前端 `{data}` 包装与后端 `{tables}` 契约不匹配 bug（手动 JSON 粘贴导入此前恒 400）
- **C1 调度任务统一运行历史**: `GET /api/scheduler/status` 聚合 checkin/balance-refresh/model-probe/site-announcements/daily-summary/log-cleanup/usage-aggregation 的 last-run + 24h 活动；Dashboard「调度任务状态」面板

### Added — all-api-hub borrow Wave A (product surface)
- **A1 余额历史快照表 + 趋势图**: 新 `balance_history` 表（per UTC day per account，同日 UPSERT 覆盖）；`RefreshBalance` 成功路径自动写快照（best-effort，不阻断刷新）；`GET /api/stats/balance-history?accountId=&days=`；Dashboard「余额趋势」卡（跨账号聚合总余额近 30 天）
- **B1 需关注看板**: `GET /api/stats/attention` severity 排序深链项（expired accounts critical → low-balance <1.0 warning → disabled sites warning → 近 24h warning/error events）；Dashboard 顶部「需要关注」面板，点击直达对应页面
- **D1 per-task 通知 + 4 新渠道**: feishu/dingtalk（HMAC-SHA256 加签）/wecom/ntfy 四专用 channel（共享有界 client + SSRF 校验）；`notify_task_toggles` 按告警类型静音（token_expired / low_balance / proxy_all_failed，缺省全开向后兼容）；NotificationSettings 扩展渠道卡 + 静音行

### Added — original parity program (ex-Electron)
- **KEYS**: per-downstream-key weight (#547); site custom header override priority (#584); allow-list bind sites/credentials on downstream keys (#579)
- **WS-1 C1–C3**: Responses WebSocket via `coder/websocket` — upgrade + HTTP SSE bridge + multi-turn/quota (C2) + Codex upstream wss runtime with dial→HTTP fallback (C3); status `c3_codex_upstream_wss`
- **#514 multi-tier ctx**: same-model routes with different `context_length` pick tightest fit from request estimate; `LoadEnabledRoutes` honors `sort_order`
- **UC-1**: Update Center hide/external — Settings ops note + GHCR/Releases links; API residual 501; no invent `updateAvailable`
- **UI**: cloud-ops design family align (tokendance-design palette/shell density)
- OAuth token auto-refresh scheduler (#251): 60s interval, per-provider lead times (codex=5d, claude=4h, gemini-cli/antigravity=5min), singleflight dedup
- P12 scheduler spec updated with scheduler #13b (video retention) + #16 (OAuth refresh)

### Fixed / Honesty
- **vulncheck GO-2026-5970**: bump `golang.org/x/text` v0.38.0 → v0.39.0 (infinite loop on invalid input; reached via `store.DB.ExecContext` → `sql.DB.ExecContext` → `norm.Form.Properties/Span/Transform`). govulncheck clean.
- **CI regression fix**: `DownstreamKeyEditorModal` allow-credential list rendered `item.title`/`item.subtitle` (not on `DownstreamCredentialOption`); restored `item.label`/`item.detail` so `npm run typecheck:web` passes (b6bee3c #579 introduced the regression; caught by CI `frontend` job)
- **staticcheck SA5011**: `dispatchUpstream` #514 estimate dropped the now-redundant `if ctx != nil` guard (ctx is constructed non-nil by `PrepareCtx`, which returns a `SurfResult` on failure); comment documents the invariant so the lint cannot regress
- **dual-dialect encapsulation**: `store.DB` now exposes `ExecContext/QueryxContext/QueryRowxContext/GetContext/SelectContext` (mirroring the non-Context helpers, rebinding `?`→`$N` for PG internally); `app/proxy_upstream.go` and `handler/proxy/proxy_log.go` delegate to them, removing 4 manual `if db.Dialect == Postgres` branches from the business layer. Remaining dialect branches are true semantic splits (PG `RETURNING id` vs SQLite `LastInsertId`; advisory lock vs local mutex; `ON CONFLICT` vs `INSERT OR IGNORE`) and stay where the SQL diverges
- **docs SSOT sweep (fable fleet review)**: `docs/README.md` residual board post-v0.8.43/M50 → post-v0.8.45/M53, latest tag v0.8.44→v0.8.45, WS-1/UC-1 moved out of "still not product"; `docs/STATE.md` active milestone M52 UI-POLISH (closed, 0 open) → M53 REL-HONESTY; `docs/deployment.md`+`docs/migration.md` Go 1.26.4+→1.26.5+; `docs/analysis/project-overview.md` tagged historical (React 18/Vite 6/Go 1.22+ were pre-rewrite); `docs/analysis/residual-next-candidates.md` release links added v0.8.44/v0.8.45; `docs/log.md` header block moved to top; `docs/progress/MASTER.md` "this session" → absolute date
- **P0-585**: production/staging cascade procedure + dry-run/live probe script (`docs/analysis/p0585-production-e2e-procedure.md`, `scripts/p0585_cascade_probe.py`); inventory stays **partial** (#557)
- **P0-585 residual**: HTTP-path multi-channel 5xx storm + recover e2e (`e2e/e2e_p0585_cascade_test.go`); channel-scoped excludeSnapshots; inventory stays **partial** (no prod e2e flip)
- **P0-555 residual observability**: `metapi_stream_missing_usage_total` when stream requested `include_usage` but ended without usable tokens (plus OrphanLogs on projection)
- **P0-555 residual observability**: usage projection reports `OrphanLogs` + slog when proxy_logs lack site join (watermark advances; site buckets skip; still present-with-residual)
- **TPM admission estimate** (#495 residual polish): count Claude `system` + Gemini `contents` string leaves (parity with routing context estimate); still best-effort chars/4, not a tokenizer
- `IsManagedSub2ApiTokenDue` no longer returns true unconditionally — now checks real 300s lead window (was always-true stub causing unnecessary refresh passes)
- **P0-555 residual**: fold OpenAI media `*_tokens_details` text/image/audio leaves when top-level usage missing (no double-count / no invent on zeros); still present-with-residual (multi-instance lag)
- **C4 docs**: WS multi-turn process-local sticky honesty only (STICKY-B deferred)

### Not yet (gates)
- **P0-585** remains **partial** until production e2e (unit load-proof does not flip present)
- **OPS-PIN** 0.8.45 requires admin auth + soak (do not auto pin)

### Engineering — codeg 对标 optimization wave (2026-07-30)
- **Package boundary CI assertion**: `docs/package_boundary_test.go` encodes `BACKEND.md` §2.3 eight hard rules + §5 documented exceptions as a `go test` machine gate (codeg `test.yml:134-148` grep-gate analogue). The test surfaced a stale `scheduler → handler/shared` edge (B1 §6 "scheduler Pass" was outdated) → recorded as §5.11 exception + follow-up (extract metrics registry to an `app/observability` leaf), not silently relaxed.
- **neat-freak archive**: 7 one-shot analysis files → `docs/archives/2026-07/` (p4-account-verify / p4-settings-proxy-test / p4-token-adapter-wiring / ui-score-2026-07-19 / ui-score-shell-2026-07-19 / ui-score-shell-mock-2026-07-19 / ui-pm-empty-state-2026-07-19); `p4-admin-test-routes.md` + `ui-score-pages-2026-07-19.md` kept live (active refs). New `docs/archives/2026-07/README.md` records archive policy.
- **Monolith split**: `handler/admin/stats.go` 1544 → 826 lines; extracted `stats_helpers.go` (pure value-coercion/query/time helpers) + `stats_marketplace.go` (marketplace/token-candidate/without-token/missing-group/endpoint-type builders). Behavior-neutral (same package, same exported surface).
- **Docs**: `package-boundaries.md` §6 marked machine-enforced + §5.11 new exception + §7#8 DONE; new `engineering-optimization-2026-07-30.md` synthesis.

### Product parity & New API 借鉴 — decision input (2026-07-30, docs-only)
- **Synthesis**: `docs/analysis/product-parity-and-newapi-borrow-2026-07-30.md` — metapi-ts original parity audit (cross-verified) + New API borrow research (cross-verified, 1 false positive removed). **Decision input only — no product code this round** (hard gate: needs Issue discussion / user sign-off before implementation).
- **Original parity**: Go lost no TS README head feature; 14 platform adapters TS=Go aligned; Go exceeds TS on slow-req / heatmap / brand bucket / cross-site price compare. G1 balance-low alert = real gap but TS also only counts in daily summary (README promise unfulfilled on both sides) — metapi-go can do better.
- **Borrow shortlist (9 confirmed)**: N1 downstream-key IP allowlist/blocklist (P0 security gap) · N2 public/downstream-visible pricing page (P1 aggregator differentiator) · N3 reasoning suffix + thinking_to_content · N4 per-channel test button · N5 downstream-key consumption dashboard · N6 log CSV export · N7 prompt-cache-ratio config · N8 single-channel multi-key rotation · N9 model/group multiplier admin UI. Out-of-scope: multi-user/pay/redemption/invite/subscription (conflicts with aggregator positioning).

### UIUX wave 1 — defensive + honesty + a11y (New API frontend benchmark, 2026-07-30)
- **RouteErrorBoundary** (`web/components/RouteErrorBoundary.tsx`): new class-component boundary wrapping all lazy `<Routes>` in `App.tsx`. Previously zero error boundaries existed — a single thrown render error in any lazy page blank-screened the entire SPA. Mirrors New API per-route `errorComponent` without adopting TanStack. Keyed on `location.pathname` so navigating away auto-clears the boundary; fallback shows message + retry. Test: `RouteErrorBoundary.test.tsx`.
- **SearchModal real keyboard navigation** (`web/components/SearchModal.tsx`): the footer advertised `↑↓`/`Enter` hints but no handler existed — a honesty bug. Refactored the six result sections into a single ordered `flat` list with a global `activeIndex`; `ArrowDown`/`ArrowUp` move + wrap, `Enter` opens the active item, `scrollIntoView` tracks the active row. Input is now a `combobox` with `aria-activedescendant`/`aria-expanded`, results are a `role="listbox"`, items carry `aria-selected`. Test extended in `search-modal.results.test.tsx`.
- **Toast a11y** (`web/components/Toast.tsx`): container is `role="status" aria-live="polite" aria-atomic="true"`; error toasts are `role="alert"` (assertive), success/info stay `status`. Test: `toast.a11y.test.tsx`.
- **Docs**: `analysis/uiux-newapi-borrow-2026-07-30.md` synthesis (metapi-go web audit + New API 12 borrowable patterns B1–B12); STATE/README/log pointers.
- Pre-push: `npm run typecheck` clean; vitest 515/515 (1 non-failing jsdom focus-trap teardown flake in `tokens.edit-and-select.test.tsx`, pre-existing, unrelated — `useFocusTrap.ts` untouched).

### UIUX wave 2/3 + visual polish (New API frontend benchmark, 2026-07-31)
- **design-system triad** (`28ce05a`): new `ErrorState` (EmptyState tone=danger + alert icon + auto Retry, role=alert) + `LoadingState` (block: N skeleton lines; inline: spinner+label; both role=status aria-live=polite). Completes Empty/Loading/Error (New API B1). Adopted LoadingState in ProgramLogs. Test: `design-system/states.test.tsx` (4 cases).
- **ImportExport i18n** (`3cd2525`): the worst-i18n'd page (884 lines, 1 `tr()` call) → all user-facing strings wrapped in `tr()` (toast/confirm/dynamic-join + module-level option labels); EN mappings added to `i18n.supplement.ts`.
- **Models → Playground quick-launch** (`df94e72`): Models card/table-row action button → `/playground?model=<name>`; ModelTester reads `?model=` and pre-fills the model selector (takes precedence over restored session). Fixed ModelTester test missing `MemoryRouter` (`useSearchParams` requires Router context).
- **ProxyLogs date-range presets** (`2da7401`): filter area gains 15m / 1h / Today / 7d preset buttons that set the datetime range and trigger load. EN mappings added.
- **Visual motion** (`a352ab4`): Models card grid gains `animate-slide-up stagger-N` (mirrors New API `CARD_STAGGER_VARIANTS`, capped at stagger-8 = 0.32s) + `model-card:hover` translateY(-2px) lift micro-interaction. `page-enter` intentionally left opacity-only — a transform on the page wrapper would disable descendant sticky table headers.

### N1 — per-downstream-key IP allowlist/blocklist (New API borrow, 2026-07-31)
- **Backend** (`9e9cad1`): `downstream_api_keys` + `ip_allowlist` / `ip_blocklist` TEXT NULL (base DDL + AdditiveStep `sc2_010`). `auth.CheckDownstreamKeyIP` enforces at the `ProxyAuth` edge after managed-key auth: blocklist wins, allowlist non-empty requires a match, both empty = unrestricted. Reuses `auth/admin.go` `parseAllowlist`/`isIPAllowed` (IPv4-mapped-IPv6 + `::1` normalization, invalid entries silently skipped). `AuthorizeDownstreamToken` signature unchanged (no test ripple). Admin CRUD carries the columns on create + partial update (empty string clears to NULL).
- **UI** (`d4633f1`): DownstreamKey editor gains IP allowlist + IP blocklist textareas (after the proxy field) with help text; buildEditorForm hydrates + submit payload carries `ipAllowlist`/`ipBlocklist` (empty → null). Tests: `auth/downstream_ip_test` (split + check, block-wins, CIDR, IPv4-mapped, loopback), `handler/admin TestDownstreamKeysIPAllowBlockListCRUD` (create round-trip + update clears), `DownstreamKeys.test` IP-field render + save.
- Closes the P0 security gap from `docs/analysis/product-parity-and-newapi-borrow-2026-07-30.md` §N1 (aggregator exposed managed keys publicly with no per-key IP restriction).

### N2 / N3 / N4 / N5 / N6 / G1 — productization batch (New API borrow, 2026-07-31)
- **N2 downstream-key pricing catalog** (`d22b808`): new `/v1/pricing` (+ `/v1/models/price-compare` alias) mounted in the /v1 ProxyAuth group — a downstream consumer with a managed key can query effective cross-site model pricing (reuses admin.modelPriceCompare; no separate catalog to drift). Not world-public (no anonymous business-cost leak). Test: router 401-without-key.
- **N3 reasoning suffix** (`9c056a4`): `ParseReasoningSuffix` strips `-thinking`/`-high`/`-medium`/`-low` so routing matches the base model; OpenAI surfaces inject `reasoning_effort` (client value not overwritten) + re-serialize RawBody. Non-OpenAI dialects strip for routing only (cross-dialect injection deferred).
- **N4 inline speed-test** (`6ed798d`): Sites list row gains an inline 测速 button (client-side `fetch(site.url/v1/models)` no-cors, same as Dashboard) — no more open-editor-to-test.
- **N5 consumption distribution** (`05e10a7`): new `ConsumptionDistribution` component — top-10 cross-key breakdown by usedCost / usedRequests (toggle) with bars + % share, aggregated from visible keys; collapsible panel above the DownstreamKeys list. Pure frontend.
- **N6 CSV export** (`18e9066`): CheckinLog / ProgramLogs / DownstreamKeys gain 导出 CSV (reuses csvExport helper; DownstreamKeys never exports the raw key, only keyMasked + accounting).
- **G1 real-time low-balance alert** (`09a619e`): `alert.ReportLowBalance` fires when a balance refresh observes balance < 1.0 (TS parity threshold), deduped per account per 24h via the events table. Hooked into `balance.RefreshBalance` success path so scheduled + manual refresh both land it in real-time — TS only counted lowBalanceAccounts in a daily summary (promise unfulfilled); metapi-go does better.

### N7 — admin-configurable prompt-cache ratio fallbacks (2026-07-31)
- **routing** (`10384ee`): `DefaultCacheRatio` / `ClaudeCacheRatio` (and creation counterparts) become runtime-overridable via `atomic.Pointer` + `SetCacheRatioDefaults`. Non-positive/NaN/Inf overrides reset to the code default. Wired end-to-end: config fields → `store.ApplyRuntimeSettings` (cache_ratio_default/claude settings) → `app.ApplyCacheRatioOverrides` at boot → `handler/admin` settings getRuntime exposes effective values / updateRuntime persists + applies immediately. Tests: override applies, bad values reset, explicit per-row ratio still wins.

### N8 / N9 — deferred (M-size, scheduler/billing core)
- **N8 single-channel multi-key rotation**: requires a `route_channels` multi-key schema + a change to `routing/selector.go` token resolution (pick round-robin from a list) + candidate/loading changes. Touches the load-balancing selection core; deferred to a dedicated session with full algorithm-test coverage rather than a rushed tail.
- **N9 model/group multiplier admin UI**: new ratio table + admin CRUD + Settings UI. M-size; deferred alongside N8.
- Both remain tracked in `docs/analysis/product-parity-and-newapi-borrow-2026-07-30.md` §4 (P3).

## [v0.8.45] — 2026-07-20

### Fixed / Reliability
- NewAPI user-id discovery regex is RE2-safe: drop PCRE `(?!\d)` lookaheads that panic under Go `regexp` during balance/user-id extract (production Exited under v0.8.44)
- User-id probe loops honor context cancel; unreachable-host adapter tests use closed-listener URLs (not `:1` blackhole) so race suite finishes under pre-push timeout

### UI (M51–M52 + console density, was unreleased on tip)
- M51–M52 polish, first-run, console density / system fonts / hi-res content column, gallery linux baselines (GHA SSOT)

## [v0.8.44] — 2026-07-19

### Fixed / Reliability
- PostgreSQL pool profiles (`DB_PROFILE`/`METAPI_DB_PROFILE`: `shared-tiny` 2/1, `normal` 10/3 default, `dedicated` 20/5); explicit `DB_MAX_*` always override (#531)
- Inject `application_name=metapi-<hostname>` (or `DB_APPLICATION_NAME`) when DSN omits it for `pg_stat_activity` attribution (#531)
- Scheduler advisory-lease: MaxOpen≤2 uses process-local lease; SQLSTATE 53300 / too-many-connections exponential backoff + log rate-limit + force-local after repeated pressure (#531)
- Metrics: `metapi_db_connections_in_use`, `metapi_db_conn_errors_total` alongside open gauge (#531)

### Docs
- Pool budget design + operator recipes: `docs/analysis/db-pool-budget.md`; deployment/README/.env.example/compose aligned (#531)

## [v0.8.43] — 2026-07-19

### Tests / Honesty
- P0-585 multi-channel load-proof honesty: 5xx storm channel-scoped exclude + MaxAttempts bound; 429 same-channel budget policy documented (#527)
- P0-555 Gemini stream usageMetadata later-wins + empty/zero usage does-not-invent (#530)

### Ops
- us1 cold standby compose pin + image pull to **0.8.42+** series with pool/credential env wiring (#528)

### Docs
- M50 residual honesty / high-value-next / MASTER after landings (#529)

## [v0.8.42] — 2026-07-18

### Fixed
- Config validation accepts default 5-field cron expressions (`0 8 * * *`, etc.) by auto-normalising to 6-field before parse (parity with scheduler)

## [v0.8.41] — 2026-07-18

### Fixed
- Move `proxy_logs_request_id_created_at_idx` out of base bootstrap indexes so upgrades from pre-request_id schemas (e.g. v0.6.5) do not fail before additive `sc2_004` runs

## [v0.8.40] — 2026-07-18

### Fixed
- Explicit PostgreSQL pool budget: configurable `DB_MAX_OPEN_CONNS` / `DB_MAX_IDLE_CONNS` / lifetime/idle-time via env; documented in deployment guide + compose (#526)

### Docs
- Split STATE/MASTER/LOG progress roles; codify docs governance

## [v0.8.39] — 2026-07-18

### Fixed
- Round-robin `consecutiveFailCount` no longer double-increments (threshold 3 restored) (#511 / #519)
- Managed-key `used_requests` not burned on RPM/TPM admission 429 (`Allow` before consume) (#512 / #522)
- Shared Redis RPM/TPM admission rolls back window counters on deny (fail-open preserved) (#513 / #518)
- Wire `RecordManagedKeyCostUsage` on proxy success so `max_cost` advances (#514 / #520)
- Gemini path model when body omits model; `streamGenerateContent` forces stream (#515 / #523)
- Retention cutoffs use RFC3339 comparable to `created_at` (same-day prune fixed) (#516 / #521)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 49 post-product landings; REL-RR-FAILCOUNT · REL-USED-REQ-429 · REL-REDIS-ADMIT-ROLLBACK · REL-MAX-COST-WIRE · REL-GEMINI-PATH-STREAM · REL-RETENTION-RFC3339 present · board #511–#517 closed (#517 / residual honesty PR)
- Keep **P0-585 partial** (load-proof still required) and **P0-555 present-with-residual**; WS-1 / STICKY-B / UC-1 residual

## [v0.8.38] — 2026-07-18

### Docs / Honesty
- Public Redis claims: optional REDIS_URL for multi-instance RPM/TPM admission (sharedcount fail-open); sticky still process-local residual (#503 / #507)
- ghcr public badge bumped to v0.8.37 series; residual inventory latest-release sequencing (#504 / #505 / #508)
- Residual inventory + MASTER for Milestone 48 post-product landings; DOCS-REDIS-TRUTH + DOCS-DOCKER-BADGE + DOCS-RESIDUAL-LATEST present · board #503–#506 closed (#506 / #509)

## [v0.8.37] — 2026-07-18

### Docs
- Align README/README_EN stack badges to Go 1.26.5 + React 19 + Vite 8 (#494 / #498)

### Reliability
- Best-effort TPM admission estimate when maxTPM is set (no invent; empty body skips) (#495 / #500)

### Tests / Honesty
- P0-585 credential usage-limit multi-channel cool honesty tests (intentional shared-key scope; cascade still partial) (#496 / #499)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 47 post-product landings; DOCS-STACK-TRUTH + REL-TPM-ESTIMATE + REL-CRED-USAGE-HONESTY present · board #494–#497 closed (#497 / #501)

## [v0.8.36] — 2026-07-18

### Security
- Clear meta_monitor_auth cookie on successful admin AuthToken change (defense-in-depth; HMAC already invalidates) (#484 / #489)

### UI
- Tokenize residual monitor-hint / route-enable-disabled / stat-summary / topbar brand hex to design tokens (#485 / #490)

### Tests / Observability
- P0-555 Claude/Anthropic stream message_delta usage merge honesty tests (never invents tokens) (#486 / #488)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 46 post-product landings; SEC-MONITOR-TOKEN-CLEAR + UI-CSS-RESIDUAL + REL-P0555-STREAM-TESTS present · board #484–#487 closed (#487 / #491)

## [v0.8.35] — 2026-07-18

### UI
- Wire DownstreamKeys maxRpm/maxTpm create/edit/list (backend #116 admission already present) (#475 / #481)
- Tokenize residual login-shell hard-coded hex to design tokens where clean (#477 / #480)

### Tests / Reliability
- P0-585 empty-filter global full-set fallback honesty regression tests (does not flip cascade to present) (#476 / #479)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 45 post-product landings; UI-KEY-RPM-TPM + REL-EMPTY-FILTER-TESTS + UI-LOGIN-TOKENS present · board #475–#478 closed (#478 / #482)

## [v0.8.34] — 2026-07-18

### UI
- Wire DownstreamKeys proxyUrl create/edit/list (backend KEY-578 already present) (#466 / #471)
- Wire TokenRoutes contextLength create/edit/list badge (backend CTX-520 admin already present) (#467 / #472)
- Migrate high-value hard-coded CSS hex clusters (checkin-toggle, route-enable, info-tip, model-tag-*, status-dot-*) to design tokens (#468 / #470)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 44 post-product landings; UI-KEY-PROXY + UI-ROUTE-CTX + UI-TOKEN-DEBT present · board #466–#469 closed (#469 / #473)

## [v0.8.33] — 2026-07-18

### UI
- Migrate hard-coded .stat-icon-* colors to design tokens (var(--color-stat-*)) for light/dark SSOT (#456 / #460)
- Wire Sites maxConcurrency in admin create/edit/list (backend limiter already present) (#457 / #461)

### Fixed
- Gemini generateContent/streamGenerateContent: reject generationConfig.maxOutputTokens above positive route context_length with honest 400 (extends CTX-520) (#458 / #462)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 43 post v0.8.32; UI-STAT-TOKENS + UI-SITE-CONC + CTX-520-GEMINI present · board #456–#459 closed (#459 / #464)

## [v0.8.32] — 2026-07-18

### Security
- system-proxy/test rejects non-empty targetUrl that fails IsValidHTTPURL / IsForbiddenSiteTargetURL (metadata/link-local) before probe (#449 / #452)

### Fixed
- OpenAI /v1/responses (+ /compact): reject max_output_tokens or max_tokens above positive route context_length with honest 400 (no silent clamp; extends CTX-520) (#450 / #454)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 42 post v0.8.31; SEC-PROXY-TEST-TARGET + CTX-520 Responses path present · board #449–#451 closed (#451 / #453)

## [v0.8.31] — 2026-07-18

### Security
- ProxyAwareHTTPClient shares RejectCrossOriginRedirect (HTTPGet/Post helpers inherit; Telegram patch idempotent) (#441 / #446)
- SiteProxy buildClients + doWithExplicitProxy share RejectCrossOriginRedirect (parity with DoWithProxy hot path) (#442 / #444)
- Downstream-keys update + reset-usage redact plaintext key (keyMasked only; create/export once-echo unchanged) (#440 / #445)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 41 post v0.8.30; SEC-PROXY-UTIL-REDIR / SEC-SITEPROXY-REDIR / SEC-KEY-MUTATE present · board #440–#443 closed (#443 / #447)

## [v0.8.30] — 2026-07-18

### Security
- Share RejectCrossOriginRedirect on residual OAuth Codex HTTP client + Telegram notify clients; public-origin 302 to different host rejected (#433 / #436)

### Fixed
- loadRouteMatch applies source route model_pattern as SourceModel fallback when channel SourceModel blank/nil (group/source eligibility + resolveModel) (#434 / #438)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 40 post v0.8.29; SEC-OAUTH-NOTIFY-REDIR + REL-SOURCE-MODEL present · board #433–#435 closed (#435 / #437)

## [v0.8.29] — 2026-07-18

### Fixed
- Preferred/sticky channel selection respects open site/model breaker and falls through when healthy siblings exist (`SelectPreferredChannel`) (#423 / #430)
- CooldownUntil eligibility parses timestamps (no millis-ISO vs RFC3339 lex compare) via `IsCooldownActive` (#424 / #427)
- Proxy conductor hard max attempt budget across same-channel + refresh + failover; cap RefreshAuth successes; nil/error RefreshAuth → sibling failover with channel-scoped exclude (#425 / #431)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 39 post v0.8.28; REL-PREFERRED-BREAKER / REL-COOLDOWN-TS / REL-CONDUCTOR-BUDGET present · board #423–#426 closed (#426 / #428)

## [v0.8.28] — 2026-07-18

### Security
- Share `RejectCrossOriginRedirect` on residual bare clients: channel health probe, channel test harness, and `defaultUpstreamClient` (no longer follow public-origin 302 → private/metadata) (#416 / #420)
- Admin logout / session clear sets `Max-Age=0` for `meta_monitor_auth` with matching `Path=/monitor-proxy/` (#417 / #421)

### Fixed
- Residual bare HTTP clients no longer inherit Go default redirect policy when site proxy is absent (#416 / #420)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 38 post v0.8.27; SEC-REDIR bare clients + SEC-MONITOR logout residual shipped · board #416–#418 closed (#418 / #419)

## [v0.8.27] — 2026-07-18

### Security
- Monitor session cookie is opaque HMAC (never embeds live `AUTH_TOKEN`); constant-time compare; cookie scoped to `Path=/monitor-proxy/` so theft cannot become full admin bearer (#407 / #414)
- Admin auth token change: constant-time `OldToken` compare (parity with AdminAuth middleware; reject mismatched lengths without leaking timing) (#408 / #411)

### Fixed
- Claude `/v1/messages`: reject `max_tokens` above positive selected-route `context_length` with honest 400 `invalid_request_error` (no silent clamp; extends OpenAI #399) (#409 / #412)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 37 post v0.8.26; SEC-MONITOR + SEC-AUTH-TIMING present · CTX-520 Claude path shipped (further dialect residual) · board #407–#410 closed (#410 / #413)

## [v0.8.26] — 2026-07-18

### Security
- `IsValidAPIEndpointURL` rejects cloud metadata / link-local targets (aligned with `IsForbiddenSiteTargetURL` / `IsValidHTTPURL`); any caller is safe by default (#398 / #403)

### Fixed
- OpenAI chat/completions (and legacy completions): reject `max_tokens` above positive selected-route `context_length` with honest 400 `invalid_request_error` (no silent clamp; Claude out of scope) (#399 / #404)
- OpenAI chat/completions stream: `slog.Warn` once when stream ends without usable usage after `stream_options.include_usage` (injected or client-provided); never invent token counts (#400 / #401)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 36 post M35 / v0.8.25; SEC-ENDPOINT fully present · CTX-520 max_tokens reject shipped · P0-555 missing-usage warn residual note (#397 / #402)

## [v0.8.25] — 2026-07-17

### Security
- `IsValidHTTPURL` rejects cloud metadata / link-local targets (aligned with `IsForbiddenSiteTargetURL`); site externalCheckin URL uses the hardened check (#382 / #385)

### Fixed
- Admin `GET /api/routes` batch-loads route channels in one query and groups in memory (kills per-route N+1; response shape + #375 redact unchanged) (#383 / #386)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 34 post v0.8.24; SEC-HTTPURL + PERF-ROUTES → present (#384 / #387)

## [v0.8.24] — 2026-07-17

### Security
- Admin routes channel list/get + `POST /api/search` redact plaintext `accessToken`/`apiToken`/`token` (masked only) (#375 / #378)
- Site create/update + API endpoint upsert reject cloud metadata / link-local URLs (`169.254.0.0/16`, IPv6 link-local, `metadata.google.internal`); RFC1918 + localhost still allowed (#376 / #379)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 33 post v0.8.23; SEC-ROUTE + SEC-SITEURL → present (#377 / #380)

## [v0.8.23] — 2026-07-17

### Security
- Admin account list/overview redacts `accessToken`/`apiToken` (masked only) and strips `passwordCipher` from list `extraConfig`; account-token list drops join credential fields (#367 / #372)
- Credential export remains intentional product path (create/update may still echo once outside list enrichment)

### Fixed
- Round-robin / stable_first / least_* soft-filter priority demotion: soft-empty higher priority tries next layer (parity with weighted #358) (#368 / #370)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 32 post v0.8.22; SEC-ADMIN + REL-SOFT-RR → present (#366 / #369)

## [v0.8.22] — 2026-07-17

### Security
- Redact plaintext `key` from admin downstream-keys list/summary/overview (`keyMasked` only) (#355 / #361)
- Deny-list sensitive site `custom_headers` (Authorization/Host/Cookie/hop-by-hop/Proxy-*/Content-Type); shared `platform.ApplyCustomHeaders`; Bearer set after custom so identity cannot be overridden (#356 / #364)
- RuntimeExecutor `CheckRedirect` rejects cross-origin and private/metadata redirect targets (#357 / #360)

### Fixed
- Weighted routing: when soft-filter empties a priority layer, try the next priority instead of reselecting the unfiltered broken layer (#358 / #362)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 31 post v0.8.21; SEC-KEY/HDR/REDIR + REL-SOFT → present (#359 / #363)

## [v0.8.21] — 2026-07-17

### Fixed
- OpenAI legacy `/v1/completions` stream: same `stream_options.include_usage=true` inject as chat (path helper; still skips codex/sub2api and non-OpenAI stream_options paths) (#350 / #352)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 30 post v0.8.20 (#351 / #353)
- P0-555: chat + legacy completions stream policy wired; residual provider-ignore / media zeros / multi-instance lag / orphan site join

## [v0.8.20] — 2026-07-17

### Fixed
- OpenAI-compatible chat/completions stream: inject/merge `stream_options.include_usage=true` on upstream body for final SSE usage chunks; skip codex/sub2api and non-chat paths (P0-555 residual) (#345 / #347)

### Docs / Honesty
- Residual inventory + MASTER for Milestone 29 post v0.8.19 (#346 / #348)
- P0-555 evidence: stream_options policy addressed for chat stream; residual provider-ignore / media zeros / multi-instance lag / orphan site join

## [v0.8.19] — 2026-07-17

### Fixed
- Race-harden `scheduleSiteRuntimeHealthPersistence` / `persistSiteRuntimeHealthState`: timer + in-flight flags under `healthStateMu`; concurrent success/failure regression (#335 / #339)

### Docs / Honesty
- Residual inventory + MASTER flip for Milestone 28 post v0.8.18 (#334 / #337)
- P0-585 cascade residual honesty: shipped channel isolation vs site/model breaker + empty-filter fallback + no production multi-channel load proof (`failover-isolation.md`) (#336 / #343)

## [v0.8.18] — 2026-07-17

### Added
- OpenAI `/v1/models` prefers positive `token_routes.context_length` (max per exposed model id) over `knownModelContextLength` heuristics; production load path SELECTs the column (#327 / #332)

### Fixed
- Admin test isolation: stop reassigning `globalAccountsCache` pointer; drain background health-refresh runners before registry reset (DATA RACE under full `-race` suite) (#328 / #331)
- Race-safe `healthPersistTimer` clear under `healthStateMu` (ConfigureProxyUpstream / site runtime health debounce) (#327 gate)

### Docs / Honesty
- Residual inventory + MASTER pointers for Milestone 27 (#329 / #330)
- CTX-520 residual: models wire present; still no proxy max-token enforcement

## [v0.8.17] — 2026-07-17

### Added
- Admin `token_routes.contextLength` create/update + list/summary/lite surfaces (metadata-only; no proxy max-token enforcement) (#320 / #323)

### Fixed / Verified
- Usage aggregation projects `proxy_logs.status=failed` tokens into `failed_calls` + `total_tokens` (regression + audit note; aggregation logic already status-agnostic) (#319 / #324)

### Docs / Honesty
- Residual inventory + MASTER pointers post v0.8.16 (#318 / #322)
- P0-555 → present-with-residual after #311/#319; residual policy/media/lag only

## [v0.8.16] — 2026-07-17

### Fixed
- Wire Gemini official tool-history `thought_signature` inject/preserve on generateContent / gemini-cli paths (#309 / #314)
- Harden multi-turn Responses reasoning content sanitize (pretty-printed type keys + input gate) (#310 / #313)
- Persist failed upstream attempts to proxy_logs with best-effort usage from error bodies (#311 / #315)

### Docs / Honesty
- Gap matrix #580/#581/#538 → present (with residual notes)
- usage-token-extraction-audit follow-up (#311)
- Hot-fix conflict markers in upstream_test after squash (#316)

## [v0.8.15] — 2026-07-17

### Fixed
- Gate `ReportTokenExpired` / checkin-balance mark paths with `ShouldMarkAccountExpired` (no bare/generic 401 over-expiry) (#298 / #301)
- Channel-scoped cascade isolation: 429 fails over, same-channel timeout budget, multi-channel same-site isolation tests (#299 / #302)
- Preserve stream/partial usage on client disconnect when usage was already extracted (#300 / #303)

### Docs / Honesty
- Failover isolation residual notes for #585 (#299)
- Gap matrix rows for #568 present + #585/#555 partial evidence refresh (via #301/#302/#303)

## [v0.8.14] — 2026-07-17

### Docs / Honesty
- Residual next candidates inventory post v0.8.13 (#290 / #293)
- Redis sticky Option B design spike (no product code) (#292 / #294)
- Admin /api/test stream and job residual honesty (#291 / #295)

## [v0.8.13] — 2026-07-17

### Added
- token_routes.sort_order + PUT /api/routes/reorder bulk drag reorder (#284 / #288)

### Docs / Honesty
- original-gap-matrix refresh for shipped surfaces (rerank/site concurrency/key proxy/rebuild/cache_ratio) (#281 / #285)
- sticky multi-instance affinity product-path evaluation (#282 / #286)
- update-center residual honesty hardening (no remote registry) (#283 / #287)

## [v0.8.12] — 2026-07-17

### Fixed
- Admin BackgroundTask snapshot under mutex (DATA RACE on get/list vs runner Result write) (#271 / #275)

### Added
- Site-announcement scheduler wires to real `SyncSiteAnnouncements` via SyncFunc (#272 / #278)
- Channel recovery active candidates via optional `ProxyChannelCoordinator` provider hook (#273 / #276)

### Docs / Honesty
- Responses WebSocket residual product path evaluation (stay 426/501 for v0.8.x) (#274 / #277)

## [v0.8.11] — 2026-07-17

### Added
- DB-backed durable admin BackgroundTask store (cross-instance list/get) (#265 / #267)

### Fixed
- Frontend CI EnvironmentTeardownError flake hardening (#266 / #268)

## [v0.8.10] — 2026-07-17

### Added
- Sub2API refresh scheduler wires to RefreshBalance (#261 / #263)
- Proxy video task age-based retention scheduler (config-gated, default off) (#262 / #263)

## [v0.8.9] — 2026-07-17

### Added
- Videos GET/DELETE sticky pin via ForcedChannelID from mapping ChannelID (#253 / #256)

### Docs / Honesty
- proxy_video_tasks retention residual (no TTL/GC) (#254 / #259)

## [v0.8.8] — 2026-07-17

### Added
- Durable `proxy_video_tasks` dual-write for video publicId mapping (multi-instance / restart) (#244 / #251)
- TPM multi-instance Redis sharing via sharedcount (fail-open, mirrors RPM) (#245 / #249)

### Docs / Honesty
- Scheduler silent TODO residual inventory (sub2api / channel-recovery / announcement / update-center) (#246 / #250)

## [v0.8.7] — 2026-07-17

### Added
- Videos create: process-local publicId mapping + response `id` rewrite on successful POST /v1/videos (#235 / #241)

### Fixed / Honesty
- ResolveInputFile returns explicit residual error (no silent vault) (#238 / #239)
- Sticky session multi-instance residual analysis + code comment (#237 / #240)
- Admin StartBackgroundTask / /api/tasks process-local multi-instance residual honesty (#236 / #242)

## [v0.8.6] — 2026-07-17

### Fixed
- Videos GET/DELETE honest upstream passthrough without empty local-store 404 theater (#225 / #231)

### Added / Tests
- Downstream key maxCost/maxRequests clear-to-NULL API tests (#226 / #233)
- Claude cache_ratio 0.1 / cache_creation_ratio 1.25 assertions on proxy billing details (#227 / #230)
- ParseInputFiles extracts OpenAI input_file/file body refs (#228 / #232)

## [v0.8.5] — 2026-07-17

### Added
- Site initialization preset registry + create/detect validation (#214 / #222)
- Gemini `/v1beta/models` from owned model catalog (#215 / #221)
- Site proxy cache invalidation hooks (routing + admin accounts snapshot) (#216 / #219)
- Responses WebSocket honest residual + boot wire (#217 / #220)

### Fixed
- Shared PG CI: prefer `SiteSelectColumns` over `SELECT * FROM sites` (probe-column drift)

## [v0.8.4] — 2026-07-17

### Fixed
- PostgreSQL CreateSite: RETURNING id + explicit sites column select (shared CI probe-column drift) (#204 / #208)
- Multipart `/v1/images/edits` forwards via dispatchUpstream (no example.com stub) (#207 / #210)

### Added
- Expired API-key account recovery on credential update (allowInactive model refresh + reactivate) (#205 / #212)
- Account token groups via platform.GetUserGroups with local fallback (#206 / #211)

## [v0.8.3] — 2026-07-17

### Added
- Admin residual stubs wave (milestone 12):
  - sub2api managed auth merge on account update/rebind (#194 / #202)
  - Real account health-refresh via balance probe (#195 / #199)
  - OAuth start/rebind CSRF state tokens (server-stored, TTL) (#196 / #200)
  - Honest update-center deploy/rollback residuals + real clear-cache invalidation (#197 / #201)

## [v0.8.2] — 2026-07-17

### Added
- P4 adapter wiring (milestone 11):
  - Account token create/delete/sync via platform adapters + SyncTokensFromUpstream (#182 / #190)
  - Account create fail-closed VerifyToken / GetModels with skipModelFetch residual (#183 / #189)
  - Real system-proxy probe + brand list from platform registry (#184 / #186)
  - `/api/test/proxy` + `/api/test/chat` wired to forced-channel harness; stream/jobs honest 501 (#185 / #187)

### Notes
- Residual TODOs: sub2api managed auth on update, expired API-key recovery model refresh, async health-refresh job, OAuth state stubs.

## [v0.8.1] — 2026-07-17

### Fixed
- Go 1.26.5 toolchain; vulncheck green (GO-2026-5856) (#168)

### Added
- Live /v1/models listing via TokenRouter.GetAvailableModels (#169)
- Boot-wired ModelProbeScheduler probe executor + health recorder (#170)
- Route decision admin APIs wired to ExplainSelection (#171)

## [v0.8.0] — 2026-07-17

### Added
- Competitive learn program (M-COMPETE) fully implemented for [learn] issues #110–#121:
  - Request trace IDs across retries/failovers (#110)
  - Per-request cost attribution + cache token types (#111)
  - TTFT/first-byte signals in routing health (#113)
  - Cross-site model price comparison APIs (#112)
  - Background channel health probing (#114)
  - Pluggable routing strategies: least_busy / lowest_latency / lowest_cost (#115)
  - Downstream-key RPM/TPM soft admission + Retry-After (#116)
  - Richer Prometheus histograms/labels + MetricsObserver export hook (#117)
  - Optional Redis-backed shared RPM admission (fail-open, zero third-party dep) (#118)
  - Admin forced-channel test harness (#119)
  - Client credential export adapters (openai/cherry/generic) (#120)
  - Usage heatmaps + slow-request ranking stats (#121)
- Enterprise ops residual milestone opened for remaining admin/proxy stubs (#154–#158).

### Changed
- MASTER progress: M-COMPETE learn #110–#121 marked complete; stack remains TS 7.0.2 + React 19.2.7 + Vite 8.1.5 + Go 1.26.4.

### Notes
- `vulncheck` may still fail on Go 1.26.4 stdlib advisory GO-2026-5856; CI continues with continue-on-error until a Go patch is available.
- Residual operator stubs (site probe-now stream, /v1/files 501, marketplace/token-candidates, notify/LDOH/tasks) tracked under milestone Enterprise ops residual + v0.8.0.

## [v0.7.0] — 2026-07-17

### Added
- Enterprise modernization program (stack TS7/React19/Vite8, UI tokens/a11y, backend boundaries, schema additive migrations, reliability SSOT).
- Feature completeness from original metapi gap matrix: site max concurrency, per-key proxy, group route rebuild, `/v1/rerank`, usage/token accounting, failover/first-byte, protocol pack (Gemini thought_signature, Minimax thinking, models shape, previous_response_id, skill-call, responses multi-turn reasoning, responses-only sites), Codex OAuth gpt-5.5 + discovery soft-retry.
- Competitive learning milestone (M-COMPETE) vs all-api-hub / axonhub / new-api / litellm with matrix + `[learn]` backlog.

### Fixed
- Admin correctness: key refresh name/enable preserve, quota clear, model whitelist non-destructive parse, in-route model config preserve, expired account health.
- Frontend CI flake: dashboard site-observability EnvironmentTeardownError hardening.

### Notes
- `vulncheck` may still fail on Go 1.26.4 stdlib advisories (GO-2026-5856 class); CI keeps continue-on-error until Go patch available.
- Competitive `[learn]` items remain backlog-only until scheduled for implementation.

## [v0.6.5] — 2026-07-10

### Fixed
- 修复 Content-Security-Policy 缺少 `frame-src` 导致 `check.linux.do` iframe 被拦截。

## [v0.6.4] — 2026-07-10

### Fixed
- 修复 Content-Security-Policy 过紧导致 dicebear 头像图片和 Cloudflare Insights 脚本被浏览器拦截。
- 新增 `img-src 'self' https://api.dicebear.com`、`connect-src 'self'` 和 `script-src https://static.cloudflareinsights.com` 指令。

## [v0.6.3] — 2026-07-07

### Fixed
- 修复后台 Admin API 被重复挂载成 `/api/api/*` 的生产路由问题，恢复 `/api/settings/auth/info`、站点、账号、签到等管理接口的正常访问。
- 登录页增加登录前明暗/跟随系统主题切换，并修复深色模式下品牌面板、链接和图标对比度。

## [v0.6.2] — 2026-07-07

### Fixed
- 修复根路径 WebUI 被非 `/v1` 代理别名鉴权拦截的问题，确保嵌入式 SPA fallback 正常返回前端页面。
- 修复嵌入式前端文件系统路径兼容性，支持 `web/dist` 作为 embed 根。
- 稳定 routing golden 与加权随机测试，避免 Windows CRLF checkout 和单次随机抽样导致 CI 偶发失败。

## [v0.6.1] — 2026-07-07

### Fixed
- CI/CD secret scan 改用开源 gitleaks CLI，避免 organization 仓库被 `gitleaks/gitleaks-action@v2` license gate 阻断发布。

## [v0.6.0] — 2026-07-07

### Security
- CI/CD 发布门禁加入 gitleaks、Go module 校验、race 测试、PostgreSQL integration 测试、前端 typecheck/test/build 和生产依赖 audit。
- CD 镜像发布前执行 Docker smoke test；发布镜像启用 provenance 和 SBOM。
- 测试和文档中的 PostgreSQL DSN 改为运行时拼接，减少 secret scanner 噪声。
- 站点自定义 headers 过滤 `Authorization`、`Cookie`、`New-API-User`、`Content-Type`、`Content-Length`、`Host` 等保留头，避免覆盖运行时认证语义。

### Fixed
- `/v1/*` 数据面接入数据库路由和真实上游选择，不再停留在未配置 stub 行为。
- 上游代理支持站点/账号代理、自定义 headers、失败记录和非流式可重试 failover。
- AnyRouter 禁用 NewAPI 风格 token 管理端点，避免错误调用 `/api/token`。
- API-key/proxy-only 账号不再执行签到或余额上游调用，禁用状态判断改为大小写不敏感。
- 账号 session rebind、manual models、account token 默认值维护补齐事务和错误处理，失败路径回滚。

### Added
- 覆盖 SQLite 和 PostgreSQL 的账号、签到、余额、AnyRouter、代理上游和路由选择回归测试。
- 运行时说明明确当前支持 SQLite 单节点和 PostgreSQL 部署；Redis 尚未集成。

## [v0.5.0] — 2026-07-05

### Security
- Admin/proxy token 比较改用 `crypto/subtle.ConstantTimeCompare`（防时序攻击）
- CI 启用 `errcheck`、`staticcheck`、`ineffassign` linter
- CI 测试启用 `-race`（data race 检测）
- `/debug/vars` 移至 admin auth 保护后（此前无认证暴露）
- 安全响应头中间件：`X-Content-Type-Options`, `X-Frame-Options`, `Referrer-Policy`, `CSP`
- AES 密钥派生不再 fallback 到 `AUTH_TOKEN`（独立默认值）

### Fixed
- 代理出口 `http.DefaultClient`（零超时）→ 接入 `RuntimeExecutor`（90s 超时 fallback）
- 6 处 OAuth `panic()` → `return error`
- SSE 流式响应 `WriteTimeout: 60s` 截断问题 → `SetWriteDeadline` 禁用
- 13 处 `log.Printf` → `slog.Warn/Error`
- DB 连接池补充 `ConnMaxLifetime`(5min) + `ConnMaxIdleTime`(2min)
- `usage_aggregation` goroutine re-panic 修复（不再能崩进程）
- `CheckinScheduler.Stop()` data race 修复
- CI：`golangci-lint-action@v6` Go 1.25 不兼容 → `go install` 最新版
- `golangci-lint` 全项目 zero warning

### Added
- `/metrics` Prometheus 端点（零依赖 text format）
- `RequestID` 中间件（`X-Request-Id` header + 日志关联）
- `handler/shared/errors.go`：`APIError` 结构化错误类型
- git pre-push hook（`.githooks/pre-push`）：自动跑 `vet + test -race`
- Claude Code push guard（`~/.claude/hooks/metapi-go-push-guard.sh`）
- `AGENTS.md` CI Discipline 规范

### Tests
- 8 个零覆盖包全部补齐测试（最低 50%，最高 100%）
- 新增 3 个 e2e 场景：并发代理、auth 时序安全、rate limit 拒绝
- `e2e` 测试包总数：4 → 5 文件

## [v0.4.0] — 2026-07-05

### Fixed
- 6 轮 audit 全部修复
- PG 兼容：`INSERT OR IGNORE` → `ON CONFLICT DO NOTHING`
- Cron 5 字段 → 6 字段自动转换
- `sqlx.BindDriver` 时序修复（`?` → `$N` 占位符重绑定）

## [v0.3.0] — 2026-07-04

### Changed
- goroutine 泄漏修复
- JSON 性能优化
- 包命名规范化
- `config.Validate()` 10 项启动校验

## [v0.2.0] — 2026-07-04

### Added
- 限流中间件（admin 100rps, OAuth 10rps）
- RWMutex 假桩替换为真实 `sync.RWMutex`
- DB 事务包裹 usage aggregation batch
- `store.Close()` 优雅关机

## [v0.1.0] — 2026-07-03

### Added
- MetAPI TypeScript → Go 完整重写初始发布
- 27 表双数据库（SQLite + PostgreSQL）
- 14 平台适配器
- 4 协议流式转换
- 15 后台调度任务
- 单二进制 + Docker 部署

[v0.6.3]: https://github.com/TokenDanceLab/metapi-go/compare/v0.6.2...v0.6.3
[v0.6.2]: https://github.com/TokenDanceLab/metapi-go/compare/v0.6.1...v0.6.2
[v0.6.1]: https://github.com/TokenDanceLab/metapi-go/compare/v0.6.0...v0.6.1
[v0.6.0]: https://github.com/TokenDanceLab/metapi-go/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/TokenDanceLab/metapi-go/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/TokenDanceLab/metapi-go/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/TokenDanceLab/metapi-go/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/TokenDanceLab/metapi-go/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/TokenDanceLab/metapi-go/releases/tag/v0.1.0
