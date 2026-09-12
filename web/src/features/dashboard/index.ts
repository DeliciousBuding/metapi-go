// metapi-go/features/dashboard — public barrel.
//
// Public API for the 4-section Dashboard workspace: the route file and the
// sidebar read the section manifest from here.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.

export type { DashboardSectionId } from './types'

// 4-section manifest + registry helpers (route registration surface)
export {
  DASHBOARD_DEFAULT_SECTION,
  DASHBOARD_SECTION_IDS,
  getDashboardSectionMeta,
} from './config/dashboard-config'

// Section dispatcher. The four sections are lazy-loaded at its call site, so
// they are not part of this barrel.
export { DashboardPage } from './components/dashboard-page'
