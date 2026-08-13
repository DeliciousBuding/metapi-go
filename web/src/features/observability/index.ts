// metapi-go/features/observability — barrel.
//
// Public API for the Observability workspace (Overview / Health / Proxy
// Logs). Consumers (route file, sidebar, drill-in registry) import from
// `@/features/observability`.

export { ObservabilityPage } from './components/observability-page'

export { OBSERVABILITY_VIEW } from './config/observability-nav'

export { observabilitySearchSchema } from './lib/observability-schema'

export type { ObservabilitySectionId } from './types'
