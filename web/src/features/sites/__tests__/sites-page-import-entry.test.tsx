// Pins the import-wizard toolbar entry on the sites page. The wizard used to
// be reachable only from the DataTablePage empty-state CTA — once the first
// site existed the empty state stopped rendering and the only batch
// site+account path became unreachable. The toolbar's preActions must keep a
// permanent Import entry that opens the same (already mounted) wizard.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { SitesPage } from '../components/sites-page'

const testState = vi.hoisted(() => ({
  importOpen: false,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useSearch: () => ({}),
}))

// Render only the toolbar preActions slot: the regression is about toolbar
// reachability with a NON-EMPTY list, where the empty-state CTA never shows.
vi.mock('@/components/data-table', () => ({
  DataTableBulkActions: () => null,
  DataTablePage: (props: {
    toolbarProps?: { preActions?: ReactNode } | null
  }) => <div>{props.toolbarProps?.preActions}</div>,
  encodeSorting: () => '',
  useDataTable: () => ({
    table: {
      getFilteredSelectedRowModel: () => ({ rows: [] }),
      resetRowSelection: vi.fn(),
    },
  }),
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
}))

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({ data: undefined }),
}))

vi.mock('@/features/import', () => ({
  ImportWizardDialog: (props: { open: boolean }) => {
    testState.importOpen = props.open
    return null
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), warning: vi.fn(), error: vi.fn() },
}))

vi.mock('../api', () => ({
  // Non-empty library: this is exactly the state where the empty-state CTA
  // disappears and the toolbar entry is the only way into the wizard.
  useSites: () => ({
    data: [
      {
        id: 1,
        name: 'Primary site',
        url: 'https://primary.example',
        platform: 'openai',
        status: 'active',
      },
    ],
    isLoading: false,
    isFetching: false,
    error: null,
    refetch: vi.fn(),
  }),
  useDeleteSite: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateSite: () => ({ mutate: vi.fn(), isPending: false, variables: null }),
  useBatchUpdateSites: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

vi.mock('../components/site-form-sheet', () => ({
  SiteFormSheet: () => null,
}))

vi.mock('../components/site-detail-sheet', () => ({
  SiteDetailSheet: () => null,
}))

vi.mock('../components/site-created-modal', () => ({
  SiteCreatedModal: () => null,
}))

vi.mock('../components/sites-columns', () => ({
  useSitesColumns: () => [],
  SITES_STATUS_FILTER_OPTIONS: [],
}))

afterEach(() => cleanup())

beforeEach(() => {
  testState.importOpen = false
})

const IMPORT_BUTTON_NAME = 'Import sites'

describe('SitesPage toolbar import entry', () => {
  it('renders an Import button in the toolbar when the list is non-empty', () => {
    render(<SitesPage />)

    expect(
      screen.getByRole('button', { name: IMPORT_BUTTON_NAME })
    ).toBeInTheDocument()
  })

  it('opens the import wizard from the toolbar button', () => {
    render(<SitesPage />)

    fireEvent.click(screen.getByRole('button', { name: IMPORT_BUTTON_NAME }))

    expect(testState.importOpen).toBe(true)
  })
})
