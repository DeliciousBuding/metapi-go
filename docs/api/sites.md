# Sites API

> **Index**: back to [API Reference](../api.md). This file is the Sites CRUD, detect/import/batch, probe & tags domain split out of the pre-`docs/api/` `docs/api.md`.

## Sites

### Connection templates in the create form

A **site** stores an upstream deployment URL and a `platform` adapter. An **account** stores the API key, management credentials, or OAuth identity used with that connection. Provider names and logos are display metadata; selecting a service such as Moonshot still uses the `openai` adapter.

The create form opens with frequently used management platforms and domestic
providers. The complete catalog has management-platform, model-service,
Coding Plan, and OAuth categories. API templates supply a name, URL, and existing
adapter. Management templates leave the deployment URL for the operator to enter.
OAuth entries open the existing authorization page; unavailable providers remain
disabled. Editing an existing site keeps the compact platform picker.

Changing templates updates empty fields and values filled by the previous template that the operator has not changed. Manually entered values are retained, with feedback identifying those fields. A conflicting manual platform choice prevents applying the template. Protocol labels such as **Chat / Responses** describe the template; they do not configure a protocol restriction or create a separate platform.

The following OpenAI-compatible defaults preserve their required path prefix through the form's URL normalization. Model discovery appends `/v1/models`; chat requests append `/v1/chat/completions`.

| Service | Saved site URL | Adapter |
| --- | --- | --- |
| Moonshot / Kimi | `https://api.moonshot.cn` | `openai` |
| SiliconFlow | `https://api.siliconflow.cn` | `openai` |
| Alibaba Bailian | `https://dashscope.aliyuncs.com/compatible-mode` | `openai` |
| Fireworks AI | `https://api.fireworks.ai/inference` | `openai` |
| Cerebras | `https://api.cerebras.ai` | `openai` |
| ModelScope | `https://api-inference.modelscope.cn` | `openai` |

The registry also includes the Chat and Messages entries for Bailian, DeepSeek,
Kimi, MiniMax, ModelScope and Qiniu; versioned GLM and Volcengine endpoints; and
Aliyun, GLM, Z.AI, Kimi, Doubao and MiMo plan endpoints. Definitions are checked
against [AxonHub's channel configuration](https://github.com/looplj/axonhub/blob/d96748f1598837b46272923580e4ec558a6d1071/frontend/src/features/channels/data/config_channels.ts)
and [New API's Coding Plan bases](https://github.com/QuantumNous/new-api/blob/1d4328e97417a043a161a0dd30a5b129be3ace49/constant/channel.go).
Versioned `/v3` and `/v4` bases retain their version when request paths are joined.

Templates supply compatible endpoint defaults, not a live availability or
credential check. Provider-specific parameter rewriting, asynchronous image tasks,
Vertex/GCP credentials and AWS request signing require their own adapter support.

After creating an API site, the next action opens the API-key form with that site
selected. Management gateways retain both session and API-key choices; OAuth
adapters open the OAuth connection page. In a blank account form, selecting a
model API site also selects API-key mode. Explicit credential choices and entered
credentials are preserved when switching sites.

When a site's adapter and normalized URL match a preset, a new API-key account
uses its model-discovery recommendation until the operator changes that option.
Presets that skip discovery are useful for plans without a model-list endpoint.
Suggested model names can fill the existing manual-model input; adding a model
still requires the explicit Add action and does not mark it as verified.

### GET /api/sites, POST /api/sites

List all sites. Create a new site.

### GET /api/sites/initialization-presets

Returns the connection presets used by the site template gallery, URL detection,
and creation validation. Requires admin authentication. Each array entry contains
`id`, `label`, `providerLabel`, `platform`, `defaultUrl`,
`recommendedSkipModelFetch`, `recommendedModels` (an array, including when empty),
and `docsUrl`. Vendor names are display metadata; `platform` is a registered
adapter such as `openai`, `claude`, or `gemini`.

The gallery includes official APIs, compatible services, and Coding Plan entries.
Selecting a preset fills the connection form; it does not verify a credential or
discover models. Send its `id` as `initializationPresetId` when creating the site
with the selected platform and URL. The server rejects unknown IDs and mismatched
platforms or URLs. Changing the URL or platform manually clears the selection in
the UI. OpenAI remains one platform; Chat Completions and Responses are distinct
wire protocols rather than separate provider IDs.

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
