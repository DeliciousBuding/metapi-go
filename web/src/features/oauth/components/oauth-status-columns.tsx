import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'
import { TriangleAlert as TriangleAlertIcon } from 'lucide-react'

import { DataTableColumnHeader } from '@/components/data-table'
import { formatDateTime, formatInt } from '@/lib/format'

import type { OAuthClient, OAuthClientStatus } from '../types'
import { QuotaCell } from './oauth-quota-cell'
import { OAuthStatusBadge } from './oauth-status-badge'

export function oauthStatusColumns(
  t: TFunction,
  locale: string
): ColumnDef<OAuthClient>[] {
  return [
    {
      id: 'status',
      accessorKey: 'status',
      size: 100,
      meta: { mobileBadge: true, mobileOrder: 2 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.status')}
        />
      ),
      cell: ({ row }) => <OAuthStatusBadge status={row.original.status} />,
      filterFn: (row, columnId, filterValue) => {
        const value = row.getValue(columnId) as OAuthClientStatus | undefined
        const resolved: OAuthClientStatus =
          value === 'abnormal' ? 'abnormal' : 'healthy'
        if (Array.isArray(filterValue)) {
          return filterValue.length === 0 || filterValue.includes(resolved)
        }
        return String(filterValue) === resolved
      },
    },
    {
      id: 'planType',
      accessorFn: (row) =>
        row.planType ?? row.quota?.subscription?.planType ?? '',
      size: 85,
      meta: { mobileHidden: true, mobileOrder: 21 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.planType')}
        />
      ),
      cell: ({ row }) => {
        const planType =
          row.original.planType ??
          row.original.quota?.subscription?.planType ??
          null
        return (
          <span className='text-muted-foreground text-sm'>
            {planType ?? '—'}
          </span>
        )
      },
    },
    {
      id: 'quota',
      enableSorting: false,
      size: 132,
      meta: { mobileHidden: true, mobileOrder: 22 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.quota')}
        />
      ),
      cell: ({ row }) => <QuotaCell client={row.original} />,
    },
    {
      id: 'modelCount',
      accessorKey: 'modelCount',
      size: 85,
      meta: { mobileOrder: 20 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.modelCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {formatInt(row.original.modelCount)}
        </span>
      ),
    },
    {
      id: 'routeChannelCount',
      accessorFn: (row) => row.routeChannelCount ?? -1,
      size: 150,
      meta: { mobileHidden: true, mobileOrder: 23 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.routeChannelCount')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm tabular-nums'>
          {row.original.routeChannelCount ?? '—'}
        </span>
      ),
    },
    {
      id: 'lastModelSyncAt',
      accessorKey: 'lastModelSyncAt',
      size: 135,
      meta: { mobileHidden: true, mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.lastSync')}
        />
      ),
      cell: ({ row }) => (
        <span
          className='text-muted-foreground block max-w-28 truncate text-sm tabular-nums'
          title={formatDateTime(row.original.lastModelSyncAt, locale)}
        >
          {formatDateTime(row.original.lastModelSyncAt, locale)}
        </span>
      ),
    },
    {
      id: 'lastModelSyncError',
      enableSorting: false,
      size: 135,
      meta: { mobileHidden: true, mobileOrder: 31 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('oauth.columns.lastModelSyncError')}
        />
      ),
      cell: ({ row }) => {
        const syncError = row.original.lastModelSyncError?.trim() || null
        if (!syncError) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span
            className='flex max-w-28 items-center gap-1.5 text-sm'
            title={syncError}
          >
            <TriangleAlertIcon
              className='text-warning size-4 shrink-0'
              aria-label={t('oauth.columns.lastModelSyncError')}
            />
            <span className='text-muted-foreground truncate'>{syncError}</span>
          </span>
        )
      },
    },
  ]
}
