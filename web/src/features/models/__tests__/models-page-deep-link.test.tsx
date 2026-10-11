// Behavior test for the one-shot `?model=<name>` deep link on the models
// page (search-palette model hits link here): the name resolves through the
// exact-name detail endpoint — NOT by scanning the currently loaded page — so
// a model on any page opens exactly once, and a name with no current supply
// produces an explicit notice instead of vanishing silently. The transient
// param is stripped afterwards so a refetch or remount never reopens it.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, waitFor } from '@testing-library/react'
import type { ReactElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ModelsPage } from '../components/models-page'
import type { ModelRow } from '../types'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
  toastError: vi.fn(),
  // The deep-linked model is deliberately absent from the loaded page: the
  // resolver must come from the detail query, not from page items.
  detail: {
    data: undefined as { model: ModelRow } | undefined,
    isPending: false,
    error: null as Error | null,
  },
  detailSheetProps: null as { model: ModelRow | null; open: boolean } | null,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => null,
  encodeSorting: () => '',
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    filters: { capability: [], endpointType: [], sourceKind: [] },
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
}))

vi.mock('../api', () => ({
  useModelsPage: () => ({
    data: { items: [], total: 0, facets: null },
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useModelDetail: () => testState.detail,
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: (...args: unknown[]) => testState.toastError(...args),
    warning: vi.fn(),
    info: vi.fn(),
  },
}))

vi.mock('../components/model-detail-sheet', () => ({
  ModelDetailSheet: (props: { model: ModelRow | null; open: boolean }) => {
    testState.detailSheetProps = props
    return null
  },
}))

vi.mock('../components/model-verify-dialog', () => ({
  ModelVerifyDialog: () => null,
}))

vi.mock('../components/models-columns', () => ({
  useModelsColumns: () => [],
}))

const deepLinkedModel: ModelRow = {
  name: 'claude-opus-4.7.7.7',
  accountCount: 3,
  tokenCount: 10,
  avgLatency: null,
  successRate: null,
  description: null,
  tags: [],
  supportedEndpointTypes: [],
  pricingSources: [],
  accounts: [],
}

beforeEach(() => {
  testState.navigate.mockReset()
  testState.toastError.mockReset()
  testState.detailSheetProps = null
  testState.detail = { data: undefined, isPending: false, error: null }
  window.history.replaceState(null, '', '/models')
})

afterEach(() => {
  cleanup()
  window.history.replaceState(null, '', '/models')
})

// The page mounts hooks that reach a QueryClient (refresh mutation); render
// through a fresh provider with retries disabled.
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

describe('models page one-shot ?model= deep link', () => {
  it('opens the detail sheet for the referenced model and strips the param', async () => {
    testState.detail = {
      data: { model: deepLinkedModel },
      isPending: false,
      error: null,
    }
    window.history.replaceState(null, '', '/models?model=claude-opus-4.7.7.7')

    renderPage()

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    expect(testState.detailSheetProps?.model?.name).toBe('claude-opus-4.7.7.7')
    expect(testState.toastError).not.toHaveBeenCalled()
    // The transient param is stripped from the URL.
    expect(testState.navigate).toHaveBeenCalledWith({
      href: '/models',
      replace: true,
    })
  })

  it('waits for the detail query before resolving the link', async () => {
    testState.detail = { data: undefined, isPending: true, error: null }
    window.history.replaceState(null, '', '/models?model=claude-opus-4.7.7.7')

    renderPage()

    // Pending detail: nothing consumed yet, no navigation, no notice.
    expect(testState.detailSheetProps?.open).not.toBe(true)
    expect(testState.toastError).not.toHaveBeenCalled()
    expect(testState.navigate).not.toHaveBeenCalled()
  })

  it('shows an explicit notice for a model with no current supply', async () => {
    testState.detail = {
      data: undefined,
      isPending: false,
      error: new Error('model not found'),
    }
    window.history.replaceState(null, '', '/models?model=nope')

    renderPage()

    await waitFor(() => {
      expect(testState.toastError).toHaveBeenCalled()
    })
    const notice = String(testState.toastError.mock.calls[0][0])
    expect(notice.length).toBeGreaterThan(0)
    expect(testState.detailSheetProps?.open).not.toBe(true)
    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalledWith({
        href: '/models',
        replace: true,
      })
    })
  })

  it('keeps the other search params when stripping the deep link', async () => {
    testState.detail = {
      data: { model: deepLinkedModel },
      isPending: false,
      error: null,
    }
    window.history.replaceState(
      null,
      '',
      '/models?model=claude-opus-4.7.7.7&q=text&page=2'
    )

    renderPage()

    await waitFor(() => {
      expect(testState.detailSheetProps?.model?.name).toBe(
        'claude-opus-4.7.7.7'
      )
    })
    expect(testState.navigate).toHaveBeenCalledWith({
      href: '/models?q=text&page=2',
      replace: true,
    })
  })
})
