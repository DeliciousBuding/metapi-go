// metapi-go/features/observability — public barrel.
//
// Public API for the Observability workspace: two sections (Overview, Health)
// behind one page. Proxy logs is not one of them — it is a separate workspace
// at `/proxy-logs` that this feature's sidebar view deep-links out to
// (`config/observability-nav.ts`).
//
// Both consumers are route files, which is why nothing here is a page-less
// surface: `routes/_authenticated/observability.tsx` (page + section-id type +
// search schema) and `routes/_authenticated/route.tsx` (registers
// `OBSERVABILITY_VIEW` with the sidebar view registry).

export { ObservabilityPage } from './components/observability-page'

export { OBSERVABILITY_VIEW } from './config/observability-nav'

export { observabilitySearchSchema } from './lib/observability-schema'

export type { ObservabilitySectionId } from './types'
