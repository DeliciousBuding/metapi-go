# log.md — Metapi Go product milestones

**Last updated**: 2026-09-03 (v0.17.1)

> Product milestone timeline: the current month is kept per release, closed months are one-line summaries
> (date · title · versions/PRs) so the file stays scannable. Not the current-state source of truth.
> Current state → [`STATE.md`](STATE.md) · open items → [`progress/MASTER.md`](progress/MASTER.md) · detailed version narrative → root [`CHANGELOG.md`](../../CHANGELOG.md)

## 2026-09-03 — v0.17.1 切版（备份表集注册表化 · 设置水合不再毁值 · **CI 竞态门复活** · 启动日志诚实性与前端布局门禁）

- **版本判断**：patch-first。v0.17.0 打 tag 之后又合入三个运维可见修复（#1172 备份导出缺 5 张表、#1173 一条坏设置行清空已配置值、#1178 每次启动一条假 WARN）与一个 CI 完整性修复（#1175），按 `git-workflow.md` §6.1「每波合入 master 且含用户可见变更 → 立即 bump 最后一位」走 v0.17.1；不构成新里程碑，故不动中间位。
- **备份表集（#1172）**：`service/backup.AllTables` 是**第三份**手抄清单，28 项 vs 注册表 37 张表 ⇒ `type=all` 导出静默丢 9 张，其中 5 张是用户可见状态（产品横幅 / 公告「已忽略」/ 手工模型改名 / 余额趋势 / 批量校验历史），而导入端回 200 报成功。改为 `store.BackupTableNames()` = 注册表 − 4 项**显式**排除（`admin_sessions` / `admin_audit_logs` / `model_probe_results` / `catalog_sources`，每项一句运维可读的理由），`AccountsTables` 从「第二份排序清单」改成「scope 集合 ∩ 注册表拓扑序」，导出 `metadata.excluded_tables` 让**备份文件自己陈述自己的缺口**，点名导入被排除表 → 400（此前静默忽略）。`catalog_sources` 是**待解除**的排除：导入 URL 闸 `service/import_url_guard.go` 的 `importURLColumns` 今天只覆盖 `sites` / `site_api_endpoints`，扩到 `catalog_sources.url` 之后删掉这一条即自动纳入（否则半可信备份可被植入 cloud-metadata / link-local 抓取目标）。顺带更正 #1165 写进 `docs/api/settings.md` 的一句假话——「需要就先导出备份」保不住审计与探测历史，那句话在写下的时候就是假的。
- **CI 抓出第四份手抄清单**：`e2e/e2e_backup_test.go` 断言 `len(tables) != 28`，注册表化之后导出是 33 张 ⇒ `test-pg` 红。改成向 `store.BackupTableNames()` 取，并逐表断言「存在且为空数组」；同文件所有 "all 28 tables" 的散文改成计数驱动（那个种子 map 其实只有 **27** 项，散文一直是错的）。变异探针：把 `service/backup.AllTables` 截到 32 项（模拟导出偏离注册表）→ 本用例红并报出被丢的表名 `model_name_redirects`，而 `TestBackupExportImportRoundtrip` **仍绿**（该表不在它的种子集里）——这正是空库用例必须向注册表取、而不能靠种子集兜住的理由。
- **设置水合不再毁值（#1173）**：`ApplyRuntimeSettings` 有两条分支把 `config.ParseJsonValue` 的结果**直接赋值**，而它用 `nil` 编码失败 ⇒ 库里一条读不出来的 `payload_rules` / `openai_service_tier_rules` 行会在下次重启时清空已配置的规则集，零日志；另两条（`checkin_schedule_mode` / `notify_task_toggles`）丢弃坏行时不告警 ⇒ 库里的行与 `GET /api/settings/runtime` 长期互相矛盾。第一性原理：**水合的职责是把库里存的意图还原成运行时值；「读不出来」意味着意图不可恢复，不等于「运维想要空」**。改为读得懂才赋值、读不懂保留既有值 + WARN（只描述形状、不回显整块 blob）；显式 `null` / `[]` 仍按「清空」生效（UI 清空 textarea 走这条，且 `upsertSettingDB` 把 nil body 值归一成 `[]` 落库，这条路径有专门用例）。`checkin_schedule_mode` 的保留是必须而非偷懒：`config/validate.go` 把未知 mode 判为 critical（启动即退出），把坏行灌进快照等于把一条坏数据升级成启动失败。可达性已核实（非纯理论）：备份导入只校验单元格标量类型与字节上限、不校验语义，这 4 个键都不在 `RuntimeLocalSettingKeys` 跳过集里且插入是 `ON CONFLICT DO NOTHING` ⇒ 导入可**新种**坏行；`notify_task_toggles` 的 admin 写路径只 marshal 不校验形状 ⇒ 直连 API 发 `{"notifyTaskToggles":"all"}` 得 200 并落库、随后再水合失败不更新 runtime，**不需要手改表就能复现**。两条机械门禁：R4（水合 switch 内每个值解析器必须登记并写明「它的失败模式为何不能毁掉已配置值」，登记项失效也红；`config.ParseJsonValue` 被**显式断言永不可登记**）与 R5（`json.Unmarshal` 必须在调用点检查 err）。
- **CI 竞态门复活（#1175，本轮最重要的发现）**：`test-sqlite-shard` 的分片算术写成 `if (( i % matrix.total == matrix.shard ))` —— 缺 `${{ }}`。bash 对每个包报一次语法错误，但那个 `(( ))` 位于 `if` 条件中、而 **`set -e` 对条件命令不生效** ⇒ 错误被吞、`shard_pkgs` 恒空、走进 `no packages in shard; nothing to run` 分支 `exit 0`。**四个分片与聚合出的必选检查 `test-sqlite` 全绿，执行的测试数量是 0**；`git tag --contains 06025d53`（引入分片矩阵的提交）实测 = **30 个 tag，v0.14.0 → v0.17.0**。该窗口里唯一真在执行 Go 套件的是不分片的 `test-pg`，而它不带 `-race` ⇒ **竞态检测三周内从 CI 完全消失**；仓库自带的 `scripts/go-race.sh`（全量不分片）本身是好的，但它只挂在 pre-push hook 上，而本机约定 `--no-verify` 推送 ⇒ 同样没跑。**发现路径值得记下来**：#1172 的 `test-pg` 红了、四个 sqlite 分片却全绿，而失败用例 `TestBackupEmptyDatabaseRoundtrip` 用的是 `store.Open(DialectSQLite, ":memory:")` —— 一个纯 SQLite 用例只可能在 sqlite 分片里失败，它却没有；这个矛盾直接指向「分片根本没跑」，而不是「用例只在 PG 下坏」。
- **修复与验证（#1175）**：matrix 两值改经 `env` 绑定（配合 `set -u`，将来谁再弄丢 `${{ }}` 报的是 `unbound variable`）· 轮转槽位改成独立的 `$(( ))` 赋值而**不再是 `if (( ... ))` 条件**（畸形选择器直接中止 step）· 断言本片应得的包数 `ceil((N - S) / T)`，**选少了和选不到一样红** · 空分片从「一次免费通过」改成 `exit 1`。真实 49 个包上实测四片选到 13 / 12 / 12 / 12，并集 == 全量、无重复无遗漏；四条守卫逐一挑发全部非零退出（其中一条就是把 `SHARD_TOTAL` 设成原始故障字面量 `matrix.total`）；把修复前的脚本对同一份包列表回放，仍打印 `no packages in shard` 并 `exit 0`，静默绿被原地复现。复活后四分片的 `Run tests` 步实测 **57s / 82s / 90s / 440s**（修复前整个 job 21s、测试步不到 1s），shard 0 的 440s 正是 `handler/admin` 在 `-race` 下的已知长杆（`docs/testing.md` 记的 217–364s 区间）。合并前另做一次全量 `go test ./... -count=1 -race`：**43 个含测试的包全绿、0 失败、0 竞态**——三周的竞态欠账底下没有藏东西。不变量、为什么槽位计算不能放在 `if` 条件里、以及受影响的发布区间都写进了 `docs/testing.md`，免得守卫被后人当仪式删掉。
- **启动日志诚实性 + 前端布局门禁（#1178）**：`setupSPAFallback` 挂着两个静态子树，其中 `/assets/*` 的注释声称「为仍然产出 `dist/assets` 的旧构建保留兼容」——而前端是 `//go:embed dist` 编译期烘进二进制的，一个二进制只有一份由本提交构建的 dist，**「更旧的构建」在构造上不存在**。内嵌树自 Rsbuild 迁移起无 `assets/` 目录 ⇒ 挂载永不可达、守卫在**每个部署每次启动**打一条 `WARN embedded web/dist subtree not readable, serving disabled dir=assets`（在已发布的 v0.17.0 二进制上实测复现；本仓测试床换进该产物启动即出现）。无条件 WARN 比没有更糟：它训练运维忽略该级别，且占用**真 `/static` 故障会用的同一句话**。删挂载，改由 `router/spa_layout_test.go` 从真正要紧的一侧钉契约：`index.html` 里每条 root-relative 引用必须回 200 **且不得以 `text/html` 回来**（只看状态码不构成契约——SPA fallback 对每个未挂载路径都答 `200 text/html`，这正是 `730bb9c2` 当初把白屏 UI 藏在绿色状态码后面的方式）· 挂载本提交交付的 dist 必须 0 条 WARN · **「零引用」判失败而非通过，这道门不许空转**（CI 的 embed 占位物只有 `placeholder.txt`、无 `index.html`，带明确理由 skip；两者都无则 fail 不 skip）。变异探针两发两中：把 `/assets/*` 加回去 → 静音门红（错误信息就是线上那句原文）；把 `/static/*` 摘掉 → 引用门对全部 11 条 `/static/...` 报红 `served as text/html; charset=utf-8`，**历史白屏缺陷被按需复现**（状态码全是 200，只有 content-type 出卖它）。
- **发布产物活体复验（v0.17.0，非本地重建）**：把已 publish 的 `metapi-linux-amd64`（sha256 对过 `checksums.txt`）换进真实上游测试床，带鉴权核对 `/api/about` = `version v0.17.0` / `commit ec1043b4…`（**正是 tag 指向的提交**），随后 new-api 链 13 PASS / 1 WARN / 0 FAIL · sub2api 链 11 PASS / 2 WARN / 0 FAIL · axonhub `AXONHUB_VERIFY_PASS` · gpt-load PASS=13 FAIL=0 · F5 双语视觉 13/13 · 五服务全 RUNNING。并对该产物 `index.html` 引用的 14 条路径逐条 curl：13 条 content-type 正确、主包 `index.7340ee69cd.js` 200 / `text/javascript` / 141242 B ⇒ **发布产物的 SPA 健康**，#1178 修的是日志诚实性与防回归门禁，不是白屏。
- **私有层复验脚本自身的两处「验证了自己没验证的东西」（同日修，不进公开仓）**：① `make build` **不重建前端**（Makefile 无前端目标），于是脚本把磁盘上任意龄期的 gitignored `web/dist` 烘进二进制 ⇒ 此前几次「合并后复验」的 F5 视觉步骤验的是**旧 UI**；现打印 dist 溯源（mtime / 主包名 / `index.html` sha256 前 16 位）并在源文件比 dist 新时 WARNING 点名，另加 `REBUILD_WEB=1`（实测 ~15s、峰值 used ~1.2GB）与 `RELEASE_BINARY=<path>`（改为验证已发布产物）两个模式。**龄期只能用 mtime 判不能用内容哈希判**：同一份源码在本机两次构建得到 `index.bfd4430f47.js` / `index.669a83d73f.js`，CI 发布产物是 `index.7340ee69cd.js`，rsbuild 哈希跨构建主机不可复现。② `GET /api/about` **需要 admin bearer**，脚本此前裸调、把 `{"error":"Missing Authorization header"}` 打印出来就往下走 ⇒ 唯一一行用来证明「当前跑的是哪个构建」的输出从来没证明过任何事；现改为带 token 请求并与被测物对账（`RELEASE_BINARY` 比 `version`，本地构建比 `commit` vs `git rev-parse HEAD`），不一致即 `fail=1`。**这与 #1175 是同一类故障：一个不检查自己前提的检查。**
- **仓库卫生**：合并后 worktree 清零、本地分支只剩 `master`、**远端只剩 `origin/master`**、开放 PR 清零、工作树零 untracked、无 stash（`git fetch --prune` 之前本地有一批陈旧的 remote-tracking 引用，prune 后消失；远端本身早已只剩 master）。
- **已知遗留（进下一波）**：`PayloadRules` 在 Go 侧无任何运行时消费者、`OpenAiServiceTierRules` 零读者（连回显都没有），但 `docs/configuration.md:184` 把它们写成 "Per-model payload rewrite rules" —— **文档超前于实现**（与 #1166 删掉的 `HOME_PAGE_CONTENT` 同一类，只是这次有个 UI 在读写它），待裁决「改文档说实情」还是「落地规则引擎」 · `notify_task_toggles` 写路径应返 400 · `catalog_sources` 的导入 URL 闸扩展 · PostgreSQL 导入不重置序列（`setval` / `sqlite_sequence` 在 `importBackupTablesWithConn` 缺位）· 约 20 个不可解析**数值**分支「静默保留 fallback」，建议在解析器失败路径做**一次聚合 WARN** 而非逐分支加 · 4 条数值键 floor 偏差（`port` / `notify_cooldown_sec` 可为负绕过 env 的 0 下限 / `proxy_max_channel_attempts` 比 env 严 / `smtp_port` 已一致）· `proxy_retry_status_ranges` 与 `proxy_disable_status_ranges` 把不可解析原文透传给消费者 · `bun install` 的 registry 完整性抖动建议加重试 · CI 增 `actionlint`（其内置 shellcheck 很可能在引入当时就报出这条算术错误）作为这一整类故障的持久守卫。

## 2026-09-03 — v0.17.0 发版（数据面与存储诚实性波：转发链身份 · 凭据回显 · 表清单注册表 · 上游编码 · 可取消等待）

- **版本判断**：按 patch-first 节奏本可以是 v0.16.24，但这一波改了三个运维可见契约（恢复出厂的清库范围、账号写响应的凭据字段、流式失败的记账口径）外加一处身份解析语义（转发链），够 `docs/internal/git-workflow.md` §6.1 的「成体系的交付波」，故 bump 中间位并把最后一位归零。收口 #1159–#1170 共 12 个 PR。
- **转发链身份（#1161）**：`TRUSTED_PROXY_CIDRS` 生效时客户端 IP 取的是 `X-Forwarded-For` 最左值，而主流反代是追加语义 ⇒ 最左那段是调用方自己塞的。三个消费者全部读改写后的 `RemoteAddr`：admin IP 白名单一个伪造头即过（#1034 的纵深防御归零）、每 IP 限流可每请求换一个假 IP 无限换桶（登录暴破上限失效且限流器自身看不出异常）、`admin_audit_logs.remote_ip` 与会话 IP 绑定记下的是编造的地址。改为「所有转发头按序拼链 + 补直接 peer + 从右往左跳过可信 CIDR + 返回第一个不可信地址」，全链可信时返回最左（否则内网调用方塌缩进同一个限流桶）；外层「peer 必须可信」这道门与 `RemoteAddr` 形状未动，默认空配置行为不变——这正是它长期没被发现的原因。
- **表清单注册表（#1165）**：仓库同时存在三份手抄表清单（`AutoMigrate` 建表序 20 项、`cmd/migrate` 拷贝集 28 项、恢复出厂删除序 28 项），而 schema 有 37 张表。后果是实测的：方言迁移**静默丢 17 张表**（命令正常退出、checksum 全对）；恢复出厂**漏 9 张**，其中 `admin_sessions` 意味着重置前签发的每个 admin cookie 仍对一个空库有写权限（会话校验每请求读该表，清空它才是真的吊销）。收敛为单一注册表 `store/tablesets.go`，五个用途（建表序 / 拷贝集 / FK-safe 清空序 / 恢复出厂集 / 逐表列规格）全部派生；顺带修掉序列同步的硬编码排除、未拷源列的静默丢弃（现在逐表 Warning）、CLI 迁移缺的 `normalize` 腿、boolean 默认值文本在 fresh 与 converged schema 之间的不一致。`sc2_029` 从注册表步骤改为每启动无条件幂等清扫——journal 门控在「数据后到」的场景下必然错。
- **凭据回显（#1163）**：读路径早就把 `accessToken`/`apiToken` 换成掩码并从 `extraConfig` 剥掉 `autoRelogin.passwordCipher`，而 `PUT /api/accounts/{id}` 与 `POST /api/accounts/{id}/rebind-session` 原样返回明文 ⇒ 一次 `{"sortOrder":7}` 空操作更新就能读出整库凭据，那层脱敏只值一次空操作。两个写响应改走与列表同一份策略（导出为 `service.RedactAccountSecrets`，账号面凭据策略的唯一所有者）。发放面刻意保留回显（login 返回刚换来的会话令牌、verify-token 返回在上游发现的 API token——调用方本来没有那个密钥），规则写进 `docs/api/accounts.md`。
- **数据面诚实性（#1159、#1168）**：流式路径此前只区分「idle 超时」与「其它一律正常结束」，内容级判定只写日志不参与记账 ⇒ 上游中途断连、被 `PROXY_MAX_STREAM_RESPONSE_BYTES` 截断、或返回命中 `PROXY_ERROR_KEYWORDS` 的错误体时，渠道健康度 / `proxy_logs` / 终端指标全部按成功记账；现在结束原因分五类、状态与 reason 与指标 outcome 同源，内容级判定收口成一个纯函数（buffered 与 streaming 喂同一份事实），客户端主动断开仍按成功记账但不与干净 EOF 混为一类。#1168 补上编码这一层：站点 `custom_headers` 里的一个 `Accept-Encoding` 就能关掉 net/http 的透明解压，于是用量记 0（漏账）、关键字扫压缩噪声（假成功）、SSE 分析器读不到 `data:` 事件 ⇒ 开了 `PROXY_EMPTY_CONTENT_FAIL` 时健康的 200 流被记成 502 空内容失败（假失败，毒化渠道健康度）。现在 `gzip`/`deflate` 解码后再判定与计费（零新依赖），解不了的（`br`/`zstd`/多层栈/解码失败）原样转发 + 用量记 `unknown` + 判官直接 pass + 稳定文案 WARN；该头在装配侧与出站各剥一次，两份过滤清单的安全关键交集由跨包门禁钉住。
- **并发与关机（#1169、#1170）**：四处夹在两次上游调用之间的等待不看 ctx（关机或预算耗尽后仍睡完并照发下一次调用），实测三个 OAuth onboard 轮询 28.3s / 25.2s / 8.1s → 0.16s，签到退避 2.21s → 0.47s；调度器三条路径与手动触发端点都已接上（手动触发跑在请求 ctx 上，已处理账号各自留 `checkin_logs` 行）。共享计数器的补偿回滚挂在限流拒绝与失败补偿两条热路径上，此前每次上行整段 Lua；改为 `EVALSHA` 优先、仅 `NOSCRIPT` 回退一次 `EVAL`（不保存需要失效处理的「服务器已有脚本」状态），ACL 禁 scripting 的 `NOPERM` 从「回滚失败」改为既有 `INCRBY` 降级 + 一次点明权限的 WARN。
- **工程面（#1160、#1162、#1164、#1166、#1167）**：对外文档成为可对账参考（env parity + 路由清单两个自证伪门禁）；smoke 的 409 兜底改按后端实际去重键 `(platform, url)` 回查（此前按脚本自己的 `SITE_NAME`，站点名不同就七步连锁失败）；`internal/pgtest.Reset` 让复用同一个 PG 库的门禁可重复（长期噪音「`handler/admin` 5 个用例第二轮假失败」消除，`-p 1` 要求写进文档）；`checkin_interval_hours` 从「范围检查即静默丢弃」改成与 env 同形的双侧钳制，并删掉从未有读取点的 `HOME_PAGE_CONTENT`（撤回一条文档承诺）；`GET /api/channels` 的快照缓存从单槽改成有界多键（此前分页与仪表盘视图互相逐出，10s TTL 形同不存在，命中率≈0）。
- **测试床复验（合并后 master 重建二进制，commit `cebb374c`）**：new-api 链 13 PASS / 1 WARN / 0 FAIL（**故意不传 `SITE_NAME`**，让 #1162 的 `(platform,url)` 兜底在活体上被走到）· sub2api 链 11 PASS / 2 WARN / 0 FAIL · axonhub 10/10 · gpt-load 13/13 · F5 双语视觉 e2e 13/13 · 五服务全 RUNNING。复验顺带抓出两条运维可见缺陷（进波次 5）：verify-token 失败时服务端零日志、`channel selection failed err=<nil>`。
- **负结果（别再查）**：`handler/admin/accounts.go` 的 `globalAccountsCache` 虽然也是单槽形状，但 `get()` 不带 key、分页路径直接绕过快照缓存 ⇒ 键空间真的是 1，不存在 channels 那种互相逐出缺陷。

- **发布事实（2026-09-03，已 publish = repo Latest）**：tag 管道全绿（22 job：12 检查 + `docker-build` + `docker-push` + `release`）→ draft notes 人工审阅（12 资产 = 5 平台 server + 6 migrate 二进制 + `checksums.txt` + `install.sh`；body 与 `CHANGELOG.md` 的 `## [v0.17.0]` 节**逐字节相同**，5744 字符；私有路径 / 内部工具名 / 主机事实扫描零命中）→ `gh release edit --draft=false`。**产物抽样校验**：下载 `metapi-linux-amd64`，sha256 与 `checksums.txt` 一致，`--version` 报 `v0.17.0`；`install.sh` 的下载基址确认已切到 `releases/latest/download`（不再钉死某个 tag）。
- **发布当轮的两处环境噪音（非代码）**：master push 的 `docker-push` 与 #1172 的 `visual-regression` 各挂一次 `bun install --frozen-lockfile` 的 registry 完整性错误（`lightningcss-linux-arm64-musl` / `@base-ui/react` / `@oxlint/binding-linux-x64-gnu`），重跑即绿；tag 管道的 `docker-push` 二次尝试成功。给 `bun install` 加重试已进 backlog。

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

