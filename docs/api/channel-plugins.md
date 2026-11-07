# 渠道插件：wire hooks 与 provider drivers

此接口属于管理鉴权面，不能使用下游 Key 访问。插件源代码只通过管理员
显式安装或详情读取；列表不返回源代码。`Cache-Control: no-store`。

## 管理生命周期

- `GET /api/channel-plugins`：`{items:[{key,revision,enabled,manifest?,loadError?}]}`。
  列表只读已校验的持久元数据，不执行插件。损坏条目保留稳定错误，不伪装为空列表。
- `GET /api/channel-plugins/:key`：`{key,revision,enabled,source}`，用于显式编辑。
- `PUT /api/channel-plugins/:key`：`{source,enabled,expectedRevision}`。安装使用
  `expectedRevision:0`；编辑、升级与停用使用当前 revision。响应为
  `{key,revision,enabled}`。旧 revision 返回 409，不能覆盖新版本或重新启用新停用项。
- `DELETE /api/channel-plugins/:key`：`{expectedRevision}`。只能删除已停用且
  revision 匹配的插件。停用/删除后的新请求显式失败，不回退到无插件转发。
  已经开始的尝试继续使用捕获的不可变版本，不在输出中途热换代码。
- `PATCH /api/channel-plugins/:key`：`{enabled,expectedRevision}`，不读取或
  重传源码的启停入口；同样使用 revision CAS，响应与 PUT 一致。

安装最多 128 项。升级与渠道写入使用数据库锁，检查所有保存的协议与选项绑定；
不兼容的升级拒绝。删除后重新安装即使复用 revision，也通过源代码/元数据摘要
区分缓存版本。`channel_plugins` 不进入可移植备份，防止备份导入安装或启用代码；
SQLite/PostgreSQL 方言迁移仍保留完整安装记录。

## 历史任务程序诊断与恢复

- `GET /api/channel-plugin-versions`：`{items:[{key,fingerprint,available,taskReferences,prunable?}]}`。
  管理员按需读取；只返回代码摘要和引用数，不读取或执行源码，也不返回任务 ID、
  owner、私有绑定或凭据。结果包含有引用但缺失的档案；读取失败、损坏/未知身份
  显式报错，不返回成功的空列表。`available` 仅表示档案行存在，不证明代码完整、
  可执行或当前已授权。列表最大 512 个存储版本加 1024 个缺失引用身份。
- `POST /api/channel-plugin-versions/restore`：`{key,fingerprint,source}`，响应
  `{key,fingerprint,restored:true}`。必须有持久任务引用，管理员提供原始已审源码，
  经原隔离编译器验证的程序 key/manifest/source 摘要必须精确匹配。错误程序
  返回 400，无任务引用返回 409；恢复同一原程序幂等，不覆盖损坏的不可变档案。
  仍受每 key 32/global 512 档案上限约束。错误响应不回显源码。
- `POST /api/channel-plugin-versions/prune`：`{key,fingerprint}`，响应
  `{key,fingerprint,pruned:true}`。只删除无持久引用且原程序声明
  `taskCapture:"durable"` 的档案；有引用或旧程序返回 409，非法/损坏程序拒绝。
  沿同一目录锁验证已审源码与指纹并重新扫描引用；不依赖列表中的 `prunable`
  提示。已移除且无引用的同一身份幂等返回成功，不读取或改变当前安装程序。

这些接口只接受管理身份且返回 `Cache-Control: no-store`。恢复不会安装、启用、
升级插件，不改变任务/渠道绑定，不绕过当前凭据和归属校验。可移植数据备份不
携带代码；管理员需另行保留并审查原源码，不能用最新版代码冒充旧指纹。
未确认捕获和已完成任务都计入持久引用数，完成任务仍可能取回内容。旧程序
没有持久捕获兼容声明，零引用不是其安全回收证明，不能在线删除。
回收不卸载插件，同一安装程序在后续任务中可以重新被捕获和归档。
界面使用不可逆操作的倒计时与输入确认词；网络/响应失败不声称已经回收，
重新读取引用后再允许操作。本接口不自动扫描并删除所有档案。

### 离线维护旧档案和遗留捕获

旧宿主未声明 durable 捕获，不能在线以零引用证明其已结束提交；崩溃遗留的
未确认捕获也不会按年龄失效。需要回收时先停止**所有**网关实例（包括旧版、
其他主机、后台自动重启），保留数据库及已审程序备份，在停止状态使用独立
`channel-plugin-maintain` 工具。它没有 HTTP apply 入口、不启动服务、不自动迁移。
数据库锁只协调存储操作，不能证明其他主机的网络请求已停止；
`--ack-all-instances-stopped` 是操作者的明确停机声明，不是自动探测结果。

从源码构建：`go build -o channel-plugin-maintain ./cmd/channel-plugin-maintain`。
数据库连接从 `--dsn-env` 指定的临时环境变量读取；该参数是变量**名称**，
不是 DSN，不读取默认或日常客户端凭据。SQLite 使用现有普通文件的绝对路径，
不接受 URI/符号链接或自动建库；PostgreSQL 使用该环境变量中的完整 DSN。

三个模式复用同一精确选择计划：

| 模式 | 输入与输出 | 是否回收 |
| --- | --- | --- |
| `inspect` | 重复 `--archive key:fingerprint` / `--capture publicId`；输出 JSON 计划 | 否，只读指定行 |
| `check`（默认） | `--plan /absolute/private/plan.json`；输出 `applied:false` 及预计数量 | 否，事务回滚 |
| `apply` | 同一计划，加 `--ack-all-instances-stopped`；提交后输出 `applied:true` | 是，一次事务 |

计划为 `{archives:[{key,fingerprint}],captures:[{id,snapshotSha256}]}`，每类最多
64 个目标、文件最多 64 KiB；拒绝空计划、重复选择、未知/重复 JSON 字段和
额外 JSON 值。不枚举或删除整个目录。输出只含摘要、公开不透明 ID 和数量，
不含源码、完整 identity、owner、绑定、账目或凭据；计划仍按私有运行材料保管。

捕获必须仍是 v3 direct provider 的未确认行、无站点/token、没有已初始化或
未知账目、没有确认后的状态。`snapshotSha256` 绑定完整非凭据行快照；ID
复用、身份/模型/时间戳/归属/账目变化都需重新 inspect。check/apply 在原目录
锁和捕获行锁下重验快照，复用全部持久引用索引与严格程序校验；任何损坏、
缺失引用来源、仍有其他引用或提交失败都拒绝整批，不部分删除。
显式选择的遗留捕获释放后，才允许回收已无引用的旧档案；已确认/完成任务不
作为捕获删除，当前安装/启停/绑定/用量表不变。旧档案不需要 durable 声明，
但仍需原 source/manifest/fingerprint 完整一致，不提供强制忽略损坏选项。

先 inspect 保存私有计划，再 check、人工核对停机/备份后 apply；禁止在服务
运行时使用停机确认参数。结果未知或已经成功的计划不能直接反复 apply：先在
停止状态查询精确目标，已删行报错而非假成功；只对仍需维护的行重新 inspect。
回收后需要恢复时使用停机前备份，不能以新程序替代旧指纹。

## Manifest 与 hooks

```js
export const meta = {
  apiVersion: 1,
  key: "vendor-options",
  name: "Vendor options",
  version: "1.0.0",
  protocols: ["chat"],
  hooks: ["request", "response", "event"],
  options: [{key: "mode", label: "Mode", type: "select", default: "safe", choices: ["safe", "fast"]}],
};
export function request(input) {
  return {
    body: [{op: "set", path: "vendor.mode", value: input.options.mode}],
    headers: [{op: "set", path: "X-Vendor-Mode", value: input.options.mode}],
  };
}
export function response(input) { return {}; }
export function event(input) { return {}; }
```

选项支持 `boolean`、`string`、`select`、`number` 和 `integer`；数值必须声明
`min`/`max`，范围限制在正负十亿内。未知选项或非法值拒绝，不静默清除。
`key` 为小写字母开头、最多 64 位的字母/数字/连字符。默认值属于 manifest。

hook 输入为 `{protocol,body,model,stream,options}`，`body` 是原生 JSON 文本，
不是经 JavaScript Number 解码的对象。输出为 `{body?:Operation[],headers?:Operation[]}`；
Operation 复用渠道的有序请求覆盖合同，值使用 JSON 文本，因此未修改的
64 位整数、对象顺序和原生扩展不会丢失。没有声明的 hook 保持原状。
`response`/`event` 禁止输出 header 操作，也不能改写用量、模型、错误或原生终态。
用量在这些输出操作前按原始上游字节记录；hook 失败不能触发已执行请求的重放。

## 渠道绑定

管理员显式读取 `/api/imported-upstreams/:id/request-config` 时取得
`pluginBindings:[{key,protocols,options?}]`，独立于不会回显的私有请求操作。
`PATCH /api/imported-upstreams/:id` 可只提交 `pluginBindings`；服务端保留
其他已保存的请求变换，不要求客户端读取或重写其内容。不能在同一补丁中
同时发送 `pluginBindings` 与整个 `requestTransform`。

每渠道最多 8 个有序绑定。协议是明确的 `chat`、`responses`、`messages`、`gemini`、
`responsesCompact`、`responsesWebsocket` 端点槽，不增加端点、模型 grant 或
transport 权限。普通 Responses 绑定不会自动覆盖 compact / WS。请求 hook 在
已有 provider wire 准备后、显式请求操作前执行；header 操作在认证头构造后执行，
保护认证/路由相关头；出站请求仍受用户准入预算检查。

`responsesCompact` 使用 `request` / `response`；`responsesWebsocket` 使用每轮
`request` / 每个原生事件的 `event`，不调用 HTTP `response` hook。WS 每轮捕获
不可变插件版本，升级/停用影响下一轮，握手头仍在认证构造后应用；变化后的
握手身份沿既有 session key 规则重连。取消、原生事件收集和无重放语义属于
原传输 owner。WS 从原始事件观测用量后才投影事件；hook 失败保留真实用量、
记录失败，关闭当前上游会话并返回稳定错误，不重发已经派发的请求。

这两个原生槽保护请求的 input、type、previous_response_id 和模式字段；响应
还保护 opaque output、item、工具参数、事件顺序及 continuation 身份，插件可改
非控制的内容/vendor 扩展，不把加密 compaction 内容或工具调用改成另一份会话。JSON 文本及
WS 的数字解析均保留精确整数。缺失端点、插件禁用或协议绑定不匹配不能隐式
降级到普通 Responses hooks。

API v1 wire hook 合同不是供应商插件 API：不拥有 OAuth 登录/刷新、网络请求、
模型发现或媒体任务生命周期。这些 owner 继续走现有路径。

## API v2 provider driver

provider 与有序 wire hooks 是两种独立绑定，共用安装、版本 CAS、选项校验和
隔离运行器。每渠道最多一个 provider；它不能选择路由、创建凭据或增加模型授权。
当前 request/response 接入全部已声明的原生同步 JSON 槽；completions/text SSE
沿原事件 owner 使用 event hook。`responsesCompact`、逐轮 `responsesWebsocket`
与视频持久任务保持各自独立消费者，视频能力见下文。实际执行仍须端点/grant/
协议绑定资格匹配；Jina、ModelScope、Ollama 等共享公共路径的槽以已选端点身份
绑定，不按客户端路径猜测或隐式回退到无 driver。

```js
export const meta = {
  apiVersion: 2, kind: "provider",
  key: "vendor-driver", name: "Vendor driver", version: "1.0.0",
  protocols: ["chat"], credentialKinds: ["api_key"],
  hooks: ["request", "response", "modelsRequest", "modelsResponse"],
  options: [{key: "mode", label: "Mode", type: "select", default: "safe", choices: ["safe", "custom"]}],
  defaults: {
    endpoints: {chat: {path: "/generate", auth: "bearer"}},
    catalog: {path: "/inventory", auth: "x-api-key", cursorParameter: "cursor"},
  },
};
export function request(input) {
  return {method: "POST", body: input.body, headers: {"X-Vendor-Mode": input.options.mode}};
}
export function response(input) { return {body: input.body}; }
export function modelsRequest() { return {method: "GET"}; }
export function modelsResponse(input) { return {models: JSON.parse(input.body).items}; }
```

`credentialKinds` 可声明 `api_key`、`oauth`、`gcp_service_account`、`none`；只是资格限制，
不授予插件读取 token、签发凭据或控制刷新策略的能力。OAuth/GCP 继续由宿主
解析，每页目录请求读取当前凭据，兑换后重验配置。`none` 仅消费宿主明确允许的
匿名身份：现有 Ollama/ollama-messages 的匿名端点与协议授权，不能让空 API Key、
失败的 resolver 或其他供应商自动变成匿名访问。

`requestKinds` 可显式声明 `json` / `multipart`，默认只有 `json`；空数组、重复项
或未知类型拒绝。它只约束创建请求，不影响旧任务查询；当前 multipart 消费者
为三个视频槽及 `imageEdit`、`imageVariation`、`audioTranscription`、
`audioTranslation`；其他协议不能从声明推断支持 multipart。旧模块不自动获得文件输入。
未声明的创建请求格式返回 415，且不派发、不归档新任务程序；插件解析/执行失败
继续按原错误合同处理，不把请求格式不匹配归类为上游网络故障。

端点默认模板与目录配置可显式选择 `auth:"none"`；模板不绕过端点 owner 对
匿名 profile 的限制。目录不发送供应商凭据，匿名身份仍须显式声明 `none` 并在
每页通过当前凭据 resolver 校验；已禁用、替换或不支持的身份在网络请求前拒绝。
不支持的凭据类型返回 400；代码或配置变化仍返回 409。缺少 resolver 不视为匿名成功。

### 宿主认证声明

API v2 provider 可选声明 `authentication`，仅允许同时声明
`credentialKinds:["api_key"]`。声明是不可变适配代码的一部分，不是 JS hook；
签名材料只来自本次已解析的当前凭据，不能放在插件选项或源码中。
API v1 wire hooks 不接受该字段。没有声明时保留原端点认证行为。
声明只作用于该 provider 实际绑定的协议槽和目录；未绑定的端点保留原认证。
需要 JWT 的 HTTP、压缩和 WS 槽须分别配置和绑定，安装或绑定 WS 不会改变
HTTP 的授权或认证。升级失败后的 HTTP 回退执行独立选择与绑定，不借用 WS 身份。

```js
// 自定义原始 Key 头
authentication: {type: "api_key", header: "X-Vendor-Key", prefix: "Key "}

// Kling 风格，凭据按 id|secret 保存
authentication: {
  type: "jwt_hs256", header: "Authorization", prefix: "Bearer ",
  jwt: {
    keySeparator: "|", issuerClaim: "iss", ttlSeconds: 1800,
    timeUnit: "seconds", notBeforeSkewSeconds: 5,
  },
}
```

`header` 是单个 HTTP 头名，`prefix` 可省略；禁止控制字符与 Host、Cookie、
Proxy、转发身份、连接、内容编码等控制头。`api_key` 不允许 `jwt` 子对象。
会话/账号/组织/项目标识、原生协议版本与 beta、请求 ID 和 trace 头同样保留给
原 owner，不能以认证声明覆盖；包括 Session-Id、Codex/Claude Code 会话头等。
`jwt_hs256` 算法固定 HS256，不接受可执行表达式、外部 JWK/签发 URL 或私钥。
`keySeparator` 为 `.` / `|`，`ttlSeconds` 为 60–86400 秒，`timeUnit` 为
`seconds` / `milliseconds`；`issuerClaim` 不得与 `exp` / `iat` / `nbf` 冲突。
可选 `issuedAtClaim` 不得覆盖 issuer/exp/nbf，`notBeforeSkewSeconds` 为 0–300，
`signType` 为空或 `SIGN`。Zhipu 风格使用 `.`、`api_key`、`milliseconds`、
`issuedAtClaim:"timestamp"` 和 `signType:"SIGN"`；仍以实际供应商合同为准。

HTTP（含原生压缩、multipart/二进制和任务 submit/query/content）在请求准备及
自定义头操作完成后注入认证；每页目录同样使用当前身份。显式 `auth:"none"`
不执行签发、不发送已解析凭据，不能被声明改成有认证请求。
宿主清除原 Authorization/X-Api-Key/X-Goog-Api-Key，再设置声明头，避免同时
发送原始材料与 JWT；同名客户端头、描述符或私有操作不能覆盖最终认证。
材料非法时在网络派发前明确失败，不回显凭据、不降级到原始 Key。
CDN 下载仍隔离认证，原 redirect/SSRF 防护和精确操作 URL 授权不变。

WS 每轮重新解析凭据和验证授权，在当前下游会话内最多保留一个短期签名头，
避免时钟变化让每轮建立新连接。密钥或程序变化、距 JWT 到期不足五秒时重新
签发，实际握手身份变化时由原 owner 轮换连接；不同下游会话不共享该值，没有全局 token 缓存或
持久 token 表。此机制不授予插件刷新 OAuth/GCP 的能力，后两者继续使用原 owner。
持久任务使用捕获时的认证声明和当前已授权凭据，升级程序不修改旧任务合同；
停用当前插件仍拒绝其后续操作。

### 绑定与管理表单

管理员显式读取 `/api/imported-upstreams/:id/request-config` 时取得
`providerBinding:{key,protocols,options?,catalog?}` 或 `null`。编辑使用
`PATCH /api/imported-upstreams/:id` 的同名字段；`providerBinding:null` 明确移除，
省略则保留。其他字段及嵌套值仍不接受 null。补丁不能同时提交
`providerBinding` 和整个 `requestTransform`。

新建原生渠道的 `requestTransform.provider` 使用同一绑定；快捷接入
`POST /api/imported-upstreams/connect` 可直接提交 `providerBinding`，其余 preset、
凭据与端点选择合同不变；provider 协议必须属于已选择/显式配置的渠道端点，
不会借默认模板扩大快捷接入的协议选择。直接新建时默认模板只补齐绑定中
选中的缺失端点/目录；
自定义地址优先，插件升级和普通编辑不重写已保存目标。编辑界面将 provider
选择器与有序 wire hook 列表分开，保留缺失/停用绑定，只有明确移除才清除。

`catalog` 为 `{url,auth,cursorParameter?}`：URL 使用 HTTP(S)，不含 userinfo、
query 或 fragment，origin 必须与绑定所选的某个渠道端点一致；WS 端点按其
等价 HTTP(S) origin 校验。未配置目录时仍使用渠道原有发现方式。新建时省略
目录可采用 manifest 默认目录，不能把省略解释为关闭该默认值。

### 执行与用量边界

输入为原生 JSON 文本与宿主提供的协议、模型、流式意图、凭据类型和选项。
调用方不能覆写已捕获的协议/凭据类型/选项。HTTP request 输出
`{method,path?,body?,contentType?,headers?}`；宿主验证方法、完整授权 URL 和
受保护头后，沿原 dispatcher 注入凭据并派发。相同 origin 的其他路径也不允许
借用本次操作授权。WS request 输出 `{path?,body,headers?}`，沿原 WS owner
每轮发送一次，不提供 JS 网络能力。

请求不可删除/影子覆盖实际模型、流式意图或输出 token 上限；重复 JSON 键
拒绝。用户预算在派发前校验最终输入，扩张输入或 hosted tools 不能借原预算
通过。响应/event 返回 `{body}`，在原始上游用量观测后作原生投影；模型、用量、
错误、终态及 opaque continuation 保持不变。失败不重放已执行请求。

### 厂商响应解码与原始事实

需要转换供应商 envelope/字段时，API v2 可额外声明 `decodeResponse` /
`decodeEvent`；它们返回 `{body:string}` 原生 JSON，位于现有 `response` / `event`
投影 hooks **之前**。JSON、SSE、required-stream 聚合及逐轮 WS 共用这一顺序，
不另建网络执行器。没有声明 decoder 时保留原生转发行为。

原始事实在 decoder 执行前由宿主按 manifest 的 `observations` 读取：

```js
observations: {
  chat: {
    response: {
      usagePaths: {
        prompt_tokens: "/meter/input",
        completion_tokens: "/meter/output",
        total_tokens: "/meter/total",
      },
      modelPath: "/engine",
      errorPath: "/failure",
    },
    event: {
      usagePaths: {completion_tokens: "/meter/output"},
      errorPath: "/failure",
    },
  },
}
```

键是已声明的协议槽，HTTP 使用 `response`，SSE/WS 使用 `event`。路径为 RFC 6901
JSON Pointer，最多 1024 字节/64 层；点号、通配符和 prototype 名称只是字面量键，
不执行查询或表达式。`usagePaths` 只允许既有 token 用量字段及 cache/reasoning
明细、Gemini TokenCount 字段，拒绝费用、倍率或授权字段。宿主保持原始数值类型，
再按既有整数/非负/预算上限和 cache 语义解释，不相信 decoder 提供的收费数量。

缺少字段表示本事件尚无该事实；显式零保留，非法/null counter 不当成缺失。
`errorPath` 的 null、false、数值零或空字符串表示无错误；其他存在值（含对象/数组）
表示上游失败，decoder 不能把它改成成功。
错误 envelope 不授予已验证的模型身份。声明的事实选择属于可信适配代码合同，
仍须核对实际供应商字段含义，不是凭空证明一条请求已经执行。

即使 decoder 抛错、超限或后续投影失败，已经观测到的原始用量仍记入同一次尝试；
已执行请求不重放，required-stream 聚合也不为确定的插件错误换源重试。
解码结果可重新组织厂商字段，但必须是所选协议的原生 envelope；输出中的用量
必须与宿主观测一致，不能制造未报告的 counter。已识别的原生控制字段继续保护，
不会把厂商自身的 `type` 字段误当成原生协议类型。
解码产物与 wire 投影拒绝新引入的控制字段大小写影子，避免协议解析器之间产生
分歧；检查限于协议拥有的根/response/message/用量容器，不扫描工具的自定义 JSON。

SSE decoder 根据解码后的原生 `type` 重建事件名，Chat/Gemini 使用默认 message；
保留原始 id/retry 与其他 framing 元数据。WS 以解码后的原生事件进入既有 collector，
分别限制原始/解码累计字节。opaque/精确整数应复用 JSON 文本或字符串，不先经
JavaScript Number 重建。用量合并、终态、取消与身份恢复继续属于既有 owner。

### 模型目录

目录须同时声明 `modelsRequest` / `modelsResponse`。前者返回 GET 描述符，路径
必须与保存的目录 URL 精确一致；游标由宿主注入，不允许插件自行指定 query。
后者返回 `{models:string[],nextCursor?:string}`，明确 `models:[]` 表示权威空目录，
缺失 models 是错误，不以推荐模型代替。宿主执行请求、拒绝重定向并限制单页
和总量：整个发现 15 秒、最多 100 页/10000 模型/16 MiB；重复游标或任何页失败
不返回部分成功。

目录刷新预览的 basis 还绑定当前配置、安装代码/manifest 的摘要；应用前在
原生命周期锁下重新验证，代码、目录或配置变化返回 409，需重新预览。
刷新发现仍不自动授予已有渠道的模型访问权；快捷接入沿用其显式初始化
模型、凭据授权和路由的既有行为，成功发现不代表模型推理已验证。

### 持久视频任务

`video`、`seedanceVideo`、`zenmuxVideo` 使用独立的 `submit`、`query`、`content`、
`taskResult` hooks，不调用文本 `request`/`response`。提交接受 JSON，显式声明后
也接受 multipart；不把文件字节或预转换的 base64 内容交给 JS。
查询和内容操作输入提供宿主捕获的 `taskId`。
请求 hook 仍返回受限描述符：提交 POST、查询与内容 GET，URL 必须保留宿主已
授权的完整操作地址。DELETE/remix 尚未接入，不借 query hook
重放写请求。已执行的媒体操作不自动重放。

#### multipart 与文件引用

显式 `requestKinds:["json","multipart"]` 的视频 provider，在 multipart submit
收到 `bodyKind:"multipart"`，`body` 为宿主完成模型映射/渠道参数后的字段 JSON
文本；单值字段为字符串，重复值为字符串数组，`stream` 的 true/false 是协议布尔
控制。文件不混入字段，`files?` 仅含 `{ref,field,filename,mimeType,size}` 元数据。
`ref` 是不可预测、请求内有效的引用，不能重用其他请求的引用；没有文件时可省略
files。字段与文件同名、重复 model/stream 或非法模型控制均拒绝。

`submit` 可返回两种输出，宿主都在语义覆盖与预算检查之后编码文件：

- `{method:"POST",parts:[{field,value:string}|{field,fileRef:string}]}`：宿主生成
  multipart boundary，保留重复字段以及所引用文件的原字节、文件名和 Content-Type；
  插件不能同时返回 body/contentType，或自行指定 Content-Type/Content-Length 头。
- `{method:"POST",body:string}`：原生 JSON 对象可在任意对象/数组深度使用
  `{__fileRef:ref,encoding:"base64"|"dataUrl",mimeType?,maxBytes?}`。宿主在自己的
  副本上替换占位符，保留未改数值的精度；严格拒绝未知字段、大小写影子和未知 ref。
  maxBytes 为额外的单文件上限，不能扩大宿主限制；遍历最多 64 层。

文件字节、reader、临时路径、认证信息不进入 moejs；编码后的文件也不会进入
后续 wire hooks。宿主复用原 multipart 字段/值/文件限制，最终编码同样受请求
总字节限制和取消约束；descriptor 最多 528 parts，输入最多 16 个文件引用。
使用 Seedance/ZenMux 原生 profile 时继续执行每文件 4 MiB 和图片类型限制；
未知 MIME 可沿原规则由宿主检测。JSON data URL 不依赖插件读取图片字节。
失败发生在上游派发前，不通过 query 或另一次 submit 重放来恢复。

`content` 支持两种明确模式：

- 直接返回 GET 描述符：读取已配置的同源内容端点，使用所选渠道认证，保持原行为。
- 首次返回严格的 `{query:true}`：宿主调用同一个不可变 driver 的 `query` hook，
  对原任务执行一次已授权的 metadata GET，不把 Range/If-Range 发给 metadata。
  验证原始事实和 `taskResult` 的 completed 状态后，再调用 `content`，输入 `body`
  为原始 metadata JSON，`state` 为 `{variant:string}`（最多 128 字节）。此时必须
  返回严格的 `{url:string}`；不允许指定认证头、请求方法或发起新任务。

第二种模式的 CDN 下载与原生视频下载共用宿主 owner，只发送 GET 及
Range/If-Range/Accept，不复制渠道认证、Cookie、自定义头或 TLS 校验跳过设置。
宿主校验全部 DNS 结果均为公网地址，固定到已验证 IP，并保留原 Host/SNI；
HTTP(S) 代理也通过 CONNECT 到固定 IP（包括 HTTP 下载），不回退到代理重新解析
目标。代理须允许目标端口。仅允许同源重定向，不允许 userinfo 或协议/主机/端口变化。
失败使用稳定错误，不将签名 URL 或供应商错误正文写入下载错误。

`taskResult` 返回 `{body:string}` 原生 video JSON，包含有效 `id`、`object:"video"`
和 `queued`/`in_progress`/`completed`/`failed` 状态。查询 ID 必须匹配原任务；原始
observations 先于解析执行，输出不能制造用量、改写已存在的原生控制或隐藏已观测
错误。宿主沿既有 public task ID、owner/grant/端点资格与累计用量账本处理任务。
已验证的创建/查询用量只计累计差额，重复查询及内容读取不重复计费。

视频 `observations.response.taskIdPath` 使用同一 RFC 6901 字面量选择合同，从
原始响应读取任务 ID；原生 `object:"video"` 响应的有效根 `id` 也属于原始事实。
ID 必须是非空有效字符串，不能含路径穿越片段或 CR/LF/NUL；上限 4096 字节。
其余供应商字符按已有任务 owner 的 `PathEscape` 合同进入路径，不收窄原生 ID 字符集。
显式 null、非法类型以及原始 ID/旧任务 ID/解码 ID 冲突均拒绝。
匹配已归属任务后，原始累计用量在执行 `taskResult` 前入账；解码、内容解析或
CDN 下载失败不抹掉已收到的差额。未能关联原始 ID 的旧模块仅在成功解析并校验
任务结果后建立关系，不声称其解码失败时已有经过验证的任务累计账本。
首次提交返回有效原始 ID 但解析失败时，请求仍失败，宿主保留归属任务映射，
通过该次尝试的 billing details 关联 public task ID，可继续查询；不制造成功状态。

新任务在执行 JS 或可计费提交前，在同一目录事务内将已审核程序保存到不可变的
`channel_plugin_versions` 并创建已有 `proxy_video_tasks` 中的未确认捕获。
其上游 ID 为空，不可查询或下载；身份只保存 key、fingerprint 与配置摘要，不含
源码或凭据。响应通过 CAS 写入有效上游 ID，不能在捕获丢失后重新创建任务。
已结束的尝试只释放未确认行，已确认任务保留；存储失败保守保留并记录诊断。
新保留清理器不按年龄删除未确认捕获。进程消失后的遗留捕获不能按租约或时限
自动释放，需在证明相关工作已停止后维护；当前没有这种自动证明/强制释放入口。

需在线回收的程序显式声明 `meta.taskCapture="durable"`，限包含 Video、Seedance
或 Zenmux 的 API v2 provider。旧 strict manifest 宿主拒绝该字段，不能无保护地
执行同一程序；新宿主也拒绝不创建持久引用的调用路径。声明成为不可变指纹的一部分，
升级不会给旧档案补写声明或改写旧任务指纹。普通旧模块继续可用，其旧档案不在线回收。
升级后和重开数据库时，旧任务继续使用原解析器及默认选项；更改绑定、请求配置、
当前凭据资格或停用/删除安装记录仍拒绝后续操作，不允许档案绕过当前权限。
缺失/损坏档案不会自动使用最新版。普通无 driver 的任务仍保持原身份版本。

档案每 key 最多 32 个、全局最多 512 个实际捕获版本；相同 fingerprint 幂等，
达到上限时在上游提交前拒绝新捕获，不删除仍需恢复的代码。档案不进入可移植
备份，方言迁移与一致数据库快照保留；恢复旧任务须另外保留原审核代码档案。
使用 driver 的任务身份版本为 3，旧程序明确拒绝，不靠忽略新字段执行。

### 其他原生消费者

同步图片编辑/变体、音频转写/翻译的 multipart 使用 request/response（不调用
视频 submit/taskResult），复用上文同一宿主文件引用与 parts/JSON 占位符合同。
模型映射、参数覆盖、重复字段、原文件字节/名称/MIME 及当前授权仍由原 owner
校验。JSON decoder 按所选原生 envelope 验证，原始用量先于代码执行；抛错、
错误 envelope、伪造用量或解码形状错误不重放已发生请求。

audioSpeech 的音频、转写字幕等非 JSON 响应沿原 media byte owner 流式转发，
保留 MIME 与字节限制/idle/取消，不执行 JSON response/decoder，也不把二进制
当 JS 字符串。没有原始计数时仍为 unknown，不从文件长度伪造 token 用量。
JS JSON 输入/输出仍受运行边界的 4 MiB 限制，不因为图片协议放宽沙箱。

显式 provider 绑定 OpenRouter/MiniMax/ModelScope 图片 profile 时，使用自己的
请求 payload，不叠加内置供应商字段改写或默认异步模式头；端点鉴权仍由宿主
构造，也保留这些 profile 原有的单次提交边界，失败不自动派发到另一凭据。
显式 provider 不借内置异步图片 profile 自动发起轮询/下载；未列出的异步
厂商任务生命周期须单独接入，不能让同步 JSON 绑定隐含授予 query/remix 权限。
内置无 provider 的原生 profile 继续使用其原消费者。

## 回退到旧插件合同

旧版本同时校验已保存的绑定和安装 manifest，不会静默忽略未知协议。
回退前先移除所有 compact / WS 绑定；若插件 manifest 仍声明这两个协议，
将其替换为只声明旧协议且兼容剩余绑定的版本。安装升级仍需当前 revision，
不能只改渠道绑定而保留旧程序无法加载的 manifest。

不保留该插件时，先从全部渠道移除它的绑定，再停用并卸载。只停用已绑定
插件会使请求明确失败，不是回退恢复步骤。保留升级前备份与插件源码，停止
服务后再切换旧二进制；重新启用渠道前核对旧版本的安装列表和实际请求。

回退到只认识 API v1 的版本前，还须从所有渠道清除 `providerBinding`，替换
或卸载 API v2 manifest。可移植备份保留绑定和目录，但不携带可执行代码；
恢复后应单独重新安装已审核的对应模块，再开放渠道请求。
回退到尚不支持 decoder/observations 的 API v2 版本前，先替换使用这些字段的
模块并核对实际原生响应，旧 manifest 读取不会静默忽略新能力。
回退到不支持 provider 匿名身份的版本前，同样先替换声明 `none` 的 v2 模块和
目录配置，或移除该绑定；不能用合成 Key 填充匿名身份来绕过旧版校验。
回退到不支持 `taskIdPath` 或两阶段 CDN content 的版本前，替换相关模块并
处理其持久任务；否则旧 manifest 或原始 GET 描述符校验会明确拒绝，不静默降级。
回退到不支持 `requestKinds` / 文件引用的版本前，同样替换声明该能力的模块及
其持久任务；新 manifest 在旧 host 会明确拒绝，不把 multipart 原文交给旧 JS。
回退到不支持 `authentication` 的版本前，替换相关已安装模块或移除其绑定，
并先完成或停用捕获了该声明的持久任务。只替换当前模块不能改写旧任务的认证
档案；旧 host 拒绝未知声明，不会自动使用原始 Key 或无认证执行。

## 运行边界

moejs 编译一次并共享不可变 bytecode；每个 hook 使用独立 runtime，不在租户间
保留可变状态。没有文件、网络、进程、凭据或动态代码执行能力，不加载外部模块。
Promise 结果拒绝，不能把 async 返回值当成成功的空对象。

源码上限 256 KiB，输入上限 4 MiB，输出操作文档上限 256 KiB；每个 runtime
分配上限 16 MiB，每个调用含初始化和结果序列化上限 250 ms，每个编译版本最多
四个并发调用。调用与冷启动等待响应 context 取消。JavaScript 异常只返回
稳定错误码，不将私有输入或源码内容写入错误/日志。
provider 描述符/结果上限 4 MiB，网络等待不占用 JS runtime 执行槽；各 transport
和目录仍执行其更严格的内容与生命周期限制。
