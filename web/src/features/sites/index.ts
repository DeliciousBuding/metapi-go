// metapi-go/features/sites — barrel re-exports.
//
// Page component is the primary surface; the rest is exported for the future
// `/sites` route file (validateSearch schema + types) and for cross-feature
// deep linking (the SiteCreatedModal → /accounts handoff).

export { SitesPage } from './components/sites-page'
export { SiteFormDialog } from './components/site-form-dialog'
export { SiteCreatedModal } from './components/site-created-modal'
export { SiteDetailSheet } from './components/site-detail-sheet'
export { useSitesColumns } from './components/sites-columns'

export {
  useSites,
  useCreateSite,
  useUpdateSite,
  useDeleteSite,
  useBatchUpdateSites,
  useDetectSite,
} from './api'

export {
  siteFormSchema,
  sitesSearchSchema,
  SITE_FORM_DEFAULT_VALUES,
  type SiteFormValues,
  type SitesSearch,
} from './lib/sites-schema'

export {
  sitesKeys,
  type Site,
  type SiteStatus,
  type SiteProbeScope,
  type SiteApiEndpoint,
  type SiteSubscriptionSummary,
  type SiteFormPayload,
  type SiteBatchResult,
  type SiteBatchAction,
} from './types'
