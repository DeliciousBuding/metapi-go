/* eslint-disable react/only-export-components -- column definitions co-located with cell renderers */
// metapi-go features/token-routes/components — TanStack Table column
// definitions for the routes list.
//
// `routingStrategyLabel()` returns an i18n key; callers wrap with `t()`.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye,
  Loader2,
  MoreHorizontal,
  Pencil,
  Power,
  Snowflake,
  Trash2,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

import type {
  RouteRowActions,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from '../types'
import {
  formatContextLength,
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

function isReadOnlyRoute(route: RouteSummaryRow): boolean {
  return route.kind === 'zero_channel' || route.readOnly === true
}

function resolveModeBadge(route: RouteSummaryRow): {
  labelKey: string
  variant: 'default' | 'secondary' | 'outline'
} {
  if (isExplicitGroupRoute(route)) {
    return { labelKey: 'tokenRoutes.columns.badgeGroup', variant: 'secondary' }
  }
  return { labelKey: 'tokenRoutes.columns.badgeMatch', variant: 'outline' }
}

function resolveChannelSummary(
  route: RouteSummaryRow,
  t: (key: string, params?: Record<string, unknown>) => string
): {
  label: string
  variant: 'success' | 'warning' | 'secondary'
  hint?: string
} {
  if (isReadOnlyRoute(route)) {
    return {
      label: t('tokenRoutes.columns.channelZeroGenerated'),
      variant: 'warning',
      hint: t('tokenRoutes.columns.channelZeroHint'),
    }
  }
  const total = route.channelCount ?? 0
  const enabled = route.enabledChannelCount ?? 0
  if (total === 0) {
    return {
      label: t('tokenRoutes.columns.channelZeroNeedChannels'),
      variant: 'warning',
      hint: t('tokenRoutes.columns.channelZeroNeedChannelsHint'),
    }
  }
  if (enabled === 0) {
    return {
      label: `${total} ${t('tokenRoutes.columns.channels')}`,
      variant: 'secondary',
      hint: t('tokenRoutes.columns.channelAllDisabled'),
    }
  }
  return {
    label: `${enabled}/${total}`,
    variant: enabled === total ? 'success' : 'warning',
    hint:
      enabled < total
        ? t('tokenRoutes.columns.channelDisabledCount', {
            count: total - enabled,
          })
        : undefined,
  }
}

function RoutesRowActions({
  route,
  actions,
  pendingToggleId = null,
  pendingCooldownId = null,
}: {
  route: RouteSummaryRow
  actions: RouteRowActions
  pendingToggleId?: number | null
  pendingCooldownId?: number | null
}) {
  const { t } = useTranslation()
  const readOnly = isReadOnlyRoute(route)
  const isTogglePending = pendingToggleId === route.id
  const isCooldownPending = pendingCooldownId === route.id
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className='text-muted-foreground hover:bg-muted hover:text-foreground inline-flex size-8 items-center justify-center rounded-md transition-colors outline-none'
        aria-label={t('tokenRoutes.columns.rowActions')}
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
          {isTogglePending ? <Loader2 className='animate-spin' /> : <Power />}
          {route.enabled
            ? t('tokenRoutes.columns.disable')
            : t('tokenRoutes.columns.enable')}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onClearCooldown(route)}
          disabled={readOnly || isCooldownPending}
        >
          {isCooldownPending ? <Loader2 className='animate-spin' /> : <Snowflake />}
          {t('tokenRoutes.columns.clearCooldown')}
        </DropdownMenuItem>
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
          disabled={readOnly}
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
  pendingCooldownId: number | null = null
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
      accessorFn: (row) => resolveRouteTitle(row as RouteSummaryRow),
      header: t('tokenRoutes.columns.modelAndName'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        const title = resolveRouteTitle(route)
        const modeBadge = resolveModeBadge(route)
        const contextLabel = formatContextLength(route.contextLength)
        const readOnly = isReadOnlyRoute(route)
        return (
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <span
                className='max-w-[240px] truncate font-medium'
                title={title}
              >
                {title}
              </span>
              <Badge variant={modeBadge.variant}>{t(modeBadge.labelKey)}</Badge>
              {readOnly && (
                <Badge variant='outline' className='text-muted-foreground'>
                  {t('tokenRoutes.columns.notGenerated')}
                </Badge>
              )}
            </div>
            <div className='text-muted-foreground flex flex-wrap items-center gap-1.5 text-[11px]'>
              <code className='bg-muted rounded px-1 py-0.5 font-mono text-[11px]'>
                {route.modelPattern}
              </code>
              {contextLabel && (
                <Badge variant='outline' className='text-[10px]'>
                  {contextLabel}
                </Badge>
              )}
              {route.decisionRefreshedAt && (
                <span>
                  {t('tokenRoutes.columns.decisionCachedAt', {
                    time: route.decisionRefreshedAt,
                  })}
                </span>
              )}
            </div>
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'channels',
      accessorFn: (row) => (row as RouteSummaryRow).channelCount,
      header: t('tokenRoutes.columns.channels'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        const summary = resolveChannelSummary(route, t)
        return (
          <div className='flex flex-col'>
            <Badge variant={summary.variant}>
              <span className='tabular-nums'>{summary.label}</span>
            </Badge>
            {summary.hint && (
              <span className='text-muted-foreground text-[11px]'>
                {summary.hint}
              </span>
            )}
          </div>
        )
      },
      meta: { mobileOrder: 2 },
    },
    {
      id: 'strategy',
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
      id: 'sites',
      accessorFn: (row) => (row as RouteSummaryRow).siteNames ?? [],
      header: t('tokenRoutes.columns.sites'),
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        const sites = route.siteNames ?? []
        if (sites.length === 0) {
          return <span className='text-muted-foreground text-xs'>—</span>
        }
        return (
          <div className='flex flex-wrap gap-1'>
            {sites.slice(0, 3).map((site) => (
              <Badge key={site} variant='outline' className='text-[10px]'>
                {site}
              </Badge>
            ))}
            {sites.length > 3 && (
              <Badge variant='outline' className='text-[10px]'>
                +{sites.length - 3}
              </Badge>
            )}
          </div>
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'enabled',
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
            <Badge variant='secondary'>
              <span className='bg-muted-foreground size-1.5 rounded-full' />
              {t('tokenRoutes.columns.notEnabled')}
            </Badge>
          )
        }
        return (
          <Badge variant={route.enabled ? 'success' : 'secondary'}>
            <span
              className={cn(
                'size-1.5 rounded-full',
                route.enabled ? 'bg-success' : 'bg-muted-foreground'
              )}
            />
            {route.enabled
              ? t('tokenRoutes.columns.enable')
              : t('tokenRoutes.columns.disable')}
          </Badge>
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
          />
        )
      },
      meta: { pinned: 'right' },
    },
  ]
}
