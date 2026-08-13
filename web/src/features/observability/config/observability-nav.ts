// metapi-go/features/observability/config — sidebar drill-in view.
//
// Entering /observability swaps the root navigation for a contextual view
// (Overview / Health / Proxy Logs) with a "Back to Home" affordance,
// mirroring the Settings drill-in. Section titles are i18n keys resolved at
// render time by nav-group.tsx.

import type { NavGroup, SidebarView } from '@/components/layout/types'

import { getObservabilitySectionNavItems } from './observability-config'

function getObservabilityNavGroups(): NavGroup[] {
  return [
    {
      id: 'observability',
      title: 'sidebar.groups.observability',
      items: getObservabilitySectionNavItems().map((item) => ({
        title: item.title,
        url: item.url,
      })),
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
