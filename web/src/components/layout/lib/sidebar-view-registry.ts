// metapi-go/layout — sidebar-view-registry ported from newapi. AGPL header stripped.
// Resolves the active nested drill-in view for a given pathname.
//
// S5 boundary inversion: the shell owns this registry. Feature-owned views
// register through `registerSidebarView` from the authenticated route's
// composition root (routes/_authenticated/route.tsx) instead of the shell
// importing features (components ↛ features, see
// docs/internal/web-package-boundaries.md).

import { SYSTEM_SETTINGS_VIEW } from '../config/system-settings.config'
import type { SidebarView } from '../types'

/**
 * Registered nested sidebar views.
 *
 * Each entry describes a contextual sidebar that replaces the root
 * navigation when the user enters that workspace (Vercel-style
 * "drill-in" pattern).
 *
 * Match priority is array order; the first matching `pathPattern` wins.
 * The settings view is layout-owned and registered statically; feature
 * views are appended by the composition root before first render.
 */
const SIDEBAR_VIEWS: SidebarView[] = [SYSTEM_SETTINGS_VIEW]

/**
 * Register a drill-in view owned outside the layout shell.
 *
 * Idempotent per view id: re-registering the same id replaces the entry
 * (keeps HMR re-evaluation from duplicating views).
 */
export function registerSidebarView(view: SidebarView): void {
  const existingIndex = SIDEBAR_VIEWS.findIndex((entry) => entry.id === view.id)
  if (existingIndex >= 0) {
    SIDEBAR_VIEWS[existingIndex] = view
    return
  }
  SIDEBAR_VIEWS.push(view)
}

/**
 * Resolve the active nested view for the given path.
 *
 * @returns Matching SidebarView, or `null` when the root
 *          navigation should be displayed.
 */
export function resolveSidebarView(pathname: string): SidebarView | null {
  return SIDEBAR_VIEWS.find((view) => view.pathPattern.test(pathname)) ?? null
}
