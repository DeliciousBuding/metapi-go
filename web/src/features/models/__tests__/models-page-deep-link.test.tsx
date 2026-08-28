// Behavior test for the one-shot `?model=<name>` deep link on the models
// page (search-palette model hits link here): the referenced model opens in
// the detail sheet exactly once, then the transient param is stripped so a
// refetch or remount never reopens it. Mirrors the accounts page's
// `accountId` deep-link test.

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
  models: [
    {
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
    },
    {
      name: 'gpt-5.5',
      accountCount: 2,
      tokenCount: 5,
      avgLatency: null,
      successRate: null,
      description: null,
      tags: [],
      supportedEndpointTypes: [],
      pricingSources: [],
      accounts: [],
    },
  ] as Array<Partial<ModelRow>>,
  detailSheetProps: null as { model: ModelRow | null; open: boolean } | null,
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: () => null,
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
  useModels: () => ({
    data: testState.models,
    isLoading: false,
    isFetching: false,
    error: null,
  }),
  useRefreshModels: () => ({ mutateAsync: vi.fn(), isPending: false }),
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

beforeEach(() => {
  testState.navigate.mockReset()
  testState.detailSheetProps = null
  window.history.replaceState(null, '', '/models')
})

afterEach(() => {
  cleanup()
  window.history.replaceState(null, '', '/models')
})

// The page mounts hooks that reach a QueryClient (capability selectors);
// render through a fresh provider with retries disabled.
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
    window.history.replaceState(null, '', '/models?model=claude-opus-4.7.7.7')

    renderPage()

    await waitFor(() => {
      expect(testState.detailSheetProps?.open).toBe(true)
    })
    expect(testState.detailSheetProps?.model?.name).toBe('claude-opus-4.7.7.7')
    // The transient param is stripped from the URL.
    expect(testState.navigate).toHaveBeenCalledWith({
      href: '/models',
      replace: true,
    })
  })

  it('clears the param silently for an unknown model name', async () => {
    window.history.replaceState(null, '', '/models?model=nope')

    renderPage()

    await waitFor(() => {
      expect(testState.navigate).toHaveBeenCalledWith({
        href: '/models',
        replace: true,
      })
    })
    expect(testState.detailSheetProps?.open).not.toBe(true)
  })

  it('keeps the other search params when stripping the deep link', async () => {
    window.history.replaceState(null, '', '/models?model=gpt-5.5&q=text&page=2')

    renderPage()

    await waitFor(() => {
      expect(testState.detailSheetProps?.model?.name).toBe('gpt-5.5')
    })
    expect(testState.navigate).toHaveBeenCalledWith({
      href: '/models?q=text&page=2',
      replace: true,
    })
  })
})
