// metapi-go/features/settings — barrel.
//
// Public API for the 5-subarea drill-in Settings workspace (plan §5.5.2).
// Consumers (route files, the main sidebar, future feature modules) import
// from `@/features/settings`.

// Types

// Generic factory + adapter types

// Generic dispatcher + in-page sidebar + overview landing + stub content
export { SettingsOverview } from './components/settings-overview'
export { SettingsPage } from './components/settings-page'

// 5-subarea manifest + validation helpers (route registration surface)
export {
  getSettingsSubarea,
  getSettingsSubareas,
  resolveDefaultSection,
  isValidSection,
} from './config/settings-config'

// Per-subarea typed registries (for route files that want compile-time ids)
