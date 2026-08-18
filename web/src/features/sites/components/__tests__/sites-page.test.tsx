// Regression tests for the sites list page. Covers the three behaviors that
// previously regressed: the one-shot `?create=1` deep link, the error state
// suppressing the empty-state CTA, and the Retry button calling refetch.
import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SitesPage } from '../sites-page'

const testState = vi.hoisted(() => ({
  search: { create: false as boolean | undefined },
  navigate: vi.fn(),
  sitesQuery: {
    data: [],
    error: null as Error | null,
    isLoading: false,
    isFetching: false,
    refetch: vi.fn().mockResolvedValue({}),
  },
  formOpenCount: 0,
  previousFormOpen: undefined as boolean | undefined,
  dataTableRendered: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
  useSearch: () => testState.search,
}))

vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  // Sentinel component so the error-state test can assert the table page is
  // NOT rendered (and therefore the empty-state CTA inside it is absent).
  DataTablePage: () => {
    testState.dataTableRendered = true
    return null
  },
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    sorting: [],
    onSortingChange: vi.fn(),
    ensurePageInRange: vi.fn(),
  }),
  useDataTable: () => ({
    table: {
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
  encodeSorting: () => '',
}))

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({ data: { accounts: [] }, isLoading: false }),
}))

vi.mock('@/features/import', () => ({
  ImportWizardDialog: () => null,
}))

vi.mock('../../api', () => ({
  useSites: () => testState.sitesQuery,
  useDeleteSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSite: () => ({ mutate: vi.fn(), isPending: false }),
  useBatchUpdateSites: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDetectSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../site-created-modal', () => ({
  SiteCreatedModal: () => null,
}))

vi.mock('../site-detail-sheet', () => ({
  SiteDetailSheet: () => null,
}))

vi.mock('../site-form-dialog', () => ({
  SiteFormDialog: (props: { open: boolean }) => {
    if (props.open && testState.previousFormOpen !== true) {
      testState.formOpenCount += 1
    }
    testState.previousFormOpen = props.open
    return null
  },
}))

vi.mock('../sites-columns', () => ({
  SITES_STATUS_FILTER_OPTIONS: [],
  useSitesColumns: () => [],
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.navigate.mockReset()
  testState.sitesQuery.refetch.mockReset()
  testState.sitesQuery.refetch.mockResolvedValue({})
  testState.formOpenCount = 0
  testState.previousFormOpen = undefined
  testState.dataTableRendered = false
  testState.search = { create: false }
  testState.sitesQuery.data = []
  testState.sitesQuery.error = null
  testState.sitesQuery.isLoading = false
  testState.sitesQuery.isFetching = false
})

describe('SitesPage create deep link', () => {
  it('opens the create dialog once and strips the transient create param', async () => {
    testState.search = { create: true }
    const { rerender } = render(<SitesPage />)

    await waitFor(() => {
      expect(testState.formOpenCount).toBe(1)
    })

    // After consumption, the page navigates to strip `create`; a remount
    // (search.create now false) must not reopen the dialog.
    testState.search = { create: false }
    rerender(<SitesPage />)

    await waitFor(() => {
      expect(testState.formOpenCount).toBe(1)
    })
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith(
      expect.objectContaining({
        to: '/sites',
        replace: true,
        search: expect.objectContaining({ create: undefined }),
      })
    )
  })
})

describe('SitesPage error state', () => {
  it('renders the error banner and Retry button, not the empty-state CTA', () => {
    testState.sitesQuery.error = new Error('boom')
    render(<SitesPage />)

    expect(screen.getByText(/Failed to load sites: boom/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()

    // The table page (which owns the empty-state "Add site" CTA) must be
    // suppressed while the query is in an error state.
    expect(testState.dataTableRendered).toBe(false)
    expect(
      screen.queryByRole('button', { name: /Add site/ })
    ).not.toBeInTheDocument()
  })

  it('renders the table page (not the error banner) when there is no error', () => {
    testState.sitesQuery.error = null
    render(<SitesPage />)

    expect(testState.dataTableRendered).toBe(true)
    expect(screen.queryByText(/Failed to load sites/)).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('calls refetch when the Retry button is clicked', () => {
    testState.sitesQuery.error = new Error('boom')
    render(<SitesPage />)

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    expect(testState.sitesQuery.refetch).toHaveBeenCalledTimes(1)
  })
})
