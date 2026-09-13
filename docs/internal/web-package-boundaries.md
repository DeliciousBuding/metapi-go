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
edge — the one deep feature import this gate caught on its first run was a
`lazy()` call that three separate static `from '…'` scans had missed. Both
barrel rules also fail when they would pass vacuously — a barrel that no longer
exists, or one nothing imports, means the check stopped covering anything
rather than that the tree got clean. Exceptions are an explicit in-script
registry with a required reason; a stale exception (no matching import) also
fails, so whitelists cannot accumulate silently. Run directly with
`bun run check:boundaries`.

Rule numbers in the script's header and failure messages are this document's
(3 = feature barrels, 6 = subsystem barrels): `src/lib/*.ts` cites them by
number, so the script must not carry a second numbering of its own.

One piece of prose is checked too. A barrel header that documents
`import { X } from '@/features/<name>'` states what that barrel exports, and
nothing type-checks a comment — so the gate parses those examples out of the
header (comment lines are concatenated first, because an example may span
several of them) and requires every named symbol to be in the target barrel's
export set. It fails vacuously if it finds no example at all, which is the
"resolution stopped matching" case rather than a clean tree. The rest of a
barrel's header — who consumes it, why a page is absent — is review-only; this
is the one claim a machine can settle. Exported names are collected from
`export { … }` lists and the barrel's own declarations, with line comments
stripped from a brace body *before* it is split on commas (a comment inside the
braces can contain one).

## Registered exceptions

**Layer rules (`EXCEPTIONS`)**: none. The shell inversion is complete: `layout/lib/settings-nav-registry.ts`
is the sole settings-nav provider and is registered from the authenticated route
composition root; `search-nav.ts` and `system-settings.config.ts` consume the
layout registry instead of importing `features/settings`. New cross-layer edges
require an explicit reviewed exception in `web/scripts/check-boundaries.mjs`; a
stale entry is rejected by the gate.

**Feature barrel rule (`FEATURE_EXCEPTIONS`)**: none. The single entry that
opened with the rule (`downstream-keys-page.tsx` lazy-loading the keys section
out of `settings/sections/downstream/`) was retired by moving the keys UI into
`features/downstream-keys/` — see the precedent log.

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
  - Sidebar drill-in views live in `sidebar-view-registry.ts`; settings is the
    only remaining drill-in and is layout-owned, so the list is static
    (observability's was removed in #1353 because it duplicated the page's
    in-content tabs; the feature-facing register function went with it).
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
- **2026-09-12** — barrel headers were re-read against the tree (#1338) and
  four claims were false: `token-routes`' example import named `RoutesPage` and
  `RouteSummaryRow`, which that barrel does not export; its rebuild-handoff
  note credited a settings section with writing the reference when the writer is
  the feature's own `api.ts`; `settings` still said the standalone
  downstream-keys page lazy-loads one of its sections, which stopped being true
  an hour earlier when the keys UI moved home; `observability` listed Proxy Logs
  as one of its sections when it is a separate workspace the sidebar deep-links
  out to. Six doc comments in `models/api.ts`, `models/types.ts` and
  `settings/config/settings-config.ts` described symbols that no longer exist
  and were deleted. The example-import claim is the part that can rot silently
  and still compile nowhere, so it is now gated (see Gate mechanics).
- **2026-09-12** — the downstream-keys UI moved home (#1335). `keys-section`,
  `key-sheet-form`, `key-cells`, `key-scope-cell`, `key-form-shared`,
  `key-created-toast`, `credential-ref-picker` and `lib/credential-{refs,display}`
  went from `features/settings/sections/downstream/` to
  `features/downstream-keys/`, whose barrel already owned the wire contract;
  `sections/downstream/` now holds only the proxy-token section. Two pieces of
  generic card chrome moved down with them — `SettingsSectionCard` →
  `components/common/section-card.tsx` (`SectionCard`) and `SettingsSectionError`
  → `components/common/section-error.tsx` (`SectionError`) — the same promotion
  `SectionSkeleton` got when a second feature needed it. `SectionError` now takes
  a required `messageKey` instead of hardcoding `settings.common.loadFailed`,
  matching `QueryErrorBanner`: shared chrome must not own one caller's copy.
  This retired the last `FEATURE_EXCEPTIONS` entry.
- **2026-09-13** — the soft-tone badge recipe (pill classes, tone map, status
  dot) was single-sourced into `components/common/soft-tone-badge.tsx`
  (#1359): `HttpStatusBadge` (common) and `LatencyBadge` (features/proxy-logs)
  had drifted into character-for-character copies. Both keep their own tier
  resolution and export names; the recipe has one owner. The same change
  merged the duplicate `SectionSkeleton` pair (`ui/` bare + `common/` shelled)
  into the `common/` implementation behind a `shelled` prop.
- **2026-09-12** — `features/proxy-logs` had deep-imported `DataTableRow` from
  `components/data-table/core/data-table-row`; the two symbols it needed were
  exported from the barrel instead. Rule 6 was then added to
  `web/scripts/check-boundaries.mjs` so the next one is caught by a gate rather
  than by review (#1332).
