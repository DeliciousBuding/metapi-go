// metapi-go/features/models — list page (model marketplace).
//
// Wires the four-layer data-table to the models query and the URL. Table
// state (search query / page / page-size / sorting / brand faceted filter /
// capability faceted filter) is mirrored to the URL search string so a deep
// link restores the exact view. The hook below is the "feature useSearch"
// stage of the three-stage pattern (route validateSearch -> feature useSearch
// -> useDataTable); it safe-parses `window.location.search` directly so the
// page works before the `/models` route file lands its own validateSearch.
// Mobile cards are handled by `DataTablePage` via the column `meta` flags.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { ColumnFiltersState } from '@tanstack/react-table'
import {
  FlaskConical as FlaskConicalIcon,
  RefreshCw as RefreshCwIcon,
  Users as UsersIcon,
} from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import { api } from '@/lib/api'
import {
  asStringParam,
  encodeStringListParam,
  parseSortingParam,
  parseStringListParam,
} from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import { useModels } from '../api'
import { modelsSearchSchema } from '../lib/models-schema'
import { modelsKeys, type ModelRow } from '../types'
import { ModelDetailSheet } from './model-detail-sheet'
import { ModelVerifyDialog } from './model-verify-dialog'
import {
  buildBrandFilterOptions,
  buildCapabilityFilterOptions,
  buildEndpointTypeFilterOptions,
  useModelsColumns,
} from './models-columns'

const MODELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:models:column-visibility'
const MODELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:models:column-sizing'

type ModelsUrlFilters = {
  brand: string[]
  capability: string[]
  endpointType: string[]
}

function readSearch(searchString?: string): UrlTableState<ModelsUrlFilters> {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { brand: [], capability: [], endpointType: [] },
    }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = modelsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    brand: params.get('brand') ?? undefined,
    capability: params.get('capability') ?? undefined,
    endpointType: params.get('endpointType') ?? undefined,
  })
  if (!parsed.success) {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { brand: [], capability: [], endpointType: [] },
    }
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: {
      brand: parseStringListParam(data.brand),
      capability: parseStringListParam(data.capability),
      endpointType: parseStringListParam(data.endpointType),
    },
  }
}

function buildHref(next: UrlTableStateUpdate<ModelsUrlFilters>): string {
  const current = readSearch()
  const merged: UrlTableState<ModelsUrlFilters> = {
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
  const brandString = encodeStringListParam(merged.filters.brand)
  if (brandString) params.set('brand', brandString)
  const capabilityString = encodeStringListParam(merged.filters.capability)
  if (capabilityString) params.set('capability', capabilityString)
  const endpointTypeString = encodeStringListParam(merged.filters.endpointType)
  if (endpointTypeString) params.set('endpointType', endpointTypeString)
  const queryString = params.toString()
  return queryString ? `/models?${queryString}` : '/models'
}

/**
 * The "feature useSearch" stage. Reads the URL on every render (cheap) and
 * hands the data-table controlled state + navigation-backed setters. The
 * shared {@link useUrlTableState} owns the router subscription and the
 * URL-sync guard.
 */
function useModelsUrlState() {
  return useUrlTableState<ModelsUrlFilters>({
    basePath: '/models',
    read: readSearch,
    buildHref,
    // Filter changes (search / faceted brand / capability / endpoint-type) reset
    // the page in the same URL transaction; TanStack's own auto-reset is disabled
    // so a single page update is issued instead of a redundant bounce.
    resetPageIndexOnFilterChange: true,
    toColumnFilters: (filters) => {
      const columnFilters: ColumnFiltersState = []
      if (filters.brand.length > 0) {
        columnFilters.push({ id: 'brand', value: filters.brand })
      }
      if (filters.capability.length > 0) {
        columnFilters.push({ id: 'capabilities', value: filters.capability })
      }
      if (filters.endpointType.length > 0) {
        columnFilters.push({ id: 'endpointTypes', value: filters.endpointType })
      }
      return columnFilters
    },
    fromColumnFilters: (filters) => {
      const brandEntry = filters.find((filter) => filter.id === 'brand')
      const capabilityEntry = filters.find(
        (filter) => filter.id === 'capabilities'
      )
      const endpointTypeEntry = filters.find(
        (filter) => filter.id === 'endpointTypes'
      )
      return {
        filters: {
          brand: Array.isArray(brandEntry?.value)
            ? (brandEntry.value as string[])
            : [],
          capability: Array.isArray(capabilityEntry?.value)
            ? (capabilityEntry.value as string[])
            : [],
          endpointType: Array.isArray(endpointTypeEntry?.value)
            ? (endpointTypeEntry.value as string[])
            : [],
        },
      }
    },
  })
}

export function ModelsPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const modelsQuery = useModels({ includePricing: true })
  const navigate = useNavigate()

  const [viewingModel, setViewingModel] = useState<ModelRow | null>(null)
  const [verifyOpen, setVerifyOpen] = useState(false)

  // Consume the one-shot `?model=<name>` deep link exactly once (the search
  // palette passes it when a model hit is selected): resolve the name against
  // the loaded marketplace, open the detail sheet with the same state the row
  // "view" action uses, then strip the transient param so a refetch or remount
  // never reopens the sheet. Mirrors the accounts page's `accountId` deep
  // link (ref guard survives strict-mode double-invoke).
  const modelDeepLinkConsumed = useRef(false)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const modelName = params.get('model')
    if (modelDeepLinkConsumed.current || !modelName) return
    modelDeepLinkConsumed.current = true

    const targetModel = (modelsQuery.data ?? []).find(
      (model) => model.name === modelName
    )
    if (targetModel) {
      setViewingModel(targetModel)
    }
    params.delete('model')
    const queryString = params.toString()
    navigate({
      href: queryString ? `/models?${queryString}` : '/models',
      replace: true,
    })
  }, [modelsQuery.data, navigate])

  const urlState = useModelsUrlState()

  const columns = useModelsColumns({
    onView: (model) => {
      setViewingModel(model)
    },
    onTest: (model) => {
      const params = new URLSearchParams({ model: model.name })
      navigate({ href: `/model-tester?${params.toString()}` })
    },
  })

  const { table } = useDataTable<ModelRow>({
    data: modelsQuery.data ?? [],
    columns,
    enableColumnResizing: true,
    columnVisibilityStorageKey: MODELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: MODELS_COLUMN_SIZING_STORAGE_KEY,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    ensurePageInRange: urlState.ensurePageInRange,
    // The URL-synced callbacks already reset the page on every filter change
    // (resetPageIndexOnFilterChange), so disable TanStack's own auto-reset to
    // avoid a second redundant page update (the models page bounced back to
    // page 0 on every pagination click otherwise).
    autoResetPageIndex: false,
    getRowId: (row) => row.name,
  })

  const refreshMutation = useMutation({
    mutationFn: async () => {
      await api.getModelsMarketplace({ refresh: true, includePricing: true })
    },
    onSuccess: () => {
      // The refresh call re-aggregates the marketplace server-side. The list
      // query has a 10s staleTime and would keep serving the pre-refresh
      // cache during that window, so invalidate the models prefix explicitly
      // to refetch immediately (list + capability selectors share it).
      void queryClient.invalidateQueries({ queryKey: modelsKeys.all })
      toast.success(t('models.toast.refreshSucceeded'))
    },
  })
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data])
  const brandFilterOptions = useMemo(
    () => buildBrandFilterOptions(models),
    [models]
  )
  const capabilityFilterOptions = useMemo(
    () => buildCapabilityFilterOptions(models),
    [models]
  )
  const endpointTypeFilterOptions = useMemo(
    () => buildEndpointTypeFilterOptions(models),
    [models]
  )

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='text-lg font-normal'>{t('models.page.title')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('models.page.description')}
        </p>
      </div>

      {/* Unified list-page error contract (W19-T1 P2-o): the failed load
          replaces the table instead of stacking over it, so a stale cache can
          never read as current data. */}
      {modelsQuery.error ? (
        <QueryErrorBanner
          error={modelsQuery.error as Error | null}
          messageKey='models.page.loadError'
          onRetry={() => modelsQuery.refetch()}
          isRetrying={modelsQuery.isFetching}
        />
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={modelsQuery.isLoading}
          isFetching={modelsQuery.isFetching}
          emptyTitle={t('models.empty.title')}
          emptyDescription={t('models.empty.description')}
          emptyAction={
            <Button
              variant='outline'
              onClick={() => void navigate({ to: '/accounts' })}
            >
              <UsersIcon className='size-4' />
              {t('models.empty.manageAccounts')}
            </Button>
          }
          skeletonKeyPrefix='model-skeleton'
          toolbarProps={{
            searchPlaceholder: t('models.toolbar.searchPlaceholder'),
            searchDebounceMs: 400,
            filters: [
              {
                columnId: 'brand',
                title: t('models.columns.brand'),
                options: brandFilterOptions,
              },
              {
                columnId: 'endpointTypes',
                title: t('models.columns.endpointTypes'),
                options: endpointTypeFilterOptions,
              },
              {
                columnId: 'capabilities',
                title: t('models.columns.capabilities'),
                options: capabilityFilterOptions,
              },
            ],
            preActions: (
              <>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => setVerifyOpen(true)}
                >
                  <FlaskConicalIcon className='size-3.5' />
                  {t('models.toolbar.batchProbe')}
                </Button>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => refreshMutation.mutate()}
                  disabled={refreshMutation.isPending}
                >
                  {refreshMutation.isPending ? (
                    <Spinner />
                  ) : (
                    <RefreshCwIcon className='size-3.5' />
                  )}
                  {t('models.toolbar.refresh')}
                </Button>
              </>
            ),
          }}
        />
      )}

      <ModelDetailSheet
        model={viewingModel}
        open={viewingModel !== null}
        onOpenChange={(open) => {
          if (!open) setViewingModel(null)
        }}
      />

      <ModelVerifyDialog open={verifyOpen} onOpenChange={setVerifyOpen} />
    </div>
  )
}
