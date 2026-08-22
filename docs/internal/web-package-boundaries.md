# Web package boundaries — frontend layering rules

**Status**: enforced by review + spot greps (no CI lint yet)
**Date**: 2026-08-23

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

1. **`src/lib/` never imports from `features/`, `components/`, or `routes/`.**
   Lib is the bottom layer: everything may depend on it, it depends on nothing
   app-specific. Verify with `grep -rn "@/features\|@/components\|@/routes" web/src/lib/`
   (must return nothing).
2. **`src/components/` does not import from `features/` or `routes/`.** Shared
   UI must stay renderable without any feature context. The registered
   exceptions for the `layout/` shell are listed below.
3. **Features may import lib, shared components, stores, i18n, and other
   features' public barrels** — feature-to-feature coupling is tolerated but
   should stay shallow (prefer a shared lib helper when logic is reusable).
4. **Routes may import everything**; they are the composition root.
5. A **pure helper that two layers both need belongs in `src/lib/`**, not in a
   feature. `sanitizeAuthRedirect` was moved from `features/auth/lib/` to
   `src/lib/helpers/` (2026-08-23) exactly because the lib-level
   `http-client.ts` needed it — a lib → feature import is never allowed, even
   for a single pure function.

## Registered exceptions (components/layout → features)

The layout shell renders navigation *about* feature workspaces, so it needs
each workspace's nav metadata. Inverting this (features registering their own
sidebar views into a layout-owned registry) is the intended long-term fix but
touches too many call sites right now. Until then these four edges are the
**complete, closed** exception list — new ones require review sign-off:

| File | Imports | Why |
|:---|:---|:---|
| `src/components/layout/config/system-settings.config.ts:9` | `getSettingsSubareas` from `@/features/settings` | Builds the Settings drill-in sidebar groups from the feature's subarea registry |
| `src/components/layout/components/user-menu.tsx:24` | `ABOUT_INFO` from `@/features/about/api` | Shows curated project metadata (name/repo links) in the user menu About entry |
| `src/components/layout/lib/sidebar-view-registry.ts:4` | `OBSERVABILITY_VIEW` from `@/features/observability` | Registers the Observability drill-in view for sidebar resolution |
| `src/components/layout/lib/search-nav.ts:11` | `getSettingsSubareas` from `@/features/settings` | Feeds the ⌘K palette's local Settings entries from the same registry |

All four pull **declarative nav/config metadata** (plain objects and a pure
registry function), never feature behavior, stores, or API hooks. The test-only
mirror of the user-menu edge (`layout/__tests__/user-menu.test.tsx`) inherits
the same exception.

**Exit criterion**: when a layout-owned `SidebarView`/nav registry with
feature-side registration exists, all four edges flip to feature → layout and
this section is deleted.

## Precedent log

- **2026-08-23** — `sanitizeAuthRedirect` moved `features/auth/lib/ →
  src/lib/helpers/sanitize-auth-redirect.ts` to break the lib → features edge
  in `http-client.ts`; `search-params-resilience.test.ts` moved
  `src/lib/helpers/__tests__/ → src/__tests__/` for the same reason (it
  exercises feature schemas).
