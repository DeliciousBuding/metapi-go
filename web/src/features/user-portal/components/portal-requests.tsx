import { useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useCallback, useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { useUrlTableState } from '@/components/data-table'

import { userPortalApi } from '../api'
import { readRequestSearch, requestSearchHref } from '../request-search'
import { RequestList } from './portal-request-list'
import { RequestSheet } from './portal-request-sheet'
import { RequestToolbar } from './portal-request-toolbar'

export function PortalRequests({
  userId,
  page,
  requestId,
  onCloseDetail,
}: {
  userId: number
  page: number
  requestId?: string
  onCloseDetail: () => void
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const detailEntry = useRef<HTMLAnchorElement | null>(null)
  const state = useUrlTableState({
    basePath: '/user',
    read: readRequestSearch,
    buildHref: requestSearchHref,
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })
  const filters = { search: state.globalFilter || undefined, ...state.filters }
  const hasFilters = Boolean(
    state.globalFilter || state.filters.status || state.filters.quotaState
  )
  const query = useQuery({
    queryKey: hasFilters
      ? ['userPortal', userId, 'requests', page, filters]
      : ['userPortal', userId, 'requests', page],
    queryFn: () => userPortalApi.requests(page, filters),
    // Calls made in a terminal do not invalidate the browser's overview cache.
    refetchOnMount: 'always',
  })
  const search = useMemo(
    () => ({
      view: 'requests' as const,
      page,
      q: state.globalFilter || undefined,
      ...state.filters,
    }),
    [page, state.globalFilter, state.filters]
  )
  const navigateToRequest = useCallback(
    (targetId: string) => {
      void navigate({
        to: '/user',
        search: { ...search, requestId: targetId },
        replace: true,
      })
    },
    [navigate, search]
  )
  const { ensurePageInRange } = state
  useEffect(() => {
    if (query.isSuccess && !query.isFetching && !requestId)
      ensurePageInRange(
        Math.max(1, Math.ceil(query.data.total / query.data.pageSize))
      )
  }, [
    query.isSuccess,
    query.isFetching,
    query.data,
    requestId,
    ensurePageInRange,
  ])
  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.requestsHint')}
        </p>
        <QuietRefreshStatus
          updatedAt={query.dataUpdatedAt}
          hasError={query.isError}
        />
      </div>
      <RequestToolbar
        q={state.globalFilter}
        filters={state.filters}
        hasFilters={hasFilters}
        onSearchChange={state.onGlobalFilterChange}
        updateUrlState={state.updateUrlState}
      />
      <RequestList
        query={query}
        search={search}
        requestId={requestId}
        detailEntry={detailEntry}
        hasFilters={hasFilters}
      />
      <RequestSheet
        userId={userId}
        requestId={requestId}
        items={query.data?.items ?? []}
        detailEntry={detailEntry}
        onCloseDetail={onCloseDetail}
        navigateToRequest={navigateToRequest}
      />
    </div>
  )
}
