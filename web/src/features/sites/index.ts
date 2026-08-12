// metapi-go/features/sites — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the future
// `/sites` route file (validateSearch schema + types) and for cross-feature
// deep linking (the SiteCreatedModal → /accounts handoff).

export { sitesSearchSchema } from './lib/sites-schema'

export { sitesKeys } from './types'
