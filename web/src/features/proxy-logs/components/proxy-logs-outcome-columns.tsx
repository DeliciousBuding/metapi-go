import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { ExpandableText } from '@/components/common/expandable-text'
import { HttpStatusBadge } from '@/components/common/http-status-badge'
import { DataTableColumnHeader } from '@/components/data-table'
import { formatCurrency } from '@/lib/format'

import type { ProxyLog } from '../types'
import { TimingCell } from './timing-cell'
import { UsageCell } from './usage-cell'

export function outcomeColumns(
  t: TFunction,
  locale: string
): ColumnDef<ProxyLog>[] {
  return [
    {
      id: 'status',
      accessorKey: 'status',
      // 200px: the cell also shows the failure reason (single truncated line),
      // so the col passed on the 110px status-only sizing left ~8 chars visible.
      size: 200,
      meta: {
        label: t('proxyLogs.columns.status'),
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
            <HttpStatusBadge status={log.status} httpStatus={log.httpStatus} />
            {log.errorMessage ? (
              <ExpandableText
                text={log.errorMessage}
                threshold={72}
                className='text-destructive-soft-fg max-w-64 text-xs leading-4'
              />
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
          firstOutputLatencyMs={row.original.firstOutputLatencyMs}
          isStream={row.original.isStream}
        />
      ),
    },
    {
      id: 'usage',
      size: 130,
      meta: { label: t('proxyLogs.columns.usage'), mobileOrder: 4 },
      header: () => t('proxyLogs.columns.usage'),
      cell: ({ row }) => <UsageCell log={row.original} />,
    },
    {
      id: 'estimatedCost',
      accessorKey: 'estimatedCost',
      size: 125,
      // The cost is short enough to sit in the card header beside the timestamp,
      // which keeps the body to two field rows instead of an orphan third row.
      meta: {
        label: t('proxyLogs.columns.cost'),
        mobileBadge: true,
        mobileOrder: 6,
      },
      header: ({ column }) => (
        <DataTableColumnHeader
          column={column}
          title={t('proxyLogs.columns.cost')}
        />
      ),
      cell: ({ row }) => (
        <span className='text-xs tabular-nums'>
          {formatCurrency(row.original.estimatedCost, {
            fractionDigits: 6,
            locale,
          })}
        </span>
      ),
    },
    {
      id: 'isStream',
      accessorKey: 'isStream',
      size: 90,
      meta: {
        label: t('proxyLogs.columns.mode'),
        mobileHidden: true,
        mobileOrder: 8,
      },
      header: () => t('proxyLogs.columns.mode'),
      cell: ({ row }) => (
        <span className='text-muted-foreground text-xs'>
          {row.original.isStream == null
            ? '—'
            : t(
                row.original.isStream
                  ? 'proxyLogs.usage.stream'
                  : 'proxyLogs.usage.nonStream'
              )}
        </span>
      ),
    },
  ]
}
