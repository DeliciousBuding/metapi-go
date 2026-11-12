import { useQuery } from '@tanstack/react-query'
import type { ColumnDef } from '@tanstack/react-table'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import {
  DataTablePage,
  type UrlTableState,
  type UrlTableStateUpdate,
  useDataTable,
  useUrlTableState,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toBcp47 } from '@/i18n/languages'
import { api } from '@/lib/api'
import { formatDateTime } from '@/lib/format'

const auditLogsResponse = z.object({
  items: z.array(
    z.object({
      id: z.number().int(),
      actor: z.string(),
      method: z.string(),
      path: z.string(),
      status: z.number().int(),
      requestId: z.string(),
      remoteIp: z.string(),
      createdAt: z.string(),
    })
  ),
  total: z.number().int().nonnegative(),
  limit: z.number().int().positive(),
  offset: z.number().int().nonnegative(),
})
type AuditEntry = z.infer<typeof auditLogsResponse>['items'][number]
type AuditFilters = { method: string }
const METHODS = ['all', 'POST', 'PUT', 'PATCH', 'DELETE'] as const

function readAuditSearch(search: string): UrlTableState<AuditFilters> {
  const url = new URLSearchParams(search)
  return {
    q: url.get('q') || url.get('path') || '',
    pageIndex: Math.max(0, Math.trunc(Number(url.get('page')) || 0)),
    pageSize: Math.min(100, Math.max(1, Number(url.get('pageSize')) || 20)),
    sorting: [],
    filters: {
      method: METHODS.find((method) => method === url.get('method')) ?? 'all',
    },
  }
}

function buildAuditHref(
  next: UrlTableStateUpdate<AuditFilters>,
  search?: string
) {
  const current = readAuditSearch(search ?? window.location.search)
  const merged = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const url = new URLSearchParams({ category: 'operations' })
  if (merged.q) url.set('q', merged.q)
  if (merged.filters.method !== 'all') url.set('method', merged.filters.method)
  if (merged.pageIndex) url.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) url.set('pageSize', String(merged.pageSize))
  return `/audit-logs?${url}`
}

export function AdminAuditLogTable() {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language)
  const state = useUrlTableState<AuditFilters>({
    basePath: '/audit-logs',
    read: readAuditSearch,
    buildHref: buildAuditHref,
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })
  const params = new URLSearchParams({
    limit: String(state.pagination.pageSize),
    offset: String(state.pagination.pageIndex * state.pagination.pageSize),
  })
  if (state.filters.method !== 'all') params.set('method', state.filters.method)
  if (state.globalFilter) params.set('path', state.globalFilter)
  const query = useQuery({
    queryKey: ['admin-audit-logs', params.toString()],
    queryFn: async () =>
      auditLogsResponse.parse(await api.getAdminAuditLogs(params)),
    staleTime: 10_000,
  })
  const columns = useMemo<ColumnDef<AuditEntry>[]>(
    () => [
      {
        accessorKey: 'createdAt',
        size: 165,
        header: t('settings.operations.auditLogs.columns.time'),
        meta: {
          label: t('settings.operations.auditLogs.columns.time'),
          mobileOrder: 2,
        },
        cell: ({ row }) => (
          <span className='text-muted-foreground text-xs tabular-nums'>
            {formatDateTime(row.original.createdAt, locale)}
          </span>
        ),
      },
      {
        accessorKey: 'path',
        size: 280,
        header: t('settings.operations.auditLogs.columns.path'),
        meta: {
          label: t('settings.operations.auditLogs.columns.path'),
          mobileTitle: true,
        },
        cell: ({ row }) => (
          <span className='min-w-0 font-mono text-xs break-all'>
            {row.original.path}
          </span>
        ),
      },
      {
        accessorKey: 'method',
        size: 100,
        header: t('settings.operations.auditLogs.columns.method'),
        meta: {
          label: t('settings.operations.auditLogs.columns.method'),
          mobileOrder: 0,
        },
      },
      {
        accessorKey: 'status',
        size: 80,
        header: t('settings.operations.auditLogs.columns.status'),
        meta: {
          label: t('settings.operations.auditLogs.columns.status'),
          mobileOrder: 1,
        },
        cell: ({ row }) => (
          <span
            className={
              row.original.status >= 400 ? 'text-destructive' : 'text-success'
            }
          >
            {row.original.status}
          </span>
        ),
      },
      {
        accessorKey: 'actor',
        size: 100,
        header: t('settings.operations.auditLogs.columns.actor'),
        meta: {
          label: t('settings.operations.auditLogs.columns.actor'),
          mobileOrder: 3,
        },
      },
      {
        accessorKey: 'remoteIp',
        size: 130,
        header: t('settings.operations.auditLogs.columns.ip'),
        meta: {
          label: t('settings.operations.auditLogs.columns.ip'),
          mobileOrder: 4,
        },
      },
    ],
    [t, locale]
  )
  const { table } = useDataTable({
    data: query.data?.items ?? [],
    columns,
    getRowId: (row) => String(row.id),
    manualFiltering: true,
    manualPagination: true,
    manualSorting: true,
    globalFilter: state.globalFilter,
    onGlobalFilterChange: state.onGlobalFilterChange,
    pagination: state.pagination,
    onPaginationChange: state.onPaginationChange,
    totalCount: query.data?.total ?? 0,
  })
  return (
    <DataTablePage
      table={table}
      isLoading={query.isLoading}
      isFetching={query.isFetching}
      error={query.error}
      errorMessageKey='settings.common.loadFailed'
      onErrorRetry={() => void query.refetch()}
      emptyTitle={t('settings.operations.auditLogs.empty')}
      toolbarProps={{
        searchPlaceholder: t('settings.operations.auditLogs.pathPlaceholder'),
        hasAdditionalFilters: state.filters.method !== 'all',
        onReset: () =>
          state.updateUrlState({
            q: '',
            pageIndex: 0,
            filters: { method: 'all' },
          }),
        additionalSearch: (
          <>
            <Select
              value={state.filters.method}
              onValueChange={(method) =>
                state.updateUrlState({
                  pageIndex: 0,
                  filters: { method: method ?? 'all' },
                })
              }
            >
              <SelectTrigger
                size='sm'
                className='w-32'
                aria-label={t('settings.operations.auditLogs.columns.method')}
              >
                <SelectValue>
                  {(method) =>
                    method === 'all'
                      ? t('settings.operations.auditLogs.allMethods')
                      : String(method)
                  }
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                {METHODS.map((method) => (
                  <SelectItem key={method} value={method}>
                    {method === 'all'
                      ? t('settings.operations.auditLogs.allMethods')
                      : method}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              variant='outline'
              size='sm'
              disabled={query.isFetching}
              onClick={() => void query.refetch()}
            >
              {t('proxyLogs.page.refresh')}
            </Button>
          </>
        ),
      }}
    />
  )
}
