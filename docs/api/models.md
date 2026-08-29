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
