# Testing Strategy and Real-Platform Testbed

**Last verified**: 2026-10-17

> Public, environment-agnostic test guidance. The version narrative lives in `CHANGELOG.md`; open outcomes are tracked as GitHub issues.

## Test layers

Pure formatter, schema and export suites can opt into Node with an explicit
`@vitest-environment node` annotation after checking their dependencies do not
require a DOM. Component suites keep jsdom. Tests use up to four workers with
file isolation enabled to keep mocks local to each suite.
The headless i18n suite verifies locale initialization without `document`, while
the browser language-switching suite protects the HTML language contract.

| Layer                     | Command / asset                                                                      | Protects                                                                     |
| :------------------------ | :----------------------------------------------------------------------------------- | :--------------------------------------------------------------------------- |
| Go unit and integration   | `go test ./... -count=1 -race`                                                       | Package behavior, dual dialect, concurrency, handlers, routing, transforms   |
| Go formatting             | `gofmt -l` on tracked files (CI `lint` job, go.mod toolchain)                        | Whole-tree formatting consistency; the pinned toolchain keeps local and CI opinions identical |
| Frontend static gates     | `bun run typecheck`, `lint`, `format:check`, `knip`, `test`, `build:web` in `web/` | Types, lint, dead code, UI contracts, production bundle                      |
| Frontend browser gates    | `bun run a11y:scan`, `bun run ui:smoke`, `bunx playwright test --config playwright.visual.config.mjs catalog-sources-responsive.spec.mjs model-playground.spec.mjs` in `web/` | Built SPA accessibility, routes, narrow catalog controls, and chat interactions; CI `a11y` job |
| Repository e2e            | `e2e/`                                                                               | Full HTTP paths with controlled upstream fixtures                            |
| Real-service CI           | `.github/workflows/main.yml`                                                         | New API, One API, Sub2API and CLIProxyAPI detect/login/route chains in service containers, plus a three-protocol `/v1` relay matrix (chat completions, Anthropic messages, OpenAI responses) reconciled against the deterministic upstream's request log, plus cross-OS binary runtime smoke (`runtime-smoke-matrix`) |
| Frontend acceptance       | [`../web/scripts/acceptance-e2e.mjs`](../web/scripts/acceptance-e2e.mjs) (+ [`acceptance-probe-header-quirk.mjs`](../web/scripts/acceptance-probe-header-quirk.mjs) for the fresh-site accounts-page race) | Real-browser user journeys (Playwright) against a live metapi + real upstream; operator-gated, not a PR check |
| Operator runtime evidence | `scripts/e2e/*.sh` and focused staging procedures                                    | Compatibility that requires real credentials, topology, or upstream behavior |

`test-e2e` runs real upstream service containers against a deterministic mock
model, not real-model inference. Its New API service is pinned to an immutable
release image so this required gate tests a reproducible login-to-relay chain;
passing it does not establish compatibility with New API `latest`. Changes to
the upstream's authentication API require a separate adapter migration and
current-version acceptance, not silently changing this fixture or weakening the
login/relay assertions (tracked in [#1383](https://github.com/DeliciousBuding/metapi-go/issues/1383)).

The same job also starts the production server binary against a copy of the
legacy TypeScript SQLite fixture and verifies readiness, preserved site data,
and additive-field defaults through the admin API. This is process-level SQLite
upgrade coverage; it does not establish PostgreSQL process-startup compatibility
or real-model relay behavior.

`test-pg` additionally starts the production server binary against a randomized,
disposable PostgreSQL schema using the CI service's dummy test credentials. The
schema starts with a reduced TS-era `sites` fixture missing additive columns; the
gate checks its preserved row, migrated PostgreSQL column types/defaults and
timestamp through both the admin API and database, then creates new records to
check sequence reconciliation. Two independently logged deterministic upstreams
are bound to one route in priority order. The gate serves a request from the
primary, stops that process, and requires the same downstream route to recover
through the sibling with two request-ID-correlated PostgreSQL attempt logs. It
restarts the server and checks persisted data. The schema is dropped on exit.
The reduced fixture is not a complete historical PostgreSQL dump, and the
deterministic relay does not establish real-model or downstream-client acceptance.

## From historical bugs to user-task regressions

Use the existing GitHub issue / PR to connect a reported failure to its regression test:

1. Describe the failed user task, minimal reproduction, expected result and root cause.
2. Add a regression check that fails for the original defect. Use an isolated checkout when testing with the fix removed, then restore it before continuing.
3. For a release candidate, run relevant checks on the final source and build. Record the commit, local changes, build version, commands and results. Distinguish fixture tests, real-model relay and downstream-client E2E in the result summary.
4. Repeat affected checks when the source, build or environment changes. Include remaining issues in the release review.

Publish only sanitized reproduction and result summaries. Keep credentials, private hosts and raw runtime evidence out of issues, PRs and this repository; follow the privacy boundary below.

## Local fixture previews (browser walkthroughs)

For UI-facing batches, verify the change in a real browser against a local
fixture instance before opening the PR. The pattern used by the route-decision
batches (A–E): a throwaway script under `.local/` builds the current tree
(`go build -o .local/<name>-preview.exe ./cmd/server` after `bun run build`),
starts it on a fixed loopback port with fixture env (`AUTH_TOKEN` /
`PROXY_TOKEN` / `ACCOUNT_CREDENTIAL_SECRET` all synthetic, SQLite under
`.local/<name>-data/`, background jobs off), seeds via the admin API (or a
scoped SQLite write when no API exists, e.g. usage projections), and records
the source commit, local changes, build command and process ID with the result.

Rules of engagement: fixture tokens never leave the local instance, upstreams
are `httptest`/loopback fakes only, seeding is idempotent via a marker file,
and the script terminates the server on exit. Record the walkthrough as a local
fixture test alongside the pre-push results.

## 开发反馈与测试夹具

开发时按修改范围运行用例和所属包，例如：

```bash
go test ./handler/admin -run '^TestOpsWS' -count=1
go test ./handler/admin ./service -count=1
```

涉及并发行为时，对相关用例加 `-race`。需要检查顺序依赖和偶发失败时，
对定向用例使用 `-shuffle=on -count=10`，保留失败输出中的 seed。
日常测试可使用 Go 的结果缓存；`-count=1` 用于需要实际执行的验证。
push 前仍运行[完整本地检查](../CONTRIBUTING.md#本地验证)。

普通业务测试可用 `internal/sqlitetest.Open(t)` 获得独立内存库。
每个测试进程按当前生产迁移生成一次模板，再由 SQLite 驱动复制到各测试连接；
表、索引、默认值、迁移记录和连接设置由真实初始化产生。
模板只存在于本进程内，不保存二进制 schema 夹具或跨运行缓存。
测试数据库打开、迁移、历史兼容和启动过程时，直接使用 `store.Open` 和
`store.AutoMigrate`；文件持久化测试继续使用独立临时文件。

添加 `t.Parallel()` 前检查数据库、配置、缓存、环境变量和后台任务的所有权。
每项并行测试应拥有自己的可变资源；修改进程级状态的用例保持串行。
网络测试使用可控上游或传输错误，未知平台测试仍执行真实 adapter detection。
异步测试优先等待明确的完成信号；超时作为失败上限，正常收尾应主动释放资源。
并发断言调用生产实现，检查操作错误与合法最终状态；手写竞争演示不能代替回归测试。
并发用例用启动信号和固定操作次数组织读写，等待所有 worker 退出；
需要在锁外读取的缓存快照，应检查后续更新不会修改已返回的数据。
纯进程内 TTL、计时器测试可使用 `testing/synctest` 验证到期前和到期边界，
网络 I/O、真实取消和传输超时测试保留实际时钟。

大数据夹具按所保护的边界选大小：协议组合矩阵使用仍能触发缺陷的最小有效数据，
容量上限、超限和多 MiB 数据另由定向用例覆盖。缩减数据时保留协议组合、
完整性和失败断言，不通过减少断言或跳过路径提速。

优化前后使用相同工具链、参数和资源预算，分别记录墙钟、CPU、分配与峰值内存。
冷缓存与暖缓存、race 与非 race 分开比较。数据库夹具的微基准可运行：

```bash
go test ./internal/sqlitetest -run '^$' -bench '^BenchmarkOpen$' -benchmem
```

`go test -json` 可定位慢用例，CPU profile 用于区分初始化与业务计算的 CPU 成本。
性能结果同时记录源码提交与未提交差异；失败运行可用于诊断耗时，但不作为通过结果。

## 本地 race 检查

`scripts/go-race.sh` 执行全量 race 测试，每包默认超时 900 秒，
可通过 `METAPI_RACE_TIMEOUT_SECONDS=<seconds>` 调整。
完整本地检查默认设置 `GOMAXPROCS=2`，限制 Go build/vet 的默认包并行度；
race 入口默认 `-p 2 -parallel 2`。显式的 `GOMAXPROCS` 会被保留，
`METAPI_RACE_PACKAGES` 和 `METAPI_RACE_PARALLEL` 分别调整 race 的包并发和包内并发。
用 `bash scripts/go-race.sh --help` 查看参数；入口固定运行所有包、禁用结果缓存，
不接受筛选用例的参数。降低并发不会减少测试范围，但可能延长墙钟时间。
完整本地检查及 race 入口在子进程环境中将 `GOFLAGS` 设为空白值，覆盖当前环境和
`go env -w` 保存的选测参数；个人定向测试继续直接使用 `go test`。
这些参数控制 Go 调度与测试并发，不是整机 CPU、内存或功耗的硬配额。
脚本在当前环境执行，不自动切换操作系统。Race detector 需要 Go 支持的
OS/架构和 C 工具链；Windows 原生若因 ThreadSanitizer 无法分配 shadow memory
而失败，脚本会打印可直接执行的 WSL 命令。也可在 Linux 容器执行同一命令，
或由 CI 复核。日常定向测试不要求 WSL。
资源竞争可能拖慢测试，超时时先区分环境负载和代码问题，不跳过失败检查。

CI 的 SQLite race shards 独立配置，不使用这个本地超时。
分片必须验证预期包数和非空选择；选择脚本出错直接失败，
不能用“没有包可跑”返回成功。PostgreSQL 共享夹具需串行运行（`-p 1`）。
`internal/pgtest.Reset` 只清空当前 schema 的业务表并保留迁移记录；
跨 schema 外键会使清理失败，不连带清空其他 schema。
本地 race 入口检测到 `PG_TEST_DSN` 时默认 `-p 1`，并拒绝更高的包并发设置。

## 门禁与 lint

机械规则由现有工具和配置维护，分工见[贡献规范](../CONTRIBUTING.md#规则由谁维护)。
项目特有检查只补工具无法覆盖的真实约束，例外限定范围并说明理由。
主观设计判断留给调用链审查，不按函数长度、文件数量或注释词频自动裁决。

新增或修改检查时，验证真实检测入口：

- 正常输入通过，已知违规输入失败；覆盖本次修复相关的语法变体。
- 读取、解析失败直接报错；应检查的入口消失、筛选器意外不再命中时失败。
  记录实际扫描面，但不依赖历史文件数量证明检测正确。
- 负例和真实扫描共用检测函数，或在临时夹具中运行真实 CLI；
  不用另一份复制实现证明自己正确。
- 必要时在隔离工作树移除待保护的修复，确认门禁变红；
  恢复修改后再确认通过。

`docs/pg_rebind_gate_test.go` 检查 SQL 字面量；动态 SQL 和跨文件句柄由
PostgreSQL 行为测试覆盖。前端静态 FormControl 检查提示常见接线问题，
Testing Library 验证实际 DOM 的 accessible name、describedby 和 invalid 状态。

查找已有门禁和负例实现时可用：

```sh
rg -n -i 'vacuous|^func Test.*Sanity' docs --glob '*_test.go'
```

搜索结果只用于导航，不作为覆盖完整性的证明。

## Golden snapshot suites (protocol conversion)

Multi-protocol conversion is the most regression-prone surface of the proxy, so
it is additionally pinned by checked-in golden snapshots (modeled after the
`relaykit/relayconvert` golden pattern). Every golden case is a pair of files
under the owning package's `testdata/golden/<category>/` directory:

- `<name>.input.json` (or `.input.sse` for raw stream fixtures) — the fixture,
  hand-written and never auto-rewritten.
- `<name>.golden.json` (or `.golden.sse`) — the recorded output snapshot.

Categories follow the surface a package owns: `request`, `response`, `stream`
fixtures, plus `decision` (policy truth tables) and `unit` (leaf helpers).

| Package | Golden coverage |
| :--- | :--- |
| `transform/openai/responses` | Responses request sanitization (continuity `previous_response_id` policy, reasoning input items, compact mode) + decision tables |
| `transform/gemini/generate_content` | OpenAI→Gemini request conversion (tool calls + thought signatures, multimodal placeholders, thinking config) + Gemini request normalization |
| `transform/openai/completions`, `transform/openai/embeddings`, `transform/openai/images` | Pass-through identity contract (request/response/stream bytes unchanged) |
| `transform/shared` | Leaf helper edge cases |
| `handler/proxy` | Response-body usage extraction and incremental SSE stream parsing (usage/finish/error/done state, chunk-boundary independence). In this codebase the response/stream half of protocol handling lives here: SSE bytes are generally preserved, except the documented native Chat/Messages complete-tool termination and empty Chat finish normalization ([`client-integration.md#native-tool-response-compatibility`](client-integration.md#native-tool-response-compatibility)); attachment responses and frames after `[DONE]` are left untouched. |

Direct-upstream media fixtures cover imported and native configurations,
independent format permissions, multipart files, binary responses, provider
application errors, cancellation, image tool streams and durable video tasks.
They test the complete HTTP path against controlled upstreams, including
ModelScope task polling and credential-free result downloads. SQLite and
PostgreSQL checks cover configuration, authorization and task persistence;
use `-p 1` when packages share one `PG_TEST_DSN` because fixtures reset tables.
These fixtures do not establish real-model availability or downstream-client
compatibility; those remain separate acceptance layers.

Run the suite like any other test (it is part of `go test ./...` and therefore
part of CI):

```bash
go test ./transform/... ./handler/proxy/... -count=1
```

To regenerate snapshots after an intentional behavior change, set
`GOLDEN_UPDATE=1`:

```bash
GOLDEN_UPDATE=1 go test ./transform/... ./handler/proxy/... -count=1
```

Regeneration discipline:

- Regenerate only after deliberately deciding the new behavior is correct; the
  diff of rewritten `*.golden.*` files is the review artifact.
- Never regenerate to silence a failure you cannot explain.
- Fixture (`*.input.*`) files are data, not generated output: edit them by
  hand to add coverage.
## Public testbed assets

- [`../testbed/compose.template.yml`](../testbed/compose.template.yml): sanitized loopback-only service template for Metapi, New API, One API, and optional adapter targets.
- [`../scripts/e2e/smoke.sh`](../scripts/e2e/smoke.sh): idempotent password-login chain from health and site detection through `/v1` proxying. The relay step is a three-protocol matrix — chat completions, Anthropic messages, and OpenAI responses each assert HTTP 2xx, protocol-shaped content, and the response model echoing the request. When `MOCK_REQUEST_LOG` points at the deterministic upstream's JSONL request log (`mock-openai.py --log`), each protocol additionally reconciles a new upstream-side event carrying the relayed model (count delta over a pre-POST snapshot); without the log that half reports an explicit SKIP. Re-runs converge: `POST /api/accounts/login` upserts, so the login step refreshes the stored credential on every run.
- [`../scripts/e2e/verify-token-import.sh`](../scripts/e2e/verify-token-import.sh): equivalent chain for session JWTs, API keys, and management keys, with the same three-protocol relay matrix and optional `MOCK_REQUEST_LOG` upstream-evidence reconciliation as `smoke.sh`. Re-runs converge as well (#1209): an account that already exists is re-bound to the credential this run holds instead of being skipped, and a reused downstream key has its relay policy re-asserted when this script owns the record. A short-lived upstream credential left stored by the previous run used to pass `verify-token` and then fail the chain two steps later on `balance`.
- Operator-gated multi-channel cascade evidence procedure (see `scripts/verify-cascade-prod.sh` and the cascade verification test).

## Run a real-platform chain

1. Copy `testbed/compose.template.yml` into an operator-controlled test environment.
2. Put real credentials in an ignored `.env`; do not edit them into the template or scripts.
3. Start only the services needed for the adapter under test.
4. Run one of the environment-driven chains:

```bash
METAPI_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN='replace-me-admin-token' \
UPSTREAM_URL=http://127.0.0.1:3001 \
UPSTREAM_USERNAME='replace-me-username' \
UPSTREAM_PASSWORD='replace-me-password' \
PLATFORM=new-api \
  bash scripts/e2e/smoke.sh
```

```bash
METAPI_URL=http://127.0.0.1:4000 \
METAPI_AUTH_TOKEN='replace-me-admin-token' \
UPSTREAM_URL=https://upstream.example.com \
UPSTREAM_TOKEN='replace-me-upstream-token' \
PLATFORM=sub2api \
  bash scripts/e2e/verify-token-import.sh
```

Both scripts print PASS/FAIL/WARN/SKIP summaries, preserve truncated failure evidence, and exit non-zero on a failed required step. Check-in verdicts follow the API's normalized `status` and `skipped` fields: a failed or malformed HTTP 200 response fails the chain, while an explicit skipped outcome is counted as SKIP, not successful check-in coverage. A run with a skipped check-in does not prove that a supported platform can complete a check-in.

## Privacy and evidence boundary

- The public repository may contain templates, placeholder URLs, scripts, and reproducible failure descriptions.
- Hostnames, host filesystem layouts, SSH commands, real credentials, and private upstream addresses stay in the operator-controlled environment.
- Attach sanitized request/response shapes and commit/PR references to public issues; never attach raw environment files or full tokens.
- A healthy run is evidence for the exercised platform and version only. Do not generalize it into a cluster-wide or all-adapter claim.

## Opt-in New API management journey

`scripts/e2e/verify-management-matrix.py` is an eight-check process-level
journey for a disposable Metapi instance and a dedicated ordinary New API user
with no existing API keys. It verifies the pinned Metapi `/api/about` commit, admin
authorization and empty inventories, site and account creation, a finite
upstream API key created through Metapi, direct New API inventory corroboration,
local-only metadata rename, and deletion on both sides. It does not send model
requests or change global upstream settings. It is **not** the historical lost
21-step runner and does not cover check-in rewards, PAT revocation/rebind,
pagination, MFA, natural JWT expiry, real models or downstream clients.

Set `METAPI_BASE_URL`, `METAPI_AUTH_TOKEN`, `METAPI_EXPECT_COMMIT` (full 40-digit
commit from the running binary), `NEWAPI_BASE_URL`, `NEWAPI_USERNAME`, and
`NEWAPI_PASSWORD` in the operator's private environment. Both
`MANAGEMENT_DISPOSABLE=1` and `MANAGEMENT_ALLOW_MUTATIONS=1` are required.
Both origins must be loopback; run it on the isolated test host. The script
refuses nonempty Metapi sites/accounts and a nonempty upstream user
token inventory. Direct requests are capped at 40 and the run at five minutes,
with no retries or redirects.
Record binary/image identity and upstream version separately: a matching
`/api/about` commit does not prove the upstream process identity.

The JSONL stdout contains fixed check/error codes, a random run label and
numeric object IDs, never URLs, credentials or response bodies. Redirect it to
private evidence storage, not a PR or the public repository. Mutation intents
are emitted before writes so an interrupted run can be reconciled by its label
and IDs. It deletes only its own upstream token; site/account and the dedicated
upstream user remain in the disposable environment for inspection. The
offline fixture `test_verify_management_matrix.py` runs in CI and proves that
an empty journey, a local-only token create and a local-only delete cannot pass;
it is instrument evidence, not a live New API result.

## Opt-in live New API authentication boundaries

### Full management matrix

`scripts/e2e/verify-management-full.py` restores the 21 behavioral checks from
the earlier management acceptance instrument, updated for scoped PAT creation,
exact-fingerprint revocation and parameter-bound proof. It requires an empty
disposable Metapi process and a disposable current New API with password proof
available, check-in enabled with a fixed 1,000-quota reward, and a deterministic
model-discovery channel. It does not call a real model.

The matrix covers repeat login/account reuse, finite API-key creation and
upstream readback, local-only rename, default-token handoff, two-sided deletion,
PAT import, check-in reward/no-double-reward/disabled skip, natural JWT expiry,
PAT management after expiry, explicit revocation/rebind, disabled accounts,
proxy-only credentials and rejected credentials without identity changes.
Exactly 21 checks in the prescribed sequence must pass; exceptions, missing
checks, reordered checks, duplicate checks and cleanup failures are failures.
The upstream critical-management window is respected with a checkpointed
20-minute wait, not weakened by changing limits or JWT duration.

Run `--help` for the required private environment. Both loopback origins,
`MANAGEMENT_DISPOSABLE=1`, `MANAGEMENT_ALLOW_MUTATIONS=1`, the exact Metapi commit,
and disjoint evidence/secret directories are mandatory. The instrument creates
one fresh ordinary user, changes only that disposable upstream's check-in toggle
and restores it, then removes its owned objects. Store the output and build details
in operator storage. `test_verify_management_full.py` is an offline
instrument check, not proof that the full live journey passed. MFA and real
model/client checks remain separate.

`scripts/e2e/verify-newapi-auth-boundaries.py` starts disposable New API and
Metapi instances and exercises the authentication boundaries that fixture tests
cannot prove against a real upstream. It requires Python 3, `openssl`, `curl`,
`bash`, and operator-provided `METAPI_BINARY`, `METAPI_EXPECT_COMMIT`,
`NEWAPI_BINARY`, and `EVIDENCE_DIR` environment variables. Optional integrity pins are
`METAPI_EXPECT_SHA256` and `NEWAPI_EXPECT_SHA256`; set
`AUTH_PROOF_EXPIRY_WAIT=0` to defer the real proof-expiry wait, and
`KEEP_WORK_ON_FAILURE=1` to retain the disposable work directory for diagnosis.

The runner checks scoped PAT creation with a grant-bound proof, MFA fail-closed
behavior, missing/wrong-scope/wrong-password/reused/expired proofs, the
encrypted-password upstream variant, and the exact number of product-initiated
automatic relogins after revocation. Against a current New API rc.41 process it
also verifies valid PAT reuse without minting another token, and exercises the
six account grants through real profile, wallet/check-in, and API-key
read/write/reveal requests. The three-protocol relay smoke uses a deterministic
upstream mock and reconciles each request with that upstream's log; it verifies
the protocol relay path, not real-model inference. Each plaintext scenario
receives a fresh New API instance so the harness cannot consume the upstream's
login rate-limit window. It writes a sanitized JSON report and does not print
credentials or proof values to stdout or the report. Temporary databases and
process logs can contain credentials during execution and are removed on
success. It does not change a persistent testbed, upstream rate limits, or
production state.

## Opt-in live model acceptance

`scripts/e2e/verify-real-relay.py` exercises Chat Completions, Responses, and
Anthropic Messages through the service under test. It requires Python 3, curl
8.4 or newer, and operator-provided `RELAY_BASE_URL`, `RELAY_API_KEY`, and
`RELAY_MODEL` environment variables. Inject the key from a secret store rather
than putting it in a command line, shell history, or committed file.

```bash
python3 scripts/e2e/verify-real-relay.py --protocol all --timeout 120 --max-tokens 256
python3 scripts/e2e/verify-real-relay.py --tool-stream --protocol all --timeout 120 --max-tokens 256
```

Each protocol checks JSON, SSE completion, and a complete echo-tool/result
roundtrip. With `--tool-stream`, it additionally exercises a streamed echo-tool
call with reassembly of fragmented arguments, then a streamed result followup; the
tool result contains a fresh receipt that must appear in the final answer, and
merely announcing a tool call is not sufficient. Without the flag the default
scenarios and 12-POST budget are unchanged. The streamed result followup must
preserve the same native replay context as the non-stream contract: Chat
`reasoning_content`, Messages thinking (including signature), and Responses
reasoning items (including raw `reasoning_text`, which never counts as visible
answer text or a receipt). Empty Chat tool identity continuation fields are no-update
fragments; the assembled call still requires a valid ID and name. Unsupported stream
events and unsupported reasoning deltas
fail explicitly. The command makes at most 12 model POSTs by default (18 with
`--tool-stream`), performs no automatic retries, and can incur upstream usage.
It prints metadata only and exits nonzero for any failed scenario. Offline
validator fixtures are instrument checks, not a substitute for a live `--tool-stream` run when claiming real streaming-tool
coverage; a real downstream CLI contract still needs a separate exercise when it
is part of the release scope.

Tool scenarios default to `--tool-choice forced`. Use explicit `--tool-choice auto`
when the upstream supports tools in automatic selection mode only (for example,
a thinking model that rejects forced selection). Auto applies to both tool legs,
is recorded as `toolChoice` in the report, and does not change the request budget
or acceptance criteria: a missing/wrong tool, wrong arguments, or a missing visible
receipt still fails. There is no automatic fallback from forced to auto, and this
option does not change how Metapi forwards another client's `tool_choice`.

A passing client report does not identify the physical upstream provider. Pair it
with verified test configuration, model request IDs, and gateway/Metapi logs for
cross-service provenance. Model aliases may legitimately differ from the
requested model. Deterministic validator tests in CI are instrument checks, not
substitutes for these live calls.

For the browser journey, use `bun run acceptance:e2e` (which executes Node, not
Bun's JavaScript runtime) against a dedicated disposable Metapi instance. Set
`ACCEPT_LOGIN=1` and `ACCEPT_EXPECT_CHECKIN=1` when the real upstream is configured
to support successful check-in. When a test upstream grants a fixed known amount,
set `ACCEPT_EXPECT_REWARD` as well to check that exact value in the API log and UI;
an already-complete check-in is success but does not mean a new reward. The journey verifies a fresh account-specific log
and its UI result; a pre-existing or failed log cannot pass. Without a required
reward, an explicit unsupported/skipped result is reported as SKIP. Fresh
accounts also create their upstream relay token through the account detail form
before route binding and the downstream relay are accepted. Never point the
cleanup-enabled browser runner at an existing user or production database.

The token-import smoke chain is strict by default too: an empty model inventory,
a missing selected model, or a structured 503 is failure, not successful relay.
`E2E_SKIP_RELAY=1` is the explicit management-only mode for a fixture with no
provider; it skips relay key/route setup and model calls and counts those steps
as SKIP. `SKIP_MODEL_FETCH=true` by itself does not relax relay acceptance.
Repeated full login journeys can hit the upstream's real sensitive-operation
rate limit. Honor its cooldown or reuse a valid management credential; do not
disable the guard or automatically retry PAT creation to make a test green.
Run live model acceptance when the test host is idle: the upstream may correctly
reject requests with `system_cpu_overloaded` while full local CI saturates it.

A route `displayName` is its downstream model alias, not just an internal label.
The smoke chains leave it unset on fresh routes and honor an existing alias
when relaying; the account model and the public model ID need not be identical.
The instrument fixtures cover both route creation and alias reuse.

The protocol report records the configured token cap and allowlisted terminal
signals on failure. `length` remains FAIL; use an explicit, bounded larger cap
when a reasoning model needs it, rather than relaxing completion validation.
Repeated terminal signals and `end_turn` with tool calls remain visible failures.
