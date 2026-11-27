import { useNavigate, useSearch } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type { RouteSummaryRow } from '../types'
import { isExplicitGroupRoute, isExactModelPattern } from '../utils'

export function useRouteDialogs(
  routes: RouteSummaryRow[],
  isLoading: boolean,
  routeId: string
) {
  const routerSearch = useSearch({ from: '/_authenticated/token-routes' })
  const navigate = useNavigate()
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editRoute, setEditRoute] = useState<RouteSummaryRow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRoute, setDetailRoute] = useState<RouteSummaryRow | null>(null)

  const openDetail = useCallback((route: RouteSummaryRow) => {
    setDetailRoute(route)
    setDetailOpen(true)
  }, [])

  // One-shot route drilldown (proxy-log detail -> `?routeId=N`): wait for
  // the list, open the detail sheet for the referenced route, then strip
  // the param so a refetch or remount never reopens the sheet. A stale or
  // unknown id is stripped without opening anything.
  const routeDrilldownConsumed = useRef(false)
  useEffect(() => {
    if (routeDrilldownConsumed.current || !routeId) return
    if (isLoading) return

    const targetId = Number(routeId)
    const target = routes.find((route) => route.id === targetId)
    routeDrilldownConsumed.current = true
    if (target) openDetail(target)
    navigate({
      to: '/token-routes',
      search: { ...routerSearch, routeId: undefined },
      replace: true,
    })
  }, [routeId, isLoading, routes, routerSearch, navigate, openDetail])

  const openCreate = () => {
    setFormMode('create')
    setEditRoute(null)
    setFormOpen(true)
  }

  const openEdit = useCallback((route: RouteSummaryRow) => {
    setFormMode('edit')
    setEditRoute(route)
    setFormOpen(true)
  }, [])

  // One-shot edit drilldown (channel detail sheet -> `?edit=N`): wait for
  // the list, open the edit dialog for the referenced route (the same state
  // the row edit action uses), then strip the param so a refetch or remount
  // never reopens it. A stale or unknown id is stripped without opening
  // anything — mirrors the sites page's `edit` consumption and this page's
  // `routeId` drilldown, including the strict-mode re-entry guard.
  const editDrilldownConsumed = useRef(false)
  useEffect(() => {
    if (editDrilldownConsumed.current || routerSearch.edit === undefined) {
      return
    }
    if (isLoading) return

    const target = routes.find((route) => route.id === routerSearch.edit)
    editDrilldownConsumed.current = true
    if (target) {
      openEdit(target)
    }
    navigate({
      to: '/token-routes',
      search: { ...routerSearch, edit: undefined },
      replace: true,
    })
  }, [routerSearch, isLoading, routes, navigate, openEdit])

  const availableRoutes = useMemo(
    () =>
      routes.filter(
        (route) =>
          !isExplicitGroupRoute(route) &&
          isExactModelPattern(route.modelPattern) &&
          route.id !== editRoute?.id
      ),
    [routes, editRoute]
  )

  const openWorkspaceRoute = useCallback(
    (routeId: number) => {
      const target = routes.find((route) => route.id === routeId)
      if (target) openDetail(target)
    },
    [routes, openDetail]
  )

  const editDiagnosticRoute = useCallback(
    (routeId: number) => {
      const target = routes.find((route) => route.id === routeId)
      if (!target || target.kind === 'zero_channel' || target.readOnly) return
      setDetailOpen(false)
      openEdit(target)
    },
    [routes, openEdit]
  )

  return {
    openCreate,
    openEdit,
    openDetail,
    openWorkspaceRoute,
    editDiagnosticRoute,
    formProps: {
      open: formOpen,
      onOpenChange: setFormOpen,
      mode: formMode,
      route: editRoute,
      availableRoutes,
    },
    detailProps: {
      route: detailRoute,
      open: detailOpen,
      onOpenChange: setDetailOpen,
      onEditRoute: editDiagnosticRoute,
      onEdit: (route: RouteSummaryRow) => {
        setDetailOpen(false)
        openEdit(route)
      },
    },
  }
}
