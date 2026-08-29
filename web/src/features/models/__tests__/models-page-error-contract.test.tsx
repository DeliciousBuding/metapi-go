// Pins the unified list-page error contract on the models page (W19-T1 P2-o):
// a failed load REPLACES the table with the QueryErrorBanner instead of
// stacking over it, and its Retry re-fetches.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelsPage } from '../components/models-page'

const testState = vi.hoisted(() => ({
  modelsQuery: {
    // S9 server-side pagination: the page query carries { items, total }.
    data: { items: [] as unknown[], total: 0 },
    isLoading: false,
    isFetching: false,
    error: null as Error | null,
    refetch: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => <div data-testid='models-table' />,
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
  useDataTable: () => ({
    table: {
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
}))

vi.mock('../api', () => ({
  useModelsPage: () => testState.modelsQuery,
  useRefreshModels: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../components/model-detail-sheet', () => ({
  ModelDetailSheet: () => null,
}))

vi.mock('../components/model-verify-dialog', () => ({
  ModelVerifyDialog: () => null,
}))

beforeEach(() => {
  testState.modelsQuery = {
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }
  window.history.replaceState(null, '', '/models')
})

afterEach(() => {
  cleanup()
  window.history.replaceState(null, '', '/models')
})

function renderPage(): void {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  const ui: ReactElement = (
    <QueryClientProvider client={queryClient}>
      <ModelsPage />
    </QueryClientProvider>
  )
  render(ui)
}

describe('ModelsPage error contract', () => {
  it('replaces the table with the error banner on failure', () => {
    testState.modelsQuery.error = new Error('boom')
    renderPage()

    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('Failed to load models: boom')
    expect(screen.queryByTestId('models-table')).not.toBeInTheDocument()
  })

  it('banner Retry re-fetches the models query', () => {
    testState.modelsQuery.error = new Error('boom')
    renderPage()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(testState.modelsQuery.refetch).toHaveBeenCalledTimes(1)
  })

  it('renders the table without a banner when the load succeeds', () => {
    renderPage()

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByTestId('models-table')).toBeInTheDocument()
  })
})
