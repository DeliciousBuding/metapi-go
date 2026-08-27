/**
 * metapi-go business API surface.
 *
 * Barrel over the domain modules in `./api/` (sites, accounts, routes,
 * stats, oauth, settings, ...). Keeping this file means the feature code that
 * does `import { api } from '@/lib/api'` keeps working unchanged. All
 * response types are re-exported from `./api/types`.
 */

import { accountsApi } from './api/accounts.ts'
import { catalogSourcesApi } from './api/catalog-sources.ts'
import { eventsApi } from './api/events.ts'
import { oauthApi } from './api/oauth.ts'
import { probeHistoryApi } from './api/probe-history.ts'
import { searchApi } from './api/search.ts'
import { settingsApi } from './api/settings.ts'
import { siteAnnouncementsApi } from './api/site-announcements.ts'
import { sitesApi } from './api/sites.ts'
import { statsApi } from './api/stats.ts'
import { systemApi } from './api/system.ts'
import { testChatApi } from './api/test-chat.ts'
import { tokenRoutesApi } from './api/token-routes.ts'

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
