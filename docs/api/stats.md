# Dashboard, Usage & Latency

> **Index**: back to [API Reference](../api.md). This file is the Stats & dashboard domain split out of the pre-`docs/api/` `docs/api.md`.

## Stats & Dashboard

### GET /api/stats/overview

One reporting window for the operator overview. `period` accepts `24h`, `7d`
(default), `30d`, or `all`; invalid values return 400. `all` includes all retained
logs, not deleted history. The returned `window` contains an inclusive RFC3339
`to` bound and, except for all time, a `from` bound. Pass these same bounds to
proxy-log drill-downs.

**Response**: `{ period, window, summary, points, siteAvailability, models }`.
`summary` contains totalCount, successCount, failedCount, totalTokensAll,
totalCost and nullable averageLatencyMs. `points` aggregates requests,
successCount, successRate, tokens and cost into UTC hours for `24h`, otherwise
UTC days. Empty buckets are omitted; clients may fill them with zero requests
and an undefined success rate. The first and last bucket can be partial.
`models` lists the five highest-cost models and an `other` bucket when needed.
`siteAvailability` lists up to eight enabled sites, failures first.

Summary, trend and model aggregates include unassigned logs and disabled sites;
the upstream table is explicitly limited to currently enabled sites. Costs are
estimates. Current balances, notices and maintenance are not historical metrics
and retain their separate endpoints. A failed aggregate query returns an error
rather than a misleading healthy zero.

### GET /api/stats/dashboard

Returns the admin dashboard snapshot.

**Response**: Site/account/token counts, total cost, usage summaries.

### GET /api/stats/proxy-logs

Query proxy request logs (server-side filtered and paginated).

**Query params**: `view` (`full`|`query`|`meta`, default `full`), `limit` (1–100, default 50), `offset` (≥0), `status` (`success`|`failed`), `search` (substring on requested/actual model), `client` (exact client family), `siteId`, `channelId`, `from`/`to` (RFC3339 bounds on createdAt), `latencyMin`/`latencyMax` (ms bounds).

**Response** (`view=query`/`full`): `{ items, total, page, pageSize }` where `total` respects all active filters; `view=meta`/`full` adds `summary` (totalCount/successCount/failedCount/totalCost/totalTokensAll), `sites`, `clientOptions`.

Each row describes one upstream attempt. `latencyMs` includes reading the buffered response or relaying the stream;
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
