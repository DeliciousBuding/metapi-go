import { useMemo } from 'react'

import { useDataTable } from '@/components/data-table'

import { useCheckinLogs } from '../api'
import { parseFilterValues } from '../lib/checkin-schema'
import { localDatetimeInputToUtcRfc3339 } from '../lib/checkin-time'
import {
  type CheckinLogRow,
  checkinLogRowSchema,
  type CheckinRowActions,
} from '../types'
import { useCheckinColumns } from './checkin-columns'
import { useCheckinUrlState } from './use-checkin-url-state'

// Module-level so the table's globalFilterFn keeps a stable identity across
// renders (a fresh inline function would re-resolve the table every render).
function checkinGlobalFilterFn(
  row: { original: unknown },
  _columnId: string,
  filterValue: string
): boolean {
  const log = checkinLogRowSchema.parse(row.original)
  const haystack = [
    log.accounts?.username ?? '',
    log.sites?.name ?? '',
    log.sites?.url ?? '',
    log.checkin_logs.message ?? '',
    log.checkin_logs.reward ?? '',
  ]
    .join(' ')
    .toLowerCase()
  return haystack.includes(String(filterValue).toLowerCase())
}

export function useCheckinLogTable(
  rowActions: CheckinRowActions,
  pendingCheckinAccountId: number | null
) {
  const {
    globalFilter,
    pagination,
    columnFilters,
    onGlobalFilterChange,
    onPaginationChange,
    onColumnFiltersChange,
    filters,
    updateUrlState,
  } = useCheckinUrlState()
  // Derive the active server-side filter values directly from the URL-owned
  // filters (single source of truth — no local mirror to sync back).
  const accountId = filters.accountId ? Number(filters.accountId) : undefined
  const from = filters.from
  const to = filters.to
  const statusValues = useMemo(
    () => parseFilterValues(filters.status),
    [filters.status]
  )
  const reasonValues = useMemo(
    () => parseFilterValues(filters.reason),
    [filters.reason]
  )
  const siteValues = useMemo(
    () => parseFilterValues(filters.site),
    [filters.site]
  )

  const fromUtc = useMemo(
    () => localDatetimeInputToUtcRfc3339(from, false),
    [from]
  )
  const toUtc = useMemo(() => localDatetimeInputToUtcRfc3339(to, true), [to])

  const queryPayload = useMemo(
    () => ({
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      accountId,
      status: statusValues[0],
      reason: reasonValues,
      site: siteValues,
      from: fromUtc,
      to: toUtc,
      search: globalFilter.trim() || undefined,
    }),
    [
      pagination,
      accountId,
      statusValues,
      reasonValues,
      siteValues,
      fromUtc,
      toUtc,
      globalFilter,
    ]
  )

  const query = useCheckinLogs(queryPayload)
  const logsPage = query.data
  const logs = useMemo(() => logsPage?.items ?? [], [logsPage])
  const total = logsPage?.total ?? 0
  const columns = useCheckinColumns(rowActions, pendingCheckinAccountId)
  const { table } = useDataTable<CheckinLogRow>({
    data: logs,
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    enableRowSelection: false,
    // The URL-synced callbacks already reset the page on every filter change
    // (resetPageIndexOnFilterChange), so disable TanStack's own auto-reset to
    // avoid a second redundant page update.
    autoResetPageIndex: false,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    globalFilterFn: checkinGlobalFilterFn,
    totalCount: total,
  })
  const handleResetFilters = () => {
    updateUrlState({
      q: '',
      pageIndex: 0,
      filters: {
        status: '',
        reason: '',
        site: '',
        accountId: '',
        from: '',
        to: '',
      },
    })
  }

  const hasActiveDateRange = from !== '' || to !== ''
  const hasActiveFilters =
    columnFilters.length > 0 || hasActiveDateRange || accountId !== undefined
  return {
    table,
    query,
    filters: {
      accountId,
      from,
      to,
      hasActiveFilters,
      updateUrlState,
      handleResetFilters,
    },
  }
}
