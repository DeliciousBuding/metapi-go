import type { ColumnDef, Row } from '@tanstack/react-table'
import { Eye } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import {
  DataTableRow,
  type DataTableRenderRowHelpers,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import type { GatewayRequest } from '@/lib/api'
import { formatCurrency, formatDateTime, formatLatency } from '@/lib/format'

import {
  handleProxyLogRowKeyDown,
  isInteractiveRowTarget,
} from '../lib/row-interaction'
import type { UpdateGatewaySearch } from './gateway-request-search'
import { RequestOutcome } from './request-outcome'
import { UsageCell } from './usage-cell'

export function useGatewayRequestColumns(
  kind: 'usage' | 'access',
  locale: string,
  updateUrlState: UpdateGatewaySearch
) {
  const { t } = useTranslation()
  return useMemo<ColumnDef<GatewayRequest>[]>(
    () => [
      {
        id: 'startedAt',
        accessorKey: 'startedAt',
        size: 165,
        header: t('requestLogs.startedAt'),
        meta: { label: t('requestLogs.startedAt'), mobileOrder: 3 },
        cell: ({ row }) => (
          <span className='text-muted-foreground text-xs tabular-nums'>
            {formatDateTime(row.original.startedAt, locale)}
          </span>
        ),
      },
      {
        id: 'model',
        accessorKey: 'modelRequested',
        size: 190,
        header: t(
          kind === 'access' ? 'requestLogs.interface' : 'requestLogs.model'
        ),
        meta: {
          label: t(
            kind === 'access' ? 'requestLogs.interface' : 'requestLogs.model'
          ),
          mobileTitle: true,
          mobileOrder: 0,
        },
        cell: ({ row }) => {
          let content
          if (kind === 'access') {
            content = (
              <span className='truncate font-mono text-xs'>
                {row.original.downstreamPath ?? t('requestLogs.unknown')}
              </span>
            )
          } else if (row.original.modelRequested) {
            content = <ModelPill model={row.original.modelRequested} />
          } else {
            content = <span>{t('requestLogs.unknownModel')}</span>
          }
          return <div className='flex min-w-0 items-center'>{content}</div>
        },
      },
      {
        id: 'result',
        accessorKey: 'terminalStatus',
        size: 170,
        header: t('requestLogs.result'),
        meta: {
          label: t('requestLogs.result'),
          mobileFullWidth: true,
          mobileOrder: 1,
        },
        cell: ({ row }) => (
          <RequestOutcome request={row.original} showTransport={false} />
        ),
      },
      {
        id: 'latencyMs',
        accessorKey: 'latencyMs',
        size: 120,
        header: t('requestLogs.duration'),
        meta: { label: t('requestLogs.duration'), mobileOrder: 2 },
        cell: ({ row }) => (
          <span className='text-xs tabular-nums'>
            {formatLatency(row.original.latencyMs, {
              autoSeconds: true,
              spaced: true,
              secondsDigits: 2,
            })}
          </span>
        ),
      },
      ...(kind === 'usage'
        ? [
            {
              id: 'usage',
              size: 145,
              header: t('proxyLogs.columns.usage'),
              meta: { label: t('proxyLogs.columns.usage'), mobileOrder: 4 },
              cell: ({ row }: { row: { original: GatewayRequest } }) => (
                <div className='flex items-center gap-1'>
                  <UsageCell log={row.original} />
                  {row.original.usagePartial && (
                    <span
                      className='text-muted-foreground text-xs'
                      title={t('requestLogs.partialUsage')}
                      aria-label={t('requestLogs.partialUsage')}
                    >
                      *
                    </span>
                  )}
                </div>
              ),
            },
            {
              id: 'cost',
              size: 110,
              header: t('requestLogs.estimatedCost'),
              meta: { label: t('requestLogs.estimatedCost'), mobileOrder: 5 },
              cell: ({ row }: { row: { original: GatewayRequest } }) => (
                <span className='text-xs tabular-nums'>
                  {row.original.estimatedCost == null
                    ? '—'
                    : formatCurrency(row.original.estimatedCost, {
                        fractionDigits: 6,
                      })}
                </span>
              ),
            },
          ]
        : []),
      {
        id: 'key',
        size: 135,
        header: t('requestLogs.key'),
        meta: { label: t('requestLogs.key'), mobileOrder: 6 },
        cell: ({ row }) => (
          <span className='truncate text-xs'>
            {row.original.downstreamKeyName ||
              (row.original.downstreamApiKeyId
                ? `#${row.original.downstreamApiKeyId}`
                : '—')}
          </span>
        ),
      },
      {
        id: 'actions',
        size: 56,
        enableHiding: false,
        meta: { mobileActionsInHeader: true },
        cell: ({ row }) => (
          <Button
            size='icon-sm'
            variant='ghost'
            aria-label={t('requestLogs.openDetail')}
            onClick={() =>
              updateUrlState({ filters: { requestId: row.original.id } })
            }
          >
            <Eye className='size-4' />
          </Button>
        ),
      },
    ],
    [t, locale, updateUrlState, kind]
  )
}

export function GatewayRequestRow({
  row,
  helpers,
  updateUrlState,
}: {
  row: Row<GatewayRequest>
  helpers: DataTableRenderRowHelpers
  updateUrlState: UpdateGatewaySearch
}) {
  return (
    <DataTableRow
      row={row}
      className='cursor-pointer'
      tabIndex={0}
      getColumnClassName={(id) => helpers.getCellClassName(id)}
      onClick={(event) => {
        if (!isInteractiveRowTarget(event.target))
          updateUrlState({ filters: { requestId: row.original.id } })
      }}
      onKeyDown={(event) =>
        handleProxyLogRowKeyDown(event, () =>
          updateUrlState({ filters: { requestId: row.original.id } })
        )
      }
    />
  )
}
