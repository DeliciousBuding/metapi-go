// metapi-go features/checkin/components — the checkin log list page.
//
// Wires the data-table four-layer package (useDataTable + DataTablePage) to
// the useCheckinLogs query. Server-side params are limited to `limit` +
// `accountId`; all other filtering (date range, status, failure-reason
// category, site, text search) is client-side over the fetched window. Table
// state (page / pageSize / accountId / status / reason / site / date-range /
// search) is URL-synced so a bookmarked URL restores the exact view.
//
// The trigger-all ("运行所有签到") and trigger-one (row action + manual
// dialog) actions call the TanStack Query mutation hooks; the detail sheet
// opens on row "查看详情".

import { useEffect, useMemo, useState } from 'react'
import {
  type ColumnFiltersState,
  type OnChangeFn,
  type PaginationState,
} from '@tanstack/react-table'
import { CalendarRange, Loader2, RotateCw, Zap } from 'lucide-react'
import { toast } from 'sonner'

import { useAccounts } from '@/features/accounts'
import {
  DataTablePage,
  useDataTable,
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

import {
  useCheckinAccount,
  useCheckinLogs,
  useManualCheckin,
} from '../api'
import {
  type CheckinLogRow,
  checkinLogRowSchema,
} from '../types'
import {
  buildCheckinSearchString,
  parseFilterValues,
  readCheckinSearchFromUrl,
} from '../lib/checkin-schema'
import {
  localDatetimeInputToEpochMs,
  parseServerUtcDateTime,
} from '../lib/checkin-time'
import { useCheckinColumns } from './checkin-columns'
import { CheckinDetailSheet } from './checkin-detail-sheet'
import { ManualCheckinDialog } from './manual-checkin-dialog'

const ACCOUNT_FILTER_ALL = 'all'

export function CheckinPage() {
  // --- URL-synced initial state ---
  const initial = useMemo(readCheckinSearchFromUrl, [])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.page - 1,
    pageSize: initial.pageSize,
  })
  const [globalFilter, setGlobalFilter] = useState(initial.q ?? '')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(
    () => {
      const filters: ColumnFiltersState = []
      const statusValues = parseFilterValues(initial.status)
      if (statusValues.length)
        filters.push({ id: 'status', value: statusValues })
      const reasonValues = parseFilterValues(initial.reason)
      if (reasonValues.length)
        filters.push({ id: 'reason', value: reasonValues })
      const siteValues = parseFilterValues(initial.site)
      if (siteValues.length)
        filters.push({ id: 'site', value: siteValues })
      return filters
    },
  )
  const [accountId, setAccountId] = useState<number | undefined>(
    initial.accountId,
  )
  const [from, setFrom] = useState(initial.from ?? '')
  const [to, setTo] = useState(initial.to ?? '')

  // --- queries ---
  const {
    data: rawLogs,
    isLoading,
    isFetching,
    error,
  } = useCheckinLogs({ accountId })
  const { data: accountsSnapshot } = useAccounts()
  const accountOptions = accountsSnapshot?.accounts ?? []

  // --- mutations ---
  const triggerAllMutation = useManualCheckin()
  const triggerOneMutation = useCheckinAccount()

  // --- dialog state ---
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRow, setDetailRow] = useState<CheckinLogRow | null>(null)
  const [manualOpen, setManualOpen] = useState(false)

  // --- client-side date-range filtering over the fetched window ---
  const logs = useMemo(() => {
    const fromMs = localDatetimeInputToEpochMs(from)
    const toMs = localDatetimeInputToEpochMs(to, true)
    return (rawLogs ?? []).filter((row) => {
      if (fromMs === null && toMs === null) return true
      const date = parseServerUtcDateTime(row.checkin_logs.createdAt)
      if (!date) return true
      if (fromMs !== null && date.getTime() < fromMs) return false
      if (toMs !== null && date.getTime() > toMs) return false
      return true
    })
  }, [rawLogs, from, to])

  // --- unique sites (derived for the site faceted filter) ---
  const siteOptions = useMemo(() => {
    const seen = new Map<string, string>()
    for (const row of rawLogs ?? []) {
      const name = row.sites?.name
      if (name && !seen.has(name)) seen.set(name, name)
    }
    return Array.from(seen.entries()).map(([value, label]) => ({
      value,
      label,
    }))
  }, [rawLogs])

  // --- URL write-back ---
  useEffect(() => {
    if (typeof window === 'undefined') return
    const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    const reasonFilter = columnFilters.find((filter) => filter.id === 'reason')
    const siteFilter = columnFilters.find((filter) => filter.id === 'site')
    const query = buildCheckinSearchString({
      pageIndex: pagination.pageIndex,
      pageSize: pagination.pageSize,
      accountId,
      statusValues: Array.isArray(statusFilter?.value)
        ? (statusFilter!.value as string[])
        : [],
      reasonValues: Array.isArray(reasonFilter?.value)
        ? (reasonFilter!.value as string[])
        : [],
      siteValues: Array.isArray(siteFilter?.value)
        ? (siteFilter!.value as string[])
        : [],
      from: from || undefined,
      to: to || undefined,
      query: globalFilter || undefined,
    })
    window.history.replaceState(
      null,
      '',
      `${window.location.pathname}${query}`,
    )
  }, [pagination, accountId, columnFilters, from, to, globalFilter])

  // --- onChange wrappers that reset to the first page on filter changes ---
  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setGlobalFilter((prev) =>
        updater instanceof Function ? updater(prev) : updater,
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    [],
  )
  const onColumnFiltersChange = useMemo<OnChangeFn<ColumnFiltersState>>(
    () => (updater) => {
      setColumnFilters((prev) =>
        updater instanceof Function ? updater(prev) : updater,
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    [],
  )

  // --- row actions ---
  const handleTriggerOne = async (row: CheckinLogRow) => {
    const targetAccountId = row.checkin_logs.accountId
    try {
      const result = await triggerOneMutation.mutateAsync(targetAccountId)
      if (result.status === 'success') {
        toast.success('签到成功', {
          description: result.reward ? `奖励：${result.reward}` : undefined,
        })
      } else if (result.status === 'skipped' || result.skipped) {
        toast.info('签到已跳过', {
          description: result.message || undefined,
        })
      } else {
        toast.error('签到失败', {
          description: result.message || undefined,
        })
      }
    } catch {
      // http-client toasted
    }
  }

  const rowActions = {
    onViewDetail: (row: CheckinLogRow) => {
      setDetailRow(row)
      setDetailOpen(true)
    },
    onTriggerAccount: handleTriggerOne,
  }

  const columns = useCheckinColumns(rowActions)

  const { table } = useDataTable({
    data: logs,
    columns,
    enableRowSelection: true,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange: setPagination,
    globalFilterFn: (row, _columnId, filterValue) => {
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
  })

  const handleTriggerAll = async () => {
    try {
      const result = await triggerAllMutation.mutateAsync()
      const summary = result.summary
      if (summary) {
        const desc = `共 ${summary.total} 个账号：成功 ${summary.success}，失败 ${summary.failed}，跳过 ${summary.skipped}`
        if (summary.failed > 0) {
          toast.error('签到部分失败', { description: desc })
        } else {
          toast.success('签到执行完成', { description: desc })
        }
      } else {
        toast.success(result.message || '签到执行完成')
      }
    } catch {
      // http-client toasted
    }
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
    columnFilters.length > 0 ||
    hasActiveDateRange ||
    accountId !== undefined

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      {/* Header */}
      <div className='flex items-center justify-between'>
        <div>
          <h1 className='text-lg font-semibold'>签到记录</h1>
          <p className='text-sm text-muted-foreground'>
            查看签到执行日志与失败原因，支持手动触发单账号或全部签到。
          </p>
        </div>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            onClick={() => setManualOpen(true)}
            disabled={accountOptions.length === 0}
          >
            <Zap />
            手动签到
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
            运行所有签到
          </Button>
        </div>
      </div>

      {error && (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive'>
          加载签到记录失败：{(error as Error).message}
        </div>
      )}

      {/* Date-range + account filter bar */}
      <div className='flex flex-wrap items-center gap-2'>
        <CalendarRange className='size-4 text-muted-foreground' />
        <Input
          type='datetime-local'
          value={from}
          onChange={(event) => {
            setFrom(event.target.value)
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
          }}
          className='w-[200px]'
          aria-label='开始时间'
        />
        <span className='text-muted-foreground text-sm'>至</span>
        <Input
          type='datetime-local'
          value={to}
          onChange={(event) => {
            setTo(event.target.value)
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
          }}
          className='w-[200px]'
          aria-label='结束时间'
        />
        <Select
          value={accountId ? String(accountId) : ACCOUNT_FILTER_ALL}
          onValueChange={(value) => {
            setAccountId(
              !value || value === ACCOUNT_FILTER_ALL
                ? undefined
                : Number(value),
            )
            setPagination((prev) => ({ ...prev, pageIndex: 0 }))
          }}
        >
          <SelectTrigger className='w-[200px]'>
            <SelectValue placeholder='全部账号' />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ACCOUNT_FILTER_ALL}>全部账号</SelectItem>
            {accountOptions.map((account) => (
              <SelectItem key={account.id} value={String(account.id)}>
                {account.username || `#${account.id}`}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {hasActiveFilters && (
          <Button variant='ghost' size='sm' onClick={handleResetFilters}>
            重置筛选
          </Button>
        )}
      </div>

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle='暂无签到记录'
        emptyDescription='运行一次签到以生成日志，或调整筛选条件。'
        skeletonKeyPrefix='checkin-skeleton'
        toolbarProps={{
          searchPlaceholder: '搜索账号 / 站点 / 信息…',
          searchDebounceMs: 300,
          filters: [
            {
              columnId: 'status',
              title: '状态',
              singleSelect: true,
              options: [
                { label: '成功', value: 'success' },
                { label: '失败', value: 'failed' },
                { label: '跳过', value: 'skipped' },
              ],
            },
            {
              columnId: 'reason',
              title: '失败原因',
              options: [
                { label: '验证', value: 'verification' },
                { label: '认证', value: 'auth' },
                { label: '网络', value: 'network' },
                { label: '站点', value: 'site' },
                { label: '状态', value: 'state' },
                { label: '未知', value: 'unknown' },
              ],
            },
            ...(siteOptions.length > 0
              ? [
                  {
                    columnId: 'site',
                    title: '站点',
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
