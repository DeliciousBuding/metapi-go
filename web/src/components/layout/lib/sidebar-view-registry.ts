// metapi-go/layout — the registry of drill-in sidebar views, and the pathname
// lookup that picks one.
//
// The shell owns this module and features push into it, never the reverse: a
// feature-owned view registers through `registerSidebarView` from the
// authenticated route's composition root (routes/_authenticated/route.tsx),
// which keeps `components/ ↛ features/` intact (see
// docs/internal/web-package-boundaries.md). The inversion is what lets a
// workspace live in its own feature folder and still replace the sidebar.

import { SYSTEM_SETTINGS_VIEW } from '../config/system-settings.config'
import type { SidebarView } from '../types'

/**
 * Registered views, in match order: the first `pathPattern` that tests true
 * wins, so a narrower pattern must be registered before a broader one.
 * Settings is layout-owned and therefore static; feature views are appended by
 * the composition root before first render.
 */
const SIDEBAR_VIEWS: SidebarView[] = [SYSTEM_SETTINGS_VIEW]

/**
 * Register a view owned outside the layout shell.
 *
 * Idempotent per `id` — re-registering replaces the entry, so a dev-server
 * module re-evaluation cannot leave the same workspace in the list twice.
 */
export function registerSidebarView(view: SidebarView): void {
  const existingIndex = SIDEBAR_VIEWS.findIndex((entry) => entry.id === view.id)
  if (existingIndex >= 0) {
    SIDEBAR_VIEWS[existingIndex] = view
    return
  }
  SIDEBAR_VIEWS.push(view)
}

/** The view for `pathname`, or null when the root navigation applies. */
export function resolveSidebarView(pathname: string): SidebarView | null {
  return SIDEBAR_VIEWS.find((view) => view.pathPattern.test(pathname)) ?? null
}
