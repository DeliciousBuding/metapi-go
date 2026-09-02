# Changelog

All notable changes to Metapi-Go will be documented in this file.

格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

## [v0.16.22] — 2026-09-02

### Added

- **仪表盘四步旅程清单（#1148）**：站点 → 账号 → 路由 → 密钥的四步 checklist 取代「建好第一个站点就退役」的单步横幅，CTA 只挂在第一个缺口上（一次只引导一件事），四步全部建成后自我退役。判态复用既有接口（仪表盘快照、路由汇总、下游密钥列表）与各功能自己的 query key/queryFn，访问过对应页面的部署不产生额外请求、不污染缓存；任一数据源未回答或出错时判态返回 null，既不首屏闪现四步待办，也不凭空编造缺口。
- **路由页 → 下游密钥交接条（#1148）**：已有路由但下游密钥数为 0 时，路由页就地提示「签发下游密钥，客户端即可用它调用 `/v1`」；加载中、请求失败、已有密钥三种情况一律沉默，签发第一把密钥后永久消失（不是又一条常驻横幅）。路由创建成功 toast 的 CTA 目的地改为一级页 `/downstream-keys`（此前指向提升前的 `/settings/downstream`），空态描述点名第 4 步，完成态文案不再一边自称「已完成」一边留着最后一步。
- **密钥创建后「立即接入」动作（#1148）**：导出对话框虽然会自动打开，但它可被关闭且受主令牌二次确认保护，关掉后此前只能回表格行里翻入口；创建成功的 toast 现带一键重开动作，并把同一把新密钥回传给对话框。
- **三个配额字段的语义说明（#1148）**：`maxRequests` / `maxCost` / `expiresAt` 补齐单位与行为——累计总量而非每分钟窗口（每分钟限速是另一组 `max_rpm` / `max_tpm` 列）、金额单位为 USD 且仅成功请求计费、超限返回 429（`over_requests` / `over_cost`）、过期返回 403（`key_expired`）、留空或填 0 表示不限制、过期时间按浏览器本地时间填写但以 UTC 存储与比较。文案逐条转录自鉴权与计费实现（含 `aria-describedby` 可访问性断言），不是凭印象写的。

### Changed

- **`/v1` 写超时不再掐断慢响应（#1145）**：`Server.WriteTimeout`（60s）在请求头读完时即武装，因此也覆盖了 handler 等待上游的时间，而缓冲派发的整体上限是 `max(90s, 首字节窗口 ×2)`——在 61~90s 之间返回的非流式响应会在回写客户端时被掐断（表现为长请求「拿到结果却连接被重置」）。现由 `router.ProxyWriteDeadline` 中间件用与执行器**同源**的预算（`proxy.WriteBudget` = 请求上限 + 2 分钟）重新武装写截止，写侧结构上不可能再短于请求侧；admin 面保持严格 60s；SSE 流仍完全清除写截止，由逐块空闲守卫（`PROXY_STREAM_IDLE_TIMEOUT_SEC`）掌管存活性，长推理流不受总时长约束。
- **客户端协议头透传（#1145）**：数据面重建上游请求时此前只带 `Content-Type` + 站点 `custom_headers` + 选中账号 token，客户端的协议开关全部静默丢失——`anthropic-version`、`anthropic-beta`、`openai-beta`、`user-agent` 与 `x-stainless-*` SDK 遥测命名空间，Claude Code 的特性开关（prompt caching、fine-grained tool streaming）因此永远到不了上游。现按白名单**填充式**透传：站点 `custom_headers` 与每站点反爬身份仍然优先，账号 token 最后设置、客户端永不可覆盖；多值头（多个 `anthropic-beta` 标志）全量保留。凭据与代理类头一律不透传（`authorization`、`x-api-key`、`cookie`、`x-forwarded-*`、hop-by-hop），下游密钥永不外泄给上游。Anthropic 原生派发（`/v1/messages`，含 `/anthropic/v1/messages` 网关形状）在客户端未带 `anthropic-version` 时补协议默认值，OpenAI 面请求转换成 Anthropic 形状后不再被上游校验拒绝。
- **上游响应头改内容语义白名单（#1145）**：缓冲响应此前全量拷贝上游响应头，把上游的身份与状态泄漏给下游——厂商指纹头（实测可见 `X-New-Api-Version`）、上游 `Set-Cookie`、以及覆盖 metapi 自己 `X-Request-Id` 的上游同名头（跨层日志无法关联，客户端报障时 id 对不上）。现只转发内容语义头（`Content-Type`、`Content-Disposition`、`Content-Encoding`、`Content-Language`、`Content-Range`、`Accept-Ranges`、`Cache-Control`、`ETag`、`Last-Modified`、`Location`、`Retry-After`）；metapi 的 request id 与 `X-Ratelimit-*` 保持唯一权威（客户端看到的限流头只描述自己的 metapi 配额）；分帧/逐跳头不外传，缓冲体由 net/http 重新分帧。流式路径的错误中继同策略，SSE 正常流仍全部由 metapi 重新构框。
- **Redis 共享限流不再全局串行（#1147）**：配置 `REDIS_URL` 后，密钥准入此前是「全局一把锁 + 每命令一次 TCP 握手」：`Allow()` 持全局互斥做共享 RPM/TPM 往返，`RedisCounter` 每条命令新建连接（dial + `AUTH` + `SELECT` + close）且完全忽略 `ctx`——所有密钥互相排队，每请求还要在锁内付一次握手。现改为：64 条 cache-line 对齐分片锁（`keyID % 64`，只保护内存窗口，**绝不跨 I/O 持有**）+ 每密钥 `sharedMu` 串行化本 key 的 `Incr` 与补偿 `Decr`（锁序恒为 `sharedMu → shard`，无环、不可能死锁）+ 连接池（并发上限 8、LIFO 复用、`AUTH`/`SELECT` 每连接一次、传输错误丢弃并重连、Redis 错误回复不重试以免造成连接风暴、完整尊重 `ctx` 取消与 deadline）。**语义零漂移**：Redis 出错仍 fail-open 回落本地窗口、`over_rpm`/`over_tpm` 原因与 `Retry-After`、`UsedRPM`/`UsedTPM`、`AdmissionDecision` 形状、`metapi:rpm:{id}`/`metapi:tpm:{id}` 命名空间、全部 env 名与函数签名均不变，零新依赖。实测（每轮 2ms 共享往返、32 并发、多密钥）：准入吞吐 117 → 3137 次/秒（26.8×），p50 259ms → 10ms、p99 360ms → 40ms；单次 `INCR` 465µs → 54µs（8.5×），`AUTH` 命令 3530 → 1 次，峰值连接 15 → 8（现有上界）；不配 Redis 的纯内存路径也因分片获得 1.7×。同一热点密钥的多请求仍按设计串行（1.0×，`Incr`/`Decr` 必须有序）。fail-open 原子性有测试钉住：Redis 全挂 + 100 并发 + 限额 50 → 恰好放行 50。
- **视频任务映射保留期默认 7 天（#1146）**：`PROXY_VIDEO_TASK_RETENTION_DAYS` 默认值由 `0`（= 保留期调度器整条禁用，`proxy_video_tasks` 无界增长）改为 `7`——视频任务映射是短寿的 id 重写记录，比代理日志/文件（30 天）退休更快。`<=0` 仍是运维显式关闭（只是不再是默认），调度器禁用原因串同步修正为 `retention_days<=0`。`.env.example` 与 `docs/configuration.md` 同步说明该旋钮同时约束持久行与进程内缓存。

### Fixed

- **数据面 SSRF dial 级纵深（#1145）**：站点批量导入与两条备份导入路径的目标校验收口之后，真正发起转发的 transport 仍是裸 `net.Dialer`——校验通过后重新解析（DNS rebinding）仍可落到不同地址。现执行器、SSE 流 transport、两个兜底派发客户端与渠道健康探针 transport 统一挂 `internal/ssrf` 的 DNS 钉扎守卫（新 `httpclient.Options.SiteDialGuard`）：主机名只解析一次、每个应答都校验、连接钉到已验证的 IP。云 metadata（`169.254.169.254`、`100.100.100.200`、`fd00:ec2::254`、`metadata` / `metadata.google.internal` 别名）、link-local、multicast、unspecified/reserved 段一律拒绝；**loopback、RFC1918、IPv6 ULA 刻意保留**——自托管上游（本地 new-api / one-api / sub2api / LiteLLM 网关）是一等部署形态，守卫只挡 metadata 外泄不挡运维内网。
- **视频任务进程内缓存无界增长（#1146）**：`publicId → 上游视频 id`（含 sticky 渠道/账号 pin）的重写缓存是包级 map 且无任何驱逐，长跑进程随累计视频流量线性泄漏内存。现按同一个 `PROXY_VIDEO_TASK_RETENTION_DAYS` 旋钮做 TTL，驱逐在插入路径摊销触发（每 256 次插入或每 5 分钟至多一次 sweep：突发护栏 + 闲置护栏，无后台 goroutine、无每请求全表扫），并加 20000 行（约 4MB）硬容量护栏——即使运维显式关闭保留期也生效，超限时按最旧优先批量 trim 到 `cap - cap/16` 以摊薄排序成本；读路径把过期行当 miss（O(1)，只需读锁）并从持久表回温。持久行的剪枝仍只由保留期调度器在其租约下负责，缓存永不删 DB 行。
- **下游密钥的过期时间此前完全不生效（#1148）**：表单出站的是裸 `datetime-local` 串（形如 `2030-01-02T03:04`，无秒、无时区），写入归一与鉴权两处解析器都不认，而不可解析的过期时间按 TS parity 会**跳过整个过期检查**——实测一把 2020 年就该过期的密钥仍能以 200 调用 `/v1/models`，而同一时刻的 RFC3339 形式正确返回 403。现出站统一为 RFC3339 UTC，同一服务端复验：已过期 403、未过期 200（含 `toISOString()` 的毫秒形式）。留空仍表示永不过期且仍进 body（更新路径需要该字段存在才能把已有过期时间清空）；不可解析的输入原样透传，绝不被静默改写成「永不过期」。**行为变更**：编辑并保存一把存量脏格式的密钥后，它的过期时间会开始真正生效（该值在表单里可见、可改）；存量行的批量归一需要后端先定死时区语义（裸本地时间串不带时区），已作为独立后续项处理而非在此猜测。

### Docs

- `docs/api/proxy.md` 新增「Header policy (/v1 surface)」（双向头策略、优先级次序、凭据头永不透传）与「Timeouts (/v1 surface)」（各相态超时与归属表）；`docs/api/conventions.md` 新增「Site upstream SSRF hardening (proxy data plane)」（URL 层 + dial 层两道守卫、拒绝段与刻意放行段）。
- 内部卫生：死字段 `SurfConfig.ExtraHeaders` 删除；`anthropic-version` 协议默认值从三处副本归一为 `platform.ClaudeDefaultAnthropicVersion`；出站 HTTP 客户端门禁说明补上 `Options.SiteDialGuard` 现已建模 dial 级守卫。

## [v0.16.21] — 2026-09-02

### Added

- **sc2_029 TS 接管库时间戳归一化（#1142）**：启动迁移自动把 TS 时代（drizzle `datetime('now')`）写在 TEXT `*_at`/`*_until` 列里的 `'YYYY-MM-DD HH:MM:SS'` 值重写为 RFC3339 UTC（`'...T...Z'`，保留小数秒）。接管库两种格式混排时，所有字典序比较失真（空格 0x20 排在 'T' 0x54 前）：`ORDER BY created_at` 列表错排、范围过滤边界失真、checkin 清扫把全部 TS 账号判过期引发首启重签风暴——归一化后消除。候选列由 schema introspection 发现（接管库中 Go DDL 未声明的列同样覆盖）；PG 原生 timestamp 列跳过（本就按时间比较）；幂等、新装零成本。
- **`/v1` 错误契约文档（#1140）**：`docs/api/proxy.md` 新增「Error shape (/v1 surface)」节——OpenAI 信封 + 完整 status/type/code 对照表；admin 面（`/api/*`）平铺契约不变，两者不混用。

### Changed

- **`/v1` 鉴权与限流错误对齐 OpenAI 信封（#1140）**：中间件裸 `{"error":"..."}` 字符串统一为 `{"error":{message,type,code,request_id}}`（与 handler 层同形）；invalid key `403`→`401 authentication_error/invalid_api_key`（SDK 对 403 视为不可重试权限错误、跳过换 key 路径），配额耗尽 `403`→`429 insufficient_quota`（OpenAI 约定）；全局限体 413 按路径前缀分流（`/api`、`/auth` 保持平铺 admin 契约，代理面走信封）。new-api 反接 metapi 作上游渠道时错误解析不再退化。
- **Go 死码清扫 -1814 行（#1137）**：oauth import 族（`/api/oauth/import` 保持文档化 UI-parity 桩）、quota `Record*` 族及其私有链、routing `PricingReference`/四端口接口、`ClearChannelFailureState` 级联（接口方法+实现+4 测试 fake）、`SendDailySummary`、`BatchUpdateTokenStatus`、`ApplyStreamPreference` 等 14 项——每项经全仓生产+测试双零引用验证。三份同实现 clamp（handler/admin、config、routing）收敛为 `config.ClampInt`；`maxInt`/`minInt` 改 Go 内建 `max`/`min`。
- **web 一次性探针清理 -1900 行（#1138）**：`web/scripts/oneoff/` 12 个零引用脚本整目录删除（probe-ab-* 系列、dom-audit、dark-contrast-probe——其亮度管线已由 contrast-gate 测试完整复刻）；5 处手写 `navigator.clipboard.writeText`（其中 3 处无 SSR/能力兜底）收敛为共享 `src/lib/clipboard.ts` `copyText()`（单测覆盖成功/拒绝/API 缺失三态）。
- **SSE 默认流上限 1MB→64MB（#1141）**：`PROXY_MAX_STREAM_RESPONSE_BYTES` 默认值上调——推理模型长输出/大型代码生成常态超 1MB，此前被中途截断且自定义错误事件多被 SDK 静默吞掉（表现为「回答戛然而止」）；上限保留为失控护栏，`.env.example` 与 `docs/configuration.md` 同步。
- **SSE 响应补 `X-Accel-Buffering: no`（#1141）**：nginx 系反代不再缓冲首 token（new-api/sub2api 同款行为）。

### Fixed

- **SSRF 目标校验缺口（安全，#1139）**：站点批量导入（`POST /api/sites/import`）、原生备份导入、TS v2.1 备份导入三条路径此前完全绕过 `IsForbiddenSiteTargetURL`——构造备份即可植入 `url=http://169.254.169.254/...` 的站点，配合下游 key 一次 `/v1` 请求穿透数据面回流云 metadata/IAM 内容。现三路径统一走 `SanitizeImportedSiteRows`（sites 的 url/external_checkin_url/proxy_url + site_api_endpoints 的 url；RFC1918/loopback/ULA 保留给自托管上游，与单行创建语义一致），导入预览同规则（所见即所导入），丢弃行显式上报（TSV21 warnings / `droppedForbiddenSiteRows`）。
- **`/v1/pricing` 双前缀路由 bug（#1141）**：`RegisterDownstreamPricingRoutes` 在 `/v1` 组内注册绝对路径，文档承诺的 `GET /v1/pricing` 一直 404、真实可达路径是 `/v1/v1/pricing`（未鉴权探测看不到——组中间件先答 401 掩盖了它）。改相对路径 + 挂载形状回归测试钉死。
- **`catalogsync.CreateSource` 在 PG 必失败（#1141）**：pgx stdlib 驱动不实现 `LastInsertId`，PG 部署下新增模型目录源恒报错（SQLite-only 测试漏网）。补 `RETURNING id` 方言分支 + `PG_TEST_DSN` 门控集成测试（CI PG job 覆盖）。
- **审计日志路径过滤 PG 大小写失配（#1141）**：`path LIKE ?` 在 SQLite 为 ASCII 大小写不敏感、PG 敏感——双侧 `LOWER()` 对齐（全仓 LIKE 过滤同款模式）。

## [v0.16.20] — 2026-09-01

### Added

- **运行事件结构化（F5 批次 1：checkin 事件族）**：`service/events` 类型化注册表（register 防重复、参数严格校验），`WriteEvent` 在持久化历史英文 title/message（非 UI 消费者字节级零破坏：notify/CSV/历史行）之外新增 `title_key` + `params` JSON；`sc2_028` 增量迁移（TEXT NULL，双方言 tableExists 守卫）+ 新装 DDL + cmd/migrate 列同步。程序日志页结构化渲染：标题经 `events.titles.*` 本地化、消息经 `events.messages.*` 模板 i18next 插值（`{{account}}`/`{{site}}`/`{{reason}}`），历史行保持原文/历史映射 fallback；en/zh-CN 各补 4 键，双语 `{{var}}` 一致性门禁（Go + FE 双侧钉死键集）。
- **删除+undo 档（#1097，#1035 S7 收官）**：叶子实体单行删除（模型重定向、目录源、令牌路由、下游 API 密钥、账号令牌）不再弹确认框——行即消失并给出 6 秒「撤销」窗口，真实删除仅在窗口关闭后发生；撤销精确恢复，服务端从未写入。批量操作、级联删除、factory reset 各自保留计数确认 / typed-confirm 档位（规约见 DESIGN.md §4.1）。


- **i18n 插值占位符 parity 门禁（#1035 S10）**：i18n-keys 测试新增断言——每个双语键在 en 与 zh-CN 中的 `{{变量}}` 集合必须一致（此前键集合 parity 抓不到译文丢失插值变量导致的「半翻译」渲染）；现状 2493 键 0 失配，随 frontend CI 锁定。
- **延迟图表无障碍数据摘要（#1087，#1035 S10）**：仪表盘延迟直方图与延迟趋势补 sr-only 数据表（直方图：区间×调用数；趋势：平均/p95 × 最新日/窗口均值）——至此仪表盘全部六个主图均有屏幕阅读器替代层。
- **凭证维度契约测试与文档（#1026 残留）**：路由选择器执行测试（两种 kind、跨 kind 不互匹配、空列表不限制、排除优先于允许、TS 遗留空 kind 语义）、管理端验证拒绝用例、悬空引用行为钉住（删号/删令牌不级联清理，悬空允许引用失败关闭）、auth→routing 映射测试；`docs/api.md` 下游密钥节新增完整契约（字段形状、空=不限制、验证规则、选择器行为、只读响应为 JSON 字符串、UI 待定说明）。
- **下游密钥凭证树形选择器（#1026，#1072）**：`allowedCredentialRefs`/`excludedCredentialRefs` 配置 UI——按站点 → 账号 → 密钥三级树勾选，与 API 契约一致。
- **命令面板动作层（#1073，#1035 S6）**：Ctrl/⌘+K 面板在页面/实体导航之外支持直接执行动作。
- **移动端表单与详情抽屉底部关闭条（#1080）**：≤640px 全宽抽屉新增拇指可达的底部关闭条（自带取消/提交的表单抽屉除外，避免同义双出口）。
- **恢复出厂设置 type-to-confirm（#1080）**：需输入 `RESET` 且倒计时结束后才可执行，防止误触清空全部数据。

### Changed


- **事件标题 i18n 统一为单一映射 + 单一词条节（#1099）**：程序日志页与 attention 管线此前各带一份事件标题映射表和 locale 节（漂移源），7 个高频生产者标题（签到成功、站点启用、令牌同步完成、运行时设置更新、管理员令牌更新、模型修复、备份导入）两边均未映射而在中文界面裸显英文。现收敛为共享 `lib/event-titles.ts`（15 个已知生产者标题 → slug）+ 单一 locale 节 `events.titles.*`；未知/动态标题仍原文渲染（诚实残留）。
- **清理 17 个不再引用的界面文案键（#1103）**：en/zh-CN 各移除 17 个从未被代码引用的键（含已下线的 `homePageContent` 字段系列、被替代的 `proxy24hHint`/`modelTester.form.channelHint` 等）；经模板/配置映射引用的动态键全部保留，双语键集合仍一致。


- **section-registry 三克隆合一（#1095，#1035 S8）**：settings/dashboard/observability 各自的 `createSectionRegistry` 收敛为共享 `web/src/lib/section-registry.ts`（`urlStyle` 承载 path/query 两种 URL 形态）；行为不变，净 -101 行。


- **列表页错误三态收敛进 DataTablePage（#1084，#1035 S7）**：加载失败的横幅+重试从 8 个列表页各自手写的条件分支收敛为底座内置契约（`error`/`errorMessageKey`/`onErrorRetry` 属性，替换/内联两种放置）；行为不变，挂载点单一化。
- **`docs/api.md` 按域拆分**：超 1500 行预算的 API 参考按域拆为 `docs/api/*.md`（17 个文件，按域自含「返回索引」标题）；`docs/api.md` 保留为索引，全部原有 H2/H3/H4 标题以 stub 承接并一一指向新家，旧 `api.md#<anchor>` 深链继续解析（F4）。
- **凭证引用畸形条目改为显式 400 拒绝（#1026 残留）**：创建/更新下游密钥时，`excludedCredentialRefs`/`allowedCredentialRefs` 中的畸形条目（非对象、未知/缺失 `kind`、非正 `siteId`/`accountId`、`account_token` 缺 `tokenId`）原被静默丢弃——对允许列表而言等于静默放宽访问（fail-open）；现以 400 显式拒绝并提示具体条目。合法引用行为不变。
- **统一 400 错误体携带机器可读 `errorCode`（#1065，#1035 S4）**：需要客户端分支处理的失败类别逐步登记错误码（认证与会话 8 码、资源不存在 5 码、能力未实现/禁用 3 码、设置校验 1 码、读取路径加载失败族 1 码覆盖 72 个调用点，见 `docs/api/conventions.md` registry），客户端应基于 `errorCode` 而非错误文本判断。
- **管理端大表服务端分页（#1075，#1077）**：账号、模型市场、渠道（含状态过滤）列表改为服务端分页，大数据量下首屏与翻页显著提速；`GET /api/channels` 新增 `status` 过滤参数，`GET /api/channels/error-summary` 新增聚合端点（渠道失败横幅一键过滤的数据源）。
- **站点表单迁移为右侧抽屉（#1082）**：添加/编辑站点从居中弹窗改为右侧滑出抽屉，与账号表单形态统一；长表单滚动时底部「取消/创建」常驻可达，未保存修改的关闭确认行为不变。
- **列表筛选状态入 URL（#1080）**：价格对比的模型过滤、站点公告的站点/平台/阅读/状态筛选与页码写入 URL——刷新、后退/前进、分享链接均保持视图。

### Fixed


- **账号页筛选服务端化（#1122，issue #1108）**：`GET /api/accounts` 新增 `q`/`status`/`site` 筛选参数——此前分页是服务端、筛选却是客户端（只过滤已加载页），站点不在当前页时永远筛不出来；现在筛选全舰队（SQL 参数化 + LIKE 字面转义），`total` 为筛选后计数，非法值显式 400。筛选重置此前只清搜索框——Reset 连发两次状态更新产生两次导航，TanStack 同 tick 合并只留最后一个导致第一次更新被静默丢弃；`useUrlTableState` 改为串行化（每次更新基于上一次 href），所有 URL 同步的列表页重置都恢复完整。账号页站点单元格支持快捷跳转（安全外链，同站点页 #985 阶梯，共享 `SafeExternalLink` 组件）。
- **运行事件标题列折叠（#1124）**：受影响路由/替代站点/面板深链此前与 model/reason 一起堆在标题列（每行约 5 行高）；现在折叠进每行「详情」开关（`aria-expanded` + 焦点环），默认行高回到可扫读的 3 行。
- **文案校对（#1123）**：路由策略「基础权重加数」→「基础权重基数」；「管理员审计日志」→「审计日志」（7 字标题在窄侧边栏截断）。



- **通知铃铛弹层告警文案未本地化（F3，#1091）**：后端 attention API 为兼容保留英文 `label` 并下发结构化 `params`，dashboard 可用性面板已做再本地化，但顶栏铃铛弹层裸渲染英文——中文界面下出现「Balance unknown: svc-onea…」等英文告警。两处现共用新共享模块 `web/src/lib/attention-label.ts`（含 8 个持久化事件标题的 i18n 映射）；en/zh-CN 补齐 7 个事件标题键；参数缺失或未知类别回退原文，绝不半翻译。
- **代理日志时间列详情行同日重复（#1101）**：7 天窗口内详情行渲染「8月23日 · 2026年8月23日」——同日绝对日期与相对时间连读重复；新增 `formatLogDateDetail`（窗口内短日期 · 相对时间，超窗仅绝对日期），恢复紧凑。
- **页面双 `<main>` landmark + 品牌 logo 冗余 alt + 空表头（#1102）**：dashboard / observability / settings 三层页级 `<main>` 与布局壳唯一 `<main id="content">` 叠加成每路由双 landmark（页级降为 `<div>`）；三处品牌 logo `alt` 与相邻同名文本重复（改 `alt=""`）；catalog-sources 拖拽列空 `<TableHead>` 补 sr-only「排序/Reorder」（axe empty-table-header）。
- **设置页单卡片节标题与描述双重叠（#1104）**：settings 单卡片节（redirects / danger-zone / keys 等约 15 页）页头 h1+description 与 `SettingsSectionCard` 卡头逐字重复；共享壳改为无头部动作时不渲染 CardHeader，带动作的卡保留（按钮需宿主）。数据迁移节的 wipe/重启警告从卡 description 移到卡内顶部警告条（更易读）。
- **channels 页「响应延迟」列默认掉出首屏**：表格默认总宽 1430px 超出 1440px 视口下约 1166px 的滚动口，该列表头在滚动口右缘被裁成「响应延…」；列宽修正（响应延迟 110→130，名称 200→170，冷却至 180→160）后该列回到首屏，残余轻微横滚由已固定的操作列与列设置承接（用户已持久化的列宽不受影响）。
- **移动端账号页头动作挤压修复（#1086）**：375px 窄视口下「添加账号」按钮与描述同 flex 行挤压、截断描述首行；页头对齐 checkin/路由页的 flex-wrap 模式，按钮窄屏独立成行。
- **今日快照 delta 不可用态去重（#1088）**：余额 7 天对比无数据时 Minus 图标与「—」占位符同形连读作「— —」；不可用态只留 em-dash 占位（零 delta 仍用 Minus 图标）。
- **下游密钥凭证维度（`allowedCredentialRefs`/`excludedCredentialRefs`）端到端修复（#1026 残留）**：`auth.ExcludedCredentialRef` 的 JSON 标签原为 snake_case（`site_id`/`account_id`/`token_id`），而管理端持久化形状为 camelCase（`siteId`/`accountId`/`tokenId`）——导致代理路径解析出的引用 ID 全为 0：允许列表密钥无法路由任何渠道、排除列表静默失效。标签统一为 camelCase 并新增 DB→策略解析往返回归测试。
- **运行时设置的并发撕裂读修复（#1079）**：`RuntimeSettings` 迁移为不可变快照交换——此前约 25 个运行时写字段与热路径无锁读并存，并发下可能读到半更新状态（如代理 token 校验瞬时 401 抖动）；快照迁移后读侧无锁且始终自洽。
- **W19–W21 前端审计修复批（#1080）**：错误提示去重单 owner 化（修复 HTTP 500 无 body 时双弹 toast，且 502–504 不再泄漏 axios 原始英文报错，改为状态感知的本地化文案）；列表页加载失败改为整块替换 + 内置重试（不再叠加在陈旧数据上）；站点公告四个筛选下拉关闭态不再裸显内部值 `all`；路由表单图标字段不再泄漏内部哨兵值 `__route_icon_none__`（以友好 token `none` 双向映射）；账号状态切换增加进行中禁用与成功反馈；OAuth 授权弹窗被浏览器拦截时提供常驻恢复入口；站点批量禁用与路由重建增加前置确认；多处操作后缓存失效遗漏补齐（签到后账号快照、目录源同步后模型清单、路由重建后渠道列表等）。
- **浅色主题图表与焦点环对比度（#1080）**：chart-1..5 全部预设在浅色卡片底上达 WCAG AA 4.5:1（对比度门禁固化）；焦点环统一为 ≥3:1 对比度 token。

### Security


- **SPA CSP 去 `style-src 'unsafe-inline'`（#1035 S2）**：CSP `style-src` 从 `'self' 'unsafe-inline'` 收紧为 `'self' 'nonce-<per-request>' 'sha256-<sonner-toast-css>'`，并保留其余 directive 不变；Go SPA fallback 在每个响应中生成 16 字节随机 nonce，注入 `<meta name="csp-nonce">` 并在 `Content-Security-Policy` 头中表达，前端静态 `bootstrap.js` 在 bundle 前把手动创建的 `<style>` 元素（sonner/chart/dialog scroll-lock 注入路径）统一标注该 nonce。sonner 的运行时样式表同时静态打包（`sonner/dist/styles.css`），即使 hash 漂移也不影响 toast 视觉；新增 CSP directive 级断言、nonce 随机性/meta 匹配测试、sonner hash 漂移守卫和 ChartStyle nonce 单测。残留：sonner 库仍会尝试注入样式（被 hash 允许），静态 fallback 保证功能；其他 directive 未放宽。
- **凭证导出遮罩改真占位符（#1080）**：遮罩态密钥不再以「CSS blur 伪掩码」渲染（明文此前仍留在 DOM 与无障碍树中，读屏可读出），改为渲染 `••••••••` 占位符 + `aria-hidden`，与顶层真掩码语义一致。

## [v0.16.19] — 2026-08-29

### Security

- **会话模型重构（#1034，#1057）**：管理员主 token 不再以明文存于浏览器 localStorage（原 12h 窗口）——登录改为 `POST /api/auth/login` 以主 token 换取服务端会话（`admin_sessions` 表，迁移步骤 `sc2_026`），凭证经 HttpOnly + SameSite=Strict 的 `metapi_session` cookie 携带，数据库只存 SHA-256 哈希；会话滑动续期（`ADMIN_SESSION_TTL_MINUTES` 默认 720，零配置安全），登出即服务端吊销，主 token 轮换吊销全部会话。Bearer 主 token 双轨保留（外部脚本兼容），前端不再持久化主 token；旧版 localStorage 明文键首次加载即清除。
- **WS 一次性 ticket（#1034，#1057）**：实时运维 WebSocket 不再接受 `?token=<主 token>` 查询参数——改由会话认证的 `POST /api/auth/ws-ticket` 签发 60s 单次 ticket，主 token 从此不进 URL（访问日志/代理日志/浏览器历史）。
- **失败认证纳入限速（#1034，#1057）**：per-IP 限速中间件移至认证之前，401/403 不再绕过桶约束；`/api/auth/*` 另加严格桶（`AUTH_RATE_LIMIT_RPS`/`AUTH_RATE_LIMIT_BURST` 默认 10/20，登录是唯一接受主 token 的表面）。
- **敏感操作主 token 重确认（#1034，#1057）**：备份导出（下载/WebDAV）、下游密钥导出、主 token 轮换要求 `X-Admin-Confirm-Token` 头重出示主 token（即使持有活跃会话），否则 403 `reauthRequired`；凭证导出对话框默认锁定遮罩（解锁需重输主 token），密钥默认遮罩显示，深链（Cherry Studio/CC Switch）打开前显式确认。

### Added

- **会话配置项（#1034，#1057）**：`ADMIN_SESSION_TTL_MINUTES`（默认 720）、`ADMIN_SESSION_COOKIE_SECURE`（默认 auto，按请求协议自适应 Secure）、`AUTH_RATE_LIMIT_RPS`/`AUTH_RATE_LIMIT_BURST`（默认 10/20）；均带安全默认值，零配置行为安全。`cmd/migrate` 方言迁移携带 `admin_sessions`（含 builder 列契约），会话跨 SQLite↔PostgreSQL 切换存活。

- **上游账号健康监测全局开关（#1027，#1056）**：新增运行时设置 `checkinEnabled`/`balanceRefreshEnabled`（热生效、持久化）+ 环境变量 `CHECKIN_ENABLED`/`BALANCE_REFRESH_ENABLED`；模型可用性探测改运行时热停；设置页双语开关 + `docs/deployment.md` FAQ。
- **账号代理支持 SOCKS5（#1009，#1059）**：账号 `proxyUrl` 接受 `socks5://` / `socks5h://`（后端 Go transport 原生支持，含进程内 RFC 1928 SOCKS5 服务器 E2E 测试）。

### Fixed

- **清空账号代理字段保存即清除（#1009，#1059）**：修复前端清空后载荷省略 + 后端 `MergeExtraConfig` typed-nil 删除缺陷双因，空值显式清除；账号更新路径补 scheme 校验。
- **路由健康懒加载数据竞态（#1052）**：`EnsureSiteRuntimeHealthStateLoaded` 快速路径无锁读改 `RLock` 读（-race 复现测试先行，行为零变化）。
- **调度器健壮性（#1061）**：job panic 统一 recover 边界（`safeJob`）；in-flight 标志锁外读竞态修复；`channel_recovery`/`backup_webdav`/`model_probe` 等吞没的 DB 错误经结构化日志显性化。
- **PG 方言陷阱清扫（#1060）**：约 260 处测试种子 + 3 处生产位点 BOOLEAN 列整数字面量绑定重写（PG 42804/22P02 类）；管理搜索 LIKE 大小写 `LOWER()` 统一；静态方言门禁扩至全包；零迁移。

### Changed

- **构建配置收敛 + vendor 块拆分（审计 S1+S9，#1053）**：`rsbuild.config.ts` 唯一 SSOT（删 `vite.config.ts`），devProxy/版本 define 收口 `web/config/build-shared.ts`，route-tree 前置校验；匿名 6432 块拆为 `vendor-i18n`/`vendor-icons`/`vendor-core`（总 JS −8.5KB gz，初始 +6.3KB gz 已记录）。
- **UX 残留清扫（审计 #1029/#1035，#1055）**：focus-ring 全库统一 `--focus-ring`（≥3:1 达标）；登录 autoComplete 评估定论；流量/成本图 sr-only 数据摘要表 + axe 组件级门禁。

### Performance

- **管理读路径索引（`sc2_027`，#1054）**：三个高频读路径索引（300k 行实测：渠道日志 channelId 过滤 17.9ms→0.5ms、siteId COUNT/summary 约 10x、range summary 约 3x）；proxy-logs LEFT→INNER、marketplace N+1 批量化；另 4 个候选索引实测无收益不加。

### Internal

- **出站 HTTP 客户端基线（#1058）**：15 处出站调用点统一 `internal/httpclient`（dial 30s/TLS 10s/idle 90s/池 100/20），AST 静态门禁拦截裸客户端。

## [v0.16.18] — 2026-08-29

### Added

- **SSE 流 chunk 间隔空闲超时（#1046）**：新增 `PROXY_STREAM_IDLE_TIMEOUT_SEC`（默认 300）——流式（SSE）响应每转发一个 chunk 重置计时窗，窗口内无新 chunk 即中断卡死的流并按上游超时故障记录（渠道健康、失败日志、终态一致）；只约束 chunk 间隔、不约束流总时长；`0`/负数/非法值回退默认。
- **重试/禁用状态码策略运营者可调（#1049）**：新增运行时设置 `proxyRetryStatusRanges` / `proxyDisableStatusRanges`（设置页代理与模型区块可编辑，`PUT /api/settings/runtime` 同契约）：retry 决定哪些上游状态码计为可重试的渠道故障，disable 决定哪些状态码在冷却升级之外直接禁用故障渠道（`enabled=false` + 人工覆盖标记）；默认值完全复刻既有判定，未配置零行为变化；区间表达式（`401`、`500-599`、逗号分隔）严格校验，非法值 400 拒绝。
- **批量测试闭环（#1047）**：模型测试台批量对比出现失败行时提供「禁用失败渠道」动作（人工确认对话框列明数量与人工覆盖后果），调用升级后的 `PUT /api/channels/batch`——部分更新语义（enabled/weight/priority 仅写字段出现的）、载荷校验（空批/超 1000/重复 id/无可更新字段）、逐项真值（`successIds` + `failedItems`）、整批成功 `success` 才为 true；写入经审计中间件记录。
- **渠道失败横幅一键过滤（#1048）**：渠道页存在冷却/熔断渠道时显示失败横幅（人工停用属运营意图、不计入），「只看失败」经可分享的 `?status=` 过滤参数进入仅失败视图；URL 已处于失败过滤时横幅切换为可清除指示。
- **下游密钥上游站点限制 UI（#1026）**：下游密钥创建/编辑表单新增「上游站点限制」多选（留空=不限制），把路由选择器既有的 `allowedSiteIds` 允许清单暴露到管理界面并完整往返；凭证（账号/令牌）维度仍可经 API 配置，另行立项。
- **搜索面板实体深链（#1039）**：CmdK 搜索面板的站点/账号/令牌/模型命中一键深链到实体本身（`/sites?edit`、`/accounts?accountId`、`/models?model`），参数一次性消费后清除，深链可分享。

### Fixed

- **路由自动重建真值（#1024）**：`POST /api/routes/rebuild` 现在消费 `refreshModels`（默认 true：先批量刷新全部活跃账号的上游模型、单账号失败不中断整批，再重建通道）；完成提示读取真实统计并区分三态——成功（真实路由/通道数）、无路由可重建（引导先创建路由）、通道无变化（引导核对账号模型与路由匹配）；docs/api.md 登记完整契约。
- **zh-CN 语言下站点表崩溃（#1036）**：`createdAt` 列对无效 Intl locale 的防护回退。

### Security

- **CSP 收紧（#1041）**：内联 bootstrap 脚本外部化，移除 `unsafe-inline`。
- **前端安全快赢批（#1037）**：体检安全批次修复。

### Changed

- **前端体检快赢批**：构建（#1040）、可访问性（#1042、#1043）、卫生（#1044）、UX（#1045）五批修复。

## [v0.16.17] — 2026-08-28

### Added

- **协议转换快照回归套件（#1018）**：46 份手写夹具 + 快照锁死协议转换层——Gemini generateContent 请求转换（thoughtSignature 哨兵/工具调用/多模态占位/thinkingConfig 与 tool_choice 矩阵）、Responses 连续性策略与 reasoning 清洗决策表、响应/SSE usage 提取四形状与增量流解析（含 7 字节分块边界）、completions/embeddings/images 恒等契约；新增快照测试 harness（`GOLDEN_UPDATE=1` 重写，纪律入 `docs/testing.md`），零生产代码改动。
- **行级探测健康条（#1020）**：渠道/账号表格新增探测历史健康条——新增只读端点 `GET /api/channels/probe-history` 与 `GET /api/accounts/probe-history`（`limit` 1–50 默认 20，单条窗口查询一次取回全表历史，无 N+1）；竖条按时间着色 success/failure/inconclusive/skipped，tooltip 汇总窗口成功率与平均延迟，键盘可达 + aria 摘要；批量查询未定显示占位、无历史如实标注。
- **结构化冷却原因（#1019）**：`route_channels` 与 `oauth_route_unit_members` 新增 `cooldown_reason_code`/`cooldown_reason`/`cooldown_reason_at` 三可空列（additive step `sc2_025`，双方言 + 迁移工具携带）；9 码只增词表（`usage_limit`/`rate_limited`/`auth_error`/`upstream_error`/`client_error`/`timeout`/`network_error`/`probe_failure`/`unknown`），错误摘要净化并截断 200 runes；流量与探测失败双路径记录、全部清除点同步清原因；渠道页冷却/熔断徽章可点击，根因弹窗含本地化触发码、错误摘要、记录时间与剩余倒计时，旧数据如实显示「原因未记录」；`GET /api/channels` 新增三个 camelCase 字段（`docs/api.md` 同步）。

## [v0.16.16] — 2026-08-28

### Added

- **定时上游模型同步（#1005）**：新增 `MODEL_SYNC_CRON`（默认 `0 4 * * *`，每日 04:00）定期批量刷新全部活跃账号的上游模型列表；设置页 `modelSyncCron` 支持运行时热更新（校验拒绝非法 cron）；单账号失败不中断整批，批量完成后路由重建与缓存失效恰好整体发生一次。手工单账号刷新端点行为与响应载荷不变。
- **上游代理超时可配置（#1009）**：新增五个 `PROXY_*_TIMEOUT_SEC` 环境变量——`PROXY_CONNECT_TIMEOUT_SEC`（默认 2）、`PROXY_TLS_HANDSHAKE_TIMEOUT_SEC`（10）、`PROXY_RESPONSE_HEADER_TIMEOUT_SEC`（30）、`PROXY_IDLE_CONN_TIMEOUT_SEC`（90）、`PROXY_REQUEST_TIMEOUT_SEC`（30）——按部署调优出站站点代理/上游请求超时；`0`/负数/非法值回退默认，未配置时零行为变化。

### Changed

- **竞品研究文档**：新增 `docs/internal/analysis/competitor-study-2026-08.md`（new-api / axonhub / sub2api 对标与可执行建议），为后续波次提供方向输入。

## [v0.16.15] — 2026-08-27

### Fixed

- **账号表单验证真值（#1007）**：inline token verification 与账号创建现在使用表单中尚未保存的 `platformUserId` / `proxyUrl`；显式表单代理覆盖站点、Resin 与系统代理，校验 `http(s)`/`socks` URL，创建后持久化 `extraConfig.proxyUrl`，非法值 fail-closed。
- **Accounts 分页（#1008）**：URL 控制的页码选择不再被 TanStack 自动重置；真实表格回归测试确认第 2 页稳定渲染第 11–20 行。

## [v0.16.14] — 2026-08-27

### Changed

- **账号 token 同步 UI 真值（#1002 后续）**：账号创建/登录后的 toast 如实报告后端 `tokenSyncStatus`/`tokenCount`/`tokenSyncMessage` 四态——synced 显示真实持久化计数、empty 提示暂无上游令牌、failed 降级为部分初始化警告（账号保留、可在令牌面板重试同步）、skipped/旧响应保持原引导文案；`/token-routes` CTA 在所有状态保留。

## [v0.16.13] — 2026-08-25

### Added

- **Session 账号 token 自动同步（#1002）**：账号创建/登录后经既有同步路径自动同步上游 token；响应报告真实持久化的 `tokenCount` 与 `tokenSyncStatus`/`tokenSyncMessage`；同步失败仅发出部分初始化警告并保留已验证账号，绝不回滚；API-key 连接显式跳过；移除硬编码 `tokenCount: 1`。
- **账号 Models 面板（#998）**：账号详情新增 Models 面板——手工上游刷新、手工添加与显式移除模型、来源/可用性状态诚实呈现；刷新把可用性持久化到既有 owner，路由重建与缓存失效每次刷新动作恰好发生一次；上游失败诚实报告且无副作用。

## [v0.16.12] — 2026-08-25

### Added

- **下游 key 模型授权 UI（#999）**：密钥表单新增模型授权编辑器——精确模型名、glob 通配（`*`）、`re:` 正则；空即拒绝所有、`*` 为显式全允许，后端既有 fail-closed 过滤保持唯一执行点。
- **账号表单可搜索站点选择器（#1001）**：账号创建/编辑对话框的站点选择支持名称/URL/平台搜索与键盘选择，写入数字 `siteId`，保留深链预选。
- **上游公告进入 attention（#1000）**：未读站点公告作为条件派生条目进入 attention bell 与仪表盘待办面板；读状态唯一来源为 `site_announcements.read_at`，审计事件行不再产生重复条目。
- **截图数据 profile 门禁**：截图扫描必须先声明 `empty` 或 `seeded`；profile 与实际数据不符时在截图前即失败，配套静态测试。

### Fixed

- **Sites 分页 URL 状态保持（#996）**：表格内部页码重置后，页码仍由 URL 参数控制，第 2 页稳定渲染 11-20 行。
- **公告来源链接安全（#1000）**：公告页外链只接受基于受信本地站点 URL 解析出的 HTTP(S) 地址；不安全或未知的来源地址回退为站点首页且永不渲染为外链。
- **attention 条目语义显式化（#997）**：attention bell 明确为“未解决条件条目”，条件解除即消失，不提供伪造的客户端清除按钮；事件条目按未读过滤并携带 program-logs 深链参数。

## [v0.16.11] — 2026-08-25

### Added

- **OAuth start 流闭环**：Start-OAuth 成功后呈现 state / redirect URI / SSH 隧道命令（均可复制），经 `getOAuthSession` 有界轮询（30 次 × 2s，卸载/取消即清理）；超限显示等待态而非假成功，支持手动粘贴回调 URL（`submitOAuthManualCallback`，输入校验 + 成功/失败明确反馈）。
- **对比行重跑**：model-tester 批量对比结果支持行级 re-run（含失败/中止行），复用原 payload 与既有探测机制，行级 pending + Stop 可中止。
- **Golden 回归扩容**：visual-regression 基线由 4 页扩至 10 页（全部空库、布局稳定、日期无关），契约同步入文档。

### Fixed

- **Accounts 行级 pending**：pin / check-in 切换补行级 pending 反馈（Spinner + disabled），与 status 切换互不串台。
- **路由页视图持久**：`showZeroChannel` 开关持久化到 localStorage，视图选择跨导航与刷新保留。
- **前端测试真值**：移除前端单元测试中对 unhandled 错误的全局豁免开关（移除后全套零 unhandled 错误）；a11y-scan 路由对齐 route-smoke 单一来源（15 → 41 路由），41 路由 0 serious/critical。

### Accessibility

- model-tester 双 slider thumb 补 aria-label；observability 热力图 bucket 补 img role 语义；audit-logs method 筛选补 aria-label。

## [v0.16.10] — 2026-08-25

### Added

- **站点快捷跳转（#985）**：站点列表名称/URL 列提供快捷跳转链接；仅合法 http(s) 地址渲染为链接，非法 scheme 保持纯文本，含键盘/焦点/新标签语义。
- **站点公告页（#986）**：新增独立 `/site-announcements` SPA，聚合上游站点公告，读/同步/空/错误状态齐备；公告正文按不可信文本渲染，不渲染来源外链。

### Fixed

- **站点公告 API 契约（#986）**：admin API 统一 camelCase；同步按站点诚实报错、计数器只统计成功写入；`siteId` 严格校验（未知站点 404，拒绝重复键/浮点/未知字段）；失败后台任务保留结构化结果供 UI 呈现部分真值。
- **上游公告信封校验**：newapi/donehub 适配器校验 success 信封与 data 类型，呈现上游失败消息而不是制造内容。
- **产品公告真值（#992）**：设置/仪表盘公告链接只接受绝对 http(s)；加载失败显示错误横幅与重试；dismiss 失败有本地化 toast 且横幅保留。
- **门禁稳定**：修复高负载下抖动的代理首字节计时测试；文档卫生门禁不再扫描 gitignored 的本地工作区（.dev-local）。

### Security

- **SSRF 加固**：新增 `internal/ssrf` 主机名策略与 DialContext 守卫，在所有站点代理传输（普通/池化/uTLS）拨号前拒绝云元数据/链路本地目标；RFC1918/localhost 保留给实验环境。站点 URL 校验收紧（拒绝 opaque、内嵌凭据与非法端口）。
- **工程门禁（#991）**：项目 pre-push CI 门禁重新挂回全局 hook 链；release.sh 在打 tag 前校验 master 与远端同步。

## [v0.16.9] — 2026-08-24

### Added

- **模型数据源多源注册表（#971）**：llm-metadata 与 models.dev 双源合并为统一模型目录，支持自动/手动同步；models 页水合真实目录数据。
- **设置中心语义重组（#971/#972）**：设置拆分为基础、代理与模型、下游、通知与数据、系统与运维五组，旧 URL 自动重定向；目录源改为指针拖拽排序。
- **目录倍率计价（#972）**：消费 llm-metadata 的 newapi ratio 倍率用于转发成本估算，并由目录数据推导支持的端点类型。

### Fixed

- **前端体验整修（#970）**：覆盖 12 个域的 55 项交互/视觉/移动端/无障碍修复，含侧边栏「路由」导航静默失败等。
- **14 条产品语义修复（#971）**：模型、路由、设置等用户可见文案与行为修正。
- **模型目录方言修复（#972）**：部分 Claude 模型被误标为 openai 协议，优先使用原生 dialect 恢复为 anthropic。
- **移动端交互审计（#972）**：修复两处小于 24px 的触控目标（签到「查看原始信息」、倍率行内编辑），并逐项核验其余遮挡/命中区信号为误报。
- **data-table 自动重置渲染环（#972）**：修复 keys 页跨 root flushSync 乒乓导致的冻结。

### Performance

- **首屏 bundle 拆分（#972）**：双语言 locale 改为按需懒加载，入口 chunk 减少约 59%（303.6 → 123.5 KB）。

### Accessibility

- **主题对比度归零（#972）**：清除全部 8 项 sub-AA 对比度豁免（删除 2 个闲置 token + 修复 6 个 preset residual），10 个主题 × 明暗模式全部通过 WCAG AA 4.5:1。

## [v0.16.8] — 2026-08-23

### Security

- **修复备份导入安全问题（#941）**：恶意备份文件可能被利用来外传凭据，建议旧版本及时升级。

### Added

- **站点 API 端点编辑器（#935）**：站点表单新增多行端点编辑，带规范化、去重与安全地址校验；未编辑的端点保持原语义。
- **站点探针产品化（#939）**：新增探针实时报告视图、手动触发与最近运行历史；模型批量探针对话框如实呈现失败与未知状态。
- **代理全失败告警（#942）**：代理全部不可用时触发告警通知，同一模型 5 分钟内不重复告警。
- **API 文档补齐（#940）**：补全 API 文档中缺失接口的参数与响应说明。

### Fixed

- **可空字段处理修复（#943）**：路由、站点、OAuth 相关数据的可空数值字段正确处理，单条空值不再导致路由装载失败。
- **告警条目页深链接（#936）**：告警条目改为站内深链，刷新后仍停留在对应条目，不丢失上下文。
- **主题对比度修复（#949）**：修复 6 处主题对比度不足（含 rose-garden 暗色主题与 lake-view 侧栏），均达到 4.5:1 以上。
- **导入与交互修复（#948）**：恢复导入向导工具栏入口、完成步骤直接跳转创建账号、下游密钥列表移动端卡片适配、签到按钮加载状态可用。
- **服务停机与启动体验（#946）**：停机时等待在途调度任务执行完成（上限 5 秒）；启动时请求错峰，避免启动峰值。
- **后端消息统一（#938）**：后端用户可见消息统一为英文，与上游错误消息保持兼容。

### Performance

- **代理转发（#944）**：无映射请求走零拷贝短路径，流式响应单遍处理，显著降低代理热路径延迟与内存占用。
- **路由匹配（#947）**：路由匹配引入缓存与惰性扫描，刷新开销大幅下降；顺带修复一处数据竞争。
- **代理日志统计（#945）**：代理日志与统计查询优化，大数据量下查询与翻页显著提速。

## [v0.16.7] — 2026-08-22

### Security

- **修复路径穿越安全问题（#922）**：修复代理转发中因危险路径段引发的越权访问风险，建议旧版本升级。
- **修复导出文件安全问题（#925）**：修复导出文件可能的公式注入风险，建议旧版本升级。
- **安全加固（#917）**：公共路由拒绝危险路径段，联网请求拦截补齐全部私网网段，建议旧版本升级。

### Added

- **迁移工具强化（#919）**：迁移前校验备份文件完整性，被篡改的备份会被识别并拒绝。

### Fixed

- **监控接口认证修复（#922）**：修复监控接口 cookie 认证失效问题。
- **空余额账号 500 修复（#931）**：账号余额为空时，站点列表正常展示，余额刷新任务不再意外中断。
- **空库快照修复（#928）**：账号/站点数据为空时返回空数组，消除前端静默反复重试。
- **假成功修复（#930）**：签到触发、批量触发与代理测试不再忽略失败结果，如实展示真实状态。
- **界面交互修复（#924）**：新建站点后账号添加按钮立即可点、侧边栏点击区域加大、登录页焦点顺序修正。
- **无障碍修复（#920）**：表单错误提示对屏幕阅读器可见，导航竞态问题修正。

### Changed

- **部署体验（#921）**：新增一键安装脚本，新用户三步完成部署；默认使用命名卷持久化数据；README 中英文一致。
- **README 重构（#929）**：定位、功能总览与三步快速开始，中英文版本同步。
- **文档校准（#916）**：API 文档与实际行为对齐。

## [v0.16.6] — 2026-08-21

### Added

- **静态资源 gzip 压缩（#910）**：内置前端资源启用 gzip 压缩传输，页面加载更快。

### Fixed

- **API 字段命名修复（#911）**：账号令牌接口字段统一为 camelCase，布尔字段类型修复，客户端解析不再丢失字段。
- **更新丢失字段修复（#911）**：站点、账号、路由更新时会话启用与协议限制等字段此前被静默丢弃，现已正确保存。
- **频道列表不再截断（#911）**：未传分页参数的频道列表返回全量数据，不再固定截断为 50 条。
- **路由频道状态补齐（#911）**：路由详情补充各频道的成功、失败计数与冷却状态。
- **账号大列表卡顿修复（#910）**：账号列表渲染优化，百行级数据不再冻结页面主线程。
- **仪表盘提速（#910）**：图表库改为按需加载，不再阻塞首页渲染。

## [v0.16.5] — 2026-08-21

### Added

- **全局告警红点（#902）**：顶栏新增告警铃铛入口，告警信息全局可及。
- **OAuth 连接详情（#903）**：OAuth 连接列表支持下钻查看详情。
- **关于页构建信息（#904）**：新增版本信息接口，关于页展示真实版本与构建信息。
- **代理日志与频道筛选（#905）**：代理日志列表支持按频道过滤；频道列表新增状态筛选。

## [v0.16.4] — 2026-08-21

### Fixed

- 修复未主动刷新的账号金额字段常显「0」、制造「余额归零」假象的问题：未刷新的金额统一显示为 em dash 占位，不再伪造数据（#889）。
- 危险操作补充确认：WebDAV 导入与 usage-log 清空前增加确认对话框；路由表单 Cancel 与脏表单关闭统一走路由守卫（#889）。
- 修复写操作 toast 重复或缺失反馈的问题：调用方已提示的写操作不再全局重复弹 toast；批量路由启用/停用补充结果 toast；复制操作补充反馈；checkin trigger 与站点开关增加行级 pending 状态（#889）。
- 移动端适配：页头动作簇响应式收纳（换行 + More 菜单）；小屏下 sheet 全宽面板 + 可滚动主体；图标按钮触控目标加大到 40px；表格/侧栏断点常量共享（#889）。
- 信息架构断链修复：所有 workspace 页面补充 `document.title`；observability proxy-logs 增加直达入口与跨页链接；侧栏打磨（#889）。
- 统一数据展示层：locale 感知的日期时间、货币、图表/延迟、千分位与 em dash 全站统一，并修复货币双前缀问题（#889）。
- 性能与稳定：realtime WebSocket 自动重连并标记数据新鲜度；账号行开关乐观更新；账号深链 pageSize 收敛 UI 上限；OAuth 连接列表服务端分页；公告 banner 走查询缓存；工具栏搜索 300ms 防抖（#889）。
- 主题与会话：补全主题系统（内容布局轴、系统色彩方案、预设整理）；登录页补 token 指引与 token 轮换后自动登出；会话过期保留返回路径、无效 token 主动登出；Electron 等待屏增加服务器退出失败态；认证设置错误消息语言中立（#889）。

### Added

- ⌘K 命令面板：全局导航层 + 过滤日志深链（#889）。
- 页头用户菜单：登出 / 版本 / About / 文档入口（#889）。
- 共享 DetailField 原语：七个详情 sheet 收敛为同一堆叠字段布局；非表格空态统一 Empty 原语（#889）。

### Changed

- 视觉一致性打磨：错误重试图标统一（RefreshCw + 静态守卫）；清理 gap 控件双重图标间距；settings 分区垂直节奏统一；批量栏尺寸与 RoutePending 壳同构；dark inset 边缘等细节修正（#889）。

## [v0.16.3] — 2026-08-21

### Fixed

- 修复 Dashboard 告警（attention）深链全线失效的问题：后端发出 SPA 未消费的跳转目标，点击 100% 落空。现改为正确的深链（站点编辑、设置页入口）；账号页新增 `accountId` 一次性深链消费，打开账号详情 sheet，过期 id 静默清除（#890）。
- 修复首次接入流程死胡同：导入向导完成步骤只有统计 + 关闭，且导入后不触发路由重建，用户不知道模型还不可路由。新增主 CTA「重建路由」+ 次 CTA「添加账号」+ 说明文案；下游密钥创建成功后自动打开「接入」对话框；接入与路由完成引导改为 SPA 导航，不再整页刷新丢失状态（#892）。
- 修复下钻死胡同：channel 详情「编辑路由」此前实际打开只读详情——routes 页新增一次性 `edit` 参数改为打开真编辑对话框；observability 熔断/冷却表补出口链接；model-tester 对比结果每行通道可点击跳转；models / checkin 空态补 CTA（#893）。
- 修复站点创建后账号页缓存滞后：此前创建站点只失效路由缓存、未失效账号快照缓存，新建首个站点后「添加账号」按钮最长 30 秒被陈旧快照禁用；现与更新/删除操作对齐补缓存失效（#895）。

### Added

- 站点页新增「余额」列（余额 → 订阅剩余阶梯兜底）+ 详情余额/订阅区块（Plans / 月用量 / Remaining / 下次到期）+ 端点冷却与失败原因状态；账号详情补 quota / unitCost / lastCheckinAt / 健康原因；路由详情通道指标新增 success/fail 命中计数；OAuth 新增 quota（5h/7d 窗口）、planType、参与路由数、最近同步错误列。空值一律显示占位或整块隐藏，不伪造数据（#894）。
- `prefers-reduced-transparency` 无障碍支持：topbar、批量操作浮动条与 dialog/sheet/alert-dialog 共 5 个玻璃表面，在系统「降低透明度」偏好下降级为实心底并移除 blur；默认外观零变化（#896）。

### Changed

- 设计系统对齐：accounts/checkin/token-routes 行操作按钮换用 ui/button 原语，恢复键盘焦点环；sign-in 页标题回到规定字阶；sites 页迁移共享 QueryErrorBanner；about 页版本号改为构建期从 package.json 注入，修复此前硬编码与实际版本脱节的问题（#891）。
- model-detail 定价文案改走 i18n（#896）。

## [v0.16.2] — 2026-08-20

### Fixed

- 修复从旧版后端（TS）迁移的数据库在启动后查询站点时崩溃的问题（#849，hb0730 报告）：旧库缺少历史迁移新增的列，查询报 `no such column` 错误。现在启动时自动补齐全部缺失列，老库无需手动操作（#878）。
- 修复 Docker 数据目录权限导致启动失败的问题（#849）：Go 镜像以非 root 用户（uid 1001）运行，旧版以 root 写入的 bind mount 数据目录会触发只读数据库错误。现在启动前探测数据目录与既有库文件的可写性，失败时给出可操作的 `chown`/`chmod` 提示；README、docker-compose.prod 与迁移、部署文档补充命名卷零配置 vs bind mount + chown 指引与 `ACCOUNT_CREDENTIAL_SECRET` 使用说明（#875）。
- 修复 `metapi-migrate --verify` 校验和误报（共 4 处）：目标侧哈希限定源列集合（列序无关）、settings 源侧过滤运行时键（db_type/db_url/db_ssl）、行哈希按规范化串排序（行序无关）、跨方言布尔规范化（SQLite 0/1 vs PG true/false）（#875）。

## [v0.16.1] — 2026-08-19

### Fixed

- 修复 OAuth Start-OAuth 流程断链：提交时后端返回的 `state`/`instructions` 被丢弃，用户无法轮询会话或手动提交回调。现保留 pending 状态并渲染 pending 面板（OAuth session 轮询 + SSH 隧道命令复制 + 手动回调输入框），成功后自动关闭、失败保留重试（#862）。
- 修复 Token-routes 链上下文 banner 显示原始 `#ID` 的问题：现在读取已加载的账户/站点数据解析为用户名/站点名，`#ID` 仅作兜底（#862）。
- 修复 site-form 的 3 个 Select 缺失 label 关联的问题：补 `FormControl` 包裹，恢复标签与控件的关联（#862）。
- 8 个页面（accounts/channels/oauth/models/proxy-logs/fix-candidates/price-compare/checkin）此前错误态只显示 banner、无重试：新增共享 QueryErrorBanner（alert 语义 + 可选 Retry + spinner），8 页统一接入获得重试（#862）。
- 修复 Routes 行级操作（toggle/clear-cooldown）无 pending 反馈的问题：下拉项在操作期间显示加载图标并禁用（#862）。
- 修复 showZeroChannel toggle 位于表格下方的问题：移入 toolbar 视图切换槽（#862）。
- 修复 Sites 页面无法通过 `?edit=<id>` 深链直达编辑的问题：新增一次性消费的深链支持（#862）。
- Channel-detail footer 此前仅在冷却时渲染、无编辑入口：改为常驻，并新增「Edit route」按钮跳转到路由详细页复用下钻（#862）。

## [v0.16.0] — 2026-08-18

### Fixed

- 修复编辑站点表单时自定义请求头覆盖设置被静默重置的问题，并新增可见开关控件（#851）。
- 修复令牌路由列表中「站点」列与详情恒为空、全局筛选无法按站点名检索的问题（#854）。
- 修复令牌路由数据刷新竞态下可能引发的整页崩溃（#855）。
- 修复实时连接多次重连失败后静默放弃、面板像无流量一样的问题：新增连接丢失提示与手动重连按钮（#850）。
- 账号列表启用/禁用改为行内按钮操作，并显示行级加载状态（#824）。
- 统一页头高度取值来源，修复不同页面页头高度不一致的问题（#824）。
- 修复代理日志的客户端、来源、目标过滤此前不生效的问题，过滤统一改为服务端执行（#832）。
- 设置页各分区在数据加载失败时显示错误状态，空态提供内联创建入口（#832）。
- 站点表单校验文案对齐、错误态增加重试按钮（#851）。
- 令牌路由列表与详情页修正误导性文案，补充空态引导与错误重试（#855）。
- 状态徽章统一为语义化组件样式（#825/#827）。
- 渠道空态增加「管理账号」入口（#839）。
- 渠道详情增加路由冷却清除操作（#834）。
- 密钥列表操作增加成功提示（#841）。
- 路由表单在草稿段为空时补充重新构建指引（#837）。
- 设置页移动端导航改为横向滚动分段样式（#831）。
- 代理日志详情中的渠道、账号、路由、令牌 ID 可点击跳转到对应过滤视图（#843）。
- OAuth 绑定/刷新操作增加行级加载状态与逐账号错误提示（#845）。
- 修复菜单的键盘 Esc 关闭行为（#833）。
- 骨架屏加载动画改进，按列宽渲染（#824）。

### Added

- 站点表单新增探测延迟阈值配置项（#853）。
- 无站点时仪表盘展示引导横幅，可一键跳转创建站点（#838/#842）。
- 仪表盘统计卡片可点击跳转到对应详情页（#828）。
- 凭证导出弹窗增加「发送测试请求」入口（#828）。
- 下游密钥支持编辑（密钥值不可修改），保存时仅更新变更字段（#835）。
- 价格对比页新增跳转到对应模型令牌路由的入口（#835）。
- 模型测试器展示本轮令牌用量（提示/完成/总量）（#840）。
- 可用性趋势图按健康状态分档着色（#846）。
- 「关注」列表时间显示改为相对时间（#847）。

## [v0.15.3] — 2026-08-17

### Fixed

- 修复弹窗底部操作栏半透明背景导致滚动内容透出的问题（#822）。
- 修复警告弹窗内容无高度约束、长内容溢出视界的问题（#822）。
- 修复长表单滚动时弹窗标题与描述滚出视界的问题（#822）。
- 修复弹出气泡长内容溢出视界的问题（#822）。
- 修复站点表单校验失败时无任何反馈的问题，新增错误提示（#822）。

## [v0.15.2] — 2026-08-17

### Fixed

- 修复弹窗长内容溢出视界、提交按钮不可达的问题（#815）。

## [v0.15.1] — 2026-08-17

### Fixed

- 修复 PostgreSQL 环境下 `/api/models/token-candidates` 接口因布尔列比较写法不兼容而返回 500 的问题（#805）。
- 修复接口内部出错时服务端无错误日志的问题：现在会记录错误并附带请求 ID（#805）。

### Changed

- 产品品牌文案统一为「Metapi」，界面文案与文档同步更新；接口行为不变。
- 品牌 Logo 更新为透明底 π 字形蓝青渐变图标（亮/暗主题通用），同步更新 favicon 与桌面图标。
- 登录页标题加大字号并精简冗余文案；README 增加品牌横幅。

### Added

- 部署文档的 Nginx 反向代理模板补充 WebSocket 升级头，避免 WSS 握手在代理层被中断（#805）。

## [v0.15.0] — 2026-08-17

### Added

- 站点表单支持按站点覆盖粘性代理与 TLS 指纹设置，可继承全局或强制开启、关闭（#807/#809）。

### Fixed

- 修复粘性代理与 TLS 指纹设置无法通过 REST API 保存的问题（#807）。
- 修复通知设置重启后静默回退的问题（#807）。
- 导入流程改进：显示每条记录的失败原因、修复无法识别 URL 时提示刷屏、补充无障碍区域与标签关联（#808）。

## [v0.14.0] — 2026-08-17

### Added

- 账号表单新增密码凭证登录模式，支持直接使用账号密码接入上游站点（#770/#772/#774/#775）。
- 路由策略新增最低成本、最少忙、最低延迟选项，并接入官方价格目录估算成本（#783/#790）。
- 站点/模型熔断器支持半开探测，已恢复的通道可重新进入候选（#791）。
- 接入向导增强：环节间自动传递站点/账号预选、内置凭证校验、创建路由时预填通道草稿（#796）。
- 路由详情页展示每个通道的配置权重、启用占比（停用通道不计入）与规范化输入/输出单价，并标注价格来源（#799）。
- 引导流程改进：创建结果按后端真实响应解析、编辑时保留脱敏凭证、批量通道部分失败时明确提示（#800）。
- 模型测试台改进：保留上游状态/延迟/错误信息、停用通道不参与比较、停止请求独立计数（#801）。
- 列表页以 URL 为分页、筛选、搜索、排序的唯一状态来源，刷新或分享链接可复现页面状态（#802）。

### Fixed

- 修复前端对旧版 URL/搜索参数的兼容问题，并补齐参数校验与本地存储边界（#767/#780）。
- 修复与 new-api v1 登录响应、one-api 识别、session 登录及 CLIProxyAPI/Sub2API 凭证导入的兼容问题（#768/#769/#773/#776/#777）。
- 修复批量写入器关闭时的稳定性问题，以及第一个模型为空值的处理（#771/#778）。
- 修复用量统计缺少缓存明细时错误套用缓存折扣的问题（#788）。
- 修复用量聚合刷新失败时统计静默丢失的问题（#789）。

### Changed

- SQLite 默认启用更快的同步级别与连接级缓存；成功请求会逐步衰减历史失败计数，已恢复的通道不再长期受罚（#784/#785）。

## [v0.13.0] — 2026-08-15

### Security

- /v1 接口支持按 IP 限流，并可配置令牌频率限额与请求体大小上限（#709）。
- 修复 WebDAV 的 SSRF 安全问题（#741/#761/#763），建议旧版本升级。
- 管理端列表接口不再返回明文凭证，改为脱敏展示（#719）。
- 修复 OAuth 出站与 WebSocket 来源校验相关的安全问题（#702/#726）。

### Added

- 平台自动识别：修复 one-hub、done-hub、veloera、sub2api、cliproxyapi 无法自动识别及 one-api 被误标为 new-api 的问题，新增商汤 SenseTime 平台检测（#684/#689/#706）。
- 出站请求使用统一浏览器 UA，支持按站点注入 cf_clearance、可选 TLS 指纹伪装与连接复用（#687/#688/#694/#701）。
- 模型列表拉取：修复异常响应时静默返回空列表的问题，并统一规范化、去重、排序；Sub2API 按分组返回可用模型（#683/#690/#695）。
- 自动签到增强：已签到幂等识别、同站限速、临时失败重试、登录态自愈、重启后补签、失败通知聚合（#691/#692/#699/#700）。
- 新增粘性代理池（#693/#698）、Grok/xAI OAuth 适配器（设备授权，#696）、图片生成接口透传（#697）、Electron 桌面客户端（#704）。
- 新增可选的 Prompt 过滤，降低 OAuth 账号池被封风险（#702）。
- 前端功能：客户端配置一键导出（Cherry Studio / CC Switch 深链）、⌘K 全局搜索命令面板、首页今日快照、告警消息富化、模型测试台会话与模板库、可观测性面板自动刷新、代理日志 CSV 导出、通道详情面板、订阅汇总、数据库迁移管理端点（#657/#658/#659/#660/#662/#711/#713/#708/#722）。

### Fixed

- 修复仪表盘单查询失败导致整页报错的问题（#714）。
- 修复管理端与处理器数据库错误传播问题（#746/#748/#759/#763/#716）。
- 修复列表端点分页边界与配置校验问题（#725/#728）。
- 修复 SSE 错误事件、路由错误边界与调度器关闭时任务未取消的问题（#726）。

### Changed

- 日志级别现在可通过配置调整（#730）。

### Performance

- 仪表盘聚合加入缓存，代理日志改为异步批量写入（#710）。
- 修复路由汇总接口的 N+1 查询问题，模型写入改用 upsert（#724）。
- 移除体积较大的前端图表依赖，设置页代码按需加载，包体明显减小（#712/#718）。
- 管理端探测改为并发流式处理，耗时显著缩短（#727）。
- Docker 镜像构建使用构建缓存挂载，构建速度提升（#729）。

## [v0.12.0] — 2026-08-14

### Fixed

- 修复离线迁移工具静默丢失数据的问题：迁移流程改为复用正式建表逻辑，并增加结构一致性守卫（#651）。
- 修复运行时切换数据库时丢失 SSL 模式与连接池配置的问题（#651）。
- 修复管理端路由解释遗漏令牌与 OAuth 检查项的问题（#653）。

## [v0.11.0] — 2026-08-14

### Security
- 管理控制台 token 列默认脱敏显示（#601–#602）。
- 生产部署配置安全加固：健康检查、禁止特权提升（no-new-privileges / 丢弃 capabilities）、只读根文件系统、临时目录隔离与资源限制（#639）。

### Added
- 管理控制台 UI/UX 与功能全量交付（#633/#634）：
  - UX 基础：共享格式化器、状态/空态组件、toast 提示、数字动画、响应式表格与移动端降级、无障碍支持、页面标题缩放（#594–#600）
  - 可观测性工作台：Overview / Health / 代理日志页面、访问日志指标、进程崩溃恢复日志携带请求标识（#603–#610）
  - 导入：站点探测 + 统一导入向导 + 幂等批量导入（#611–#616）
  - 模型：定价层、价格对比、修复候选与推荐（#617–#621）
  - 通道：只读列表 + 重建过滤（#622–#626）
- 路由重建接口返回新增 changed 统计；无实际变化时跳过不必要的重建。
- 访问日志新增 status / bytes / duration_ms 字段；SSE 与 WebSocket 长连接的访问日志不再丢失；/metrics 新增 Go 运行时指标（goroutine / 内存 / GC）（#593）。
- .env.example 补全约 80 个可选配置项（#640）。
- API 文档补全 `/api` 端点清单（#643）。

### Fixed
- 修复周期任务从未执行：余额刷新 / 日志清理 / WebDAV 备份此前未被调度器注册（#635）。
- 旧版本数据库平滑升级：增量迁移兼容已有结构，迁移文档明确只进不退（#636）。
- 前端无障碍标签全部改为国际化文本，移除硬编码（#642）。

### Changed
- 发布流程：SemVer tag 触发多平台二进制构建（含校验和）与 GitHub Release；master 变更仅更新镜像（latest + sha）；CI/CD 合并为单一主流程。
- 新增发布脚本：校验 tag / 前端版本 / 变更日志节一致性后打 tag 并推送。
- 依赖与维护更新：GitHub Actions 组件与前端依赖升级（#584 / #588 / #592）；Dependabot 分组与升级流程文档（#591）。
- 发布资产新增 install.sh 安装脚本，附带 sha256 校验（#637）。

## [v0.10.0] — 2026-08-12

### Added
- 定时计划配置增强：新增 v1 版计划格式（每日 / 间隔 / 随机窗口 / 自定义 Cron），提供预览与应用迁移接口，完整保留旧字段兼容。
- 设置页统一表单：保存流程、加载错误状态、未保存变更离开确认、计划语义化控件、响应式导航。
- 审计日志服务端分页（limit/offset）+ 前端分页表格（页码、上一页/下一页、筛选后重置回第一页）。
- 数据库设置页改用统一脏值保护表单；应用内迁移按钮移除，改为提示通过 CLI 执行迁移。
- 中文字体回退完善：CJK 无衬线字体栈（Noto Sans SC/TC/JP/KR、PingFang SC、Microsoft YaHei），zh-CN 界面字体不再随机。
- 小屏设备顶栏新增侧栏开关。
- 品牌名统一 **MetAPI**；透明 SVG 徽标与 favicon 替换旧版 PNG。
- 顶栏语言切换（en / zh-CN），自动跟随浏览器语言，页面 lang/dir 同步。
- 主题定制面板：配色 / 字体 / 圆角 / 缩放 4 轴，可单独重置；全部预设默认使用无衬线字体。
- 侧栏导航完整本地化。
- 社区文档：贡献指南、安全说明、行为准则、issue 模板；dependabot 覆盖 Go / npm / Actions / Docker 依赖。
- 发布产物增强：多平台二进制与校验和、Docker 镜像 amd64 + arm64 双架构、`--version` 版本输出。

### Fixed
- 旧 cron 值与会话 v1 计划保持同步（含混合新旧格式的配置）。
- 通知文本与任务静音配置持久化改为重启安全格式；PostgreSQL 设置审计写库问题修复。
- 修复部分保存会清空未触碰项（通知开关、WebDAV 字段、白名单、掩码令牌输入）。
- 设置保存错误只提示一次，不再多处重复弹窗。
- 修复等价配置仍被要求重启：变更检测基于实际差异；SQLite 连接串路径规范化；兼容旧编码的 db_ssl 值。
- 更新中心版本列表不再显示开发占位版本号；程序日志区适配不同响应格式并展示加载错误态。
- 修复表格排序/分页与 URL 不同步：同路径 search 导航后表格滞后，现已立即生效。
- 修复长页面内容被裁切（内容区无法滚动）。
- 修复侧栏导航点击崩溃（循环引用 JSON 报错）。
- 修复 URL 参数序列化噪声：`?sort=%5B%5D` 一类无意义参数不再写入地址栏。

### Changed
- 文案术语统一（启用/停用、额度、签到、通道）；内部计划编号移出用户可见文案；移除公开设置页的私有品牌信息；9 处硬编码改为国际化。
- 视觉润色：登录页、Dashboard 统计卡、设置页移动端响应式与固定侧栏；全站渐变移除改为纯色。
- 开发工具默认隐藏（显式开启）；首屏渲染前恢复持久化外观设置。
- 移除更新中心前端入口（对应接口未实现）。
- 仓库治理：GitHub Flow 工作流（master 受保护、短命分支 PR squash 合并）、PR 模板与工作流文档；README（中/英）修正过时信息并移除私有仓链接。
- 发布流程整合：镜像推送成功后再创建 GitHub Release。

## [v0.9.0] — 2026-08-11

### Added
- 前端管理界面整体重写：全新单页应用（Bun + Rsbuild 2 + TanStack Router/Query + Tailwind 4 + Zustand + shadcn/Base UI），替换旧版前端。
- 新 Dashboard：概览 / 流量 / 模型 / 可用性 4 区块，图表展示与 WebSocket 实时状态。
- 通用数据表格：URL 状态同步（排序、分页可恢复）、移动端卡片降级、批量操作。
- 站点 / 账号 / Token 路由管理：引导式配置流程（站点 → 账号 → 路由）+ 表单校验。
- 设置页重做：5 个子区导航（常规 / 下游 / 模型 / 内容 / 系统信息），移动端响应式分级下钻。
- 签到页：失败原因分类着色徽章 + 手动签到。
- 代理日志页：数据表格（服务端分页）+ 详情抽屉（会话路径、计费信息）。
- 模型页 + 模型测试器：品牌图标、SSE 流式响应（OpenAI / Claude / Responses / Gemini 协议）。
- 关于 / OAuth / 站点公告：完整管理功能。
- 国际化：key-based 双语（en / zh-CN）。
- 品牌名统一 **MetAPI**；透明 SVG 徽标与 favicon 替换旧版 PNG。
- 顶栏语言切换（en / zh-CN），自动跟随浏览器语言，页面 lang/dir 同步。
- 主题定制面板：配色预设 / 字体 / 圆角 / 缩放 4 轴，可单独重置；全部预设默认使用无衬线字体。
- 侧栏导航完整本地化。

### Fixed
- 修复签到失败原因分类不生效：后端新增失败原因字段（SQLite / PostgreSQL 兼容），API 嵌套返回，前端分类着色恢复正常。
- 修复嵌入式前端白屏：SPA 回退曾把 `/static/*` 静态资源当作 HTML 返回；现已正确挂载静态资源并设置缓存头。
- 修复表格排序/分页与 URL 不同步：同路径 search 导航后表格滞后，现已立即生效。
- 修复长页面内容被裁切（内容区无法滚动）。
- 修复侧栏导航点击崩溃（循环引用 JSON 报错）。
- 修复 URL 参数序列化噪声：`?sort=%5B%5D` 一类无意义参数不再写入地址栏。

### Changed
- 前端构建迁移：npm → Bun（本地开发与 Docker 构建一致）。
- 文案术语统一（启用/停用、额度、签到、通道）；内部计划编号移出用户可见文案；9 处硬编码文本改为国际化（含移除公开设置页的私有品牌信息）。
- 视觉润色：登录页、Dashboard 统计卡（骨架屏、状态图标、连接指示）、设置页移动端体验；全站渐变改为纯色。
- 开发工具默认隐藏，需显式开启。
- 移除旧版前端页面与组件代码。
- 移除更新中心前端入口（对应接口未实现）。
- 仓库治理与社区文档：GitHub Flow 工作流（master 受保护、PR squash 合并）、PR 模板、贡献指南 / 安全说明 / 行为准则 / issue 模板；README（中/英）修正过时信息并移除私有仓链接。
