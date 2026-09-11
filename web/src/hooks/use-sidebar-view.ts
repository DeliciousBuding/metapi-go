// metapi-go/hooks — useSidebarView: which navigation set the current URL selects.
//
// The root sidebar and a drill-in workspace are mutually exclusive, so this is a
// single lookup: `resolveSidebarView(pathname)` returns the registered view whose
// path pattern matches, or null for the root navigation. The returned `key` is
// what the sidebar animates on when the two swap.
//
// There is no role or module filtering here. metapi's routes are open and the
// route guards own access, so this hook resolves a view — it does not authorise
// one.

import { useLocation } from '@tanstack/react-router'

import { ROOT_NAVIGATION } from '@/components/layout/config/root-navigation'
import { resolveSidebarView } from '@/components/layout/lib/sidebar-view-registry'
import type { ResolvedSidebarView } from '@/components/layout/types'

/** Animation key for the root navigation, which has no view id of its own. */
const ROOT_VIEW_KEY = '__root'

export function useSidebarView(): ResolvedSidebarView {
  const pathname = useLocation({ select: (location) => location.pathname })
  const view = resolveSidebarView(pathname)

  return view
    ? { key: view.id, navGroups: view.getNavGroups(), view }
    : { key: ROOT_VIEW_KEY, navGroups: ROOT_NAVIGATION.navGroups, view: null }
}
