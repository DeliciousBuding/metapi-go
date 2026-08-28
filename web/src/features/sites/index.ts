// metapi-go/features/sites — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the future
// `/sites` route file (validateSearch schema + types) and for cross-feature
// deep linking (the SiteCreatedModal → /accounts handoff).

export { sitesSearchSchema } from './lib/sites-schema'
// Endpoint URL guard (http/https + forbidden-host check) reused by
// cross-feature detail sheets that render site-provided URLs as links.
export { isValidEndpointUrl } from './lib/endpoints'

export { sitesKeys } from './types'
export { useSites } from './api'
