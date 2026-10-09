# Downstream Keys

> **Index**: back to [API Reference](../api.md). This file is the Downstream API keys, scope contract & export domain split out of the pre-`docs/api/` `docs/api.md`.

## Downstream API Keys

### GET /api/downstream-keys

List all downstream API keys.

### GET /api/downstream-keys/summary

Key list with usage summaries. **Query params**: `group`, `tags`, `tagMatch`.

### GET /api/downstream-keys/:id/overview

Usage overview for a specific key (24h, 7d, all-time).

### GET /api/downstream-keys/:id/trend

Usage trend data for a specific key.

### POST /api/downstream-keys

Create a new downstream API key. Both `name` and `key` are required. The server
does not generate the key for this endpoint; supply a cryptographically random
secret beginning with `sk-` (the admin UI generates one before submission).

**Body**:
```json
{
  "name": "My Key",
  "key": "${DOWNSTREAM_API_KEY}",
  "groupName": "production",
  "tags": "tag1,tag2",
  "supportedModels": ["gpt-4o", "claude-sonnet-4-20250514"],
  "allowedRouteIds": [1, 2],
  "excludedSiteIds": [3],
  "excludedCredentialRefs": [
    { "kind": "account_token", "siteId": 1, "accountId": 2, "tokenId": 7 }
  ],
  "allowedSiteIds": [1],
  "allowedCredentialRefs": [
    { "kind": "account_token", "siteId": 1, "accountId": 2, "tokenId": 7 },
    { "kind": "default_api_key", "siteId": 1, "accountId": 2 }
  ],
  "maxCost": 100.0,
  "maxRequests": 10000,
  "expiresAt": "2026-12-31T23:59:59Z"
}
```

Routing-policy fields (`excludedSiteIds`, `excludedCredentialRefs`, `allowedSiteIds`, `allowedCredentialRefs`, `siteWeightMultipliers`, `keyWeight`) are accepted on both create and update; their full contract is documented under **Credential & site scope** below.

### Usage limits are not a prepaid billing ledger

- `maxRequests` is an atomic admission counter. RPM/TPM-denied requests do not
  consume it, but an admitted request can still fail later; it is not a count of
  successful model completions.
- `maxCost` rejects subsequent requests once stored `usedCost` reaches the
  threshold. Successful proxy requests add their **estimated** cost after
  completion. No money is reserved for in-flight requests, so overlapping
  requests or a single completion can exceed the threshold. It is not a strict
  spending guarantee.
- Failed attempts may retain reported usage and an estimate in proxy logs, but
  currently do not increase the key's `usedCost`.
- Proxy cost estimates use the recorded pricing-source/fallback calculation.
  They are not the upstream wallet debit or an authoritative customer invoice;
  do not require monetary amounts at different relay layers to match.
- For `maxCost` and `maxRequests`, omitted, `null`, zero, or an empty string
  clears the corresponding limit.

### Exact access boundaries and period quotas

Create/update also accept an optional `accessPolicy` object. GET returns its
stored JSON string; omitting it on update preserves it, while `null` clears it.
The existing model/route grants still apply, and this policy narrows them:

```json
{
  "supportedModels": ["*"],
  "accessPolicy": {
    "allowedUpstreamChannelIds": [4, 7],
    "modelIds": ["gpt-4o"],
    "modelMappings": [{"from": "client-.*", "to": "gpt-4o"}],
    "quota": {
      "requests": 1000,
      "totalTokens": 1000000,
      "cost": 10,
      "period": {"type": "calendar_duration", "calendarDuration": {"unit": "day"}},
      "timezone": "Asia/Hong_Kong"
    }
  }
}
```

`allowedUpstreamChannelIds` uses IDs from the direct upstream channel inventory.
An omitted list is unrestricted; a present empty list denies every channel,
including native site/account channels. `modelIds` matches the mapped model
exactly. Ordered mappings use anchored Go regular expressions (`*` matches all)
and the first match wins. Invalid policy JSON or unsupported patterns fail closed.

Quota limits are inclusive exhaustion thresholds; zero immediately exhausts a
limit. Supported periods are `all_time`, `past_duration` with positive `value`
and `unit` (`minute`, `hour`, `day`), and `calendar_duration` with `unit` (`day`,
`month`). Calendar boundaries use the policy's IANA timezone (default UTC).
Quota usage counts completed successes and failed attempts with observed token
usage, independently of proxy-log retention. Cost uses the same estimated cost
as the proxy logs. These are completion-based limits: concurrent requests may
overrun a threshold. Unknown token/cost usage blocks the corresponding quota
until its period expires or accounting is reconciled.

AxonHub imports intersect the project and API key active profiles, including
channel ID and `any`/`all`/`none` tag filters, into a snapshot of channel IDs.
Reimport refreshes that boundary. Keys without `write_requests`, `noauth` keys,
inactive projects, and unsupported active routing overrides remain blocked.
Source usage logs are associated by their key and imported idempotently; local
usage is retained on reimport. If the backup lacks usage history,
`quota.historyMissingBefore` blocks periods overlapping that gap. Rolling and
calendar quotas recover when the gap leaves the period; all-time quotas require
reconciliation. Do not remove the marker merely to enable the key.

The native backup carries both the policy and `downstream_quota_usage` ledger.
The legacy reset-usage action resets only `usedCost`/`usedRequests`, not this
ledger. The key editor exposes structured channel selection, model mappings and
quota controls for both imported and native keys. Import reconciliation markers
remain read-only and are preserved by ordinary edits.

### Credential & site scope (downstream keys)

Optional per-key routing dimensions. All four fields are independent; each is
evaluated during channel selection for every proxied request.

| Field | Stored column | Type |
| --- | --- | --- |
| `allowedSiteIds` | `allowed_site_ids` | JSON array of site IDs |
| `excludedSiteIds` | `excluded_site_ids` | JSON array of site IDs |
| `allowedCredentialRefs` | `allowed_credential_refs` | JSON array of credential refs |
| `excludedCredentialRefs` | `excluded_credential_refs` | JSON array of credential refs |

**Semantics.** Omitted, `null`, or empty (`[]`) means **unrestricted** for that
dimension (the column stores `NULL`). A non-empty list activates the gate:

- `allowedSiteIds` / `allowedCredentialRefs` (allow-lists): only candidates
  matching **at least one** entry are eligible; everything else is rejected.
- `excludedSiteIds` / `excludedCredentialRefs` (exclude lists): candidates
  matching any entry are rejected.
- When the same target appears in both lists, **exclude wins** (deny).
- Site and credential dimensions are independent gates — a candidate must pass
  both.

> **UI status:** the credential-ref dimensions now have a site → account →
> token tree picker in the admin API-key sheet (issue #1026 follow-up,
> #1064 contract). The sheet serializes real arrays on create/update and
> parses the stored JSON strings when editing; the key list renders resolved
> site/account/token names, with unresolved IDs shown explicitly. The site
> picker (#1050) remains unchanged.

**Credential ref shape.** Each ref is one of two kinds:

```json
{ "kind": "account_token",   "siteId": 1, "accountId": 2, "tokenId": 7 }
{ "kind": "default_api_key", "siteId": 1, "accountId": 2 }
```

- `account_token` — a specific token of a specific account
  (`siteId` + `accountId` + `tokenId`, all required and > 0). Matches only
  channels bound to that exact token.
- `default_api_key` — the account's own default API key (`siteId` +
  `accountId`; no `tokenId`). Matches only channels that use the account's
  `apiToken` (no explicit token binding).
- The two kinds never match each other's channel class.
- Refs persisted by the legacy TS version without a `kind` are treated with
  `default_api_key` semantics at selection time (read-only compatibility; new
  writes must carry an explicit kind).

**Validation (create and update).** Requests with invalid refs are rejected
with `400` and nothing is persisted:

- malformed entries (non-object, unknown/missing `kind`, non-positive
  `siteId`/`accountId`, `account_token` without positive `tokenId`) — rejected
  rather than silently dropped, so an allow-list can never be quietly widened;
- `account_token` refs must reference an existing token whose
  `accountId`/`siteId` match the token's actual account/site;
- `default_api_key` refs must reference an existing account on the given site
  that has a non-empty default API key;
- `allowedSiteIds`/`excludedSiteIds`/`siteWeightMultipliers` site IDs must
  exist; `allowedRouteIds` route IDs must exist.

**Selector behavior.** During channel selection (`routing.ChannelSelector`):

- non-empty `allowedCredentialRefs` → candidates not matching any ref are
  rejected (decision reason: `API Key/令牌不在下游密钥允许列表中`);
- matching `excludedCredentialRefs` → rejected
  (`API Key/令牌已被下游密钥排除`);
- equivalent site-dimension reasons: `站点不在下游密钥允许列表中` /
  `站点已被下游密钥排除`.

**Dangling refs.** Refs are validated only at write time. Deleting an account
or token afterwards does **not** cascade-clean stored refs; a dangling ref
simply never matches a candidate — a dangling allow ref makes that credential
slot permanently ineligible (fail-closed), a dangling exclude ref is a no-op.

**Read responses.** `GET /api/downstream-keys`, `/summary`, and
`/:id/overview` return the stored columns verbatim: each of the four fields is
either `null` or a **JSON string** containing the array above (clients must
`JSON.parse` the value). Create/update request bodies use real arrays.

### PUT /api/downstream-keys/:id

Update a downstream API key. Partial-update semantics: omitted fields keep
their current value; a field present in the body replaces the stored value
(`null`/empty clears it back to the unrestricted default). The same
credential/site-scope validation rules as create apply; a rejected update
leaves the stored policy untouched.

### DELETE /api/downstream-keys/:id

Delete a downstream API key.

### POST /api/downstream-keys/:id/reset-usage

Reset usage counters (used_cost, used_requests) to zero.

### POST /api/downstream-keys/batch

Batch enable/disable/delete/reset-usage/updateMetadata on downstream keys. Body: `{ "ids": [1, 2], "action": "enable|disable|delete|resetUsage|updateMetadata", "groupName": "prod", "groupOperation": "set|clear" }` — `groupOperation`/`groupName` only apply to `updateMetadata`.

**Response**: `{ success, successIds, failedItems }`.

---

### GET /api/downstream-keys/:id/export

Export a downstream key's full secret as one-click credentials profiles. **Query params**: `profile` (`all` default, or one profile id such as `openai` | `cherry` | `claude` | `codex` | `generic`).

**Response** (200): `{ "success": true, "formatVersion": "1.0.0", "keyId": 1, "keyName": "prod-key", "baseUrl": "http://localhost:4000", "profiles": [ { "id": "openai", "label": "...", "description": "...", "contentType": "text/plain", "content": "..." } ], "notes": ["..."] }`

The full secret is intentionally returned by this endpoint only (see `handler/admin/credential_export.go`); 400 for unknown profile ids.

---
