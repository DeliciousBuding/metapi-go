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
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { RefreshCw as RefreshCwIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'

import { api } from '@/lib/api'

import { useModels } from '../api'
import { modelsSearchSchema } from '../lib/models-schema'
import type { ModelRow } from '../types'
import {
  buildBrandFilterOptions,
  buildCapabilityFilterOptions,
  useModelsColumns,
} from './models-columns'
import { ModelDetailSheet } from './model-detail-sheet'

const MODELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:models:column-visibility'
const MODELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:models:column-sizing'

type ResolvedSearch = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  brand: string[]
  capability: string[]
}

function resolveUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue,
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

function encodeSorting(sorting: SortingState): string {
  return sorting
    .map((item) => `${item.id}:${item.desc ? 'desc' : 'asc'}`)
    .join(',')
}

function encodeStringList(values: string[]): string {
  return values.join(',')
}

function readSearch(): ResolvedSearch {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      brand: [],
      capability: [],
    }
  }
  const params = new URLSearchParams(window.location.search)
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
      brand: [],
      capability: [],
    }
  }
  const data = parsed.data
  return {
    q: data.q ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: Array.isArray(data.sort) ? (data.sort as SortingState) : [],
    brand: data.brand ?? [],
    capability: data.capability ?? [],
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
  if (merged.brand.length > 0) params.set('brand', encodeStringList(merged.brand))
  if (merged.capability.length > 0)
    params.set('capability', encodeStringList(merged.capability))
  const queryString = params.toString()
  return queryString ? `/models?${queryString}` : '/models'
}

function useModelsUrlState() {
  const navigate = useNavigate()
  const search = readSearch()

  const columnFilters: ColumnFiltersState = useMemo(() => {
    const filters: ColumnFiltersState = []
    if (search.brand.length > 0) {
      filters.push({ id: 'brand', value: search.brand })
    }
    if (search.capability.length > 0) {
      filters.push({ id: 'capabilities', value: search.capability })
    }
    return filters
  }, [search.brand, search.capability])

  const onGlobalFilterChange = (updater: Updater<string>) => {
    const next = resolveUpdater(updater, search.q)
    navigate({ href: buildHref({ q: next }), replace: true })
  }
  const onPaginationChange = (updater: Updater<PaginationState>) => {
    const next = resolveUpdater(updater, {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    })
    navigate({
      href: buildHref({ pageIndex: next.pageIndex, pageSize: next.pageSize }),
      replace: true,
    })
  }
  const onSortingChange = (updater: Updater<SortingState>) => {
    const next = resolveUpdater(updater, search.sorting)
    navigate({ href: buildHref({ sorting: next }), replace: true })
  }
  const onColumnFiltersChange = (updater: Updater<ColumnFiltersState>) => {
    const next = resolveUpdater(updater, columnFilters)
    const brandEntry = next.find((filter) => filter.id === 'brand')
    const capabilityEntry = next.find((filter) => filter.id === 'capabilities')
    const brandValues = Array.isArray(brandEntry?.value)
      ? (brandEntry!.value as string[])
      : []
    const capabilityValues = Array.isArray(capabilityEntry?.value)
      ? (capabilityEntry!.value as string[])
      : []
    navigate({
      href: buildHref({ brand: brandValues, capability: capabilityValues }),
      replace: true,
    })
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
      navigate({ href: `/model-tester?${params.toString()}`, replace: true })
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
    onError: () => {
      toast.error(t('models.toast.refreshFailed'))
    },
  })

  // The refresh call re-aggregates the marketplace server-side; the next
  // render re-reads the (now fresh) `useModels` cache via its 10s staleTime,
  // so no explicit query invalidation is needed for the list to update.
  const models = modelsQuery.data ?? []
  const brandFilterOptions = useMemo(
    () => buildBrandFilterOptions(models),
    [models],
  )
  const capabilityFilterOptions = useMemo(
    () => buildCapabilityFilterOptions(models),
    [models],
  )

  return (
    <>
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
