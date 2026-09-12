// metapi-go/features/settings — public barrel.
//
// Public API for the 5-subarea drill-in Settings workspace: the route files
// and the sidebar read the subarea manifest from here, and the standalone
// downstream-keys page lazy-loads one section.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.

// Dispatcher + in-page sidebar + overview landing
export { SettingsOverview } from './components/settings-overview'
export { SettingsPage } from './components/settings-page'

// 5-subarea manifest + validation helpers (route registration surface)
export {
  getSettingsSubarea,
  getSettingsSubareas,
  resolveDefaultSection,
  isValidSection,
} from './config/settings-config'
