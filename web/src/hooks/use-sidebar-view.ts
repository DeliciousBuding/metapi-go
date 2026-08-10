// metapi-go/hooks — use-sidebar-view ported from newapi, simplified for metapi.
// metapi has no role-based filtering (fully open) and no server-side module gating,
// so this hook only resolves the nested drill-in view (Settings) vs root nav.
// TODO(phase 2): re-add role filtering once auth-store + roles land.

import { useLocation } from '@tanstack/react-router'
import { useMemo } from 'react'

import { resolveSidebarView } from '@/components/layout/lib/sidebar-view-registry'
import type { NavGroup, ResolvedSidebarView } from '@/components/layout/types'

import { useSidebarData } from './use-sidebar-data'

/** Sentinel key used for the root navigation in animation `key=` props */
const ROOT_VIEW_KEY = '__root'

/**
 * Resolve the active sidebar view for the current location.
 *
 * - Returns the matching nested SidebarView (with its nav groups) when the
 *   URL belongs to a registered drill-in workspace (e.g. /settings/*).
 * - Otherwise returns the root navigation from useSidebarData unfiltered.
 */
export function useSidebarView(): ResolvedSidebarView {
  const pathname = useLocation({ select: (l) => l.pathname })
  const rootSidebarData = useSidebarData()

  const rootNavGroups = useMemo<NavGroup[]>(
    () => rootSidebarData.navGroups,
    [rootSidebarData]
  )

  const view = resolveSidebarView(pathname)

  if (view) {
    return {
      key: view.id,
      view,
      navGroups: view.getNavGroups(),
    }
  }

  return {
    key: ROOT_VIEW_KEY,
    view: null,
    navGroups: rootNavGroups,
  }
}
