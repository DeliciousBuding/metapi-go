# Changelog

All notable changes to Metapi-Go will be documented in this file.

格式基于 [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)，
版本号遵循 [Semantic Versioning](https://semver.org/spec/v2.0.0.html)。

**写作契约**（CI 把 `## [vX.Y.Z]` 段直接抽成 GitHub release body：这里写的就是发布说明）：

- 读者是**部署和调用 Metapi 的人**。写「行为怎么变了、我要不要动手」，不写「我们怎么查出来的」。
- 一条 = 一行结论 + 至多两句因果 + `(#PR)`。取证过程、测试与门禁的形状、行数增减、内部文件清单留在 commit body 与 PR 里（`git log --grep '#1210'` 一步到达）。
- **需要运维动手的必须显式写出来**：升级后要重新登录、环境变量默认值变了、备份不再含某张表、某个端点删了。这是本文件唯一不可省略的部分。
- 编号写成 `#问题 → #PR`：前者是报障/立项的 issue，后者是落地它的 PR（commit subject 带的是 PR 号，`git log --grep '#1211'` 直达）。只有一个编号时它就是 PR。
- 分节固定为 `安全` `修复` `变更` `移除` `开发者可见` `文档` `已知遗留`，空节不写。v0.16.23 及更早沿用当年的 `Added/Changed/Fixed/…` 与措辞，不回填重写。

## [v0.19.0] — 2026-09-04

> 核心链（站点 → 账号 → 长期凭据 → 模型 → 路由 → 可用通道 → 下游密钥 → `/v1/models` → `/v1/chat/completions` → 真实中继）做成可重复、可解释、可恢复的契约。**无新增产品面**：无新 adapter、无新协议、无新 dashboard、无新计费，#1132 继续排队。四态旅程 Fresh / Restart / Aged / Restore 从此都是能红的门（issue #1215）。

### 修复

- **balance 刷新失败给出分类原因（#1210 → #1211）**：此前只回 `502 {"message":"balance refresh failed"}`，而同一刻服务端日志已写着真因。「凭据过期」「上游拒绝」「上游不可达」「上游没返回余额」是修法完全不同的四个问题，却共用一句文案。现在单个与批量两个出口都保留稳定前缀并追加分类原因（如 `balance refresh failed: upstream rejected the credential (HTTP 401)`）；只渲染既有错误类枚举，**不外泄**上游 URL、token、query 或原文，原文另进 WARN 日志。顺带修掉 `service/balance` 一处 `return nil, nil`：DB 读失败曾被当成「账号不存在」回 404。
- **PostgreSQL 备份导入后重置 id 序列（#1217 → #1218）**：备份带的是行当年写入时的显式 id，导入后 serial 序列仍停在原位，于是**恢复完成后的第一次普通写入**就撞 `duplicate key value violates unique constraint "sites_pkey"`——错误只提约束、不提序列，看不出与导入有关。现在 migrator 与导入共用 `store.ResyncPGIDSequences`；失败语义分开：migrator 警告续跑，导入判致命。SQLite 不需要（`AUTOINCREMENT` 自维护 `sqlite_sequence`）。
- **`model_probe_results` 有了保留期（#1221 → #1222）**：全仓写入量最大的一张表（探针开启时繁忙实例一天数万行）此前**没有任何 DELETE 路径**，且被排除在备份外。现在登记进既有 `RetentionScheduler`（与 `proxy_logs` 同一 owner，不造新框架、不加新配置项）：7 天窗口、每小时一次，并豁免每个 `(account_id, model_name)` 的**最新一行**——路由重建读的就是那一行，纯按年龄删会在探针暂停或账号闲置超过窗口后让一个清理任务悄悄改变路由行为。
- **UI 不再把能用的通道说成坏的（#1219 → #1220）**：账号详情抽屉按 wire 逐个报出每条通道真正使用的凭据来源，不再用一个笼统状态掩盖；下游密钥的空 model policy 明确是 deny-all（不授权模型＝发一把看着健康、什么都调不动的密钥）。
- **架构边界门不再可能空转（#1214）**：扫描遍历不到任何生产 Go 文件时曾返回「零违规」并判通过——扫不到东西的门不是宽松的门，是不存在的门，与让约三十个 tag 在必选分片跑 0 个测试仍持续全绿的是同一形状（#1175）。现在任一域扫到 0 个文件即失败，denylist 一字未改；skip 列表补上 agent worktree 与 gitignored 私有目录（两个 checkout 在场时曾把 366 个生产文件解析成 732 个）。

### 变更

- **公开 e2e 脚本兑现「可重跑」（#1209 → #1212）**：`scripts/e2e/verify-token-import.sh` 此前只在账号**不存在**时创建它，于是上游凭据短命的平台每过一条寿命就必红一次。现在账号存在时用本轮已持有且刚验证过的同一枚凭据 `PUT /api/accounts/{id}` 收敛（绝不二次签发），site / account / key / route 各自打印 `created` / `reused` / `refreshed`，让「只是没动它」无法冒充「验过了」。`smoke.sh` 行为不变。

### 开发者可见

- **Restart 门（#1213）**：链跑完后 kill 服务、对**同一个 data dir** 重启，中间不重新登录、不重新绑定、不重建路由、不重发密钥，再重新证明：kill 前写入的辨识性设置仍读得回、`/v1/models` 非空、completion 带精确 marker 且确定性 mock 上游的事件计数递增（防缓存或重放冒充中继）。kill 后必须先证明端口不再应答，否则整条门失败。
- **Aged 门（#1209 → #1212）**：先老化**存量**凭据、再用同一枚凭据复跑同一条链并要求绿；老化请求先自检 HTTP 200，以免「什么都没老化」的探针静默通过。
- **Restore 门（#1216）**：从刚建好链的实例导出备份（属敏感操作，需 `X-Admin-Confirm-Token`），在**全新 data dir** 上起第二个实例导入，然后**不重建任何东西**直接断言 `/v1/models` 非空 + completion 精确 marker + mock 收到新事件。两条前提自检：导出必须含中继所需的 6 张表，导入前该实例必须 0 模型。
- **前端验收补上旅程尾段（#1220）**：此前停在签到，证明的是运营**能配置**——恰好就是「部署完基本用不了」那类报障的形状。新增四步：绑定后账号详情必须显示**非空**模型且与 `GET /api/accounts/{id}/models` 条数一致 → 建出第一条路由并逐个报出通道凭据来源 → 在 UI 里签发下游密钥并授权该模型 → 用**独立 HTTP 客户端**（无页面、无会话、无 admin token）调 `/v1/models` 与 `/v1/chat/completions`，要求真 2xx + content 非空，结构化 error 体判失败，无中继能力的链只报 SKIP 不报 PASS。另加 `/api/about` 身份 preflight（`EXPECT_SERVER_COMMIT` 不符即拒跑）：本轮有三次「绿跑」验的其实是旧内嵌 SPA，因为上一轮的孤儿实例一直占着端口。

### 已知遗留

- **#1132 请求头模板**：新产品面，冻结期排队，不计为欠债。
- **`admin_audit_logs` 与 `checkin_logs` 仍无保留期**：删审计历史是需要运营明确点头的产品决定（保留多久、是否可配、有无合规要求），不顺手塞在一次遥测清理旁边。
- **`handler/admin` 仍有 4 个千行单文件**（`stats.go`、`downstream_keys.go`、`token_routes.go`、`sites.go`）：同包文件族拆分在 `accounts_*` / `settings_*` / `stats_*` 上已落地；再往下要么按动词切单一资源 CRUD、要么搬包，而**没有任何已观察到的用户故障可归因于此**。
- **第二轮测试消融未做**：KPI 是故障场景守恒而非 LOC，没有变异探针支撑的折叠宁可不做。
- **12 个顶层 Go 包整体搬进 `internal/` 判 NO-GO 并已丢弃**：`pkg.go.dev` 上该 module 未被索引、仓库只出一个二进制 ⇒ `internal/` 的语言层收益对本仓为 0，代价是数百文件路径 churn 与 4 道发布门被语义改动。这次消融唯一的收获转成了上面的边界门空转防护（#1214）。

## [v0.18.0] — 2026-09-03

> 减法里程碑：净减约 5000 行（未发布的外壳、无调用者的 helper、未接线的分支、四张无读写者的表、一整个 debug-trace 子系统、重复测试簇、维护者过程文档），同时把 New API 主链上三个能各自独立致死的缺陷根修。

### 修复

- **New API 中继链从「看起来绑上」变成持久（#1179 → #1187）**：三个缺陷叠在一起。① v1 登录返回的约 15 分钟 dashboard JWT 被当长期凭据持久化 ⇒ 15 分钟后模型刷新塌成空列表、路由无通道、`503 No available channels`；② 令牌列表把 relay key 显示成掩码值，掩码显示值被当真实凭据落库 ⇒ 请求 401、通道被停用；③ 模型列表要等后台调度（默认凌晨 4 点）才出现。现在登录时把 JWT 提升为 New API 的长期 dashboard PAT，**发不出持久凭据就拒绝绑定**而不是显示成功，并用 live JWT 撤销临时上游会话；掩码 key 经所有权校验的 `POST /api/token/batch/keys` hydrate 成真实 key，hydrate 不出来就明确报错（掩码行在 UI 上诚实地保持 `masked_pending`，不声称持有没有的 key）；登录成功后立即同步模型，route rebuild 可以服务触发它的那次请求。**升级后需要重新登录一次账号；令牌不需要手工绑定。**
- **`503 No available channels` 现在说清为什么（#1179 → #1186）**：通道选择曾把「无可选」报成 `(nil, nil)`，失败路径记 `err=<nil>`，于是「没有启用的路由匹配这个模型」「路由匹配但每个通道令牌未绑定或停用」「所有通道都在冷却，或下游密钥策略排除了站点」三种完全不同的运维问题输出一字不差。现在 `proxy.ExplainNoChannel` 渲染一行紧凑结论加主候选拒绝原因，并同时进 503 body。
- **重建路由不再随发出它的 HTTP 请求一起死（#1174 → #1185）**：`POST /api/routes/rebuild` 曾把整趟 pass 跑在 `r.Context()` 上，`refreshModels: true` 时真实车队要几分钟，而 web client 只给 30s ⇒ 客户端先挂断、日志里是 `context canceled`、重建永久失败。现在 handler 用 `context.WithoutCancel` 分离请求上下文并配 30 分钟预算，收尾重建总是执行（无变化时短路不写）。`POST /api/settings/maintenance/clear-cache` 也不再删掉重建要用的输入：它只失效进程内缓存并排同一个后台重建，清业务行是 `factory-reset` 的职责。
- **账号编辑保存的凭据模式不再蒸发（#1176 → #1184）**：编辑对话框每次保存都发 `credentialMode`，但更新 payload 没有这个字段，handler 解码时静默丢掉 ⇒ 改成 session、拿到成功 toast、重开又是 API key。现在该字段进 payload 并合并进 `extraConfig`（创建路径写的同一个存储），且在碰任何凭据字段**之前**解析——此前显式切到 session 也会把 session 凭据抄进 `api_token`，而代理的凭据回落会把它当 API key 发出去。存储凭据撑不起的模式现在 400 而不是落库。
- **AnyRouter 校验失败终于说明它要什么凭据（#1133 → #1195）**：运营粘贴一段对上证明可用的 cookie，得到的只有 `token verification failed`。现在 bind 与 verify 两条路径共用 `credentialVerificationFailureMessage(platform)`：AnyRouter 明确说 access-token / API-key 绑定不支持、必须用 session 模式、字段接受 `session=<value>` 或完整 `Cookie:` 头；其它平台保留原指引并补同一形状的提示。
- **「同步站点令牌」幂等，掩码 key 不再每点一次多一行（#1193 → #1196）**：上游掩码 relay key 时行解析成 `masked_pending`、永远匹配不上 ready 行，于是每次同步都 INSERT 一份副本。现在按精确 key 值匹配已有行、优先 ready 行、回落 masked 行，UPDATE 保留匹配行解析出的 `value_status`——掩码行诚实地保持掩码。已知残余：本版之前绑定的账号可能留一条 `masked_pending` 旧行，一次性、可见标记、不可路由，运营可删。
- **删除一张表不再让删除之前写的每个备份都恢复不了（#1201）**：删表曾让导入拒绝它不认识的任何 payload key，于是一个本来有效的旧备份变成 `400 {"error":"import failed: unknown table proxy_debug_traces"}`。现在按**来源**区分而不是维护一份永远增长的退役表清单：策略排除表（`admin_sessions`、`admin_audit_logs` …）与手写 JSON 里的未知 key 仍响亮 400；真正导出文件里的未知 key 说明是本 build 已删的表，跳过并在 import 与 preview 两个响应里报告为 `ignoredTables`。

### 移除

- **未发布的 Electron 桌面外壳（#1197）**：连同公开、未认证、只为托盘而设的 `GET /api/desktop/health` 一起删除。发布流水线从未产出过桌面产物，外壳只能由维护者手工构建到。
- **永久为空或无人实现的控制面（#1199、#1201）**：proxy debug-trace 子系统（两张表、五个索引、两个读路由、九个 `PROXY_DEBUG_*` 开关及其设置 UI——从没有任何东西写过一行，九个用户可见控件控制的表面永久为空）· `proxy_files` 表 + `PROXY_FILE_RETENTION_*` + 其 retention 调度器（被创建、备份、修剪，却从未被写入）· `admin_snapshots` 表 + 其调度器（唯一碰它的语句是周期性 DELETE 从未插入过的行；usage aggregator 的 `RunProjectionPass` 仍由 usage-aggregation 调度器跑）· update-center 的五个操作臂（`POST /api/update-center/check`、`PUT /config`、`POST /deploy`、`POST /rollback`、`GET /tasks/{id}/stream`）+ `METAPI_ENABLE_UPDATE_CENTER`，只读状态卡保留 · model-tester 的流式/任务臂（`/api/test/proxy`、`/api/test/chat/stream` 及各自 jobs 端点，从未实现成 SSE），同步探针 `POST /api/test/chat` 保留。
- **无实现者与无调用者的代码（#1187、#1190、#1198、#1201）**：`RouteRefreshWorkflow` 家族（生产没有实现者，只有测试 mock 在养活这个接口）· `RESPONSES_COMPACT_FALLBACK_TO_RESPONSES_ENABLED` · `routing` 里整条死的 pricing override 链（`NormalizePricingRatio`、`EstimateProxyCostFromModel`、`BuildPricingOverrideModel` 全仓零调用者，#1204）· 六个零引用 helper · 四个不可能失败的测试（零断言、只断言本地字面量算术、接受任何结果、唯一的 `if` 是空 body）。
- **16 份维护者过程史移出公开仓（#1191、#1194）**：`docs/internal/` 下的 STATE / log / MASTER / benchmark 与 11 份 analysis 及设计稿删除。它们的角色改由 GitHub issues 与 releases（现状与开放项）、本文件（版本叙事）、`docs/architecture.md` 的归属与边界图（由 `docs/package_boundary_test.go` 机器强制）承接。

### 开发者可见

- **同形测试折成表格（#1189、#1202、#1203、#1206）**：proxy 重试/失败判定的 104 个 `t.Run` 折成 6 块，决策用例 71 → 71（输入→期望 tuple 从原文件抽取后按集合机械比对，差集为空）；15 份 per-adapter `PlatformName` 测试并成一张注册表门禁，行跑完后 anti-shrink 交叉检查断言表名集合与 `ListAdapters()` 双向相等；`handler/admin` 18 个错误路径测试折成两张数据行表，18 → 18 且子测试名保留；web 侧同法折叠四个 zod schema 套件（#1188）与七个 status-badge 布线套件（#1200）。`handler/admin/accounts.go` 按关注点拆成 6 个同包文件（#1192，零行为变化）。
- **CI 的 `bun install` 对瞬时故障有界重试（#1181）**：损坏或截断的 tarball 报 `Integrity check failed`——不是被测提交的属性、重跑就装得上，却能挂掉必选检查挡住合并（一天内命中四个包、跨三个 job）。同时修 Dockerfile 的缓存挂载路径：挂在 `~/.bun/install-cache`，而 bun 读写的是 `~/.bun/install/cache`，一个连字符对一个斜杠，那句缓存声明从未缓存过任何东西。
- **仓库卫生（#1194）**：`.gitignore` 补 `.env.*`（保留 `!.env.example`）、`*.log`、`/dist-bin/`；缓存 key 从哈希一个不存在的 lockfile 改为哈希真实存在的那个（此前 `hashFiles` 对不存在路径返回空串、缓存 key 恒定、依赖升级从不失效）。

### 已知遗留

- **#1132 请求头模板**：新产品面，继续排队。
- **`PayloadRules` 没有运行时消费者、`OpenAiServiceTierRules` 没有读者，而 `docs/configuration.md` 仍把两者写成可用**：改文档说实情还是落地规则引擎，未决。
- **`OpenAiAdapter.GetModels` 不再有直接单元门禁**：被删的那个测试接受任何结果，此前也并非真被把守；真门禁需要一个 httptest 上游，是单独一件事。

## [v0.17.1] — 2026-09-03

### 修复

- **备份导出不再静默丢 5 张用户可见状态表（#1172）**：`service/backup.AllTables` 是仓库里第三份手抄表清单，漂移到 37 张表里的 28 张，于是 `GET /api/settings/backup/export?type=all` 每次静默丢弃 `product_announcements`（运营撰写的产品横幅消失）、`announcement_dismissals`（用户的「已忽略」状态丢失 ⇒ 已读公告重新弹出）、`model_name_redirects`（`source=manual` 的行是运营手写、目录同步永不重生成 ⇒ 手工改名规则全丢，上游模型名解析回退）· `balance_history`（余额趋势从恢复日起空白，过去某天的上游余额永远无法回看）· `model_verify_history`（运营发起的批量校验历史消失，不会自动重生成）——而导入端回 200 报成功。
- **备份文件自己陈述自己的缺口（#1172）**：导出 payload 的 `metadata` 新增 `excluded_tables`（表名 → 理由）。**只加字段**：`exported_at` / `version` / `type` / `tables` 形状一字未改，两条导入路径都完全忽略 `metadata`，WebDAV 往返与 TS v2.1 兼容层不受影响。导入请求点名要一张被排除的表 → **400**（此前静默忽略）；旧备份缺表 = 跳过而不是错误。
- **4 张表显式排除，每项都带理由并进 metadata 与文档（#1172）**：`admin_sessions`（备份是半可信输入，导入 token hash 会让源站签发的 cookie 在本站生效；恢复后要求重新登录才是正确语义）· `admin_audit_logs`（源站 admin 写操作的 append-only 轨迹，灌进另一个库等于让该库断言从未发生过的操作）· `model_probe_results`（后台探针每 tick 重建，且路由重建只取每 `(account, model)` 的最新一行）· `catalog_sources`（每行的 `url` 由目录同步**服务端抓取**，而导入 URL 闸当时只覆盖 `sites` 与 `site_api_endpoints`；让半可信备份写入这张表等于可以被植入 cloud-metadata / link-local 抓取目标——扩展那道闸之后删掉这一条即自动纳入）。
- **一处对上一版文档的更正（#1172）**：`docs/api/settings.md` 给恢复出厂写的退路是「需要就先导出备份」——这句话在写下时就是假的（备份从来不含 `admin_audit_logs` 与 `model_probe_results`），等于引导运维走一条不存在的退路。现明确说明备份不保留它们，需要就自己用数据库工具 dump。
- **库里一条读不出来的设置行不再清空已配置的值（#1173）**：`payload_rules` / `openai_service_tier_rules` / `checkin_schedule_mode` / `notify_task_toggles` 的水合分支曾把解析结果直接赋值，而解析器用 `nil` 编码失败 ⇒ 一条坏行在下次重启时把已经配置好的规则集清空，日志里一个字都没有。现在坏行丢弃并告警；JSON `null` 与 `[]` 是**可读的清空意图**，照常生效（UI 清空 textarea 走的正是这条）。**启动/水合日志因此可能新增 WARN 行**，那是此前静默的不一致第一次变得可见。
- **每个部署每次启动都会打的那条假 WARN 没有了（#1178）**：`setupSPAFallback` 除挂 Rsbuild 真正产出的 `/static/*` 外还挂着 Vite 时代的 `/assets/*`，注释说是为旧构建保留兼容——**这个兼容在构造上不可能成立**：前端由 `//go:embed dist` 在编译期烘进二进制，一个二进制只携带本提交构建的那一份 dist，内嵌树自迁移起就没有 `assets/` 目录，于是那个挂载永不可达而守卫每次启动报警。
- **前端布局 ↔ 路由挂载的契约现在有门禁（#1178）**：构建出的 `index.html` 里每一条 root-relative 的 `src` / `href` 都必须回 200 **且不得以 `text/html` 回来**。只看状态码不构成契约：SPA fallback 对每个未挂载路径都答 `200 text/html`，而这正是把白屏 UI 藏在一个看起来正常的状态码后面的方式（nosniff 浏览器拒绝把 HTML 当脚本执行）。

### 开发者可见

- **竞态门在 CI 里真的跑起来了（#1175）**：`test-sqlite-shard` 的分片算术把两个 matrix 值当裸字面量写进 bash，报的语法错误恰好位于 `if (( … ))` 条件里，而 `set -e` 对条件命令不生效 ⇒ 错误被吞掉、分片选到 0 个包、走进 `nothing to run` 分支 `exit 0`。四个分片与聚合出的必选检查全绿而**执行的测试数量是 0**，`-race` 也随之长期消失，从引入分片矩阵的那个提交起约三十个 tag 都是这个状态。修复分三层，每层都让「静默变绿」在结构上不可能再发生：matrix 值改经 `env` 绑定（配合 `set -u`，再弄丢就报 `unbound variable`）· 轮转槽位改成独立赋值而不再是条件（畸形选择器直接中止 step）· 断言本片应得的包数，**选少了和选不到一样红**。这条不变量写进了 `docs/testing.md`，免得守卫被后人当成仪式删掉。
- **e2e 备份用例的「导出应有多少张表」不再字面量**：从注册表取值——那曾是仓库里第四份手抄清单，注册表化之后它立刻变红，也正是这次 CI 抓出它的。

### 已知遗留

- **`PayloadRules` / `OpenAiServiceTierRules` 文档超前于实现**（与上一版删掉的 `HOME_PAGE_CONTENT` 同一类，只是这次有个 UI 在读写它）：改文档说实情还是落地规则引擎，待裁决。
- **`notify_task_toggles` 的 admin 写路径应当对错误形状返 400**（本版只让它不再毁掉 runtime）。
- `catalog_sources` 的导入 URL 闸扩展（扩展后即可解除备份排除）· PostgreSQL 导入不重置序列 · 约 20 个不可解析**数值**分支静默保留 fallback，建议在解析器失败路径做一次聚合 WARN · `POST /api/accounts/verify-token` 失败时只回 400、服务端零日志。

## [v0.17.0] — 2026-09-03

### 安全

- **转发链客户端 IP 改为从右往左解析（#1161）**：配置 `TRUSTED_PROXY_CIDRS` 时，客户端身份不再取 `X-Forwarded-For` 的最左值（那一段由调用方自己塞、攻击者可控），而是把所有转发头按出现顺序拼成一条链、补上直接 peer，再从最右往左跳过属于可信 CIDR 的地址，返回第一个不可信地址。修掉三个后果：admin IP 白名单可被一个伪造头直接绕过；每 IP 限流可通过每请求换一个假 IP 无限换桶，登录暴破上限形同虚设；审计日志记的是攻击者自选的 IP。
- **账号写路径不再回显明文凭据（#1163）**：`PUT /api/accounts/{id}` 与 `POST /api/accounts/{id}/rebind-session` 此前原样返回 `accessToken`、`apiToken` 与 `extraConfig.autoRelogin.passwordCipher`，使读路径的脱敏形同虚设——任何能调 PUT 的会话发一次 `{"sortOrder": 7}` 空操作更新就能读出整库凭据。两个写响应现在与列表共用唯一所有者 `service.RedactAccountSecrets`。**凭据发放面刻意保留回显**：`POST /api/accounts/login` 返回它刚用密码换来的会话令牌、`POST /api/accounts/verify-token` 返回它在上游发现的 API token——这两种情况调用方本来没有那个密钥；取回**已存**密钥仍然是显式动作（`GET /api/account-tokens/{id}/value`、下游密钥导出）。规则写进了 `docs/api/accounts.md`。

### 修复

- **流式请求的中断、截断与空内容不再记成功（#1159）**：此前流式路径只区分「idle 超时」与「其它一律正常结束」，内容级判定只写日志不参与记账，于是上游中途断连、被 `PROXY_MAX_STREAM_RESPONSE_BYTES` 截断、或返回命中 `PROXY_ERROR_KEYWORDS` 的错误体时，渠道健康度、`proxy_logs` 与终端指标全部按成功记账。现在流结束原因分五类（正常 / idle 超时 / 上游故障 / 被截断 / 客户端断开），状态、原因与指标 outcome 由同一个所有者决定。
- **恢复出厂设置真的恢复到出厂（#1165）**：`POST /api/settings/maintenance/factory-reset` 此前遍历一份手抄表清单，它已经比 schema 少 9 张表——其中包含 `admin_sessions`。会话校验每个请求都读这张表，所以它没被清空意味着**重置前签发的每一个 admin cookie 仍然对一个空库有写权限**；`admin_audit_logs`、`model_probe_results` 等同样幸存。现在表清单从 schema 注册表派生，唯一排除项是 additive 迁移日志 `schema_migrations`。
- **`cmd/migrate` 不再静默丢表（#1165）**：方言迁移（SQLite ↔ PostgreSQL）此前按另一份手抄清单拷贝，37 张表里只拷了 20 张——命令正常退出、checksum 全部匹配，但 17 张表的数据根本没被搬运。现在拷贝集、清空序、逐表列规格与建表序全部从同一个注册表派生，加一张表只需在注册表加一行。源库里存在而 Go schema 未声明的列现在会在拷贝前逐表打 Warning（丢弃不可避免，但不再静默）。
- **`checkin_interval_hours` 的越界值不再被静默丢弃（#1166）**：库内该键此前走「范围检查不通过就不赋值、不告警、还算已处理」，而环境变量 `CHECKIN_INTERVAL_HOURS` 走双侧钳制 ⇒ 库里存一个 `30`，进程留在环境变量的值，`GET /api/settings/runtime` 回显的也是那个值，三者互不相同。
- **`GET /api/channels` 的快照缓存此前几乎从不命中（#1167）**：缓存只有一个槽位，而键空间是多键的（仪表盘无界视图 + 每个分页/状态筛选视图各一键），于是 `page=1` 与 `page=2` 互相逐出，10s TTL 从来没吸收掉它存在是为了吸收的轮询，那条 fleet-wide 5-way JOIN 照跑。现在是有界多键缓存（至多 16 个键，FIFO 淘汰）；TTL、`?refresh=true` 绕过、`x-channels-snapshot-cache` 响应头、并发去重与「数据变了就整体失效」的语义一字未改。

### 变更

- **一个站点自定义请求头就能让 Metapi 读不懂上游答案，这个头不再可配（#1168）**：站点 `custom_headers` 里的 `Accept-Encoding` 会关掉 net/http 的透明解压，于是答案以压缩字节进入解析：用量提取找不到 token（**一次真实调用计零费**）、`PROXY_ERROR_KEYWORDS` 扫的是压缩噪声（**该失败的没失败**）、流式分析器读不到任何 `data:` 事件（开了 `PROXY_EMPTY_CONTENT_FAIL` 时一个健康的 200 流被记成 502「空内容」失败，**毒化渠道健康度**）。现在该头在装配侧就被过滤，`gzip` / `deflate` 先解码再判定与计费，响应由 Metapi 重构后不再带 `Content-Encoding`（零新依赖，标准库实现）。
- **解不了的编码诚实上报，不猜（#1168）**：`br`、`zstd`、多层编码栈、或在损坏/超大 body 上解码失败时，响应体连同它自己的 `Content-Encoding` **原样转发**（客户端仍能解），绝不解析、用量记为显式的 `unknown`（不凭空造 token），关键字规则与空内容规则都不在没人读过的字节上开火；每次这样处理都打一条稳定文案的 WARN 便于告警匹配。两节文档同步：`docs/configuration.md` 的「Upstream content encoding」、`docs/api/proxy.md` 的双向头策略。
- **夹在两次上游调用之间的等待现在可以被取消（#1169）**：签到的 transient 重试退避、同站点节奏等待，以及三个 OAuth onboard 轮询此前都不看 context——关机或每账号预算耗尽之后，worker 仍把整段等待睡完，**并且把睡完之后的那次上游调用照发**。等待时长语义未动，只加可取消性；context 同时贯通到这些轮询的上游请求本身。`POST /api/checkin/trigger` 是刻意的例外：手动触发跑在请求 context 上，调用方走了就停下剩余账号，已处理的每个账号保留自己的 `checkin_logs` 行。
- **Redis 共享计数器的补偿回滚不再每次上行脚本体（#1170）**：回滚挂在限流拒绝与失败补偿两条热路径上，此前每次都把整段 Lua 发给服务器重新解析。现在先 `EVALSHA`，仅在服务器答 `NOSCRIPT`（重启、`SCRIPT FLUSH`、切到副本）时回退一次 `EVAL`。原子语义（减量 + 非正数即删键自愈）完全不变；ACL 禁掉 scripting 的 Redis（`NOPERM`）降级为 `INCRBY`。
- **文档成为可对账的参考（#1160）**：环境变量清单与代码读取点逐条对账、路由清单与 router 注册逐条对账，两者都由门禁守着（漂移即 CI 红）；`.env.example`、`docs/configuration.md`、`docs/api/**` 与实现不一致的地方按实现纠正。

### 移除

- **`HOME_PAGE_CONTENT`（#1166）**：`.env.example` 与 `docs/configuration.md` 不再列这个键。它在 Go 实现里从来没有读取点，数据库侧的孪生键 `home_page_content` 与前端字段早已因「存了但渲染不出来」下线；留着它等于文档在承诺一个不存在的能力。设置了该变量的部署不会报错，但它本来就什么都没做。`SYSTEM_NAME` / `LOGO` / `FOOTER` / `ABOUT` 不受影响。

### 开发者可见

- **PostgreSQL 门禁套件在复用同一个库时可重复运行（#1164）**：新增 `internal/pgtest.Reset`，此前「第二次跑就假失败」的用例现在幂等。全套 PG 门禁必须以 `-count=1 -tags=integration -p 1` 运行（共享库），已写进文档与 CI。
- **端到端冒烟脚本的站点解析与后端去重键一致（#1162）**：`scripts/e2e/smoke.sh` 遇到 `POST /api/sites` 返回 409 时，按后端实际去重的 `(platform, url)` 找回已有站点，而不是按脚本自己的 `SITE_NAME`——此前站点名不同就会让后续 login / account / models / balance / checkin 全线跳过并报失败。

## [v0.16.23] — 2026-09-03

### Added

- **`USAGE_PROJECTION_INTERVAL_MS`（#1151）**：用量聚合的投影节奏（`proxy_logs` → 站点/模型汇总）此前硬编码 5s，现可调（钳制 1000–3600000 ms，默认不变）。小型单节点部署可调高，用仪表盘新鲜度换更少的扫描趟数。
- **启动日志说明日志清理体制归属（#1156）**：一条 `settings: log retention regime` 给出 `regime`（`log_cleanup` / `legacy_fallback`）、`configured`、`source`（`db_settings` / `env_toggle` / `none`）、两个 toggle 与 retention 天数。这个决定此前由一条静默推断做出，升级后运维无从知道现在谁在清日志。

### Changed

- **密钥准入贯穿请求上下文（#1152）**：`KeyAdmissionLimiter.Allow()` 此前自造 `context.Background()` 去跑共享计数器往返，因此**客户端已经断开**的请求仍会把整个计数器超时耗完，而这段时间它正持有该密钥的串行化互斥锁——同一密钥的其它请求全排在一次没人在等的往返后面。
- **`store.GetDB()` 改原子指针（#1151）**：活动库单例不再让每条请求路径争一把全局互斥锁；打开/迁移/切换序列仍在锁上串行。

### Fixed

- **老 PostgreSQL 库启动即失败（#1153）**：增量迁移注册表里三个 BOOLEAN 列写的是数字默认值（`sites.use_system_proxy`、`sites.post_refresh_probe_enabled`、`model_availability.is_manual`）。SQLite 把布尔存成 INTEGER、两种拼写都接受；PostgreSQL 对默认值表达式做类型检查，`ALTER TABLE … ADD COLUMN … BOOLEAN DEFAULT 0` 直接报 `column … is of type boolean but default expression is of type integer`（42804）。**全新库永远复现不了**：只有从旧版本升上来的库会走到那一步。
- **共享准入的补偿回滚会丢弃并发预占（#1154）**：Redis 回滚此前是两个往返（先减，若结果非正再 `DEL`），中间窗口里别的请求预占的计数会被一起删掉。现在单脚本原子完成。
- **管理界面保存的运行时设置重启后静默失效（#1156）**：写侧把键持久化进 settings 表，读侧却没有对应分支——实测 33 个键没有水合，本版补齐 27 个，另 6 个进带理由的白名单（`db_type` / `db_url` / `db_ssl` 在水合之前就被 bootstrap 消费，三个 `*_schedule_v2` 由迁移服务与排班端点自己读）。最贵的一条是 `admin_ip_allowlist`：**重启后控制面板的 IP 限制静默消失**。同一族还有：三个凭据键（`auth_token` / `proxy_token` / `account_credential_secret`）写侧 JSON 编码入库、读侧只 `TrimSpace` ⇒ 通过管理 API 轮换过的令牌重启后带着引号进快照、常量时间比较必然失配；日志清理设置水合侧读 `log_cleanup.enabled` 而写侧从来只写 `log_cleanup_enabled` ⇒ 整块是死码（统一到写侧拼写，点号保留为只读兼容别名）；日志清理体制判定（新 cleanup 调度器还是 legacy `PROXY_LOG_RETENTION_DAYS` pruner 拥有日志表）曾被 settings 表里**任意一行**（存过站点名也算）静默开启，把显式配置的 `LOG_CLEANUP_CRON` / `LOG_CLEANUP_RETENTION_DAYS` 顶掉；`system_name` / `logo` / `footer` / `about` / `server_address` 五个键此前丢弃空值 ⇒ 清空的站点品牌信息重启后复活（三个凭据键的空值守卫保留：空值不得覆盖已配置凭据）；`admin_ip_allowlist` 与 `proxy_error_keywords` 的写库错误此前被丢弃并返回 200 ⇒ 持久化失败被当成成功，现返回 500。
- **导入站点后账号列表不刷新、行内开关点了不即时翻转（#1155）**：导入 mutation 失效的是站点列表与账号快照，而 `/accounts` 表格读的是分页缓存 ⇒ 导入成功后表格纹丝不动；账号行的置顶/启停/签到乐观更新打在快照上而行数据来自分页缓存 ⇒ 点击后要等一次网络往返。同一个动作在 `/sites` 与 `/accounts` 给出两套答案。现改为失效查询工厂根 + 按分页前缀批量 patch。
- **两处界面文案渲染出裸 i18n 键（#1155）**：模型测试遇到 401/403（管理令牌过期是最常见情形）显示字面量 `modelTester.error.sessionExpired`，账号模型面板显示 `accounts.models.refreshFailed` / `manualFailed`——三个键在两个语言包里都不存在，而缺键时 i18next 返回键本身。

## [v0.16.22] — 2026-09-02

### Added

- **仪表盘四步旅程清单（#1148）**：站点 → 账号 → 路由 → 密钥的 checklist 取代「建好第一个站点就退役」的单步横幅，CTA 只挂在第一个缺口上（一次只引导一件事），四步全部建成后自我退役。
- **路由页 → 下游密钥交接条（#1148）**：已有路由但下游密钥数为 0 时就地提示怎么签发第一把密钥并用它调用 `/v1`；加载中、请求失败、已有密钥三种情况一律沉默，签发后永久消失（不是又一条常驻横幅）。密钥创建成功的 toast 带一键重开导出对话框的动作（该对话框可被关闭且受主令牌二次确认保护，关掉后此前只能回表格行里翻入口）；路由创建成功 toast 的 CTA 目的地改为一级页 `/downstream-keys`。
- **三个配额字段的语义说明（#1148）**：`maxRequests` / `maxCost` / `expiresAt` 补齐单位与行为——累计总量而非每分钟窗口（每分钟限速是另一组 `max_rpm` / `max_tpm`）、金额单位 USD 且仅成功请求计费、超限返回 429（`over_requests` / `over_cost`）、过期返回 403（`key_expired`）、留空或填 0 表示不限制。

### Changed

- **`/v1` 写超时不再掐断慢响应（#1145）**：`Server.WriteTimeout`（60s）在请求头读完时即武装，因此也覆盖了 handler 等待上游的时间，而缓冲派发的整体上限是 `max(90s, 首字节窗口 ×2)` ⇒ 在 61~90s 之间返回的非流式响应会在回写客户端时被掐断，表现为长请求「拿到结果却连接被重置」。现由专用中间件持有代理写期限。
- **客户端协议头透传（#1145）**：数据面重建上游请求时此前只带 `Content-Type` + 站点 `custom_headers` + 选中账号 token，客户端的协议开关全部静默丢失——`anthropic-version`、`anthropic-beta`、`openai-beta`、`user-agent` 与 `x-stainless-*`，Claude Code 的特性开关（prompt caching、fine-grained tool streaming）因此永远到不了上游。
- **上游响应头改内容语义白名单（#1145）**：缓冲响应此前全量拷贝上游响应头，把上游的身份与状态泄漏给下游——厂商指纹头（实测可见 `X-New-Api-Version`）、上游 `Set-Cookie`、以及覆盖 metapi 自己 `X-Request-Id` 的上游同名头（跨层日志无法关联，客户端报障时 id 对不上）。现只转发内容语义头。
- **Redis 共享限流不再全局串行（#1147）**：配置 `REDIS_URL` 后，密钥准入此前是「全局一把锁 + 每命令一次 TCP 握手」：`Allow()` 持全局互斥做共享 RPM/TPM 往返，计数器每条命令新建连接（dial + `AUTH` + `SELECT` + close）且完全忽略 `ctx` ⇒ 所有密钥互相排队，每请求还要在锁内付一次握手。现改为 64 条 cache-line 对齐分片 + 连接复用 + 贯穿 ctx。**语义零漂移**：Redis 出错仍 fail-open 回落本地窗口，原因码 `over_rpm` / `over_tpm`、`Retry-After`、`UsedRPM` / `UsedTPM`、键命名空间与全部 env 名均不变。
- **视频任务映射保留期默认 7 天（#1146）**：`PROXY_VIDEO_TASK_RETENTION_DAYS` 默认值由 `0`（= 保留期调度器整条禁用，`proxy_video_tasks` 无界增长）改为 `7`。`<=0` 仍是运维显式关闭，只是不再是默认。

### Fixed

- **数据面 SSRF dial 级纵深（#1145）**：站点批量导入与两条备份导入路径的目标校验收口之后，真正发起转发的 transport 仍是裸 `net.Dialer` ⇒ 校验通过后重新解析（DNS rebinding）仍可落到不同地址。现执行器、SSE 流 transport、两个兜底派发客户端与渠道健康探针 transport 统一挂 DNS 钉扎守卫。
- **视频任务进程内缓存无界增长（#1146）**：`publicId → 上游视频 id`（含 sticky 渠道/账号 pin）的重写缓存是包级 map 且无任何驱逐，长跑进程随累计视频流量线性泄漏内存。现按同一个保留期旋钮做 TTL，驱逐在插入路径摊销触发（每 256 次插入或每 5 分钟至多一次 sweep），无后台 goroutine。
- **下游密钥的过期时间此前完全不生效（#1148）**：表单出站的是裸 `datetime-local` 串（形如 `2030-01-02T03:04`，无秒无时区），写入归一与鉴权两处解析器都不认，而不可解析的过期时间会**跳过整个过期检查**——实测一把 2020 年就该过期的密钥仍能以 200 调用 `/v1/models`，同一时刻的 RFC3339 形式正确返回 403。

### Docs

- `docs/api/proxy.md` 新增「Header policy (/v1 surface)」（双向头策略、优先级次序、凭据头永不透传）与「Timeouts (/v1 surface)」；`docs/api/conventions.md` 新增「Site upstream SSRF hardening (proxy data plane)」（URL 层 + dial 层两道守卫、拒绝段与刻意放行段）。

## [v0.16.21] — 2026-09-02

### Added

- **接管库时间戳归一化（#1142）**：启动迁移自动把 TS 时代写在 TEXT `*_at` / `*_until` 列里的 `'YYYY-MM-DD HH:MM:SS'` 值重写为 RFC3339 UTC。接管库两种格式混排时所有字典序比较失真（空格 0x20 排在 `T` 0x54 前）：`ORDER BY created_at` 列表错排、范围过滤边界失真、checkin 清扫把全部行当成同一时刻。
- **`/v1` 错误契约文档（#1140）**：`docs/api/proxy.md` 新增「Error shape (/v1 surface)」——OpenAI 信封 + 完整 status/type/code 对照表；admin 面（`/api/*`）平铺契约不变，两者不混用。

### Changed

- **`/v1` 鉴权与限流错误对齐 OpenAI 信封（#1140）**：中间件裸 `{"error":"..."}` 统一为 `{"error":{message,type,code,request_id}}`；invalid key `403` → `401 authentication_error/invalid_api_key`（SDK 把 403 视为不可重试的权限错误、跳过换 key 路径），配额耗尽 `403` → `429 insufficient_quota`。
- **Go 死码清扫 −1814 行（#1137）**：oauth import 族（`/api/oauth/import` 保持文档化 UI-parity 桩）、quota `Record*` 族及其私有链、routing `PricingReference` 与四端口接口、`ClearChannelFailureState` 级联、`SendDailySummary`、`BatchUpdateTokenStatus`、`ApplyStreamPreference` 等 14 项，每项经全仓生产 + 测试双零引用验证。
- **web 一次性探针清理 −1900 行（#1138）**：`web/scripts/oneoff/` 12 个零引用脚本整目录删除；5 处手写 `navigator.clipboard.writeText`（其中 3 处无 SSR/能力兜底）收敛为共享 `copyText()`。
- **SSE 默认流上限 1MB → 64MB（#1141）**：`PROXY_MAX_STREAM_RESPONSE_BYTES` 默认值上调——推理模型长输出与大型代码生成常态超 1MB，此前被中途截断且自定义错误事件多被 SDK 静默吞掉，表现为「回答戛然而止」。上限保留为失控护栏。
- **SSE 响应补 `X-Accel-Buffering: no`（#1141）**：nginx 系反代不再缓冲首 token。

### Fixed

- **SSRF 目标校验缺口（安全，#1139）**：站点批量导入（`POST /api/sites/import`）、原生备份导入、TS v2.1 备份导入三条路径此前完全绕过 `IsForbiddenSiteTargetURL` ⇒ 构造备份即可植入 `url=http://169.254.169.254/…` 的站点，配合下游密钥一次 `/v1` 请求穿透数据面回流云 metadata / IAM 内容。现三路径统一走同一个净化器。
- **`/v1/pricing` 双前缀路由 bug（#1141）**：在 `/v1` 组内注册绝对路径，文档承诺的 `GET /v1/pricing` 一直 404，真实可达路径是 `/v1/v1/pricing`（未鉴权探测看不到——组中间件先答 401 掩盖了它）。
- **`catalogsync.CreateSource` 在 PostgreSQL 必失败（#1141）**：pgx stdlib 驱动不实现 `LastInsertId`，PG 部署下新增模型目录源恒报错（SQLite-only 测试漏网）。补 `RETURNING id` 方言分支。
- **审计日志路径过滤 PG 大小写失配（#1141）**：`path LIKE ?` 在 SQLite 为 ASCII 大小写不敏感、PG 敏感，双侧 `LOWER()` 对齐（全仓 LIKE 过滤同款模式）。

## [v0.16.20] — 2026-09-01

### Added

- **运行事件结构化（checkin 事件族先行）**：`service/events` 类型化注册表（注册防重复、参数严格校验），`WriteEvent` 在持久化历史英文 title/message（非 UI 消费者字节级零破坏：notify / CSV / 历史行）之外新增 `title_key` + `params` JSON。
- **删除 + undo（#1097）**：叶子实体单行删除（模型重定向、目录源、令牌路由、下游 API 密钥、账号令牌）不再弹确认框——行即消失并给出 6 秒「撤销」窗口，真实删除仅在窗口关闭后发生，撤销精确恢复且服务端从未写入。
- **下游密钥凭证树形选择器（#1026、#1072）**：`allowedCredentialRefs` / `excludedCredentialRefs` 配置 UI，按站点 → 账号 → 密钥三级树勾选，与 API 契约一致。
- **命令面板动作层（#1073）**：Ctrl/⌘+K 面板在页面与实体导航之外支持直接执行动作。
- **误触防护与移动端可达性（#1080）**：恢复出厂设置改 type-to-confirm（需输入 `RESET` 且倒计时结束）；≤640px 全宽抽屉新增拇指可达的底部关闭条（自带取消/提交的表单抽屉除外，避免同义双出口）。
- **无障碍与契约测试补齐（#1087、#1026）**：仪表盘延迟直方图与趋势补 sr-only 数据表（至此六个主图全部有屏幕阅读器替代层）；i18n 双语键的 `{{变量}}` 集合必须一致（此前键集合 parity 抓不到译文丢失插值变量导致的半翻译渲染）；路由选择器的凭证维度契约测试（两种 kind、跨 kind 不互匹配、空列表不限制、排除优先于允许、悬空引用失败关闭）。

### Changed

- **管理端大表服务端分页（#1075、#1077）**：账号、模型市场、渠道（含状态过滤）列表改为服务端分页；`GET /api/channels` 新增 `status` 过滤参数，新增 `GET /api/channels/error-summary` 聚合端点（渠道失败横幅一键过滤的数据源）。
- **账号页筛选服务端化（#1122，issue #1108）**：`GET /api/accounts` 新增 `q` / `status` / `site` 筛选参数——此前分页是服务端、筛选却是客户端（只过滤已加载页），站点不在当前页时永远筛不出来。现在筛选覆盖全舰队，`total` 是筛选后计数，非法值显式 400。同一批还修了两处：所有 URL 同步的列表页「重置」此前只清搜索框（两次状态更新被同 tick 合并、第一次静默丢弃），现改为串行化；账号页站点单元格支持安全外链快捷跳转。
- **统一 400 错误体携带机器可读 `errorCode`（#1065）**：需要客户端分支处理的失败类别逐步登记错误码（认证与会话 8 码、资源不存在 5 码、能力未实现/禁用 3 码、设置校验 1 码、读取路径加载失败族 1 码覆盖 72 个调用点，见 `docs/api/conventions.md`）。
- **凭证引用畸形条目改为显式 400 拒绝（#1026）**：创建/更新下游密钥时，`excludedCredentialRefs` / `allowedCredentialRefs` 中的畸形条目（非对象、未知或缺失 `kind`、非正 `siteId` / `accountId`、`account_token` 缺 `tokenId`）原被静默丢弃——对允许列表而言等于静默放宽策略。
- **事件标题 i18n 单一映射（#1099）**：程序日志页与 attention 管线此前各带一份事件标题映射表和 locale 节（漂移源），7 个高频生产者标题归一。
- **`docs/api.md` 按域拆分**：超预算的 API 参考拆为 `docs/api/*.md`（17 个文件），`docs/api.md` 保留为索引，原有标题以 stub 承接并指向新家，旧 `api.md#<anchor>` 深链继续解析。
- **前端结构收敛（#1084、#1095）**：settings / dashboard / observability 三份 `createSectionRegistry` 克隆合一；8 个列表页各自手写的加载失败横幅 + 重试收敛进表格底座的内置契约。行为均不变。
- **界面形态与可分享视图（#1080、#1082）**：站点表单从居中弹窗改为右侧抽屉（与账号表单统一，长表单滚动时底部动作常驻可达）；价格对比与站点公告的筛选与页码写入 URL（刷新、后退/前进、分享链接均保持视图）。
- **文案与行高校对（#1123、#1124）**：路由策略「基础权重加数」→「基础权重基数」、「管理员审计日志」→「审计日志」（7 字标题在窄侧边栏截断）；运行事件标题列的受影响路由/替代站点/面板深链折叠进每行「详情」开关，默认行高从约 5 行回到可扫读的 3 行。

### Fixed

- **下游密钥凭证维度端到端修复（#1026）**：`auth.ExcludedCredentialRef` 的 JSON 标签原为 snake_case，而管理端持久化形状是 camelCase ⇒ 代理路径解析不出运营刚配好的排除项。
- **运行时设置的并发撕裂读（#1079）**：`RuntimeSettings` 迁移为不可变快照交换——此前约 25 个运行时写字段与热路径无锁读并存，并发下可能读到半更新状态（如代理 token 校验瞬时 401 抖动）。
- **通知铃铛弹层告警文案未本地化（#1091）**：后端 attention API 为兼容保留英文 `label` 并下发结构化 `params`，可用性面板已再本地化，但顶栏铃铛弹层裸渲染英文 ⇒ 中文界面下出现英文告警。两处现共用同一份再本地化。
- **前端审计修复批（#1080、#1086、#1088、#1101、#1102、#1104）**：HTTP 500 无 body 时双弹 toast、502–504 泄漏 axios 原始英文报错改为状态感知的本地化文案；列表页加载失败改为整块替换 + 内置重试（不再叠加在陈旧数据上）；浅色主题图表与焦点环对比度达 WCAG AA；页面双 `<main>` landmark、品牌 logo 冗余 `alt`、空表头；设置页单卡片节标题与描述逐字重复；代理日志详情行同日重复渲染绝对日期与相对时间；移动端账号页头动作挤压；今日快照 delta 不可用态渲染成「— —」。
- **channels 页「响应延迟」列默认掉出首屏**：表格默认总宽超出 1440px 视口下的滚动口，该列表头被裁成「响应延…」；列宽修正后回到首屏。

### Security

- **SPA CSP 去 `style-src 'unsafe-inline'`**：收紧为 `'self' 'nonce-<per-request>' 'sha256-<toast-css>'`，其余 directive 不变；Go SPA fallback 每个响应生成 16 字节随机 nonce 并注入 `<meta name="csp-nonce">`。
- **凭证导出遮罩改真占位符（#1080）**：遮罩态密钥不再以 CSS blur 伪掩码渲染（明文此前仍留在 DOM 与无障碍树中，读屏可读出），改为 `••••••••` 占位符 + `aria-hidden`。

## [v0.16.19] — 2026-08-29

### Security

- **管理员会话模型重构（#1034、#1057）**：主 token 不再以明文存于浏览器 localStorage（原 12h 窗口）——登录改为 `POST /api/auth/login` 以主 token 换取服务端会话（`admin_sessions` 表），凭证经 HttpOnly + SameSite=Strict 的 `metapi_session` cookie 携带，数据库只存 SHA-256 哈希，会话滑动续期。
- **WebSocket 一次性 ticket（#1034、#1057）**：实时运维 WS 不再接受 `?token=<主 token>` 查询参数，改由会话认证的 `POST /api/auth/ws-ticket` 签发 60s 单次 ticket——主 token 从此不进 URL（访问日志、代理日志、浏览器历史）。
- **失败认证纳入限速（#1034、#1057）**：per-IP 限速中间件移至认证之前，401/403 不再绕过桶约束；`/api/auth/*` 另加严格桶（`AUTH_RATE_LIMIT_RPS` / `AUTH_RATE_LIMIT_BURST`，默认 10/20）。
- **敏感操作主 token 重确认（#1034、#1057）**：备份导出（下载与 WebDAV）、下游密钥导出、主 token 轮换要求 `X-Admin-Confirm-Token` 头重出示主 token（即使持有活跃会话），否则 403 `reauthRequired`；凭证导出对话框默认锁定遮罩，密钥默认遮罩显示，深链打开前显式确认。**这些操作从此需要多带一个头，自动化脚本要跟着改。**

### Added

- **会话配置项（#1034、#1057）**：`ADMIN_SESSION_TTL_MINUTES`（默认 720）、`ADMIN_SESSION_COOKIE_SECURE`（默认 auto，按请求协议自适应）、`AUTH_RATE_LIMIT_RPS` / `AUTH_RATE_LIMIT_BURST`（默认 10/20）。均带安全默认值，零配置即安全；`cmd/migrate` 方言迁移携带 `admin_sessions`，会话跨 SQLite ↔ PostgreSQL 切换存活。
- **上游账号健康监测全局开关（#1027、#1056）**：运行时设置 `checkinEnabled` / `balanceRefreshEnabled`（热生效、持久化）+ 环境变量 `CHECKIN_ENABLED` / `BALANCE_REFRESH_ENABLED`；模型可用性探测改运行时热停。
- **账号代理支持 SOCKS5（#1009、#1059）**：账号 `proxyUrl` 接受 `socks5://` 与 `socks5h://`。

### Fixed

- **清空账号代理字段保存即清除（#1009、#1059）**：修复前端清空后载荷省略 + 后端 `MergeExtraConfig` typed-nil 删除缺陷双因，空值显式清除；账号更新路径补 scheme 校验。
- **调度器健壮性（#1061）**：job panic 统一 recover 边界；in-flight 标志锁外读竞态修复；`channel_recovery` / `backup_webdav` / `model_probe` 等吞没的 DB 错误经结构化日志显性化。
- **路由健康懒加载数据竞态（#1052）**：快速路径无锁读改 `RLock` 读，行为零变化。
- **PostgreSQL 方言陷阱清扫（#1060）**：BOOLEAN 列的整数字面量绑定重写（42804 / 22P02 类），管理搜索 LIKE 大小写 `LOWER()` 统一，静态方言门禁扩至全包；零迁移。

### Changed

- **构建配置收敛（#1053）**：`rsbuild.config.ts` 成为唯一构建配置（删 `vite.config.ts`），devProxy 与版本 define 收口共享模块，route-tree 前置校验；匿名大块拆为 `vendor-i18n` / `vendor-icons` / `vendor-core`。
- **管理读路径索引（#1054）**：三个高频读路径加索引（300k 行实测：渠道日志按 channelId 过滤 17.9ms → 0.5ms、siteId 聚合约 10x、区间聚合约 3x）；proxy-logs `LEFT` → `INNER`、marketplace N+1 批量化；另 4 个候选索引实测无收益，不加。
- **出站 HTTP 客户端基线（#1058）**：15 处出站调用点统一 `internal/httpclient`（dial 30s / TLS 10s / idle 90s / 池 100-20），AST 静态门禁拦截裸客户端。
- **UX 残留清扫（#1055）**：focus-ring 全库统一 token（≥3:1）；流量与成本图补 sr-only 数据摘要表。

## [v0.16.18] — 2026-08-29

### Added

- **SSE 流 chunk 间隔空闲超时（#1046）**：新增 `PROXY_STREAM_IDLE_TIMEOUT_SEC`（默认 300）——每转发一个 chunk 重置计时窗，窗口内无新 chunk 即中断卡死的流并按上游超时故障记录（渠道健康、失败日志、终态一致）。只约束 chunk 间隔、不约束流总时长；`0`、负数、非法值回退默认。
- **重试/禁用状态码策略运营者可调（#1049）**：新增运行时设置 `proxyRetryStatusRanges` / `proxyDisableStatusRanges`（设置页可编辑，`PUT /api/settings/runtime` 同契约）：retry 决定哪些上游状态码计为可重试的渠道故障，disable 决定哪些状态码在冷却升级之外直接禁用故障渠道（`enabled=false` + 人工覆盖标记）。
- **批量测试闭环（#1047）**：模型测试台批量对比出现失败行时提供「禁用失败渠道」动作（确认对话框列明数量与人工覆盖后果），调用升级后的 `PUT /api/channels/batch`——部分更新语义（只写字段出现的）、载荷校验（空批 / 超 1000 / 重复 id / 无可更新字段）、逐项真值返回。
- **渠道失败横幅一键过滤（#1048）**：渠道页存在冷却或熔断渠道时显示失败横幅（人工停用属运营意图、不计入），「只看失败」经可分享的 `?status=` 参数进入仅失败视图。
- **下游密钥上游站点限制 UI（#1026）**：创建/编辑表单新增「上游站点限制」多选（留空 = 不限制），把路由选择器既有的 `allowedSiteIds` 暴露到管理界面并完整往返。
- **搜索面板实体深链（#1039）**：CmdK 面板的站点/账号/令牌/模型命中一键深链到实体本身，参数一次性消费后清除，深链可分享。

### Fixed

- **路由自动重建真值（#1024）**：`POST /api/routes/rebuild` 现在消费 `refreshModels`（默认 true：先批量刷新全部活跃账号的上游模型、单账号失败不中断整批，再重建通道）；完成提示读取真实统计并区分三态——成功（真实路由/通道数）、无路由可重建（引导先创建路由）、通道无变化（引导核对账号模型）。
- **zh-CN 语言下站点表崩溃（#1036）**：`createdAt` 列对无效 Intl locale 的防护回退。

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
