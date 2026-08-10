// metapi-go/layout — sidebar-view-registry ported from newapi. AGPL header stripped.
// Resolves the active nested drill-in view for a given pathname.

import type { NavGroup, SidebarView } from '../types'
import { SYSTEM_SETTINGS_VIEW } from '../config/system-settings.config'

/**
 * Registered nested sidebar views.
 *
 * Each entry describes a contextual sidebar that replaces the root
 * navigation when the user enters that workspace (Vercel-style
 * "drill-in" pattern). Add new entries here to register a new view.
 *
 * Match priority is array order; the first matching `pathPattern` wins.
 */
const SIDEBAR_VIEWS: readonly SidebarView[] = [SYSTEM_SETTINGS_VIEW]

/**
 * Resolve the active nested view for the given path.
 *
 * @returns Matching SidebarView, or `null` when the root
 *          navigation should be displayed.
 */
export function resolveSidebarView(pathname: string): SidebarView | null {
  return SIDEBAR_VIEWS.find((view) => view.pathPattern.test(pathname)) ?? null
}

/**
 * Backwards-compatible helper for consumers (e.g. command palette) that
 * just need the navigation groups for the current path, without caring
 * about the view metadata.
 *
 * @returns Nav groups for the matched view, or `null` if no nested view
 *          matches (callers should then fall back to root nav groups).
 */
export function getNavGroupsForPath(pathname: string): NavGroup[] | null {
  const view = resolveSidebarView(pathname)
  return view ? view.getNavGroups() : null
}
