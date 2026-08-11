// metapi-go/features/oauth — list page.
//
// Wires the four-layer data-table to the OAuth connections query and the
// URL. Table state (search query / page / page-size / sorting / status
// faceted filter) is mirrored to the URL search string so a deep link
// restores the exact view. The "start authorization" dialog opens an
// OAuth flow in a new tab. Row actions: refresh quota, rebind (re-run the
// OAuth flow), delete. Mobile cards are handled by `DataTablePage` via the
// column `meta` flags.

import { useLocation, useNavigate } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { Plus as PlusIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

import {
  useDeleteOAuthConnection,
  useOAuthConnections,
  useRebindOAuthConnection,
  useRefreshOAuthQuota,
} from '../api'
import { oauthSearchSchema } from '../lib/oauth-schema'
import type { OAuthClient } from '../types'
import {
  OAUTH_STATUS_FILTER_OPTIONS,
  useOAuthColumns,
} from './oauth-columns'
import { OAuthStartDialog } from './oauth-start-dialog'

const OAUTH_COLUMN_VISIBILITY_STORAGE_KEY = 'metapi-go:oauth:column-visibility'
const OAUTH_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:oauth:column-sizing'

type ResolvedSearch = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  status: string | undefined
}

function resolveUpdater<TValue>(updater: Updater<TValue>, previous: TValue): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

function encodeSorting(sorting: SortingState): string {
  return sorting
    .map((item) => `${item.id}:${item.desc ? 'desc' : 'asc'}`)
    .join(',')
}

function readSearch(searchString?: string): ResolvedSearch {
  if (typeof window === 'undefined') {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], status: undefined }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = oauthSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    status: params.get('status') ?? undefined,
  })
  if (!parsed.success) {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], status: undefined }
  }
  const data = parsed.data
  return {
    q: data.q ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: Array.isArray(data.sort) ? (data.sort as SortingState) : [],
    status: data.status,
  }
}

function buildHref(next: Partial<ResolvedSearch>): string {
  const current = readSearch()
  const merged: ResolvedSearch = { ...current, ...next }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.status) params.set('status', merged.status)
  const queryString = params.toString()
  return queryString ? `/oauth?${queryString}` : '/oauth'
}

function useOAuthUrlState() {
  const navigate = useNavigate()
  // Subscribe to the router location: TanStack Router does not re-render a
  // route component on same-path search-only navigation unless a hook
  // consumes the location, and readSearch() reads the URL on render.
  const searchStr = useLocation({ select: (loc) => loc.searchStr })
  const search = readSearch(searchStr)

  const columnFilters: ColumnFiltersState = useMemo(() => {
    if (!search.status) return []
    return [{ id: 'status', value: search.status }]
  }, [search.status])

  // URL-sync guard: table state callbacks can fire while the router is
  // navigating away (the useLocation subscription re-renders this page with
  // the *next* location's search string). Without the pathname check the
  // callback would navigate straight back, hijacking the in-flight
  // navigation. Only sync when we are still on this page.
  function syncUrl(next: Partial<ResolvedSearch>) {
    const href = buildHref(next)
    if (!href.startsWith(window.location.pathname)) return
    navigate({ href, replace: true })
  }

  const onGlobalFilterChange = (updater: Updater<string>) => {
    const next = resolveUpdater(updater, search.q)
    syncUrl({ q: next })
  }
  const onPaginationChange = (updater: Updater<PaginationState>) => {
    const next = resolveUpdater(updater, {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    })
    syncUrl({ pageIndex: next.pageIndex, pageSize: next.pageSize })
  }
  const onSortingChange = (updater: Updater<SortingState>) => {
    const next = resolveUpdater(updater, search.sorting)
    syncUrl({ sorting: next })
  }
  const onColumnFiltersChange = (updater: Updater<ColumnFiltersState>) => {
    const next = resolveUpdater(updater, columnFilters)
    const statusEntry = next.find((filter) => filter.id === 'status')
    const statusValue =
      statusEntry && Array.isArray(statusEntry.value)
        ? (statusEntry.value as string[])[0]
        : (statusEntry?.value as string | undefined)
    syncUrl({ status: statusValue })
  }

  return {
    globalFilter: search.q,
    onGlobalFilterChange,
    pagination: {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    } as PaginationState,
    onPaginationChange,
    sorting: search.sorting,
    onSortingChange,
    columnFilters,
    onColumnFiltersChange,
  }
}

export function OAuthPage() {
  const { t } = useTranslation()
  const connectionsQuery = useOAuthConnections()
  const deleteConnection = useDeleteOAuthConnection()
  const refreshQuota = useRefreshOAuthQuota()
  const rebindConnection = useRebindOAuthConnection()

  const [startOpen, setStartOpen] = useState(false)
  const [deletingClient, setDeletingClient] = useState<OAuthClient | null>(null)

  const urlState = useOAuthUrlState()

  const columns = useOAuthColumns({
    onRefreshQuota: (client) => {
      refreshQuota.mutate(client.accountId, {
        onSuccess: () =>
          toast.success(t('oauth.toast.quotaRefreshed', { id: client.accountId })),
        onError: () => toast.error(t('oauth.toast.quotaRefreshFailed')),
      })
    },
    onRebind: (client) => {
      rebindConnection.mutate(client.accountId, {
        onSuccess: (result) => {
          if (result.authorizationUrl) {
            window.open(result.authorizationUrl, '_blank', 'noopener,noreferrer')
          }
          toast.success(t('oauth.toast.rebindStarted', { id: client.accountId }))
        },
        onError: () => toast.error(t('oauth.toast.rebindFailed')),
      })
    },
    onDelete: (client) => {
      setDeletingClient(client)
    },
  })

  const { table } = useDataTable<OAuthClient>({
    data: connectionsQuery.data ?? [],
    columns,
    enableRowSelection: true,
    enableColumnResizing: true,
    columnVisibilityStorageKey: OAUTH_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: OAUTH_COLUMN_SIZING_STORAGE_KEY,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    getRowId: (row) => String(row.accountId),
  })

  function handleStart() {
    setStartOpen(true)
  }

  async function confirmDelete() {
    if (!deletingClient) return
    const client = deletingClient
    try {
      await deleteConnection.mutateAsync(client.accountId)
      toast.success(t('oauth.toast.deleted', { id: client.accountId }))
    } catch {
      toast.error(t('oauth.toast.deleteFailed'))
    } finally {
      setDeletingClient(null)
    }
  }

  const statusFilters = OAUTH_STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={connectionsQuery.isLoading}
        isFetching={connectionsQuery.isFetching}
        emptyTitle={t('oauth.empty.title')}
        emptyDescription={t('oauth.empty.description')}
        emptyAction={
          <Button onClick={handleStart}>
            <PlusIcon className='mr-1 size-4' />
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
              <PlusIcon className='mr-1 size-4' />
              {t('oauth.toolbar.startAuth')}
            </Button>
          ),
        }}
      />

      <OAuthStartDialog open={startOpen} onOpenChange={setStartOpen} />

      <Dialog
        open={deletingClient !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingClient(null)
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('oauth.delete.title')}</DialogTitle>
            <DialogDescription>
              {t('oauth.delete.description', {
                provider: deletingClient?.provider ?? '',
                id: deletingClient?.accountId ?? 0,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeletingClient(null)}
              disabled={deleteConnection.isPending}
            >
              {t('oauth.delete.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteConnection.isPending}
            >
              {t('oauth.delete.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
