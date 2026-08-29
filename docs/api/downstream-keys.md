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

Create a new downstream API key.

**Body**:
```json
{
  "name": "My Key",
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
