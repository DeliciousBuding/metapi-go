# EVIDENCE — Wave 4 E 线（实测验收）· v0.16.6 (master=f32d99d)

日期均为 2026-08-22（本机时区 +08:00）。所有命令可在同环境 10 分钟内复跑。`$METAPI_BIN` = `go build -o <任意路径> ./cmd/server` 产物。
脱敏约定：相对路径；不含主机名/内部实例标识；`request_id=<sanitized>` 表示原值含环境标识已抹除。

---

## E1 · Go e2e 套件全绿（任务 1）

命令（仓库根）：

```bash
go test ./e2e/... -count=1 -v
```

输出摘录（完整日志见会话；31 pass / 0 fail / 0 skip）：

```text
--- PASS: TestBackupExportImportRoundtrip (0.17s)
...
--- PASS: TestShutdownRejectsNewConnections (0.08s)
--- PASS: TestBasicChatCompletions (0.00s)
--- PASS: TestChannelRetry (0.00s)
...
ok  	github.com/deliciousbuding/metapi-go/e2e	5.903s
```

7 个文件逐一结论（按 `--- PASS` 归属）：

| 文件 | 结论 |
|:---|:---|
| `e2e/e2e_backup_test.go` | PASS ×8（roundtrip/accounts-only/preferences-only/invalid-type/空库/坏 JSON/设置保真/route_channel token NULL FK） |
| `e2e/e2e_cascade_isolation_test.go` | PASS ×2（5xx 风暴下 channel 级排除；兄弟通道恢复） |
| `e2e/e2e_flow_test.go` | PASS ×3（建站→代理全链路 / 未授权 / 流式） |
| `e2e/e2e_hardening_test.go` | PASS ×4（常量时间比较 / 并发 / body 限流先于上游 / 限流拒绝） |
| `e2e/e2e_shutdown_test.go` | PASS ×2（流式负载下优雅退出；退出后拒新连接） |
| `e2e/e2e_test.go` | PASS ×12（基础补全/重试/下游鉴权/模型匹配/错误传播/SSE 等） |
| `e2e/e2e_p0585_production_test.go` | 默认构建不编译（`//go:build staging`）。带 tag 无环境实跑：`--- FAIL ... staging cascade test requires env: METAPI_STAGING_URL, METAPI_AUTH_TOKEN, METAPI_PROXY_TOKEN` — fail-fast 行为符合文件头注释；无真实 staging 实例，未深跑（环境不允许，诚实记录） |

## E2 · SQLite 方言启动 + 优雅退出（任务 2）

```bash
mkdir -p .tmp-e2e-data/sqlite
PORT=4180 DATA_DIR=<worktree>/.tmp-e2e-data/sqlite \
AUTH_TOKEN=<临时值> PROXY_TOKEN=<临时值> "$METAPI_BIN" &
```

探针输出（`curl --noproxy '*'`；注意：本机 shell 有 HTTP 代理变量，直连必须绕过）：

```text
GET /health → HTTP=200 {"status":"ok"}
GET /ready  → HTTP=200 {"status":"ok","database":"ok"}
```

`kill -TERM <pid>` 后日志：

```text
level=INFO msg="received signal, shutting down" signal=terminated
level=INFO msg="server stopped"
```

附加证据（端口冲突的诚实处理）：第二实例同端口启动 →
`level=ERROR msg="server listen failed" error="listen tcp 0.0.0.0:4180: bind: address already in use"` → 后台调度器有序停止后退出。

启动日志另观察到：`pricingcatalog: fetch https://models.dev/api.json: context deadline exceeded`（出网受限，降级到内置预设，属环境特性非缺陷）。

## E3 · PostgreSQL 方言启动 + 优雅退出（任务 2）

```bash
psql -h 127.0.0.1 -p 55432 -U metapi_test -d metapi_test \
  -c "CREATE DATABASE metapi_e2e_wave4;"
PORT=4181 DATABASE_URL="postgres://metapi_test:<密码>@127.0.0.1:55432/metapi_e2e_wave4?sslmode=disable" \
AUTH_TOKEN=<临时值> PROXY_TOKEN=<临时值> "$METAPI_BIN" &
```

```text
level=INFO msg="store: database opened" dialect=postgres
level=INFO msg="store: running auto-migration" dialect=postgres
（24 条 additive migration 全部应用）
GET /health → HTTP=200 {"status":"ok"}
GET /ready  → HTTP=200 {"status":"ok","database":"ok"}
kill -TERM → "received signal, shutting down" signal=terminated → "server stopped"
psql ... -c "DROP DATABASE metapi_e2e_wave4;" → DROP DATABASE
```

## E4 · acceptance 旅程：假上游诚实行为（任务 3）

一次性后端（SQLite 临时目录，端口 4182，临时 AUTH_TOKEN）+ 不可达上游 `http://127.0.0.1:39999`：

```bash
cd web && BASE_URL=http://127.0.0.1:4182 AUTH_TOKEN=<临时值> \
UPSTREAM_URL=http://127.0.0.1:39999 UPSTREAM_USERNAME=metapi-e2e \
PLATFORM=new-api bun run acceptance:e2e
```

```text
$ node scripts/acceptance-e2e.mjs
[PASS] journey: site onboarding: created "acceptance-real-site" -> http://127.0.0.1:39999 (new-api) via UI
== acceptance:e2e summary — 1 passed, 0 failed ==
```

发现（非缺陷，属设计事实，已写进文档）：旅程显式选择平台，建站不依赖上游可达性——**假上游下旅程 1 也通过**，站点以 `status:"active"` 落库。检测端点本身对不可达上游诚实失败：

```text
POST /api/sites/detect {"url":"http://127.0.0.1:39999"}
→ HTTP=400 {"error":"Could not detect platform","request_id":"<sanitized>"}
```

服务端逻辑：`handler/admin/sites.go` createSite 仅在请求未带 platform 时才跑 `service.DetectSite`。

## E5 · acceptance 旅程：ACCEPT_LOGIN=1 全链路（任务 3）

```bash
cd web && BASE_URL=http://127.0.0.1:4182 AUTH_TOKEN=<临时值> \
UPSTREAM_URL=http://127.0.0.1:39999 UPSTREAM_USERNAME=metapi-e2e \
UPSTREAM_PASSWORD=<假值> ACCEPT_LOGIN=1 PLATFORM=new-api bun run acceptance:e2e
```

```text
[PASS] journey: site onboarding: created "acceptance-real-site" -> http://127.0.0.1:39999 (new-api) via UI
== acceptance:e2e summary — 1 passed, 2 failed ==
  [FAIL] journey: account login via UI: step "account row appears": locator.waitFor: Timeout 25000ms exceeded.
  [FAIL] journey: check-in via UI: step "locate account": account "metapi-e2e" not found
```

结论：脚本链路完整可跑、失败按步标注且诚实（上游不可达 → 后端真实登录不可能成功 → 账户不出现）。旅程 2 在失败前走过的全部步骤（快照轮询→/accounts→Add account→Password tab→站点选择→Username/Password→submit）证明这些选择器与当前 DOM 全部匹配。旅程 2/3 完整通过需要真实上游（见 BLOCKED.md）。

## E6 · bun 调用形式（脚本层发现，已修文档）

```bash
bun --cwd web run acceptance:e2e   # bun 1.3.14：打印 usage 后 exit 0 —— 静默不执行！
bun run --cwd web acceptance:e2e   # 正确形式，实际执行旅程
```

风险：操作者照文档（旧形式）跑会拿到 exit 0 误判为通过。已修正
`docs/internal/analysis/e2e-acceptance-platform.md` §3 并加警示。

## E7 · 选择器对账记录（任务 3）

| 旅程 | 选择器 | 结论 | 依据 |
|:---|:---|:---|:---|
| 1 | `Add site` 按钮 / Name / URL label / Platform combobox / `Create` | 实测命中 | E4 旅程 PASS（真实 Chromium + built SPA） |
| 2 | `Add account` / Password tab / `Select a site` combobox / Username / Password / dialog submit | 实测命中（至 submit） | E5 步骤日志在 submit 之后才失败 |
| 3 | `Run all check-ins` / `table tbody tr` | 代码核对（未实跑到） | `features/checkin/components/checkin-page.tsx:406` 用 `t('checkin.page.runAll')`，en.json:1927 = "Run all check-ins"；表格经 data-table core 渲染真 `<table>/<tbody>/<tr>`（`components/data-table/core/data-table-view.tsx:103,259`） |

i18n 标签核对：`accounts.page.addButton` = "Add account"、`sites.*.addSite` = "Add site"（en.json:1507/467）。`seedAuth` 的 `auth_token` localStorage 键与 SPA 鉴权一致（旅程通过登录守卫）。**无失效选择器，无需修复。**

## E8 · §6 quirk 复测（任务 4，新探针 `web/scripts/acceptance-probe-header-quirk.mjs`）

条件重建：API 建全新站点 → 立即进 /accounts（无快照轮询），20Hz 采样「Add account」按钮中心的 elementFromPoint，并行发起真实点击。累计 30+ trial（最终轮 10 次输出如下）：

```text
trial 1: click=clicked dialog=true firstEnabledAt=542ms ... loadTimeIntercepted=0 postClickIntercepted=35
trial 2: click=clicked dialog=true firstEnabledAt=1360ms ... loadTimeIntercepted=0 postClickIntercepted=11
trial 3..10: click 超时（按钮 1234~2265ms 才可操作）...
== header-overlap probe summary: 2/10 clean clicks, 8 failed (...), load-time interception samples=0 ==
NOT reproduced at load time: every interception sample occurred AFTER the probe's own click opened the create sheet
```

补充实测：
- 无并发采样时，按钮 355~1176ms 出现、出现即可用、无 disabled 窗口（3 trial）。
- 页面加载 2s 内 `[data-slot="sheet-content"]` popup 0 次挂载（100ms 采样 ×20）。
- 曾观察到的拦截元素 `form.flex-1.space-y-5.overflow-y-auto.p-4` = AccountFormDialog 表单体（`features/accounts/components/account-form-dialog.tsx:280`，右侧 Sheet、fixed、z-50），经祖先链确认属于**点击成功后打开的 Sheet 本体**（`role="dialog" data-open data-slot="sheet-content"`），非加载期覆盖物。

结论：**§6 所述「页头盖住工具栏」未在 v0.16.6 复现**（文档所述，2026-08-20 记录；本轮实测未见）。可复现的真实竞态是：全新站点后立即进入 /accounts，按钮要 ~0.4–2.3s 才可操作，且 `disabled={sites.length===0}`（`features/accounts/components/accounts-page.tsx:453`）在快照未含站点时钉住按钮——旅程脚本的 `waitForSiteInSnapshot` 正是为此而设，工作正常。

## E9 · 环境诚实记录（未跑/不允许）

- 旅程 2/3 完整通过：需真实上游凭据——本轮决策不碰真实上游（BLOCKED.md）。
- `e2e_p0585_production_test.go` 深跑：需 `METAPI_STAGING_URL` 等（无）。
- `testbed/compose.template.yml` 链路：本机无 Docker（只读参考）。
- `scripts/e2e/smoke.sh` / `verify-token-import.sh`：需要真实上游（可读可跑，无上游可跑）；未改动其内容（白名单外）。

## E10 · 运维事故自述（防编造：真实发生，已处置）

验证 bun 调用形式时，未显式传 BASE_URL 的一次测试命中了本机默认 4000 端口上的一个非预期监听（SSH 隧道）。acceptance 脚本按其设计清空了该实例的站点/账户并建了 1 个站点。已立即删除该站点并验证列表为空（`DELETE /api/sites/<id>` → `{"success":true}`，随后 `GET /api/sites` → `[]`）。教训已写入文档 §3 安全提示（显式 BASE_URL + 确认目标是一次性实例）。
