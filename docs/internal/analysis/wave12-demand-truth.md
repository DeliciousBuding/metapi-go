# Wave 12 demand truth and account-bootstrap plan

**Last verified**: 2026-08-25 14:05 +08:00

## Objective

Turn the seven open user reports (#996-#1002) into two coherent, testable delivery batches without adding duplicate owners or fake completion paths. The first batch repairs concrete UI and notification truth gaps; the second batch closes account bootstrap and upstream-model workflows on top of existing services.

## Verified baseline

- Source: `master` at `138c785` (`v0.16.11`); no open PR and no active delivery wave.
- #996 is reproducible: Sites pagination is URL-controlled but TanStack resets the page index to zero.
- #999 is already enforced by Go (`supportedModels`, managed-key auth, request filtering, `/v1/models` filtering); the admin UI cannot configure it.
- #1001 is a local-form UX gap: the account form already owns the complete Sites snapshot, but exposes a non-searchable Select.
- #1002 has a complete manual token-sync path, but account creation never invokes it and password-login reports a fixed `tokenCount: 1` without persistence proof.
- #998 already has upstream discovery, account model APIs, availability persistence, route rebuild and cache invalidation; the missing surface is a coherent account Models panel plus honest post-create persistence.
- #1000 already syncs and stores site announcements. Typical `info` announcements do not enter the attention bell, and the stored upstream URL is intentionally not trusted by the UI.
- #997 is not a broken delete endpoint: the bell is a live attention projection, not a message inbox. Hiding an item only in the browser would be false state.
- Visual-QA evidence from the interrupted session was invalid: ports 4110/4111 were occupied by old Wave 9 processes, so the alleged empty profile contained seeded data. Fresh current-master instances now run on 4120 (seeded) and 4121 (confirmed empty), and both 112-shot matrices were recaptured.

## Product decisions

These defaults are selected because the user asked the captain to take ownership and continue without stopping for broad clarification.

1. **Attention semantics (#997)**: keep attention items condition-derived. Rename/explain the surface as unresolved items that disappear when the condition is resolved; do not add a fake client-side clear action.
2. **Site-announcement notification (#1000)**: unread state remains `site_announcements.read_at`; `events.read` is audit-only. The bell links to the local announcement page. External navigation uses only an HTTP(S) URL resolved against the trusted local Site URL; unsafe or unknown source URLs fall back to the Site home page.
3. **Managed-key model policy (#999)**: preserve fail-closed behavior. Empty means no model authorization; explicit all-model access is stored as `['*']`. The UI must support exact, glob and regex patterns.
4. **Token bootstrap (#1002)**: only session accounts auto-sync. A sync failure is a partial initialization warning and never rolls back an already verified account. API-key connections are explicitly skipped.
5. **Model sync (#998)**: this wave delivers manual upstream refresh, manual add/remove and post-create persistence using the existing availability owner. A periodic scheduler is deferred until there is explicit demand for cadence/scope; it must not be conflated with model health probes.
6. **QA profile truth**: screenshot review must declare and verify `empty` or `seeded` before capture. A directory name is not evidence.

## Delivery batches

### Wave 12A — demand and notification truth (`v0.16.12`)

| Lane | Scope | Exclusive write area | Acceptance |
| --- | --- | --- | --- |
| A | #996 Sites pagination | `web/src/features/sites/**` | real rendered page 2 stays at `page=1` and shows rows 11-20 |
| B | #999 key model policy UI | downstream keys section/tests | create/edit round-trip exact/glob/regex; empty is deny-all; `*` is explicit all |
| C | #1001 searchable Site selector | account form dialog/tests | search name/URL/platform; keyboard selection; numeric `siteId`; deep-link preselect preserved |
| D | #1000 + #997 notification truth | stats attention, site announcements, attention bell/target tests | unread announcement appears once, read clears it, unsafe URLs never become links, attention semantics are explicit |
| E | screenshot profile guard + current model examples | `web/scripts/screenshot-scan.mjs` and focused tests; locale examples owned by integration | wrong profile fails before screenshots; current seeded/empty profiles pass |
| I | integration/docs/release | locale JSON, `MASTER.md`, `STATE.md`, `log.md`, `CHANGELOG.md`, package version | all lane tests + full local gate + current-master visual smoke |

### Wave 12B — account bootstrap truth (`v0.16.13`)

| Lane | Scope | Exclusive write area | Acceptance |
| --- | --- | --- | --- |
| F | #1002 service-owned post-create token sync | account-token service, sync handler, account creation tests | success/empty/failure/API-key-skip all truthful; no account rollback on sync failure; no fixed token count |
| G | #998 account Models panel and refresh owner | model refresh service/handler, account Models UI/tests | upstream refresh persists availability, manual rows can be removed explicitly, route/cache refresh occurs once |
| I | integration/docs/release | shared locale/docs/release files | focused Go/UI tests, full local gate, SQLite+PostgreSQL-safe review |

## Dependency and merge order

```text
12A: A | B | C | D | E  -> integration I -> v0.16.12
12B: F -> G -> integration I -> v0.16.13
```

- A/B/C/D/E use disjoint implementation files; only integration owns locale and release documents.
- F lands before G so the account bootstrap outcome and credential ownership are stable before model persistence is attached.
- No lane may create a second Sites search API, announcement read-state store, model catalog table, or browser-only success fallback.

## Required verification

### Focused

```powershell
cd web
bun run test -- <lane test files>
bun run typecheck

cd ..
go test ./handler/admin ./service ./auth ./handler/proxy ./routing ./scheduler -count=1
```

### Full release gate

```powershell
go build ./cmd/server
go vet ./...
go test ./... -count=1 -race
cd web
bun run test
bun run typecheck
bun run lint
bun run knip
bun run build
```

### Adversarial checks retained by integration owner

1. Run the screenshot scanner against the seeded service with `EXPECTED_DATA_PROFILE=empty`; it must fail before creating a screenshot.
2. Create a managed key with no models/routes and prove it remains fail-closed; then create one with `['*']` and prove explicit all-model access.
3. Feed `javascript:`, `data:`, `file:` and protocol-relative announcement URLs; none may render as an external link.
4. Force token sync failure after account persistence; the account must exist and the response must report partial initialization rather than full success or rollback.

## Deferred

- Periodic upstream model-discovery scheduler and its global/site/account scope.
- Persistent acknowledgment of computed attention conditions. If demanded later, it needs stable fingerprints and a real store; it is not a one-line clear button.
- Single-announcement upstream detail-page routing until each platform's trusted browser path is verified.
