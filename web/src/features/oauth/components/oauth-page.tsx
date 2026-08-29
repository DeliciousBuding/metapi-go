// metapi-go/features/oauth — list page.
//
// Wires the four-layer data-table to the OAuth connections query and the
// URL. Table state (search query / page / page-size / sorting / status
// faceted filter) is mirrored to the URL search string so a deep link
// restores the exact view. The "start authorization" dialog opens an
// OAuth flow in a new tab. Row actions: view details (opens the read-only
// detail sheet), refresh quota, rebind (re-run the OAuth flow), delete.
// Mobile cards are handled by `DataTablePage` via the column `meta` flags.
//
// Pagination is SERVER-side (manualPagination): the connections query
// fetches one limit/offset page at a time and the pager reflects the
// backend's real total — the old client-side mode fetched limit:1000 and
// silently truncated larger fleets. The backend has no server-side
// q/status/sort params, so the toolbar search + status filter remain
// client-side over the fetched page and column sorting is disabled (a
// sortable header would promise cross-page sorting the backend cannot
// honour). Deep links keep parsing `sort` tolerantly for old URLs.

import { Plus as PlusIcon } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
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
import { Spinner } from '@/components/ui/spinner'
import { asStringParam, parseSortingParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import {
  useDeleteOAuthConnection,
  useOAuthConnections,
  useRebindOAuthConnection,
  useRefreshOAuthQuota,
} from '../api'
import { resolveOAuthConnectionLabel } from '../lib/oauth-connection-label'
import { oauthSearchSchema } from '../lib/oauth-schema'
import type { OAuthClient } from '../types'
import { OAUTH_STATUS_FILTER_OPTIONS, useOAuthColumns } from './oauth-columns'
import { OAuthDetailSheet } from './oauth-detail-sheet'
import { OAuthStartDialog } from './oauth-start-dialog'

const OAUTH_COLUMN_VISIBILITY_STORAGE_KEY = 'metapi-go:oauth:column-visibility'
const OAUTH_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:oauth:column-sizing'

type OAuthUrlFilters = { status: string | undefined }

function readSearch(searchString?: string): UrlTableState<OAuthUrlFilters> {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { status: undefined },
    }
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
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { status: undefined },
    }
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: { status: asStringParam(data.status) },
  }
}

function buildHref(next: UrlTableStateUpdate<OAuthUrlFilters>): string {
  const current = readSearch()
  const merged: UrlTableState<OAuthUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.filters.status) params.set('status', merged.filters.status)
  const queryString = params.toString()
  return queryString ? `/oauth?${queryString}` : '/oauth'
}

/**
 * The "feature useSearch" stage. Reads the URL on every render (cheap) and
 * hands the data-table controlled state + navigation-backed setters. The
 * shared {@link useUrlTableState} owns the router subscription and the
 * URL-sync guard.
 */
function useOAuthUrlState() {
  return useUrlTableState<OAuthUrlFilters>({
    basePath: '/oauth',
    read: readSearch,
    buildHref,
    toColumnFilters: (filters) =>
      filters.status ? [{ id: 'status', value: filters.status }] : [],
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? (statusEntry.value as string[])[0]
            : (statusEntry?.value as string | undefined),
        },
      }
    },
  })
}

/**
 * Pick the account id of whichever mutation (refresh-quota or rebind) is
 * currently pending. Both mutations take a bare `accountId: number` as
 * variables. Returns `null` when neither is in flight — the actions cell
 * renders all rows as clickable. Kept as a plain function (not a hook) so
 * the page render stays cheap and the derivation is grep-able/testable.
 */
function resolvePendingAccountId(
  refresh: { isPending: boolean; variables?: number },
  rebind: { isPending: boolean; variables?: number }
): number | null {
  if (refresh.isPending) return refresh.variables ?? null
  if (rebind.isPending) return rebind.variables ?? null
  return null
}

export function OAuthPage() {
  const { t } = useTranslation()
  const urlState = useOAuthUrlState()
  const connectionsQuery = useOAuthConnections({
    page: urlState.pagination.pageIndex,
    pageSize: urlState.pagination.pageSize,
  })
  const connections = connectionsQuery.data?.items ?? []
  const connectionsTotal = connectionsQuery.data?.total ?? 0
  const deleteConnection = useDeleteOAuthConnection()
  const refreshQuota = useRefreshOAuthQuota()
  const rebindConnection = useRebindOAuthConnection()

  const [startOpen, setStartOpen] = useState(false)
  const [deletingClient, setDeletingClient] = useState<OAuthClient | null>(null)
  // The detail sheet tracks the account id rather than a row snapshot: the
  // sheet's own refresh-quota action invalidates the connections query, and a
  // frozen snapshot would keep showing the pre-refresh quota. Re-deriving
  // from the current page makes the refreshed numbers appear in place.
  const [detailAccountId, setDetailAccountId] = useState<number | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const detailConnection =
    connections.find(
      (connection) => connection.accountId === detailAccountId
    ) ?? null

  // Derive the per-row pending account id from whichever mutation is in
  // flight. A single value is sufficient because BOTH row actions (refresh +
  // rebind) should be disabled for the row with an action pending, while
  // every other row stays clickable (no global lock) — mirroring the
  // accounts page's `pendingStatusId` pattern. `useRefreshOAuthQuota` and
  // `useRebindOAuthConnection` both take a bare `accountId: number` as
  // variables, so the pending id is `mutation.variables` when pending.
  const pendingAccountId = resolvePendingAccountId(
    refreshQuota,
    rebindConnection
  )

  // Both entry points (row action menu + detail sheet footer) call these, so
  // there is exactly one mutation instance and one toast vocabulary per
  // operation.
  function handleRefreshQuota(client: OAuthClient) {
    refreshQuota.mutate(client.accountId, {
      onSuccess: () =>
        toast.success(
          t('oauth.toast.quotaRefreshed', { id: client.accountId })
        ),
      onError: () =>
        toast.error(
          t('oauth.toast.refreshFailed', {
            name: resolveOAuthConnectionLabel(client),
          })
        ),
    })
  }

  function handleRebind(client: OAuthClient) {
    rebindConnection.mutate(client.accountId, {
      onSuccess: (result) => {
        if (result.authorizationUrl) {
          window.open(result.authorizationUrl, '_blank', 'noopener,noreferrer')
        }
        toast.success(t('oauth.toast.rebindStarted', { id: client.accountId }))
      },
      onError: () =>
        toast.error(
          t('oauth.toast.rebindFailed', {
            name: resolveOAuthConnectionLabel(client),
          })
        ),
    })
  }

  const columns = useOAuthColumns(
    {
      onViewDetails: (client) => {
        setDetailAccountId(client.accountId)
        setDetailOpen(true)
      },
      onRefreshQuota: handleRefreshQuota,
      onRebind: handleRebind,
      onDelete: (client) => {
        setDeletingClient(client)
      },
    },
    pendingAccountId
  )

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
      // http-client toasted
    } finally {
      setDeletingClient(null)
    }
  }

  const statusFilters = OAUTH_STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='text-lg font-normal'>{t('oauth.page.title')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('oauth.page.description')}
        </p>
      </div>

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={connectionsQuery.isLoading}
        isFetching={connectionsQuery.isFetching}
        error={connectionsQuery.error as Error | null}
        errorMessageKey='oauth.page.loadError'
        onErrorRetry={() => connectionsQuery.refetch()}
        isErrorRetrying={connectionsQuery.isFetching}
        errorPlacement='inline'
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

      <OAuthStartDialog open={startOpen} onOpenChange={setStartOpen} />

      <OAuthDetailSheet
        connection={detailConnection}
        // A row that leaves the current page (deleted, or the operator paged
        // away) closes its own sheet instead of leaving an empty panel open.
        open={detailOpen && detailConnection !== null}
        onOpenChange={(open) => {
          setDetailOpen(open)
          if (!open) setDetailAccountId(null)
        }}
        onRefreshQuota={handleRefreshQuota}
        onRebind={handleRebind}
        isRefreshingQuota={
          refreshQuota.isPending &&
          refreshQuota.variables === detailConnection?.accountId
        }
        isRebinding={
          rebindConnection.isPending &&
          rebindConnection.variables === detailConnection?.accountId
        }
      />

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
              {deleteConnection.isPending && <Spinner />}
              {t('oauth.delete.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
