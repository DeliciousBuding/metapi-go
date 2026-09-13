// metapi-go/features/observability — public barrel.
//
// Public API for the Observability workspace: two sections (Overview, Health)
// behind one page, switched by the in-content tabs (the dashboard pattern).
// Proxy logs is not one of them — it is a separate workspace at `/proxy-logs`
// that lives in the root navigation. (The sidebar drill-in view was removed
// in #1353: it duplicated the in-content tabs at the same navigation level.)
//
// Both consumers are route files: `routes/_authenticated/observability.tsx`
// (page + section-id type + search schema).

export { ObservabilityPage } from './components/observability-page'

export { observabilitySearchSchema } from './lib/observability-schema'

export type { ObservabilitySectionId } from './types'
