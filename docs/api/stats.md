# Dashboard, Usage & Latency

> **Index**: back to [API Reference](../api.md). This file is the Stats & dashboard domain split out of the pre-`docs/api/` `docs/api.md`.

## Stats & Dashboard

### GET /api/stats/overview

One reporting window for the operator overview. `period` accepts `24h`, `7d`
(default), `30d`, or `all`; invalid values return 400. `all` includes all retained
logs, not deleted history. The returned `window` contains an inclusive RFC3339
`to` bound and, except for all time, a `from` bound. Pass these same bounds to
proxy-log drill-downs.

**Response**: `{ period, window, summary, requestSummary, points, requestPoints, gatewayRequestSummary, gatewayRequestPoints, siteAvailability, models }`.
`summary` 和 `points` 按已记录的上游尝试统计，包含重试。
`requestSummary` 和 `requestPoints` 按非空 `requestId` 归组，以最高
`retryCount`（同序数按 id）的已记录尝试判断成败：两次失败后一次成功计为一次
成功。未派发、因而没有尝试日志的请求不计入。
没有 `requestId` 的行不进入请求分母，单列为 `legacyAttemptCount`；该值
大于零时，`partial` 为 true，`partialReasons` 包含 `requestIdMissing`。
`requestPoints` 按各组末次尝试的写入时间分桶。
上述 `requestSummary` / `requestPoints` 仅保留旧版「末次尝试」口径兼容，
不是网关终态。新界面应使用 `gatewayRequestSummary` / `gatewayRequestPoints`：
从真实请求终态表统计，同样使用 `period` 与 `window`，按 `startedAt` 分桶，
包括已观测到的零次派发请求，不从历史尝试推断或补造结果。
`gatewayRequestSummary` 为 `{totalCount,completedCount,failedCount,cancelledCount,rejectedCount,coverageStartedAt}`；
`gatewayRequestPoints` 为 `[{date,requests,completedCount,failedCount,cancelledCount,rejectedCount}]`。
`failedCount` 合计上游失败、输出中断和网关失败，取消与拒绝独立计算。
`coverageStartedAt` 是当前保留的真实请求中最早的 `startedAt`，无记录时为 null；
它不证明之后没有记录失败，也不代表迁移以前的请求已覆盖。
`models` lists the five highest-cost models and an `other` bucket when needed.
`siteAvailability` lists up to eight enabled sites, failures first.

Summary, trend and model aggregates include unassigned logs and disabled sites;
the upstream table is explicitly limited to currently enabled sites. Costs are
estimates. Current balances, notices and maintenance are not historical metrics
and retain their separate endpoints. A failed aggregate query returns an error
rather than a misleading healthy zero.

### GET /api/stats/dashboard

Returns the admin dashboard snapshot.

**Response**: Site/account counts, `upstreamChannelCount`, total cost and usage summaries.

The summary and full views include `upstreamChannelCount`, the number of stored
independent upstream connections. It is separate from `siteCount` and
`totalAccounts`; account-backed and OAuth connections retain the account count.
These are configuration counts, including disabled records, not availability or
successful-request measurements. A failed count is `null` and appears in the
dashboard's failed-metric status. The UI keeps unanswered counts unknown.

### GET /api/stats/requests

查询网关观测到的终态。HTTP 请求与 WebSocket turn 各自一条记录，
不把 WebSocket 握手当作模型 turn，也不承诺客户端已确认收妥。

**Query params**: `limit`（1–100，默认 50）、`offset`（非负，默认 0）、
`status`（下表中的精确终态）、`search`（请求模型、内部 ID、外部请求 ID、原因码的
不区分大小写字面子串）、`downstreamKeyId`（正整数）、`from` / `to`
（含边界 RFC3339，过滤 `startedAt`，支持小数秒及时间偏移）。非法值返回 400。
管理页面在浏览器端将本地时间输入与快捷范围转换为 UTC RFC3339。

**Response**: `{items,total,page,pageSize,summary,coverageStartedAt}`。
每行包含 `id`、`externalRequestId`、`downstreamPath`、`transport`（`http` / `websocket`）、
`modelRequested`、`downstreamApiKeyId`、`terminalStatus`、`reasonCode`、`httpStatus`、
`outputStarted`、`firstByteLatencyMs`、`firstOutputLatencyMs`、`latencyMs`、
`startedAt`、`endedAt`、`attemptCount`。未观测的可空字段保留 null；
WS turn 无独立 HTTP 响应时 `httpStatus` 为 null，而不是握手的 101 或推测的 200。
`firstByteLatencyMs` 为响应头时间，`firstOutputLatencyMs` 为模型输出事件时间；
两者不能互相代填。`summary` 与列表、`total` 使用全部相同过滤条件，
包含 `{totalCount,completedCount,failedCount,cancelledCount,rejectedCount}`；
`coverageStartedAt` 使用全部保留请求的最早 `startedAt`，不受列表筛选影响。

| `terminalStatus` | 含义 |
| --- | --- |
| `completed` | 网关观察到正常协议结束，不仅是 HTTP 2xx |
| `upstream_failed` | 上游失败，尚未开始模型输出 |
| `output_interrupted` | 已开始输出，但未正常结束 |
| `client_cancelled` | 网关观察到下游取消或断开 |
| `rejected_no_route` | 无可用路由，可能零次派发 |
| `rejected_policy` | 鉴权后策略或输入限制拒绝 |
| `gateway_failed` | 网关内部失败 |

内部 `id` 关联尝试；外部 `externalRequestId` 可重复，不能作为唯一身份。
同 ID 重复完成不会覆盖首次记录。历史 `proxy_logs` 仍可独立查询，
不会根据最后一次尝试生成请求记录。费用与 token 仍属于每次尝试，
不可只取最终成功尝试，也不可把未知量补成 0。
请求与尝试共用有界批写队列，各自 best-effort 写入；
异步窗口或写入失败产生的不完整关联通过详情中的 `partial` 表达。

### GET /api/stats/requests/:id

返回请求行全部字段、`attemptCount`、`partial` 和 `attempts`。`attemptCount`
由请求生产者记录实际派发次数，不根据当前已落库的尝试行数推测。`partial`
在已落库尝试数与该值不同（如异步写入尚未完成、记录失败）时为 true；
不能把这种空数组显示成「零次派发」。`attempts` 按
`retryCount`、id 升序，沿用 `proxy-logs` 的完整列表字段、计费投影和可空测量。
关联到新请求的 `retryCount` 是从 0 开始的实际派发序数，包含同通道的协议 fallback；
旧记录保留原重试循环序数。fallback 尝试可记录失败，但不因此额外触发通道健康惩罚。
零次派发返回空数组。内部 ID 不存在返回 404；请求、关联尝试或聚合读取失败
返回 500 / `resourceLoadFailed`，不降级为健康空列表。

### GET /api/stats/proxy-logs

Query proxy request logs (server-side filtered and paginated).

**Query params**: `view` (`full`|`query`|`meta`, default `full`), `limit` (1–100, default 50), `offset` (≥0), `status` (`success`|`failed`), `search` (substring on requested/actual model), `client` (exact client family), `siteId`, `channelId`, `downstreamKeyId`, `from`/`to` (RFC3339 bounds on createdAt), `latencyMin`/`latencyMax` (ms bounds).

**Response** (`view=query`/`full`): `{ items, total, page, pageSize }` where `total` respects all active filters; `view=meta`/`full` adds `summary` (totalCount/successCount/failedCount/totalCost/totalTokensAll), `sites`, `clientOptions`.

Each row describes one upstream attempt. `latencyMs` includes reading the buffered response or relaying the stream;
`downstreamPath` is a nullable canonical client API path captured for successful and failed dispatches, independently
of the chosen outbound protocol. It omits URL parameters and replaces Gemini model identifiers with `{model}`;
streamGenerateContent, count-tokens, Responses compact and Gemini CLI keep distinct paths. Unrecognized or
resource-specific paths and historical rows stay null. Error text and upstream paths do not supply this value.
The detail UI can open the current route workspace for supported recorded interfaces, using `modelRequested`
and the recorded downstream key when present. Without a key it is an administrator preview. This checks current
configuration, not the historical selection; unsupported or unknown interfaces have no diagnostic shortcut.
`cacheReadTokens` and `cacheCreationTokens`, when present, are validated projections of stored billing usage.
Unknown, invalid or absent cache counts stay absent; cache counts are not added to total tokens or used to invent
a cache-hit percentage. Billing detail sections normalize legacy snake_case fields to the camelCase API contract.
`upstreamReportedModel` is nullable original response metadata observed before downstream model-name rewriting.
It is separate from `modelRequested` and the selected outbound `modelActual`. Known Chat, Responses, Messages and
Gemini JSON/SSE model fields are captured per attempt, including streams that omit usage; arbitrary nested content
and error-only model fields are not scanned. Missing and historical observations remain null. Different aliases can
be legitimate; neither agreement nor disagreement proves the provider or actual model identity.

`firstByteLatencyMs` measures time to response headers (TTFB), not the first generated token. Attempts that fail before
receiving headers keep it null. `firstOutputLatencyMs` is a separate nullable measurement for SSE: elapsed time to
the first complete, recognized text, reasoning or tool-call output event at the relay, excluding comments, heartbeats,
role-only, lifecycle and usage events. It is event-level first output, not token-level TTFT. Buffered JSON, unknown
protocol output, unreadable encoded streams and historical rows have no first-output measurement. No historical
value is inferred from headers or total latency. Protocol bridging measures the output-bearing event in the relayed
protocol; network chunking and bridge buffering can affect the observation.
Retries remain separate rows. New attempts carry nullable `gatewayRequestId` for
the unique gateway request/turn identity; historical rows remain null. The existing
external `requestId` and `retryCount` fields are retained for compatibility.
Earlier builds recorded header latency as total latency on successful responses; existing rows retain those original values.

### GET /api/stats/proxy-logs/:id

Get a single proxy log entry by ID.

记录不存在返回 `404`；数据库读取失败返回 `500` / `resourceLoadFailed`。

`requestAttempts` 对新记录优先关联同一 `gatewayRequestId`，历史缺少内部关联时
沿用 `requestId`。外部 ID 重复不会合并新的独立请求或 WS turn。新关联记录的
`retry_count` 是实际派发序数；旧记录仍是重试循环序数。结构如下：

```
requestAttempts: {
  requestId: string | null,
  currentOrdinal: number,          // this row's retry_count
  attempts: [{ id, ordinal, status, httpStatus, channelId, siteId, siteName,
               accountUsername, modelActual, errorMessage, latencyMs, createdAt, isCurrent }],
  lastAttempt: { status, httpStatus, logId } | null, // highest-ordinal recorded row; tie-break by id
  terminal: { status, httpStatus, logId } | null,  // deprecated compatibility projection; not a gateway outcome
  partial: boolean,
  partialReasons: string[]          // "requestIdMissing" | "ordinalGap" | "attemptQueryFailed"
}
```

`lastAttempt` 是关联结果中序数最高的一行；缺少 `requestId`、关联查询失败或
无关联行时为 null，未知的 `httpStatus` 保持 null。`terminal` 仅为兼容旧客户端
保留。尝试状态不等于网关返回给客户端的状态：重试耗尽后网关可能另返 503，
流也可能在 HTTP 200 之后中断。

`partial` 标明已知缺口：`requestIdMissing`（无法关联，`attempts` 为空）、
`ordinalGap`（序数有空洞，如站点并发饱和跳过）、`attemptQueryFailed`
（关联查询失败）。缺口只标注，不补造记录。

### GET /api/stats/site-distribution

Token distribution across sites.

### GET /api/stats/site-trend

Usage trend data by site.

### GET /api/stats/model-by-site

Model usage breakdown by site.

### GET /api/stats/usage-heatmap

Hour × site/model usage density for admin analytics.

**Query params**: `days` (1–31, default 7), `dimension` (`site`|`model`, default `site`)

**Response**: `{ dimension, days, since, source, cellLimit, count, cells:[{bucket,key,label,calls,tokens,spend}] }`. Bounded with hard `LIMIT` (2000). Prefers `site_hour_usage` for site dimension; model dimension aggregates `proxy_logs`. Never includes chat content.

### GET /api/stats/slow-requests

Top slow proxy requests by `latency_ms` within a time window.

**Query params**: `limit` (1–200, default 50), `minLatencyMs` (default 1000), `hours` (1–168, default 24)

**Response**: `{ hours, minLatencyMs, limit, since, count, items:[{id,model,status,latencyMs,firstByteLatencyMs,httpStatus,requestId,accountId,siteId,siteName,createdAt}] }`. Metadata only — no request/response bodies.

### GET /api/stats/attention

Severity-ranked actionable items for the dashboard ("what needs my eyes"): expired accounts (critical), low-balance accounts with `balance < 1.0` (warning), disabled sites (warning), and the last 24h of warning/error events.

**Query params**: `limit` (default 20, max 100).
**Response** (200): `{ "items": [ { "severity": "critical", "category": "expired_account", "label": "账号已过期：svc-1", "target": "/accounts?accountId=5", "createdAt": "2026-08-22T00:00:00Z" } ], "total": 1 }`. `severity` is `critical` | `warning` | `info`; `category` is `expired_account` | `low_balance` | `disabled_site` | `event`; `target` is a frontend deep link.

### GET /api/stats/balance-history, GET /api/stats/balance-income-outcome

Daily balance snapshots per account (`balance-history`) and the derived income/outcome accounting (`balance-income-outcome`).

**Response** (`balance-history`): `{ series: [{ accountId, points: [{ day, balance, balanceUsed, quota, capturedAt }] }], days }` — only days with actual snapshots are emitted.
**Response** (`balance-income-outcome`): `{ generatedAt, days, points: [{ day, income, outcome, net }], summary: { totalIncome, totalOutcome, net, accounts } }` — derived from the snapshots via `income - outcome = Δbalance`; an account's first snapshot day counts as initial income.
**Query params** (both): `days` (1–365, default 30), `accountId` (optional).

### GET /api/stats/latency-histogram, GET /api/stats/latency-trend

Request-count histogram over `latency_ms` buckets (`latency-histogram`) and the per-day latency/throughput series (`latency-trend`).

**Query params** (both): `days` (1–90, default 7); histogram adds `bucketMs` (100–60000, default 500).
**Response** (`latency-histogram`): `{ days, since, bucketMs, total, buckets: [{ bucketStartMs, bucketEndMs, label, count, percent }] }` — zero-count buckets are omitted.
**Response** (`latency-trend`): `{ days, points: [{ date, requests, avgLatencyMs, maxLatencyMs, avgFirstByteMs, p95LatencyMs, successRate }], truncatedDays }` — `truncatedDays` flags days whose p95 sample exceeded the 10000-row cap (honest under-reporting).

### GET /api/stats/model-cost-distribution

Top-N model cost concentration aggregated from `proxy_logs`; everything below top-N is grouped as `other`.

**Query params**: `days` (1–90, default 30), `topN` (1–20, default 8).
**Response** (200): `{ "days": 30, "since": "2026-07-24T00:00:00Z", "topN": 8, "items": [ { "model": "gpt-4o", "label": "gpt-4o", "cost": 1.23, "calls": 1200, "tokens": 450000 } ], "totals": { "cost": 4.56, "calls": 3000, "tokens": 1000000 } }`

### GET /api/stats/model-prices

Alias of `GET /api/models/price-compare` (same handler) — kept for frontend/back-compat.
