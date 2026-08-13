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
import { cn } from '@/lib/utils'

import type { ProxyLog } from '../types'
import { TimingCell } from './timing-cell'
import { StatusBadge } from './status-badge'

export type ProxyLogsColumnActions = { onView: (log: ProxyLog) => void }

function parseServerDate(value: string | null | undefined): Date | null {
  if (!value || typeof value !== 'string') return null
  const normalized = value.endsWith('Z') ? value : `${value}Z`
  const parsed = new Date(normalized)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

function pad2(value: number): string {
  return value < 10 ? `0${value}` : String(value)
}

function formatTime(value: string | null | undefined): string {
  const date = parseServerDate(value)
  if (!date) return '—'
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`
}

function useFormatRelativeShort() {
  const { t } = useTranslation()
  return function formatRelativeShort(
    value: string | null | undefined
  ): string {
    const date = parseServerDate(value)
    if (!date) return ''
    const deltaMs = Date.now() - date.getTime()
    const abs = Math.abs(deltaMs)
    const suffix =
      deltaMs >= 0
        ? t('proxyLogs.columns.relativeBefore')
        : t('proxyLogs.columns.relativeAfter')
    if (abs < 60_000) {
      return `${Math.max(1, Math.round(abs / 1000))} ${t('proxyLogs.columns.relativeSeconds')}${suffix}`
    }
    if (abs < 3_600_000) {
      return `${Math.round(abs / 60_000)} ${t('proxyLogs.columns.relativeMinutes')}${suffix}`
    }
    if (abs < 86_400_000) {
      return `${Math.round(abs / 3_600_000)} ${t('proxyLogs.columns.relativeHours')}${suffix}`
    }
    if (abs < 2_592_000_000) {
      return `${Math.round(abs / 86_400_000)} ${t('proxyLogs.columns.relativeDays')}${suffix}`
    }
    return `${pad2(date.getMonth() + 1)}-${pad2(date.getDate())}`
  }
}

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
  const { t } = useTranslation()
  const formatRelativeShort = useFormatRelativeShort()
  const columns: ColumnDef<ProxyLog>[] = [
    {
      id: 'createdAt',
      accessorKey: 'createdAt',
      size: 160,
      meta: { mobileTitle: true, mobileOrder: 0 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.createdAt')}
        />
      ),
      cell: ({ row }) => {
        const createdAt = row.original.createdAt
        return (
          <div className='flex flex-col'>
            <span className='text-sm tabular-nums'>
              {formatTime(createdAt)}
            </span>
            <span className='text-muted-foreground text-[10px]'>
              {formatRelativeShort(createdAt)}
            </span>
          </div>
        )
      },
    },
    {
      id: 'account',
      accessorFn: (row) => row.username ?? row.accountId ?? null,
      size: 140,
      meta: { mobileOrder: 5 },
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
      meta: { mobileHidden: true, mobileOrder: 10 },
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
      meta: { mobileOrder: 2 },
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
      size: 110,
      meta: { mobileBadge: true, mobileOrder: 1 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.status')}
        />
      ),
      cell: ({ row }) => (
        <StatusBadge status={row.original.status} httpStatus={null} />
      ),
    },
    {
      id: 'latencyMs',
      accessorKey: 'latencyMs',
      size: 120,
      meta: { mobileOrder: 3 },
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
      meta: { mobileHidden: true, mobileOrder: 20 },
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
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.retry')}
        />
      ),
      cell: ({ row }) => {
        const retryCount = row.original.retryCount
        if (!retryCount || retryCount <= 0) {
          return <span className='text-muted-foreground text-sm'>—</span>
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
