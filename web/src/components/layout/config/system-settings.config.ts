// metapi-go/layout — the Settings drill-in view: its back target and its
// navigation groups.
//
// Settings is five subareas (basic / proxy-models / downstream / content /
// operations), each rendered as a `NavCollapsible` whose sub-items are that
// subarea's sections, read from the shared section-registry manifest so a
// section added by a feature appears in the sidebar without an edit here.
//
// This sidebar tree is the *only* navigation surface for the workspace — there
// is no in-page section list, breadcrumb trail or overview index to keep in
// sync. That is why the active subarea auto-expands (`activePrefix` +
// `checkIsActive`) and the active section carries `aria-current`: losing the
// highlight would leave the user with no way to tell where they are.
//
// Titles are i18n keys resolved with t() at render time (nav-group.tsx /
// sidebar-view-header.tsx).

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
