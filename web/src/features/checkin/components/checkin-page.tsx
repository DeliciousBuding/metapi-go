// metapi-go features/checkin/components — the checkin log list page.
// i18n: all user-visible strings migrated to t() calls.
//
// Server-side pagination + filtering (mirrors /api/stats/proxy-logs): the
// table runs in manualPagination/manualFiltering/manualSorting mode and the
// backend returns the real total, so date-range / status / reason / site /
// search filters see the full log history instead of the legacy 500-row
// client cap that silently dropped older records.
//
// URL state uses the shared useUrlTableState hook (same as sites/oauth/
// models/accounts): the URL is the single source of truth and every control
// navigates in one transaction (filter + page reset together), so there is no
// local `useState` mirror + `useEffect` write-back that can feed back into an
// infinite render loop.

import { useNavigate } from '@tanstack/react-router'
import type { ColumnFiltersState } from '@tanstack/react-table'
import axios from 'axios'
import { CalendarRange, RotateCw, Users, Zap } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  DataTablePage,
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
import { Spinner } from '@/components/ui/spinner'
import { useAccounts } from '@/features/accounts'
import { useSites } from '@/features/sites/api'
import { asStringParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import { useCheckinAccount, useCheckinLogs, useManualCheckin } from '../api'
import {
  DEFAULT_CHECKIN_PAGE_SIZE,
  parseCheckinSearch,
  parseFilterValues,
} from '../lib/checkin-schema'
import { localDatetimeInputToUtcRfc3339 } from '../lib/checkin-time'
import { type CheckinLogRow, checkinLogRowSchema } from '../types'
import { useCheckinColumns } from './checkin-columns'
import { CheckinDetailSheet } from './checkin-detail-sheet'
import { ManualCheckinDialog } from './manual-checkin-dialog'

const ACCOUNT_FILTER_ALL = 'all'

/** Page-specific URL filters: comma-separated lists + date range + account. */
type CheckinUrlFilters = {
  status: string
  reason: string
  site: string
  accountId: string
  from: string
  to: string
}

/**
 * Parse the raw search string into URL table state. Reuses the route's
 * `parseCheckinSearch` (same schema the loader uses) so the page's derived
 * query payload exactly matches the prefetched cache key — no double fetch.
 */
function readCheckinSearch(
  searchString: string
): UrlTableState<CheckinUrlFilters> {
  const search = parseCheckinSearch(searchString)
  return {
    q: asStringParam(search.q) ?? '',
    pageIndex: search.page - 1,
    pageSize: search.pageSize,
    sorting: [],
    filters: {
      status: asStringParam(search.status) ?? '',
      reason: asStringParam(search.reason) ?? '',
      site: asStringParam(search.site) ?? '',
      accountId: search.accountId === undefined ? '' : String(search.accountId),
      from: asStringParam(search.from) ?? '',
      to: asStringParam(search.to) ?? '',
    },
  }
}

/** Serialize a partial state update back to the checkin href, merging over
 *  the CURRENT url state so a single filter change preserves all others. */
function buildCheckinHref(
  next: UrlTableStateUpdate<CheckinUrlFilters>
): string {
  const current = readCheckinSearch(window.location.search)
  const merged: UrlTableState<CheckinUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_CHECKIN_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.status) params.set('status', merged.filters.status)
  if (merged.filters.reason) params.set('reason', merged.filters.reason)
  if (merged.filters.site) params.set('site', merged.filters.site)
  if (merged.filters.accountId) {
    params.set('accountId', merged.filters.accountId)
  }
  if (merged.filters.from) params.set('from', merged.filters.from)
  if (merged.filters.to) params.set('to', merged.filters.to)
  const queryString = params.toString()
  return queryString ? `/checkin?${queryString}` : '/checkin'
}

function useCheckinUrlState() {
  return useUrlTableState<CheckinUrlFilters>({
    basePath: '/checkin',
    read: readCheckinSearch,
    buildHref: buildCheckinHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const statusValues = parseFilterValues(filters.status)
      const reasonValues = parseFilterValues(filters.reason)
      const siteValues = parseFilterValues(filters.site)
      if (statusValues.length) out.push({ id: 'status', value: statusValues })
      if (reasonValues.length) out.push({ id: 'reason', value: reasonValues })
      if (siteValues.length) out.push({ id: 'site', value: siteValues })
      return out
    },
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      const reasonEntry = filters.find((filter) => filter.id === 'reason')
      const siteEntry = filters.find((filter) => filter.id === 'site')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? statusEntry.value.join(',')
            : '',
          reason: Array.isArray(reasonEntry?.value)
            ? reasonEntry.value.join(',')
            : '',
          site: Array.isArray(siteEntry?.value)
            ? siteEntry.value.join(',')
            : '',
        },
      }
    },
    resetPageIndexOnFilterChange: true,
  })
}

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

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function CheckinPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
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

  const { data: accountsSnapshot } = useAccounts()
  const accountOptions = accountsSnapshot?.accounts ?? []
  const { data: sitesData } = useSites()
  const siteOptions = useMemo(
    () =>
      (sitesData ?? []).map((site) => ({
        value: site.name,
        label: site.name,
      })),
    [sitesData]
  )

  const triggerAllMutation = useManualCheckin()
  const triggerOneMutation = useCheckinAccount()
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRow, setDetailRow] = useState<CheckinLogRow | null>(null)
  const [manualOpen, setManualOpen] = useState(false)

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

  const {
    data: logsPage,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useCheckinLogs(queryPayload)
  const logs = useMemo(() => logsPage?.items ?? [], [logsPage])
  const total = logsPage?.total ?? 0

  const handleTriggerOne = useCallback(
    async (row: CheckinLogRow) => {
      const targetAccountId = row.checkin_logs.accountId
      try {
        const result = await triggerOneMutation.mutateAsync(targetAccountId)
        if (result.status === 'success') {
          toast.success(t('checkin.toast.success'), {
            description: result.reward
              ? t('checkin.toast.successReward', { reward: result.reward })
              : undefined,
          })
        } else if (result.status === 'skipped' || result.skipped) {
          toast.info(t('checkin.toast.skipped'), {
            description: result.message || undefined,
          })
        } else {
          toast.error(t('checkin.toast.failed'), {
            description: result.message || undefined,
          })
        }
      } catch {
        // http-client toasted
      }
    },
    [triggerOneMutation, t]
  )

  // Memoized so the column defs keep a stable identity across renders.
  const rowActions = useMemo(
    () => ({
      onViewDetail: (row: CheckinLogRow) => {
        setDetailRow(row)
        setDetailOpen(true)
      },
      onTriggerAccount: handleTriggerOne,
    }),
    [handleTriggerOne]
  )

  // Per-row pending state (accounts-columns' pendingStatusId pattern): only
  // the row whose single-account check-in is in flight shows a spinner, so
  // the trigger stays a seconds-long external request without a global lock.
  const pendingCheckinAccountId = triggerOneMutation.isPending
    ? (triggerOneMutation.variables ?? null)
    : null

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

  const handleTriggerAll = async () => {
    try {
      const result = await triggerAllMutation.mutateAsync()
      const summary = result.summary
      if (summary) {
        const desc = t('checkin.toast.summary', {
          total: summary.total,
          success: summary.success,
          failed: summary.failed,
          skipped: summary.skipped,
        })
        if (summary.failed > 0) {
          toast.error(t('checkin.toast.partialFailed'), { description: desc })
        } else {
          toast.success(t('checkin.toast.complete'), { description: desc })
        }
      } else {
        toast.success(result.message || t('checkin.toast.complete'))
      }
    } catch (error) {
      // Transport failures (non-2xx) are already toasted by the http-client
      // error interceptor; envelope failures thrown by the parser are not, so
      // they get their own honest error toast instead of being swallowed.
      if (!axios.isAxiosError(error)) {
        toast.error(
          error instanceof Error && error.message
            ? error.message
            : t('checkin.toast.triggerFailed')
        )
      }
    }
  }

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

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal'>{t('checkin.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('checkin.page.description')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <Button
            variant='outline'
            onClick={() => setManualOpen(true)}
            disabled={accountOptions.length === 0}
          >
            <Zap />
            {t('checkin.page.manualCheckin')}
          </Button>
          <Button
            onClick={handleTriggerAll}
            disabled={triggerAllMutation.isPending}
          >
            {triggerAllMutation.isPending ? <Spinner /> : <RotateCw />}
            {t('checkin.page.runAll')}
          </Button>
        </div>
      </div>

      <QueryErrorBanner
        error={error as Error | null}
        messageKey='checkin.page.loadError'
        onRetry={() => refetch()}
        isRetrying={isFetching}
      />

      <div className='flex flex-wrap items-center gap-2'>
        <CalendarRange className='text-muted-foreground size-4' />
        <Input
          type='datetime-local'
          value={from}
          onChange={(event) => {
            updateUrlState({
              filters: { from: event.target.value },
              pageIndex: 0,
            })
          }}
          className='w-[200px]'
          aria-label={t('checkin.page.startTime')}
        />
        <span className='text-muted-foreground text-sm'>
          {t('checkin.page.to')}
        </span>
        <Input
          type='datetime-local'
          value={to}
          onChange={(event) => {
            updateUrlState({
              filters: { to: event.target.value },
              pageIndex: 0,
            })
          }}
          className='w-[200px]'
          aria-label={t('checkin.page.endTime')}
        />
        <Select
          value={accountId ? String(accountId) : ACCOUNT_FILTER_ALL}
          onValueChange={(value) => {
            updateUrlState({
              filters: {
                accountId: !value || value === ACCOUNT_FILTER_ALL ? '' : value,
              },
              pageIndex: 0,
            })
          }}
        >
          <SelectTrigger
            aria-label={t('checkin.page.filterAccountTitle')}
            className='w-[200px]'
          >
            <SelectValue>
              {(selected) => {
                if (!selected || selected === ACCOUNT_FILTER_ALL) {
                  return t('checkin.page.allAccounts')
                }
                const account = accountOptions.find(
                  (item) => String(item.id) === selected
                )
                return account
                  ? account.username || `#${account.id}`
                  : String(selected)
              }}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ACCOUNT_FILTER_ALL}>
              {t('checkin.page.allAccounts')}
            </SelectItem>
            {accountOptions.map((account) => (
              <SelectItem key={account.id} value={String(account.id)}>
                {account.username || `#${account.id}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {hasActiveFilters && (
          <Button variant='ghost' size='sm' onClick={handleResetFilters}>
            {t('checkin.page.resetFilters')}
          </Button>
        )}
      </div>

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('checkin.page.emptyTitle')}
        emptyDescription={t('checkin.page.emptyDescription')}
        emptyAction={
          // With accounts present the CTA reuses the header's manual
          // check-in flow (same dialog, same mutation) to generate logs;
          // without accounts there is nothing to check in, so the exit
          // points to the accounts page instead of a disabled button.
          accountOptions.length > 0 ? (
            <Button onClick={() => setManualOpen(true)}>
              <Zap className='size-4' />
              {t('checkin.page.manualCheckin')}
            </Button>
          ) : (
            <Button
              variant='outline'
              onClick={() => void navigate({ to: '/accounts' })}
            >
              <Users className='size-4' />
              {t('checkin.page.manageAccounts')}
            </Button>
          )
        }
        skeletonKeyPrefix='checkin-skeleton'
        toolbarProps={{
          searchPlaceholder: t('checkin.page.searchPlaceholder'),
          searchDebounceMs: 300,
          filters: [
            {
              columnId: 'status',
              title: t('checkin.page.filterStatusTitle'),
              singleSelect: true,
              options: [
                {
                  label: t('checkin.page.filterStatusSuccess'),
                  value: 'success',
                },
                {
                  label: t('checkin.page.filterStatusFailed'),
                  value: 'failed',
                },
                {
                  label: t('checkin.page.filterStatusSkipped'),
                  value: 'skipped',
                },
              ],
            },
            {
              columnId: 'reason',
              title: t('checkin.page.filterReasonTitle'),
              options: [
                {
                  label: t('checkin.page.filterReasonVerification'),
                  value: 'verification',
                },
                { label: t('checkin.page.filterReasonAuth'), value: 'auth' },
                {
                  label: t('checkin.page.filterReasonNetwork'),
                  value: 'network',
                },
                { label: t('checkin.page.filterReasonSite'), value: 'site' },
                { label: t('checkin.page.filterReasonState'), value: 'state' },
                {
                  label: t('checkin.page.filterReasonUnknown'),
                  value: 'unknown',
                },
              ],
            },
            ...(siteOptions.length > 0
              ? [
                  {
                    columnId: 'site',
                    title: t('checkin.page.filterSiteTitle'),
                    options: siteOptions,
                  },
                ]
              : []),
          ],
          onReset: handleResetFilters,
        }}
      />

      <CheckinDetailSheet
        row={detailRow}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />
      <ManualCheckinDialog open={manualOpen} onOpenChange={setManualOpen} />
    </div>
  )
}
