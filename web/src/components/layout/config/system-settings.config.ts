// metapi-go/layout — system-settings.config adapted from newapi per plan.md §5.5.2.
// metapi Settings is a 5-subarea drill-in workspace (general / downstream / models /
// content / system-info). The 7-section newapi registry is collapsed to metapi's 5.
// Titles are i18n keys resolved via t() at render time (nav-group.tsx /
// sidebar-view-header.tsx).

import { Boxes, Database, KeyRound, ServerCog, Settings } from 'lucide-react'

import type { NavGroup, SidebarView } from '../types'

/**
 * Sidebar nav groups for the Settings nested view.
 *
 * Kept as a single group because the workspace title in the sidebar
 * header already provides top-level context — the inner group label
 * scopes the items as "administration" actions.
 */
function getSettingsNavGroups(): NavGroup[] {
  return [
    {
      id: 'system-administration',
      title: 'sidebar.groups.systemAdministration',
      items: [
        {
          title: 'sidebar.items.general',
          icon: Settings,
          url: '/settings/general',
        },
        {
          title: 'sidebar.items.downstreamKeys',
          icon: KeyRound,
          url: '/settings/downstream',
        },
        {
          title: 'sidebar.items.modelSettings',
          icon: Boxes,
          url: '/settings/models',
        },
        {
          title: 'sidebar.items.content',
          icon: Database,
          url: '/settings/content',
        },
        {
          title: 'sidebar.items.systemInfo',
          icon: ServerCog,
          url: '/settings/system-info',
        },
      ],
    },
  ]
}

/**
 * Nested sidebar view for `/settings/*`.
 *
 * Activates the Vercel / Cloudflare-style drill-in sidebar:
 * the root navigation is replaced by the system administration
 * groups, with a "Back to Home" affordance in the header.
 */
export const SYSTEM_SETTINGS_VIEW: SidebarView = {
  id: 'settings',
  pathPattern: /^\/settings(\/|$)/,
  parent: {
    to: '/',
    label: 'sidebar.backToHome',
  },
  getNavGroups: getSettingsNavGroups,
}
