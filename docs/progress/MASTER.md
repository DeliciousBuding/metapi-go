# Roadmap

**Last verified**: 2026-08-19

**Release**: [v0.16.1](https://github.com/DeliciousBuding/metapi-go/releases/tag/v0.16.1) · released on master; production promotion follows the release and soak gate

> This is the only execution plan. It contains open work, order, ownership, and acceptance criteria. Current facts → [`../STATE.md`](../STATE.md) · product positioning → [`../benchmark.md`](../benchmark.md) · timeline → [`../log.md`](../log.md).

## Delivery model

Metapi Go has **3 delivery mainlines**. CI, dual-dialect support, security, release automation, and documentation hygiene form one cross-cutting engineering baseline, not a fourth product line.

| Mainline              | Current state                                                                                                                                 | Remaining outcome                                                                                                                                      |
| :-------------------- | :-------------------------------------------------------------------------------------------------------------------------------------------- | :----------------------------------------------------------------------------------------------------------------------------------------------------- |
| **A. 路由与成本真值** | Core proxy, retry/cooldown/breaker, routing strategies, usage pricing, models.dev cold-start catalog, half-open recovery, and route price truth are implemented. | Collect operator-gated live multi-channel cascade evidence; keep multi-instance limits explicit. |
| **B. 上游兼容与实测** | 16 adapters; real new-api/one-api CI e2e plus Sub2API/CLIProxyAPI verification chains; live-platform defects are covered by CI and focused tests. | Run optional Codex and AnyRouter probes from [#558](https://github.com/DeliciousBuding/metapi-go/issues/558). No blocking compatibility issue is open. |
| **C. 分发与管理体验** | Client export, global search, daily snapshot, enriched alerts, tester history/templates, guided onboarding, truthful channel comparison, and URL-owned table state are shipped. | No implementation slice is open; future work must be promoted here with an owner and acceptance criteria before coding. |

## Execution rules

1. Implement the smallest end-to-end slice in the existing feature owner; do not add a wizard framework, pricing facade, batch service, or job queue.
2. Reuse existing contracts. A missing UI connection is not grounds for a duplicate backend endpoint or a second source of truth.
3. Validate at trust boundaries. Inside typed flows, prefer explicit invariants over repeated fallbacks and speculative compatibility branches.
4. Unsupported behavior stays explicit. Do not turn a 501 residual, unavailable price, failed probe, or missing credential into fake success.
5. Each slice lands with its focused regression test and updates `STATE.md` only after the behavior is verified. Historical detail belongs in `CHANGELOG.md` / `log.md`, not here.

## Completed delivery outcomes

- **Guided onboarding**: site → account → route handoff, one-shot deep links, typed IDs, credential verification, redacted-credential edit behavior, and partial batch failure truth are implemented and covered by focused tests.
- **Tester truth**: forced-channel synchronous probes, explicit unsupported streaming behavior, enabled-channel filtering, bounded comparison concurrency, shared abort handling, and stopped-result rendering are implemented and covered by focused tests.
- **Route truth**: enabled-weight allocation and account/model-specific price provenance use exact concrete-model joins and are covered by focused Go and frontend tests.
- **URL state stability**: list-page table state has one URL owner, stable callbacks, latest-URL merge semantics, and real Chromium acceptance gates documented in [`../design/state-stability.md`](../design/state-stability.md).
- **Onboarding polish + team visibility (v0.17)**: platform picker (16-adapter searchable Select with manual-entry fallback), client export breadth (claude-code/codex/openwebui profiles), and downstream-key 24h usage summary (requests/tokens/cost) are implemented and covered by focused Go + frontend tests.

These outcomes are closed. New work enters this plan only with a concrete owner, scope, and acceptance test; completed waves do not remain as open checklists.

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
