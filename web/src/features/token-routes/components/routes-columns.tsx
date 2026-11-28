import type { ColumnDef } from '@tanstack/react-table'
import { CircleSlash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'

import type { RouteRowActions, RouteSummaryRow } from '../types'
import { resolveRouteTitle } from '../utils'
import { RoutesRowActions } from './routes-row-actions'
import { RoutesTitleCell, type RouteDisclosure } from './routes-title-cell'

function isReadOnlyRoute(route: RouteSummaryRow): boolean {
  return route.kind === 'zero_channel' || route.readOnly === true
}

export function useRoutesColumns(
  actions: RouteRowActions,
  pendingToggleId: number | null = null,
  pendingCooldownId: number | null = null,
  cooldownRouteIds?: ReadonlySet<number>,
  disclosure?: RouteDisclosure
): ColumnDef<RouteSummaryRow>[] {
  const { t } = useTranslation()
  return [
    {
      id: 'select',
      size: 40,
      enableSorting: false,
      enableHiding: false,
      header: ({ table }) => (
        <Checkbox
          checked={table.getIsAllPageRowsSelected()}
          onCheckedChange={(value) =>
            table.toggleAllPageRowsSelected(Boolean(value))
          }
          aria-label={t('tokenRoutes.columns.selectAll')}
        />
      ),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) {
          return <span className='text-muted-foreground text-xs'>—</span>
        }
        return (
          <Checkbox
            checked={row.getIsSelected()}
            onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
            aria-label={t('tokenRoutes.columns.selectRow')}
          />
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'title',
      size: 520,
      accessorFn: (row) => resolveRouteTitle(row as RouteSummaryRow),
      header: t('tokenRoutes.columns.modelAndName'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        return (
          <RoutesTitleCell
            route={route}
            readOnly={isReadOnlyRoute(route)}
            disclosure={disclosure}
          />
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'enabled',
      size: 56,
      accessorFn: (row) => {
        const route = row as RouteSummaryRow
        if (isReadOnlyRoute(route)) return 'readonly'
        return route.enabled ? 'enabled' : 'disabled'
      },
      header: t('tokenRoutes.columns.enabled'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) {
          return (
            <span
              className='text-muted-foreground inline-flex'
              title={t('tokenRoutes.columns.notEnabled')}
            >
              <CircleSlash2 className='size-4' aria-hidden='true' />
              <span className='sr-only'>
                {t('tokenRoutes.columns.notEnabled')}
              </span>
            </span>
          )
        }
        const pending = pendingToggleId === route.id
        const actionLabel = route.enabled
          ? t('tokenRoutes.columns.disable')
          : t('tokenRoutes.columns.enable')
        return (
          <Switch
            size='sm'
            checked={route.enabled}
            disabled={pending}
            onCheckedChange={() => actions.onToggleEnabled(route)}
            aria-label={actionLabel}
            title={actionLabel}
          />
        )
      },
      filterFn: (row, _columnId, filterValue: unknown) => {
        if (!Array.isArray(filterValue) || filterValue.length === 0) return true
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) return false
        const status = route.enabled ? 'enabled' : 'disabled'
        return filterValue.includes(status)
      },
      meta: { mobileBadge: true },
    },
    {
      id: 'actions',
      size: 48,
      enableSorting: false,
      enableHiding: false,
      header: () => <span className='sr-only'>{t('common.actions')}</span>,
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        return (
          <RoutesRowActions
            route={route}
            readOnly={isReadOnlyRoute(route)}
            actions={actions}
            pendingToggleId={pendingToggleId}
            pendingCooldownId={pendingCooldownId}
            cooldownRouteIds={cooldownRouteIds}
          />
        )
      },
      // The card header already carries the state badge; putting the menu
      // there removes the lone bottom-right row every card used to spend.
      meta: { pinned: 'right', mobileActionsInHeader: true },
    },
  ]
}
