// metapi-go/layout — the drill-in sidebar views, and the pathname lookup that
// picks one.
//
// Settings is the only remaining drill-in view and it is layout-owned, so the
// list is static. (Observability's drill-in was removed in #1353 — it
// duplicated the page's in-content tabs.) If a future feature needs its own
// drill-in, reintroduce a register function called from the authenticated
// route's composition root so `components/ ↛ features/` stays intact (see
// docs/internal/web-package-boundaries.md).

import { SYSTEM_SETTINGS_VIEW } from '../config/system-settings.config'
import type { SidebarView } from '../types'

/**
 * Views in match order: the first `pathPattern` that tests true wins, so a
 * narrower pattern must come before a broader one.
 */
const SIDEBAR_VIEWS: SidebarView[] = [SYSTEM_SETTINGS_VIEW]

/** The view for `pathname`, or null when the root navigation applies. */
export function resolveSidebarView(pathname: string): SidebarView | null {
  return SIDEBAR_VIEWS.find((view) => view.pathPattern.test(pathname)) ?? null
}
