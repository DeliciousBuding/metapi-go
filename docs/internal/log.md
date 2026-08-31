# log.md — Metapi Go product milestones

**Last updated**: 2026-08-31

> Product milestone timeline (grouped by version). Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../../CHANGELOG.md)

## 2026-08-31 — 前端视觉 QA 波 + auth 文案国际化启动

- **#1101**：代理日志时间列详情行同日重复（窗口内短日期与相对时间连读），新增 `formatLogDateDetail` 收敛为「短日期 · 相对时间」，超窗仅绝对日期。
- **#1102**：清理页面级无障碍残留——三层页级 `<main>` 与布局壳唯一 `<main>` 叠加成双 landmark（页级降为 `<div>`）；三处品牌 logo `alt` 与相邻同名文本重复（改 `alt=""`）；catalog-sources 拖拽列空表头补屏幕阅读器标签。
- **#1103**：清理 17 个零引用文案键（en/zh-CN 各删 17），保留经模板/配置映射引用的动态键；双语键集合保持一致。
- **#1104**：设置页单卡片节标题与描述从逐字重复收敛为单处——`SettingsSectionCard` 无头部动作时不渲染卡头；数据迁移节的警告信息移至卡内顶部警示条。骨架规约记入 DESIGN.md §4.2。
- **auth 文案国际化启动**：admin 认证中间件拒绝信息（会话过期/缺凭据/令牌无效/IP 白名单）及 reauth 确认改用机器可读 `errorCode` 承载，前端据 `errorCode` 渲染本地化文案（`errors.auth.*`），未注册码回退后端原文——后端英文文案本地化的首块。

## 2026-08-30 — 事件标题 i18n 统一 + S7 收官 + S8 注册表合一 + F4 文档拆分

- **#1099 事件标题 i18n 统一**：程序日志页与 attention 管线各带一份事件标题映射 + 各自 locale 节（漂移源）；收敛为共享 `lib/event-titles.ts` 单一 title→slug 映射（15 个已知生产者标题）+ 单一 locale 节 `events.titles.*`；未知/动态标题仍原文渲染。
- **#1097 删除+undo 档**：叶子实体单行删除（模型重定向、目录源、令牌路由、下游密钥、账号令牌）改为延迟删除——乐观移除 + 6s 撤销 + 快照恢复，真实删除仅在窗口关闭后；批量/级联/factory reset 保留各自确认档。四档规约入 DESIGN.md §4.1。
- **#1095 section-registry 三合一**：settings/dashboard/observability 三份克隆收敛为共享 `lib/section-registry.ts`（`urlStyle` 承载 path/query 两种 URL 形态）。
- **#1084 列表页错误三态收敛**：加载失败横幅+重试从 8 个列表页手写分支收敛为 `DataTablePage` 底座内置契约。
- **#1091 F3 告警文案 i18n**：attention 铃铛弹层与可用性面板共用共享模块 `lib/attention-label.ts`（8 个持久化事件标题映射）；en/zh-CN 补齐 7 键；参数缺失回退原文。
- **#1092 i18n 占位符 parity 门禁**：i18n-keys 测试断言每双语键的 `{{var}}` 集合一致，锁死半翻译回归。
- **#1093 channels 列宽修正**：响应延迟/名称/冷却至列宽适配，宽表回到首屏。
- **#1085 F4 api.md 按域拆分**：1739 行超预算的 API 参考拆为 `docs/api/*.md` 17 个文件；`api.md` 保留索引 + 全部标题 stub 承接旧深链。
- **#1086 移动端账号页头挤压修复**：375px 窄视口页头对齐 checkin/路由 flex-wrap 模式。
- **#1087 图表无障碍替代层**：延迟直方图/趋势接入 ChartDataTable，dashboard 六图 sr-only 数据摘要全齐。
- **#1088 快照 delta 去重**：余额 7 天对比无数据时 em-dash 与 Minus 图标同形连读，不可用态只留 em-dash。

## 2026-08-29 — 后端收口波 + C1 config 竞态 + 双线融合（#1063–#1080）

- **#1079 C1 config 竞态收口**：`RuntimeSettings` 不可变快照迁移，消除约 25 个运行时写字段 × 热路径无锁读的撕裂风险。
- **#1077 渠道错误汇总**：`GET /api/channels/error-summary` + `/api/channels` 状态过滤与服务端分页。
- **#1075 S9 服务端分页**：accounts + models marketplace 大表；至此 accounts/models/channels/checkin/oauth/proxy-logs/audit-logs 全部服务端分页。
- **#1071–#1073 前端架构项**：S5 shell 边界反转 + 可执行 layer gate；#1026 凭证维度 UI 树形选择器；cmdk actions 层。
- **#1065 S4 errorCode**：统一 400 错误体带机器可读 errorCode，前端 `resolveResponseErrorCode`/`extractApiErrorBody` 消费。
- **#1080 双线融合**：前端 W19–W21 审计波经集成分支语义合并后端线后合入（http-client 双函数族并集、models 分页 × 错误契约合并）。

## 2026-08-29 — W21 CSP 去 unsafe-inline（#1035 S2）

- SPA `style-src` 收紧为 `'self' 'nonce-<per-request>' 'sha256-<sonner-toast-css>'`；Go 每响应生成 CSP nonce 经 `<meta name="csp-nonce">` + 响应头传递；静态打包 sonner 样式作视觉 fallback。

## 2026-08-29 — W20 凭证维度后端加固（#1026 残留）

- `auth.ExcludedCredentialRef` JSON 标签 snake_case 与管理员端 camelCase 不一致致代理路径解析引用 ID 全为 0——统一 camelCase + 往返回归测试。
- 畸形引用条目由静默丢弃改为显式 400（避免允许列表 fail-open）。

## 2026-08-29 — 前端审计三波（本地 W19-W21）

- **W19 审计修复波**：错误 toast 单 owner 收口；列表页错误契约统一；URL 态收口；`ui/notice.tsx` 横幅原语；chart-1..5 全 preset 对 card AA 4.5:1；focus-ring ≥3:1；移动端 sheet 底部粘性关闭条；危险区 type-to-confirm；批量禁用/重建确认；OAuth 弹窗拦截恢复；invalidate 漏刷修复。
- **W20 seeded 视觉审图波**：标题字重 calm-titles 对齐；observability 热力图轴标签 9→10px；`ui/kpi-value.tsx` KPI 数字组件收编；screenshot 管道加 auth preflight。
- **W21 交互弹层审计波**：新建交互截图管道 `shot-interactions.mjs`（补静态扫描盲区）；修复站点公告四筛选裸显 `all`、路由表单图标哨兵值泄漏、表单 Sheet 豁免移动端关闭条、`theme.toggle` aria-label i18n。
- **遗留**：D2 site-form 抽屉迁移（产品决策）；后端告警文案 i18n（P3）。

## 2026-08-29 — Wave 18 多线并行 发布 v0.16.19

- **#1057 会话模型重构**：`POST /api/auth/login` 服务端会话（`admin_sessions`）+ HttpOnly/SameSite=Strict cookie + 滑动 TTL + 登出/轮换吊销；WS 一次性 ticket；限速前置；备份/导出/轮换 `X-Admin-Confirm-Token` 重确认；前端去 localStorage 主 token。
- **#1052 并发**：`runtime_health_persist.go` 懒加载无锁读修复；审计升级项（config 单例运行时写入簇竞态）转 #1079 快照交换。
- **#1058 网络**：15 处出站调用点统一 `internal/httpclient` 基线 + AST 静态门禁。
- **#1054 数据库/性能**：`sc2_027` 三个管理读路径索引（channelId 过滤 17.9ms→0.5ms）；proxy-logs LEFT→INNER、marketplace N+1 批量化。
- **#1061 调度器健壮性**：job panic 统一 recover；in-flight 标志锁外读修复；吞没 DB 错误显性化。
- **#1060 方言**：约 260 处测试种子 + 3 处生产位点 BOOLEAN 整数字面量重写；静态方言门禁扩至全包；零迁移。
- **#1053 构建**：rsbuild 唯一 SSOT；匿名 6432 块拆 vendor-i18n/icons/core（总 JS −8.5KB gz）。
- **#1055 UX**：focus-ring 全库 ≥3:1；图表 sr-only 摘要 + axe 组件门禁。
- **#1056 健康监测开关**：`checkinEnabled`/`balanceRefreshEnabled` 运行时设置 + 环境变量，热生效持久化。
- **#1059 SOCKS5**：账号代理支持 `socks5://`/`socks5h://`；清空代理保存即清除。

## 2026-08-28 — Wave 17 竞品研究 P1×4 发布 v0.16.18

- **#1046** SSE 流 chunk 间隔空闲超时（`PROXY_STREAM_IDLE_TIMEOUT_SEC`）。
- **#1049** 重试/禁用状态码策略运营者可调。
- **#1047** 批量测试闭环（失败清单 + 一键禁用）。
- **#1048** 渠道失败横幅一键过滤（`?status=` 可分享过滤）。
- **#1040–#1045** 前端审计快赢（构建/CSP/a11y/卫生/UX）；#1036 zh-CN locale 崩溃修复；#1024 路由重建真值；#1039 搜索面板实体深链。
- **#1050** 下游密钥「上游站点限制」UI。

## 2026-08-28 — Wave 16 竞品研究 P0×3 发布 v0.16.17

- **#1018 协议转换快照回归套件**：46 份手写夹具 + 快照锁死 transform 层（Gemini 请求转换、Responses 连续性、SSE usage 提取、增量流解析）。
- **#1020 行级探测健康条**：`GET /api/channels/probe-history` + `/api/accounts/probe-history`；渠道/账号表行级健康条 + tooltip。
- **#1019 结构化冷却原因**：`route_channels`/`oauth_route_unit_members` 各 +3 可空原因列；9 码只增词表；徽章点击→根因弹窗。
- **#1022** 加权选择测试去 flake；**#1023** Release notes 卫生守卫（清内部词 + 措辞修订）。
- GHA Release v0.16.17：23 项检查 + 多架构镜像 + 5 平台二进制 + smoke。

## 2026-08-28 — Wave 15 issue 收口 发布 v0.16.16

- **#1005 定时上游模型同步**（`MODEL_SYNC_CRON` + `modelSyncCron` 运行时热更新）；单账号失败不中断整批。
- **#1009 出站代理超时可配置**（五个 `PROXY_*_TIMEOUT_SEC`）。
- **竞品研究**：`docs/internal/analysis/competitor-study-2026-08.md`（new-api / axonhub / sub2api）。

## 2026-08-27 — Wave 14 账号表单与分页修复 发布 v0.16.15

- **#1007**：inline token 验证与账号创建使用表单中未保存的 `platformUserId`/`proxyUrl`；显式代理覆盖站点/Resin/系统代理。
- **#1008**：Accounts 表格 URL 单一控制分页；补第 2 页 11–20 行渲染回归测试。

## 2026-08-27 — Wave 13 token sync UI 真值 发布 v0.16.14

- 账号创建/登录 toast 如实报告 `tokenSyncStatus` 四态——synced 真实计数、empty 无上游令牌、failed 部分初始化、skipped 原文案；保留 `/token-routes` CTA。

## 2026-08-25 — Wave 12B account bootstrap 发布 v0.16.13

- **#1002**：session 账号创建/登录后自动同步 token；四态真值测试。
- **#998**：账号详情 Models 面板——上游刷新持久化可用性、手工行显式移除；刷新后路由重建/缓存失效恰好一次。

## 2026-08-25 — Wave 12A demand truth 发布 v0.16.12

- **#996** Sites 分页 URL 控制；**#999** 下游 key 模型策略编辑器（精确/通配/`re:` 正则）；**#1001** 账号表单站点选择器可搜索；**#1000** 公告进 attention + 待办语义澄清；截图扫描数据 profile 门禁。

## 2026-08-25 — Wave 11 UX 真值波发布 v0.16.11

- **#862 核验**：chain-context 名称解析 + toolbar 开关 + `showZeroChannel` 持久。
- model-tester 对比行 re-run；Start-OAuth 呈现 state/instructions + 有界轮询。
- 移除 `dangerouslyIgnoreUnhandledErrors`；a11y 0 serious/critical；golden 4→10 页。
- a11y residual：region/双 main/image-redundant-alt/空表头（本轮不强修，后于 #1102 清）。

## 2026-08-25 — Wave 10 Sites demand batch 发布 v0.16.10

- **#985 站点快捷跳转**；**#986** `/site-announcements` 独立 SPA + camelCase 契约 + 诚实同步错误；SSRF dial-time 守卫；newapi/donehub 公告信封校验。
- **#991** pre-push 门禁链 + release freshness 门禁；**#992** 产品公告前端真值。

## 2026-08-24 — Wave 10 #981 迁移兼容修复

- Sub2API `tokenExpiresAt` epoch 毫秒归一；站点摘要用 `time.UnixMilli` 修 JSON 序列化失败。

## 2026-08-24 — 计费货币整理（USD 收口）

- 全链路 USD；`lib/format` 新增 `USD_SYMBOL` + `formatCurrency`/`formatPrice` 收敛全部货币渲染；docs/api.md 补 Billing & Currency 约定。

## 2026-08-24 — Wave 9 发布 v0.16.9

- mobile audit + entry-chunk split + contrast zero-exemption 合入；tag v0.16.9（12-check 全绿 + 多架构镜像 + 5 平台二进制）。

## 2026-08-24 — Wave 9 冻结恢复 + 集成

- 恢复 SOP；首屏 bundle 拆分（locale 双 JSON 懒加载 async chunk，入口 303.6→123.5KB gz 90.7→33.1KB）；对比度豁免 8→6；6 项 preset residual 最小改色至 AA，豁免表归零。
- a11y 0 serious/critical；route-smoke clean。

## 2026-08-23 — Wave 8 收官 + Wave 9 冻结交接

- **#971**：模型数据源多源注册表（llm-metadata + models.dev，自动/手动同步）+ models 页水合 + fix-candidates 删页合并 + settings IA 重构 + 14 条产品语义修复。
- 生产实例滚动部署；版本号不动，攒波待批。

## 2026-08-23 — Wave 7 收官：合并 master + 生产滚动部署（未发版）

- **#970**：55 修复 + 2 授权跳过 + 治理改动（Release draft + notes 卫生门禁 + CHANGELOG 19 段公开化重写）。
- 生产实例滚动部署；版本号不动，攒波待批。
- 19 份历史 Release notes 改写为公开安全版（清漏洞细节/内部覆盖率/波次术语）；Release 流程改 draft + 黑名单门禁。

## 2026-08-23 — Wave 7 前端体验波启动

- clickability 门禁（遮挡中心+四角检测 + 24px 命中区，发现 81 条真实硬失败）；种子实例 ×3；桌面 82 + 移动 28 张预采集截图。
- 竞品研究：TS 原版退化清单 + new-api 模式目录。
- 12 页组实测审查 → 60 条确认（1 P0 / 16 P1 / 43 P2）并修复。

## 2026-08-23 — Wave 5+6 → v0.16.8

- Wave 5 功能线（#935 编辑器 / #939 探针 / #938 英文化 / #936 审计残留 / #937 测试矩阵 / #951 截图管道）+ Wave 6 六维深审计（#941 备份凭据植入 / #942 全失败告警 / #943 可空列同族 / #944-947 性能 / #948 UX 动线 / #949 对比度 / #950 前端卫生 / #940 api.md）；15 PR squash 合入。
- 测试矩阵扩至 CI 真实上游 4/16；runtime-smoke 4 平台；截图证据管道 + 4 页 golden 门禁。

## 2026-08-22 — Wave 4 综合质量波 → v0.16.7

- 9 + 4 条 PR（契约回归/安全/a11y/部署体验/迁移兼容/实测验收/残留打磨/空快照/对外门面/假成功/NULL 余额）squash 合入。
- **#933** 可空余额列根治（`float64 → *float64`）。

## 2026-08-21 — Round 3 修复波 → v0.16.6

- **#911** D 域 4 个 P0 契约 bug（账号 token snake_case / 表单静默丢弃 / channels 截断 / route channels 缺三态）；**#910** H 域性能（gzip / accounts 冻结 / recharts 按需）；**#912** Spinner 双轨收口。

## 2026-08-21 — #887 补遗收口 + E2E Journey 3 → v0.16.5

- 全局告警红点 / OAuth sheet / About 构建信息 / proxy-log channel 过滤 + status facet / E2E Journey 3 / 死 CSS / docs-sync。

## 2026-08-21 — UI/UX Round 2 收口（v0.16.4）

- 八域（交互/移动端/视觉/IA/呈现/性能/主题/会话）并行修复 + oxfmt CI 门禁；#901 死 CSS 清理。

## 2026-08-21 — UI/UX Round 1 #887 → v0.16.3

- 四路审计立 #887，六 PR（#890–#896）并行修复 + #897 发布。

## 2026-08-20 — TS 兼容与迁移收官

- **#881** CLI 诚实化（`--verify` 失败非零退出；方言报告真实）；反向检测（TS 库比 Go 新启动即警告，不阻断）。
- **#882** 管理 UI 数据迁移接入 database-section；**#883** TS 备份 v2.1 导入兼容；**#884** 迁移三场景文档。

## 2026-08-20 — 仓库店头重建（#872 #873）

- **#872 docs 公私分离**：用户/贡献者文档留 `docs/` 根，维护者流程文档移入 `docs/internal/`；门禁测试把边界变机器断言（README 禁深链 internal + 全站相对链接可解析）。
- **#873 店头重建**：README 重写（hero + 徽章 5 个 + retina 截图墙 + copy-paste 快速上手）；Diátaxis 四象限公开文档（getting-started/configuration/client-integration/faq）；根目录 `llms.txt`。

## 2026-08-20 — 工程与测试基线加固（#866 #868 #869 #870 #871）

- **#866** Dependabot 安全告警清零（x/crypto 0.55.0 / electron 39）。
- **#868** route-smoke 滚动补强（懒加载图片断言）；**#869** i18n 残留本地化；**#870** TokenRouter 公共面覆盖；**#871** dom-audit 硬/软门分离。

## 2026-08-19 — Real-e2e UIUX audit: self-hosted brand icons + audit tooling

- **品牌图标自托管**：`BrandIcon` 从 CDN 加载被 CSP 拦截，改为 110 个图标本地化 + 静态导入索引；`dataUriLimit: 0` 防内联。
- 空态文案对齐；小打磨；审计工具链入库（screenshot-scan/dom-audit/fetch-brand-icons/verify-brand-icons）。

## 2026-08-19 — Deep backend testing: 6 defect fixes

- **Sub2API 余额刷新损坏**：`GetBalance` 吞错、`RefreshBalance` 在瞬态失败写 `balance=0` 并翻转 expired→active；改为出错时中止刷新。
- **账号 extraConfig 丢失更新**：read-merge-write 非原子（16 并发丢 12 键）；改事务 + PG `FOR UPDATE` 行锁。
- **路由边界 4**：大小写 `re:`/`RE:` 正则前缀未剥离；glob `?` 按字节不按 rune 匹配（坏非 ASCII 模型名）；JSON 映射解析跳 `\uXXXX`；usage-total 修复 int64 溢出改为饱和。

## 2026-08-19 — v0.17 onboarding polish + per-key usage + positioning honesty

- **Platform picker**：site-form 平台自由文本 → 16 适配器可搜索 Select；auto-detect 保留；未知平台「手动输入」切换。
- **Client export 广度**：claude-code / codex / openwebui profile。
- **Downstream-key 24h 用量**：`proxy_logs` 按 key 聚合 requests/tokens/cost。
- **定位诚实性**：benchmark 修正为「唯一做跨网关聚合转发」；README_EN 加 One key for every AI gateway。

## 2026-08-18 — a11y 残差核实：axe 全绿 + 菜单 Esc 行为钉死

- `bun run a11y:scan`（Playwright + axe-core）15 条认证路由 0 serious/critical；菜单 Esc 补 2 行为用例；清单卫生。

## 2026-08-18 — 复审驱动：token-routes 死列 + 列表/详情打磨

- **#854** Routes siteNames 死列修复（`listSummary` 硬编码空 → 批量 GROUP BY JOIN）。
- **#855** Routes 列表/详情打磨（删误导行、首跑空态 CTA、detail Rebuild all routes、error banner + Retry）。
- **#853** Sites gap-11：探针延迟阈值字段补全。

## 2026-08-18 — 复审驱动：availability WS 重连 + sites 表单数据丢失修复

- **#850** `useRealtimeOps` 重构为 `{ sample, reconnect }`；`gaveUp` 态「Connection lost — Reconnect」通知。
- **#851** Sites 表单静默数据丢失（`customHeadersOverrideRequestHeaders` 硬编码 false → round-trip 真实值 + 可见 Switch）。

## 2026-08-18 — 复审驱动：onboarding 闭环 + model-tester 清晰度 + availability 可视化

- **#838/#842** Dashboard onboarding banner + sites `?create=1` 深链闭环。
- **#840** model-tester 结果清晰度（四协议 `parseUsage`；删 dead stat）。
- **#846** availability 实时 sparkline 健康分档着色。
- **#847** Attention 项 `createdAt` 相对时间（`Intl.RelativeTimeFormat`）。

## 2026-08-18 — 复审驱动：动线 dead-end 收口 + proxy-logs 过滤服务端化

- **#828** Dashboard stat 卡 drilldown + 凭证导出测试链。
- **#832** Proxy-logs 过滤服务端化（latencyMin/max + client/from/to 入共享 where）。
- **#835** Downstream key edit mode（secret 不可改 + PATCH partial）；Price-compare → routes 深链。

## 2026-08-18 — UI/UX 批次：账户行内操作 + header SSOT + skeleton shimmer

- 导入向导 focus-first-invalid 回归；accounts 行内 Enable/Disable；header 高度 SSOT（CSS 变量 + 静态守卫）；skeleton shimmer（`--skeleton-highlight` + reduced-motion）；徽章机械迁移 → `<Badge>` 语义变体。

## 2026-08-17 — v0.15.x Resin per-site + 弹窗视口合约 + 设计系统溢出安全

- **Resin per-site（v0.15.0）**：站点表单 `resin_enabled`/`use_utls` 三态覆盖；持久化设置读侧 case 补齐。
- **PG BOOLEAN dialect gate（v0.15.1）**：15 处 `COALESCE(<bool>,0)=1` 改双方言通用写法 + 静态 gate 覆盖 16 BOOLEAN 列。
- **弹窗视口合约（v0.15.2 + v0.15.3）**：Dialog/AlertDialog/Popover 补 `max-h` + `overflow-y-auto`；footer sticky 不透明；静态护栏。
- **导入流程 UX（v0.15.0）**：per-item 失败原因渲染、错误防误炸、aria-busy/live、label 关联。

## 2026-08-17 — 产品品牌升级（Metapi 改名 + logo + 登录 UI）

- **品牌改名** MetAPI → Metapi（62 文件）；wire 标识、env 名、行为零变更。
- **logo 系统**：透明底 π 字形 + 蓝青渐变 SVG（亮/暗双主题）；栅格化 512/32/64 PNG。
- **登录页**：标题升 text-3xl、删冗余脚注；README 顶部 hero banner；本地 Vite dev server 补 config。

## 2026-08-17 — v0.14.0 发布收口

- 引导链/测试台真值/路由成本真值/URL 单一所有者与 Chromium smoke 合并；#800/#801/#802 CI 全绿合入。

## 2026-08-16 — 路由成本真值 + 恢复可靠性

- 路由/计价：三种暴露策略、usage 缓存缺失全价计费、models.dev 冷启动价格目录、成功衰减 + breaker half-open。
- 可靠性：SQLite 默认调优；usage aggregation flush 失败保留 delta/watermark。

## 2026-08-15 — 真实平台测试战役：测试床 + 6 个实测 bug 修复 + CI e2e

- **测试平台**：临时 ARM 机跑 metapi + new-api + one-api + sub2api + cliproxyapi；公开层用 sanitized compose 模板。
- **实测 6 个真 bug**：前端 10 路由 validateSearch 抛错、new-api 登录响应无顶层 success、one-api `/api/status` 判据失效、password 站点绑定、one-api 凭证在 session cookie、sub2api/cliproxyapi VerifyToken 静态分派。
- **CI e2e**：`test-e2e` job（真实 new-api/one-api 容器跑 smoke.sh）+ `test-sqlite` 4 分片 + golangci-lint。
- **发布** v0.13.0。

## 2026-08-14 — Leader/Worker 并行 fan-out：5 分支合入

- 5 个 worker 独立分支实现，PR #657-#662 全 squash 合入（全局搜索/今日快照+StatCard/告警富化/表格交互/测试台会话化）。
- 关闭分支保护 strict「分支必须最新」——保留 12 项必检 + squash 线性历史。
- hook-kit 修复 `leak_guard.py` 新分支 push 空树 diff 误报。

## 2026-08-14 — 多 Agent UI/UX 对照审计 + 分发端 P0

- 4 个审计 agent 对照 New API × All API Hub → `ui-ux-audit-2026-08.md`（25 视觉/9 交互/5 动线/8 功能差距）。
- 分发端 P0：客户端接入导出对话框（Cherry Studio/CC Switch 深链）；视觉 P0：`-foreground` 误用修复 + badge 变体。

## 2026-08-14 — 产品对标 + 文档卫生（neat-freak）

- benchmark.md（New API × All API Hub）；结论 P0 = 客户端一键导出 + 接入向导；"明确不做" = 多租户计费/支付/桌面版。
- 文档卫生：STATE/package-boundaries/AGENTS 对齐 as-built。

## 2026-08-14 — v0.12.0 架构简化（净删 ~21K 行）

- **死代码大清扫**（#650–#654）：删 test-only canonical 转换层、三套测试编排层、未激活队列、死 facade；Go 170K→150K。
- **数据完整性修复**：cmd/migrate 自维护 schema 漂移 → 复用 AutoMigrate + 漂移守卫。
- **去重+标准化**：手写 stdlib 重实现 → 标准库；各包去重；god-file 按接缝拆分。

## 2026-08-14 — v0.11.0 管理控制台全量 + 开源打磨

- 管理控制台 UI/UX 全量交付（共享格式化器/状态组件/toast/响应式 table/移动端降级/无障碍/token 脱敏/可观测性工作台/统一导入向导）。
- 可观测性：访问日志 status/bytes/duration_ms + statusRecorder 转发 + slog panic 恢复带 request_id + /metrics。
- 调度器注册回归修复；CI/CD 单一管道；旧 schema 增量 ALTER 回归测试；docker-compose.prod 安全硬化。

## 2026-08-12 — v0.10.0 设置中心 + 调度规格 v1

- **ScheduleSpec v1**：daily / interval / random-window / custom cron 四种调度规格。
- 设置中心：统一表单 actions + load-error 态 + dirty 导航守卫；审计日志服务端分页。
- CJK 字体回退（Noto Sans SC/TC/JP/KR + PingFang + Microsoft YaHei）。

## 2026-08-11 — post-v0.9.0 UI completion batch

- **GitHub Flow 落地**：master 唯一长期分支 + 分支保护 + squash-only + PR 模板。
- **v0.9.0 发布补推**；CD 双跑（ghcr 发布 latest/0.9.0/0.9/sha 镜像）；修 go:embed 需 web/dist、responses-websocket doc 指针。
- 前端修复：Base UI render prop + TanStack Link 冲突；searchParams parse/encode 分离；updateCenter 幽灵前端删除。
- 品牌 rename → Metapi；i18n language switcher（en/zh-CN + browser 自动跟随 + documentElement.lang/dir）；URL-synced tables 修复；copy 审计（术语统一/去内部计划码）；视觉 polish；主题定制器（4 轴 10 preset，默认 sans）；侧边栏 i18n。

## 2026-08-11 — v0.9.0 frontend rewrite

- **Frontend rewrite**：newapi 栈 100% 对齐（Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH）。
- **i18n key-based**：i18next + en/zh-CN 1369 键；`i18n-keys.test.ts` 门禁。
- **Tooling**：npm → Bun；Playwright + ui-visual 移除；旧前端树删除。

## 2026-08-02 — v0.8.46 → v0.8.52

- **PG dialect hardening**：4 处裸 `?` 占位符包 `db.Rebind` + `pg_rebind_gate_test.go` 静态检查。
- **v0.8.49+**：BOOLEAN DEFAULT PG 42804 与 balance_history UPSERT 42601 修复。
- 前端 i18n 扫查（414 处裸中文 JSX 包 `tr()`）；图表系列色 token 接线；Feishu 通知防刷；资源完整性三层；登录页重构。

## 2026-07-31 — Feature batch

- 模型重定向映射、快照 PNG、风险横幅、标签系统、批量校验、图表画廊、随机窗口调度、备份导入预览、调度任务运行历史。
- 下游密钥加固：IP allowlist/blocklist、公开价格端点、inference 后缀解析、spend dashboard、CSV 导出、缓存乘子。
- 前端 UX：RouteErrorBoundary、SearchModal 键盘导航、toast a11y、状态三态组件、Models→Playground 快速跳、ProxyLogs 日期预设。

## 2026-07-30 — Engineering optimization + parity review

- **Package boundary enforcement**：`docs/package_boundary_test.go` 把 BACKEND.md §2.3 八条硬规则变成 go test 检查。
- **Product parity review**：Go 重写未丢 TS README 产品特性；14 平台适配器对齐；多用户/支付/兑换码/邀请/订阅明确不适用。
- **Dual-dialect encapsulation**：`store.DB` 加 Context 方法（rebind `?`→`$N`），去 4 处手动分支。

## 2026-07-20 — v0.8.45 RE2-safe

- **RE2 panic 修复**：NewAPI user-id 提取的 PCRE lookahead 致 Go RE2 panic；改预编译正则 + 8 位长度上限。
- 四轨原始特性对齐：前端 18 路由 + 14 侧栏 100% parity；14 平台适配器；16 调度任务；WS/Sticky/UC 显式 residual。

## 2026-07-19 — UI polish milestone

- UI polish 批：流量 sparkline、真实页面评分、axe a11y 烟测、Dashboard 入门、Sites 横幅。
- UI parity 清单：侧栏 18 路由 parity，Sites/Accounts/Tokens/Routes/Settings 按钮数 parity。
- Focus 管理共享：`useFocusTrap` 接入 SearchModal/CenteredModal/MobileDrawer/NotificationPanel + skip-link。
