import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { ClientPill } from '@/components/common/client-pill'
import { ModelPill } from '@/components/common/model-pill'
import { DataTableColumnHeader } from '@/components/data-table'
import {
  formatDateTime,
  formatLogDateDetail,
  formatTimeOfDay,
} from '@/lib/format'

import type { ProxyLog } from '../types'

function formatModelCell(
  modelRequested: string | null | undefined,
  modelActual: string | null | undefined
) {
  const requested = modelRequested?.trim() || '—'
  if (!modelActual || modelActual.trim() === requested) return requested
  return modelActual.trim()
}

export function identityColumns(
  t: TFunction,
  locale: string
): ColumnDef<ProxyLog>[] {
  return [
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
            <span className='text-muted-foreground text-3xs'>
              {formatLogDateDetail(createdAt, locale)}
            </span>
          </div>
        )
      },
    },
    {
      id: 'account',
      accessorFn: (row) => row.username ?? row.accountId ?? null,
      size: 140,
      meta: {
        label: t('proxyLogs.columns.account'),
        mobileOrder: 5,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.account')}
        />
      ),
      cell: ({ row }) => {
        const username = row.original.username
        const accountId = row.original.accountId
        if (row.original.upstreamGrantId) {
          return (
            <span className='text-sm'>
              {t('proxyLogs.columns.directGrant', {
                channelId: row.original.upstreamChannelId ?? '—',
                grantId: row.original.upstreamGrantId,
              })}
            </span>
          )
        }
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
        <ModelPill
          model={formatModelCell(
            row.original.modelRequested,
            row.original.modelActual
          )}
          title={row.original.modelActual || row.original.modelRequested}
        />
      ),
    },
    {
      id: 'client',
      // No accessorKey: the cell composes appName→family from the row (the
      // backend detects these from the UA upstream), so there is nothing to
      // sort/filter by locally — the client filter hits the server.
      size: 150,
      meta: {
        label: t('proxyLogs.columns.client'),
        mobileHidden: true,
        mobileOrder: 21,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.client')}
        />
      ),
      cell: ({ row }) => (
        <ClientPill
          appName={row.original.clientAppName}
          family={row.original.clientFamily}
        />
      ),
    },
  ]
}
