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

## Header policy (/v1 surface)

The data plane rebuilds the upstream request instead of tunnelling the client
request, so headers cross the proxy in both directions only by explicit policy.

**Client → upstream (allow-list, fill-only).** Applied in this precedence order,
lowest to highest:

1. Allow-listed client protocol headers — `anthropic-version`, `anthropic-beta`,
   `openai-beta`, `user-agent`, and the `x-stainless-*` SDK telemetry namespace.
   Multi-value headers (e.g. several `anthropic-beta` flags) keep every value.
2. Site `custom_headers` and the per-site anti-bot identity (`cf_clearance`,
   browser UA override). Because step 1 is fill-only, a site-level value always
   wins over the client's.
3. The selected account token as `Authorization: Bearer …`. A client can never
   override it.

Credential-bearing client headers are never forwarded: `authorization`,
`x-api-key`, `cookie`, `proxy-*`, hop-by-hop headers, and `x-forwarded-*`. The
downstream key stays a Metapi secret and never reaches the upstream.

Anthropic-native dispatch (`/v1/messages`, including the `/anthropic/v1/messages`
gateway shape) requires `anthropic-version`; when the client does not send one
the API default is used, so an OpenAI-surface request transformed into the
Anthropic shape does not fail upstream validation.

**Upstream → client (allow-list).** Buffered (non-SSE) responses relay only
content-semantic headers: `Content-Type`, `Content-Disposition`,
`Content-Encoding`, `Content-Language`, `Content-Range`, `Accept-Ranges`,
`Cache-Control`, `ETag`, `Last-Modified`, `Location`, `Retry-After`.

Everything else is dropped, which keeps the upstream's identity and state from
leaking through Metapi: vendor fingerprint headers (`X-New-Api-Version` and
similar), `Server` / `Via` / `X-Powered-By` / `Cf-Ray`, upstream `Set-Cookie`,
framing headers (net/http re-frames the buffered body), and upstream
`X-Request-Id` — Metapi's own request id stays authoritative so a client-side
report can be correlated with the proxy log. Upstream rate-limit headers are
dropped for the same reason: the only `X-Ratelimit-*` a client sees describes
its Metapi key/IP budget.

SSE responses are always re-framed by Metapi (`text/event-stream`,
`Cache-Control: no-cache`, `X-Accel-Buffering: no`) and carry no upstream
headers at all.

## Timeouts (/v1 surface)

| Phase | Control | Default |
|---|---|---|
| Request header read | `Server.ReadHeaderTimeout` | 10s |
| Whole request (admin surface) | `Server.ReadTimeout` / `WriteTimeout` | 30s / 60s |
| Upstream connect / TLS | transport dial + handshake | 30s / 10s |
| Upstream first byte (observed) | `PROXY_FIRST_BYTE_TIMEOUT_SEC` | 0 = off |
| Whole upstream request (buffered) | executor ceiling `max(90s, first-byte × 2)` | 90s |
| Response write (proxy surface) | write budget = executor ceiling + 2m | 210s |
| Stream chunk gap | `PROXY_STREAM_IDLE_TIMEOUT_SEC` | see `.env.example` |

The proxy surface re-arms its own write deadline (`router.ProxyWriteDeadline`)
from the same source of truth as the executor ceiling, so the write side can
never be shorter than the request side: a buffered response that takes 61–90s
upstream is delivered instead of being cut mid-write. Admin routes keep the
strict 60s `WriteTimeout`.

Streaming responses are not bounded by total elapsed time at all — the relay
clears the write deadline and enforces liveness per chunk with the idle guard,
so a long reasoning-model stream stays alive as long as chunks keep arriving.

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
