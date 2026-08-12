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

import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import type { ColumnFiltersState } from '@tanstack/react-table'
import { RefreshCw as RefreshCwIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
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

import { useModels } from '../api'
import { modelsSearchSchema } from '../lib/models-schema'
import type { ModelRow } from '../types'
import { ModelDetailSheet } from './model-detail-sheet'
import {
  buildBrandFilterOptions,
  buildCapabilityFilterOptions,
  useModelsColumns,
} from './models-columns'

const MODELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:models:column-visibility'
const MODELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:models:column-sizing'

type ModelsUrlFilters = {
  brand: string[]
  capability: string[]
}

function readSearch(searchString?: string): UrlTableState<ModelsUrlFilters> {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { brand: [], capability: [] },
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
  })
  if (!parsed.success) {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { brand: [], capability: [] },
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
    },
  }
}

function buildHref(next: Partial<UrlTableState<ModelsUrlFilters>>): string {
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
    toColumnFilters: (filters) => {
      const columnFilters: ColumnFiltersState = []
      if (filters.brand.length > 0) {
        columnFilters.push({ id: 'brand', value: filters.brand })
      }
      if (filters.capability.length > 0) {
        columnFilters.push({ id: 'capabilities', value: filters.capability })
      }
      return columnFilters
    },
    fromColumnFilters: (filters) => {
      const brandEntry = filters.find((filter) => filter.id === 'brand')
      const capabilityEntry = filters.find(
        (filter) => filter.id === 'capabilities'
      )
      return {
        filters: {
          brand: Array.isArray(brandEntry?.value)
            ? (brandEntry.value as string[])
            : [],
          capability: Array.isArray(capabilityEntry?.value)
            ? (capabilityEntry.value as string[])
            : [],
        },
      }
    },
  })
}

export function ModelsPage() {
  const { t } = useTranslation()
  const modelsQuery = useModels({ includePricing: true })
  const navigate = useNavigate()

  const [viewingModel, setViewingModel] = useState<ModelRow | null>(null)

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
    getRowId: (row) => row.name,
  })

  const refreshMutation = useMutation({
    mutationFn: async () => {
      await api.getModelsMarketplace({ refresh: true, includePricing: true })
    },
    onSuccess: () => {
      toast.success(t('models.toast.refreshSucceeded'))
    },
  })

  // The refresh call re-aggregates the marketplace server-side; the next
  // render re-reads the (now fresh) `useModels` cache via its 10s staleTime,
  // so no explicit query invalidation is needed for the list to update.
  const models = useMemo(() => modelsQuery.data ?? [], [modelsQuery.data])
  const brandFilterOptions = useMemo(
    () => buildBrandFilterOptions(models),
    [models]
  )
  const capabilityFilterOptions = useMemo(
    () => buildCapabilityFilterOptions(models),
    [models]
  )

  return (
    <>
      {modelsQuery.error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive rounded-lg border p-3 text-sm'>
          {t('models.page.loadError', {
            message: (modelsQuery.error as Error).message,
          })}
        </div>
      )}
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={modelsQuery.isLoading}
        isFetching={modelsQuery.isFetching}
        emptyTitle={t('models.empty.title')}
        emptyDescription={t('models.empty.description')}
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
              columnId: 'capabilities',
              title: t('models.columns.capabilities'),
              options: capabilityFilterOptions,
            },
          ],
          preActions: (
            <Button
              variant='outline'
              size='sm'
              onClick={() => refreshMutation.mutate()}
              disabled={refreshMutation.isPending}
            >
              {refreshMutation.isPending ? (
                <Spinner className='mr-1' />
              ) : (
                <RefreshCwIcon className='mr-1 size-3.5' />
              )}
              {t('models.toolbar.refresh')}
            </Button>
          ),
        }}
      />

      <ModelDetailSheet
        model={viewingModel}
        open={viewingModel !== null}
        onOpenChange={(open) => {
          if (!open) setViewingModel(null)
        }}
      />
    </>
  )
}
