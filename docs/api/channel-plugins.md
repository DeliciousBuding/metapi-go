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
当前执行路径支持 `chat`、`responses`、`messages`、`gemini`、`responsesCompact`
与逐轮 `responsesWebsocket`。其他协议槽及媒体操作可以声明，但尚未接入完整
生命周期，绑定后不能隐式回退到无 driver 转发。

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
失败的 resolver 或其他供应商自动变成匿名访问。自定义认证签发尚未接入。

端点默认模板与目录配置可显式选择 `auth:"none"`；模板不绕过端点 owner 对
匿名 profile 的限制。目录不发送供应商凭据，匿名身份仍须显式声明 `none` 并在
每页通过当前凭据 resolver 校验；已禁用、替换或不支持的身份在网络请求前拒绝。
不支持的凭据类型返回 400；代码或配置变化仍返回 409。缺少 resolver 不视为匿名成功。

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
