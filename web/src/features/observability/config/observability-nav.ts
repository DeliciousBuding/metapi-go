// metapi-go/features/observability/config — sidebar drill-in view.
//
// Entering /observability swaps the root navigation for a contextual view
// (Overview / Health / Proxy Logs) with a "Back to Home" affordance,
// mirroring the Settings drill-in. Section titles are i18n keys resolved at
// render time by nav-group.tsx.
//
// Proxy logs are not an in-page section: the item deep-links to the
// dedicated `/proxy-logs` workspace (its own URL state + page title).

import type { NavGroup, NavItem, SidebarView } from '@/components/layout/types'

import {
  OBSERVABILITY_DEFAULT_SECTION,
  getObservabilitySectionNavItems,
} from './observability-config'

function getObservabilityNavGroups(): NavGroup[] {
  const sectionItems: NavItem[] = getObservabilitySectionNavItems().map(
    (item) => ({
      title: item.title,
      url: item.url,
      // Bare /observability renders the default section, so its nav item
      // must highlight before any `?section=` param exists in the URL.
      ...(item.url === `/observability?section=${OBSERVABILITY_DEFAULT_SECTION}`
        ? { activeUrls: ['/observability'] }
        : {}),
    })
  )

  return [
    {
      id: 'observability',
      title: 'sidebar.groups.observability',
      items: [
        ...sectionItems,
        {
          // Deep link out of the workspace: clicking swaps the sidebar back
          // to the root navigation, where /proxy-logs lives under Console.
          title: 'observability.sections.proxyLogs.title',
          url: '/proxy-logs',
        },
      ],
    },
  ]
}

export const OBSERVABILITY_VIEW: SidebarView = {
  id: 'observability',
  pathPattern: /^\/observability(\/|$)/,
  parent: {
    to: '/',
    label: 'sidebar.backToHome',
  },
  getNavGroups: getObservabilityNavGroups,
}
