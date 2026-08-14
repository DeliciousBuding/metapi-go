// metapi-go features/checkin/components — the checkin log list page.
// i18n: all user-visible strings migrated to t() calls.
//
// Server-side pagination + filtering (mirrors /api/stats/proxy-logs): the
// table runs in manualPagination/manualFiltering/manualSorting mode and the
// backend returns the real total, so date-range / status / reason / site /
// search filters see the full log history instead of the legacy 500-row
// client cap that silently dropped older records.

import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import { CalendarRange, Loader2, RotateCw, Zap } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useAccounts } from '@/features/accounts'
import { useSites } from '@/features/sites/api'
import { toast } from '@/lib/toast'

import { useCheckinAccount, useCheckinLogs, useManualCheckin } from '../api'
import {
  buildCheckinSearchString,
  parseFilterValues,
  readCheckinSearchFromUrl,
} from '../lib/checkin-schema'
import { localDatetimeInputToUtcRfc3339 } from '../lib/checkin-time'
import { type CheckinLogRow, checkinLogRowSchema } from '../types'
import { useCheckinColumns } from './checkin-columns'
import { CheckinDetailSheet } from './checkin-detail-sheet'
import { ManualCheckinDialog } from './manual-checkin-dialog'

const ACCOUNT_FILTER_ALL = 'all'

export function CheckinPage() {
  const { t } = useTranslation()
  const initial = useMemo(readCheckinSearchFromUrl, [])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.page - 1,
    pageSize: initial.pageSize,
  })
  const [globalFilter, setGlobalFilter] = useState(initial.q ?? '')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const filters: ColumnFiltersState = []
    const statusValues = parseFilterValues(initial.status)
    if (statusValues.length) filters.push({ id: 'status', value: statusValues })
    const reasonValues = parseFilterValues(initial.reason)
    if (reasonValues.length) filters.push({ id: 'reason', value: reasonValues })
    const siteValues = parseFilterValues(initial.site)
    if (siteValues.length) filters.push({ id: 'site', value: siteValues })
    return filters
  })
  const [accountId, setAccountId] = useState<number | undefined>(
    initial.accountId
  )
  const [from, setFrom] = useState(initial.from ?? '')
  const [to, setTo] = useState(initial.to ?? '')

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

  // Resolve the active server-side filter values from the column-filter
  // state (the toolbar faceted filters drive these). status is single-select,
  // so only the first value is forwarded to the backend.
  const statusValues = useMemo(() => {
    const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    const value = statusFilter?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])
  const reasonValues = useMemo(() => {
    const reasonFilter = columnFilters.find((filter) => filter.id === 'reason')
    const value = reasonFilter?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])
  const siteValues = useMemo(() => {
    const siteFilter = columnFilters.find((filter) => filter.id === 'site')
    const value = siteFilter?.value
    return Array.isArray(value) ? (value as string[]) : []
  }, [columnFilters])

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
  } = useCheckinLogs(queryPayload)
  const logs = useMemo(() => logsPage?.items ?? [], [logsPage])
  const total = logsPage?.total ?? 0

  useEffect(() => {
    if (typeof window === 'undefined') return
    const query = buildCheckinSearchString({
      pageIndex: pagination.pageIndex,
      pageSize: pagination.pageSize,
      accountId,
      statusValues,
      reasonValues,
      siteValues,
      from: from || undefined,
      to: to || undefined,
      query: globalFilter || undefined,
    })
    window.history.replaceState(null, '', `${window.location.pathname}${query}`)
  }, [
    pagination,
    accountId,
    columnFilters,
    from,
    to,
    globalFilter,
    statusValues,
    reasonValues,
    siteValues,
  ])

  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setGlobalFilter((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    []
  )
  const onColumnFiltersChange = useMemo<OnChangeFn<ColumnFiltersState>>(
    () => (updater) => {
      setColumnFilters((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    []
  )
  const onPaginationChange = useMemo<OnChangeFn<PaginationState>>(
    () => (updater) => {
      setPagination((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
    },
    []
  )

  const handleTriggerOne = async (row: CheckinLogRow) => {
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
    } catch {}
  }

  const rowActions = {
    onViewDetail: (row: CheckinLogRow) => {
      setDetailRow(row)
      setDetailOpen(true)
    },
    onTriggerAccount: handleTriggerOne,
  }

  const columns = useCheckinColumns(rowActions)
  const { table } = useDataTable<CheckinLogRow>({
    data: logs,
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    enableRowSelection: false,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    globalFilterFn: (row, _columnId, filterValue) => {
      // With manualFiltering the table does not run this; kept so the column
      // definition stays valid if the table ever flips back to client mode.
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
    },
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
    } catch {}
  }

  const handleResetFilters = () => {
    setFrom('')
    setTo('')
    setAccountId(undefined)
    setColumnFilters([])
    setGlobalFilter('')
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  const hasActiveDateRange = from !== '' || to !== ''
  const hasActiveFilters =
    columnFilters.length > 0 || hasActiveDateRange || accountId !== undefined

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-center justify-between'>
        <div>
          <h1 className='text-lg font-normal'>{t('checkin.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('checkin.page.description')}
          </p>
        </div>
        <div className='flex items-center gap-2'>
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
            {triggerAllMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <RotateCw />
            )}
            {t('checkin.page.runAll')}
          </Button>
        </div>
      </div>

      {error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
          {t('checkin.page.loadError', { message: (error as Error).message })}
        </div>
      )}

      <div className='flex flex-wrap items-center gap-2'>
        <CalendarRange className='text-muted-foreground size-4' />
        <Input
          type='datetime-local'
          value={from}
          onChange={(event) => {
            setFrom(event.target.value)
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
            setTo(event.target.value)
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
          }}
          className='w-[200px]'
          aria-label={t('checkin.page.endTime')}
        />
        <Select
          value={accountId ? String(accountId) : ACCOUNT_FILTER_ALL}
          onValueChange={(value) => {
            setAccountId(
              !value || value === ACCOUNT_FILTER_ALL ? undefined : Number(value)
            )
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
