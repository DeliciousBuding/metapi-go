# Web package boundaries — frontend layering rules

**Status**: machine-enforced since 2026-08-29 by `web/scripts/check-boundaries.mjs`
**Date**: 2026-08-23 (updated 2026-09-12)
**Gate**: `bun run lint` chains `oxlint && bun run check:boundaries`; pre-push
and the CI `frontend` job both run `bun run lint`, so violations are caught
locally and in CI.

The admin SPA under `web/src/` is layered. Imports are allowed to point
**downward** through these layers; upward or sideways-into-core imports are
defects unless registered as an exception below.

## Layers (top → bottom)

| Layer | Path | Role |
|:---|:---|:---|
| Routes | `src/routes/` | TanStack Router route definitions; wire features to URLs |
| Features | `src/features/<name>/` | Business slices (auth, settings, checkin, observability, …) |
| Shared components | `src/components/` | `ui/` primitives, `layout/` shell, `data-table/` |
| App state / i18n | `src/stores/`, `src/i18n/`, `src/hooks/` | Cross-feature state and services |
| Lib | `src/lib/` | Framework-agnostic infrastructure (http client, formatters, helpers) |

## Rules

1. **`src/lib/` never imports from `features/` or `routes/`.** Lib is the
   bottom layer: everything may depend on it, it depends on nothing
   app-specific. The boundary gate enforces these two edges; the remaining
   `lib → components` edges (`lib/router.ts` fallback pages) and `lib → i18n`
   edges are a pre-existing residual tracked separately.
2. **`src/components/` does not import from `features/` or `routes/`.** Shared
   UI must stay renderable without any feature context. The boundary gate
   enforces this; any exception requires an explicit in-script registry entry.
3. **Features may import lib, shared components, stores, i18n, and other
   features' public barrels** — and *only* their barrels: `@/features/<name>`,
   never a path below it. Feature-to-feature coupling is tolerated but must
   stay shallow, and a deep path is never shallow: it freezes a directory
   layout the owning feature must then keep. When a symbol is missing from the
   barrel, export it there; when two features need the same contract, it
   belongs in the feature that owns the domain, or in `src/lib/` when neither
   does. The boundary gate enforces this for every feature file. Route files
   are exempt — they are the composition root (rule 4) and load page
   components directly on purpose, so no barrel re-exports a page.
4. **Routes may import everything**; they are the composition root.
5. A **pure helper that two layers both need belongs in `src/lib/`**, not in a
   feature. `sanitizeAuthRedirect` was moved from `features/auth/lib/` to
   `src/lib/helpers/` (2026-08-23); a later change extended the same rule to
   `ABOUT_INFO`, token-route summary types, and the model-pattern predicates.
6. **A subsystem that publishes a barrel is imported through the barrel.**
   Outside `src/components/data-table/`, only `@/components/data-table` may be
   imported — never `core/`, `layout/`, `toolbar/` or `hooks/`. This is what
   makes the subsystem's internals refactorable: `index.ts` states the deal
   ("anything exported here is frozen against feature call sites; anything not
   exported here is free to be reorganised"), and the promise is only worth
   making while nothing outside reaches in. When a consumer needs a symbol the
   barrel does not export, the fix is to export it, not to deep-import it. The
   boundary gate enforces this for every layer, and asserts it is not passing
   vacuously (the barrel must exist and have at least one external consumer).

## Gate mechanics

`web/scripts/check-boundaries.mjs` statically scans all `.ts`/`.tsx` under
`web/src/`, resolves `@/` and relative specifiers to their layer, and fails
with file:line when a component or lib file imports `features/` or `routes/`,
when any file deep-imports a barrel subsystem (rule 6), or when one feature
deep-imports another (rule 3). Static, dynamic and side-effect specifiers are
all resolved, so a `lazy(() => import(...))` boundary is checked like any other
edge. Both barrel rules also fail when they would pass vacuously — a barrel
that no longer exists, or one nothing imports, means the check stopped
covering anything rather than that the tree got clean. Exceptions are an
explicit in-script registry with a required reason; a stale exception (no
matching import) also fails, so whitelists cannot accumulate silently. Both
rule families also fail when they would pass vacuously — a barrel that no
longer exists, or one nothing imports, means the check stopped covering
anything rather than that the tree got clean. Run directly with
`bun run check:boundaries`.

## Registered exceptions

**Layer rules (`EXCEPTIONS`)**: none. The shell inversion is complete: `layout/lib/settings-nav-registry.ts`
is the sole settings-nav provider and is registered from the authenticated route
composition root; `search-nav.ts` and `system-settings.config.ts` consume the
layout registry instead of importing `features/settings`. New cross-layer edges
require an explicit reviewed exception in `web/scripts/check-boundaries.mjs`; a
stale entry is rejected by the gate.

**Feature barrel rule (`FEATURE_EXCEPTIONS`)**: one entry.
`features/downstream-keys/downstream-keys-page.tsx` lazy-loads
`features/settings/sections/downstream/components/keys-section`, because the
keys UI was promoted to a first-class route without ever moving out of the
settings section directory it was written in. Tracked by
[#1335](https://github.com/DeliciousBuding/metapi-go/issues/1335): promoting
`SettingsSectionCard` / `SettingsSectionError` to `components/common/` unblocks
the move, after which this entry must be deleted — the gate rejects it as stale
the moment the import goes away.

## Precedent log

- **2026-08-23** — `sanitizeAuthRedirect` moved `features/auth/lib/ →
  src/lib/helpers/sanitize-auth-redirect.ts` to break the lib → features edge
  in `http-client.ts`; `search-params-resilience.test.ts` moved
  `src/lib/helpers/__tests__/ → src/__tests__/` for the same reason (it
  exercises feature schemas).
- **2026-08-29** — shell boundary inversion:
  - `ABOUT_INFO` moved `features/about/api.ts → src/lib/about-info.ts` to break
    `components/layout/user-menu.tsx → features/about/api`.
  - Settings nav metadata now flows feature → layout: layout owns
    `lib/settings-nav-registry.ts`, the authenticated route composition root
    registers `features/settings`' `getSettingsSubareas`, and
    `system-settings.config.ts` / `search-nav.ts` consume the layout registry.
  - `OBSERVABILITY_VIEW` now registers through
    `sidebar-view-registry.registerSidebarView()` from the same composition
    root instead of `components → features/observability`.
  - `RouteSummaryRow` / `RouteMode` / `RouteRoutingStrategy` /
    `RouteDecision` moved to `src/lib/helpers/token-route-contract.ts`; pure
    model-pattern predicates moved to `src/lib/helpers/model-pattern.ts` and
    are re-exported from the feature for compatibility. This fixes the
    pre-existing `lib/helpers/zeroChannelRoutes → features/token-routes` edge
    that the new gate exposed.
  - Added `web/scripts/check-boundaries.mjs` and chained it into `bun run
    lint` (pre-push + CI frontend gate).
- **2026-09-12** — feature barrels became the enforced cross-feature surface.
  32 deep imports (`@/features/<name>/api`, `/types`, `/lib/…`,
  `/price-compare/…`) were rewritten to their feature's `index.ts`, which
  gained the symbols its consumers actually needed (`accountSchema`,
  `AccountsSnapshot`, `useClearRouteCooldown`, the rebuild handoff, the
  price-compare contract and its grade badge). Two pieces of shared code moved
  to the layer that owns them: the downstream-key wire contract out of
  `settings/sections/downstream/components/key-form-shared.ts` into
  `features/downstream-keys/` (three features read it), and
  `SettingsSectionSkeleton` out of `features/settings/components/` into
  `components/common/section-skeleton.tsx` as `SectionSkeleton` (a second
  feature renders it). Feature barrel headers were corrected at the same time:
  several claimed a page component was "the primary surface" while exporting
  none, pointed at route files as future work when those routes already ship,
  and carried empty section headings.
- **2026-09-12** — `features/proxy-logs` had deep-imported `DataTableRow` from
  `components/data-table/core/data-table-row`; the two symbols it needed were
  exported from the barrel instead. Rule 6 was then added to
  `web/scripts/check-boundaries.mjs` so the next one is caught by a gate rather
  than by review (#1332).
