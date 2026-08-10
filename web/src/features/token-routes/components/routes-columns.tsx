// metapi-go features/token-routes/components — TanStack Table column
// definitions for the routes list. Column meta drives the mobile card layout
// (mobileTitle / mobileBadge / mobileHidden / mobileOrder) so the data-table
// package's automatic mobile degradation needs zero per-feature card code.
//
// Zero-channel placeholder rows (kind: 'zero_channel' / readOnly) are rendered
// with a muted "未生成" + "0 通道" badge and have their row actions disabled
// (no edit/delete/toggle) — mirroring the legacy RouteCard readOnly treatment.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye,
  MoreHorizontal,
  Pencil,
  Power,
  RefreshCw,
  Snowflake,
  Trash2,
} from 'lucide-react'

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

import {
  type RouteRowActions,
  type RouteRoutingStrategy,
  type RouteSummaryRow,
} from '../types'
import {
  formatContextLength,
  isExplicitGroupRoute,
  resolveRouteTitle,
  routingStrategyLabel,
} from '../utils'

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function isReadOnlyRoute(route: RouteSummaryRow): boolean {
  return route.kind === 'zero_channel' || route.readOnly === true
}

function resolveModeBadge(route: RouteSummaryRow): {
  label: string
  variant: 'default' | 'secondary' | 'outline'
} {
  if (isExplicitGroupRoute(route)) {
    return { label: '分组', variant: 'secondary' }
  }
  return { label: '匹配', variant: 'outline' }
}

function resolveChannelSummary(route: RouteSummaryRow): {
  label: string
  variant: 'default' | 'warning' | 'secondary'
  hint?: string
} {
  if (isReadOnlyRoute(route)) {
    return { label: '0 通道', variant: 'warning', hint: '未生成' }
  }
  const total = route.channelCount ?? 0
  const enabled = route.enabledChannelCount ?? 0
  if (total === 0) {
    return { label: '0 通道', variant: 'warning', hint: '需补齐通道' }
  }
  if (enabled === 0) {
    return {
      label: `${total} 通道`,
      variant: 'secondary',
      hint: '全部禁用',
    }
  }
  return {
    label: `${enabled}/${total}`,
    variant: 'default',
    hint: enabled < total ? `${total - enabled} 个禁用` : undefined,
  }
}

// ---------------------------------------------------------------------------
// Row actions cell
// ---------------------------------------------------------------------------

function RoutesRowActions({
  route,
  actions,
}: {
  route: RouteSummaryRow
  actions: RouteRowActions
}) {
  const readOnly = isReadOnlyRoute(route)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className='inline-flex size-8 items-center justify-center rounded-md text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground'
        aria-label='路由操作'
      >
        <MoreHorizontal className='size-4' />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' sideOffset={4}>
        <DropdownMenuItem onClick={() => actions.onViewDetail(route)}>
          <Eye />
          查看详情
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onToggleEnabled(route)}
          disabled={readOnly}
        >
          <Power />
          {route.enabled ? '禁用' : '启用'}
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onClearCooldown(route)}
          disabled={readOnly}
        >
          <Snowflake />
          清除冷却
        </DropdownMenuItem>
        <DropdownMenuItem
          onClick={() => actions.onRefreshDecision(route)}
          disabled={readOnly}
        >
          <RefreshCw />
          刷新决策
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onClick={() => actions.onEdit(route)}
          disabled={readOnly}
        >
          <Pencil />
          编辑
        </DropdownMenuItem>
        <DropdownMenuItem
          variant='destructive'
          onClick={() => actions.onDelete(route)}
          disabled={readOnly}
        >
          <Trash2 />
          删除
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---------------------------------------------------------------------------
// Columns hook
// ---------------------------------------------------------------------------

export function useRoutesColumns(
  actions: RouteRowActions,
): ColumnDef<RouteSummaryRow>[] {
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
          aria-label='全选'
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
            aria-label='选择此行'
          />
        )
      },
      meta: { mobileHidden: true },
    },
    {
      id: 'title',
      header: '模型 / 名称',
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        const title = resolveRouteTitle(route)
        const modeBadge = resolveModeBadge(route)
        const contextLabel = formatContextLength(route.contextLength)
        const readOnly = isReadOnlyRoute(route)
        return (
          <div className='flex flex-col gap-1'>
            <div className='flex items-center gap-2'>
              <span className='font-medium truncate max-w-[240px]' title={title}>
                {title}
              </span>
              <Badge variant={modeBadge.variant}>{modeBadge.label}</Badge>
              {readOnly && (
                <Badge variant='outline' className='text-muted-foreground'>
                  未生成
                </Badge>
              )}
            </div>
            <div className='flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground'>
              <code className='rounded bg-muted px-1 py-0.5 font-mono text-[11px]'>
                {route.modelPattern}
              </code>
              {contextLabel && (
                <Badge variant='outline' className='text-[10px]'>
                  {contextLabel}
                </Badge>
              )}
              {route.decisionRefreshedAt && (
                <span>决策缓存于 {route.decisionRefreshedAt}</span>
              )}
            </div>
          </div>
        )
      },
      meta: { mobileTitle: true },
    },
    {
      id: 'channels',
      header: '通道',
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        const summary = resolveChannelSummary(route)
        return (
          <div className='flex flex-col'>
            <Badge variant={summary.variant}>
              <span className='tabular-nums'>{summary.label}</span>
            </Badge>
            {summary.hint && (
              <span className='text-[11px] text-muted-foreground'>
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
      header: '策略',
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) {
          return <span className='text-muted-foreground text-xs'>—</span>
        }
        const label = routingStrategyLabel(route.routingStrategy as RouteRoutingStrategy | null)
        return <Badge variant='outline'>{label}</Badge>
      },
      meta: { mobileOrder: 3 },
    },
    {
      id: 'sites',
      header: '站点',
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
      header: '状态',
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        if (isReadOnlyRoute(route)) {
          return (
            <Badge variant='secondary'>
              <span className='size-1.5 rounded-full bg-muted-foreground' />
              未启用
            </Badge>
          )
        }
        return (
          <Badge variant={route.enabled ? 'default' : 'secondary'}>
            <span
              className={cn(
                'size-1.5 rounded-full',
                route.enabled ? 'bg-emerald-500' : 'bg-muted-foreground',
              )}
            />
            {route.enabled ? '启用' : '禁用'}
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
      header: () => <span className='sr-only'>操作</span>,
      cell: ({ row }) => {
        const route = row.original as RouteSummaryRow
        return <RoutesRowActions route={route} actions={actions} />
      },
      meta: { pinned: 'right' },
    },
  ]
}
