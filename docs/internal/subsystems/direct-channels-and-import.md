# Direct 渠道、协议扩展与备份导入子系统契约

**最后更新**：2026-11-26

> 本文从 [`docs/architecture.md`](../../architecture.md) 下沉而来，专门承载 Direct 渠道多协议适配、OAuth/GCP 凭据族、JS 插件与 API v2 Provider 驱动、媒体/视频持久任务，以及外部备份导入的细粒度所有权与不变量。顶层包边界与请求主链路仍以 [`docs/architecture.md`](../../architecture.md) 和 [`docs/package_boundary_test.go`](../../package_boundary_test.go) 为准。

## 1. OAuth / GCP 凭据兑换与 Refresh Family

`service/oauth` 拥有凭据解析与供应商兑换，`store` 拥有记录 lease/CAS 和持久
refresh family。family 按签发供应商与 OAuth client 划分，用不可逆 token 指纹关联
同一旋转凭据的多个账号/direct 记录，不把渠道或来源当成签发身份。兑换前提交
消费意图；进程消失或 lease 到期只能转移所有权，不能证明旧 refresh token 未消耗。
成功结果和 generation/退休 alias 与凭据在同一事务发布；落库重试不重新兑换，
提交确认丢失时用发布 nonce 核实结果。结果未知则要求重新授权或替换凭据。
复用结果不改变接收记录的启用状态、来源身份、项目和模型/grant。

`handler/proxy` 在 OAuth/GCP 凭据解析后复用已有授权与路由资格校验，保留原
请求条件、流式意图和准入预算。兑换期间撤权、来源/端点/实际模型或有效请求
配置变化会拒绝本次派发，不拿旧 permit 改选新渠道；HTTP 和 WS 使用同一边界。
持久消费事实随一致快照备份与方言迁移保留，临时 lease 不复制。

## 2. 渠道插件（moejs Wire Hooks）与 API v2 Provider 驱动

`internal/channelplugin` 负责无 I/O 的 moejs wire hooks 与受限 provider 描述符；`service/channelplugins`
拥有安装元数据、版本 CAS 和不可变尝试快照，`store.channel_plugins` 保存本地可信代码。
渠道 `DirectRequestConfig.Plugins` 指定有序协议槽绑定，管理界面使用同一 manifest
渲染选项，不读取私有请求操作。请求、响应和 SSE 接入已有 handler pipeline；
compact 与 WS 独立绑定，后者在既有每轮 owner 捕获不可变快照，先观察原始用量
再投影事件，不给 hook 凭据/路由/重放能力；
路由、凭据、transport 和预算 owner 保持不变。合同与限制见
[渠道插件 API](../../api/channel-plugins.md)。

API v2 provider 通过独立 `DirectRequestConfig.Provider` 绑定，不混入有序
wire 链。新建时选中端点/目录的默认模板落入原配置，升级不移动目标；实际
请求由既有 HTTP/WS dispatcher 授权和执行，原始用量先于原生投影。
`service/upstream` 目录 owner 消费独立 models hooks，复用当前凭据解析，
有界分页且拒绝部分成功；预览 basis 绑定配置与程序 identity，应用沿原锁重验。
厂商响应通过独立 decoder 进入原生 JSON/SSE/WS 和聚合 owner；原始 observations
在执行代码前按不可变 manifest 选择字段，计费不从解码输出反推。原生投影和
wire hooks 位于解码后，已发生请求的 decoder 失败不重放。
provider 的匿名身份复用既有凭据 resolver 与 Ollama 端点资格；每页目录同样解析
当前身份，空 Key/解析失败不授予匿名权限。显式无认证目录不发送供应商凭据。

### 视频任务捕获、归档与离线维护

视频 driver 的 JSON/multipart submit/query/content 复用原任务 owner 与累计账本，原始 task ID
与用量在 decoder 前关联；解析/下载失败保留已经验证的累计差额。两阶段 content
先查询已授权 metadata，再给出无认证 CDN URL；`handler/proxy` 的下载 owner 与
原生视频共用，`platform` 固定已验证公网 IP，保留 Host/SNI，并隔离认证/TLS 设置。
`channel_plugin_versions` 只归档实际被持久任务捕获的已审程序，任务保存摘要引用。
旧任务用原代码/默认选项恢复，但当前安装启用与归属/端点/凭据资格仍独立验证。
`service/channelplugins.ListArchivedVersions` 只读任务程序摘要与引用数；管理员可通过
`RestoreArchivedVersion` 为缺失的持久引用恢复经有界编译、指纹匹配的原程序，
不改变当前安装或授权。已完成任务也计入引用，因为后续仍可取回内容。
新 provider submit 在目录事务内归档并向已有任务表写未确认捕获，响应 CAS 确认
原行；派发完成只释放未确认行。捕获丢失后不重建任务，保留清理不按年龄撤销捕获。
只有不可变 manifest 声明 `taskCapture:"durable"` 的无引用档案可在线沿同一锁回收；
显式 `deleteTask` / `remix` 是原视频任务消费者的写操作，不叠加内置 profile 转换。
remix 复用来源程序，在同一目录事务捕获新任务及来源公共 ID，派发前重新核对
当前安装资格；源与新任务分别累计原始用量。删除只在明确成功后移除原任务行，
供应商 204 空响应不执行 JS 解码。写操作失败或取消不跨槽、不重放。
旧宿主拒绝新声明，旧档案的零引用仍不是在途结束证明，遗留捕获也不自动过期释放。
旧档案/遗留捕获使用独立离线维护入口：操作者停止全部实例并备份后，指定精确
目标生成摘要计划；原目录锁、捕获行锁与快照/引用/程序校验控制一次事务回收，
不改变当前安装/权限/用量。数据库锁不能替操作者证明旧宿主已停止网络 I/O。
这不增加另一条程序版本轴或任务表，当前安装/归属/授权仍由原 owner 验证。

### Multipart、认证声明与图片任务流式

multipart driver 在原生 profile 预转换前接收字段和请求内文件引用；字节、文件句柄
与临时路径由 `handler/proxy` 的原 multipart owner 保留。JSON 占位符或 multipart
parts 在语义覆盖/预算之后由宿主最终编码，不给 JS 文件能力或另建上传执行器。
同步 JSON 槽与图片/音频 multipart 复用同一 descriptor/文件 owner；provider 绑定
按实际已选端点槽而非公共路径别名捕获。非 JSON 音频/字幕保留原 byte relay，不进
JS decoder。显式 provider 不隐式借原内置异步图片 profile 执行额外轮询/下载。
provider 的 `authentication` 是不可变 manifest 声明，由 `service/channelplugins`
用当前 API Key 材料设置自定义认证头或签发 HS256；HTTP/目录在头构造末端应用，
显式无认证端点不签发。WS 的短期签名头仅保存在当前下游会话，每轮仍重新解析
和授权，密钥/程序变化或临近过期会轮换身份；不新增全局 token 池或持久表。
持久任务使用原认证声明与当前凭据，CDN 下载不取得签名头。OAuth/GCP 兑换仍
属于原 resolver，认证声明不赋予 JS 密钥、网络或刷新能力。
显式 `imageTasks` 的四个图片槽由 `handler/proxy.directImageTaskOperation` 消费，
`store.DirectProviderBinding.ImageQueries` 保存每槽独立的查询目标与认证。
`internal/channelplugin/contract` 拥有模板/ID 编码，`service/channelplugins` 仅在
新建时按实际所选图片端点 origin 应用默认值；升级不移动保存目标。宿主提交一次
POST，沿原授权/凭据 owner 逐次复核 GET，共用一次 attempt 与请求 ceiling；
原始任务 ID/累计用量先于结果代码，失败不重放。图片物化复用原无凭据媒体下载
owner，`platform.WithoutRequestRedirects` 仅限制本次供应商请求，不全局改变 client。
查询前和材料解析后由原安装 manager 复核当前启用状态，兼容升级不替换本次
已捕获的程序与选项；下一请求捕获新版，不新增独立安装或任务状态轴。
图片流式由每槽显式 `imageTasks.stream` 声明；同一个 request-local session 通过
Read 推进 submit/query，Close 取消，JSON 与 SSE 不建立两套生命周期。
响应体在收到头时登记，取消与读取共享一次关闭；取消后不再输出已缓存帧。
预览去重、原用量先于投影、完整结果物化后输出原生图片事件；原 SSE owner 处理背压、idle、
字节上限与结算。分析器只保留元数据，不留图片正文。没有声明的旧模块仍拒绝流式。
这不是持久视频任务或新的任务平台，断开不授予图片任务恢复能力。
当前未另建 router、凭据池或任务平台；其他异步媒体任务消费者等
未列出的消费者尚未接通，不能据协议声明推断这些能力已可用。

## 3. 预设、请求覆写、流式策略与协议适配

产品预设由 `service/upstream_presets.go` 管理品牌、套餐、凭据类型和各协议的
默认主机；它们是连接的初始值，不限制高级自定义可以配置的协议。
`service/upstream_preset_endpoints.go` 从 `internal/upstreamadapter.Templates`
取得对应 wire profile 的端点模板，再组合成同一渠道的多个端点。

`internal/upstreamadapter` 同时拥有这些模板和实际请求准备、响应投影函数。
`handler/proxy` 把已选端点和已经解析的身份元数据交给 `Lookup(profile)` 的实现；
该 owner 不读取凭据、不发网络请求、不选择路由，也不重复实现已有跨协议转换。
原生标准接口共用透传实现，提供商差异由具体定义处理；专有媒体、视频和
OpenCode 的独立协议 owner 继续负责各自转换。

显式 `endpointConfig` 原样保存经过校验的多个 URL、认证方式和 profile，不再与
预设接口列表取交集。实际转发仍受端点配置、模型凭据 grant 和路由成员的
`protocolOrder` 共同约束；新增端点不会自动扩大存量授权。客户端原生协议优先，
需要转换时复用 `direct_protocol_bridge.go` 与 `transform`。站点管理面的
`platform.PlatformAdapter` 继续负责登录、余额、签到和令牌管理。

`internal/requestoverride` 拥有已校验的渠道有序请求操作与纯模板执行。
`service/backup` 将源配置编译为一份计划，`store.DirectRequestConfig` 保存
JSON 合同并由路由候选携带；`handler/proxy` 在协议转换前应用语义选项，
在 provider wire 准备后应用 body 操作，基础认证头构造后应用 header 操作；
provider 显式宿主认证在最后应用，不能被有序操作覆盖。
该 owner 不读取凭据、不选择路由、不发请求；每次尝试只修改自己的副本。

渠道 `stream_policy` 由 `store` 持久化，`routing/stream_policy.go` 在硬授权
和关联条件之后按原始客户端模式筛选。`handler/proxy` 只在已选 require
尝试强制上游流式，使用现有帧限制、总字节限制、idle 和取消控制；
`transform/streamaggregate` 负责原生文本协议的完整 JSON 聚合，成功后再交给
既有跨协议响应转换。该聚合 owner 不发请求、不改路由、不结算用量。
Direct Responses 的 HTTP、compaction 和 WebSocket 使用独立 endpoint/grant
位。`responses_ws.go` 负责下游逐轮鉴权，`direct_responses_ws.go` 将当前凭据、
渠道准入和显式代理接入既有 `codex_ws_runtime.go`；不建立第二个连接池。
运行时区分升级失败与已发送事件，后者不得被回退路径重放。

### 渠道配置的职责与执行

渠道由多个独立配置维度组成，不将供应商名称当作完整能力，也不将添加设置
视作扩大权限。管理界面和实际派发应沿用以下已存在的 owner：

| 配置维度 | 实现归属 | 不能隐含改变的边界 |
| --- | --- | --- |
| 供应商预设与协议端点 | `service/upstream`、`store.DirectEndpoints`、`proxy/profiles` | 预设只是初始配置；原生 HTTP、compact、WebSocket 分别配置与授权 |
| 凭据与身份生命周期 | `service/oauth`、`store` | API Key/OAuth/GCP 由对应 resolver 处理，不由请求 hook 接管 |
| 模型与访问授权 | `upstream_models`、`upstream_grants`、`upstream_group_items` | 模型发现、新增端点或插件安装不自动授予访问 |
| 选路与流式策略 | `routing`、`handler/proxy`、`transform/streamaggregate` | 硬授权先于偏好；流式聚合不扩大协议或用户预算 |
| 渠道流量与队列 | `internal/channellimit`、`handler/proxy` | 本进程并发/RPM 硬限制和实测 TPM 偏好分开；不宣称集群配额 |
| 请求自定义与插件 | `internal/requestoverride`、`internal/channelplugin`、`service/channelplugins` | 变换执行当前渠道尝试，不选择路由、不获得凭据或额外网络能力 |

渠道编辑页的高级设置按“流量与流式策略”和“请求配置”分区。前者直接消费
安全的详情字段，不触发私有配置读取；后者继续要求管理员显式打开。
`connectionPatch` 仅提交改动字段，对 `rateLimit` 按管理 API 的整份替换
合同保留其他限流维度。具体默认值、校验和生效范围见
[渠道管理 API](../../api/routes.md#patch-apiimported-upstreamsid)。

## 4. Direct 协议转换、媒体记账、限流与外部备份导入

### 外部备份导入所有权

Octopus v5 import stays within `service/backup`: `octopus_v5_types.go` defines
the source envelope, `octopus_v5.go` parses and checks compatibility, and
`octopus_v5_import.go` owns transactional ID remapping and persistence. Preview
and commit use the same compatibility checks; unsupported executable settings
remain blocking rather than being silently accepted.

AxonHub v1.4 import lives beside it: `axonhub_source.go` parses the source
envelope, `axonhub_providers.go` is the audited provider/auth table, and
`axonhub_compile.go` resolves model associations into the executable channel
graph. `PreviewAxonHubV14` and `ImportAxonHubV14` consume that one compiled
plan, so the preview cannot promise a channel the commit will not write; a
feature the direct relay cannot carry is a named per-channel skip, never a
guess.

Magpie import follows the same split: `magpie_backup.go` owns the encrypted
envelope, PBKDF2/AES-GCM opening, strict JSON parsing and source sections;
`magpie_compile.go` turns providers, groups and gateway keys into one
executable plan with explicit residuals; `magpie_import.go` writes that plan
in one transaction. The handler unwraps the passphrase-bearing body without
retaining it. Source identifiers, downstream-key scopes and removal revisions
use the shared `external_source_ids` lifecycle, so repeated imports and
origin replacement behave like the other external sources.

### Direct 多协议转换、RelayKit 与媒体任务记账

Direct channels use resolved per-protocol endpoint URLs and authentication.
`axonhub_protocols.go` resolves model protocol preferences onto individual route
items, leaving shared model/credential grants unchanged. The handler chooses an
allowed outbound protocol, preferring a client protocol match, and composes the
existing Chat, Responses, Messages and Gemini converters when needed. HTTP
dispatch, decompression, stream bounds and health accounting remain in the same
relay. Native bodies bypass conversion; converted streams retain upstream usage
before transforming the client-facing events. Converters reject unsupported
continuity and malformed or incomplete tool calls rather than inventing success.
Ollama adds a native Chat/NDJSON adapter through the Chat intermediate; Bedrock
Messages retains its native body and uses a bounded AWS EventStream decoder.
Anonymous credentials are explicitly typed, restricted to Ollama endpoints,
and resolved again after selection before any upstream request.

OpenCode Go resolves its internal wire only after selecting an authorized
logical Chat endpoint. Its typed internal URLs are separate from independently
configured endpoints and grants. Request and response conversion share the
resolved protocol; session identity survives channel retries. Bailian's tool
stream filter runs before client conversion, while Cline envelopes normalize
before shared usage and content analysis. Native Chat, Responses and Messages
streams require their actual terminal event; EOF alone is not success.

`store/direct_protocols.go` owns persisted endpoint-to-bit mappings. Media
selection requires an exact supported format, keeping Jina/OpenAI embeddings
and ModelScope/OpenAI image generation distinct. These requests use the same
dispatcher and accounting as conversation requests. MiniMax and ModelScope
profiles adapt their native image APIs; Codex images use its Responses tool
contract with a separate request model. Video task persistence stores ownership,
route graph references and configuration fingerprints and revalidates access
before task operations. External backup defaults enumerate source formats
explicitly, so adding a format locally cannot expand an old source permission.

`store/video_accounting.go` owns cumulative direct-video accounting. A task row
lock serializes updates to its usage high-water mark, fixed-time quota event,
managed-key cost and grant cost in one transaction. HTTP polling logs keep the
observed cumulative usage separately from the increment used by aggregate columns.
Budget-exempt result reads require durable task ownership before admission and
still undergo the handler's current route/credential checks.

Domestic Chat profiles share `handler/proxy/domestic_chat_request.go` across
direct channels and exact native-site presets. Messages tool continuation uses
the existing bounded replay cache; direct entries bind route/item/grant/model,
resolved credential and wire configuration, and recheck routing authorization
before selecting their negative internal item ID. These IDs are not accepted
through the external tester header. Responses/Chat plain reasoning conversion
is stateless and lives entirely in `transform/openai/responses`.

`transform/relaykitbridge` adapts New API RelayKit to the direct conversation
dispatcher. An attempt selects its conversion owner before I/O and keeps one
request/response session. Ordinary text and function-tool conversions use
RelayKit; provider wire contracts, Messages replay and requests with specialized
image/schema/state requirements keep their dedicated converters. Native requests
remain byte-preserving. Conversion errors never trigger a second conversion
engine. A precision-preserving JSON codec is installed once, loss diagnostics
are checked, and source terminal events are verified before stream finalization.
Transport limits and billing continue to use the original upstream response.

Direct channels prefer the authorized native client protocol. Explicit endpoint
or model rejection with no output or measured usage can try another authorized
protocol; authentication, throttling, transport failures and partial streams do
not replay within the channel. Each candidate starts from the original request.

### 渠道限流、路由亲和与本地目录图锁

`internal/channellimit` owns process-local direct-channel concurrency, FIFO queues
and natural-minute RPM/observed-TPM counters. Routing reads immutable load
snapshots after authorization; the dispatcher waits before acquiring the
independent credential safety slot, consumes RPM immediately before each logical
upstream attempt and records final measured usage once. Rate pressure can change
candidate preference but cannot widen grants or become a second billing ledger.

`store.DownstreamAccessPolicy` owns Key strategy/trace overrides; routing resolves
them over the matched route. The selector's bounded trace-affinity cache stores
only completed successful selections, keyed by an ingress-provided opaque
Key/model namespace and current route mapping. Preferred selections re-use the
normal condition, authorization, cooldown and load checks. HTTP and Responses
WebSocket turns share this identity through request context; queue revalidation
reloads permissions without changing the turn's identity.

`handler/admin/imported_config.go` owns imported connection updates and explicit
request-setting reads; `imported_members.go` owns route preferences and shared
grant cooldowns. All successful writes invalidate routing state. The channels
UI separates account-derived routing health from independent upstream maintenance;
it reads secret-bearing request settings only on demand, outside Query caches.
`service/upstream` owns local channel, model, grant and group creation and the
transactional deletion graph. Local records use `native:local` and negative
source IDs derived from their database IDs, without external source mappings.
Model/credential grants are explicit; creating either side never authorizes their
Cartesian product. The platform quick-connect workflow resolves one product's
complete endpoint set and discovers models before committing its channel,
credential, grants and route memberships atomically. Default discovery and empty
results stay distinct from verified model listings. Each group is paired with one public route, and grant protocol
reductions cannot invalidate a member's explicit protocol order.
Catalog writes, existing connection/member edits, deletion commits and source
replacements take the same graph lock before row locks. Deletion follows actual
relationships across origins, clears all mapping aliases and stale manual active
IDs, and preserves fail-closed downstream route permissions. Source import and
the admin deletion API share this owner and revision-based impact confirmation.

`internal/httpclient.ExpandClientHeaderTemplate` owns the request metadata
template parser and allowlist. Both backup validation and direct forwarding
call it, so accepting a template at import time cannot bypass the forwarding
policy. It never expands credentials or cookies and returns no partial value
for invalid templates.

The settings import workflow owns its reviewed payload snapshot and mutations.
`BackupImportPreviewPanel` only displays the plan and reports acknowledgement;
it sits beside the import inputs, before the independent WebDAV configuration.
Blocking plans cannot claim full support or offer partial-import acknowledgement.
