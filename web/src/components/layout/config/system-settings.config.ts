// metapi-go/layout — system-settings.config adapted from newapi per plan.md §5.5.2.
// metapi Settings is a 5-subarea drill-in workspace (general / downstream / models /
// content / system-info). The 7-section newapi registry is collapsed to metapi's 5.
// Titles are i18n keys resolved via t() at render time (nav-group.tsx /
// sidebar-view-header.tsx).
//
// IA restructure (wave 8 lane C): the 5 subareas render as NavCollapsible
// nested-tree entries whose sub-items are the subarea's sections (from the
// shared section-registry manifest). Together with the removed in-page
// settings sidebar, breadcrumbs and overview section lists, the sidebar tree
// is now the single navigation surface for the Settings workspace — aligned
// with the newapi "System Administration" drill-in view. The active subarea
// auto-expands (activePrefix + checkIsActive) and the active section is
// highlighted with aria-current.

import { LayoutGrid } from 'lucide-react'

import { getSettingsSubareas } from '../lib/settings-nav-registry'
import type { NavCollapsible, NavGroup, SidebarView } from '../types'

/**
 * Sidebar nav groups for the Settings nested view.
 *
 * Kept as a single group because the workspace title in the sidebar
 * header already provides top-level context — the inner group label
 * scopes the items as "administration" actions.
 */
function getSettingsNavGroups(): NavGroup[] {
  const subareas = getSettingsSubareas()
  return [
    {
      id: 'system-administration',
      title: 'sidebar.groups.systemAdministration',
      items: [
        {
          title: 'sidebar.settingsOverview',
          url: '/settings',
          icon: LayoutGrid,
          // The overview is the workspace root: only exact /settings is the
          // current page, never /settings/<subarea>/... descendants.
          activeOptions: { exact: true },
        },
        ...subareas.map((subarea) => {
          const collapsible: NavCollapsible = {
            title: subarea.title,
            icon: subarea.icon,
            // Keep the subarea open + highlighted on every one of its
            // section URLs (checkIsActive → SidebarMenuCollapsible opens it).
            activePrefix: subarea.basePath,
            items: subarea.getSectionNavItems().map((section) => ({
              title: section.title,
              url: section.url,
              // Read-only surfaces (audit logs, update center) keep their
              // "readonly" marker as a small inline badge instead of the
              // retired chip strip (audit P2 #6 regression-intent change).
              ...(section.readonly
                ? { badge: 'settings.common.readonly' }
                : {}),
            })),
          }
          return collapsible
        }),
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
