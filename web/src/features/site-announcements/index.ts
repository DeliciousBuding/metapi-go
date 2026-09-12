// metapi-go/features/site-announcements — public barrel.
//
// Cross-feature consumers import from here and not from a subdirectory: what
// is exported below is frozen against their call sites, and what is not
// exported is free to move.
//
// The `/site-announcements` route builds its loader prefetch params through
// the same helper the page uses, so the prefetch cache key matches the page's
// first fetch. `SiteAnnouncementsPage` is loaded by that route directly and is
// deliberately not exported here.

export {
  buildSiteAnnouncementsParams,
  DEFAULT_SITE_ANNOUNCEMENTS_FILTERS,
  siteAnnouncementsKeys,
} from './types'
