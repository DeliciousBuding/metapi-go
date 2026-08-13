// metapi-go/features/observability — barrel.
//
// Public API for the Observability workspace (Overview / Health / Proxy
// Logs). Consumers (route file, sidebar, drill-in registry) import from
// `@/features/observability`.

export { ObservabilityPage } from './components/observability-page'

export {
  OBSERVABILITY_DEFAULT_SECTION,
  OBSERVABILITY_SECTION_IDS,
} from './config/observability-config'

export { OBSERVABILITY_VIEW } from './config/observability-nav'

export {
  observabilitySearchSchema,
  type ObservabilitySearch,
} from './lib/observability-schema'

export { useMonitorHealth, useSlowRequests, useUsageHeatmap } from './api'

export type { ObservabilitySectionId } from './types'
