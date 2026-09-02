# Model Catalog & Probing

> **Index**: back to [API Reference](../api.md). This file is the Marketplace, price compare, probes & model redirects domain split out of the pre-`docs/api/` `docs/api.md`.

## Model Marketplace & Probing

### GET /api/models/marketplace

Available models by site.

**Query params**: `page`/`pageSize` — when `page` is present the endpoint
returns `{ items, total, page, pageSize, meta }` where `total` is the full
marketplace row count and `pageSize` clamps to 1–200 (default 50); when
`page` is absent it keeps the legacy `{ models, meta }` shape. Filtering and
sorting are not server-side today, so the frontend applies them over the
returned page only.

**Response (page-gated)**: `{ "items": [...], "total": 240, "page": 2,
"pageSize": 50, "meta": { "refreshRequested": false, "includePricing": true } }`.

### GET /api/models/price-compare

Cross-site effective model price comparison for operators.

Query: `model` (optional name/substring), `days` (default 30), `limit` (default 50), `topModels` (default 12 when model empty).

Returns `{ model, days, limit, sampleUsage, items: [{ siteId, siteName, platform, model, accountId, username, inputPerMillion, outputPerMillion, source, ratesSource, estimatedCostSample, observedSamples, configuredUnitCost, missingPrice, recommended }], meta }`.

`source` is one of `billing_details` | `observed` | `configured` | `fallback`. Fallback is always labeled; `missingPrice=true` when no catalog/observed/configured signal exists. Rows are sorted cheapest-first and `recommended=true` marks the best channel per model.

Alias: `GET /api/stats/model-prices` (same handler).

### GET /api/models/token-candidates

Available token candidates for route configuration.

### POST /api/models/check/:accountId

Check model availability for a specific account. Returns `{ "success": true, "models": [...] }`.

### POST /api/models/probe

Trigger a model probe. Body: `{ "models": ["gpt-4o"], "wait": false }`. Returns 202 with probe job.

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
When `PRICING_CATALOG_ENABLED=false` there is no runtime manager, and the
`catalog-sync` routes answer `409` with
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

**Response**: the manager's status — the auto-sync flag plus per-source last
success / last error / entry count.
**Errors**: `409` with `{"message": ..., "autoSync": false}` when the catalog is
disabled.

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
