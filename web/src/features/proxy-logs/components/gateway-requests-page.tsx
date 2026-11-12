import { Link, useLocation } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { Eye, RefreshCw, ScrollText, X } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { DateRangePresets } from '@/components/common/date-range-presets'
import { ModelPill } from '@/components/common/model-pill'
import { PageHeader } from '@/components/common/page-header'
import {
  DataTablePage,
  DataTableRow,
  type UrlTableState,
  type UrlTableStateUpdate,
  useDataTable,
  useUrlTableState,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toBcp47 } from '@/i18n/languages'
import type { GatewayRequest, GatewayRequestStatus } from '@/lib/api'
import { gatewayRequestStatusSchema } from '@/lib/api/gateway-requests'
import {
  formatCurrency,
  formatDateTime,
  formatInt,
  formatLatency,
} from '@/lib/format'
import { asStringParam } from '@/lib/helpers/searchParams'

import { useGatewayRequests } from '../lib/gateway-requests'
import { proxyLogsSearchSchema } from '../lib/proxy-logs-schema'
import {
  handleProxyLogRowKeyDown,
  isInteractiveRowTarget,
} from '../lib/row-interaction'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import { GatewayRequestDetailSheet } from './gateway-request-detail-sheet'
import { LogViewNavigation } from './log-view-navigation'
import { ProxyLogsAutoRefreshToggle } from './proxy-logs-auto-refresh-toggle'
import { ProxyLogsPage } from './proxy-logs-page'
import { RequestOutcome } from './request-outcome'
import { SummaryStat, SummaryStrip } from './summary-strip'
import { UsageCell } from './usage-cell'

type RequestFilters = {
  status: string
  downstreamKeyId: string
  from: string
  to: string
  requestId: string
}

export function readGatewayRequestsSearch(
  search: string
): UrlTableState<RequestFilters> {
  const params = new URLSearchParams(search)
  const parsed = proxyLogsSearchSchema.parse(Object.fromEntries(params))
  return {
    q: asStringParam(parsed.q) ?? '',
    pageIndex: parsed.page,
    pageSize: parsed.pageSize,
    sorting: [],
    filters: {
      status: parsed.requestStatus,
      downstreamKeyId:
        parsed.downstreamKeyId === undefined
          ? ''
          : String(parsed.downstreamKeyId),
      from: asStringParam(parsed.from) ?? '',
      to: asStringParam(parsed.to) ?? '',
      requestId: asStringParam(parsed.requestId) ?? '',
    },
  }
}

export function buildGatewayRequestsHref(
  next: UrlTableStateUpdate<RequestFilters>,
  currentSearch?: string,
  basePath = '/proxy-logs'
): string {
  const current = readGatewayRequestsSearch(
    currentSearch ?? window.location.search
  )
  const merged = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  if (merged.filters.status !== 'all')
    params.set('requestStatus', merged.filters.status)
  for (const key of ['downstreamKeyId', 'from', 'to', 'requestId'] as const) {
    if (merged.filters[key]) params.set(key, merged.filters[key])
  }
  if (basePath === '/audit-logs') params.set('category', 'access')
  return `${basePath}${params.size ? `?${params}` : ''}`
}

export function ProxyLogsWorkspace() {
  const location = useLocation()
  const params = new URLSearchParams(location.searchStr)
  // Existing site/channel/client/latency deep links only have attempt-level
  // meaning. Keep those links honest until their producers opt into requests.
  const attempts =
    params.get('view') === 'attempts' ||
    ['siteId', 'channelId', 'client', 'latencyMin', 'latencyMax'].some((key) =>
      params.has(key)
    ) ||
    (params.has('status') && !params.has('requestStatus'))
  return attempts ? <ProxyLogsPage /> : <GatewayRequestsPage />
}

export function GatewayRequestsPage({
  kind = 'usage',
  navigation,
}: {
  kind?: 'usage' | 'access'
  navigation?: ReactNode
} = {}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language)
  const basePath = kind === 'access' ? '/audit-logs' : '/proxy-logs'
  const state = useUrlTableState<RequestFilters>({
    basePath,
    read: readGatewayRequestsSearch,
    buildHref: (next, search) =>
      buildGatewayRequestsHref(next, search, basePath),
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })
  const { filters, pagination, updateUrlState } = state
  const { intervalMs, setIntervalMs } = useProxyLogsAutoRefresh()
  const params = useMemo(
    () => ({
      kind,
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      status:
        filters.status === 'all'
          ? undefined
          : (filters.status as GatewayRequestStatus),
      search: state.globalFilter.trim() || undefined,
      downstreamKeyId: filters.downstreamKeyId
        ? Number(filters.downstreamKeyId)
        : undefined,
      from: filters.from || undefined,
      to: filters.to || undefined,
    }),
    [
      kind,
      pagination,
      filters.status,
      filters.downstreamKeyId,
      filters.from,
      filters.to,
      state.globalFilter,
    ]
  )
  const query = useGatewayRequests(params, intervalMs)
  const columns = useMemo<ColumnDef<GatewayRequest>[]>(
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
        cell: ({ row }) => (
          <div className='flex min-w-0 items-center'>
            {kind === 'access' ? (
              <span className='truncate font-mono text-xs'>
                {row.original.downstreamPath ?? t('requestLogs.unknown')}
              </span>
            ) : row.original.modelRequested ? (
              <ModelPill model={row.original.modelRequested} />
            ) : (
              <span>{t('requestLogs.unknownModel')}</span>
            )}
          </div>
        ),
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
  const { table } = useDataTable({
    data: query.data?.items ?? [],
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    autoResetPageIndex: false,
    globalFilter: state.globalFilter,
    onGlobalFilterChange: state.onGlobalFilterChange,
    pagination,
    onPaginationChange: state.onPaginationChange,
    getRowId: (row) => row.id,
    totalCount: query.data?.total ?? 0,
  })
  const summary = query.data?.summary
  const reset = () =>
    updateUrlState({
      q: '',
      pageIndex: 0,
      filters: { status: 'all', downstreamKeyId: '', from: '', to: '' },
    })
  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t(kind === 'access' ? 'auditLogs.title' : 'requestLogs.title')}
        actions={
          <>
            <ProxyLogsAutoRefreshToggle
              intervalMs={intervalMs}
              setIntervalMs={setIntervalMs}
            />
            <Button
              size='sm'
              variant='outline'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              <RefreshCw className='size-3.5' />
              {t('proxyLogs.page.refresh')}
            </Button>
          </>
        }
      >
        {navigation ?? <LogViewNavigation view='requests' />}
      </PageHeader>
      {summary && (
        <SummaryStrip>
          {(
            [
              'totalCount',
              'completedCount',
              'failedCount',
              'cancelledCount',
              'rejectedCount',
            ] as const
          ).map((key) => (
            <SummaryStat
              key={key}
              label={t(`requestLogs.summary.${key}`)}
              value={formatInt(summary[key])}
            />
          ))}
          {query.data && (
            <span className='text-muted-foreground text-2xs ms-auto'>
              {query.data.coverageStartedAt
                ? t(
                    kind === 'access'
                      ? 'auditLogs.coverage'
                      : 'requestLogs.coverage',
                    {
                      time: formatDateTime(
                        query.data.coverageStartedAt,
                        locale
                      ),
                    }
                  )
                : t('requestLogs.noCoverage')}
            </span>
          )}
        </SummaryStrip>
      )}
      <DataTablePage
        table={table}
        isLoading={query.isLoading}
        isFetching={query.isFetching}
        error={query.error}
        errorMessageKey='requestLogs.loadError'
        onErrorRetry={() => void query.refetch()}
        isErrorRetrying={query.isFetching}
        emptyIcon={<ScrollText className='size-6' />}
        emptyTitle={t(
          kind === 'access' ? 'auditLogs.emptyAccess' : 'requestLogs.emptyTitle'
        )}
        emptyDescription={t(
          kind === 'access'
            ? 'auditLogs.accessDescription'
            : 'requestLogs.emptyDescription'
        )}
        emptyAction={
          kind === 'usage' ? (
            <Link
              to='/token-routes'
              className='text-primary text-sm hover:underline'
            >
              {t('proxyLogs.page.emptyAction')}
            </Link>
          ) : undefined
        }
        renderRow={(row, helpers) => (
          <DataTableRow
            key={row.id}
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
        )}
        toolbarProps={{
          searchPlaceholder: t(
            kind === 'access'
              ? 'auditLogs.searchPlaceholder'
              : 'requestLogs.searchPlaceholder'
          ),
          searchDebounceMs: 400,
          additionalSearch: (
            <>
              <Select
                value={filters.status}
                onValueChange={(value) =>
                  updateUrlState({
                    pageIndex: 0,
                    filters: { status: value ?? 'all' },
                  })
                }
              >
                <SelectTrigger
                  size='sm'
                  className='w-44'
                  aria-label={t('requestLogs.filterStatus')}
                >
                  <SelectValue>
                    {(value) =>
                      t(
                        value === 'all'
                          ? 'requestLogs.all'
                          : `requestLogs.status.${value}`
                      )
                    }
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {['all', ...gatewayRequestStatusSchema.options].map(
                    (status) => (
                      <SelectItem value={status} key={status}>
                        {t(
                          status === 'all'
                            ? 'requestLogs.all'
                            : `requestLogs.status.${status}`
                        )}
                      </SelectItem>
                    )
                  )}
                </SelectContent>
              </Select>
              {filters.downstreamKeyId && (
                <Button
                  variant='secondary'
                  size='sm'
                  aria-label={t('proxyLogs.page.filterKeyClear')}
                  onClick={() =>
                    updateUrlState({
                      pageIndex: 0,
                      filters: { downstreamKeyId: '' },
                    })
                  }
                >
                  {t('requestLogs.key')} #{filters.downstreamKeyId}
                  <X className='size-3.5' />
                </Button>
              )}
            </>
          ),
          expandable: (
            <div className='flex w-full flex-wrap items-center gap-2'>
              <Input
                type='datetime-local'
                className='w-50 max-sm:w-full'
                aria-label={t('proxyLogs.page.startTime')}
                value={filters.from}
                onChange={(event) =>
                  updateUrlState({
                    pageIndex: 0,
                    filters: { from: event.target.value },
                  })
                }
              />
              <Input
                type='datetime-local'
                className='w-50 max-sm:w-full'
                aria-label={t('proxyLogs.page.endTime')}
                value={filters.to}
                onChange={(event) =>
                  updateUrlState({
                    pageIndex: 0,
                    filters: { to: event.target.value },
                  })
                }
              />
              <DateRangePresets
                onApply={(from, to) =>
                  updateUrlState({ pageIndex: 0, filters: { from, to } })
                }
              />
            </div>
          ),
          hasExpandedActiveFilters: !!filters.from || !!filters.to,
          hasAdditionalFilters:
            filters.status !== 'all' ||
            !!filters.downstreamKeyId ||
            !!filters.from ||
            !!filters.to,
          onReset: reset,
        }}
      />
      <GatewayRequestDetailSheet
        id={filters.requestId || null}
        onClose={() => updateUrlState({ filters: { requestId: '' } })}
      />
    </div>
  )
}
