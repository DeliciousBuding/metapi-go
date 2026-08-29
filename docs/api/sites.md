# Sites API

> **Index**: back to [API Reference](../api.md). This file is the Sites CRUD, detect/import/batch, probe & tags domain split out of the pre-`docs/api/` `docs/api.md`.

## Sites

### GET /api/sites, POST /api/sites

List all sites. Create a new site.

### GET /api/sites/:id, PUT /api/sites/:id, DELETE /api/sites/:id

Get, update, delete a site.

### POST /api/sites/detect

Detect the platform for a single URL (used by the import wizard's auto-detect step). Tries site-initialization presets first, then hostname heuristics.

**Auth**: admin Bearer token.

**Body**:
```json
{ "url": "https://api.deepseek.com/v1" }
```

**Response** (200):
```json
{
  "url": "https://api.deepseek.com/v1",
  "canonicalUrl": "https://api.deepseek.com/v1",
  "platform": "openai",
  "siteType": "openai",
  "confidence": 1.0,
  "initializationPresetId": "deepseek-openai"
}
```

`confidence` is `0..1` (`1.0` when a preset matched). `initializationPresetId` is omitted when no preset matched. Returns `400` with `{"error":"Could not detect platform"}` when no platform can be resolved, and `400` with `{"error":"Invalid url. Expected non-empty string."}` for an empty/missing URL.

### POST /api/sites/import

Idempotent batch import of sites (and optional accounts) from the import wizard. A site is keyed by `(platform, url)`; duplicates are resolved per the chosen strategy.

**Auth**: admin Bearer token.

**Body**:
```json
{
  "items": [
    {
      "name": "DeepSeek",
      "url": "https://api.deepseek.com/v1",
      "platform": "openai",
      "globalWeight": 1.0,
      "maxConcurrency": 0,
      "duplicateStrategy": "skip",
      "accounts": [
        { "username": "svc-1", "accessToken": "sk-...", "apiToken": "" }
      ]
    }
  ],
  "duplicateStrategy": "skip"
}
```

`items[].platform` is optional (auto-detected when omitted). `duplicateStrategy` is `skip` (default) or `merge`; per-item `duplicateStrategy` overrides the batch default. `accounts` is optional; each account is stored as an API-key style credential (model discovery is deferred to background refresh).

**Response** (200):
```json
{
  "imported": 1,
  "skipped": 0,
  "failed": 0,
  "results": [
    {
      "name": "DeepSeek",
      "url": "https://api.deepseek.com/v1",
      "status": "imported",
      "siteId": 12
    }
  ]
}
```

`status` is one of `imported` | `merged` | `skipped` | `failed`; `reason` and `siteId` are omitted when not applicable. `imported` counts newly created sites plus accounts merged into an existing site; `skipped` counts idempotent no-ops; `failed` counts hard errors. Returns `400` with `{"error":"items is required"}` for an empty `items` array and `500` with `{"error":"Import sites failed"}` on internal failure. Site/route caches are invalidated when at least one site was imported.

### POST /api/sites/batch

Batch enable/disable/delete sites and toggle their system-proxy usage. Body: `{ "ids": [1, 2], "action": "enable|disable|delete|enableSystemProxy|disableSystemProxy" }`.

**Response**: `{ success, successIds, failedItems }` — site/routing caches are invalidated after any mutating action.

### POST /api/sites/{id}/probe-now, GET /api/sites/{id}/probe-stream

Probe the site's active channels (bounded to 32 targets, 8-way parallel, 30s wall clock): synchronously for `probe-now`, as server-sent events for `probe-stream`.

**Body** (probe-now, optional): `{ "modelName": "gpt-4o" }` — filters which results are surfaced.
**Response** (probe-now): `{ success, totalModels, available, unavailable, results: [{ channelId, accountId, model, status, latencyMs, error? }], complete }` — `complete: false` adds `truncated` and `reason`. `status` is `success` | `failure`.
**Stream** (probe-stream): `text/event-stream`; events are `probe-start` (`{ startedAt, streaming, modelFilter }`), one `probe-result` per probed model (same row shape), `complete` (`{ totalModels, available, unavailable }`), then a `data: [DONE]` sentinel. `?modelName=` filters streamed results. The stream clears the server WriteTimeout so long probe passes are not cut off.

### GET /api/sites/{id}/available-models, GET /api/sites/{id}/disabled-models, PUT /api/sites/{id}/disabled-models

Available (account+token merged, case-insensitive sorted) and disabled model lists for a site.

**Response** (GETs): `{ siteId, models: ["gpt-4o", ...] }`.
**Body** (PUT disabled): `{ "models": ["gpt-4o"] }` — deduped/trimmed, full replace; returns `{ siteId, models }` and invalidates the routing cache.

### PUT /api/sites/{id}/tags, PUT /api/accounts/{id}/tags, GET /api/tags

Global tag system: per-row writes and the aggregated index driving the Accounts/Sites filter chips.

**Body** (PUT): `{ "tags": ["prod", "priority"] }` (a raw JSON array is also accepted). Tags are trimmed/deduped and stored as JSON-array text; returns `{ success, tags }` (400 for a malformed body, 404 for unknown rows).
**Response** (GET): `{ items: [{ name, accounts, sites, total }] }` — union of all account + site tags, sorted by total usage descending (`total` = accounts + sites).

---
