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
import { cn } from '@/lib/utils'

import type { ProxyLog } from '../types'
import { identityColumns } from './proxy-logs-identity-columns'
import { outcomeColumns } from './proxy-logs-outcome-columns'

export type ProxyLogsColumnActions = { onView: (log: ProxyLog) => void }

export function useProxyLogsColumns(
  actions: ProxyLogsColumnActions
): ColumnDef<ProxyLog>[] {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const columns: ColumnDef<ProxyLog>[] = [
    ...identityColumns(t, locale),
    ...outcomeColumns(t, locale),
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
              <span className='text-muted-foreground text-3xs'>
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
      meta: {
        mobileHidden: false,
        mobileOrder: 4,
        mobileActionsInHeader: true,
      },
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
