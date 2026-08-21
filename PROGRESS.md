# PROGRESS — Wave 4 / M 线（迁移兼容实测复核）

分支：`wave4/migration-compat`，基线 master=f32d99d (v0.16.6)。
完成一项立即更新本文件；不做/存疑事项进 `BLOCKED.md`。
注：本地演练目录统一在 worktree 内 `.drill/`（未跟踪）；日志证据已脱敏绝对路径。

## 任务 0 — 读码 + 构建（状态：完成）

已读：
- `scripts/regen-ts-fixture.sh`：启动**真实** TS 服务器（cita-777/metapi 只读对照检出）→ 其 drizzle 迁移建库 → 经 admin HTTP API 写入确定性哑数据集（种子 7 站点 + 3 站点 + 5 账号 + 2 下游密钥）→ better-sqlite3 `wal_checkpoint(TRUNCATE)` + vacuum → 产出 `$REPO_ROOT/store/testdata/ts-source/{hub.db,manifest.json}`。
- `docs/migration.md`：三场景（SQLite/PG 直接接管、MySQL 两段式）+ metapi-migrate CLI 参考。
- `cmd/migrate/main.go` flag 定义：`--from` `--to` `--dry-run` `--progress` `--verify` `--overwrite`（默认 true）`--batch-size`（默认 1，保留以兼容 TS）。
- `store/migrate.go` AutoMigrate：35 表 CREATE IF NOT EXISTS + additive 步骤（含 `sc2_017`~`sc2_024` TS-heritage 收敛），收敛步数 >0 时日志 `store: converged legacy schema additive_migrations=N`。
- `store/migrate_runner.go` RunMigration：搬 18 表（`AllTableNames`）；`--verify` 是**迁移执行后**的 source snapshot vs target 哈希核对（`verifyChecksums`），CLI 无独立 verify-only 模式（见 BLOCKED-1）。
- `store/bootstrap.go` + `store/open.go` `probeSQLiteWritable`：SQLite 打开前探测目录/文件可写，失败报带 `chown -R 1001:1001` 的可操作提示（#849/#875）。`store/writable_test.go` 证实 root 运行时权限探测被绕过（测试自身 t.Skip），因此任务 2 演练以非 root 用户（uid 65534/nobody）执行。

### fixture 脚本依赖链（实测结论）

| 依赖 | 要求 | 本机事实 | 结论 |
| --- | --- | --- | --- |
| Node | ≥ 25（node:zlib zstd） | 本机 Node 25.0.0 可用（NODE_BIN 指向它）；系统 PATH 上另有 Node 22 | NODE_BIN=Node 25 |
| METAPI_TS_DIR | cita-777/metapi 只读检出，依赖已装、tsx 可用、better-sqlite3 与 NODE_BIN 同 ABI | 工作区只读对照检出（依赖已装，未改任何文件） | 实测启动验证 |
| curl | 可直连 127.0.0.1（脚本自带 `--noproxy '*'`） | 可用 | OK |
| 输出目录 | `$REPO_ROOT/store/testdata/ts-source/`（REPO_ROOT=脚本所在目录的上一级） | 见下方重定向说明 | OK |
| 其它 | 本地 PG 127.0.0.1:55432（角色有 CREATEDB，实测 t）；psql/runuser/curl/jq 可用；无 sqlite3 CLI（用 Go 服务 admin API 读取抽查） | 实测 | OK |

**产出重定向（不改动脚本内容）**：脚本把产物写进 `$(dirname $0)/../store/testdata/ts-source/`，直接跑会覆盖 worktree 里已提交的黄金 fixture（`store/testdata/` 不在白名单）。做法：把脚本**原样复制**到 worktree 内临时目录 `.drill/fixture-gen/scripts/` 再执行 → 产物落 `.drill/fixture-gen/store/testdata/ts-source/`。脚本逻辑零改动，只是换了 REPO_ROOT。

**构建**：`go build -o <builddir>/metapi-m ./cmd/migrate && go build -o <builddir>/metapi-s ./cmd/server`（结果见下；`<builddir>` 为仓外临时构建目录，路径已脱敏）。
worktree 无 gitignored `web/dist`（`go:embed dist` 需要），从主 checkout 复制了 stub（纯构建产物，`git check-ignore` 确认被 .gitignore 忽略，不进提交）。

## 任务 1 — TS→Go 直接接管演练（SQLite）（状态：完成 ✅）

流程：真实 TS 服务器生成 fixture（27 表/10 站点/5 账号/2 下游密钥，见任务 0 输出）→ 复制为演练库 → `<builddir>/metapi-s` 以 DATA_DIR 指向演练库、PORT=4310 启动。

验收证据（绝对路径已脱敏为 `<wt>`=worktree 根）：

- 收敛日志（启动即出现）：
  - `store: running auto-migration dialect=sqlite`
  - `store: applying additive migration version=sc2_001...` 直至 `sc2_024_model_availability_is_manual`（TS-heritage `sc2_017`~`sc2_024` 全部执行）
  - `store: converged legacy schema additive_migrations=24 dialect=sqlite` ✅
  - `store: auto-migration complete dialect=sqlite` → `bootstrap: database ready` → `listening addr=127.0.0.1:4310`
- `curl /health` → `{"status":"ok"}` ✅
- `curl /ready` → `{"status":"ok","database":"ok"}` ✅
- sites 抽查（经管理 API，camelCase 契约）：TS 种子站 `{"id":1,"name":"OpenAI 官方","platform":"openai"}` 可读；TS 期创建站全在：`[{"id":8,"name":"测试中转站A","platform":"new-api"},{"id":9,"name":"测试中转站B","platform":"one-api"},{"id":10,"name":"订阅站C","platform":"sub2api"}]` ✅
- accounts 抽查：5 行全可读 `[{"id":1,"username":"zhongzhuan-a-user","siteId":8,...},{"id":2,"username":"zhongzhuan-b-user",...},{"id":3,"username":"sub-c-user",...},{"id":4,"username":"openai-dummy",...},{"id":5,"username":"claude-dummy",...}]`，密钥已掩码（`sk-f****0000`）✅
- 收尾：SIGTERM 正常退出，WAL 折叠回单文件（演练库 561152→684032 bytes，收敛写入所致）；**源 fixture 字节未变**（sha256 前后一致 `1de8f731...`）。

## 任务 2 — #875 反向验证（状态：完成 ✅）

方法说明：`store/writable_test.go` 自己注明 root 下权限探测被绕过（本机以 root 运行），故红证据用非 root 用户（uid 65534/nobody）+ chmod 500 目录复现真实的 uid 不匹配场景（对应 #849/#875 容器场景：镜像 uid 1001 vs root 属主目录）。

**红证据**（新建目录 `chmod 500`，路径脱敏为 `<tmp>`）：

```text
$ ls -ld <tmp>/data
dr-x------ 2 root root 4096 ... <tmp>/data
$ runuser -u nobody -- env DATA_DIR=<tmp>/data AUTH_TOKEN=... PORT=4312 <builddir>/metapi-s ; echo EXIT=$?
level=ERROR msg="startup bootstrap failed" error="ensure runtime database: bootstrap: store: data directory \"<tmp>/data\" is not writable by the current process (uid 65534). The metapi image runs as non-root uid 1001 — on the Docker host, make the mounted data directory writable by uid 1001, e.g. 'chown -R 1001:1001 <host-data-dir>' or 'chmod -R a+rwX <host-data-dir>'"
EXIT=1
```

✅ 可操作提示（含 `chown -R 1001:1001`），非裸 SQLite 错误；非零退出。

**绿证据**（`chmod 755` 恢复后同一目录正常启动）：

```text
level=INFO msg="bootstrap: database ready" dialect=sqlite
level=INFO msg="bootstrap complete"
level=INFO msg=listening addr=127.0.0.1:4313
$ curl /ready → {"status":"ok","database":"ok"}
```

✅ #875 行为健在，无回归。演练后目录已删。

**附带观察（移交候选）**：若 DATA_DIR 的某个**父级**目录对运行 uid 不可遍历（例如数据目录嵌在 700 的 home 下），失败会更早发生在 `bootstrapRuntime` 的 `MkdirAll`，报 `create data directory ...: mkdir <parent>: permission denied`（裸 mkdir 错误，不走 #875 的友好探针）。建议（不在本线白名单，移交 C/B 评估）：bootstrap 的 MkdirAll 失败也走同一条可操作提示。

## 任务 3 — migrate 矩阵 6 格（状态：完成 ✅）

源 = 任务 1 的纯净 fixture（TS 形态，未 Go 化；sha256 `1de8f731...`）。PG 目标 = 本地 55432 自建临时库 `wave4_m_tgt`（用完 drop）。路径脱敏：`<src>` = fixture hub.db。

| # | 格 | 结果 | 关键输出 |
| --- | --- | --- | --- |
| 1 | SQLite→SQLite `--dry-run` | ✅ exit 0 | `Direction: SQLite → SQLite (copy / dialect check)`；`[Dry-run] Would insert 18 rows across 18 tables.`；`[Dry-run] No data written.`；summary: sites=10 accounts=5 settings=1 downstream_api_keys=2，其余 0 |
| 2 | SQLite→SQLite 正式 + `--verify --progress` | ✅ exit 0 | `Inserting 18 rows... Done: 18 rows in 1ms`；`Verifying checksums... All checksums match.`；summary 行同上（完整日志 `.drill/matrix/cell2.log`） |
| 3 | SQLite→PG `--dry-run` | ✅ exit 0 | `Direction: SQLite → PostgreSQL (forward migration)`；`Would insert 18 rows`；目标库 dry-run 后仍 0 张表；summary 里密码自动掩码 `postgres://metapi_test:***@127.0.0.1:55432/...` |
| 4 | SQLite→PG 正式 + `--verify --progress` | ✅ exit 0 | `Done: 18 rows in 21ms`；`Syncing PostgreSQL sequences...`；`All checksums match.`；psql 复核目标 `sites/accounts/downstream_api_keys/settings = 10/5/2/1` |
| 5 | 反向①：篡改目标后核验 | ✅ 红证据齐全（见下） | checksum 机制确实检出篡改 |
| 6 | 反向②：`--dry-run` 源库字节未变 | ✅ | 前后 sha256 均 `1de8f7310bdaa13d1e163ae0b655b41b4f2b2943d7aac500cc8d07508f8947dd`（格 1/2/3/4 每格跑完都复测，全程只读） |

**反向格①（篡改检测）三层证据**：
1. CLI 现状实测（`psql UPDATE sites SET name='TAMPERED-BY-DRILL' WHERE id=1` 后）：
   - 重跑 `--verify`（overwrite 默认 true）→ 先清空重拷再核对 → `All checksums match.` exit 0（篡改被静默修复，**检不出来**）；
   - 篡改后 `--overwrite=false --verify` → `Error: target database already contains data. Use --overwrite to replace` exit 1（非 checksum 失败）。
   → 结论：CLI 无独立 verify-only 模式，任务书设想的「篡改后单独跑 --verify 报 checksum 失败」现有 CLI 做不到 → 记入 BLOCKED-1，不加新功能，移交 C 线评估。
2. 引擎级红证据（新增测试 `store/migrate_verify_tamper_test.go`，直接驱动 RunMigration 第 12 步同款 `verifyChecksums`）：迁移成功 → UPDATE 目标一行 → `verifyChecksums` 报 `sites: checksum mismatch (source=..., target=...)`。
   ```text
   --- PASS: TestVerifyDetectsTamperedSQLiteTarget (0.83s)
   --- PASS: TestVerifyDetectsTamperedPGTarget (0.68s)   # PG_TEST_DSN 指向本地临时 PG
   ok  	github.com/deliciousbuding/metapi-go/store	1.633s
   ```
   （测试断言的就是「篡改 → 必须 mismatch」，PASS = 检出能力为真。）
3. 源库只读承诺：全程 sha256 未变（同格 6）。

**附带实测（文档对账用）**：PG→PG 拒绝 `Error: unsupported direction: PostgreSQL source to PostgreSQL target` ✅ 与文档一致。

## 任务 4 — 文档对账（状态：完成 ✅）

逐条对照 `docs/migration.md` 与实测，共 5 处修正 + 若干确认一致：

**改了（文档与实测不符 → 改文档）**：
1. A.4 首启日志示例缺 `store: converged legacy schema` 行（实测必现，任务 1/2 均复现）→ 补上并注明仅在实际收敛时出现。
2. `--batch-size` 描述过时：文档原写「当前实现固定逐行插入」，实测代码支持多行批插（`groupInsertStatements`，默认 1=逐行）→ 按 flag 实际语义改写。
3. `--verify` 失败文案：文档引用了代码里不存在的 `Verification warning: ...`；实测失败输出是 `Verification failed: ...` + 非零退出 → 改正，并补充「无独立 verify-only 模式」的说明（对应 BLOCKED-1）。
4. A.3/故障排查的权限错误描述：#875 探针落地后，非 root 启动遇到不可写目录**首先**报可操作的 `store: data directory ... is not writable ... chown -R 1001:1001`（任务 2 红证据），裸 SQLite 错误只在 root 运行绕过探针时才可能看到 → 两处均按实测改写。
5. 「说明」第一条：原文称迁移后需「再启动一次 Go 服务让 additive 迁移把目标库补到当前版本」，实测迁移器自身的 `AutoMigrate` 已含全部 additive 步骤（cell 4 日志：PG 目标 `converged legacy schema additive_migrations=24`）→ 改为「一步到位，之后启动幂等」。
6. 头部 Last updated 日期刷新。

**确认一致（不改）**：18 表集合与各表行数输出；全部 7 个 flag 名称与默认值（`--overwrite` 默认 true）；方向矩阵含 PG→PG 拒绝文案 `unsupported direction: PostgreSQL source to PostgreSQL target`（实测复现）；`target database already contains data` 拒绝文案（实测复现）；`All checksums match.` 成功文案；`make migrate-build` 目标存在；`/ready` 返回 `{"status":"ok","database":"ok"}`；源库只读承诺（全程 sha256 未变）；密码在 summary 中自动掩码；文档无内部路径/主机名（正则扫描确认）。

## 任务 5 — 门禁与 PR（状态：进行中）

## 决策记录

- 任务 3 反向格①（篡改目标后跑 --verify）：CLI 的 --verify 只附在执行型迁移之后（先 overwrite 重拷再核对），无法对已迁移目标做独立复核；按现有工具形态给出真实红证据的方式 = 新增包内测试直接驱动 `verifyChecksums`（白名单允许新增 *_test.go），并在 BLOCKED-1 记录 CLI 缺口。
- PROGRESS.md/BLOCKED.md 与仓内 .gitignore（`# Agent handoff files (never commit these)`）冲突：任务书完成条件明确要求「BLOCKED.md 随交付提交」，故用 `git add -f` 强制纳入本次交付；不修改 .gitignore（白名单外），它继续对未来误加兜底。
