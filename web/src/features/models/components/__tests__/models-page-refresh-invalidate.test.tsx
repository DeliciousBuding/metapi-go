// Regression test: refreshing the marketplace re-aggregates server-side, but
// the list query's 10s staleTime kept serving the pre-refresh cache during
// that window. The refresh mutation must invalidate the models query prefix
// so the table refetches immediately (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelsPage } from '../models-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
  mockGetModelsMarketplace: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/data-table', () => ({
  // Surface the toolbar's preActions so the refresh button is clickable.
  DataTablePage: (props: {
    toolbarProps?: { preActions?: ReactNode }
    emptyAction?: ReactNode
  }) => <div>{props.toolbarProps?.preActions ?? props.emptyAction}</div>,
  encodeSorting: () => '',
  useDataTable: () => ({ table: {} }),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
}))

vi.mock('@/components/common/query-error-banner', () => ({
  QueryErrorBanner: () => null,
}))

vi.mock('@/lib/api', () => ({
  api: { getModelsMarketplace: testState.mockGetModelsMarketplace },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useModels: () => ({ data: [], isLoading: false, isFetching: false }),
}))

vi.mock('../model-detail-sheet', () => ({
  ModelDetailSheet: () => null,
}))

vi.mock('../models-columns', () => ({
  useModelsColumns: () => [],
  buildBrandFilterOptions: () => [],
  buildCapabilityFilterOptions: () => [],
  buildEndpointTypeFilterOptions: () => [],
}))

beforeEach(() => {
  testState.navigate.mockReset()
  testState.mockGetModelsMarketplace.mockReset()
  testState.mockGetModelsMarketplace.mockResolvedValue({ models: [] })
})

afterEach(() => cleanup())

function renderModelsPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  render(
    <QueryClientProvider client={queryClient}>
      <ModelsPage />
    </QueryClientProvider>
  )
  return { invalidateSpy }
}

describe('ModelsPage marketplace refresh', () => {
  it('invalidates the models queries after a successful refresh', async () => {
    const { invalidateSpy } = renderModelsPage()

    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    await waitFor(() => {
      expect(testState.mockGetModelsMarketplace).toHaveBeenCalledTimes(1)
    })
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['models'] })
    })
  })
})
