# Downstream Keys

> **Index**: back to [API Reference](../api.md). This file is the Downstream API keys, scope contract & export domain split out of the pre-`docs/api/` `docs/api.md`.

## Downstream API Keys

单条 key 不存在返回 `404`，数据库读取失败返回 `500` / `resourceLoadFailed`；
策略引用校验同样区分数据库失败（`500`）与未知账号或 token（`400`）。批处理中
单项读取失败记入该项的 `failedItems`。创建、更新或重置用量后若读回失败，写入
已经生效，客户端应重新读取状态，不要重复创建。

### GET /api/downstream-keys

List all downstream API keys.

响应的 `migratedKeyId` 是旧根密钥迁入后的受管 key ID，未发生导入时为 `0`。
它只提供升级引导，不代表该 key 仍然存在或启用；状态以 `items` 为准。
所有下游鉴权都查询该表，数据库不可用返回 `503`，不会退回旧环境变量。

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

### 模型与路由授权

`supportedModels` 支持精确名称、`*` / `?` 通配符和 `re:` 正则表达式。
模型范围与 `allowedRouteIds` 为并集：命中模型范围，或命中指定路由，均可继续选路。
`accessPolicy`、站点和凭据限制仍需同时满足；受管 Key 的模型范围和路由列表都为空时拒绝请求。

`GET /v1/models` 对路由授权使用与转发相同的只读选路检查，只展示当前存在可选候选的模型，
不调用上游。请求入口与选路共用模型匹配规则。策略或暴露的模型目录变化会让缓存立即失效；
仅运行时候选状态变化时，列表最迟在 15 秒的进程内缓存 TTL 后更新。

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

The admin key list shows additional site/credential scope, direct channel counts,
exact-model restrictions, mappings, source blocks and period quotas. "No
additional routing restriction" refers to that scope column only; the model
grants, enabled state, expiry and usage limits still apply. An unreadable saved
policy stays visible and is preserved on ordinary edits.

The edit form shows dimension counts and links to their controls. Site,
credential and direct-channel pickers support search, selected-only view and
locating a selection. Filtering does not change the saved list. Unavailable
references remain visible for explicit removal. Empty legacy site/credential
allow-lists mean no additional restriction; a present empty direct-channel
allow-list means deny all.

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
as the proxy logs. A request admits atomically against the current window: it
reserves one request slot (and, when the policy has a token limit, a
best-effort input-token estimate) in the same transaction as the check, so
concurrent requests cannot overrun the requests limit. The hold is settled by
the completed attempt or released when the request ends without billable usage;
a hold left by a crashed process expires after 30 minutes. Output tokens and
the selected channel's price are unknown before dispatch, so concurrent
in-flight output can still exceed a token limit and concurrent in-flight cost
can exceed a cost limit. Unknown token/cost usage blocks the corresponding
quota until its period expires or accounting is reconciled.

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

OAuth 路由池按实际成员的站点和凭据应用这些限制，再执行轮询或保持当前成员策略。
仅授权直连渠道的 Key 不能使用账号路由池。没有授权成员时，诊断预览显示池不可用，
代理请求不向上游派发。`default_api_key` 只匹配实际使用该 API Key 的成员，
不会授权同一账号下不同的 OAuth access token。

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
