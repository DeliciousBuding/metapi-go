/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go features/token-routes/components — TanStack Table column
// definitions for the routes list.
//
// `routingStrategyLabel()` returns an i18n key; callers wrap with `t()`.

import type { ColumnDef } from '@tanstack/react-table'
import {
  CircleSlash2,
  Eye,
  Layers3,
  MoreHorizontal,
  Pencil,
  Power,
  RadioTower,
  Snowflake,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { BrandGlyph } from '@/assets/brand-icons/BrandIcon'
import { RowDisclosureTrigger } from '@/components/common/row-disclosure'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

import type {
  RouteRowActions,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from '../types'
import {
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

function isReadOnlyRoute(route: RouteSummaryRow): boolean {
  return route.kind === 'zero_channel' || route.readOnly === true
}

function RoutesRowActions({
  route,
  actions,
  pendingToggleId = null,
  pendingCooldownId = null,
  cooldownRouteIds,
}: {
  route: RouteSummaryRow
  actions: RouteRowActions
  pendingToggleId?: number | null
  pendingCooldownId?: number | null
  /** Route ids with at least one channel in cooldown/breaker state (derived
   * from the channels-list query by the page). `undefined` means still
   * loading — the item renders disabled until the set resolves. */
  cooldownRouteIds?: ReadonlySet<number>
}) {
  const { t } = useTranslation()
  const readOnly = isReadOnlyRoute(route)
  const isTogglePending = pendingToggleId === route.id
  const isCooldownPending = pendingCooldownId === route.id
  // No active cooldown → hide the item entirely. Clicking it with nothing
  // to clear only produced a false "已清除冷却" success toast (the detail
  // sheet gates the same action on `hasActiveCooldown`).
  const showClearCooldown =
    cooldownRouteIds === undefined || cooldownRouteIds.has(route.id)
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon-sm'
            className='data-popup-open:bg-accent'
            aria-label={t('tokenRoutes.columns.rowActions')}
            data-hit-area
          />
        }
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(route)}>
          <Eye />
          {t('tokenRoutes.columns.viewDetails')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onToggleEnabled(route)}
          disabled={readOnly || isTogglePending}
        >
          {isTogglePending ? <Spinner /> : <Power />}
          {route.enabled
            ? t('tokenRoutes.columns.disable')
            : t('tokenRoutes.columns.enable')}
        </DropdownMenuItem>
        {showClearCooldown && (
          <DropdownMenuItem
            onClick={() => actions.onClearCooldown(route)}
            disabled={readOnly || isCooldownPending}
          >
            {isCooldownPending ? <Spinner /> : <Snowflake />}
            {t('tokenRoutes.columns.clearCooldown')}
          </DropdownMenuItem>
        )}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => actions.onEdit(route)}
          disabled={readOnly}
        >
          <Pencil />
          {t('common.edit')}
        </DropdownMenuItem>
        <DropdownMenuItem
          variant='destructive'
          onClick={() => actions.onDelete(route)}
          disabled={readOnly || actions.isDeletePending}
        >
          <Trash2 />
          {t('common.delete')}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

export function useRoutesColumns(
  actions: RouteRowActions,
  pendingToggleId: number | null = null,
  pendingCooldownId: number | null = null,
  cooldownRouteIds?: ReadonlySet<number>,
  disclosure?: {
    expandedIds: ReadonlySet<number>
    toggle: (id: number) => void
    getContentId: (id: number) => string
  }
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
        const title = resolveRouteTitle(route)
        const readOnly = isReadOnlyRoute(route)
        const group = isExplicitGroupRoute(route)
        const total = route.channelCount ?? 0
        const enabled = route.enabledChannelCount ?? 0
        const partial = enabled < total
        const sites = route.siteNames ?? []
        const modeLabel = t(
          group
            ? 'tokenRoutes.columns.badgeGroup'
            : 'tokenRoutes.columns.badgeMatch'
        )
        let channelLabel: string
        if (total === 0) {
          channelLabel = t('tokenRoutes.columns.channelZeroNeedChannels')
        } else if (partial) {
          channelLabel = t('tokenRoutes.columns.channelDisabledCount', {
            count: total - enabled,
          })
        } else {
          channelLabel = t('tokenRoutes.columns.channelEnabledSummary', {
            enabled,
            total,
          })
        }
        let channelTone = 'text-foreground'
        if (enabled === 0) channelTone = 'text-muted-foreground'
        else if (partial) channelTone = 'text-warning'

        return (
          <div className='flex min-w-0 items-center gap-2.5 py-0.5'>
            <span className='bg-background dark:bg-muted grid size-8 shrink-0 place-items-center rounded-lg shadow-sm ring-1 ring-black/8 dark:ring-white/12'>
              <BrandGlyph
                model={title}
                icon={route.displayIcon}
                size={22}
                alt=''
              />
            </span>
            <div className='min-w-0'>
              <div className='flex min-w-0 items-center gap-1.5'>
                {disclosure ? (
                  <RowDisclosureTrigger
                    expanded={disclosure.expandedIds.has(route.id)}
                    controls={disclosure.getContentId(route.id)}
                    onClick={() => disclosure.toggle(route.id)}
                    title={title}
                    className='min-h-0 gap-1 px-0 text-sm font-medium hover:bg-transparent'
                  >
                    <span className='min-w-0 truncate'>{title}</span>
                  </RowDisclosureTrigger>
                ) : (
                  <span className='max-w-60 truncate text-sm font-medium'>
                    {title}
                  </span>
                )}
                {group ? (
                  <span
                    className='text-muted-foreground inline-flex shrink-0'
                    title={modeLabel}
                  >
                    <Layers3 className='size-3.5' aria-hidden='true' />
                    <span className='sr-only'>{modeLabel}</span>
                  </span>
                ) : (
                  <span className='sr-only'>{modeLabel}</span>
                )}
                {readOnly && (
                  <span
                    className='text-muted-foreground inline-flex shrink-0'
                    title={t('tokenRoutes.columns.notGenerated')}
                  >
                    <CircleSlash2 className='size-3.5' aria-hidden='true' />
                    <span className='sr-only'>
                      {t('tokenRoutes.columns.notGenerated')}
                    </span>
                  </span>
                )}
              </div>
              <div className='text-muted-foreground text-2xs mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5'>
                <code
                  className='max-w-44 truncate font-mono'
                  title={route.modelPattern}
                >
                  {route.modelPattern}
                </code>
                {!readOnly && (
                  <>
                    <span aria-hidden='true'>·</span>
                    <span
                      className={cn(
                        'inline-flex shrink-0 items-center gap-1 tabular-nums',
                        channelTone
                      )}
                      title={channelLabel}
                    >
                      <RadioTower className='size-3' aria-hidden='true' />
                      {enabled}/{total}
                    </span>
                  </>
                )}
                {sites.length > 0 && (
                  <>
                    <span aria-hidden='true'>·</span>
                    <span
                      className='min-w-0 truncate'
                      title={sites.join(' → ')}
                    >
                      {sites.slice(0, 2).join(' → ')}
                      {sites.length > 2 && ` +${sites.length - 2}`}
                    </span>
                  </>
                )}
              </div>
            </div>
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'strategy',
      size: 112,
      accessorFn: (row) => (row as RouteSummaryRow).routingStrategy ?? '',
      header: t('tokenRoutes.columns.strategy'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) {
          return <span className='text-muted-foreground text-xs'>—</span>
        }
        const label = routingStrategyLabel(
          route.routingStrategy as RouteRoutingStrategy | null
        )
        return <Badge variant='outline'>{t(label)}</Badge>
      },
      meta: { mobileOrder: 3 },
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
