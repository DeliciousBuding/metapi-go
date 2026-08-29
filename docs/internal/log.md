# log.md — Metapi Go product milestones

**Last updated**: 2026-08-29

> Product milestone timeline (grouped by version). Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../../CHANGELOG.md)

## 2026-08-29 — W21 CSP 去 `style-src 'unsafe-inline'`（#1035 S2）

- **安全**：SPA `style-src` 改为 `'self' 'nonce-<per-request>' 'sha256-<sonner-toast-css>'`；Go 每响应生成 16B CSP nonce 并通过 `<meta name="csp-nonce">` + 响应头传递，`bootstrap.js` 在 bundle 前给运行时创建的 `<style>` 自动标 nonce；其余 directive 不变。
- **内联源盘点**：`web/dist/index.html` 无内联 script/style；运行时注入仅 sonner toast CSS（模块顶层 `__insertCSS`）与 ChartStyle 颜色变量；`react-remove-scroll` 路径保留兼容（当前 Base UI 栈未使用），未放宽其他 directive。
- **验证**：Go CSP directive 级测试、nonce 随机/meta 匹配、sonner hash 漂移守卫、ChartStyle nonce 单测；真实 Chromium 页面登录+仪表盘+Ctrl+K 弹窗探针 0 CSP 违例、nonce 匹配；前端 1425+ 测试与 Go `-race` 全门禁绿。
- **残留**：sonner 库仍会尝试模块级 DOM 注入（在 `style-src` nonce 下被允许），静态打包 `sonner/dist/styles.css` 作为视觉 fallback。

## 2026-08-29 — W20 凭证维度后端加固（#1026 残留，API-only）

- **修复（关键）**：`auth.ExcludedCredentialRef` JSON 标签 snake_case 与管理员端持久化 camelCase 不一致 → 代理路径解析引用 ID 全为 0：`allowedCredentialRefs` 密钥无法路由任何渠道（允许列表永不匹配）、`excludedCredentialRefs` 静默失效。标签统一 camelCase；DB→策略解析往返回归测试钉住。
- **收紧**：畸形引用条目（非对象/未知 kind/缺 ID）由静默丢弃改为显式 400——允许列表场景静默丢弃等于 fail-open。
- **契约测试**：路由选择器执行（两种 kind、跨 kind 隔离、空=不限制、排除优先、TS 遗留空 kind）、管理端验证拒绝矩阵、悬空引用不级联清理（失败关闭）、auth→routing 映射钉住。
- **文档**：`docs/api.md` 下游密钥节补完整契约；明确 UI 树形选择器待定（API-only）。


## 2026-08-29 — Wave 18 十线并行（安全/并发/网络/数据库/性能/构建/UX/CX）发布 v0.16.19

- **组织**：10 worktree + 10 subagent（qwen3.8-max）并行；本地推送门禁在 8 并发 race 套件下出现 `handler/admin` 300s 预算临界（环境性，各 lane 用 `METAPI_RACE_TIMEOUT_SECONDS` 官方旋钮），建议后续波次错峰推送或上调默认值。
- **安全（#1057，closes #1034）**：会话模型重构——`POST /api/auth/login` 服务端会话（`admin_sessions`，`sc2_026`）+ HttpOnly/SameSite=Strict cookie + 滑动 TTL + 登出/轮换吊销；WS 一次性 60s ticket；限速前置 + `/api/auth/*` 严格桶；备份/导出/轮换 `X-Admin-Confirm-Token` 重确认；前端去 localStorage 主 token。附带将六个浏览器门禁 harness 迁移到会话登录（原 localStorage token 注入已失效）。
- **并发（#1052）**：`routing/runtime_health_persist.go` 懒加载快速路径无锁读修复（-race 复现测试先行）。审计升级项：`*config.Config` 单例运行时写入簇为系统性竞态（约 25 字段写者 × 热路径无锁读者，撕裂读可致 401 抖动），需独立里程碑（快照交换/守卫访问器）。
- **网络（#1058）**：15 处出站调用点统一 `internal/httpclient` 基线（dial 30s/TLS 10s/idle 90s/池 100/20）+ AST 静态门禁拦截裸客户端。
- **数据库/性能（#1054）**：`sc2_027` 三个管理读路径索引（300k 行实测 channelId 过滤 17.9ms→0.5ms、siteId COUNT ~10x）+ proxy-logs LEFT→INNER、marketplace N+1 批量化；4 个候选索引实测无收益不加。
- **并发/健壮性（#1061）**：调度器审计——job panic 统一 recover 边界；in-flight 标志锁外读竞态修复；吞没 DB 错误显性化；重叠/关停/方言审计为无缺陷留痕。
- **方言（#1060）**：约 260 处测试种子 + 3 处生产位点 BOOLEAN 整数字面量重写；管理搜索 LIKE 大小写统一；静态方言门禁扩至全包；零迁移。
- **构建（#1053，S1+S9）**：rsbuild 唯一 SSOT（删 vite.config.ts），`web/config/build-shared.ts` 收口 devProxy/版本 define，route-tree 前置校验；匿名 6432 块拆 `vendor-i18n`/`vendor-icons`/`vendor-core`（总 JS −8.5KB gz）。
- **UX（#1055）**：focus-ring 全库 `--focus-ring` ≥3:1；登录 autoComplete 评估定论；图表 sr-only 摘要 + axe 组件门禁（#1029/#1035 残留）。
- **CX（#1056，closes #1027）**：健康监测全局开关（`checkinEnabled`/`balanceRefreshEnabled` 运行时设置 + 同名环境变量，热生效持久化），模型探测热停；双语入口 + 部署文档。
- **CX（#1059，closes #1009）**：账号代理支持 `socks5://`/`socks5h://`（后端原生支持验证 + RFC 1928 进程内 E2E）；清空代理保存即清除（前端载荷省略 + 后端 typed-nil 删除缺陷双修）。
- **发布收口**：10 PR（#1052–#1061）全部 required CI 绿后 squash 合入；release PR 随附 STATE/MASTER/log 更新。

## 2026-08-28 — Wave 17 竞品研究 P1×4 + 前端审计快赢 发布 v0.16.18

- **P1×4 落地**：#1046 SSE 流 chunk 间隔空闲超时（`PROXY_STREAM_IDLE_TIMEOUT_SEC`）；#1049 重试/禁用状态码策略运营者可调（运行时设置 + 严格区间校验，默认行为不变）；#1047 批量测试闭环（失败清单 + 一键禁用，`PUT /api/channels/batch` 部分更新语义与逐项真值）；#1048 渠道失败横幅一键过滤（`?status=` 可分享过滤）。
- **前端审计快赢批**：#1040 构建 / #1041 CSP 去 unsafe-inline / #1042、#1043 a11y / #1044 卫生 / #1045 UX 五批；#1036 zh-CN locale 崩溃修复；#1024 路由重建真值；#1039 搜索面板实体深链。
- **#1026 站点维度**：#1050 下游密钥「上游站点限制」UI（选择器既有 `allowedSiteIds` 暴露到管理界面）；凭证维度留 open 另行立项。
- **发布收口**：tag v0.16.18 pipeline 全绿（23 项检查 + 多架构镜像 + 5 平台二进制），GitHub Release 已正式发布（12 资产）。

## 2026-08-28 — Wave 16 竞品研究 P0×3 发布 v0.16.17

- **三线并行**（3 worktree + 3 subagent，写集无交集）：
  - **P0-1 协议转换快照回归套件（#1018）**：46 份手写夹具 + 快照锁死 transform 层——Gemini 请求转换矩阵、Responses 连续性/清洗决策表、响应与 SSE usage 提取四形状 + 增量流解析（含 7 字节分块边界）、completions/embeddings/images 恒等契约；新增快照测试 harness（`GOLDEN_UPDATE=1` 重写，纪律入 docs/testing.md）；零生产代码改动。
  - **P0-2 行级探测健康条（#1020）**：新增只读端点 `GET /api/channels/probe-history` 与 `GET /api/accounts/probe-history`（limit 1–50 默认 20，单条窗口查询无 N+1，实体列白名单防注入）；渠道/账号表行级健康条 + tooltip 成功率/延迟 + 键盘可达与 aria；双语齐全。
  - **P0-3 结构化冷却原因（#1019）**：`route_channels` 与 `oauth_route_unit_members` 各 +3 可空原因列（additive `sc2_025`，双方言 + 迁移工具携带 + OAuth 回滚恢复）；9 码只增词表与净化摘要（200 runes）；流量/探测双路径记录、全部清除点同步清原因；徽章可点击→根因弹窗（本地化触发码/摘要/记录时间/剩余倒计时/路由级清除），旧数据如实「原因未记录」；`GET /api/channels` 新增三 camelCase 字段，docs/api.md 同步。
- **附带修复**：#1022 加权选择测试去 flake（单次抽签断言→200 抽统计，与既有 fallback-penalty 测试同款模式；该 flake 在 docs-only 发布 PR 的 test-pg 首次现形）；#1023 发布 notes 卫生守卫命中内部词，CHANGELOG 措辞修订后重建 tag。
- **验收**：私有面运维 testbed 综合验收通过——版本注入（`metapi --version` = v0.16.17）、/ready、探测历史端点信封与 limit clamp、cooldownReason* camelCase 契约、SPA 资产，新功能 7/7 PASS；既有 smoke 13 PASS / 1 WARN / 0 FAIL（WARN = 上游零渠道、空模型为上游事实）。为使渠道行存在，经账号手工模型端点钉住 `gpt-3.5-turbo` 后重建路由；原始证据留私有面。
- **发布收口**：tag pipeline run 33133330480 全绿（23 项检查 + 多架构镜像 `:0.16.17`/`:0.16` + 5 平台二进制与 smoke）；GitHub Release 已正式发布，含 12 个资产。

## 2026-08-28 — Wave 15 issue 收口 发布 v0.16.16

- **#1005 定时上游模型同步**：`MODEL_SYNC_CRON`（默认 `0 4 * * *`）+ 设置页 `modelSyncCron` 运行时热更新（非法 cron 400）；单账号失败不中断整批，批量结束后路由重建与缓存失效恰好整体发生一次；手工单账号刷新端点行为不变（PR #1015）。
- **#1009 出站代理超时可配置**：五个 `PROXY_*_TIMEOUT_SEC` 环境变量（connect 2 / TLS 10 / 响应头 30 / 空闲 90 / 总请求 30），接入 pooled/uTLS transport 与 SiteProxy 客户端；未配置零行为变化；reset-by-peer 根因证据待报告者补充，issue 保持 open（PR #1013）。
- **竞品研究**：`docs/internal/analysis/competitor-study-2026-08.md`（new-api / axonhub / sub2api），P0×3 成为 Wave 16 候选（PR #1014）。
- **验收**：私有面运维 testbed 综合验收通过——版本注入、/ready、model-sync/balance 调度器启动与热更新、设置往返与 400 校验、真实上游 e2e smoke 13 PASS / 0 FAIL、模型刷新诚实空态（上游零渠道为独立核实事实）、SPA 资产；原始证据留私有面。附带发现并修复 testbed compose 的 4 字段无效 `BALANCE_REFRESH_CRON`（testbed 侧配置错误；产品按设计 fail-closed 告警）。
- **发布收口**：tag pipeline run 33109606314 全绿（23 项检查 + 多架构镜像 `:0.16.16`/`:0.16` + 5 平台二进制与 smoke）；GitHub Release 已正式发布，含 12 个资产。

## 2026-08-27 — Wave 14 账号表单与分页修复 发布 v0.16.15

- **#1007**：inline token verification 与账号创建现在使用表单中尚未保存的 `platformUserId` / `proxyUrl`；显式代理覆盖站点、Resin 与系统代理，创建后持久化 `extraConfig.proxyUrl`，并拒绝非法代理 URL。
- **#1008**：Accounts 表格由 URL 单一控制分页，关闭 TanStack 自动页码重置；新增真实表格回归测试，验证第 2 页渲染 11–20 行。
- **证据**：后端 NewAPI + HTTP proxy + `New-Api-User` 回归、前端分页回归、本地 frontend 191 文件/1,276 用例与 Go build/vet/WSL race 全部通过；PR #1010 的 required CI run `33068217016` 全绿并 squash 合入 `master`（`840d930`）。
- **发布收口（2026-08-27）**：tag pipeline run 33072276537 全部成功（含多架构 Docker、5 平台二进制与 smoke）；GitHub Release 已正式发布，含 12 个资产。

## 2026-08-27 — Wave 13 token sync UI 真值 发布 v0.16.14

- Lane A（#1002 UI 后续）：账号创建/登录 toast 如实报告后端 `tokenSyncStatus` 四态——synced 显示真实持久化计数、empty 提示无上游令牌、failed 降级为部分初始化警告（账号保留、令牌面板可重试）、skipped/旧响应保持原文案；所有状态保留 `/token-routes` CTA。
- 集成 lane 补齐 7 个 i18n 键（en + zh-CN 对称，含复数）。
- 证据：focused Vitest 28 例 + 前端全门禁 + 本地 CI（build/vet/WSL race）全绿；PR #1006 squash merge。

## 2026-08-25 — Wave 12B account bootstrap 发布 v0.16.13

- 两 lane 并行（captain 编排，subagent 执行）后按 F → G 顺序集成 `batch/w12-account-bootstrap`：
  - F（#1002）：session 账号创建/登录后自动同步 token（复用既有 `executeAccountTokenSync`）；四态真值测试（成功持久化 / 空=0 / 失败保留账号+部分初始化+event 行 / API-key 跳过）；移除硬编码 `tokenCount: 1`。
  - G（#998）：账号详情 Models 面板——上游刷新持久化可用性、手工行显式移除、来源/可用性诚实状态；刷新动作路由重建/缓存失效恰好一次；上游失败无副作用。
- 集成 lane 补齐 15 个 i18n 键（en + zh-CN 对称）。
- 发布链：Wave 12A → PR #1003 → v0.16.12；Wave 12B → v0.16.13；v0.16.10/v0.16.11 draft release 已发布。

## 2026-08-25 — Wave 12A demand truth 发布 v0.16.12

- 五 lane 并行（captain 编排）后集成 `batch/w12-product-truth`：
  - A：Sites 分页保持 URL 控制（#996）；页码重置后 page=1 仍渲染 11-20 行。
  - B：下游 key 模型策略编辑器（#999）——精确/通配/`re:` 正则规则，空即拒绝、`*` 显式全允许；后端过滤保持唯一执行点。
  - C：账号表单站点选择器可搜索（#1001）——名称/URL/平台搜索、键盘选择、数字 siteId、深链预选保留。
  - D：公告进 attention（#1000）+ 待办语义澄清（#997）——read_at 唯一读状态、审计事件不重复、外链只信本地站点 URL 解析出的 HTTP(S)。
  - E：截图扫描数据 profile 门禁——采集前声明 empty/seeded，错配在截图前失败。
- 集成 lane 补齐 14 个 i18n 键（en + zh-CN 对称，含 `_one`/`_other` 复数形态）。
- 上一会话 4110/4111 视觉证据被旧进程污染；已在 4120（seeded）/4121（确认 empty）用当前 master 重建，并重扫 112 张×2 矩阵。

## 2026-08-25 — Wave 11 UX 真值波发布 v0.16.11

- 四 lane 并行（captain 编排）后集成 `batch/w11-ux-truth`：
  - A：核验 #862 已交付 chain-context 名称解析与 toolbar 开关，补 9 个 focused 测试 + `showZeroChannel` localStorage 持久（真实缺口）。
  - B：accounts pin/check-in 行级 pending（成功 toast 基线已有，未重复加）；model-tester 对比行 re-run（含失败行，复用原 payload）。
  - C：Start-OAuth 呈现 state/instructions + `getOAuthSession` 有界轮询（30×2s，卸载清理）+ 手动回调 `submitOAuthManualCallback`；超限态诚实。
  - D：移除 `dangerouslyIgnoreUnhandledErrors`（零暴露）；a11y-scan 对齐 route-smoke 单一来源 41 路由，0 serious/critical，真修 3 处 aria；golden 4→10 页（WSL noto-cjk 基线，对照 10/10 过）。
- 任务书两处前提过期（#862/#948 已交付部分审计观察），执行者按实际缺口落地并明示——审计文件保留为历史证据，非当前 backlog。
- a11y residual（moderate/minor）：region / landmark 双 main / image-redundant-alt / catalog-sources empty-table-header，不强修。
- 集成验收：batch 全套 vitest 185 文件 1230 用例 + typecheck/lint/format/knip 全绿。

## 2026-08-25 — Wave 10 Sites demand batch 发布 v0.16.10

- Batch 分支 `batch/w10-sites-demand` 集成合入：#985 站点快捷跳转（http(s) 校验链接）、#986 `/site-announcements` 独立 SPA + camelCase 契约 + 诚实同步错误、SSRF dial-time 守卫（internal/ssrf）、newapi/donehub 公告信封校验。
- 配套修复：#991 pre-push 门禁链 + release freshness 门禁；#992 产品公告前端真值（链接策略/加载错误/dismiss 反馈）；高负载抖动的代理首字节计时测试修复；文档卫生门禁不再扫描 .dev-local。
- 联合验收：Go 全量 -race 全绿（WSL）、web vitest 1165 用例 + typecheck/lint/format/knip/build 全绿；版本随 PR bump 至 v0.16.10。

## 2026-08-24 — Wave 10 #981 迁移兼容修复

- 用户迁移数据中的 Sub2API `sub2apiAuth.tokenExpiresAt` 为 epoch milliseconds；站点摘要误用 `time.Unix` 按秒解释，生成超出 JSON 支持范围的年份，使 `/api/sites` 序列化失败。
- #987 将 managed token expiry 统一归一为 milliseconds，同时继续接受 legacy seconds；站点摘要改用 `time.UnixMilli`，Sub2API refresh lead window 同步为毫秒。
- 回归覆盖毫秒时间戳 JSON 序列化、秒级迁移兼容与 account merge；WSL Go 1.26.6 full `-race` 和 GitHub 12-check CI 全绿，squash 合入 `ffbdae1`，#981 自动关闭。

## 2026-08-24 — 计费货币整理（USD 收口）

- 明确计费货币：全链路 USD（`balance`/`quota`/`used`/`todayIncome`/`unitCost`/`estimatedCost`/每百万价），无 RMB / 多租户钱包路径。上游 NewAPI/OneAPI 族 `quota` 为整数额度（1 USD = 500000 quota；veloera 1 USD = 1000000 quota），platform 适配器在边界 `quota/divisor` 归一为 USD，前端永不接触原始 quota。
- 前端收口：`lib/format` 新增单一 `USD_SYMBOL`，`formatCurrency`（金额，千分位 + 固定小数位）与 `formatPrice`（每百万价，自适应或显式小数位）收敛全部货币渲染；删除重复/分散的 `formatUsd`、accounts `formatAmount`、token-routes `formatRoutePrice`、price-compare 本地 `formatPerMillion`/`formatSampleCost`、charts `$` 拼接。
- docs/api.md 新增 “Billing & Currency” 约定节，MASTER 已知残差 `$` 项改记为已收口。前端 typecheck/lint/format 全绿，vitest 171 文件 1131 用例通过。

## 2026-08-24 — Wave 9 发布 v0.16.9

- PR #972（Wave 9，mobile audit + entry-chunk split + contrast zero-exemption）squash 合入 master（9d864bf）。
- release PR #973（chore(release): v0.16.9，CHANGELOG 节 + web/package.json 0.16.9）squash 合入 master（592f1ba）。
- tag v0.16.9 推送 → CI/CD 全 12-check 全绿 + docker-push（linux/amd64+arm64，provenance+SBOM）+ release job 构建 5 平台二进制（10 个）+ checksums.txt + install.sh。
- GitHub Release v0.16.9 由 draft 发布（12 assets）；生产滚动部署为运维面后续动作（本仓只到 Release 收口）。

## 2026-08-24 — Wave 9 冻结恢复 + 集成

- 恢复 SOP 执行：a rebase 受阻（远端已推）改 merge 到 6f44088 → 重写两个 env-independent 不变式测试 → 375×812 Chromium 连续 5 次侧栏开合无冻结 → push；b 全前端门禁 + 41 desktop/19 mobile smoke + 旧 URL redirect + 拖拽持久实测通过 → push；c 门禁复核发现 models.dev catalog vendor 误标 Claude 为 openai 协议，修 dialect 优先 + 真实实例验证 anthropic → push。
- integration 分支（wave9/integration）c→b→a 顺序 merge + docs 分支 merge；a11y 15 路由 0 serious/critical、route-smoke clean、vitest 171 文件 1130 用例全绿、go test 全绿（Windows 本机 TSan 内存分配 + store 只读目录两处环境性失败除外，CI Linux 权威）。
- 前端卫生：删除死代码 formatDateTimeMinuteLocal（生产零消费）。
- Lane D 移动端深审：dom-audit --hard 全量核验，修 2 处 <24px 行内按钮（checkin 查看原始信息 / rates 行内编辑）→24px（min-h-6，Playwright 实测 20→24px + 点击交互回归）；其余硬信号逐条核验为误报（accounts 卡片按钮为 below-fold 裁剪、Switch/Checkbox 为伪元素扩展命中区、settings 侧栏为时序噪声、列头为负边距角点）。
- 首屏 bundle 拆分：locale 双份 JSON 静态 import → i18next 自定义 backend 懒加载（按当前语言拆 async chunk，切换时按需加载 sibling）；入口 chunk 303.6→123.5KB（gzip 90.7→33.1KB）；vitest 171 文件 1129 用例全绿 + a11y/route-smoke clean + 实例实测 en/zh 首屏无裸 key、切换正常。
- 对比度豁免收口：删除 dormant `--sidebar-primary`/`--sidebar-primary-foreground` token（全仓 0 消费，theme.css + 10 preset 共 42 行）→ 豁免表 8→6 项；剩余 6 项为 preset residual（forest-whisper dark secondary 3.12 / ocean-breeze light secondary 4.47 / simple-large+anthropic dark destructive 2.97+2.79 / anthropic light success 3.83+soft 4.48），留待设计决策。
- 对比度清零：6 项 preset residual 全部按「只降 lightness、保 hue/chroma」最小改色修复——forest-whisper dark secondary 3.12→4.60、ocean-breeze light secondary 4.47→4.60、simple-large dark destructive 2.97→4.60、anthropic dark destructive 2.79→4.60、anthropic light success 3.83→4.60、success-soft 4.48→4.61（新增 anthropic light `--success-soft-fg` override）；contrast-gate 豁免表归零，10 preset × light/dark 全过 AA。
- 集成交付：开 PR #972（Wave 9，26 commits，93 files，+3769/−2245）；版本号仍未动，待管理员批后 bump+CHANGELOG+tag。
- PR #972 12-check CI 全绿（vet / frontend / a11y / test-sqlite 4 shard / test-pg / vulncheck / secret-scan / runtime-smoke 3 平台 / docker-build）；待管理员 squash 合入 + 生产滚动部署 + 版本号。
- 冻结交接文档已消化（恢复 + 集成完成），随 integration PR 删除。

## 2026-08-23 — Wave 8 收官 + Wave 9 冻结交接

- #971（wave8/integration）squash 合并（6f44088）：模型数据源多源注册表（llm-metadata + models.dev，自动/手动同步）+ models 页水合 + fix-candidates 删页合并 + settings IA 重构 + 14 条产品语义修复。
- 生产实例滚动部署 master 6f44088（digest d75bf354，23:19，免备份直上）；10 分钟 soak 0 错误；生产验证 2235 模型双源合并、水合近半。版本号不动，攒波待批。
- **Wave 9 三线冻结**（用户指令全员停止）：交接与恢复 SOP 已消化入本文件 + MASTER.md，不再单列 handoff 文档。

## 2026-08-23 — Wave 7 收官：合并 master + 生产滚动部署（未发版）

- #970（wave7/integration）squash 合并（4802ffa）：55 修复 + 2 授权跳过 + 治理改动（Release draft + notes 卫生门禁 + CHANGELOG 19 段公开化重写）。
- 集成期修复：CI lint/knip/typecheck 三轮、站点移动卡 l3a 合并覆盖回归（四字段标签实测回归）、golden 基线刷新。
- 检查矩阵（4 agent）：60 条 finding 算术闭合无无归宿；49/49 lane commit 零丢失；仓库卫生清零；发布就绪缺口仅剩 bump+CHANGELOG+tag（均停手待批）。
- 生产实例滚动部署 master 4802ffa（digest b0e229cf，19:35，免备份直上）；版本号不动，攒波待批。
- 19 份历史 Release notes 全部改写为公开安全版（P0 漏洞利用细节/内部覆盖率数字/波次术语清除）；Release 流程改 draft + 黑名单门禁（防复发三道闸）。

## 2026-08-23 — Wave 7 前端体验波启动（目标 v0.16.9）

- P0：clickability 门禁（`aeda771`，遮挡中心+四角检测 + 24px 命中区，发现 81 条真实硬失败）；种子实例 ×3（4100/4101/4102）；桌面 82 + 移动 28 张预采集截图。
- 竞品研究：TS 原版 20 条退化清单（disabled-models API 零消费等）+ new-api 25+ 模式目录（同源分叉；go 版超越点：dirtyDialogClose/⌘K 双层等）。
- P1：12 页组实测审查 + 对抗核实（24 sonnet 多模态 agent）→ **60 条确认**（1 P0：侧边栏「路由」导航静默失败；16 P1；43 P2）/ 3 拒绝 / 6 重复合并 / 2 条撤销（实例未重建 dist 的构建缺陷，SOP 已补）。
- P2：12 条修复线并行（worktree 隔离，全 sonnet）。

## 2026-08-23 — Wave 5+6 → v0.16.8

- Wave 5 功能线（#935 #861 编辑器 / #939 #558 探针 / #938 #926 英文化 / #936 审计残留 / #937 测试矩阵 / #951 截图双管道）+ Wave 6 六维深审计线（22 项坐实全修复：#941 P0 备份凭据植入 / #942 全失败告警 / #943 可空列同族 / #944-#947 性能 / #946 后端卫生 / #948 UX 动线 / #949 对比度+守卫 / #950 前端卫生 / #940 api.md 74 条），15 PR 全部 squash 合入，开放 issue 归零；发布 #954 → v0.16.8（Release 12 assets）。
- 测试矩阵：CI 真实上游 2/16→4/16（+sub2api/+cliproxyapi 容器链）；`runtime-smoke-matrix` 4 平台（ubuntu/macos/windows/arm64）运行时冒烟；截图证据管道 + 4 页 golden 门禁。

## 2026-08-22 — Wave 4 综合质量波 → v0.16.7

- Wave 4 首轮 9 条 PR（#916 契约回归 / #917 安全审计 / #920 a11y / #921 部署体验 / #922 安全移交 / #919 迁移兼容 / #918 实测验收 / #924 残留打磨 / #925 A/B 域重审）+ 本轮 4 条（#928 空快照 / #929 对外门面 / #930 三处假成功 / #931 NULL 余额 500）全部 squash 合入；Round 3 审计 8 域闭环，#915 关闭。
- #933 可空余额列根治（`store.Account` 数值列 `float64 → *float64`）关闭 #927。
- 发布：#932 release → v0.16.7。

## 2026-08-21 — Round 3 修复波 → v0.16.6

- **D 域 4 个 P0 契约 bug（#911）**：account-token snake_case 丢失 / site-account-route 表单静默丢弃 / channels 截断 50 / route channels 缺 success-fail-cooldown，双方言回归测试。
- **H 域性能（#910）**：静态资源 gzip（stdlib）· accounts 100 行 2s 冻结（parse-once + memo）· dashboard recharts 按需加载。
- **Spinner 双轨收口（#912）**：21 文件 56 处裸 Loader2 统一到 canonical Spinner。
- 发布：#913 release → v0.16.6。

## 2026-08-21 — #887 补遗收口 + E2E Journey 3 → v0.16.5

- #902 全局告警红点 / #903 OAuth sheet / #904 About 构建信息 / #905 proxy-log channel 过滤 + channels status facet / #899 E2E Journey 3 / #901 死 CSS / #906 docs-sync，7 PR squash 合入；#907 release → v0.16.5 发布。
- 抢救 list-filter 未提交工作（commit + rebase）→ #905；清理 9 stale 本地分支 + 3 空壳 worktree；落地「主 checkout 只挂 master + worktree」工作流 + `post-checkout` 告警 hook。

## 2026-08-21 — UI/UX Round 2 收口（v0.16.4）+ Round 3 审计启动

- **Round 2 #889 八域 → #898 squash 合入（`1604f49`）→ v0.16.4 发布**：交互/移动端/视觉/IA/呈现/性能/主题/会话八域并行修复，含 oxfmt CI 门禁。
- **#901 死 CSS 清理（`4cf12fd`）**：落地页移除后的零引用 CSS 残留。
- **Round 3 审计启动**：运行时/契约/代码健康/产品能力/性能/安全 8 域，A/B/D/E/F/H 完成、C/G 因 provider 故障中断；产出 D 域 4 个 P0 契约 bug + H 域性能（零 gzip / accounts ~2041ms 冻结 / recharts 332KB 白下载），转 v0.16.6 立项。

## 2026-08-21 — UI/UX Round 1 #887 → v0.16.3

- 四路审计立 #887，六 PR（#890–#896）并行修复 + #897 发布 v0.16.3。详情见根 `CHANGELOG.md` v0.16.3。

## 2026-08-20 — TS 兼容与迁移收官：CLI 诚实化 + 反向检测 + 迁移 UI + 备份兼容 + 三场景文档

一次综合设计解决 TS 版兼容与迁移的全部已知问题（技术 + UX），分析依据 `docs/internal/analysis/ts-go-migration-gap.md`，五个 PR 并行交付（#880 计划 / #881 后端 / #882 UI / #883 备份 / #884 文档）：

- **CLI 诚实化（#881）**：`--verify` 校验和失败现在非零退出（此前只 warning、exit 0——脚本里等于没校验）；顺带修复一个「假绿」测试（NULL 被强转默认值吞掉校验和不匹配）。`buildSummary` 按目标报真实方言（不再硬编码 postgres）；`--batch-size` 从「接受但丢弃」变为真实批量 INSERT。
- **反向检测（#881）**：防 hb0730 事故翻版——TS 库比 Go 认知新时启动即警告（读 `__drizzle_migrations` 判龄 + 以内存库 AutoMigrate 为权威清单扫未知列，列出列名 + 建议升级），永不阻断。干净库不误报（4 用例测试）。
- **启动汇总（#881）**：AutoMigrate 有实际变更时输出 `store: converged legacy schema` 汇总。
- **管理 UI 数据迁移（#882）**：此前被雪藏的迁移能力（后端 task 轮询早已就绪）接入 database-section——危险确认 + 2s 轮询进度 + 结果统计；`migrateExternalDatabase` 死代码复活为类型化 API；i18n 双语全量。后端补解析 overwrite 字段（#881）。
- **TS 备份 v2.1 导入兼容（#883）**：Go import/preview 兼容 TS 原版备份 JSON（11 节 camelCase→snake_case 完整映射，FK 安全序单事务，未知字段忽略并计 warnings，settings value 重新 marshal），作为迁移最后兜底路径。
- **文档三场景（#884）**：migration.md 重写——场景 A（TS SQLite 直接接管）/ B（TS PG 直接接管）/ C（TS MySQL 两段式：TS 管理界面自带迁移迁到 SQLite/PG 再接管）+ 版本锁定 + 故障排查；消除 README「metapi-migrate 支持 MySQL」的谎言与 FAQ 矛盾。
- **PG 直接接管实测**：真实 TS 服务器在 PG 16 建 27 表 + 种子数据 → Go 直接接管：`/ready` OK、`/api/sites` 返回 TS 数据、additive 在 PG 上逐列收敛无崩溃。

## 2026-08-20 — 仓库店头重建：docs 公私分离 + README/Diátaxis 文档集 + llms.txt（#872 #873）

对照原版 TS 仓库（cita-777/metapi，已 clone 到本机对照）与 2026 GitHub 开源最佳实践（Diátaxis 文档四象限、紧迫度递减 README 序、徽章 3-5 个上限、死链 CI 自动化、llms.txt）重做仓库对外门面：

- **docs 公私分离（#872）**：按受众边界重组 `docs/`——用户/贡献者文档留在 `docs/` 根（api/architecture/deployment/migration/testing/assets），维护者流程文档全部移入 `docs/internal/`（STATE/log/MASTER roadmap/benchmark/git-workflow/residual 说明/design/analysis/progress）。内部上下文与对外文档结构性隔离，不再可能渗进店面。门禁测试（`docs/doc_hygiene_test.go`）把边界变成机器断言：`TestStorefrontDoesNotLinkInternalDocs`（README 禁深链 internal）+ `TestRelativeMarkdownLinksResolve`（全站 markdown 相对链接必须可解析，抓出 1 个存量死链）。全部交叉引用同步（AGENTS.md/web/AGENTS.md/CONTRIBUTING.md/docs 地图/Go doc 指针）。
- **对外店头重建（#873）**：README.md/README_EN.md 重写——hero + 徽章裁剪到 5 个、8 张 retina 截图墙（2x DPR webp，`screenshot-scan` 加 DPR 支持）、痛点表、copy-paste 快速上手、首个 `/v1` curl 示例、精简配置节指向新 reference。补齐 Diátaxis 四象限公开文档：`getting-started.md`（tutorial）/ `configuration.md`（reference，完整 env 矩阵）/ `client-integration.md`（how-to：Cursor/Claude Code/Codex/Open WebUI + 配置导出）/ `faq.md`（诚实问答）。根目录 `llms.txt`：面向 coding agent 的精选索引（一句描述 + 纯 markdown 链接，内部文档明确排除）。顺手清除 `.env.example` 指向已不存在 analysis 文档的残留指针；仓库 topics 已设置为 ai-aggregation/api-gateway/go/llm/metapi/new-api/one-api/openai/react/self-hosted/single-binary。

## 2026-08-20 — 工程与测试基线加固（#866 #868 #869 #870 #871）

- **Dependabot 安全告警清零（#866）**：golang.org/x/crypto `0.36.0 → 0.55.0`（15 条告警，含 7 critical——2026-06 ssh 包 CVE 系列）+ electron `^31 → ^39.8.10`（31 条，仅可选桌面壳 devDependency），仓库 46 条安全告警全部关闭。
- **route-smoke 滚动补强（#868）**：`loading='lazy'` 图片在首屏外不触发请求，导致品牌图标 CSP bug 在 CI 烟测中漏检。现每页加载后、断言前滚动一遍，懒加载资产请求必发，CSP 拦截/加载失败以 console error 形式被 CI 捕获。
- **i18n 残留本地化（#869）**：36 路由 zh-CN 全量扫查修复 3 处 raw 内部值泄漏——observability 慢请求表 `success/failed` 复用 proxy-logs `StatusBadge`；program-logs 表格 level 徽章（`warning/info/error`）与事件类型 cell 走新增 `programLogs.level.*` + 既有 `type.*` key。
- **TokenRouter 公共面覆盖（#870）**：deep-test 审计标记的 0% 覆盖收口——`routing/router_test.go`（ChannelSelectorDB fake：构造默认值/GetAvailableModels 去重与通配名覆盖/上下文长度 max-wins/SelectChannel 委托）+ `pricing_cost_test.go`（别名等价/逐调用成本/配额组乘数）+ StatusBadge 标签。
- **dom-audit 硬/软门分离（#871）**：`dom-audit.mjs` 重构为确定性硬信号门（console errors/pageerrors/HTTP 5xx/横向溢出 → exit 1）可在 CI 当门禁，软启发式（小点击区/截断/对比度/重复 id）仅全量模式输出；合并两个一次性逐路由探针为参数化 `probe-page.mjs`（`ui:probe-page`），移动端路由 pass + 行菜单→详情 sheet 交互 dump。

## 2026-08-19 — Real-e2e UIUX audit: self-hosted brand icons + audit tooling

真实启动服务器(种子数据:6 站点/10 账号/7 路由/60 代理日志/16 签到)+ Playwright 全路由截图(light/dark × desktop/mobile,110 张)+ DOM 级审计脚本驱动本轮修复:

- **品牌图标自托管(CSP 修复)**:`BrandIcon` 原从 `registry.npmmirror.com` CDN 加载 55 个品牌图标,但生产 CSP `img-src 'self' …` 拦截该域名 → 所有品牌图标在模型/路由等页面全部回退为字母头像(DOM 审计抓到 5+ 条 console CSP 违规)。现下载 110 个图标(暗/亮 96×96,460KB)本地化到 `src/assets/brand-icons/icons/`,静态导入索引(`icons.ts`)+ `getBrandIconUrl` 改本地解析;`rsbuild.config.ts` 设 `dataUriLimit: 0` 防小图标内联成 `data:` URI(同样会被 CSP 拦截)。验证:模型页 6/6 图标真实加载、0 console 错误。
- **空态文案对齐**:账号页空态描述「或先到「站点」页添加站点」与 CTA「导入站点」不一致 → 统一为「或先导入站点」(zh/en)。
- **小打磨**:表格「查看」列开关按钮加 `Columns3` 图标;设置总览子区链接 `py-1`→`py-1.5`(更大点击区)。
- **审计工具链入库**(`web/scripts/`,knip 可见):`screenshot-scan.mjs`(全路由截图)、`dom-audit.mjs`(溢出/截断/对比度/空标签/console 错误审计)、`fetch-brand-icons.mjs` + `generate-brand-icon-index.mjs`(图标再生成)、`verify-brand-icons.mjs`(渲染验证);package.json 加 `screenshots`/`ui:audit`/`icons:fetch`/`icons:verify` 脚本。
- **验证**:tsgo 0 error · oxlint 0 error · oxfmt green · knip clean · vitest 637/637 · 服务器 `go build` 干净。

## 2026-08-19 — Deep backend testing: 6 defect fixes (balance corruption + lost updates + routing edge cases)

Deep-test audit (full suite + race + static analysis + concurrency/SQL review) surfaced 6 real defects, all now fixed with regression probes:

- **HIGH — Sub2API balance refresh corruption**: `platform.Sub2ApiAdapter.GetBalance` swallowed the `/api/v1/auth/me` fetch error and returned an empty `BalanceInfo`; `RefreshBalance` then wrote `balance=0/quota=0` and flipped `expired`→`active` on a transient upstream failure. Now surfaces the error (nil `BalanceInfo`) so refresh aborts instead of corrupting stored data; `VerifyToken` keeps a non-nil balance for its best-effort path.
- **MEDIUM — account extraConfig lost updates**: `PUT /api/accounts/{id}` read-merge-write of `extra_config` was non-atomic — 16 concurrent patches lost up to 12/16 keys. Now wraps read→merge→write in a transaction with a `FOR UPDATE OF a` row lock on PostgreSQL (SQLite serializes via its single-connection pool).
- **LOW routing edge cases (4)**: mixed-case `re:`/`RE:` regex prefixes were accepted but never stripped (compiled with the prefix, matching nothing); glob `?` matched one byte instead of one rune (broke non-ASCII model names); the hand-rolled JSON mapping parser skipped `\uXXXX` escapes; int64 overflow in usage-total repair now saturates instead of silently disabling the total≥components invariant.
- **Regression probes**: 3 deeptest probe files (routing matcher/mapping/usage, concurrent extraConfig, Sub2API balance) added as permanent tests. All 38 packages pass non-race; affected packages pass under `-race`.

## 2026-08-19 — v0.17 onboarding polish + per-key usage + positioning honesty

三线并行交付（platform picker / client export / downstream-key usage）+ 定位诚实性修正，基于 PM 视角分析与竞品对标（new-api/octopus/sub2api/all-api-hub）产出的机会清单：

- **Platform picker**：site-form `platform` 自由文本 → 16 适配器可搜索 Select（openai..one-api）；auto-detect 保留；未知平台经「手动输入」切换保留（诚实规则）。3 测试。
- **Client export 广度**：新增 claude-code / codex / openwebui profile（backend `buildCredentialExportProfiles` + export dialog）。Go 测试扩至 6 profile。
- **Downstream-key 24h 用量**：`proxy_logs` 按 key 聚合 requests/tokens/cost（单条 GROUP BY 查询 + 内存 join，dual-dialect `db.Rebind`），keys 表 usage cell 呈现。Go + 前端测试。
- **定位诚实性**：benchmark.md「唯一做真实流量转发」最高级修正（New API 也转发）→「唯一做跨网关聚合转发」；MASTER.md 版本漂移 v0.15.3→v0.16.1；README_EN 加「One key for every AI gateway」主张。
- **验证**：go build/vet + admin/docs 测试 + typecheck/lint/knip + i18n parity 8/8 全绿。

## 2026-08-18 — a11y 拡差核实：axe 全绿 + 菜单 Esc 行为钉死（a11y-checklist 卫生）

- **活体扫描**：`bun run a11y:scan`（Playwright + axe-core，dev-admin 会话）15 条认证路由 0 serious/critical 违规；结果进 `a11y-checklist.md` §1 AC。
- **残差 #2（菜单 Esc）核实为 stale**：header 语言菜单（Base UI `DropdownMenu modal={false}`）与外观定制（`Popover`）原生支持 Esc 关闭；补 2 个行为用例钉死（`interface-controls.test.tsx`），防原语替换静默丢行为。§7 移除该项。
- **清单卫生**：§3.1 删除已不存在的 `Avatar menu` 行、§2.1 删除 stale `Profile modal` 并修正 topbar Tab 顺序描述。
- **验证**：vitest 布局/行为用例全绿（interface-controls 4/4）· tsgo/oxlint/oxfmt green。

## 2026-08-18 — 四轮复审驱动：token-routes siteNames 死列 + 列表/详情打磨

四轮 PM/工程师/用户复审（token-routes list/columns/detail，避开 form dialog——并行进程 `route-form-drafts-hint` 拥有；发现写入审计 doc `### 四轮复审`）后交付：

- **Routes siteNames 死列修复**（gap-1 P1，#854）：`listSummary` 硬编码 `siteNames: []string{}` → 「Sites」列 + detail 恒为 `—`，全局过滤也搜不到 site 名。修复：加批量 `GROUP BY (route_id, site_name)` JOIN（route_channels→accounts→sites），dedup per route，nil→`[]string{}`；后端测试覆盖 linked（dedup）+ empty。Dual-dialect 安全。
- **Routes 列表/详情打磨**（gap-2/3/4/8/9，#855）：删误导性行「Refresh decision」（实为全局，与 header 冗余）；first-run 空态加 CTA（Add route + Auto-rebuild）；error banner 加 Retry + 抑制 table；detail「Rebuild」改「Rebuild all routes」+ 图标 `ExternalLink`→`RefreshCw`；`requireChannelAllocation` render-path throw → `resolveChannelAllocation` graceful fallback（防 refetch race 崩整页）。546 测试。
- 四轮 token-routes 共 11 gap：6 已收口（#854 后端 + #855 前端），5 暂缓（chain-context #ID→name / detail edit callback / per-row pending——均轻触 form dialog 需协调；showZeroChannel 布局 + barrel stub——P3）。
- **Sites gap-11 收口**（#853）：`postRefreshProbeLatencyThresholdMs` 加 number FormField inside `probeEnabled` block，probe 配置表面完整（model + scope + threshold）。543 测试。

## 2026-08-18 — 三轮复审驱动：availability WS 重连 + sites 表单数据丢失修复

三轮 PM/工程师/用户复审（sites feature，此前未深覆盖；发现写入审计 doc `### 三轮复审（sites）`）+ availability Gap 3 收口后交付：

- **Realtime WS 重连 affordance**（#850）：`useRealtimeOps` 重构为 `{ sample, reconnect }`（稳定 `useCallback`，经 `connectRef` 重入 `connect`，reset `failsRef`/`backoffRef`/socket）；`gaveUp` 态渲染「Connection lost — Reconnect」notice 替代归零 metrics（原看起来像「无流量」的 incident-handling gap）；`min-h-[8rem]` 防塌缩 + 3 测试。
- **Sites 表单静默数据丢失修复**（gap-1 P1，#851）：`customHeadersOverrideRequestHeaders` 在 `siteToFormValues` 硬编码 `false` → 编辑站点名等不相关字段会静默把 header-merge 从「site-wins」降级为「request-wins」。修复：round-trip 真实值 + 加可见 Switch FormField（label + hint）；`Site` type 加 optional 字段。
- **Sites 批量打磨**（#851）：i18n 限额文案对齐 Zod（120/64）；error 态抑制空态 CTA + 加 Retry 按钮；`platform` placeholder 改「Enter a platform」；`SiteCreatedModal` 迁移 typed navigate；加 `sites-page` + `site-form-dialog` 测试（7 例）。
- 三轮 sites 复审共 12 gap：6 已收口（#851），6 暂缓（endpoints editor feature gap / a11y label linkage 跨 feature / probe-now surfacing 跨 models / edit 深链 + retry shared 组件 + latency threshold 字段，均低优先级或需协调）。

## 2026-08-18 — 二轮复审驱动：onboarding 闭环 + model-tester 结果清晰度 + availability 健康可视化

二轮 PM/工程师/用户复审（model-tester / oauth / availability，发现写入审计 doc `### 二轮复审`）后交付：

- **Dashboard onboarding banner + sites `?create=1` 深链闭环**（#838 + #842）：`siteCount===0` 时 brand-tinted `<Card>` + 「Create site」CTA；sites-page 加 `create` schema + 一次性消费 strip（镜像 accounts `?create`）；overview CTA 改 `<Link search={{ create: true }}>` 直达 create dialog。闭合首次落地 dead-end。
- **Model-tester 结果清晰度**（#840）：`parseUsage` 从 upstream body 解析 token 用量跨四协议（cost 不在 wire 上，harness 绕过计费——未伪造，记 residual）；删 always-dead `chunks: 0` stat；failed run 抑制 empty badge 只显 error。516 测试。
- **Availability realtime sparkline 健康分档着色**（#846）：success-rate 按 healthy/degraded/unhealthy/idle 着色（原单色 `bg-chart-1/70`）+ `role="img"`/`aria-label`。Latency 不在 realtime wire 上（后端 `RealtimePoint` 无 latency）→ 未伪造，记后端 residual。
- **Attention 项 `createdAt` 相对时间**（#847）：`Intl.RelativeTimeFormat` via `toBcp47`（零新 key，en/zh-CN 自动本地化）+ `<time dateTime>` + absolute `title` tooltip。`formatRelativeTime`/`formatAbsoluteDateTime` 入 `lib/format.ts` 共享。
- **OAuth 二轮**（并行进程 #844/#845）：start-dialog provider 三态分支 + refresh/rebind 行级 pending + per-account error。
- **验证**：go build/vet 绿；web tsgo/oxlint/oxfmt format:check/vitest 全绿（529→531 测试）；GHA CI 全绿（#832 + #847 首跑 golangci-lint schema 拉取超时 flaky，rerun 后绿）。
- **工作流**：多 worktree 并行 + explore 子代理二轮深覆盖（model-tester/oauth/availability）+ 暗卷独立复跑聚焦测试。避让并行进程的 badge-feature 文件；2 处（oauth Gap 4/Gap 1）与并行进程 #844/#845 重复→本地丢弃未强推。剩余 entangled 项（proxy-log drilldown / WS 重连 / oauth quota 列）写入审计 doc 供协调。

## 2026-08-18 — 多角度复审驱动：动线 dead-end 收口 + proxy-logs 过滤服务端化 + downstream key edit

三轮 PM/工程师/用户只读复审（发现写入审计 doc `## 2026-08-18 多角度复审`）后，按 backlog 交付：

- **Dashboard stat 卡 drilldown + 凭证导出测试链**（#828）：`StatCard` 加 `to` → `<Link>`（四卡接线 `/accounts`/`/sites`/`/checkin`/`/proxy-logs`）；`credential-export-dialog` footer 加「发送测试请求」`<Link to='/model-tester'>`，闭合 onboarding 旅程最后一步 dead-end。503 测试。
- **Proxy-logs 列表过滤服务端化**（功能 bug，#832）：`latencyMin`/`latencyMax` + 原 silent no-op 的 `client`/`from`/`to` 全部移入 `statsHandler.proxyLogs` 共享 `where`/`args`（items/count/summary 一致，`rebindAdminQuery` 双 dialect 安全），删客户端过滤 memo；6 后端 + 3 前端测试。
- **Settings 边缘态硬化**（#832）：keys query 错误 → `SettingsSectionError`；enable `Switch` pending 禁用 + 唯一 `aria-label`；空态内联「Create」按钮；`update-center` 错误 → `SettingsSectionError`。506 测试。
- **Downstream key edit mode**（#835）：`KeySheetForm` 加 `editingKey`，`editKeySchema = createKeySchema.omit({ key })`（secret 不可改），调 `api.updateDownstreamApiKey`（PATCH partial update，`key`/`description` 省略以保留）；Pencil 行按钮 + 4 测试。
- **Price-compare → routes 深链**（#835）：`PriceRow` 加 `<Link to='/token-routes' search={{ q: row.model }}>`（routes 页 `q` 匹配 `modelPattern`）+ 4 测试。514 测试。
- **验证**：go build/vet + handler/admin 测试绿；web tsgo/oxlint/oxfmt format:check/vitest 全绿；12 项 GHA CI 全绿（#832 首跑 golangci-lint schema 拉取超时 flaky，rerun 后绿）。
- **工作流**：多 worktree 并行 + explore 子代理先核实审计真伪（proxy-logs 过滤 bug + client/from/to silent no-op 均经核实）+ 暗卷独立复跑聚焦测试。避让并行进程的 badge-feature 文件（accounts/checkin/routes/channels 列）。

## 2026-08-18 — UI/UX 批次：账户行内操作 + header SSOT + skeleton shimmer + 徽章机械迁移

- **P2 #5 收口**：导入向导 focus-first-invalid 补 4 回归用例 + 2 处 `curly` lint 修复（#824）。
- **P1 #3 行内高频操作**：accounts 行内 Enable/Disable 按钮（`Power` 图标 + 每行 pending via mutation `variables`，无全局锁，复用现有 i18n），下拉菜单保留（#824）。
- **P2 #4 header 高度 SSOT**：`app-header` `h-14` → `var(--app-header-height)`，删 2 处冗余 inline re-declaration，加静态守卫 `header-height-ssot.test.ts`；圆角半 stale 不动（#824）。
- **P2 #7 skeleton shimmer**：唤醒休眠 `--skeleton-highlight` token（`.animate-shimmer` 渐变 + reduced-motion gate，替换 `animate-pulse`）；`table-skeleton` 按 `column.getSize()` 取宽替换固定百分比池；`no-gradients` allowlist 加 `index.css` 例外（#824）。
- **P1 #1 徽章机械迁移**：3 个手写 `<span>` 徽章 → `<Badge>` 语义变体（overview `SCHEDULER_STATUS_BADGE` + availability `SEVERITY_TONE`，map `className`→`variant`）（#825）。剩余 7 处 `variant='default'+dot` 需设计决策，按 feature 分批。
- **工作流**：3 worktree 并行开发（`wave-2-2`/`wave-3-2`/`wave-3-3`）+ explore 子代理核实审计真伪（P2 #4 圆角半 stale、P2 #7 valid）+ cherry-pick 合并为 PR #824；徽章机械迁移为 PR #825。
- **验证**：tsgo 0 error · oxlint 0 error · oxfmt `format:check` green · vitest 500 全绿 · `go build`/`vet` clean · 12 项 GHA CI 全绿。

## 2026-08-18 — 导入向导 focus-first-invalid 回归覆盖 + lint 收尾

- **P2 #5 收口**：`import-wizard-dialog.tsx` 的 focus-first-invalid（e1991ef 已落地 `markInvalidAndFocusFirst` + `aria-invalid` + per-field clear）补 4 个回归用例（source empty / identify missing platform / routes invalid weight / clear-on-edit），并修两处已提交 onChange clear 的 `curly` lint 错误（无行为变更）。审计观察 P2 #5 导入向导部分状态改「已交付（代码）」；`account-form-dialog.tsx` 等其他表单仍为观察。
- **验证**：tsgo 0 error · oxlint 0 error（wizard 文件）· vitest 486 全绿（`import-wizard-dialog` 9/9）。

## 2026-08-17 — v0.15.x Resin per-site + 弹窗视口合约 + 设计系统溢出安全

- **Resin per-site（v0.15.0）**：站点表单新增 `resin_enabled`/`use_utls` per-site tri-state 覆盖（继承 / 强制开 / 强制关），CreateSite INSERT 补齐两列、UpdateSite `jsonKeyToColumn` 补齐映射、`SiteSelectColumns` 补齐 `use_utls` 读回；`ApplyRuntimeSettings` 补齐 `smtp_secure`/`notify_cooldown_sec`/`telegram_*`/`system_proxy_url` 六个读侧 case，持久化设置不再重启静默回退（#807/#809）。
- **导入流程 UX 收口（v0.15.0）**：per-item 失败原因渲染、URL 检测失败时 toast 防误炸（`skipErrorHandler`）、`aria-busy`/`aria-live` 无障碍区域、label `htmlFor`/`id` 关联 + Switch `aria-label`、`ImportSiteItem.duplicateStrategy` 类型漂移修复（#808）。17 个前端测试 + 14 个后端测试。
- **PG BOOLEAN dialect gate（v0.15.1）**：`stats_marketplace.go` 等 15 处 SQLite-only `COALESCE(<bool>, 0) = 1`/`<bool> = 1` 在生产 PG 报 SQLSTATE 42804，改为双 dialect 通用 `false`/`true`；`tokenCandidates` handler builder `err` 静默丢失改 `slog.Error` + `writeErrorWithRequest` 带 request_id；新增 `docs/pg_boolean_gate_test.go` 静态 gate 覆盖全部 16 个 BOOLEAN 列 + 真实 PG integration test（#805）。
- **弹窗视口合约（v0.15.2 + v0.15.3）**：`DialogContent` 无高度约束致长表单溢出视界、提交按钮不可达 → 补 `max-h-[calc(100dvh-2rem)]` + `overflow-y-auto` + `flex-col`，`DialogFooter` `sticky bottom-0`；v0.15.3 扩展到 `AlertDialogContent`/`PopoverContent` + `DialogHeader` sticky top + footer 不透明 `bg-popover` 防穿透；`site-form-dialog` 补 `onInvalid` handler + i18n key；新增 `dialog-viewport.test.ts` 静态护栏（#815/#822）。
- **nginx 反代 WebSocket（v0.15.1）**：`docs/deployment.md` 补 `proxy_http_version 1.1` + `Upgrade`/`Connection $connection_upgrade` map 模式，防 `wss://` 握手在代理层被掐断。本地 Vite dev server 见下条「产品品牌升级」。

## 2026-08-17 — 产品品牌升级（Metapi 改名 + logo + 登录 UI）

- **品牌改名**：MetAPI → Metapi 机械改名跨 62 文件（注释、用户文案、docs、i18n、SVG aria-label、electron、脚本、测试）；wire 标识（`modelsOwnedBy="metapi"`）、env var 名、行为零变更。
- **logo 系统**：圆角徽章文本 π 换成透明底 π 字形 + 蓝青渐变 SVG（亮/暗双主题可读，单资产免切换）；栅格化 logo.png(512)/favicon.png(32)/favicon-64.png(64)，重生成 desktop-icon/desktop-tray-template；`generate-icons.mjs` 去掉徽章时代的圆角裁剪以免切掉无徽章字形。
- **登录页**：标题升 `text-3xl font-bold`，删冗余脚注段；README 顶部加透明底 hero banner。
- **本地开发**：新增 `vite.dev.config.ts` + `@tailwindcss/vite`/`@tanstack/router-vite-plugin` devDeps，Vite dev server 跑 :5173（index.html module script 仅 dev 用，生产仍由 Rsbuild 注入）。
- **验证**：go build/vet 绿；web tsgo typecheck + oxlint 0 error。

## 2026-08-17 — v0.14.0 发布收口

- **交付闭环**：引导链、测试台真值、路由成本真值、URL 单一所有者与 Chromium smoke 全部合并；#800/#801/#802 CI 全绿并合入 master。
- **知识卫生**：Wave 2–3 从 MASTER 毕业，旧 #782 关闭为 #802 的 superseded，剩余开放项仅 #558 的 operator-gated 真实探针。
- **发布**：准备 v0.14.0，版本、CHANGELOG、STATE、MASTER 与 release pipeline 对齐；生产升级继续遵循 active production compose plane 的镜像 pin + soak + 四层验证门禁。

## 2026-08-16 — 路由成本真值 + 恢复可靠性 + 知识收口

- **路由/计价**：下游暴露三种策略；补 usage 缓存明细缺失时的全价计费、models.dev 冷启动价格目录、成功衰减 failCount 与 breaker half-open 探测（#783/#785/#788/#790/#791）。
- **可靠性**：SQLite 默认调优；usage aggregation flush 失败时保留 delta/watermark，避免静默丢统计（#784/#789）。
- **发布与知识卫生**：明确 #767–#791 位于 v0.13.0 之后并归入 Unreleased；删除已完成 WEB-ARCH 计划和 agent handoff，STATE/MASTER/benchmark 收敛为 3 条交付主线 + 1 条工程基线（#793 + 本轮）。

## 2026-08-15 — 真实平台测试战役：测试床 + 6 个实测 bug 修复 + CI e2e

- **测试平台（真实上游，compose 管理）**：临时 ARM 机跑 metapi + new-api v1 + one-api v0.6.10 + sub2api + cliproxyapi 7 容器；私有层（host-local git + `.env` chmod 600）与公开层（`testbed/compose.template.yml` sanitized 模板 + env 驱动脚本）隔离，主机 IP/凭据不进公开仓；设计/SOP 现位于 [`testing.md`](../testing.md)
- **实测修 6 个真 bug（全部真实平台端到端复测通过）**：
  - #767 前端 10 路由严格 validateSearch 在旧 URL 参数下抛错 → error boundary「服务器错误」
  - #768 new-api v1 登录响应无顶层 `success`（token 在 `data.access_token`）
  - #769 one-api v0.6.10 与 new-api 的 `/api/status` 都带 `system_name`+`version`，旧判据失效 → 按 system_name 值区分
  - #770 账号表单只有 session/apikey 模式，password 站点无法绑定 → 新增 password 模式接 login 流
  - #773 one-api v0.6.10 凭证在 session cookie（`data.access_token` 空串）→ cookie 登录 + cookie-aware self/checkin/balance
  - #776/#777 sub2api + cliproxyapi VerifyToken 静态分派（Go 内嵌无虚分发）→ 各自 override
- **CI 工作流（GitHub Actions 免费持久化）**：#774 新增 `test-e2e` job（真实 new-api/one-api 服务容器跑 `scripts/e2e/smoke.sh` 双全链）+ `test-sqlite` 4 分片（`-race` 长杆 4m56s→~1m30s）+ golangci-lint/Playwright 缓存 + `go-toolchain` composite action；#775 补 `scripts/e2e/verify-token-import.sh`（token 导入链）
- **实测链**：`smoke.sh`（password 登录链）+ `verify-token-import.sh`（token 导入链）四条链全绿：new-api 13 PASS、one-api 13 PASS、sub2api 11 PASS、cliproxyapi 11 PASS
- **发布**：v0.13.0 tag 发布（CHANGELOG + web/package.json 同步）；#778 修 smoke.sh `set -u` unbound（`first_model`）

## 2026-08-14 — Leader/Worker 并行 fan-out：5 分支合入 + strict 模式关闭

- **并行开发**：5 个 Flash worker 各自在 `.worktrees/` 独立分支实现，PR #658-#662 全部 squash 合入 master（全局搜索 / 首页今日快照+StatCard / 告警富化 / 表格交互 / 测试台会话化+模板库），连同 #657 共 6 个 PR
- **工程策略**：关闭分支保护 strict「分支必须最新」——保留 12 项必检 + squash 线性历史，允许并行分支各自 CI 绿后直接合入（`docs/internal/git-workflow.md` §3 已记录）
- **hook-kit 修复**：修 `leak_guard.py` 新分支 push 用空树 diff 误报历史占位符的 bug，改为与远端 merge-base 求差（补回归测试）
- 剩余 P1：接入向导 onboarding checklist、价格对比权重对照、测试台批量延迟对比

## 2026-08-14 — 多 Agent UI/UX 对照审计 + 分发端 P0 落地

- **4 个审计 agent**（动线/视觉/交互/功能对标，对照 New API × All API Hub）：结论聚合进 `docs/internal/analysis/ui-ux-audit-2026-08.md`（25 视觉项 / 9 交互项 / 5 动线 / 8 功能差距）
- **分发端 P0**：客户端接入导出对话框（Cherry Studio/CC Switch 深链 + env/JSON 复制，复用后端 credential_export）；建路由完成 toast CTA 改接 `/settings/downstream`
- **视觉 P0**：6 处 `-foreground` 误用修复 + badge success/info 变体 + 站点徽章语义 + 软徽章对比度 token 修正
- **交互 P0/P1**：rates 行内编辑安全网（防重/空值/丢 draft）；404 与错误边界页；深链页码钳位（4 页接线）；导入向导脏确认
- **验证**：tsgo 0 error · oxlint 0 error · vitest 337 全绿（i18n 键门禁）· 生产 build 通过

## 2026-08-14 — 产品对标 + 文档卫生（neat-freak）

- **产品对标文档**：`docs/internal/benchmark.md`（New API v1.0.0-rc.24 × All API Hub 上游 #1290）；结论 P0 = 客户端一键导出 + 接入向导；roadmap 表进 MASTER.md；"明确不做"= 多租户计费/支付/桌面版
- **文档卫生**：STATE.md 发行/生产 pin/i18n/RE2 事实对齐 v0.12.0；package-boundaries.md 清除 canonical/anthropic/Conductor 残留 + 修正 transform 接线状态（部分接线）；AGENTS.md 结构注释对齐 as-built（35 表/16 调度器/transform 3 协议族）；删除 .claude/UPGRADE-POLISH-PLAN.md（未完成项转入 MASTER backlog）

## 2026-08-14 — v0.12.0 架构简化（净删 ~21K 行）

- **死代码大清扫**（#650–#654，5 个 PR）：删 test-only 的 canonical 转换层（~6.6K）、三套测试专用编排层（conductor/surface/endpoint_flow）、未激活的 lease/sticky 队列、死 facade（app/prometheus、shared 半套 metrics/errors、input_files、检测管线、service/adapter 桥接）；Go 170K→150K，无生产行为变化
- **数据完整性修复**：cmd/migrate 自维护 schema 副本已漂移（sites 漏 7 列）→ 复用 store.AutoMigrate + 运行时漂移守卫；app/proxy_upstream.go 垃圾抽屉移入 service/routing_store.go
- **去重 + 标准化**：手写 stdlib 重实现（Hinnant 日历/JSON/int/TrimSpace）→ 标准库；handler/routing/scheduler/platform 各去重 3-11 份复制；god-file 按自然接缝拆分（含前端 api.ts 1997→10 模块）
- **文档对账**：architecture/BACKEND/package-boundaries 同步为 as-built 真相（无 canonical、无 Conductor 编排）

## 2026-08-14 — v0.11.0 管理控制台 UI/UX 全量 + 开源打磨

- **管理控制台 UI/UX 与功能全量交付**（#594–#626，合并为 #633/#634）：共享格式化器、状态/空态组件、toast、CountUp、响应式 data-table + 移动端降级、无障碍、token 列脱敏、可观测性工作台（Overview/Health/Proxy Logs）、站点探测 + 统一导入向导、模型定价/价格对比/重定向修复、通道只读列表 + probe 驱动重建过滤
- **可观测性**：访问日志 status/bytes/duration_ms + statusRecorder 转发 SSE/WebSocket + slog panic 恢复带 request_id + /metrics 纯 stdlib go runtime 指标（#593）；/api/routes/rebuild 响应新增 changed 统计
- **调度器注册回归修复**：balance-refresh / log-cleanup / backup-webdav 曾构造未 Register()，已注册并加 16 集合回归测试（#635）
- **CI/CD 单一管道**：test → 镜像推送 → GitHub Release 合一（main.yml）；master push 推 latest+sha 镜像；SemVer tag 另建 5 平台二进制 + checksums + 二进制冒烟 + Release（#637）
- **升级与打磨**：旧 schema 增量 ALTER 回归测试 + forward-only 迁移文档（#636）；docker-compose.prod 安全硬化（#639）；重新启用 unused/gosimple + 删死代码（#641）；a11y 标签 i18n + SSE any→unknown（#642）；docs/api.md 端点清单（#643）；.env.example 补齐（#640）
- **类型/卫生收尾**：api.ts payload/response any→unknown（#646）；Go 1.26.6 升级（#647）；清理过期子代理拆分注释（#648）

## 2026-08-12 — v0.10.0 设置中心 + 调度规格 v1

- **ScheduleSpec v1**：daily / interval / random-window / custom cron 四种调度规格，additive preview/apply 迁移端点保留所有 legacy key
- **设置中心**：统一表单 actions + load-error 态 + dirty 导航守卫 + 语义化调度控件 + 响应式导航；DB 设置页统一 dirty-guard，应用内 501 migrate 按钮改为 CLI-only 迁移提示；审计日志服务端分页 + 分页表 UI
- **CJK 字体回退**：--font-sans 加 Noto Sans SC/TC/JP/KR + PingFang SC + Microsoft YaHei；移动端侧栏 header 触发器（md:hidden）

## 2026-08-11 — post-v0.9.0 UI completion batch

- **Git workflow（GitHub Flow 落地，已实际启用）**：master 唯一长期分支 + 分支保护（要求 PR + 11 CI 检查必选 + enforce admins + 禁强推/删除）+ 仓库级 Squash-only 合并 + PR 模板；规则文档 `docs/internal/git-workflow.md`；ci.yml 移除 paths-ignore（必选检查与跳过互斥）
- **v0.9.0 发布补推（此前仅本地）**：本地 master 18+ commit（v0.9.0 重写 + UI completion）推上 GitHub；`v0.9.0` tag + Release 创建；CD 双跑成功 → `ghcr.io/deliciousbuding/metapi-go` 发布 `latest`/`0.9.0`/`0.9`/sha 镜像（首跑暴露 CI 盲点并修复：go:embed 需 web/dist，测试 job 构建真实前端；responses-websocket doc 指针 `.md` → 真实文档）
- **前端修复收尾**：Base UI render prop + TanStack Link 冲突（侧栏点击 JSON circular 崩溃）→ SidebarNavLink 只透传 DOM-safe props；searchParams parse/encode 分离消除 `?sort=%5B%5D` URL 噪声；updateCenterReminder/updateCenterPresentation（501 residual 幽灵前端）删除
- **品牌 rename → Metapi**：display name unified (identity-branding / locales / About / index title); transparent SVG badge `logo.svg` (gradient rounded-square + real π glyph) + `favicon.svg` replace the white-background PNG; router root-file whitelist + table-driven regression test extended to `image/svg+xml`
- **i18n language switcher**: header `LanguageSwitcher` dropdown (en/zh-CN) + browser auto-follow (localStorage → navigator) + `documentElement.lang`/`dir` sync via `toBcp47`; locale parity now 1381 keys each, bidirectional 0 missing
- **URL-synced tables fix**: sites/models/oauth/site-announcements read the router location (`useLocation` + `searchStr`) so sort/pagination now update the table in place instead of waiting for an unrelated re-render
- **Copy audit**: terminology unification (启用/停用, 额度, Check-in, 通道), internal plan codes (K1a/N9a) removed from user-visible copy, tokenRoutes toast/chain-banner concatenation bugs fixed, 9 hardcoded strings → t() (incl. TokenDance brand leak removed from the public settings copy)
- **Visual polish**: sign-in real logo mark + brand glow + lg CTA; dashboard StatCard skeletons + useId-unique gradients + iconized empty/error states + pulsing WS indicator; settings responsive drill-in + sticky sidebar; authenticated-layout scroll-clip fix (content taller than viewport was unscrollable)
- **Theme customizer**: header Palette panel with 4 axes (10 color presets / font Auto-Sans-Serif / radius 6 / scale 4) + per-axis & global reset; **default font is sans for every preset** (Anthropic no longer inlines serif; serif is an explicit choice); legacy FontProvider dual-track removed
- **Sidebar i18n**: nav titles moved to keys (sidebar.groups/items/backToHome, en + zh-CN), sidebar renders fully in the active language

## 2026-08-11 — v0.9.0 frontend rewrite

- **Frontend rewrite**: newapi stack 100% alignment — Bun + Rsbuild 2 + TanStack Router/Query/Table + Tailwind 4 + shadcn Base UI + OKLCH tokens (details in root `CHANGELOG.md`)
- **i18n key-based**: i18next + en/zh-CN locales each 1369 keys, bidirectional 0 missing; MutationObserver dictionary + `tr()` sweep retired, replaced by vitest `i18n-keys.test.ts` gate (1151 keys scanned)
- **Tooling**: npm → Bun (CI/Dockerfile/Makefile); Playwright + `ui-visual` removed; old frontend tree (`App.tsx`/pages/components/styles/e2e) deleted

## 2026-08-02 — v0.8.46 → v0.8.52

- **PG dialect hardening**: 4 bare `*sqlx.DB` `?` placeholders hitting PG directly (SQLSTATE 42601) all wrapped with `db.Rebind`; added static check `pg_rebind_gate_test.go` to prevent regressions
- **v0.8.49+**: `sc2_008` migration `BOOLEAN DEFAULT` PG 42804 and balance_history UPSERT 42601 — two PG dialect blind spots fixed; CI PG integration tests serialized (`-p 1`)
- **Frontend i18n sweep**: 414 bare Chinese JSX text nodes wrapped with `tr()`; added `i18n.gate.test.tsx` static check
- **Chart series color token wiring**: VChart canvas series colors — 27 `var(--color-chart-N)` silent fallbacks → `useChartColors()` JS lookup + chart-colors check
- **Design token unification**: RealtimeOpsPanel real-time traffic badge hardcoded rgba → `--color-*-soft` token
- **Feishu notifications**: TaskTag signature aggregation anti-spam (same-class alerts merged within cooldown window) + notification channel save pre-validation
- **Resource integrity three layers**: dist self-check + CD build verification + deploy live smoke test
- **Login page refactor**: single-card layout + dark de-gradient; root static files (logo/favicon) SPA fallback image-swallow fix

## 2026-07-31 — Feature batch

- **All features shipped**: model redirect mapping, snapshot PNG, risk banner, tag system, batch validation, chart gallery, randomized window scheduling, backup import preview, scheduler task run history
- **Downstream key hardening**: downstream key IP allowlist/blocklist (security gap), public price endpoint, inference suffix parsing, spend distribution dashboard, CSV export, configurable cache multiplier; remaining items honestly deferred
- **Frontend UX pass**: RouteErrorBoundary, SearchModal real keyboard navigation, Toast a11y, design-system state trio (Empty/Loading/Error), Models→Playground quick jump, ProxyLogs date presets

## 2026-07-30 — Engineering optimization + parity review

- **Package boundary enforcement**: `docs/package_boundary_test.go` turns BACKEND.md §2.3 eight hard rules into a `go test` check; caught and remediated `scheduler/lease.go` stale exception on the spot
- **Product parity review**: Go rewrite lost no TS README product features; 14 platform adapters TS=Go aligned; multi-user/payments/redemption-codes/invitations/subscriptions explicitly not applicable
- **Dual-dialect encapsulation**: `store.DB` gained Context methods (rebind `?`→`$N`), removed 4 manual dialect branches

## 2026-07-20 — v0.8.45 RE2-safe

- **RE2 panic fix** (production crash root cause): NewAPI user-id extract compiled PCRE lookahead `(?!\d)` → Go RE2 panic; switched to pre-compiled regex + 8-digit length cap
- **Four-track original feature alignment**: frontend 18 routes + 14 sidebar 100% parity; 14 platform adapters fully aligned; 16 scheduler tasks covered; WS/Sticky/UC explicitly residual
- **Original parity plan** (ex-Electron): WS full TS parity, sticky single-instance honesty, UC hide/external

## 2026-07-19 — UI polish milestone

- **UI polish batch**: Traffic sparkline, real page scoring, axe a11y smoke, Dashboard getting-started, Sites banner
- **UI parity inventory**: sidebar 18 routes at parity, Sites/Accounts/Tokens/Routes/Settings button counts at parity
- **Focus management shared**: `useFocusTrap` wired into SearchModal / CenteredModal / MobileDrawer / NotificationPanel; skip-link accessibility jump
