# Model Catalog & Probing

> **Index**: back to [API Reference](../api.md). This file is the Marketplace, price compare, probes & model redirects domain split out of the pre-`docs/api/` `docs/api.md`.

## Model Marketplace & Probing

### GET /api/models/marketplace

Model supply from site accounts (including OAuth accounts), direct upstream
grants, and enabled exact public routes. Catalog metadata enriches these rows;
catalog-only entries do not add supply.

Each row retains `accountCount`, `tokenCount`, and `accounts`, and also returns
`supplyCount` and `supplySources`. The supply count is one per account with an
availability record plus one per direct grant, deduplicated within that model.
It is not a count of eligible routing candidates. Direct upstream model names
appear even before route binding; enabled exact public aliases also show the
grants bound to them. Wildcard/regex and explicit-group expansion are not
projected as additional public model names.

`supplySources` entries contain `kind` (`account` | `oauth` | `direct`), `id`,
`name`, `connectionName`, `upstreamModel`, `enabled`, `evidence`, and `bindings`.
Use `(kind, id)` as identity: account/OAuth IDs refer to accounts; direct IDs
refer to grants, with separate `channelId`, `credentialId`, and `credentialKind`.
Account entries include `siteId`. Credential values, headers, OAuth state, and
upstream URLs are excluded.

For account/OAuth sources, `upstreamModel` is the name in the availability
record, labeled "Recorded model" in the UI. It may be normalized; the selector
still resolves route mappings and per-account redirects at dispatch. For direct
sources it is the bound upstream model name used when that grant is selected.

For direct supply, `evidence: "configured"` means stored configuration;
`enabled` is the conjunction of channel, model, credential, and grant switches.
Disabled supply remains visible. `successCount` and `failCount` are cumulative
grant request observations (zero means no recorded outcomes), not verification
verdicts or a recent success rate. Account/OAuth `evidence: "availability"`
means an existing available record, possibly discovered or manually configured;
it does not prove successful inference.

Direct `bindings` contain `itemId`, `routeId`, `publicModel`, `routeEnabled`,
`groupEnabled`, and `selected` (whether a manual group selects that item; true
for other modes). Bindings describe configuration, including disabled and
unselected members, not dispatch eligibility. Protocol grants, runtime health,
cooldowns, and downstream access checks remain owned by the existing selector.

**Query params**:

- `page`/`pageSize` — when `page` is present the endpoint returns the
  paged envelope (below); when absent it keeps the legacy `{ models, meta }`
  shape. `pageSize` clamps to 1–200 (default 50).
- `q` — case-insensitive substring over the model name, catalog description,
  catalog tags, and supply connection/site/account names, matched against the
  whole marketplace (not the returned page).
- `capability`, `endpointType`, `sourceKind` — comma-separated filters.
  Values match case-insensitively against catalog tags, supported endpoint
  types, and supply source kinds (`account`/`oauth`/`direct`). Multiple
  values inside one dimension are OR-ed; dimensions and `q` are AND-ed. An
  unknown `sourceKind` value matches nothing instead of disabling the filter.
- `sort` — one `column:asc|desc` term (the dotted form `column.asc` is
  accepted). Whitelist: `name`, `supplyCount`, `accountCount`,
  `tokenCount`, `avgLatency`, `successRate`, `priceInput`. Unknown
  columns fall back to `name.asc`; ties break by ascending name; models
  without a measurement (latency/success/price) sort last in both directions.

Filters and sort also apply to the legacy no-page response. The normalized
query context is echoed at `meta.query` (`q`, `capabilities`,
`endpointTypes`, `sourceKinds`, `sort`), so a shared link never needs its
raw parameters re-parsed to be understood.

**Response (page-gated)**: `{ "items": [...], "total": 240, "page": 2,
"pageSize": 50, "facets": { "endpointTypes": [{ "value": "openai", "count":
200 }], "capabilities": [...], "sourceKinds": [...] }, "facetScope":
"filteredExcludingSelf", "meta": { "refreshRequested": false,
"includePricing": true, "query": { "q": "", "capabilities": [],
"endpointTypes": [], "sourceKinds": [], "sort": "name.asc" } } }`.

`total` and `facets` are computed over the whole marketplace under the same
filter context — never from the returned page. Facets are disjunctive per
dimension (`facetScope: "filteredExcludingSelf"`): each dimension's options
are counted with that dimension's own filter removed, so selecting a value
never hides the alternatives.

### GET /api/models/marketplace/detail?name=<exact>

One fully hydrated marketplace row by exact model name, independent of
pagination — the resolver behind the UI's `?model=<name>` deep link, so a
model that is not on the loaded page still opens. Same row shape and the same
projection boundary as the list (no credential material). `400` when `name`
is empty; `404` when the name has no supply and no exact route in the current
aggregation. Response: `{ "model": { ... } }`.

### GET /api/models/price-compare

Cross-site effective model price comparison for operators.

Query: `model` (optional name/substring), `days` (default 30), `limit` (default 50), `topModels` (default 12 when model empty).

Returns `{ model, days, limit, sampleUsage, items: [{ siteId, siteName, platform, model, accountId, username, inputPerMillion, outputPerMillion, source, ratesSource, estimatedCostSample, observedSamples, configuredUnitCost, missingPrice, recommended }], meta }`.

`source` is one of `billing_details` | `observed` | `configured` | `fallback`. Fallback is always labeled; `missingPrice=true` when no catalog/observed/configured signal exists. Rows are sorted cheapest-first and `recommended=true` marks the best channel per model.

Alias: `GET /api/stats/model-prices` (same handler).

响应还包含 `pricingAccounts`、`currentPrices` 和 `officialPrices`。GET 只读取本地
快照。当前报价按模型分页：`pricePage` 默认 1，`pricePageSize` 默认 10、最多 50；
`currentPage` 返回 `page`、`pageSize`、`totalModels`，越界页码收至最后一页。
同模型的账号和分组保留在同一页；历史 `items` 的 `limit` 仍单独生效。
`currentPrices` 按账号和模型保存站点报价，每项的 `groups` 包含可用组的
有效倍率、输入/输出/缓存读写单价、相对官方倍率以及 1K 输入 + 1K 输出（无缓存）
示例费用。金额单位为 USD/百万 token，`perRequest` 为 USD/次。`null` 表示未知，
`0` 表示免费；官方基准为零时，相对倍率为 `null`。

New API 的 `group_ratio` 已包含当前用户组的特殊倍率覆盖，计算时只应用一次：
输入单价 = `2 × modelRatio × groupMultiplier`，输出与缓存分别再乘相应倍率。
分组以账号的 `usable_group` 和模型的 `enable_groups` 交集为准，`all` 展开为
账号全部可用组。缺少分组倍率或某项费率时保留未知。按次计费使用
`modelPrice × groupMultiplier`；表达式、阶梯及未知模式保留规则，不计算线性单价。

`officialPrices` 复用已配置的 basellm/models.dev 目录，返回匹配模型、供应商、
别名匹配、来源及获取/目录更新时间。目录没有价格的元数据记录保持未知；目录标准档
价格与站点的条件计费分别展示。历史 `items` 保持兼容，观测到的请求平均费用与
账号配置单价只作为历史信号，界面不把它们放入固定 token 示例。

### POST /api/models/price-compare/accounts/:accountId/refresh

管理员显式刷新一个账号的报价，最多等待 20 秒。复用账号的管理凭据、用户 ID、
代理和站点请求头读取上游 `/api/pricing`；当前支持 New API 系列适配器。
成功返回 `{ "success": true }` 并替换该账号快照。失败返回错误，保留旧报价及
原获取时间，同时记录本次失败状态。刷新无需发起模型调用。删除账号时删除报价快照。

前端把合法的 `items: []` 显示为空状态；`items` 缺失、类型错误或含非法行时
显示加载错误并可重试，不当作“没有报价”。

### GET /api/models/token-candidates

Available token candidates for route configuration.

### POST /api/models/check/:accountId

Refresh model discovery for one account. The response includes `success`,
`refresh` (including `status`, `modelCount`, and `models`), `rebuild`, and
`tokenBackfilled`. A discovered model is not proof of successful inference.

### POST /api/models/probe

Trigger a **scheduler-wide** model probe. The handler does not consume a request
body: `models`, `accountId`, and `wait` do not narrow or wait for this operation.
The 202 response (`queued`, `jobId`, `status: "pending"`) acknowledges the trigger,
not successful inference or completion. Inspect the resulting probe history.

For a bounded operator test, use `POST /api/models/verify-batch` with explicit
`accountId`, `models`, and `limit`. That endpoint targets existing route channels;
`probed: 0` is not successful inference coverage.

### POST /api/models/verify-batch, GET /api/models/verify-history

Operator-initiated verification of route-channel model targets (same lightweight probe the background scheduler uses) with durable per-row history, and the recent verification rows.

**Body** (verify-batch): `{ "models": ["gpt-5"], "accountId": 0, "limit": 50 }` — empty `models` = all enabled route channels; `accountId > 0` narrows to one account; `limit` 1–200 (default 50). 503 when the probe scheduler is not running.
**Response** (verify-batch): `{ "success": true, "batchId": "vb-...", "probed": 2, "summary": { "success": 1, "failure": 0, "inconclusive": 1, "skipped": 0 }, "items": [ { "model": "gpt-4o", "channelId": 12, "accountId": 5, "siteId": 3, "status": "success", "latencyMs": 842, "httpStatus": 200, "errorText": "", "healthApplied": true } ] }` — no matching targets answers with `probed: 0` and a `note`.
**Query params** (verify-history): `limit` (default 50, max 200), `model` (exact match). Response: `{ items: [{ id, batchId, model, channelId, accountId, siteId, siteName, status, latencyMs, httpStatus, errorText, createdAt }] }`.

---

## Model Redirects

Canonical→actual model name mappings, created by availability sync (`source: sync`) or operator edits (`source: manual`). Rows live in `model_name_redirects` and feed the in-process redirect registry (`service.ReloadRedirectRegistry`), so every write below reloads it immediately.

### GET /api/model-redirects, PUT /api/model-redirects/{id}, DELETE /api/model-redirects/{id}

List, correct, and delete redirect mappings.

**Query params** (list): `accountId`, `source` (`sync`|`manual`).
**Body** (update): `{ "actual": "gpt-4o", "source": "manual" }` — partial update; empty `actual` or an unknown `source` is 400, "nothing to update" is 400, missing row is 404.

**Response** (list): `{ items: [{ id, accountId, username, siteName, canonical, actual, source, lastSeenAt, createdAt, updatedAt }] }` (updatedAt descending). Update/delete: `{ "success": true }`.

### POST /api/model-redirects/generate, POST /api/model-redirects/apply

Generate sync mappings idempotently (per account or all accounts), or apply redirect fixes to models disabled by availability drift.

**Body** (generate): `{ "accountId": 0, "models": ["gpt-4o"] }` — `accountId: 0` = all accounts; omitted `models` = every available model of the target account; manual mappings are never overwritten. Response: `{ success, created }` (all-accounts mode adds `accounts`).
**Body** (apply): `{ "dryRun": false }` (default `true` = report only). Dry run: `{ success, dryRun: true, candidates, count }`; applied: `{ success, dryRun: false, removed, count }` — each removal writes a `model_redirect_applied` event.

---

## Model catalog data sources & sync

The merged model catalog is built from a DB-persisted registry
(`catalog_sources`), not from a single hardcoded feed. Sources are fetched in
list order and **earlier sources override later ones**; per-source sync status
(last success, last error, entry count) is recorded as it goes. Registered by
`RegisterCatalogSourceRoutes`; the runtime side lives in `service/catalogsync`
(`Manager`, `Store`).

All routes below sit on the admin surface: same auth, error envelope and
pagination conventions as everything else in [`conventions.md`](conventions.md).
When `PRICING_CATALOG_ENABLED=false`, source management and status reads remain
available. Sync requests and auto-sync configuration writes answer `409` with
`{"message": "pricing catalog is disabled (PRICING_CATALOG_ENABLED=false)"}`;
the `catalog-sources` CRUD routes keep working, because they talk to the store
directly.

### GET /api/models/catalog-sources

**Response**: `{"sources": [...]}` in the order the merge is applied.
**Errors**: `500` `failed to list catalog sources`.

### POST /api/models/catalog-sources

**Body**: `{ "name": "...", "url": "...", "type": "...", "enabled": true }`
(`type` and `enabled` optional).
**Response**: `{"source": {...}}`. When a runtime manager exists the registry is
reloaded immediately, so the next merge sees the new source.
**Errors**: `400` `{"message": "invalid JSON body"}` on malformed JSON; `400`
with the store's validation message on a rejected source; `500`
`failed to reload catalog sources` when the reload after a successful write
fails.

### PUT /api/models/catalog-sources/{id}, DELETE /api/models/catalog-sources/{id}

Update (same body as create, partial) or remove one source; both reload the
registry afterwards.
**Errors**: `400` `{"message": "invalid source id"}` when `{id}` is not a
positive integer; `404` `{"message": "source not found"}` when the row is gone;
`500` on a store or reload failure.

### POST /api/models/catalog-sync

Triggers a sync now — all sources, or one when a body is given.
**Body** (optional): `{"sourceId": 123}`; an absent or empty body syncs every
source.
**Errors**: `409` when the catalog is disabled (see above); `400`
`{"message": "invalid JSON body"}` when a body is present but malformed.

### GET /api/models/catalog-sync

**Response**: `enabled`, `autoSync`, `intervalMin`, `snapshot` and `sources`,
including per-source last success / last error / entry count. A disabled catalog
returns `200` with `enabled: false`, `autoSync: false` and the persisted sources.
Its `intervalMin` and `snapshot` have zero values; no runtime snapshot is available.
**Errors**: `500` on a source/status read failure; `503` when the catalog is
configured as enabled but its runtime is unavailable.

### PUT /api/models/catalog-sync/config

**Body**: `{"autoSync": true}` — the field is **required**.
**Errors**: `400` `{"message": "invalid JSON body: autoSync required"}` when the
body is malformed or `autoSync` is absent; `409` when the catalog is disabled.

---

## Probe history

Read-only exposure of `model_probe_results` for the row-level health bars on
the channels and accounts pages. The probe scheduler is the **only** writer, so
these endpoints are empty unless `MODEL_AVAILABILITY_PROBE_ENABLED=true` has
been running long enough to record something.

Both endpoints answer one bounded window-function query per page render (the
most recent N results *per entity*), which is why the tables do not issue a
request per row. The query is written to run on both SQLite and PostgreSQL.

### GET /api/channels/probe-history, GET /api/accounts/probe-history

**Query params**: `limit` — results per entity, default `20`, capped at `50`.
**Response**:

```json
{
  "limit": 20,
  "items": [
    {
      "channelId": 7,
      "results": [
        {
          "id": 1041,
          "status": "success",
          "latencyMs": 812.5,
          "httpStatus": 200,
          "errorText": null,
          "modelName": "gpt-4o",
          "createdAt": "2026-09-03T08:15:00Z"
        }
      ]
    }
  ]
}
```

The grouping key is `channelId` on the channels endpoint and `accountId` on the
accounts endpoint. Within one entity, results are newest-first, so
`results[0]` is the latest probe. `status` shares the probe vocabulary used
everywhere else: `success` | `failure` | `inconclusive` | `skipped`. `latencyMs`, `httpStatus` and `errorText` are
nullable — a probe that never got a response has no latency or HTTP status.
Entities with no probe results at all are simply absent from `items`.
**Errors**: `500` `Failed to load probe history` on a query failure.
