import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { encodeSorting, useDataTable } from '@/components/data-table'

import { useModelsPage } from '../api'
import type { ModelRow } from '../types'
import { useModelsColumns } from './models-columns'
import { useModelsUrlState } from './use-models-url-state'

const MODELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:models:column-visibility'
const MODELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:models:column-sizing'

export function useModelsTable(onView: (model: ModelRow) => void) {
  const urlState = useModelsUrlState()
  const navigate = useNavigate()

  // URL-owned state is the query contract: any search/filter/sort change
  // re-keys the page query and is answered over the whole marketplace.
  const sortParam = encodeSorting(urlState.sorting)
  const modelsPageQuery = useModelsPage({
    pageIndex: urlState.pagination.pageIndex,
    pageSize: urlState.pagination.pageSize,
    includePricing: true,
    q: urlState.globalFilter,
    capabilities: urlState.filters.capability,
    endpointTypes: urlState.filters.endpointType,
    sourceKinds: urlState.filters.sourceKind,
    sort: sortParam,
  })

  const columnActions = useMemo(
    () => ({
      onView,
      onTest: (model: ModelRow) => {
        const params = new URLSearchParams({ model: model.name })
        navigate({ href: `/model-tester?${params.toString()}` })
      },
    }),
    [navigate, onView]
  )
  const columns = useModelsColumns(columnActions)

  const { table } = useDataTable<ModelRow>({
    data: modelsPageQuery.data?.items ?? [],
    columns,
    // Fully server-driven: pagination, filtering, and sorting are all the
    // backend's job; the table renders exactly the page it was given.
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    totalCount: modelsPageQuery.data?.total ?? 0,
    enableColumnResizing: true,
    columnVisibilityStorageKey: MODELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: MODELS_COLUMN_SIZING_STORAGE_KEY,
    // sourceKinds exists to back the toolbar's supply-source faceted filter;
    // it stays hidden until the operator reveals it from View Options.
    initialColumnVisibility: { sourceKinds: false },
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
  return { table, modelsPageQuery }
}
