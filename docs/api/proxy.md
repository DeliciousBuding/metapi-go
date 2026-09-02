# Proxy-Facing Endpoints

> **Index**: back to [API Reference](../api.md). This file is the /v1/files & /v1/pricing domain split out of the pre-`docs/api/` `docs/api.md`.

## Error shape (/v1 surface)

All `/v1` surface failures — auth middleware, rate limits, body limit, and
handler errors — use one OpenAI-compatible envelope so SDKs and upstream-relay
integrations (e.g. new-api consuming Metapi as a channel) parse them uniformly:

```json
{
  "error": {
    "message": "Invalid API key",
    "type": "authentication_error",
    "code": "invalid_api_key",
    "request_id": "present when the ingress middleware set one"
  }
}
```

Status conventions (OpenAI-aligned):

| Case | Status | `type` | `code` |
|------|--------|--------|--------|
| Missing credential | 401 | `authentication_error` | `missing_api_key` |
| Unknown/invalid key | 401 | `authentication_error` | `invalid_api_key` |
| Key disabled / expired | 403 | `permission_error` | `key_disabled` / `key_expired` |
| Key IP policy denied | 403 | `permission_error` | `ip_blocked` / `ip_not_allowed` |
| RPM/TPM rate limit | 429 + `Retry-After` | `rate_limit_error` | `over_rpm` / `over_tpm` / `rate_limit_exceeded` / `global_token_rate_exceeded` |
| Budget exhausted (`maxCost` / `maxRequests`) | 429 | `insufficient_quota` | `insufficient_quota` |
| Body too large | 413 | `invalid_request_error` | — |

The admin surface (`/api/*`) keeps the flat `{"error", "errorCode"}` body
documented in [conventions.md](conventions.md); the two contracts never mix
(the global body-limit middleware picks the shape by path prefix).

---

## Proxy files (`/v1/files`)

OpenAI-compatible Files surface (proxy auth required). Forwards to the selected upstream channel; does not persist customer file bytes on Metapi disk.

| Method | Path | Notes |
|--------|------|--------|
| POST | `/v1/files` | Multipart upload (`file` field). |
| GET | `/v1/files` | List files from upstream. |
| GET | `/v1/files/{fileId}` | File metadata. |
| GET | `/v1/files/{fileId}/content` | File content download. |
| DELETE | `/v1/files/{fileId}` | Delete file. |

Channel selection uses model key: body/multipart `model`, else `?model=`, else `X-Metapi-Files-Model`, else default `gpt-4o`. Residual platforms without a Files API return upstream errors.

---

## Downstream Pricing (`/v1/pricing`)

Cross-site effective model price catalog exposed behind downstream-key (ProxyAuth) auth — NOT admin auth. A downstream consumer (中转站) holding a managed key can query effective cross-site model pricing for its own planning. Reuses the `modelPriceCompare` handler so the data surface is identical to `GET /api/models/price-compare`; no separate catalog to drift.

Mounted under `/v1/*` so it inherits ProxyAuth + wildcard CORS.

### GET /v1/pricing

**Auth**: ProxyAuth (downstream API key), not admin Bearer token.

**Query params**: `model` (optional name/substring), `days` (1–365, default 30), `limit` (default 50, max 200), `topModels` (1–50, default 12, used only when `model` is empty).

**Response** (200):
```json
{
  "model": "",
  "days": 30,
  "limit": 50,
  "sampleUsage": { "promptTokens": 1000, "completionTokens": 500, "totalTokens": 1500 },
  "items": [
    {
      "siteId": 3,
      "siteName": "anthropic",
      "platform": "anthropic",
      "model": "claude-sonnet-4-20250514",
      "accountId": 5,
      "username": "svc-1",
      "inputPerMillion": 3.0,
      "outputPerMillion": 15.0,
      "source": "billing_details",
      "ratesSource": "billing_details",
      "estimatedCostSample": 0.0105,
      "observedSamples": 1200,
      "configuredUnitCost": 0.003,
      "missingPrice": false,
      "recommended": true
    }
  ],
  "meta": {
    "count": 12,
    "modelsConsidered": 12,
    "sources": ["billing_details", "observed", "configured", "fallback"],
    "notes": "Rates/costs from billing_details, observed proxy_logs, account unit_cost, or labeled fallback. missingPrice=true means no catalog/observed/configured signal."
  }
}
```

`source` is one of `billing_details` | `observed` | `configured` | `fallback`. Fallback is always labeled; `missingPrice=true` when no catalog/observed/configured signal exists. Rows are sorted cheapest-first and `recommended=true` marks the best channel per model. Alias: `GET /v1/models/price-compare` (same handler).

---
