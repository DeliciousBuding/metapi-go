import { Link } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction, i18n } from 'i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { toBcp47 } from '@/i18n/languages'
import {
  formatAbsoluteDateTime,
  formatCurrency,
  formatRelativeTime,
} from '@/lib/format'

import { resolveSiteBalanceUsd } from '../lib/site-balance'
import type { Site } from '../types'

export function operationalColumns(
  t: TFunction,
  i18n: i18n
): ColumnDef<Site>[] {
  return [
    {
      id: 'accountCount',
      accessorKey: 'accountCount',
      size: 120,
      meta: { label: t('sites.columns.accountCount'), mobileOrder: 30 },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.accountCount')}
        />
      ),
      cell: ({ row }) => {
        const count = row.original.accountCount
        const activeFromSubscription =
          row.original.subscriptionSummary?.activeCount
        let resolved: number | null = null
        if (typeof count === 'number') {
          resolved = count
        } else if (typeof activeFromSubscription === 'number') {
          resolved = activeFromSubscription
        }
        return resolved === null ? (
          <span className='text-muted-foreground'>—</span>
        ) : (
          <Link
            to='/accounts'
            search={{ site: row.original.id }}
            className='text-primary hover:bg-accent border-border/80 bg-muted/30 inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium tabular-nums transition-colors'
            aria-label={t('sites.page.viewAccounts', {
              name: row.original.name,
              count: resolved,
            })}
          >
            {resolved}
          </Link>
        )
      },
    },
    {
      id: 'balance',
      accessorFn: (row) => resolveSiteBalanceUsd(row),
      size: 140,
      meta: {
        label: t('sites.columns.balance'),
        mobileOrder: 35,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.balance')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-sm font-medium tabular-nums'>
          {formatCurrency(resolveSiteBalanceUsd(row.original))}
        </span>
      ),
    },
    {
      id: 'globalWeight',
      accessorKey: 'globalWeight',
      size: 120,
      meta: {
        label: t('sites.columns.globalWeight'),
        mobileHideWhen: (site) => (site.globalWeight ?? 1) === 1,
        mobileOrder: 40,
      },
      header: ({ column }) => (
        <span title={t('sites.page.weightFormula')}>
          <DataTableColumnHeader
            column={column}
            title={t('sites.columns.globalWeight')}
          />
        </span>
      ),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-sm tabular-nums'>
          {row.original.globalWeight ?? 1}
        </span>
      ),
    },
    {
      id: 'useSystemProxy',
      accessorKey: 'useSystemProxy',
      size: 130,
      meta: {
        label: t('sites.columns.useSystemProxy'),
        mobileHidden: true,
        mobileOrder: 45,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.useSystemProxy')}
        />
      ),
      cell: ({ row }) => {
        const enabled = row.original.useSystemProxy
        return (
          <Badge variant={enabled ? 'success' : 'secondary'}>
            {enabled ? t('sites.detail.yes') : t('sites.detail.no')}
          </Badge>
        )
      },
    },
    {
      id: 'externalCheckinUrl',
      accessorKey: 'externalCheckinUrl',
      size: 240,
      meta: {
        label: t('sites.columns.externalCheckinUrl'),
        mobileHidden: true,
        mobileOrder: 46,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.externalCheckinUrl')}
        />
      ),
      cell: ({ row }) => {
        const url = row.original.externalCheckinUrl
        if (!url) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span className='block max-w-56 truncate text-sm' title={url}>
            {url}
          </span>
        )
      },
    },
    {
      id: 'createdAt',
      accessorKey: 'createdAt',
      size: 170,
      meta: {
        label: t('sites.columns.createdAt'),
        mobileHidden: true,
        mobileOrder: 47,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('sites.columns.createdAt')}
        />
      ),
      cell: ({ row }) => {
        const createdAt = row.original.createdAt
        if (!createdAt) {
          return <span className='text-muted-foreground text-sm'>—</span>
        }
        return (
          <span
            className='text-muted-foreground text-sm whitespace-nowrap'
            title={formatAbsoluteDateTime(
              createdAt,
              toBcp47(i18n.language || 'en')
            )}
          >
            {formatRelativeTime(createdAt, toBcp47(i18n.language || 'en'))}
          </span>
        )
      },
    },
  ]
}
