// metapi-go/layout — system-settings.config adapted from newapi per plan.md §5.5.2.
// metapi Settings is a 5-subarea drill-in workspace (general / downstream / models /
// content / system-info). The 7-section newapi registry is collapsed to metapi's 5.
// TODO(phase 2): wrap labels in useTranslation once i18n is wired.

import {
  Boxes,
  KeyRound,
  Package,
  ServerCog,
  Settings,
} from 'lucide-react'

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
      title: 'System Administration',
      items: [
        {
          title: 'General',
          icon: Settings,
          url: '/settings/general',
        },
        {
          title: 'Downstream Keys',
          icon: KeyRound,
          url: '/settings/downstream',
        },
        {
          title: 'Models',
          icon: Boxes,
          url: '/settings/models',
        },
        {
          title: 'Content',
          icon: Package,
          url: '/settings/content',
        },
        {
          title: 'System Info',
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
    label: 'Back to Home',
  },
  getNavGroups: getSettingsNavGroups,
}
