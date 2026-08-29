// Behavior test for the models page empty-state CTA: the empty copy says
// models appear once accounts are connected, so the empty state must offer
// the exit to the accounts page instead of a dead end.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelsPage } from '../models-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@tanstack/react-query', () => ({
  useMutation: () => ({ mutate: vi.fn(), isPending: false }),
  useQueryClient: () => ({ invalidateQueries: vi.fn() }),
}))

vi.mock('@/components/data-table', () => ({
  // Render only the empty-state action slot so the test can assert the CTA.
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div>{props.emptyAction}</div>
  ),
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
  api: { getModelsMarketplace: vi.fn() },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useModelsPage: () => ({ data: { items: [], total: 0 }, isLoading: false, isFetching: false }),
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
})

afterEach(() => cleanup())

describe('ModelsPage empty state', () => {
  it('renders a manage-accounts CTA that navigates to /accounts', () => {
    render(<ModelsPage />)

    const cta = screen.getByRole('button', { name: /Manage accounts/ })
    expect(cta).toBeInTheDocument()

    fireEvent.click(cta)
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith({ to: '/accounts' })
  })
})
