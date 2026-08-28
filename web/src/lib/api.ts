/**
 * metapi-go business API surface.
 *
 * Barrel over the domain modules in `./api/` (sites, accounts, routes,
 * stats, oauth, settings, ...). Keeping this file means the feature code that
 * does `import { api } from '@/lib/api'` keeps working unchanged. All
 * response types are re-exported from `./api/types`.
 */

import { accountsApi } from './api/accounts'
import { catalogSourcesApi } from './api/catalog-sources'
import { eventsApi } from './api/events'
import { oauthApi } from './api/oauth'
import { probeHistoryApi } from './api/probe-history'
import { searchApi } from './api/search'
import { settingsApi } from './api/settings'
import { siteAnnouncementsApi } from './api/site-announcements'
import { sitesApi } from './api/sites'
import { statsApi } from './api/stats'
import { systemApi } from './api/system'
import { testChatApi } from './api/test-chat'
import { tokenRoutesApi } from './api/token-routes'

export * from './api/types'

export const api = {
  ...sitesApi,
  ...siteAnnouncementsApi,
  ...accountsApi,
  ...tokenRoutesApi,
  ...statsApi,
  ...searchApi,
  ...oauthApi,
  ...probeHistoryApi,
  ...eventsApi,
  ...settingsApi,
  ...systemApi,
  ...testChatApi,
  ...catalogSourcesApi,
}
