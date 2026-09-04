# Changelog

Metapi-Go 的版本叙事。格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

打 tag 时 CI 把 `## [vX.Y.Z]` 段逐字抽成该版本的 GitHub release body。**那是快照，不是引用**：tag 之后再改这里，已发布版本的发布页不跟着变。

写作契约：

- 读者是**部署和调用 Metapi 的人**。一条只回答两件事：行为怎么变了、我要不要动手。
- 一条 = 一行结论（带符号／端点／环境变量／表名）+ 至多一句因果。取证过程、测试与门禁的形状、行数增减、内部文件清单不进本文件——它们在 commit body 与 PR 里，`git log --grep '#1210'` 一步到达。
- **要运维动手的必须显式写**：升级后重新登录、默认值变了、备份不再含某张表、端点删了、请求要多带一个头、日志新增 WARN。这是本文件唯一不可省略的部分。
- 编号写 `#问题 → #PR`；只有一个编号时它就是 PR。
- 分节固定 `安全` `修复` `变更` `移除` `开发者可见` `文档` `已知遗留`，空节不写；`已知遗留` 只收**运维可见的残留**（配了不生效、旧数据留一行、会无界增长），有 issue 的开放项看 GitHub issues。v0.16.23 及更早沿用当年的 `Added` / `Changed` / `Fixed`。
- **v0.16.17 及更早逐字保留**；v0.16.18–v0.19.0 于 2026-09-05 按本契约做过第二次压缩（事实、编号与运维动作未变），这几版的发布页仍是压缩前长文。

## [v0.19.0] — 2026-09-04

> 核心链（站点 → 账号 → 长期凭据 → 模型 → 路由 → 可用通道 → 下游密钥 → `/v1/models` → `/v1/chat/completions`）做成可重复、可解释、可恢复的契约（#1215）。**无新增产品面。**

### 修复

- **balance 刷新失败给出分类原因（#1210 → #1211）**：单个与批量两个出口都保留稳定前缀并追加分类后缀（凭据过期／上游拒绝／上游不可达／上游没返回余额），如 `balance refresh failed: upstream rejected the credential (HTTP 401)`。只渲染既有错误枚举，上游 URL、token 与原文不进响应（原文进 WARN）。同批：`service/balance` 的 DB 读失败不再被当成「账号不存在」回 404。
- **PostgreSQL 备份导入后重置 id 序列（#1217 → #1218）**：导入带显式 id 而 serial 序列停在原位，恢复后的第一次写入撞 `duplicate key value violates unique constraint "sites_pkey"`。migrator 与导入共用 `store.ResyncPGIDSequences`（migrator 警告续跑，导入判致命）。SQLite 不受影响。
- **`model_probe_results` 有了保留期（#1221 → #1222）**：此前没有任何 DELETE 路径。现由既有 `RetentionScheduler` 按 7 天窗口每小时清理，豁免每个 `(account_id, model_name)` 的最新一行（路由重建读的就是它）。无新配置项。
- **账号详情抽屉逐条报出通道真正使用的凭据来源（#1219 → #1220）**：不再用一个笼统状态掩盖；下游密钥的空 model policy 明确显示为 deny-all。
- **架构边界门扫不到文件时不再判通过（#1214）**：任一域解析到 0 个生产 Go 文件即失败；skip 列表补上 agent worktree 与 gitignored 私有目录。

### 变更

- **`scripts/e2e/verify-token-import.sh` 兑现「可重跑」（#1209 → #1212）**：账号已存在时用本轮已验证的同一枚凭据 `PUT /api/accounts/{id}` 收敛（不二次签发），site / account / key / route 各自打印 `created` / `reused` / `refreshed`。

### 开发者可见

- **核心链四态门进 CI `test-e2e`（#1213、#1216、#1212）**：Fresh 之外新增 Restart（同一 data dir 重启，不重新登录／不重建）、Aged（老化存量凭据后复跑）、Restore（备份导入全新 data dir 后直接中继），三态都要求真实 completion 成功。前端验收补上尾段（#1220）：UI 签发下游密钥 → 独立 HTTP 客户端（无页面、无 admin token）真调 `/v1/chat/completions`，并按 `EXPECT_SERVER_COMMIT` 对 `/api/about` 做身份 preflight。

### 已知遗留

- `admin_audit_logs` 与 `checkin_logs` 仍无保留期，随运行无界增长（清审计历史是需要运营点头的产品决定）。

## [v0.18.0] — 2026-09-03

> 减法版本：删掉从未发布或从未接线的面，同时根修 New API 主链上三个各自足以致死的缺陷。**升级后需要重新登录一次账号；令牌不需要手工绑定。**

### 修复

- **New API 中继链从「看起来绑上」变成持久（#1179 → #1187）**：① 登录返回的约 15 分钟 dashboard JWT 曾被当长期凭据落库，过期后模型刷新塌成空列表、路由无通道、`503 No available channels`——现在登录时提升为 New API 长期 dashboard PAT，**发不出持久凭据就拒绝绑定**，并用 live JWT 撤销临时上游会话。② 令牌列表的掩码显示值曾被当真实 relay key 落库（请求 401、通道被停用）——现在经所有权校验的 `POST /api/token/batch/keys` hydrate，hydrate 不出来即报错，掩码行在 UI 上保持 `masked_pending`。③ 模型列表此前要等后台调度（默认凌晨 4 点）——现在登录后立即同步，route rebuild 可以服务触发它的那次请求。
- **`503 No available channels` 说清为什么（#1179 → #1186）**：`proxy.ExplainNoChannel` 给一行结论加主候选拒绝原因（无启用路由匹配该模型／通道令牌未绑定或停用／全部冷却或下游密钥策略排除站点），同时进 503 body。
- **重建路由不再随发出它的 HTTP 请求一起死（#1174 → #1185）**：`POST /api/routes/rebuild` 曾整趟跑在 `r.Context()` 上，`refreshModels: true` 时客户端 30s 先挂断、日志只剩 `context canceled`；现改用 `context.WithoutCancel` + 30 分钟预算。`POST /api/settings/maintenance/clear-cache` 也不再删掉重建要用的输入（清业务行是 `factory-reset` 的职责）。
- **账号编辑保存的 `credentialMode` 不再蒸发（#1176 → #1184）**：更新 payload 缺该字段、handler 静默丢弃 ⇒ 改成 session、拿到成功 toast、重开又是 API key。现在进 payload 并合并进 `extraConfig`，且在碰任何凭据字段之前解析（此前切 session 也会把 session 凭据抄进 `api_token`，被凭据回落当 API key 发出）。撑不起的模式回 400。
- **AnyRouter 校验失败说明它要什么凭据（#1133 → #1195）**：`token verification failed` → 明确指引：access-token / API-key 绑定不支持、必须用 session 模式、字段接受 `session=<value>` 或完整 `Cookie:` 头。bind 与 verify 共用 `credentialVerificationFailureMessage(platform)`。
- **「同步站点令牌」幂等（#1193 → #1196）**：上游掩码 relay key 时行解析成 `masked_pending`、永远匹配不上 ready 行，每次同步都 INSERT 一份副本；现按精确 key 值匹配、优先 ready 行、回落 masked 行，UPDATE 保留 `value_status`。
- **删表不再让删除之前写的备份恢复不了（#1201）**：导入曾拒绝任何不认识的 payload key ⇒ 有效旧备份变 `400 unknown table proxy_debug_traces`。现按来源区分：真导出文件里的未知 key ＝本 build 已删的表，跳过并在 import 与 preview 响应里报 `ignoredTables`；策略排除表与手写 JSON 的未知 key 仍 400。

### 移除

- **未发布的 Electron 桌面外壳（#1197）**：连同公开、未认证、只为托盘而设的 `GET /api/desktop/health`。
- **永久为空或无人实现的控制面（#1199、#1201）**：proxy debug-trace 子系统（两张表、两个读路由、九个 `PROXY_DEBUG_*` 开关及其设置 UI）· `proxy_files` 表 + `PROXY_FILE_RETENTION_*` + 其调度器 · `admin_snapshots` 表 + 其调度器（`RunProjectionPass` 仍由 usage-aggregation 调度器跑）· update-center 五个操作臂（`POST /api/update-center/check`、`PUT /config`、`POST /deploy`、`POST /rollback`、`GET /tasks/{id}/stream`）+ `METAPI_ENABLE_UPDATE_CENTER`（只读状态卡保留）· model-tester 流式/任务臂（`/api/test/proxy`、`/api/test/chat/stream` 及各自 jobs 端点；同步探针 `POST /api/test/chat` 保留）。
- **无实现者与无调用者的代码（#1187、#1190、#1198、#1201、#1204）**：`RouteRefreshWorkflow` 家族 · `RESPONSES_COMPACT_FALLBACK_TO_RESPONSES_ENABLED` · `routing` 的死 pricing override 链（`NormalizePricingRatio`、`EstimateProxyCostFromModel`、`BuildPricingOverrideModel`）· 六个零引用 helper · 四个不可能失败的测试。
- **16 份维护者过程史移出公开仓（#1191、#1194）**：`docs/internal/` 下的 STATE / log / MASTER / benchmark 与 11 份 analysis 及设计稿；角色改由 GitHub issues 与 releases、本文件、`docs/architecture.md` 承接。

### 开发者可见

- **`handler/admin/accounts.go` 按关注点拆成 6 个同包文件（#1192）**；同形测试折成表格（#1188、#1189、#1200、#1202、#1203、#1206），用例集合机械比对、差集为空、子测试名保留。
- **CI 的 `bun install` 对瞬时故障有界重试（#1181）**：损坏或截断的 tarball 报 `Integrity check failed`，重跑就装得上，却能挂掉必选检查。同时修 Dockerfile 缓存挂载路径（`~/.bun/install-cache` → `~/.bun/install/cache`，此前那句声明从未缓存过任何东西）。
- **仓库卫生（#1194）**：`.gitignore` 补 `.env.*`（保留 `!.env.example`）、`*.log`、`/dist-bin/`；CI 缓存 key 从哈希一个不存在的 lockfile 改为哈希真实存在的那个。

### 已知遗留

- `PayloadRules` 无运行时消费者、`OpenAiServiceTierRules` 无读者，而 `docs/configuration.md` 仍把两者写成可用：**配了不生效**。
- 本版之前绑定的账号可能留一条 `masked_pending` 旧令牌行：一次性、不可路由，运营可删。
## [v0.17.1] — 2026-09-03

### 修复

- **备份导出不再静默丢 5 张用户可见状态表（#1172）**：`service/backup.AllTables` 是仓库里第三份手抄清单，漂移到 37 张里的 28 张 ⇒ `GET /api/settings/backup/export?type=all` 每次静默丢弃 `product_announcements`（产品横幅消失）、`announcement_dismissals`（已读公告重新弹出）、`model_name_redirects`（`source=manual` 行永不重生成）、`balance_history`（余额趋势从恢复日起空白）、`model_verify_history`（批量校验历史消失），而导入端回 200 报成功。
- **备份文件自己陈述自己的缺口（#1172）**：导出 payload 的 `metadata` 新增 `excluded_tables`（表名 → 理由）；既有形状（`exported_at` / `version` / `type` / `tables`）与两条导入路径不变，WebDAV 往返与 TS v2.1 兼容层不受影响。**导入点名要一张被排除的表现在回 400**（此前静默忽略）；旧备份缺表＝跳过而不是错误。
- **4 张表显式排除，理由进 metadata 与文档（#1172）**：`admin_sessions`（导入 token hash 会让源站签发的 cookie 在本站生效；**恢复后要求重新登录**）· `admin_audit_logs`（源站写操作的 append-only 轨迹）· `model_probe_results`（探针每 tick 重建，路由只取每 `(account, model)` 的最新一行）· `catalog_sources`（每行 `url` 由目录同步服务端抓取，而导入 URL 闸当时只覆盖 `sites` 与 `site_api_endpoints`）。
- **更正上一版文档给的一条不存在的退路（#1172）**：`docs/api/settings.md` 曾写「需要就先导出备份」以保留审计与探测历史，而备份从来不含这两张表；现明确说明要用数据库工具自己 dump。
- **库里一条读不出来的设置行不再清空已配置的值（#1173）**：`payload_rules` / `openai_service_tier_rules` / `checkin_schedule_mode` / `notify_task_toggles` 的水合分支把解析失败（解析器用 `nil` 编码）当值赋上去 ⇒ 一条坏行在下次重启时清空已配好的规则集且日志无字。现在坏行丢弃并告警；JSON `null` 与 `[]` 仍是可读的清空意图，照常生效。**启动日志因此可能新增 WARN 行。**
- **每次启动都打的那条假 WARN 没有了（#1178）**：`setupSPAFallback` 还挂着 Vite 时代的 `/assets/*`，而内嵌 dist 自迁移起就没有该目录 ⇒ 挂载永不可达而守卫报警。同批立门禁：`index.html` 里每条 root-relative `src` / `href` 必须回 200 **且不得以 `text/html` 回来**（SPA fallback 对未挂载路径也答 `200 text/html`，正是把白屏藏在正常状态码后面的方式）。

### 开发者可见

- **CI 必选分片此前跑 0 个测试仍全绿（#1175）**：`test-sqlite-shard` 的分片算术写错，错误又恰好落在 `set -e` 管不到的条件里 ⇒ 约三十个 tag 的 SQLite 测试与 `-race` 实际未执行；现在断言本片应得的包数——选少了和选不到一样红（不变量见 `docs/testing.md`）。e2e 备份用例的「导出应有多少张表」改从注册表取值（那曾是第四份手抄清单）。

### 已知遗留

- `notify_task_toggles` 的 admin 写路径应当对错误形状返 400（本版只让它不再毁掉 runtime）。
- `POST /api/accounts/verify-token` 失败时只回 400、服务端零日志。

## [v0.17.0] — 2026-09-03

### 安全

- **转发链客户端 IP 改为从右往左解析（#1161）**：配置 `TRUSTED_PROXY_CIDRS` 时不再取 `X-Forwarded-For` 最左值（那一段由调用方自己塞），而是把所有转发头按序拼成链、补上直接 peer，再从最右往左跳过可信 CIDR，返回第一个不可信地址。修掉三个后果：admin IP 白名单可被一个伪造头绕过、每 IP 限流可无限换桶、审计日志记的是攻击者自选 IP。
- **账号写路径不再回显明文凭据（#1163）**：`PUT /api/accounts/{id}` 与 `POST /api/accounts/{id}/rebind-session` 此前原样返回 `accessToken`、`apiToken` 与 `extraConfig.autoRelogin.passwordCipher` ⇒ 一次 `{"sortOrder": 7}` 空操作更新就能读出整库凭据；两个写响应现在与列表共用 `service.RedactAccountSecrets`。**凭据发放面刻意保留回显**（`POST /api/accounts/login`、`POST /api/accounts/verify-token`）；取回**已存**密钥仍是显式动作（`GET /api/account-tokens/{id}/value`、下游密钥导出）。规则写进 `docs/api/accounts.md`。

### 修复

- **流式请求的中断、截断与空内容不再记成功（#1159）**：此前只区分「idle 超时」与「其它一律正常结束」，内容级判定只写日志不记账 ⇒ 上游中途断连、被 `PROXY_MAX_STREAM_RESPONSE_BYTES` 截断、或返回命中 `PROXY_ERROR_KEYWORDS` 的错误体时，渠道健康度、`proxy_logs` 与终端指标全按成功记账。现在流结束原因分五类（正常／idle 超时／上游故障／被截断／客户端断开），状态、原因与 outcome 由同一个所有者决定。
- **恢复出厂设置真的恢复到出厂（#1165）**：`POST /api/settings/maintenance/factory-reset` 此前遍历一份比 schema 少 9 张表的手抄清单，其中包含 `admin_sessions` ⇒ **重置前签发的每个 admin cookie 仍对一个空库有写权限**。现表清单从 schema 注册表派生，唯一排除项是 additive 迁移日志 `schema_migrations`。**重置成功后必须重新登录。**
- **`cmd/migrate` 不再静默丢表（#1165）**：方言迁移（SQLite ↔ PostgreSQL）按另一份手抄清单只拷了 37 张表里的 20 张——命令正常退出、checksum 全匹配，17 张表的数据没被搬运。现在拷贝集、清空序、逐表列规格与建表序全部从同一个注册表派生；源库存在而 Go schema 未声明的列在拷贝前逐表打 Warning。
- **`checkin_interval_hours` 的越界值不再被静默丢弃（#1166）**：库内该键此前「范围检查不通过就不赋值、不告警、还算已处理」，而 `CHECKIN_INTERVAL_HOURS` 走双侧钳制 ⇒ 库里存 30、进程用环境变量的值、`GET /api/settings/runtime` 回显第三个值。
- **`GET /api/channels` 的快照缓存此前几乎从不命中（#1167）**：缓存只有一个槽位而键空间是多键的（无界视图 + 每个分页/状态筛选各一键），`page=1` 与 `page=2` 互相逐出，那条 fleet-wide 5-way JOIN 照跑。现改有界多键缓存（至多 16 键、FIFO 淘汰）；TTL、`?refresh=true`、`x-channels-snapshot-cache`、并发去重与失效语义未变。

### 变更

- **站点 `custom_headers` 里的 `Accept-Encoding` 不再可配（#1168）**：它关掉 net/http 的透明解压，答案以压缩字节进入解析 ⇒ 用量提取找不到 token（真实调用计零费）、`PROXY_ERROR_KEYWORDS` 扫压缩噪声（该失败的没失败）、流式分析器读不到 `data:` 事件（开 `PROXY_EMPTY_CONTENT_FAIL` 时健康 200 流被记成 502「空内容」）。现该头在装配侧被过滤，`gzip` / `deflate` 先解码再判定与计费，重构后的响应不再带 `Content-Encoding`。
- **解不了的编码诚实上报，不猜（#1168）**：`br`、`zstd`、多层编码栈或解码失败时，响应体连同它自己的 `Content-Encoding` 原样转发（客户端仍能解），不解析、用量记为显式 `unknown`，关键字与空内容规则不在没人读过的字节上开火；每次这样处理打一条稳定文案的 WARN。文档同步 `docs/configuration.md` 与 `docs/api/proxy.md`。
- **夹在两次上游调用之间的等待现在可以被取消（#1169）**：签到 transient 重试退避、同站点节奏等待与三个 OAuth onboard 轮询此前不看 context ⇒ 关机或每账号预算耗尽后 worker 仍睡完，并把睡完之后的那次上游调用照发。等待时长语义未动，只加可取消性；`POST /api/checkin/trigger` 是刻意例外（跑在请求 context 上）。
- **Redis 共享计数器的补偿回滚不再每次上行脚本体（#1170）**：回滚挂在限流拒绝与失败补偿两条热路径上。现在先 `EVALSHA`，仅在服务器答 `NOSCRIPT`（重启、`SCRIPT FLUSH`、切副本）时回退一次 `EVAL`；原子语义（减量 + 非正数即删键自愈）不变，ACL 禁 scripting 的 Redis（`NOPERM`）降级为 `INCRBY`。
- **文档成为可对账的参考（#1160）**：环境变量清单 ↔ 代码读取点、路由清单 ↔ router 注册逐条对账，由门禁守着（漂移即 CI 红）；`.env.example`、`docs/configuration.md`、`docs/api/**` 与实现不一致处按实现纠正。

### 移除

- **`HOME_PAGE_CONTENT`（#1166）**：`.env.example` 与 `docs/configuration.md` 不再列这个键——它在 Go 实现里从来没有读取点，数据库侧的孪生键与前端字段早已下线。设置了该变量的部署不会报错，但它本来就什么都没做（`SYSTEM_NAME` / `LOGO` / `FOOTER` / `ABOUT` 不受影响）。

### 开发者可见

- **PostgreSQL 门禁套件在复用同一个库时可重复运行（#1164）· `scripts/e2e/smoke.sh` 的站点解析与后端去重键一致（#1162）**：新增 `internal/pgtest.Reset`，全套 PG 门禁必须以 `-count=1 -tags=integration -p 1` 运行（共享库），已写进文档与 CI；`smoke.sh` 遇 `POST /api/sites` 409 时改按后端实际去重的 `(platform, url)` 找回已有站点，而不是按脚本自己的 `SITE_NAME`（此前站点名不同就让后续 login / account / models / balance / checkin 全线跳过并报失败）。

## [v0.16.23] — 2026-09-03

### Added

- **`USAGE_PROJECTION_INTERVAL_MS`（#1151）**：用量聚合的投影节奏（`proxy_logs` → 站点/模型汇总）可调，钳制 1000–3600000 ms，默认仍 5s。
- **启动日志报出日志清理体制归属（#1156）**：一条 `settings: log retention regime` 给出 `regime`（`log_cleanup` / `legacy_fallback`）、`configured`、`source`（`db_settings` / `env_toggle` / `none`）、两个 toggle 与保留天数。

### Changed

- **`KeyAdmissionLimiter.Allow()` 贯穿请求上下文（#1152）· `store.GetDB()` 改原子指针（#1151）**：客户端已断开的请求不再耗完计数器超时、也不再占着该密钥的串行互斥锁把同密钥的其它请求排在后面；请求路径不再争一把全局库互斥锁（打开/迁移/切换仍串行）。

### Fixed

- **老 PostgreSQL 库启动即失败（#1153）**：三个 BOOLEAN 列写了数字默认值（`sites.use_system_proxy`、`sites.post_refresh_probe_enabled`、`model_availability.is_manual`）⇒ 增量迁移 `ALTER TABLE … ADD COLUMN … BOOLEAN DEFAULT 0` 报 42804（`column … is of type boolean but default expression is of type integer`）。**全新库永远复现不了**，只有升上来的库会走到那一步。
- **管理界面保存的运行时设置重启后静默失效（#1156）**：写侧入库、读侧没有水合分支——33 个键里 27 个重启后不生效，本版补齐；6 个进带理由的白名单（`db_type` / `db_url` / `db_ssl` 在水合前就被 bootstrap 消费，三个 `*_schedule_v2` 由迁移服务与排班端点自己读）。最贵的一条是 `admin_ip_allowlist`：**重启后控制面板的 IP 限制静默消失**。同族：三个凭据键（`auth_token` / `proxy_token` / `account_credential_secret`）写侧 JSON 编码、读侧只 `TrimSpace` ⇒ 轮换过的令牌重启后带引号进快照、常量时间比较必然失配；日志清理设置水合侧读 `log_cleanup.enabled` 而写侧只写 `log_cleanup_enabled` ⇒ 整块死码（统一到写侧拼写，点号留只读别名）；日志清理体制（新 cleanup 调度器还是 legacy `PROXY_LOG_RETENTION_DAYS` pruner 拥有日志表）曾被 settings 表里任意一行静默开启，顶掉显式配置的 `LOG_CLEANUP_CRON` / `LOG_CLEANUP_RETENTION_DAYS`；`system_name` / `logo` / `footer` / `about` / `server_address` 丢弃空值 ⇒ 清空的品牌信息重启后复活（凭据键的空值守卫保留）；`admin_ip_allowlist` 与 `proxy_error_keywords` 写库失败此前返回 200，**现返回 500**。
- **共享准入的补偿回滚丢弃并发预占（#1154）**：Redis 回滚从两个往返（先减、非正再 `DEL`）改为单脚本原子完成——窗口期内别人预占的计数不再被一起删掉。
- **界面批（#1155）**：导入站点后账号列表不刷新、行内开关不即时翻转（mutation 失效快照而表格读分页缓存 ⇒ 同一动作在 `/sites` 与 `/accounts` 两套答案，现改为失效查询工厂根 + 按分页前缀批量 patch）；三个 i18n 键在两个语言包里都不存在，界面渲染出裸键（`modelTester.error.sessionExpired`、`accounts.models.refreshFailed` / `manualFailed`）。

## [v0.16.22] — 2026-09-02

### Added

- **三个配额字段的语义（#1148）**：`maxRequests` / `maxCost` / `expiresAt` 是累计总量而非每分钟窗口（每分钟限速是另一组 `max_rpm` / `max_tpm`）、金额单位 USD 且仅成功请求计费、超限 429（`over_requests` / `over_cost`）、过期 403（`key_expired`）、留空或 0 ＝不限制。
- **界面批（#1148）**：仪表盘四步旅程清单（站点 → 账号 → 路由 → 密钥，CTA 只挂在第一个缺口、四步建成后自我退役）；已有路由但下游密钥数为 0 时就地提示怎么签发第一把密钥并用它调用 `/v1`，签发后永久消失。

### Changed

- **`/v1` 与上游之间的头策略（#1145）**：上行透传客户端协议开关 `anthropic-version`、`anthropic-beta`、`openai-beta`、`user-agent`、`x-stainless-*`（此前只带 `Content-Type` + 站点 `custom_headers` + 账号 token，Claude Code 的 prompt caching 与 fine-grained tool streaming 到不了上游）；下行改内容语义白名单，厂商指纹头（实测可见 `X-New-Api-Version`）与上游 `Set-Cookie` 不再泄漏，上游同名头也不再覆盖 metapi 自己的 `X-Request-Id`。
- **`/v1` 写超时不再掐断慢响应（#1145）**：`Server.WriteTimeout`（60s）在请求头读完时即武装、因此也覆盖等待上游的时间，而缓冲派发上限是 `max(90s, 首字节窗口 ×2)` ⇒ 61~90s 之间返回的非流式响应在回写时被掐断（「拿到结果却连接被重置」）。现由专用中间件持有代理写期限。
- **Redis 共享限流不再全局串行（#1147）**：配置 `REDIS_URL` 后此前是「全局一把锁 + 每命令一次 TCP 握手」；现改为 64 条 cache-line 对齐分片 + 连接复用 + 贯穿 ctx。**语义零漂移**：出错仍 fail-open 回落本地窗口，`over_rpm` / `over_tpm`、`Retry-After`、`UsedRPM` / `UsedTPM`、键命名空间与全部 env 名不变。
- **`PROXY_VIDEO_TASK_RETENTION_DAYS` 默认 0 → 7（#1146）**：`0` 等于保留期调度器整条禁用、`proxy_video_tasks` 无界增长；`<=0` 仍是运维显式关闭，只是不再是默认。

### Fixed

- **下游密钥的过期时间此前完全不生效（#1148）**：表单出站裸 `datetime-local` 串（形如 `2030-01-02T03:04`，无秒无时区），两处解析器都不认，而不可解析的过期时间会**跳过整个过期检查**——一把 2020 年就该过期的密钥仍能以 200 调 `/v1/models`（RFC3339 形式正确返回 403）。
- **数据面 SSRF dial 级纵深（#1145）**：转发 transport 曾是裸 `net.Dialer`，校验通过后重新解析（DNS rebinding）仍可落到不同地址；现执行器、SSE 流 transport、两个兜底派发客户端与渠道健康探针统一挂 DNS 钉扎守卫。
- **视频任务进程内缓存无界增长（#1146）**：`publicId → 上游视频 id`（含 sticky 渠道/账号 pin）的重写缓存曾是包级 map 且无驱逐；现按同一保留期旋钮做 TTL，驱逐在插入路径摊销触发，无后台 goroutine。

### Docs

- `docs/api/proxy.md` 新增「Header policy (/v1 surface)」与「Timeouts (/v1 surface)」；`docs/api/conventions.md` 新增「Site upstream SSRF hardening (proxy data plane)」。

## [v0.16.21] — 2026-09-02

### Added

- **接管库时间戳归一化（#1142）**：启动迁移把 TS 时代写在 TEXT `*_at` / `*_until` 列里的 `'YYYY-MM-DD HH:MM:SS'` 重写为 RFC3339 UTC——两种格式混排时字典序比较失真（空格 0x20 排在 `T` 0x54 前）：`ORDER BY created_at` 错排、范围过滤边界失真、checkin 清扫把全部行当成同一时刻。
- **`/v1` 错误契约文档（#1140）**：`docs/api/proxy.md` 新增「Error shape (/v1 surface)」（OpenAI 信封 + 完整 status/type/code 对照表）；admin 面（`/api/*`）平铺契约不变。

### Changed

- **`/v1` 鉴权与限流错误对齐 OpenAI 信封（#1140）**：中间件裸 `{"error":"..."}` 统一为 `{"error":{message,type,code,request_id}}`；invalid key `403` → `401 authentication_error/invalid_api_key`（SDK 把 403 视为不可重试的权限错误、跳过换 key 路径），配额耗尽 `403` → `429 insufficient_quota`。
- **`PROXY_MAX_STREAM_RESPONSE_BYTES` 默认 1MB → 64MB（#1141）**：推理模型长输出常态超 1MB，此前被中途截断且自定义错误事件多被 SDK 静默吞掉（表现为「回答戛然而止」）；上限保留为失控护栏。同批 SSE 响应补 `X-Accel-Buffering: no`，nginx 系反代不再缓冲首 token。
- **死码清扫（#1137、#1138）**：Go 侧 14 项零引用生产码删除（oauth import 族、quota `Record*` 族、routing `PricingReference` 与四端口接口、`ClearChannelFailureState` 级联等；`/api/oauth/import` 保持文档化 UI-parity 桩）；web 侧 `web/scripts/oneoff/` 整目录删除，5 处手写 clipboard 收敛为共享 `copyText()`。

### Fixed

- **SSRF 目标校验缺口（安全，#1139）**：站点批量导入（`POST /api/sites/import`）、原生备份导入、TS v2.1 备份导入三条路径绕过 `IsForbiddenSiteTargetURL` ⇒ 构造备份即可植入 `url=http://169.254.169.254/…` 的站点，配合下游密钥一次 `/v1` 请求回流云 metadata / IAM 内容。现三路径统一走同一个净化器。
- **PostgreSQL 与路由注册批（#1141）**：`GET /v1/pricing` 曾在 `/v1` 组内注册绝对路径 ⇒ 文档承诺的路径一直 404、真实可达的是 `/v1/v1/pricing`；`catalogsync.CreateSource` 在 PG 必失败（pgx stdlib 不实现 `LastInsertId`），补 `RETURNING id` 方言分支；审计日志 `path LIKE ?` 双侧 `LOWER()` 对齐（SQLite 不敏感、PG 敏感）。

## [v0.16.20] — 2026-09-01

### Added

- **运行事件结构化（checkin 事件族先行）**：`service/events` 类型化注册表（注册防重复、参数严格校验）；`WriteEvent` 在历史英文 title/message 之外新增 `title_key` + `params` JSON（notify / CSV / 历史行等非 UI 消费者字节级不变）。
- **界面与无障碍批（#1097、#1026、#1072、#1073、#1080、#1087）**：叶子实体单行删除改 6 秒「撤销」窗口（真实删除延后到窗口关闭）；下游密钥凭证树形选择器（站点 → 账号 → 密钥三级勾选 `allowedCredentialRefs` / `excludedCredentialRefs`）；Ctrl/⌘+K 可直接执行动作；恢复出厂设置改 type-to-confirm（输入 `RESET` + 倒计时）；六个主图全部有 sr-only 数据表；i18n 双语键的 `{{变量}}` 集合必须一致。

### Changed

- **管理端大表服务端分页与筛选（#1075、#1077、#1122，issue #1108）**：账号、模型市场、渠道列表改服务端分页，`GET /api/channels` 新增 `status` 过滤与 `GET /api/channels/error-summary`；`GET /api/accounts` 新增 `q` / `status` / `site`（此前分页在服务端、筛选却在客户端，站点不在当前页时永远筛不出来），`total` 是筛选后计数、非法值 400。
- **统一 400 错误体携带机器可读 `errorCode`（#1065）· 畸形凭证引用改显式 400（#1026）**：认证与会话 8 码、资源不存在 5 码、能力未实现/禁用 3 码、设置校验 1 码、加载失败族 1 码（覆盖 72 个调用点，见 `docs/api/conventions.md`）；`allowed/excludedCredentialRefs` 里的非对象、未知或缺失 `kind`、非正 `siteId` / `accountId`、`account_token` 缺 `tokenId` 不再被静默丢弃（对允许列表而言等于静默放宽策略）。
- **`docs/api.md` 按域拆分为 `docs/api/*.md`（17 个文件）**：`docs/api.md` 保留为索引并以 stub 承接原标题，旧 `api.md#<anchor>` 深链继续解析。
- **前端结构与界面形态批（#1099、#1084、#1095、#1082、#1123、#1124）**：事件标题 i18n 两份映射归一；三份 `createSectionRegistry` 克隆合一、8 个列表页手写的加载失败横幅 + 重试收进表格底座、站点表单改右侧抽屉、筛选与页码写入 URL、文案与行高校对。行为均不变。

### Fixed

- **下游密钥凭证维度端到端修复（#1026）**：`auth.ExcludedCredentialRef` 的 JSON 标签是 snake_case，而管理端持久化形状是 camelCase ⇒ 代理路径解析不出运营刚配好的排除项。
- **运行时设置的并发撕裂读（#1079）**：`RuntimeSettings` 迁移为不可变快照交换——此前约 25 个运行时写字段与热路径无锁读并存，并发下可能读到半更新状态（如代理 token 校验瞬时 401 抖动）。
- **前端错误与本地化批（#1091、#1080、#1086、#1088、#1101、#1102、#1104）**：铃铛弹层不再裸渲染后端为兼容保留的英文 `label`；错误 toast 去重与状态感知本地化（502–504 不再泄漏 axios 英文原文）；列表加载失败改整块替换 + 内置重试；浅色主题对比度达 WCAG AA；landmark / `alt` / 空表头 / 重复日期 / 移动端头部 / 不可用态 delta 等修正；channels 页「响应延迟」列回到首屏。

### Security

- **SPA CSP 去 `style-src 'unsafe-inline'`**：收紧为 `'self' 'nonce-<per-request>' 'sha256-<toast-css>'`，其余 directive 不变；Go SPA fallback 每响应生成 16 字节随机 nonce 并注入 `<meta name="csp-nonce">`。
- **凭证导出遮罩改真占位符（#1080）**：不再以 CSS blur 伪掩码渲染（明文此前仍留在 DOM 与无障碍树中，读屏可读出），改为 `••••••••` 占位符 + `aria-hidden`。

## [v0.16.19] — 2026-08-29

### Security

- **管理员会话模型重构（#1034、#1057）**：主 token 不再以明文存于浏览器 localStorage（原 12h 窗口）——登录改为 `POST /api/auth/login` 以主 token 换取服务端会话（`admin_sessions` 表），凭证经 HttpOnly + SameSite=Strict 的 `metapi_session` cookie 携带，库里只存 SHA-256 哈希，会话滑动续期。实时运维 WS 不再接受 `?token=<主 token>`，改由 `POST /api/auth/ws-ticket` 签发 60s 单次 ticket——主 token 从此不进 URL。
- **失败认证纳入限速 + 敏感操作主 token 重确认（#1034、#1057）**：per-IP 限速中间件移至认证之前（401/403 不再绕过桶约束），`/api/auth/*` 另加严格桶。备份导出（下载与 WebDAV）、下游密钥导出、主 token 轮换要求 `X-Admin-Confirm-Token` 头重出示主 token（即使持有活跃会话），否则 403 `reauthRequired`；凭证导出对话框默认锁定遮罩、密钥默认遮罩显示、深链打开前显式确认。**这些操作从此需要多带一个头，自动化脚本要跟着改。**

### Added

- **会话配置项（#1034、#1057）**：`ADMIN_SESSION_TTL_MINUTES`（默认 720）、`ADMIN_SESSION_COOKIE_SECURE`（默认 auto，按请求协议自适应）、`AUTH_RATE_LIMIT_RPS` / `AUTH_RATE_LIMIT_BURST`（默认 10/20），零配置即安全；`cmd/migrate` 携带 `admin_sessions`，会话跨 SQLite ↔ PostgreSQL 切换存活。
- **上游账号健康监测全局开关（#1027、#1056）· 账号代理支持 SOCKS5（#1009、#1059）**：运行时设置 `checkinEnabled` / `balanceRefreshEnabled`（热生效、持久化）+ 环境变量 `CHECKIN_ENABLED` / `BALANCE_REFRESH_ENABLED`，模型可用性探测改运行时热停；账号 `proxyUrl` 接受 `socks5://` 与 `socks5h://`。

### Fixed

- **清空账号代理字段保存即清除（#1009、#1059）**：空值显式清除（前端清空后载荷省略 + 后端 `MergeExtraConfig` typed-nil 双因）；账号更新路径补 scheme 校验。
- **调度器与并发批（#1061、#1052）· PostgreSQL 方言陷阱清扫（#1060）**：job panic 统一 recover、in-flight 标志锁外读竞态修复、`channel_recovery` / `backup_webdav` / `model_probe` 吞没的 DB 错误显性化、路由健康快速路径无锁读改 `RLock`；BOOLEAN 列整数字面量绑定重写（42804 / 22P02 类）、管理搜索 LIKE 双侧 `LOWER()`、静态方言门禁扩至全包（零迁移）。

### Changed

- **管理读路径索引（#1054）· 出站 HTTP 客户端基线（#1058）· 构建配置与 UX 批（#1053、#1055）**：三个高频读路径加索引（300k 行实测最贵一条 17.9ms → 0.5ms），proxy-logs `LEFT` → `INNER`、marketplace N+1 批量化；15 处出站调用点统一 `internal/httpclient`（dial 30s / TLS 10s / idle 90s / 池 100-20）并由 AST 门禁拦截裸客户端；`rsbuild.config.ts` 成为唯一构建配置（删 `vite.config.ts`）、匿名大块拆为 `vendor-i18n` / `vendor-icons` / `vendor-core`；focus-ring 统一 token（≥3:1）、流量与成本图补 sr-only 摘要表。

## [v0.16.18] — 2026-08-29

### Added

- **`PROXY_STREAM_IDLE_TIMEOUT_SEC`（默认 300，#1046）**：每转发一个 chunk 重置计时窗，窗口内无新 chunk 即中断卡死的流并按上游超时故障记录（渠道健康、失败日志、终态一致）。只约束 chunk 间隔、不约束流总时长；`0`、负数、非法值回退默认。
- **`proxyRetryStatusRanges` / `proxyDisableStatusRanges`（#1049）**：运行时设置（设置页可编辑，`PUT /api/settings/runtime` 同契约）——retry 决定哪些上游状态码计为可重试的渠道故障，disable 决定哪些状态码在冷却升级之外直接禁用故障渠道（`enabled=false` + 人工覆盖标记）。
- **`PUT /api/channels/batch` 部分更新（#1047）**：只写字段出现的、载荷校验（空批 / 超 1000 / 重复 id / 无可更新字段）、逐项真值返回；模型测试台批量对比出现失败行时提供「禁用失败渠道」动作（确认对话框列明数量与人工覆盖后果）。
- **界面批（#1048、#1026、#1039、#1036）**：渠道页失败横幅 +「只看失败」经可分享的 `?status=` 进入仅失败视图（人工停用不计入）；密钥表单新增「上游站点限制」多选（留空 = 不限制，完整往返既有 `allowedSiteIds`）；CmdK 面板命中一键深链到实体本身；zh-CN 语言下站点表崩溃修复（`createdAt` 列对无效 Intl locale 的防护回退）。

### Fixed

- **路由自动重建真值（#1024）**：`POST /api/routes/rebuild` 现在消费 `refreshModels`（默认 true：先批量刷新全部活跃账号的上游模型、单账号失败不中断整批，再重建通道）；完成提示读取真实统计并区分三态——成功（真实路由/通道数）、无路由可重建（引导先创建路由）、通道无变化（引导核对账号模型）。

### Security

- **CSP 收紧（#1041）**：内联 bootstrap 脚本外部化，移除 `unsafe-inline`。
- **前端安全快赢批（#1037）**。

### Changed

- **前端体检快赢批**：构建（#1040）、可访问性（#1042、#1043）、卫生（#1044）、UX（#1045）。

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
