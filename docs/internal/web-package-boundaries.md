# Web package boundaries — frontend layering rules

**Status**: machine-enforced since 2026-08-29 by `web/scripts/check-boundaries.mjs`
**Date**: 2026-08-23 (updated 2026-08-29)
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
   features' public barrels** — feature-to-feature coupling is tolerated but
   should stay shallow (prefer a shared lib helper when logic is reusable).
4. **Routes may import everything**; they are the composition root.
5. A **pure helper that two layers both need belongs in `src/lib/`**, not in a
   feature. `sanitizeAuthRedirect` was moved from `features/auth/lib/` to
   `src/lib/helpers/` (2026-08-23); Wave 21 S5 extended the same rule to
   `ABOUT_INFO`, token-route summary types, and the model-pattern predicates.

## Gate mechanics

`web/scripts/check-boundaries.mjs` statically scans all `.ts`/`.tsx` under
`web/src/`, resolves `@/` and relative specifiers to their layer, and fails
with file:line when a component or lib file imports `features/` or `routes/`.
Exceptions are an explicit in-script registry with a required reason; a stale
exception (no matching import) also fails, so whitelists cannot accumulate
silently. Run directly with `bun run check:boundaries`.

## Registered exceptions

None. Wave 21 S5 shell inversion is complete: `layout/lib/settings-nav-registry.ts`
is the sole settings-nav provider and is registered from the authenticated route
composition root; `search-nav.ts` and `system-settings.config.ts` consume the
layout registry instead of importing `features/settings`. New cross-layer edges
require an explicit reviewed exception in `web/scripts/check-boundaries.mjs`; a
stale entry is rejected by the gate.

## Precedent log

- **2026-08-23** — `sanitizeAuthRedirect` moved `features/auth/lib/ →
  src/lib/helpers/sanitize-auth-redirect.ts` to break the lib → features edge
  in `http-client.ts`; `search-params-resilience.test.ts` moved
  `src/lib/helpers/__tests__/ → src/__tests__/` for the same reason (it
  exercises feature schemas).
- **2026-08-29 (Wave 21 S5)** — shell boundary inversion:
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