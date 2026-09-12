// metapi-go/features/sites — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
//
// The `/sites` route reads the validateSearch schema from here and loads
// `SitesPage` directly; other features read the site list, the entity type and
// the endpoint guard (cross-feature detail sheets render site-provided URLs as
// links, and the SiteCreatedModal → /accounts handoff needs the entity).
// `SitesPage` is deliberately not exported here, so those consumers do not pull
// the page into their chunk.

export { sitesSearchSchema } from './lib/sites-schema'
// Endpoint URL guard (http/https + forbidden-host check) reused by
// cross-feature detail sheets that render site-provided URLs as links.
export { isValidEndpointUrl } from './lib/endpoints'

export { sitesKeys, type Site } from './types'
export { useSites } from './api'
