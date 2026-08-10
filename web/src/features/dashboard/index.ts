// metapi-go/features/dashboard — barrel.
//
// Public API for the 4-section Dashboard workspace (plan §5.5.1). Consumers
// (route files, the main sidebar, future feature modules) import from
// `@/features/dashboard`.

// Types
export type {
  AnnouncementItem,
  DashboardSection,
  DashboardSectionId,
  DashboardSectionNavItem,
  IncomeOutcomePoint,
  ModelCostRow,
  RealtimeOpsFrame,
  RealtimeOpsSample,
  SiteDistributionSlice,
  SiteTrendPoint,
  VChartSpec,
} from './types'

// Generic factory + registry types
export {
  createSectionRegistry,
  type SectionRegistry,
  type SectionRegistryConfig,
} from './utils/section-registry'

// 4-section manifest + registry helpers (route registration surface)
export {
  DASHBOARD_DEFAULT_SECTION,
  DASHBOARD_SECTION_IDS,
  getDashboardSectionContent,
  getDashboardSectionMeta,
  getDashboardSectionNavItems,
} from './config/dashboard-config'

// Section dispatcher
export { DashboardPage, type DashboardPageProps } from './components/dashboard-page'

// Shared components
export { AnnouncementBanner } from './components/announcement-banner'
export { ChartShell } from './components/chart-shell'
export { StatCard } from './components/stat-card'

// Hooks
export {
  useChartColors,
  useThemeLabelColor,
  CHART_COLORS_FALLBACK,
  type ChartColors,
} from './hooks/use-chart-colors'
export { useRealtimeOps } from './hooks/use-realtime-ops'

// VChart spec builders
export {
  VCHART_OPTION,
  buildIncomeOutcomeSpec,
  buildModelCostSpec,
  buildSiteDistributionSpec,
  buildSiteTrendSpec,
  prefersReducedMotion,
} from './lib/chart-specs'

// The 4 sections (lazy imports land at the call site in phase 3)
export {
  OverviewSection,
  TrafficSection,
  ModelsSection,
  AvailabilitySection,
} from './sections'
