// metapi-go/features/dashboard — barrel.
//
// Public API for the 4-section Dashboard workspace (plan §5.5.1). Consumers
// (route files, the main sidebar, future feature modules) import from
// `@/features/dashboard`.

// Types

// Generic factory + registry types

// 4-section manifest + registry helpers (route registration surface)
export {
  DASHBOARD_DEFAULT_SECTION,
  DASHBOARD_SECTION_IDS,
} from './config/dashboard-config'

// Section dispatcher
export { DashboardPage } from './components/dashboard-page'

// Shared components

// Hooks

// VChart spec builders

// The 4 sections (lazy imports land at the call site in phase 3)
