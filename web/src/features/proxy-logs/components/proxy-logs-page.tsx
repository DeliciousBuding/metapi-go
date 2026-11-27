import { useNavigate } from '@tanstack/react-router'
import type { Row } from '@tanstack/react-table'
import { ScrollText } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import {
  DataTablePage,
  DataTableRow,
  type DataTableRenderRowHelpers,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { formatCurrency, formatInt } from '@/lib/format'

import { useProxyLogs, useProxyLogsMeta } from '../api'
import {
  handleProxyLogRowKeyDown,
  isInteractiveRowTarget,
} from '../lib/row-interaction'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import type { ProxyLog } from '../types'
import { LogViewNavigation } from './log-view-navigation'
import { ProxyLogDetailSheet } from './proxy-log-detail-sheet'
import { ProxyLogsAutoRefreshToggle } from './proxy-logs-auto-refresh-toggle'
import {
  useProxyLogsColumns,
  type ProxyLogsColumnActions,
} from './proxy-logs-columns'
import {
  ProxyLogsAdvancedFilters,
  ProxyLogsQuickFilters,
} from './proxy-logs-filters'
import { ProxyLogsHeaderActions } from './proxy-logs-header-actions'
import { SummaryStat, SummaryStrip } from './summary-strip'
import { useProxyLogsExport } from './use-proxy-logs-export'
import {
  useProxyLogsUrlState,
  type ProxyLogsUrlFilters,
} from './use-proxy-logs-url-state'

const PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:proxy-logs:column-visibility'
const PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY =
  'metapi-go:proxy-logs:column-sizing'

export function ProxyLogsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const {
    globalFilter,
    pagination,
    onGlobalFilterChange,
    onPaginationChange,
    filters,
    updateUrlState,
    metaPayload,
    queryPayload,
  } = useProxyLogsUrlState()
  const [detailLog, setDetailLog] = useState<ProxyLog | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const { isExporting, handleExportCsv } = useProxyLogsExport(queryPayload)
  const { intervalMs, setIntervalMs } = useProxyLogsAutoRefresh()
  const logsQuery = useProxyLogs(queryPayload, {
    refetchInterval: intervalMs === false ? false : intervalMs,
  })
  // The meta query is the single owner of the summary aggregate (the list
  // fetch is view=query and carries no summary) — refetch it on the same
  // interval so auto-refresh keeps the summary strip as fresh as the rows.
  const metaQuery = useProxyLogsMeta(metaPayload, {
    refetchInterval: intervalMs === false ? false : intervalMs,
  })
  const items = useMemo(() => logsQuery.data?.items ?? [], [logsQuery.data])
  const downstreamKeyId = metaPayload.downstreamKeyId
  // Display name for the active key-filter chip: resolved from the loaded
  // rows (every row carries downstreamKeyName); falls back to the bare id so
  // a zero-result page still shows an honest, dismissible chip.
  const downstreamKeyLabel = useMemo(() => {
    if (downstreamKeyId === undefined) return null
    const hit = items.find((item) => item.downstreamKeyId === downstreamKeyId)
    return hit?.downstreamKeyName || `#${downstreamKeyId}`
  }, [downstreamKeyId, items])
  const total = logsQuery.data?.total ?? 0

  // Memoized so the column defs keep a stable identity across renders.
  const columnActions = useMemo<ProxyLogsColumnActions>(
    () => ({
      onView: (log) => {
        setDetailLog(log)
        setDetailOpen(true)
      },
    }),
    []
  )
  const columns = useProxyLogsColumns(columnActions)

  // Desktop row click opens the detail sheet directly — the failure reason is
  // now readable in the row itself, and one click (instead of the two-step
  // ⋯ → View details) reaches the full detail. Rows are keyboard-accessible
  // (focusable; Enter/Space opens the sheet). Clicks originating inside
  // nested interactive elements (the row ⋯ dropdown trigger, links, form
  // controls) bubble to the row — skip them so a nested control is never
  // swallowed by the row-level handler.
  const renderRow = (
    row: Row<ProxyLog>,
    helpers: DataTableRenderRowHelpers
  ) => (
    <DataTableRow
      row={row}
      className='cursor-pointer'
      tabIndex={0}
      onClick={(event) => {
        if (isInteractiveRowTarget(event.target)) return
        columnActions.onView(row.original)
      }}
      onKeyDown={(event) =>
        handleProxyLogRowKeyDown(event, () =>
          columnActions.onView(row.original)
        )
      }
      getColumnClassName={(columnId) => helpers.getCellClassName(columnId)}
    />
  )

  const { table } = useDataTable<ProxyLog>({
    initialColumnVisibility: {
      account: false,
      client: false,
      token: false,
      retryCount: false,
      isStream: false,
    },
    data: items,
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    enableColumnResizing: true,
    columnVisibilityStorageKey: PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY,
    // The URL-synced callbacks already reset the page on every filter change
    // (resetPageIndexOnFilterChange), so disable TanStack's own auto-reset to
    // avoid a second redundant page update.
    autoResetPageIndex: false,
    globalFilter,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
    getRowId: (row) => String(row.id),
    totalCount: total,
  })

  const summary = metaQuery.data?.summary
  const {
    status,
    siteId,
    channelId,
    client,
    from,
    to,
    latencyMin,
    latencyMax,
  } = filters
  const hasFacetFilters = Boolean(siteId || channelId || client || from || to)
  function updateFilters(next: Partial<ProxyLogsUrlFilters>) {
    updateUrlState({ filters: next, pageIndex: 0 })
  }

  function handleReset() {
    updateUrlState({
      q: '',
      pageIndex: 0,
      filters: {
        status: 'all',
        siteId: '',
        channelId: '',
        downstreamKeyId: '',
        client: '',
        from: '',
        to: '',
        latencyMin: '',
        latencyMax: '',
      },
    })
  }

  const detailIndex = detailLog
    ? items.findIndex((item) => item.id === detailLog.id)
    : -1
  const updatedAt = Math.max(
    logsQuery.dataUpdatedAt ?? 0,
    metaQuery.dataUpdatedAt ?? 0
  )
  const hasRefreshError = Boolean(logsQuery.isError || metaQuery.isError)

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t('requestLogs.title')}
        actions={
          <>
            <QuietRefreshStatus
              updatedAt={updatedAt}
              hasError={hasRefreshError}
              className='hidden sm:inline'
            />
            <ProxyLogsAutoRefreshToggle
              intervalMs={intervalMs}
              setIntervalMs={setIntervalMs}
            />
            <ProxyLogsHeaderActions
              onExport={handleExportCsv}
              isExporting={isExporting}
              onRefresh={() => {
                // Refresh both halves of the split fetch: the list (view=query)
                // and the meta facets/summary (view=meta).
                logsQuery.refetch()
                metaQuery.refetch()
              }}
              isRefreshing={logsQuery.isFetching || metaQuery.isFetching}
            />
          </>
        }
      >
        <LogViewNavigation view='attempts' />
      </PageHeader>

      {summary && (
        <SummaryStrip>
          <SummaryStat
            label={t('proxyLogs.page.summaryTotal')}
            value={formatInt(summary.totalCount)}
          />
          {/* Semantic tint only when the count is non-zero: an all-zero
              summary is a healthy/neutral state, not a red/green alarm. */}
          <SummaryStat
            label={t('proxyLogs.page.summarySuccess')}
            value={formatInt(summary.successCount)}
            tone={summary.successCount > 0 ? 'success' : undefined}
          />
          <SummaryStat
            label={t('proxyLogs.page.summaryFailed')}
            value={formatInt(summary.failedCount)}
            tone={summary.failedCount > 0 ? 'danger' : undefined}
          />
          <SummaryStat
            label={t('proxyLogs.page.summaryCost')}
            value={formatCurrency(summary.totalCost, { fractionDigits: 4 })}
          />
        </SummaryStrip>
      )}

      {/* The meta query owns the summary strip; surface its failure so the
          missing strip reads as an error, not as "no data yet". */}
      <QueryErrorBanner
        error={metaQuery.error as Error | null}
        messageKey='proxyLogs.page.metaLoadError'
        onRetry={() => metaQuery.refetch()}
        isRetrying={metaQuery.isFetching}
      />

      <DataTablePage
        table={table}
        renderRow={renderRow}
        isLoading={logsQuery.isLoading}
        isFetching={logsQuery.isFetching}
        error={logsQuery.error as Error | null}
        errorMessageKey='proxyLogs.page.loadError'
        onErrorRetry={() => logsQuery.refetch()}
        isErrorRetrying={logsQuery.isFetching}
        errorPlacement='inline'
        emptyIcon={<ScrollText className='size-6' />}
        emptyTitle={t('proxyLogs.page.emptyTitle')}
        emptyDescription={t('proxyLogs.page.emptyDescription')}
        emptyAction={
          <Button
            variant='outline'
            onClick={() => void navigate({ to: '/token-routes' })}
          >
            {t('proxyLogs.page.emptyAction')}
          </Button>
        }
        skeletonKeyPrefix='proxy-log-skeleton'
        toolbarProps={{
          searchPlaceholder: t('proxyLogs.page.searchPlaceholder'),
          searchDebounceMs: 400,
          additionalSearch: (
            <ProxyLogsQuickFilters
              status={status}
              downstreamKeyLabel={downstreamKeyLabel}
              onChange={updateFilters}
            />
          ),
          expandable: (
            <ProxyLogsAdvancedFilters
              filters={filters}
              meta={metaQuery.data}
              onChange={updateFilters}
            />
          ),
          hasExpandedActiveFilters:
            hasFacetFilters || Boolean(latencyMin || latencyMax),
          hasAdditionalFilters:
            hasFacetFilters ||
            downstreamKeyId !== undefined ||
            status !== 'all',
          onReset: handleReset,
        }}
      />

      <ProxyLogDetailSheet
        log={detailLog}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        hasPrev={detailIndex > 0}
        hasNext={detailIndex >= 0 && detailIndex < items.length - 1}
        onNavigatePrev={() => {
          if (detailIndex > 0) setDetailLog(items[detailIndex - 1])
        }}
        onNavigateNext={() => {
          if (detailIndex >= 0 && detailIndex < items.length - 1) {
            setDetailLog(items[detailIndex + 1])
          }
        }}
        onViewAttempt={(attempt) =>
          setDetailLog((prev) =>
            prev
              ? {
                  ...prev,
                  id: attempt.id,
                  createdAt: attempt.createdAt,
                  status: attempt.status,
                  httpStatus: attempt.httpStatus,
                  latencyMs: attempt.latencyMs,
                  retryCount: attempt.ordinal,
                  modelActual: attempt.modelActual,
                  errorMessage: attempt.errorMessage || null,
                }
              : prev
          )
        }
      />
    </div>
  )
}
