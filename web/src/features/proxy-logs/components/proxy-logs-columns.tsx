// metapi-go/features/proxy-logs/components — TanStack Table column definitions.
// i18n: all user-visible strings migrated to t() calls.

import type { ColumnDef } from '@tanstack/react-table'
import {
  Eye as EyeIcon,
  MoreHorizontal as MoreHorizontalIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { toBcp47 } from '@/i18n/languages'
import {
  formatDateTime,
  formatRelativeTime,
  formatShortDate,
  formatTimeOfDay,
} from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ProxyLog } from '../types'
import { StatusBadge } from './status-badge'
import { TimingCell } from './timing-cell'

export type ProxyLogsColumnActions = { onView: (log: ProxyLog) => void }

function formatModelCell(
  modelRequested: string | null | undefined,
  modelActual: string | null | undefined
) {
  const requested = modelRequested?.trim() || '—'
  if (!modelActual || modelActual.trim() === requested) return requested
  return modelActual.trim()
}

export function useProxyLogsColumns(
  actions: ProxyLogsColumnActions
): ColumnDef<ProxyLog>[] {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const columns: ColumnDef<ProxyLog>[] = [
    {
      id: 'createdAt',
      accessorKey: 'createdAt',
      size: 160,
      meta: {
        label: t('proxyLogs.columns.createdAt'),
        mobileTitle: true,
        mobileOrder: 0,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.createdAt')}
        />
      ),
      cell: ({ row }) => {
        const createdAt = row.original.createdAt
        return (
          <div
            className='flex flex-col'
            title={formatDateTime(createdAt, locale)}
          >
            <span className='text-sm tabular-nums'>
              {formatTimeOfDay(createdAt, locale)}
            </span>
            <span className='text-muted-foreground text-[10px]'>
              {formatShortDate(createdAt, locale)} ·{' '}
              {formatRelativeTime(createdAt, locale)}
            </span>
          </div>
        )
      },
    },
    {
      id: 'account',
      accessorFn: (row) => row.username ?? row.accountId ?? null,
      size: 140,
      meta: { label: t('proxyLogs.columns.account'), mobileOrder: 5 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.account')}
        />
      ),
      cell: ({ row }) => {
        const username = row.original.username
        const accountId = row.original.accountId
        return (
          <span className='text-sm'>
            {username || (accountId ? `#${accountId}` : '—')}
          </span>
        )
      },
    },
    {
      id: 'site',
      accessorFn: (row) => row.siteName ?? row.siteId ?? null,
      size: 150,
      meta: {
        label: t('proxyLogs.columns.site'),
        mobileHidden: true,
        mobileOrder: 10,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.site')}
        />
      ),
      cell: ({ row }) => {
        const siteName = row.original.siteName
        return (
          <span className='text-muted-foreground text-sm'>
            {siteName ||
              (row.original.siteId ? `#${row.original.siteId}` : '—')}
          </span>
        )
      },
    },
    {
      id: 'model',
      accessorKey: 'modelRequested',
      size: 180,
      meta: { label: t('proxyLogs.columns.model'), mobileOrder: 2 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.model')}
        />
      ),
      cell: ({ row }) => (
        <span
          className='block max-w-[16rem] truncate text-sm'
          title={row.original.modelActual || row.original.modelRequested}
        >
          {formatModelCell(
            row.original.modelRequested,
            row.original.modelActual
          )}
        </span>
      ),
    },
    {
      id: 'status',
      accessorKey: 'status',
      // 200px: the cell also shows the failure reason (single truncated line),
      // so the col passed on the 110px status-only sizing left ~8 chars visible.
      size: 200,
      meta: {
        label: t('proxyLogs.columns.status'),
        mobileBadge: true,
        mobileOrder: 1,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.status')}
        />
      ),
      cell: ({ row }) => {
        const log = row.original
        return (
          <div className='flex min-w-0 flex-col items-start gap-1'>
            <StatusBadge status={log.status} httpStatus={log.httpStatus} />
            {log.errorMessage ? (
              <span
                className='text-destructive-soft-fg block max-w-[16rem] truncate text-[11px] leading-tight'
                title={log.errorMessage}
              >
                {log.errorMessage}
              </span>
            ) : null}
          </div>
        )
      },
    },
    {
      id: 'latencyMs',
      accessorKey: 'latencyMs',
      size: 120,
      meta: { label: t('proxyLogs.columns.latency'), mobileOrder: 3 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.latency')}
        />
      ),
      cell: ({ row }) => (
        <TimingCell
          latencyMs={row.original.latencyMs}
          firstByteLatencyMs={row.original.firstByteLatencyMs}
        />
      ),
    },
    {
      id: 'token',
      accessorFn: (row) => row.downstreamKeyName ?? row.downstreamKeyId ?? null,
      size: 160,
      meta: {
        label: t('proxyLogs.columns.token'),
        mobileHidden: true,
        mobileOrder: 20,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.token')}
        />
      ),
      cell: ({ row }) => {
        const keyName = row.original.downstreamKeyName
        const keyId = row.original.downstreamKeyId
        const groupName = row.original.downstreamKeyGroupName
        return (
          <div className='flex flex-col gap-0.5'>
            <span className='text-sm'>
              {keyName || (keyId ? `#${keyId}` : '—')}
            </span>
            {groupName && (
              <span className='text-muted-foreground text-[10px]'>
                {groupName}
              </span>
            )}
          </div>
        )
      },
    },
    {
      id: 'retryCount',
      accessorKey: 'retryCount',
      size: 90,
      meta: {
        label: t('proxyLogs.columns.retry'),
        mobileHidden: true,
        mobileOrder: 30,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.retry')}
        />
      ),
      cell: ({ row }) => {
        const retryCount = row.original.retryCount
        if (retryCount === null || retryCount === undefined) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        if (retryCount <= 0) {
          return (
            <span className='text-muted-foreground text-sm tabular-nums'>
              0
            </span>
          )
        }
        return (
          <Badge variant='warning' className='tabular-nums'>
            ×{retryCount}
          </Badge>
        )
      },
    },
    {
      id: 'actions',
      size: 56,
      enableSorting: false,
      enableHiding: false,
      enableResizing: false,
      meta: { mobileHidden: false, mobileOrder: 4 },
      header: () => (
        <span className='text-muted-foreground text-xs'>
          {t('proxyLogs.columns.actions')}
        </span>
      ),
      cell: ({ row }) => {
        const log = row.original
        return (
          <div className={cn('flex justify-end')}>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <Button
                    variant='ghost'
                    size='icon-sm'
                    className='data-popup-open:bg-accent'
                    aria-label={t('proxyLogs.columns.rowActions')}
                  />
                }
              >
                <MoreHorizontalIcon className='size-4' />
              </DropdownMenuTrigger>
              <DropdownMenuContent align='end' className='w-44'>
                <DropdownMenuItem onClick={() => actions.onView(log)}>
                  <EyeIcon className='text-muted-foreground/70 size-3.5' />
                  {t('proxyLogs.columns.viewDetails')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        )
      },
    },
  ]
  return columns
}
