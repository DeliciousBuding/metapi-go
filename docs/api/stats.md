# Dashboard, Usage & Latency

> **Index**: back to [API Reference](../api.md). This file is the Stats & dashboard domain split out of the pre-`docs/api/` `docs/api.md`.

## Stats & Dashboard

### GET /api/stats/overview

One reporting window for the operator overview. `period` accepts `24h`, `7d`
(default), `30d`, or `all`; invalid values return 400. `all` includes all retained
logs, not deleted history. The returned `window` contains an inclusive RFC3339
`to` bound and, except for all time, a `from` bound. Pass these same bounds to
proxy-log drill-downs.

**Response**: `{ period, window, summary, requestSummary, points, requestPoints, siteAvailability, models }`.
`summary` 和 `points` 按已记录的上游尝试统计，包含重试。
`requestSummary` 和 `requestPoints` 按非空 `requestId` 归组，以最高
`retryCount`（同序数按 id）对应的已记录尝试判断成功或失败。例如，两次失败
之后一次成功计为一组末次尝试成功。此口径不包含无尝试日志的请求，也没有
单独记录网关最终响应、流结束方式或客户端接收结果。
没有 `requestId` 的行不进入请求分母，单列为 `legacyAttemptCount`；该值
大于零时，`partial` 为 true，`partialReasons` 包含 `requestIdMissing`。
`partial` 只说明已知的记录缺口，不证明整个请求已完成。
`requestPoints` 使用各组末次已记录尝试的写入时间分桶，不推算请求开始时间。
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
Retries remain separate rows linked by `requestId` and `retryCount`.
Earlier builds recorded header latency as total latency on successful responses; existing rows retain those original values.

### GET /api/stats/proxy-logs/:id

Get a single proxy log entry by ID.

主记录的 SQL 执行或扫描失败返回 `500` / `resourceLoadFailed`，不会误报
`404`；记录确实不存在时仍返回 `404`。

`requestAttempts` 关联同一 `requestId` 的已记录尝试，`retry_count` 是重试
循环序数。它回答已记录了哪些尝试、使用哪个渠道及各次状态，不提供网关最终
响应或客户端接收结果。结构如下：

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

`lastAttempt` 只在关联查询成功且有记录时返回最后一行。缺少 `requestId`、查询
失败或查不到关联行时为 null；旧服务端没有此字段时，客户端也显示未知。
历史记录的 `httpStatus` 未知时保留 null，不补为 `0`。
`terminal` 为兼容旧客户端保留原投影：通常是关联结果最后一行，缺少关联或
查询失败时可能只是正在查看的一行。新客户端不据此推断末次尝试或请求终态。

`partial` 标明已知缺口：`requestIdMissing` 表示无法关联（`attempts` 为空）；
`ordinalGap` 表示循环序数有空洞（如站点并发饱和跳过）；`attemptQueryFailed`
表示关联查询失败。界面显示实际返回的记录数，不补造缺失尝试。
重试后网关可能返回 503，流也可能在已发出 HTTP 200 后失败；日志的尝试状态码
不能替代网关响应状态。没有派发的请求可能完全没有尝试日志。

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
