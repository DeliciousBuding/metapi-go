import { KeyRound, Plus as PlusIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'

import type { useOAuthConnections } from '../api'
import type { OAuthClient } from '../types'
import {
  OAUTH_STATUS_FILTER_OPTIONS,
  useOAuthColumns,
  type OAuthColumnActions,
} from './oauth-columns'
import type { useOAuthUrlState } from './use-oauth-url-state'

// Pagination is SERVER-side (manualPagination): the connections query
// fetches one limit/offset page at a time and the pager reflects the
// backend's real total — the old client-side mode fetched limit:1000 and
// silently truncated larger fleets. The backend has no server-side
// q/status/sort params, so the toolbar search + status filter remain
// client-side over the fetched page and column sorting is disabled (a
// sortable header would promise cross-page sorting the backend cannot
// honour). Deep links keep parsing `sort` tolerantly for old URLs.

const OAUTH_COLUMN_VISIBILITY_STORAGE_KEY = 'metapi-go:oauth:column-visibility'
const OAUTH_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:oauth:column-sizing'

export function OAuthConnectionsTable({
  urlState,
  connectionsQuery,
  connections,
  connectionsTotal,
  actions,
  pendingAccountId,
  handleStart,
}: {
  urlState: ReturnType<typeof useOAuthUrlState>
  connectionsQuery: ReturnType<typeof useOAuthConnections>
  connections: OAuthClient[]
  connectionsTotal: number
  actions: OAuthColumnActions
  pendingAccountId: number | null
  handleStart: () => void
}) {
  const { t } = useTranslation()
  const columns = useOAuthColumns(actions, pendingAccountId)
  const { table } = useDataTable<OAuthClient>({
    data: connections,
    columns,
    manualPagination: true,
    // The backend list endpoint has no sort param; keep headers static
    // instead of offering sorting that cannot work across server pages.
    manualSorting: true,
    // Server-paged data changes on every page fetch; TanStack's auto page
    // reset would snap deep-linked pages back to the first one.
    autoResetPageIndex: false,
    totalCount: connectionsTotal,
    enableRowSelection: false,
    enableColumnResizing: true,
    columnVisibilityStorageKey: OAUTH_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: OAUTH_COLUMN_SIZING_STORAGE_KEY,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    ensurePageInRange: urlState.ensurePageInRange,
    getRowId: (row) => String(row.accountId),
  })

  const statusFilters = OAUTH_STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <DataTablePage
      table={table}
      isLoading={connectionsQuery.isLoading}
      isFetching={connectionsQuery.isFetching}
      error={connectionsQuery.error as Error | null}
      errorMessageKey='oauth.page.loadError'
      onErrorRetry={() => connectionsQuery.refetch()}
      isErrorRetrying={connectionsQuery.isFetching}
      errorPlacement='inline'
      emptyIcon={<KeyRound className='size-6' />}
      emptyTitle={t('oauth.empty.title')}
      emptyDescription={t('oauth.empty.description')}
      emptyAction={
        <Button onClick={handleStart}>
          <PlusIcon className='size-4' />
          {t('oauth.empty.startAuth')}
        </Button>
      }
      skeletonKeyPrefix='oauth-skeleton'
      toolbarProps={{
        searchPlaceholder: t('oauth.toolbar.searchPlaceholder'),
        searchDebounceMs: 400,
        filters: [
          {
            columnId: 'status',
            title: t('oauth.columns.status'),
            options: statusFilters,
            singleSelect: true,
          },
        ],
        preActions: (
          <Button onClick={handleStart}>
            <PlusIcon className='size-4' />
            {t('oauth.toolbar.startAuth')}
          </Button>
        ),
      }}
    />
  )
}
