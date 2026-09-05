# Changelog

Metapi-Go 的版本叙事。格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

打 tag 时 CI 把 `## [vX.Y.Z]` 段逐字抽成该版本的 GitHub release body。**那是快照，不是引用**：tag 之后再改这里，已发布版本的发布页不跟着变。

**写作契约**（由 `docs/doc_hygiene_test.go` 机械守着，不靠自觉）

- 读者是部署和调用 Metapi 的人。一条只回答两件事：行为怎么变了、我要不要动手。
- 条目形状 `- <主体>：<事实>（#问题 → #PR）`。主体是标识符（端点、环境变量、表名、脚本）或最短名词短语，**不是修辞标题**；粗体只留给运维必须看见的动作，不当语气强调用。
- **必须写进来**：端点路径、环境变量名与默认值变化、表名与列名、HTTP 状态码变化、新增日志级别、升级后要做的动作。这是本文件唯一不可省略的部分。
- **不许写进来**：为什么以前是错的、内部 Go 符号与文件名、测试与门禁的形状、行数与计数。它们在 commit body 与 PR 里，`git log --grep '#1210'` 一步到达。
- 一条 ≤ 170 字符，**这是上限不是配额**。超了只有两条出路：拆条，或指向该事实的 owner 文档——环境变量清单、错误码表、端点清单各有 SSOT 与门禁，本文件指路，不抄第二份。
- 同族合并：同一根因或同一 PR 的多条并成一条。`开发者可见` 每版至多**一节**（门数的是节，不是条）。
- 分节固定 `安全` `修复` `变更` `移除` `开发者可见` `文档` `已知遗留`，空节不写；v0.16.23–v0.16.13 沿用当年的 `Added` / `Changed` / `Fixed` / `Security` / `Docs`，归档区不分节。`已知遗留` 只收**运维可见的残留**（配了不生效、旧数据留一行、会无界增长），有 issue 的开放项看 GitHub issues。

**两个分区**

- **治理区**（v0.16.13 起）：全条目受上述契约与门禁约束。
- **归档区**（v0.16.12 及更早）：每版只留安全修复、破坏性变更与运维动作。这些版本的全文逐字保存在各自的 [release 页](https://github.com/DeliciousBuding/metapi-go/releases)——已逐版核对，发布页与归档前的本文件段落字节一致——UI 与交互级修复因此不在这里抄第二份。

## [v0.19.0] — 2026-09-04

> 核心链（站点 → 账号 → 长期凭据 → 模型 → 路由 → 可用通道 → 下游密钥 → `/v1/models` → `/v1/chat/completions`）做成可重复、可解释、可恢复的契约（#1215）。无新增产品面。

### 修复

- balance 刷新失败返回分类原因，如 `balance refresh failed: upstream rejected the credential (HTTP 401)`；上游 URL、token 与原文不进 body（原文进 WARN）；余额读库失败不再回 404（#1210 → #1211）。
- PostgreSQL 备份导入后重置 id 序列，恢复后的首次写入不再撞 `sites_pkey` 唯一约束；SQLite 不受影响（#1217 → #1218）。
- `model_probe_results` 有了保留期：7 天窗口、每小时清理，豁免每个 `(account_id, model_name)` 的最新一行（路由重建读它）。无新配置项（#1221 → #1222）。
- 账号详情抽屉逐条报出每个通道实际使用的凭据来源，不再合并成一个笼统状态；下游密钥的空 model policy 显示为 deny-all（#1219 → #1220）。

### 变更

- `scripts/e2e/verify-token-import.sh` 可重跑：账号已存在时用本轮同一枚已验证凭据走 `PUT /api/accounts/{id}` 收敛，不二次签发（#1209 → #1212）。
- 同脚本对 site / account / key / route 逐个打印 `created` / `reused` / `refreshed`（#1212）。

### 开发者可见

- 核心链 CI 门由 Fresh 一态扩为四态：Restart（同一 data dir 重启）、Aged（老化存量凭据）、Restore（备份导入全新 data dir），三态都要求真实 completion 成功（#1213、#1216、#1212）。
- 前端验收补上尾段：UI 签发密钥后由独立 HTTP 客户端（无页面、无 admin token）真调 `/v1/chat/completions`，并按 `EXPECT_SERVER_COMMIT` 对 `/api/about` 做身份 preflight（#1220）。
- 架构边界门扫不到文件时判失败，不再判通过（#1214）。

### 已知遗留

- `admin_audit_logs` 与 `checkin_logs` 无保留期，随运行无界增长；清审计历史是需要运营点头的产品决定，本版未做。

## [v0.18.0] — 2026-09-03

> 减法版本：删掉从未发布或从未接线的面，同时根修 New API 主链。**升级后重新登录一次账号；令牌由账号同步，不需要手工绑定。**

### 修复

- New API 登录凭据持久化：约 15 分钟的登录 JWT 提升为长期 PAT，**发不出持久凭据就拒绝绑定**，并撤销临时上游会话（#1179 → #1187）。
- 掩码显示值不再当 relay key 落库，改经 `POST /api/token/batch/keys` hydrate，hydrate 不出即报错（UI 状态 `masked_pending`）；登录后立即同步模型，不等后台调度（#1179 → #1187）。
- `503 No available channels` 带主候选拒绝原因（无启用路由匹配该模型／通道令牌未绑定或停用／全部冷却或下游密钥策略排除站点），同时进 503 body（#1179 → #1186）。
- `POST /api/routes/rebuild` 脱离请求 context、改 30 分钟独立预算，客户端挂断不再连带取消重建（#1174 → #1185）。
- `POST /api/settings/maintenance/clear-cache` 不再删掉路由重建所需的输入；清业务行是 `factory-reset` 的职责（#1185）。
- 账号编辑的 `credentialMode` 进更新 payload 且在碰任何凭据字段之前解析：切 session 不再把 session 凭据抄进 `api_token`，撑不起的模式回 400（#1176 → #1184）。
- AnyRouter 校验失败改报所需凭据形状：必须 session 模式，字段接受 `session=<value>` 或完整 `Cookie:` 头（原文案只有 `token verification failed`）（#1133 → #1195）。
- 删表后旧备份仍可导入：真导出文件里属于本 build 已删表的未知 key 跳过，import 与 preview 响应报 `ignoredTables`；策略排除表与手写 JSON 的未知 key 仍 400（#1201）。
- 「同步站点令牌」幂等：按精确 key 值匹配已有行并 UPDATE（保留 `value_status`），上游只给掩码 relay key 时不再每次 INSERT 一份副本（#1193 → #1196）。

### 移除

- 未发布的 Electron 桌面外壳，连同公开、未认证、只为托盘而设的 `GET /api/desktop/health`（#1197）。
- proxy debug-trace 子系统：`proxy_debug_traces` / `proxy_debug_attempts` 两表、其读路由、`PROXY_DEBUG_*` 开关族与设置 UI（#1199、#1201）。
- 同批移除 `proxy_files`、`admin_snapshots` 两表及其调度器与 `PROXY_FILE_RETENTION_*`（#1201）。
- update-center 只留只读状态卡：移除 `POST /api/update-center/check`、`PUT /config`、`POST /deploy`、`POST /rollback`、`GET /tasks/{id}/stream`（#1201）。
- 同批移除 `METAPI_ENABLE_UPDATE_CENTER`（#1201）。
- model-tester 的流式与任务臂：`/api/test/proxy`、`/api/test/chat/stream` 及各自 jobs 端点；同步探针 `POST /api/test/chat` 保留（#1201）。
- 无实现者与无调用者的代码：只被测试 mock 养活的路由刷新接口家族、`RESPONSES_COMPACT_FALLBACK_TO_RESPONSES_ENABLED`、死 pricing override 链与一批零引用 helper（#1187、#1190、#1198、#1201、#1204）。
- 16 份维护者过程史移出公开仓；角色由 GitHub issues 与 releases、本文件、`docs/architecture.md` 承接（#1191、#1194）。

### 开发者可见

- 账号 handler 按关注点拆为同包文件族（#1192）；同形测试折成表驱动（#1188、#1189、#1200、#1202、#1203、#1206），用例集合机械比对、差集为空、子测试名保留。
- 前端依赖安装对瞬时故障有界重试：损坏 tarball 报 `Integrity check failed`、重跑即好，却能挂掉必选检查（#1181）。
- `.gitignore` 补 `.env.*`（保留 `!.env.example`）、`*.log`、`/dist-bin/`（#1194）。

### 已知遗留

- 设置键 `payload_rules` 与 `openai_service_tier_rules` 无运行时消费者：**配了不生效**（设置页可写、重启后水合，但没有代码读它做决策）。
- 本版移除的四张表不发 DROP TABLE：老库保留 `proxy_debug_traces` / `proxy_debug_attempts` / `proxy_files` / `admin_snapshots` 孤儿表，无读无写，运营可自行 drop（#1201）。
- 本版之前绑定的账号可能留一条 `masked_pending` 旧令牌行：一次性、不可路由，运营可删。

## [v0.17.1] — 2026-09-03

### 修复

- `/api/settings/backup/export?type=all` 补回五张状态表：`product_announcements`、`announcement_dismissals`、`balance_history`、`model_name_redirects`、`model_verify_history`（#1172）。
- 备份导出 `metadata` 新增 `excluded_tables`（表名 → 理由）；payload 形状、两条导入路径、WebDAV 往返与 TS v2.1 兼容层不变（#1172）。
- 备份导入点名要一张被排除的表：**现在回 400**（此前静默忽略）；旧备份缺表＝跳过。被排除的表逐张列在 `metadata.excluded_tables`，其中 `admin_sessions` 意味着**恢复后需重新登录**（#1172）。
- 设置表里读不出来的行不再清空已配置的值：`payload_rules` / `openai_service_tier_rules` / `checkin_schedule_mode` / `notify_task_toggles` 改为保留旧值。**启动日志可能新增 WARN 行**（#1173）。
- 启动时那条固定出现的假 WARN 消失：SPA fallback 挂着一个内嵌 dist 里不存在的目录挂载点（#1178）。

### 开发者可见

- CI 必选分片 `test-sqlite-shard` 曾跑 0 个测试仍全绿：分片算术错误落在 `set -e` 管不到的条件里，约三十个 tag 的 SQLite 测试与 `-race` 实际未执行；现断言本片应得的包数，选少了和选不到一样红（#1175）。

### 文档

- `docs/api/settings.md` 更正一条不存在的退路：曾写「需要就先导出备份」以保留审计与探测历史，而备份从来不含这两张表（#1172）。

### 已知遗留

- `notify_task_toggles` 的 admin 写路径应当对错误形状返 400（本版只让它不再毁掉 runtime）。
- `POST /api/accounts/verify-token` 失败时只回 400、服务端零日志。

## [v0.17.0] — 2026-09-03

### 安全

- 配置 `TRUSTED_PROXY_CIDRS` 时，转发链客户端 IP 改为从右往左解析 `X-Forwarded-For`（最左段由调用方自填）。修掉三个后果：admin IP 白名单可被一个伪造头绕过、每 IP 限流可无限换桶、审计日志记攻击者自选 IP（#1161）。
- `PUT /api/accounts/{id}` 与 `POST /api/accounts/{id}/rebind-session` 不再回显 `accessToken`、`apiToken` 与自动重登密码密文：此前一次空操作更新就能读出整库凭据（#1163）。
- 凭据发放端点（`POST /api/accounts/login`、`POST /api/accounts/verify-token`）仍回显明文；取回已存密钥仍是显式动作（`GET /api/account-tokens/{id}/value`）。规则见 `docs/api/accounts.md`（#1163）。

### 修复

- `POST /api/settings/maintenance/factory-reset` 的清空表集改为从 schema 注册表派生，仅排除 `schema_migrations`；旧手抄清单少 9 张表且含 `admin_sessions`，**重置后必须重新登录**（#1165）。
- `cmd/migrate` 的方言迁移（SQLite ↔ PostgreSQL）拷贝集与建表序改为从 schema 注册表派生：此前只拷 37 张表里的 20 张，而命令正常退出、checksum 全匹配（#1165）。
- 流式请求的中断、截断与空内容不再记成功：上游中途断连、被 `PROXY_MAX_STREAM_RESPONSE_BYTES` 截断、或返回命中 `PROXY_ERROR_KEYWORDS` 的错误体时，渠道健康度、`proxy_logs` 与终端指标此前全按成功记账（#1159）。
- `checkin_interval_hours` 的越界库值不再被静默丢弃：此前库存 30、进程用 `CHECKIN_INTERVAL_HOURS` 的值、`GET /api/settings/runtime` 回显第三个值（#1166）。
- `GET /api/channels` 的快照缓存改为有界多键（单槽位下 `page=1` 与 `page=2` 互相逐出、几乎从不命中）；TTL、`?refresh=true` 与 `x-channels-snapshot-cache` 语义未变（#1167）。

### 变更

- 站点 `custom_headers` 里的 `Accept-Encoding` 不再可配，装配侧过滤该头，`gzip` / `deflate` 先解码再判定与计费。配了它的部署此前用量提取找不到 token，真实调用计零费（#1168）。
- 解不了的编码不猜：`br`、`zstd`、多层编码栈或解码失败时原样转发（客户端仍能解），用量记为显式 `unknown`，`PROXY_ERROR_KEYWORDS` 与 `PROXY_EMPTY_CONTENT_FAIL` 不在未读过的字节上开火，并打一条稳定文案 WARN（#1168）。
- 两次上游调用之间的等待可被 context 取消：签到重试退避、同站点节奏等待与三个 OAuth onboard 轮询此前不看 context，关机后 worker 仍睡完并发出下一次上游调用。`POST /api/checkin/trigger` 是刻意例外（#1169）。
- Redis 共享计数器的补偿回滚先 `EVALSHA`，仅在服务器答 `NOSCRIPT` 时回退一次 `EVAL`；ACL 禁 scripting 的 Redis 降级为 `INCRBY`（#1170）。

### 移除

- `HOME_PAGE_CONTENT`：Go 实现里从来没有读取点，配了不报错也不做任何事；文档条目一并移除。`SYSTEM_NAME` / `LOGO` / `FOOTER` / `ABOUT` 不受影响（#1166）。

### 开发者可见

- 文档成为可对账的参考：环境变量清单 ↔ 代码读取点、路由清单 ↔ router 注册逐条对账，漂移即 CI 红（#1160）。
- PostgreSQL 门禁可在复用同一个库时重复运行，须 `-count=1 -tags=integration -p 1`（#1164）。
- `scripts/e2e/smoke.sh` 遇 `POST /api/sites` 409 时按后端真实去重键 `(platform, url)` 找回站点（#1162）。

## [v0.16.23] — 2026-09-03

### Added

- `USAGE_PROJECTION_INTERVAL_MS`（默认 5s，钳制 1000–3600000 ms）：用量聚合的投影节奏（`proxy_logs` → 站点/模型汇总）可调（#1151）。
- 启动日志新增一行 `settings: log retention regime`，给出 `regime`（`log_cleanup` / `legacy_fallback`）、`configured`、`source`（`db_settings` / `env_toggle` / `none`）与保留天数（#1156）。

### Fixed

- 老 PostgreSQL 库启动即失败已修：`sites.use_system_proxy`、`sites.post_refresh_probe_enabled`、`model_availability.is_manual` 三列写了数字默认值，增量迁移报 42804。**只影响升上来的库，全新库不复现**（#1153）。
- 管理界面保存的运行时设置重启后生效：33 个键里 27 个此前没有水合分支，本版补齐；6 个进带理由的白名单（`db_type` / `db_url` / `db_ssl` 在水合前就被消费，三个 `*_schedule_v2` 由迁移服务与排班端点自己读）（#1156）。
- `admin_ip_allowlist` 重启后**控制面板的 IP 限制静默消失**已修；同批三个凭据键 `auth_token` / `proxy_token` / `account_credential_secret` 写侧 JSON 编码、读侧只去空格，轮换过的令牌重启后必然比较失配（#1156）。
- `LOG_CLEANUP_CRON` / `LOG_CLEANUP_RETENTION_DAYS` 生效：水合侧读的键名与写侧写的不一致，此前配了不起作用（#1156）。
- 日志表的清理体制（新调度器还是 legacy `PROXY_LOG_RETENTION_DAYS` pruner）不再被 settings 表里任意一行静默开启、顶掉显式配置（#1156）。
- `system_name` / `logo` / `footer` / `about` / `server_address` 接受空值，清空的品牌信息重启后不再复活；`admin_ip_allowlist` 与 `proxy_error_keywords` **写库失败改回 500**（此前 200）（#1156）。
- 并发批：Redis 补偿回滚改单脚本原子完成，窗口期内别人预占的计数不再被一起删掉；已断开的请求不再耗完准入计数器超时、也不再占着该密钥的串行锁；请求路径不再争一把全局库互斥锁（#1154、#1152、#1151）。
- 界面批：导入站点后账号列表不刷新、行内开关不即时翻转（同一动作在 `/sites` 与 `/accounts` 有两套答案）；三个 i18n 键在两个语言包里都不存在，界面渲染出裸键（#1155）。

## [v0.16.22] — 2026-09-02

### Added

- 下游密钥配额字段 `maxRequests` / `maxCost` / `expiresAt` 是累计总量而非每分钟窗口（每分钟限速是另一组 `max_rpm` / `max_tpm`）；金额单位 USD、仅成功请求计费；留空或 0 ＝不限制（#1148）。
- 配额超限回 429 `insufficient_quota`、过期回 403 `key_expired`（#1148）。
- 界面批：仪表盘四步旅程清单（站点 → 账号 → 路由 → 密钥，CTA 只挂在第一个缺口、四步建成后自我退役）；已有路由但下游密钥数为 0 时就地提示如何签发第一把密钥并用它调 `/v1`，签发后永久消失（#1148）。

### Changed

- `/v1` 上行透传 `anthropic-version`、`anthropic-beta`、`openai-beta`、`user-agent`、`x-stainless-*`：此前 Claude Code 的缓存到不了上游（#1145）。
- `/v1` 下行改内容语义白名单：厂商指纹头与上游 `Set-Cookie` 不再泄漏，也不再覆盖 metapi 自己的 `X-Request-Id`（#1145）。
- `/v1` 写超时不再掐断慢响应：61~90s 之间返回的非流式响应此前被 60s 服务器写超时掐断（表现为「拿到结果却连接被重置」），现由专用中间件持有代理写期限（#1145）。
- Redis 共享限流不再全局串行：配置 `REDIS_URL` 后此前是全局一把锁 + 每命令一次 TCP 握手。**语义零漂移**——出错仍 fail-open 回落本地窗口，错误码、`Retry-After` 与全部 env 名不变（#1147）。
- `PROXY_VIDEO_TASK_RETENTION_DAYS` 默认 0 → 7：`0` 等于保留期调度器整条禁用、`proxy_video_tasks` 无界增长；`<=0` 仍是运维显式关闭，只是不再是默认（#1146）。

### Fixed

- 下游密钥的过期时间生效：表单出站是裸 `datetime-local` 串（形如 `2030-01-02T03:04`），两处解析器都不认，而不可解析的过期时间会**跳过整个过期检查**——一把 2020 年就该过期的密钥仍能以 200 调 `/v1/models`（#1148）。
- 内部正确性批：数据面补 dial 级 SSRF 纵深（DNS rebinding，校验通过后重新解析仍可落到不同地址）；视频任务的进程内重写缓存改按保留期做 TTL（此前是无驱逐的包级 map）（#1145、#1146）。

### Docs

- `docs/api/proxy.md` 新增「Header policy」与「Timeouts」两节；`docs/api/conventions.md` 新增站点上游 SSRF 加固一节。

## [v0.16.21] — 2026-09-02

### Security

- 站点批量导入（`POST /api/sites/import`）、原生备份导入、TS v2.1 备份导入三条路径绕过内网地址闸：构造备份即可植入指向云 metadata 的站点，配合下游密钥一次 `/v1` 请求回流 IAM 内容。现三路径统一走同一个净化器（#1139）。

### Added

- 接管库时间戳归一化：启动迁移把 TS 时代写在 TEXT `*_at` / `*_until` 列里的 `'YYYY-MM-DD HH:MM:SS'` 重写为 RFC3339 UTC——混排时字典序比较失真，`ORDER BY created_at` 错排、范围过滤边界失真、checkin 清扫把全部行当成同一时刻（#1142）。

### Changed

- `/v1` 鉴权与限流错误对齐 OpenAI 信封：裸 `{"error":"..."}` 统一为 `{"error":{message,type,code,request_id}}`（#1140）。
- 同批状态码变更：invalid key `403` → `401 authentication_error/invalid_api_key`，配额耗尽 `403` → `429 insufficient_quota`（#1140）。
- `PROXY_MAX_STREAM_RESPONSE_BYTES` 默认 1MB → 64MB：推理模型长输出常态超 1MB，此前被中途截断且错误事件多被 SDK 静默吞掉（表现为「回答戛然而止」）。同批 SSE 响应补 `X-Accel-Buffering: no`，nginx 系反代不再缓冲首 token（#1141）。
- 死码清扫：零引用生产码删除（`/api/oauth/import` 保持文档化 UI-parity 桩）；web 侧一次性脚本目录整体删除，手写 clipboard 收敛为共享实现（#1137、#1138）。

### Fixed

- `GET /v1/pricing` 此前一直 404（真实可达的是 `/v1/v1/pricing`）；`catalog_sources` 写入在 PostgreSQL 必失败；审计日志路径搜索在 PostgreSQL 大小写敏感（#1141）。

### Docs

- `docs/api/proxy.md` 新增「Error shape (/v1 surface)」（OpenAI 信封 + 完整 status/type/code 对照表）；admin 面（`/api/*`）平铺契约不变（#1140）。

## [v0.16.20] — 2026-09-01

### Security

- SPA CSP 去掉 `style-src 'unsafe-inline'`：收紧为 `'self'` + 每响应随机 nonce + 一处 toast CSS 哈希，其余 directive 不变。
- 凭证导出遮罩改真占位符 `••••••••` + `aria-hidden`：此前用 CSS blur 伪掩码，明文仍留在 DOM 与无障碍树里、读屏可读出（#1080）。

### Added

- 运行事件结构化（checkin 事件族先行）：事件写路径在历史英文 title/message 之外新增 `title_key` + `params` JSON，notify / CSV / 历史行等非 UI 消费者字节级不变。
- 界面批：单行删除改 6 秒「撤销」窗口（真实删除延后到窗口关闭）；下游密钥凭证树形选择器（站点 → 账号 → 密钥三级勾选）；Ctrl/⌘+K 可直接执行动作（#1097、#1026、#1072、#1073、#1080、#1087）。
- 同批无障碍：恢复出厂设置改 type-to-confirm（输入 `RESET` + 倒计时）；主图补 sr-only 数据表。

### Changed

- 账号、模型市场、渠道列表改服务端分页；`GET /api/channels` 新增 `status` 过滤与 `GET /api/channels/error-summary`（#1075、#1077、#1122，issue #1108）。
- `GET /api/accounts` 新增 `q` / `status` / `site` 过滤（此前站点不在当前页就永远筛不出来），`total` 是筛选后计数（#1122）。
- 400 错误体统一携带机器可读 `errorCode`，码表见 `docs/api/conventions.md`（#1065）。
- 畸形凭证引用改显式 400：`allowed/excludedCredentialRefs` 里的非对象、未知或缺失 `kind`、非正 `siteId` / `accountId`、`account_token` 缺 `tokenId` 不再被静默丢弃（对允许列表而言等于静默放宽策略）（#1026）。

### Fixed

- 下游密钥的凭证排除项生效：其 JSON 标签是 snake_case 而管理端持久化形状是 camelCase，代理路径解析不出运营刚配好的排除项（#1026）。
- 运行时设置不再并发撕裂读：运行时写字段与热路径无锁读并存，并发下可能读到半更新状态（如代理 token 校验瞬时 401 抖动），现改为不可变快照交换（#1079）。
- 错误 toast 去重与状态感知本地化（502–504 不再泄漏 axios 英文原文）、列表加载失败改整块替换 + 内置重试（#1082、#1084、#1086、#1088、#1091、#1095、#1099、#1101、#1102、#1104、#1123、#1124）。
- 同批界面：浅色主题对比度达 WCAG AA；站点表单改右侧抽屉，筛选与页码写入 URL。

### Docs

- `docs/api.md` 按域拆分为 `docs/api/*.md`：原文件保留为索引并以 stub 承接原标题，旧深链继续解析。

## [v0.16.19] — 2026-08-29

### Security

- 主 token 不再明文存于浏览器 localStorage：登录改为 `POST /api/auth/login` 换取服务端会话（`admin_sessions` 表），凭证经 HttpOnly + SameSite=Strict 的 `metapi_session` cookie 携带（#1034、#1057）。
- 会话在库里只存 SHA-256 哈希，滑动续期（#1034、#1057）。
- 实时运维 WS 不再接受 `?token=<主 token>`：改由 `POST /api/auth/ws-ticket` 签发 60s 单次 ticket，主 token 从此不进 URL（#1034、#1057）。
- per-IP 限速中间件移至认证之前，401/403 不再绕过桶约束；`/api/auth/*` 另加严格桶（#1034、#1057）。
- 备份导出（下载与 WebDAV）、下游密钥导出、主 token 轮换要求 `X-Admin-Confirm-Token` 头，否则 403 `reauthRequired`。**这些操作从此要多带一个头，自动化脚本需跟着改**（#1034、#1057）。

### Added

- `ADMIN_SESSION_TTL_MINUTES`（默认 720）、`ADMIN_SESSION_COOKIE_SECURE`（默认 auto）、`AUTH_RATE_LIMIT_RPS` / `AUTH_RATE_LIMIT_BURST`（默认 10/20）（#1034、#1057）。
- `cmd/migrate` 携带 `admin_sessions`，会话跨 SQLite ↔ PostgreSQL 切换存活（#1034、#1057）。
- 上游账号健康监测全局开关：运行时设置 `checkinEnabled` / `balanceRefreshEnabled`（热生效、持久化）与环境变量 `CHECKIN_ENABLED` / `BALANCE_REFRESH_ENABLED`（#1027、#1056）。
- 账号 `proxyUrl` 接受 `socks5://` 与 `socks5h://`（#1009、#1059）。

### Fixed

- 清空账号代理字段保存即清除：此前前端清空后载荷省略该字段、后端合并又把旧值留下（#1009、#1059）。
- 内部正确性批：job panic 统一 recover、in-flight 标志锁外读竞态修复、`channel_recovery` / `backup_webdav` / `model_probe` 吞没的 DB 错误显性化（#1061、#1052、#1060）。
- 同批 PostgreSQL 方言陷阱清扫：BOOLEAN 整数字面量绑定、管理搜索 LIKE 大小写。

### Changed

- 高频管理读路径加索引（300k 行实测最贵一条 17.9ms → 0.5ms）；出站调用点统一超时与连接池；focus-ring 统一 token，流量与成本图补 sr-only 摘要表（#1054、#1058、#1053、#1055）。
- `rsbuild.config.ts` 成为唯一前端构建配置，`vite.config.ts` 删除（#1053）。

## [v0.16.18] — 2026-08-29

### Added

- `PROXY_STREAM_IDLE_TIMEOUT_SEC`（默认 300）：每转发一个 chunk 重置计时窗，窗口内无新 chunk 即中断卡死的流并按上游超时故障记录（渠道健康、失败日志、终态一致）。只约束 chunk 间隔、不约束流总时长；`0`、负数、非法值回退默认（#1046）。
- `proxyRetryStatusRanges` / `proxyDisableStatusRanges`：运行时设置（设置页可编辑，`PUT /api/settings/runtime` 同契约）——retry 决定哪些上游状态码计为可重试的渠道故障，disable 决定哪些状态码在冷却升级之外直接禁用故障渠道（#1049）。
- `PUT /api/channels/batch` 部分更新：只写字段出现的、载荷校验（空批 / 超 1000 / 重复 id / 无可更新字段）、逐项真值返回；模型测试台出现失败行时提供「禁用失败渠道」动作（#1047）。
- 界面批：渠道页失败横幅 +「只看失败」经可分享的 `?status=` 进入仅失败视图；密钥表单新增「上游站点限制」多选（留空 = 不限制，完整往返既有 `allowedSiteIds`）；CmdK 面板命中直接深链到实体本身；zh-CN 语言下站点表崩溃修复（#1048、#1026、#1039、#1036）。

### Fixed

- `POST /api/routes/rebuild` 消费 `refreshModels`（默认 true：先批量刷新全部活跃账号的上游模型、单账号失败不中断整批，再重建通道）；完成提示读取真实统计并区分三态——成功、无路由可重建、通道无变化（#1024）。

### Security

- CSP 收紧：内联 bootstrap 脚本外部化，移除 `unsafe-inline`（#1041）。同批前端安全、构建、可访问性、卫生与 UX 小修，无对外行为契约变化（#1037、#1040、#1042、#1043、#1044、#1045）。

## [v0.16.17] — 2026-08-28

### Added

- 渠道与账号表格新增探测历史健康条，配只读端点 `GET /api/channels/probe-history` 与 `GET /api/accounts/probe-history`（`limit` 1–50，默认 20）；竖条按时间着色，tooltip 汇总窗口成功率与平均延迟，键盘可达（#1020）。
- `route_channels` 与 `oauth_route_unit_members` 新增可空列 `cooldown_reason_code` / `cooldown_reason` / `cooldown_reason_at`（additive step `sc2_025`，双方言）（#1019）。
- 渠道页冷却/熔断徽章可点开根因弹窗，旧数据如实显示「原因未记录」（#1019）。

### 开发者可见

- 协议转换层补快照回归套件，零生产代码改动（#1018）。

## [v0.16.16] — 2026-08-28

### Added

- `MODEL_SYNC_CRON`（默认 `0 4 * * *`，每日 04:00）：定期批量刷新全部活跃账号的上游模型列表；设置页 `modelSyncCron` 支持运行时热更新；单账号失败不中断整批。手工单账号刷新端点行为与响应载荷不变（#1005）。
- `PROXY_*_TIMEOUT_SEC`（connect 2 / TLS handshake 10 / response header 30 / idle conn 90 / request 30 秒）：非法值回退默认，未配置时零行为变化；完整变量名见 `docs/configuration.md`（#1009）。

## [v0.16.15] — 2026-08-27

### Fixed

- 账号表单验证真值：inline token verification 与账号创建改用表单中尚未保存的 `platformUserId` / `proxyUrl`；显式表单代理覆盖站点、Resin 与系统代理，创建后持久化 `extraConfig.proxyUrl`，非法值 fail-closed（#1007）。
- Accounts 分页：URL 控制的页码选择不再被表格库自动重置，第 2 页稳定渲染第 11–20 行（#1008）。

## [v0.16.14] — 2026-08-27

### Changed

- 账号创建/登录后的 toast 如实报告后端 `tokenSyncStatus` / `tokenCount` / `tokenSyncMessage` 四态：synced 显示真实持久化计数、empty 提示暂无上游令牌、failed 降级为部分初始化警告、skipped 与旧响应保持原引导文案（#1002 后续）。
- 同步 failed 不回滚：账号保留、可在令牌面板重试同步，`/token-routes` CTA 在所有状态保留（#1002 后续）。

## [v0.16.13] — 2026-08-25

### Added

- Session 账号 token 自动同步：账号创建/登录后自动同步上游 token，响应报告真实持久化的 `tokenCount` 与 `tokenSyncStatus` / `tokenSyncMessage`；同步失败仅发出部分初始化警告并保留已验证账号，绝不回滚；API-key 连接显式跳过（#1002）。
- 账号详情新增 Models 面板：手工上游刷新、手工添加与显式移除模型、来源/可用性状态诚实呈现；路由重建与缓存失效每次刷新动作恰好发生一次；上游失败诚实报告且无副作用（#998）。

## [v0.16.12] — 2026-08-25

- 下游密钥模型授权语义：精确模型名、glob（`*`）、`re:` 正则；**留空＝拒绝所有**，`*` 是显式全允许（#999）。
- 公告外链只接受基于受信站点 URL 解析出的 http(s) 地址，不安全来源回退站点首页且不渲染为外链（#1000）。

## [v0.16.11] — 2026-08-25

- 无运维可见变更：均为前端与测试层修复（OAuth start 流、model-tester 行级重跑、视觉回归基线）。

## [v0.16.10] — 2026-08-25

- 安全：站点代理的全部传输（普通 / 池化 / uTLS）在拨号前拒绝云元数据与链路本地目标；站点 URL 校验收紧（拒绝 opaque、内嵌凭据与非法端口）。RFC1918 与 localhost 保留给实验环境。
- 新增 `/site-announcements` SPA 与站点公告 admin API（统一 camelCase、`siteId` 严格校验、未知站点 404）（#986）。

## [v0.16.9] — 2026-08-24

- 模型目录改双源合并（llm-metadata + models.dev），支持自动与手动同步；`newapi` ratio 倍率用于转发成本估算（#971、#972）。
- 设置中心拆为五组，旧 URL 自动重定向（#971）。

## [v0.16.8] — 2026-08-23

- 安全：恶意备份文件可被用来外传凭据，**旧版本应升级**（#941）。
- 后端用户可见消息统一为英文（#938）；停机等待在途调度任务，上限 5 秒（#946）。

## [v0.16.7] — 2026-08-22

- 安全：代理转发路径穿越（#922）、导出文件公式注入（#925）、公共路由危险路径段与私网网段拦截补齐（#917），**旧版本应升级**。
- 迁移前校验备份文件完整性，被篡改的备份会被拒绝（#919）。
- 新增 `install.sh` 安装脚本；Docker 默认改用命名卷持久化数据（#921）。

## [v0.16.6] — 2026-08-21

- 账号令牌接口字段统一 camelCase、布尔字段类型修正，**调用方需跟着改**（#911）。
- 站点 / 账号 / 路由更新不再静默丢弃会话启用与协议限制字段；未传分页参数的频道列表返回全量，不再截断为 50 条（#911）。

## [v0.16.5] — 2026-08-21

- 新增版本信息接口，关于页展示真实版本与构建信息（#904）。

## [v0.16.4] — 2026-08-21

- 未刷新的账号金额显示为占位符，不再伪造「余额归零」（#889）。
- WebDAV 导入与 usage-log 清空前增加确认对话框；路由表单脏值关闭走路由守卫（#889）。
- 登录页补 token 指引，token 轮换后自动登出；会话过期保留返回路径、无效 token 主动登出（#889）。

## [v0.16.3] — 2026-08-21

- 关于页版本号改为构建期注入，不再与实际版本脱节（#891）。
- 新建站点后账号页缓存同步失效，「添加账号」不再被陈旧快照禁用最长 30 秒（#895）。

## [v0.16.2] — 2026-08-20

- Docker 镜像以非 root（uid 1001）运行：**bind mount 数据目录若由 root 写入会触发只读数据库错误**。启动前探测数据目录与库文件可写性并给出 `chown` / `chmod` 提示；命名卷零配置（#875）。
- 从 TS 旧后端迁移的库启动时自动补齐缺失列，老库无需手动操作（#878）。
- `metapi-migrate --verify` 校验和语义修正：列序与行序无关、settings 过滤运行时键、跨方言布尔归一（#875）。

## [v0.16.1] — 2026-08-19

- 无运维可见变更：OAuth start 流断链修复与一批前端错误态、深链、无障碍修复（#862）。

## [v0.16.0] — 2026-08-18

- 代理日志的客户端 / 来源 / 目标过滤改为服务端执行（#832）。
- 下游密钥支持编辑（密钥值不可修改），保存时仅更新变更字段（#835）。

## [v0.15.3] — 2026-08-17

- 无运维可见变更：弹窗滚动与溢出、站点表单校验反馈修复（#822）。

## [v0.15.2] — 2026-08-17

- 无运维可见变更：弹窗长内容溢出修复（#815）。

## [v0.15.1] — 2026-08-17

- PostgreSQL 下 `/api/models/token-candidates` 因布尔列比较写法不兼容返回 500，已修（#805）。
- 接口内部出错时服务端记录错误并附请求 ID（#805）。
- 品牌文案统一为「Metapi」，接口行为不变；Nginx 反代模板补 WebSocket 升级头，避免 WSS 握手在代理层被中断（#805）。

## [v0.15.0] — 2026-08-17

- 站点表单支持按站点覆盖粘性代理与 TLS 指纹（继承全局或强制开启、关闭）（#807、#809）。
- 通知设置重启后不再静默回退（#807）。

## [v0.14.0] — 2026-08-17

- 账号表单新增密码凭证登录模式（#770、#772、#774、#775）。
- 路由策略新增最低成本 / 最少忙 / 最低延迟，成本取自官方价格目录（#783、#790）。
- 站点与模型熔断器支持半开探测，已恢复的通道可重新进入候选（#791）。
- SQLite 默认 `PRAGMA synchronous = NORMAL`（WAL 下仍 crash-safe）与 `cache_size = 10000`；成功请求逐步衰减历史失败计数（#784、#785）。

## [v0.13.0] — 2026-08-15

- 安全：WebDAV SSRF（#741、#761、#763）、OAuth 出站与 WebSocket 来源校验（#702、#726）、管理端列表不再返回明文凭证（#719），**旧版本应升级**。
- `/v1` 支持按 IP 限流，可配令牌频率限额与请求体大小上限（#709）。
- 新增粘性代理池（#693、#698）、Grok/xAI OAuth 设备授权适配器（#696）、图片生成接口透传（#697）、Electron 桌面客户端（#704，已于 v0.18.0 移除）。
- 平台自动识别修复（one-hub / done-hub / veloera / sub2api / cliproxyapi，one-api 不再被误标为 new-api），新增 SenseTime 检测（#684、#689、#706）。

## [v0.12.0] — 2026-08-14

- 离线迁移工具不再静默丢数据：迁移复用正式建表逻辑并加结构一致性守卫（#651）。
- 运行时切换数据库不再丢失 SSL 模式与连接池配置（#651）。

## [v0.11.0] — 2026-08-14

- 安全：生产部署加固（健康检查、no-new-privileges、丢弃 capabilities、只读根文件系统、临时目录隔离、资源限制）（#639）；管理控制台 token 列默认脱敏（#601、#602）。
- 周期任务此前从未执行：余额刷新 / 日志清理 / WebDAV 备份未被调度器注册（#635）。
- 访问日志新增 `status` / `bytes` / `duration_ms`；`/metrics` 新增 Go 运行时指标（goroutine / 内存 / GC）（#593）。
- `.env.example` 补全可选配置项（#640）；发布改为 SemVer tag 触发多平台二进制 + 校验和 + GitHub Release，新增 install.sh（#637）。

## [v0.10.0] — 2026-08-12

- 定时计划新增 v1 格式（每日 / 间隔 / 随机窗口 / 自定义 Cron），旧字段完整兼容。
- 审计日志改服务端分页（`limit` / `offset`）。
- 数据库设置页移除应用内迁移按钮，改为提示通过 CLI 执行迁移。

## [v0.9.0] — 2026-08-11

- 前端整体重写为单页应用（Bun + Rsbuild 2 + TanStack Router/Query + Tailwind 4 + Zustand），构建由 npm 改为 Bun；旧版前端页面与组件移除。
- 嵌入式前端白屏修复：SPA 回退曾把 `/static/*` 静态资源当作 HTML 返回。
- 品牌名统一 MetAPI，透明 SVG 徽标与 favicon 替换旧版 PNG；顶栏语言切换（en / zh-CN）。
