# Credential mental model + IA + PROXY_TOKEN removal — audited design

**Date**: 2026-08-24
**Status**: design proposal, audited (not implemented)
**Scope**: credential taxonomy · navigation/IA · PROXY_TOKEN retirement path · upgrade reliability
**Evidence**: metapi-go source + three reference projects (QuantumNous/new-api fork, qixing-jk/all-api-hub, Wei-Shaw/sub2api), read-only

## 1. Decision

`PROXY_TOKEN` (全局下游主密钥) must be retired. It is a non-standard,
policy-free global key. None of the three reference projects
has an equivalent. Retirement is a **breaking** change and must go through a
materialization + deprecation window, not a hard delete.

## 2. Cross-project evidence

### 2.1 Consensus (all three references agree)

- **No global master key.** newapi, sub2api, all-api-hub all authenticate
  downstream traffic with **per-user / per-account API keys**, never a global
  policy-free fallback.
- **Management auth is a separate channel.** newapi `User.AccessToken` (admin
  `/api`, one per user); sub2api `Admin API Key + x-api-key`; all-api-hub
  per-managed-site admin key. None reuses the downstream gateway key.
- **Upstream credential ≠ downstream key.** Upstream account tokens/keys are
  masked, stored separately, and never handed out as the client credential.

### 2.2 newapi (QuantumNous/new-api fork)

Three credential objects, fully decoupled:

| Concept | Field | Guards | Format | Multi/Quota |
|:--------|:------|:-------|:-------|:------------|
| AccessToken 访问令牌 | `model/user.go:94` `access_token char(32)` | management `/api` | no `sk-`, 29–32 char, shown once | one per user, rotatable |
| Token / API key 令牌 | `model/token.go:17` `Key varchar(128)` | `/v1` LLM | `sk-` tolerant on input, stored bare, 48 char | many, each with quota/model-limit/IP/group/expiry |
| Channel key 渠道密钥 | `model/channel.go` `Key` | outbound upstream only | vendor-specific | many, weight/priority/auto-ban |

`/v1` auth is **pure per-user Token** (`middleware/auth.go:352-483` →
`model.ValidateUserToken`); there is no global fallback. "root" exists only as
a role (`RoleRootUser=100`), never a key. IA: **API Keys** is a top-level
sidebar item; **AccessToken** is buried in Profile → security card.

### 2.3 sub2api (Wei-Shaw/sub2api)

A forwarding gateway. Splits **upstream account credentials** (`accounts[]`,
`type: oauth | apikey`; oauth carries `access_token/refresh_token/id_token`)
from **platform-issued User API keys** (`sk-` prefix). Three separate auth
channels: JWT (Web UI), Admin API Key (automation), User API Key (AI gateway).
Credentials are masked by default (`credentials_status.has_api_key`); raw
values are reveal/export on demand.

### 2.4 all-api-hub (qixing-jk/all-api-hub)

Local-first asset manager (no traffic forwarding). Explicit four-layer model:
`Site → Account (access_token/cookie) → ApiToken.key / ApiCredentialProfile`.
Normalizes runtime keys via `Account Runtime Key` (source =
account_token | service_credential | account_key_resource). IA: high-frequency
items (Account / Keys / Models) top-level; low-frequency config folded into one
tabbed `BasicSettings` page.

## 3. Current MetAPI model (as built, corrected)

```
sites (upstream platforms)
  └─ accounts (logins: check-in / balance / key extraction)
       └─ account_tokens (upstream keys -> channels)
            └─ route_channels -> token_routes (model -> weighted channels)
                 └─ /v1/* proxied

downstream_api_keys  managed per-client keys (allowlist/quota/IP/expiry)
PROXY_TOKEN          global downstream master key (deprecated target)
AUTH_TOKEN           admin / management API
```

Four credential roles (`router/router.go:49,166,184`):

| Role | Identifier | Guards | Used by |
|:-----|:-----------|:-------|:--------|
| Admin | `AUTH_TOKEN` | `/api/*` `auth.AdminAuth` | operator |
| Upstream | `account_tokens` | outbound | aggregated sites |
| Downstream managed | `downstream_api_keys` (`store/schema_ddl.go:913`) | `/v1/*` `auth.ProxyAuth` | client |
| Downstream global | `PROXY_TOKEN` (`config/config.go:565`) | `/v1/*` fallback | client |

### 3.1 PROXY_TOKEN dual source (the corrected fact)

`PROXY_TOKEN` has **two** persistence sources, not one:

1. env `PROXY_TOKEN` (`config/config.go:565`, default `change-me-proxy-sk-token`).
2. DB `settings.proxy_token`, written by `PUT /api/settings/runtime`
   (`handler/admin/settings_apply.go:42-53`) and re-applied over env at startup
   (`store/settings.go:95-97` ← `cmd/server/main.go:145`).

Any migration must read the **hydrated effective value `cfg.ProxyToken`**, not
raw env. Companion env `PROXY_GLOBAL_TOKEN_RPM` (`config/config.go:729`) applies
a global RPM cap to `Source=="global"` traffic, so PROXY_TOKEN is not "no
policy"; it is "no per-key policy, global RPM cap only".

### 3.2 PROXY_TOKEN is a data-contract discriminator

- `auth/context.go:47,70` writes `owner_type="global_proxy_token"` /
  `owner_id="global"` into `proxy_logs` / `proxy_files` (historical rows depend
  on the literal value).
- `service/backup/ts_backup_v21.go:106` blacklists `proxy_token` on import.

## 4. Target mental model (terminology, audited)

| Current | Proposed | Function | Placement |
|:--------|:---------|:---------|:----------|
| `AUTH_TOKEN` | **访问令牌 (AccessToken)** | admin / management API (single-instance single value, not per-user) | Settings |
| `downstream_api_keys` | **API 密钥 (API key / 令牌)** | call LLM `/v1/*` | Sidebar (frequent) |
| `PROXY_TOKEN` | **根下游密钥 (Root downstream key, deprecated)** | `/v1/*` fallback, global RPM only | migrate into API 密钥 page |
| `account_tokens` | **上游凭证 / 渠道密钥 (Channel key)** | outbound upstream | channels page |

One-line model: **访问令牌 = dashboard/管理访问；API 密钥 = 调 LLM 的消费凭据；
上游凭证 = 你聚合进来的资源**。No global policy-free key.

## 5. IA rule + target map

Rule: **frequent/operational → sidebar; rarely-changed config → Settings.**

Keep in sidebar: dashboard, sites, accounts, checkin, proxy-logs, observability,
token-routes, channels, **API 密钥**, oauth, models, model-tester, price-compare.

Keep in Settings: site branding, notifications, announcements, import-export,
proxy-transport, routing, redirects, rates, allowlist, catalog-sources,
scheduling, database, data-migration, maintenance, program-logs, audit-logs,
update-center, danger-zone, **访问令牌** (authentication).

Changes:

1. `settings/basic/authentication` (`AUTH_TOKEN`) -> title **访问令牌 (AccessToken)**; stays in Settings.
2. `settings/downstream/proxy-token` -> migrate out; the whole `downstream`
   subarea then collapses (it only contains proxy-token today).
3. Sidebar `downstreamKeys` -> title **API 密钥**.
4. The migrated root key becomes a special entry in the API 密钥 page. It is
   **not exportable** today (`handler/admin/credential_export.go` only exports
   managed key ids); materialization (Step 4 below) removes this gap by making
   it a real `downstream_api_keys` row.

## 6. Parity red lines (frozen, do not touch)

Safe to change: **i18n labels, copy, docs** only.

Frozen (parity + data contract):

| Item | Why |
|:-----|:----|
| env names `AUTH_TOKEN` / `PROXY_TOKEN` / `PROXY_GLOBAL_TOKEN_RPM` | TS parity, compose/README/testbed depend |
| JSON fields `key`, `proxyToken`, `proxyTokenMasked`, `ownerType`, `ownerId` | camelCase contract |
| table `downstream_api_keys`, column `key`, settings keys `proxy_token` / `auth_token` | schema |
| routes `/api/downstream-keys*`, `/api/settings/runtime`, `/api/settings/auth/*`, `/v1` | API surface |
| `owner_type` literal `global_proxy_token` | historical rows reference it |

## 7. PROXY_TOKEN retirement path (audited)

### 7.1 Full usage surface (must-change, P3)

15 runtime sites: `config/defaults.go:8`, `config/config.go:66/255/258/565/729`,
`auth/downstream.go:160-164` (core fallback), `auth/proxy.go` (`KeyName="global"`),
`auth/context.go:47/70`, `auth/ratelimit.go:308/315`, `config/validate.go:233-238`,
`handler/admin/settings.go:143`, `handler/admin/settings_apply.go:42-53`,
`store/settings.go:95-97`, `cmd/server/main.go:145`,
`service/backup/ts_backup_v21.go:106`, `router/router.go` (rate-limit mount).

Also: `downstream` settings subarea (only proxy-token remains), i18n keys,
5 test files, and optional CI/scripts/compose/testbed/docs sweep.

### 7.2 Recommended path: materialize, do not hard-delete

Hard delete breaks every client using the existing master key with a silent 403
(`auth/downstream.go:168-172`). Instead, **one-time materialize** the hydrated
effective token into a `downstream_api_keys` row:

- If effective `cfg.ProxyToken` is non-empty, not default, and no row with the
  same `key` exists -> `INSERT` `enabled=1`, `name='root (migrated from
  PROXY_TOKEN)'`, no quota/IP/expiry policy.
- `key` is UNIQUE (`downstream_api_keys_key_unique`), so this is a safe single
  INSERT; rollback = delete the row + restore the binary.
- After materialization, `Source` flips `global -> managed`, `owner_type`
  flips `global_proxy_token -> managed_key` for new writes (old literal kept
  read-only), and the key becomes exportable/disableable/quotaable through the
  existing managed-key surface.

Four companion surfaces to handle: `proxy_token` DB/env dual-source dedupe;
`PROXY_GLOBAL_TOKEN_RPM` retirement (fold into the key's `max_rpm`); historical
`owner_type` rows; backup blacklist entry.

## 8. Upgrade reliability

### 8.1 Version gate

- P1/P2 (labels/copy/additive) -> `v0.16.10`.
- P3 (materialize + remove fallback) -> `v0.17.0` (breaking).

### 8.2 Compatibility contract

- Additive-only this round; no env/JSON/schema/route renames (§6).
- Deprecation window with in-UI warning + one-click migration CTA before any
  fallback removal.

### 8.3 Phased rollout

| Phase | Content | Risk | Gate |
|:------|:--------|:-----|:-----|
| P1 | terminology + IA relabel + deprecation copy | low | v0.16.10; e2e/visual on settings + API 密钥 pages |
| P2 | observable deprecation flag (default off) | low | flag-off parity with P1 |
| P3a | migration design + read-only preview | none | preview reports target row + conflicts |
| P3b | one-time materialize + remove fallback | **breaking** | v0.17.0; old key still 200 as managed |
| P3c | observe window, then disable/delete root key | low | no new global writes |

### 8.4 Rollback & observability

Digest rollback pin + compose backup per deploy. POSTCHECK hard criteria:
(a) old token auth still passes, (b) old URL redirects still 200,
(c) relabeled pages render, (d) `proxy_token` DB round-trip unchanged until P3,
(e) backup blacklist unchanged until P3, (f) `owner_type` attribution unchanged,
(g) rate-limit source discrimination unchanged.

## 9. Step-by-step roadmap

- **Step 0 — baseline freeze**: pin v0.16.9 digest; capture hydrated effective
  token value; compose backup.
- **Step 1 — P1 (v0.16.10)**: relabel `basic/authentication` -> 访问令牌;
  sidebar `downstreamKeys` -> API 密钥; proxy-token section -> 根下游密钥（已弃用）
  + warning. i18n/copy only.
- **Step 2 — P2**: deprecation flag + logging marker, default off.
- **Step 3 — P3a**: materialization design + `/api/.../preview` dry-run.
- **Step 4 — P3b (v0.17.0)**: execute materialization; remove env/fallback/
  validation/rate-limit/DB-hydration PROXY_TOKEN branches; sweep CI/scripts/docs.
- **Step 5 — P3c (v0.17.x)**: observation window, then disable/delete the
  migrated root key; write STATE/CHANGELOG.

## 10. Decisions needed

- Approve Step 1 (P1, v0.16.10) now?
- Accept "materialize into downstream_api_keys row" (zero-breakage) over
  "hard-delete" for P3?
- Accept terminology: 访问令牌 (AccessToken) / API 密钥 (API key) / 根下游密钥
  (Root downstream key, deprecated) / 上游凭证 (Channel key)?