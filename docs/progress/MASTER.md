# Roadmap

**Last verified**: 2026-08-16

**Release**: [v0.13.0](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.13.0) · current master continues after the release

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Delivery model

MetAPI Go has **3 delivery mainlines**. CI, dual-dialect support, security, release automation, and documentation hygiene form one cross-cutting engineering baseline, not a fourth product line.

| Mainline              | Current state                                                                                                                                 | Remaining outcome                                                                                                                                      |
| :-------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A. 路由与成本真值** | Core proxy, retry/cooldown/breaker, routing strategies, usage pricing, models.dev cold-start catalog, and half-open recovery are implemented. | Collect operator-gated live multi-channel cascade evidence; keep multi-instance limits explicit.                                                       |
| **B. 上游兼容与实测** | 16 adapters; real new-api/one-api CI e2e plus Sub2API/CLIProxyAPI verification chains; six live-platform defects closed after v0.13.0.        | Run optional Codex and AnyRouter probes from [#558](https://github.com/DeliciousBuding/metapi-go/issues/558). No blocking compatibility issue is open. |
| **C. 分发与管理体验** | Client export, global search, daily snapshot, enriched alerts, and tester history/templates are shipped.                                      | Finish guided onboarding, tester channel/latency truth, and route allocation/price comparison.                                                         |

## Execution rules

1. Implement the smallest end-to-end slice in the existing feature owner; do not add a wizard framework, pricing facade, batch service, or job queue.
2. Reuse existing contracts. A missing UI connection is not grounds for a duplicate backend endpoint or a second source of truth.
3. Validate at trust boundaries. Inside typed flows, prefer explicit invariants over repeated fallbacks and speculative compatibility branches.
4. Unsupported behavior stays explicit. Do not turn a 501 residual, unavailable price, failed probe, or missing credential into fake success.
5. Each slice lands with its focused regression test and updates `STATE.md` only after the behavior is verified. Historical detail belongs in `CHANGELOG.md` / `log.md`, not here.

## Delivery sequence

### Wave 1 — P0 guided onboarding

**Outcome**: the existing Site → Account → Token Route chain carries context forward and verifies credentials without introducing a parallel onboarding state machine.

| Slice                       | Owner                                                                                                                             | Implementation                                                                                                                                                                                                                                      | Acceptance                                                                                                                                     |
| :-------------------------- | :-------------------------------------------------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------------------------------- |
| C1. Account deep link       | `web/src/routes/_authenticated/accounts.tsx`, `web/src/features/accounts/components/accounts-page.tsx`, `account-form-dialog.tsx` | Add validated `siteId` / `create` search state. Open the create dialog once when the referenced site exists; consume or neutralize the open flag so refetches do not reopen it.                                                                     | The site-created CTA reaches an open account form with the site selected. Invalid/stale IDs fall back safely and never create data.            |
| C2. Credential verification | `web/src/features/accounts/api.ts`, `components/account-form-dialog.tsx`, existing `POST /api/accounts/verify-token`              | Add one mutation and an inline verify action for session/API-key credentials using current unsaved form values. Password binding continues through the real login submit path. Reset verification state whenever site, mode, or credential changes. | Operators see verified/failed/pending state before save; verification never bypasses schema validation and never persists secrets separately.  |
| C3. Route draft handoff     | `web/src/features/token-routes/components/route-form-dialog.tsx`                                                                  | In create mode, seed one `channelDrafts` entry from a valid `chainContext.accountId`; preserve normal defaults for direct entry and all edit behavior.                                                                                              | Account-created CTA reaches a route form with the account already selected; the operator still chooses model and routing settings before save. |

**Focused tests**

- Extend `web/src/features/accounts/__tests__/accounts-search-schema.test.ts` for valid, missing, and malformed deep-link values.
- Add behavior coverage for one-time dialog opening, site preselection, verification-state invalidation, and password-mode exclusion.
- Extend route form/schema coverage for account draft seeding without changing edit defaults.

**Non-goals**: a new `/onboarding` route, persisted wizard progress, automatic credential saving, automatic route creation, or backend workflow orchestration.

### Wave 2 — P1 tester truth foundation

**Depends on**: no Wave 1 dependency; land before batch comparison because the current tester does not identify a forced channel.

| Slice                         | Owner                                                                                                          | Implementation                                                                                                                                                                                                          | Acceptance                                                                                                                                         |
| :---------------------------- | :------------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------------- |
| C4. Channel response contract | `web/src/features/channels/api.ts` and channel types                                                           | Parse the actual `{items,total,page,pageSize}` response instead of casting the envelope to `ChannelRow[]`. Keep the list hook's public value as `ChannelRow[]` if existing consumers need it.                           | Channel pages and tester selectors receive real rows; malformed responses fail explicitly rather than masquerading as empty arrays.                |
| C5. Forced-channel probe      | `web/src/features/model-tester/types.ts`, `lib/tester-schema.ts`, `api.ts`, `components/model-tester-page.tsx` | Add channel selection and send `channelId` through the existing synchronous `/api/test/chat` forced-channel harness. Use the chosen channel as the routing identity; do not infer it from a label.                      | A single run reaches the selected channel and reports measured wall-clock latency, response, and upstream error honestly. Stop aborts the request. |
| C6. Streaming honesty         | same tester owner + `handler/admin/test.go` contract                                                           | Make the admin tester's supported path visibly sync-only while `/api/test/chat/stream` remains 501. Keep parser code only if it has a real supported caller; otherwise remove the dead branch in the implementation PR. | The UI cannot advertise a stream success path that the Go backend does not implement; no synthetic chunks or delayed sync-response theater.        |

**Focused tests**

- Add channel-envelope parsing coverage.
- Extend `web/src/features/model-tester/__tests__/api.test.ts` and schema tests for `channelId`, sync payload construction, abort, and explicit server failure.
- Add or retain the focused Go handler test proving missing channel identity returns the documented residual.

**Non-goals**: implementing SSE, adding automatic routing to the admin harness, or creating another channel-discovery endpoint.

### Wave 3 — P1 batch latency comparison

**Depends on**: Wave 2 forced-channel sync probe.

**Outcome**: run one model/prompt against selected channels and compare observed latency without hiding partial failure.

| Slice                 | Owner                                                                                    | Implementation                                                                                                                                            | Acceptance                                                                                                                      |
| :-------------------- | :--------------------------------------------------------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------ |
| C7. Batch selection   | `web/src/features/model-tester/components/model-tester-page.tsx` and tester schema/types | Support explicit multi-channel selection while retaining the single-run path. Require at least two eligible channels for comparison.                      | The submitted batch is visible and deterministic; disabled/unselected channels are not probed.                                  |
| C8. Bounded execution | `web/src/features/model-tester/api.ts` or one tester-local hook                          | Reuse the single sync probe with small fixed concurrency and one shared abort lifecycle. Settle every selected request independently.                     | One slow/failing channel does not erase successful rows; Stop cancels outstanding work; no unbounded fan-out.                   |
| C9. Result contract   | tester components + i18n locales                                                         | Render channel/site identity, success/failure, latency, and concise error per row; sort completed successes by latency and show `N succeeded / M failed`. | Mixed outcomes remain visible, empty/error states are accessible, and rerun replaces or clearly separates the prior comparison. |

**Focused tests**: success ordering, mixed success/failure, bounded scheduling, abort, empty selection, and rerun state. Prefer testing the orchestration helper plus one user-facing component interaction; do not mock the module under test.

**Non-goals**: backend batch endpoints, cross-model Cartesian matrices, persistent benchmark history, percentile analytics, charts, or background jobs.

### Wave 4 — P1 route allocation and price truth

**Depends on**: independent of Waves 1–3; may run in parallel after the P0 onboarding slice.

| Slice                   | Owner                                                                                                              | Implementation                                                                                                                                                                | Acceptance                                                                                                                                                        |
| :---------------------- | :----------------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| C10. Price join         | `web/src/features/token-routes/components/route-detail-sheet.tsx`, existing `features/models/price-compare/api.ts` | Resolve the concrete model used by each route channel, query the existing `/api/models/price-compare` data, and join by `accountId`. Query each distinct concrete model once. | Each comparable channel uses the same normalized pricing source as the Price Compare page. Pattern-only or missing-price rows show `—`, not zero or guessed cost. |
| C11. Allocation display | route detail row/helper                                                                                            | Show configured weight and enabled-weight share beside normalized input/output price and provenance. Disabled channels are excluded from the share denominator.               | Enabled shares are deterministic and total 100% within normal display rounding; zero-total weights show unavailable rather than dividing by zero.                 |

**Focused tests**: enabled/disabled share calculation, zero total, account/model join, missing price, and mixed source models. Put pure calculation coverage in the feature's `__tests__/`; avoid snapshots of Tailwind class strings.

**Non-goals**: changing routing weights automatically, declaring cheapest as healthiest, duplicating pricing formulas, or adding a route-specific pricing endpoint.

## Evidence closeout

### A. Live cascade proof — operator-gated, no coding by default

- Use [`../analysis/p0585-production-verification.md`](../analysis/p0585-production-verification.md), `scripts/verify-cascade-prod.sh`, and optionally the `staging` Go test against an operator-controlled multi-channel topology.
- Trigger one controlled retryable failure outside normal production traffic. Evidence must show one stable request ID, distinct channel IDs, increasing retry counts bounded by `PROXY_MAX_CHANNEL_ATTEMPTS`, and either sibling recovery or an explicit bounded all-fail result.
- Keep raw reports, host details, and credentials in the private operator surface. Record only the sanitized date/verdict/evidence pointer in public state.
- If the evidence reveals a defect, create a narrowly scoped fix. Do not pre-build another cascade layer before that evidence exists.

### B. Optional Codex / AnyRouter probes — #558

- AnyRouter: reuse `scripts/e2e/verify-token-import.sh` and record adapter detection, token verification, model listing, and one minimal request when credentials permit.
- Codex: use the existing OAuth/import flow and record one minimal supported request; do not introduce CI secrets or a second OAuth implementation.
- Done means [#558](https://github.com/DeliciousBuding/metapi-go/issues/558) contains reproducible sanitized evidence, or names the exact credential/environment limitation and the command that remains to be run. “Present” without a live call is not completion.

## Engineering baseline and release gate

Every wave inherits root [`AGENTS.md`](../../AGENTS.md) and backend simplicity rules in [`../design/BACKEND.md`](../design/BACKEND.md).

- Frontend slices: i18n parity, focused Vitest coverage, `bun run typecheck`, `bun run lint`, `bun run knip`, and production build.
- Any Go slice: SQLite + PostgreSQL-safe implementation, focused tests, then `go build ./cmd/server`, `go vet ./...`, and `go test ./... -count=1 -race` before push.
- No new dependency unless the existing stack cannot express the slice directly.
- Update `STATE.md` and this plan in the same PR that closes an outcome; do not leave completed checklists here.

## Deferred or out of scope

- Demand-gated: encrypted WebDAV sync, mobile PWA, Realtime transport.
- Explicitly out of scope: multi-tenant billing/wallet/subscription/payment and shared multi-instance sticky sessions.
- Update-center deploy/rollback remains external through GHCR and GitHub Releases; the admin API stays an honest 501 residual.
- Historical UI audit observations remain evidence in [`../analysis/ui-ux-audit-2026-08.md`](../analysis/ui-ux-audit-2026-08.md). They become commitments only when promoted here with an owner and acceptance criteria.
