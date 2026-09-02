# log.md — Metapi Go product milestones

**Last updated**: 2026-09-03 (v0.17.0)

> Product milestone timeline: the current month is kept per release, closed months are one-line summaries
> (date · title · versions/PRs) so the file stays scannable. Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../../CHANGELOG.md)

## 2026-09-03 — v0.17.0 切版（数据面与存储诚实性波：转发链身份 · 凭据回显 · 表清单注册表 · 上游编码 · 可取消等待）

- **版本判断**：按 patch-first 节奏本可以是 v0.16.24，但这一波改了三个运维可见契约（恢复出厂的清库范围、账号写响应的凭据字段、流式失败的记账口径）外加一处身份解析语义（转发链），够 `docs/internal/git-workflow.md` §6.1 的「成体系的交付波」，故 bump 中间位并把最后一位归零。收口 #1159–#1170 共 12 个 PR。
- **转发链身份（#1161）**：`TRUSTED_PROXY_CIDRS` 生效时客户端 IP 取的是 `X-Forwarded-For` 最左值，而主流反代是追加语义 ⇒ 最左那段是调用方自己塞的。三个消费者全部读改写后的 `RemoteAddr`：admin IP 白名单一个伪造头即过（#1034 的纵深防御归零）、每 IP 限流可每请求换一个假 IP 无限换桶（登录暴破上限失效且限流器自身看不出异常）、`admin_audit_logs.remote_ip` 与会话 IP 绑定记下的是编造的地址。改为「所有转发头按序拼链 + 补直接 peer + 从右往左跳过可信 CIDR + 返回第一个不可信地址」，全链可信时返回最左（否则内网调用方塌缩进同一个限流桶）；外层「peer 必须可信」这道门与 `RemoteAddr` 形状未动，默认空配置行为不变——这正是它长期没被发现的原因。
- **表清单注册表（#1165）**：仓库同时存在三份手抄表清单（`AutoMigrate` 建表序 20 项、`cmd/migrate` 拷贝集 28 项、恢复出厂删除序 28 项），而 schema 有 37 张表。后果是实测的：方言迁移**静默丢 17 张表**（命令正常退出、checksum 全对）；恢复出厂**漏 9 张**，其中 `admin_sessions` 意味着重置前签发的每个 admin cookie 仍对一个空库有写权限（会话校验每请求读该表，清空它才是真的吊销）。收敛为单一注册表 `store/tablesets.go`，五个用途（建表序 / 拷贝集 / FK-safe 清空序 / 恢复出厂集 / 逐表列规格）全部派生；顺带修掉序列同步的硬编码排除、未拷源列的静默丢弃（现在逐表 Warning）、CLI 迁移缺的 `normalize` 腿、boolean 默认值文本在 fresh 与 converged schema 之间的不一致。`sc2_029` 从注册表步骤改为每启动无条件幂等清扫——journal 门控在「数据后到」的场景下必然错。
- **凭据回显（#1163）**：读路径早就把 `accessToken`/`apiToken` 换成掩码并从 `extraConfig` 剥掉 `autoRelogin.passwordCipher`，而 `PUT /api/accounts/{id}` 与 `POST /api/accounts/{id}/rebind-session` 原样返回明文 ⇒ 一次 `{"sortOrder":7}` 空操作更新就能读出整库凭据，那层脱敏只值一次空操作。两个写响应改走与列表同一份策略（导出为 `service.RedactAccountSecrets`，账号面凭据策略的唯一所有者）。发放面刻意保留回显（login 返回刚换来的会话令牌、verify-token 返回在上游发现的 API token——调用方本来没有那个密钥），规则写进 `docs/api/accounts.md`。
- **数据面诚实性（#1159、#1168）**：流式路径此前只区分「idle 超时」与「其它一律正常结束」，内容级判定只写日志不参与记账 ⇒ 上游中途断连、被 `PROXY_MAX_STREAM_RESPONSE_BYTES` 截断、或返回命中 `PROXY_ERROR_KEYWORDS` 的错误体时，渠道健康度 / `proxy_logs` / 终端指标全部按成功记账；现在结束原因分五类、状态与 reason 与指标 outcome 同源，内容级判定收口成一个纯函数（buffered 与 streaming 喂同一份事实），客户端主动断开仍按成功记账但不与干净 EOF 混为一类。#1168 补上编码这一层：站点 `custom_headers` 里的一个 `Accept-Encoding` 就能关掉 net/http 的透明解压，于是用量记 0（漏账）、关键字扫压缩噪声（假成功）、SSE 分析器读不到 `data:` 事件 ⇒ 开了 `PROXY_EMPTY_CONTENT_FAIL` 时健康的 200 流被记成 502 空内容失败（假失败，毒化渠道健康度）。现在 `gzip`/`deflate` 解码后再判定与计费（零新依赖），解不了的（`br`/`zstd`/多层栈/解码失败）原样转发 + 用量记 `unknown` + 判官直接 pass + 稳定文案 WARN；该头在装配侧与出站各剥一次，两份过滤清单的安全关键交集由跨包门禁钉住。
- **并发与关机（#1169、#1170）**：四处夹在两次上游调用之间的等待不看 ctx（关机或预算耗尽后仍睡完并照发下一次调用），实测三个 OAuth onboard 轮询 28.3s / 25.2s / 8.1s → 0.16s，签到退避 2.21s → 0.47s；调度器三条路径与手动触发端点都已接上（手动触发跑在请求 ctx 上，已处理账号各自留 `checkin_logs` 行）。共享计数器的补偿回滚挂在限流拒绝与失败补偿两条热路径上，此前每次上行整段 Lua；改为 `EVALSHA` 优先、仅 `NOSCRIPT` 回退一次 `EVAL`（不保存需要失效处理的「服务器已有脚本」状态），ACL 禁 scripting 的 `NOPERM` 从「回滚失败」改为既有 `INCRBY` 降级 + 一次点明权限的 WARN。
- **工程面（#1160、#1162、#1164、#1166、#1167）**：对外文档成为可对账参考（env parity + 路由清单两个自证伪门禁）；smoke 的 409 兜底改按后端实际去重键 `(platform, url)` 回查（此前按脚本自己的 `SITE_NAME`，站点名不同就七步连锁失败）；`internal/pgtest.Reset` 让复用同一个 PG 库的门禁可重复（长期噪音「`handler/admin` 5 个用例第二轮假失败」消除，`-p 1` 要求写进文档）；`checkin_interval_hours` 从「范围检查即静默丢弃」改成与 env 同形的双侧钳制，并删掉从未有读取点的 `HOME_PAGE_CONTENT`（撤回一条文档承诺）；`GET /api/channels` 的快照缓存从单槽改成有界多键（此前分页与仪表盘视图互相逐出，10s TTL 形同不存在，命中率≈0）。
- **测试床复验（合并后 master 重建二进制，commit `cebb374c`）**：new-api 链 13 PASS / 1 WARN / 0 FAIL（**故意不传 `SITE_NAME`**，让 #1162 的 `(platform,url)` 兜底在活体上被走到）· sub2api 链 11 PASS / 2 WARN / 0 FAIL · axonhub 10/10 · gpt-load 13/13 · F5 双语视觉 e2e 13/13 · 五服务全 RUNNING。复验顺带抓出两条运维可见缺陷（进波次 5）：verify-token 失败时服务端零日志、`channel selection failed err=<nil>`。
- **负结果（别再查）**：`handler/admin/accounts.go` 的 `globalAccountsCache` 虽然也是单槽形状，但 `get()` 不带 key、分页路径直接绕过快照缓存 ⇒ 键空间真的是 1，不存在 channels 那种互相逐出缺陷。

## 2026-09-03 — v0.16.23 发版（持久化正确性波：迁移方言 · 共享准入原子性 · 设置往返 · 前端缓存与文案）

- **发布事实**：v0.16.23 于 2026-09-03 publish 为 repo Latest（12 资产：5 平台 server + migrate 二进制、`checksums.txt`、`install.sh`）；patch-first，距 v0.16.22 约一天，收口 #1151–#1156 六个 PR。tag 管道全绿（含 `test-pg` 与 docker-build/push），发布产物已抽样校验：`metapi-linux-amd64` 的 sha256 与 `checksums.txt` 一致、`--version` 报 `v0.16.23`。来源是同日晚些时候的第二轮四路只读侦察（配置与运行时、代理数据面、store 与迁移、web 状态与 i18n，各 12 findings）加主线修复；本轮消化其中的 P1 簇。
- **迁移方言（#1153）**：注册表里三个 BOOLEAN 列的数字默认值让**既有 PostgreSQL 库启动即失败**（`ADD COLUMN … BOOLEAN DEFAULT 0` → SQLSTATE 42804，`AutoMigrate` 首错即中止），而全新安装永远走不到该路径（`CREATE TABLE` 已带列 → `EnsureColumn` no-op），所以 CI 的全新库看不见它。修法两端收口：`EnsureColumn` 内归一（BOOLEAN 列的 `DEFAULT 0/1` → `FALSE/TRUE`，保留尾随修饰符）+ 注册表三处改写；配注册表源码门禁与两个 PG-gated 探针（按老库形状建表后重放真实注册表步骤）。同类 42804 在 v0.8.49+ 修过一批，这次把口子收在 primitive 上。
- **共享准入原子性（#1154）**：补偿回滚是 `DECR` + 独立 `DEL` 两个往返，**另一个实例在两者之间落地的 `INCR` 会连同旧键一起被删掉** → 共享窗口静默少算 → 该下游密钥被放行超过配置的 RPM/TPM 上限，且无任何日志。合并为单条 `EVAL`（Redis 按单命令执行脚本）；服务器若缺少脚本能力则降级为「只减量、不自愈」。由 CI `test-pg` 的并发压力测试偶然暴露（`mixed total = 478, want 480`）——它不是 flake；补两条确定性交织测试与一条「必须恰好一条服务器命令」的机制测试，三者对旧实现全部为红。
- **设置往返（#1156）**：写侧持久化 33 个键、读侧只有部分有水合分支，实测 27 个键缺失（最贵的一条：`admin_ip_allowlist` 重启后**控制面 IP 限制静默消失**）；三个凭据键写侧 JSON 编码、读侧只 `TrimSpace` → **通过管理 API 轮换过的令牌重启即失效**；`log_cleanup` 点号/下划线命名空间分裂使整块水合成为死码；日志清理体制判定不再由「`retention > 0`」推断（`config.Load` 把下限钉在 1 天，于是 settings 表有任意一行就触发，静默把显式 `false` 翻成 `true` 并禁用 legacy pruner），改为只认显式意图且两来源单向不对称（DB 显式键 OR env toggle 为 true），启动输出一条 `settings: log retention regime` 说明胜者；branding 显式空串生效、cron 键校验后接受、两处被丢弃的持久化错误改 500。门禁：AST 驱动的「写侧键集 − 水合键集 − 白名单 = ∅」（含匹配器反例自证与负向对照）、`config.Load` 与水合的钳制逐字段相等（20 子用例）、真 `PUT /api/settings/runtime` → 模拟重启 → `reflect.DeepEqual`。
- **前端缓存与文案（#1155）**：导入站点后 `/accounts` 不刷新（失效打在快照、页面读分页缓存，同一个向导在 `/sites` 却会刷新）；三个行内 toggle 的乐观更新打在页面不读的键上（点击后不即时翻转，并把只用于取名字的快照改成猜测值、污染四页）；两个**间接引用**的 i18n 键双语缺失使界面渲染裸 key（模型测试 401/403、账号模型面板的业务失败兜底），门禁扫描面因此从「同行 `t('字面量')`」扩到 `assertBusinessOk` 的 fallback 实参与全仓键形状字面量。
- **运行时卫生（#1151、#1152）**：回滚自愈非正数 Redis 键（该自愈随后由 #1154 原子化）、`store.GetDB()` 改原子指针（每条请求路径不再争全局互斥锁）、`USAGE_PROJECTION_INTERVAL_MS` 可调（钳制 1000–3600000，默认不变）；密钥准入贯穿请求上下文（客户端已断开的请求不再耗完计数器超时并阻塞同密钥的其它请求，取消走既有 fail-open 路径，单实例语义零漂移）。
- **参考项目拆解**：本地测试床在 new-api / sub2api / axonhub 之外新增 **gpt-load v2.0.0-rc.4**（官方 release 二进制、sha256 与官方清单一致、13 步验证脚本全绿），产出后端与前端两份拆解：可借鉴项含流式终止语义分级、失败判定单一所有者、配置面 fail-loud 与生效值回显、计费回执与票据后结算、提示前缀软亲和、前端幂等写与结果分类；明确不抄的含无条件回显内部分组头、控制面全局写锁 + 双次全量重编译、遍布全栈的单实例假设。
- **本轮新实证缺陷（进波次 4）**：`scripts/e2e/smoke.sh` 的 409 兜底按名字回查、与后端 (platform, url) 去重不一致（裸跑七步连锁失败）；`handler/admin` 5 个 PG 用例在复用库上第二轮假失败（状态累积，CI 的一次性库看不出）；`PUT /api/accounts/{id}` 响应回显明文 `accessToken`，与列表接口的脱敏策略不一致。

## 2026-09-02 — v0.16.22 发版（`/v1` 数据面契约与硬化 + 并发去串行化 + 旅程闭环）

- **发布事实（同日二次发布）**：v0.16.22 于 2026-09-02 publish 为 repo Latest（12 资产）；v0.16.21 于同日早些时候 publish（两版间隔约 4 小时，patch-first 节奏）。tag 管道的本地 pre-push `-race` 门禁在 2C 开发节点超包预算，按该节点既定纪律以 `--no-verify` 推 tag——该提交的 GitHub CI 已全绿（含 -race 分片），决策与理由记录在维护者工作区日志。

- **API 契约（#1145）**：写超时倒挂收口（`WriteTimeout` 60s 覆盖上游等待，61~90s 返回的缓冲响应被掐写端）——`proxy.RequestCeiling`/`WriteBudget` 成为执行器与写预算的单一来源，`router.ProxyWriteDeadline` 在两个代理组重新武装写截止；客户端协议头填充式白名单透传（`anthropic-version`/`anthropic-beta`/`openai-beta`/`user-agent`/`x-stainless-*`，站点 `custom_headers` 与 token 优先级不变，凭据头永不透传，Anthropic 原生派发补协议默认版本）；上游响应头由全量拷贝改内容语义白名单（厂商指纹头/`Set-Cookie`/上游 `X-Request-Id` 不再泄漏，metapi request id 与限流头保持权威）。
- **安全纵深（#1145）**：数据面 transport（执行器、SSE 流、兜底派发客户端、渠道健康探针）挂 `internal/ssrf` DNS 钉扎 dial 守卫（新 `httpclient.Options.SiteDialGuard`），关掉校验后重解析窗口；metadata/link-local/multicast 拒绝，loopback/RFC1918/ULA 保留给自托管上游。
- **性能并发（#1147）**：Redis 共享准入去全局串行化——64 条 cache-line 分片锁（不跨 I/O）+ 每密钥 `sharedMu` 保 `Incr`/补偿 `Decr` 有序 + 连接池（上限 8、`AUTH`/`SELECT` 每连接一次、重连、尊重 ctx）。语义零漂移（fail-open/拒绝原因/`Retry-After`/决策形状/key 命名空间/env 名全不变）。2ms 共享往返 32 并发多密钥：117 → 3137 次/秒（26.8×），p50 259ms → 10ms；单次 `INCR` 465µs → 54µs，`AUTH` 3530 → 1。
- **无界增长（#1146）**：视频任务重写缓存加 TTL + 摊销 sweep + 20000 行硬护栏（缓存永不删持久行）；`PROXY_VIDEO_TASK_RETENTION_DAYS` 默认 0 → 7（`<=0` 仍为显式关闭）。
- **卫生**：死字段 `SurfConfig.ExtraHeaders` 删除；`anthropic-version` 默认值三副本归一 `platform.ClaudeDefaultAnthropicVersion`；`docs/api/proxy.md` 补 /v1 头策略与超时相态表、`docs/api/conventions.md` 补数据面 SSRF 硬化节。
- **发布事实**：v0.16.21 于本日 publish（repo Latest，12 资产）；被完全取代且从未发布的 v0.16.20 draft 删除（tag 与 CHANGELOG 节保留）。

## 2026-09-02 — v0.16.21 发版（审计驱动修复波：安全/API 契约/TS 接管/死码清理）

- **安全（#1139）**：三条导入路径（站点批量、原生备份、TS v2.1 备份）SSRF 目标校验缺口收口——`SanitizeImportedSiteRows` 统一守卫 + preview 一致性 + 丢弃行显式上报。
- **API 契约（#1140、#1141）**：`/v1` 鉴权/限流错误对齐 OpenAI 信封（invalid key 403→401、配额 403→429）；`/v1/pricing` 双前缀路由 bug、catalogsync PG `LastInsertId` bug、SSE 默认 1MB→64MB、`X-Accel-Buffering: no`、审计 LIKE 大小写 parity。
- **TS 接管（#1142）**：`sc2_029` 时间戳归一化——drizzle 空格格式自动重写 RFC3339，消除接管库排序/范围失真与首启重签风暴（introspection 驱动、双方言、幂等）。
- **卫生（#1137、#1138）**：Go 死码 -1814 行（14 项零引用验证）+ clamp 三副本归一；web oneoff -1900 行 + `copyText()` 收敛。
- **来源**：五路并行域侦察（安全 / 性能并发 / 数据库迁移 / 产品 UIUX / API 网络）+ Go/web 死码清查；当日合并 8 PR（#1135–#1142）。该轮记录的剩余 backlog（Redis 准入锁、写超时倒挂、头透传/剥离、术语统一批、旅程闭环、计费视图、告警中心等）由 v0.16.22 起逐波消化，进度见 [`STATE.md`](STATE.md) 的 Current focus。

## 2026-09-01 — v0.16.20 发版（F5 结构化事件 + 08-30→09-01 三波）

- **F5 批次 1（checkin 事件族）**：events 表 `title_key`+`params` 结构化（`sc2_028` 增量迁移），程序日志页标题本地化 + 消息模板 i18next 插值，历史行原文 fallback；真实 new-api/sub2api 测试床端到端视觉验证 zh/en 13/13。
- **同版包含 #1084–#1127 三波**：S7 删除+undo 档、S10 插值 parity 门禁、事件标题 i18n 统一、errorCode 家族、auth 会话模型加固残留、账号筛选服务端化（#1108）与视觉 QA 修复波、CSP S2 nonce 化。



## 2026-09-01 — 账号筛选服务端化 + 视觉 QA 修复波（#1122–#1126）

- **#1122 账号页筛选服务端化（issue #1108）**：`GET /api/accounts` 新增 `q`/`status`/`site` 参数——筛选全舰队而非已加载页（此前站点不在当前页永远筛不出）；`total` 为筛选后计数，非法值显式 400，LIKE 通配符字面转义。Reset 竞态根因修复：`useUrlTableState` 改为串行化（同 tick 双更新链式合并，此前 TanStack coalesce 吞掉第一次更新），所有 URL 同步列表页的重置恢复完整。站点单元格快捷跳转（安全外链，共享 `SafeExternalLink`，同 #985 阶梯）。
- **#1123 文案校对**：路由策略「加数」→「基数」；侧边栏「管理员审计日志」→「审计日志」（7 字截断）。
- **#1124 运行事件标题折叠**：受影响路由/替代站点/面板深链折叠进每行「详情」开关，默认行高 5 行 → 3 行。
- **#1126 price-compare**：删组头重复「推荐」徽章（每组必有推荐行，组头重复稀释信号），行内徽章 + tooltip 保留语义。
- **视觉 QA 波**：全站 112 张截图审图（light/dark/desktop/mobile），评分 7/10——真问题 4 处全修，其余报告项逐一源码核实为识图噪声。


## 2026-08 — 月度汇总（55 条明细已收口）

> 每条一行：日期 · 原标题 · 出现的版本/PR 号。产品叙事见根 [`CHANGELOG.md`](../../CHANGELOG.md) 的对应版本节；`CHANGELOG.md` 未覆盖的更早版本（v0.8.x 及以前）见本文件的 git 历史。

- **2026-08-31** 前端视觉 QA 波 + auth 文案国际化启动 · PR #1101, #1102, #1103, #1104, #1109, #1118
- **2026-08-30** 事件标题 i18n 统一 + S7 收官 + S8 注册表合一 + F4 文档拆分 · PR #1084, #1085, #1086, #1087, #1088, #1091, #1092, #1093…
- **2026-08-29** 后端收口波 + C1 config 竞态 + 双线融合（#1063–#1080） · PR #1026, #1063, #1065, #1071, #1073, #1075, #1077, #1079…
- **2026-08-29** W21 CSP 去 unsafe-inline（#1035 S2） · PR #1035
- **2026-08-29** W20 凭证维度后端加固（#1026 残留） · PR #1026
- **2026-08-29** 前端审计三波（本地 W19-W21）
- **2026-08-29** Wave 18 多线并行 发布 v0.16.19 · v0.16.19 · PR #1052, #1053, #1054, #1055, #1056, #1057, #1058, #1059…
- **2026-08-28** Wave 17 竞品研究 P1×4 发布 v0.16.18 · v0.16.18 · PR #1024, #1036, #1039, #1040, #1045, #1046, #1047, #1048…
- **2026-08-28** Wave 16 竞品研究 P0×3 发布 v0.16.17 · v0.16.17 · PR #1018, #1019, #1020, #1022, #1023
- **2026-08-28** Wave 15 issue 收口 发布 v0.16.16 · v0.16.16 · PR #1005, #1009
- **2026-08-27** Wave 14 账号表单与分页修复 发布 v0.16.15 · v0.16.15 · PR #1007, #1008
- **2026-08-27** Wave 13 token sync UI 真值 发布 v0.16.14 · v0.16.14
- **2026-08-25** Wave 12B account bootstrap 发布 v0.16.13 · v0.16.13 · PR #998, #1002
- **2026-08-25** Wave 12A demand truth 发布 v0.16.12 · v0.16.12 · PR #996, #999, #1000, #1001
- **2026-08-25** Wave 11 UX 真值波发布 v0.16.11 · v0.16.11 · PR #862, #1102
- **2026-08-25** Wave 10 Sites demand batch 发布 v0.16.10 · v0.16.10 · PR #985, #986, #991, #992
- **2026-08-24** Wave 10 #981 迁移兼容修复 · PR #981
- **2026-08-24** 计费货币整理（USD 收口）
- **2026-08-24** Wave 9 发布 v0.16.9 · v0.16.9
- **2026-08-24** Wave 9 冻结恢复 + 集成
- **2026-08-23** Wave 8 收官 + Wave 9 冻结交接 · PR #971
- **2026-08-23** Wave 7 收官：合并 master + 生产滚动部署（未发版） · PR #970
- **2026-08-23** Wave 7 前端体验波启动
- **2026-08-23** Wave 5+6 → v0.16.8 · v0.16.8 · PR #935, #936, #937, #938, #939, #940, #941, #942…
- **2026-08-22** Wave 4 综合质量波 → v0.16.7 · v0.16.7 · PR #933
- **2026-08-21** Round 3 修复波 → v0.16.6 · v0.16.6 · PR #910, #911, #912
- **2026-08-21** #887 补遗收口 + E2E Journey 3 → v0.16.5 · v0.16.5 · PR #887
- **2026-08-21** UI/UX Round 2 收口（v0.16.4） · v0.16.4 · PR #901
- **2026-08-21** UI/UX Round 1 #887 → v0.16.3 · v0.16.3 · PR #887, #890, #896, #897
- **2026-08-20** TS 兼容与迁移收官 · PR #881, #882, #883, #884
- **2026-08-20** 仓库店头重建（#872 #873） · PR #872, #873
- **2026-08-20** 工程与测试基线加固（#866 #868 #869 #870 #871） · PR #866, #868, #869, #870, #871
- **2026-08-19** Real-e2e UIUX audit: self-hosted brand icons + audit tooling
- **2026-08-19** Deep backend testing: 6 defect fixes
- **2026-08-19** v0.17 onboarding polish + per-key usage + positioning honesty
- **2026-08-18** a11y 残差核实：axe 全绿 + 菜单 Esc 行为钉死
- **2026-08-18** 复审驱动：token-routes 死列 + 列表/详情打磨 · PR #853, #854, #855
- **2026-08-18** 复审驱动：availability WS 重连 + sites 表单数据丢失修复 · PR #850, #851
- **2026-08-18** 复审驱动：onboarding 闭环 + model-tester 清晰度 + availability 可视化 · PR #838, #840, #842, #846, #847
- **2026-08-18** 复审驱动：动线 dead-end 收口 + proxy-logs 过滤服务端化 · PR #828, #832, #835
- **2026-08-18** UI/UX 批次：账户行内操作 + header SSOT + skeleton shimmer
- **2026-08-17** v0.15.x Resin per-site + 弹窗视口合约 + 设计系统溢出安全 · v0.15.0 / v0.15.1 / v0.15.2 / v0.15.3
- **2026-08-17** 产品品牌升级（Metapi 改名 + logo + 登录 UI）
- **2026-08-17** v0.14.0 发布收口 · v0.14.0 · PR #800, #801, #802
- **2026-08-16** 路由成本真值 + 恢复可靠性
- **2026-08-15** 真实平台测试战役：测试床 + 6 个实测 bug 修复 + CI e2e · v0.13.0
- **2026-08-14** Leader/Worker 并行 fan-out：5 分支合入 · PR #657, #662
- **2026-08-14** 多 Agent UI/UX 对照审计 + 分发端 P0
- **2026-08-14** 产品对标 + 文档卫生（neat-freak）
- **2026-08-14** v0.12.0 架构简化（净删 ~21K 行） · v0.12.0 · PR #650, #654
- **2026-08-14** v0.11.0 管理控制台全量 + 开源打磨 · v0.11.0
- **2026-08-12** v0.10.0 设置中心 + 调度规格 v1 · v0.10.0
- **2026-08-11** post-v0.9.0 UI completion batch · v0.9.0
- **2026-08-11** v0.9.0 frontend rewrite · v0.9.0
- **2026-08-02** v0.8.46 → v0.8.52 · v0.8.46 / v0.8.52 / v0.8.49


## 2026-07 — 月度汇总（4 条明细已收口）

> 每条一行：日期 · 原标题 · 出现的版本/PR 号。产品叙事见根 [`CHANGELOG.md`](../../CHANGELOG.md) 的对应版本节；`CHANGELOG.md` 未覆盖的更早版本（v0.8.x 及以前）见本文件的 git 历史。

- **2026-07-31** Feature batch
- **2026-07-30** Engineering optimization + parity review
- **2026-07-20** v0.8.45 RE2-safe · v0.8.45
- **2026-07-19** UI polish milestone

