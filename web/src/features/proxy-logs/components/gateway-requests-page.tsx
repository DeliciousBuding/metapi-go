import { Link, useLocation } from '@tanstack/react-router'
import { RefreshCw, ScrollText } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import {
  DataTablePage,
  useDataTable,
  useUrlTableState,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'
import { toBcp47 } from '@/i18n/languages'
import type { GatewayRequestStatus } from '@/lib/api'
import { formatDateTime, formatInt } from '@/lib/format'

import { useGatewayRequests } from '../lib/gateway-requests'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import {
  useGatewayRequestColumns,
  GatewayRequestRow,
} from './gateway-request-columns'
import { GatewayRequestDetailSheet } from './gateway-request-detail-sheet'
import { useGatewayRequestToolbar } from './gateway-request-filters'
import {
  buildGatewayRequestsHref,
  readGatewayRequestsSearch,
  type RequestFilters,
} from './gateway-request-search'
import { LogViewNavigation } from './log-view-navigation'
import { ProxyLogsAutoRefreshToggle } from './proxy-logs-auto-refresh-toggle'
import { ProxyLogsPage } from './proxy-logs-page'
import { SummaryStat, SummaryStrip } from './summary-strip'

export {
  buildGatewayRequestsHref,
  readGatewayRequestsSearch,
} from './gateway-request-search'

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
  const showRefreshSpin = useDelayedSpin(query.isFetching)
  const columns = useGatewayRequestColumns(kind, locale, updateUrlState)
  const toolbar = useGatewayRequestToolbar(kind, filters, updateUrlState)
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
  const items = query.data?.items ?? []
  const selectedIndex = filters.requestId
    ? items.findIndex((item) => item.id === filters.requestId)
    : -1
  const summary = query.data?.summary
  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t(kind === 'access' ? 'auditLogs.title' : 'requestLogs.title')}
        actions={
          <>
            <QuietRefreshStatus
              updatedAt={query.dataUpdatedAt}
              hasError={query.isError}
              className='hidden sm:inline'
            />
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
              <RefreshCw
                className={
                  showRefreshSpin ? 'size-3.5 animate-spin' : 'size-3.5'
                }
              />
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
          <GatewayRequestRow
            key={row.id}
            row={row}
            helpers={helpers}
            updateUrlState={updateUrlState}
          />
        )}
        toolbarProps={toolbar}
      />
      <GatewayRequestDetailSheet
        id={filters.requestId || null}
        onClose={() => updateUrlState({ filters: { requestId: '' } })}
        hasPrev={selectedIndex > 0}
        hasNext={selectedIndex >= 0 && selectedIndex < items.length - 1}
        onNavigatePrev={() => {
          if (selectedIndex > 0) {
            updateUrlState({
              filters: { requestId: items[selectedIndex - 1].id },
            })
          }
        }}
        onNavigateNext={() => {
          if (selectedIndex >= 0 && selectedIndex < items.length - 1) {
            updateUrlState({
              filters: { requestId: items[selectedIndex + 1].id },
            })
          }
        }}
      />
    </div>
  )
}
