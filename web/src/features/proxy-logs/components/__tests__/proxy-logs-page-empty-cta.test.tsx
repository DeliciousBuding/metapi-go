// Regression test: the proxy-logs empty state was a dead end — users staring
// at "no logs" had no path forward. It must offer a "View routes" CTA that
// navigates to the token-routes page (audit #1029 batch B).
import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ProxyLogsPage } from '../proxy-logs-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/common/query-error-banner', () => ({
  QueryErrorBanner: () => null,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div>{props.emptyAction}</div>
  ),
  useUrlTableState: () => ({
    globalFilter: '',
    onGlobalFilterChange: vi.fn(),
    columnFilters: [],
    onColumnFiltersChange: vi.fn(),
    pagination: { pageIndex: 0, pageSize: 20 },
    onPaginationChange: vi.fn(),
    filters: {
      status: '',
      siteId: '',
      channelId: '',
      client: '',
      from: '',
      to: '',
      latencyMin: '',
      latencyMax: '',
    },
  }),
  useDataTable: () => ({ table: {} }),
}))

vi.mock('@/lib/api', () => ({
  api: { getProxyLogsQuery: vi.fn() },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
}))

vi.mock('../../api', () => ({
  useProxyLogs: () => ({
    data: { items: [], total: 0 },
    isLoading: false,
    isFetching: false,
    refetch: vi.fn(),
  }),
  useProxyLogsMeta: () => ({
    data: undefined,
    isFetching: false,
    refetch: vi.fn(),
  }),
}))

vi.mock('../../lib/use-proxy-logs-auto-refresh', () => ({
  useProxyLogsAutoRefresh: () => ({ intervalMs: false, setIntervalMs: vi.fn() }),
}))

vi.mock('../proxy-logs-header-actions', () => ({
  ProxyLogsHeaderActions: () => null,
}))

vi.mock('../proxy-log-detail-sheet', () => ({
  ProxyLogDetailSheet: () => null,
}))

vi.mock('../proxy-logs-auto-refresh-toggle', () => ({
  ProxyLogsAutoRefreshToggle: () => null,
}))

vi.mock('../proxy-logs-columns', () => ({
  useProxyLogsColumns: () => [],
}))

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

beforeEach(() => {
  testState.navigate.mockReset()
})

afterEach(() => cleanup())

describe('ProxyLogsPage empty-state CTA', () => {
  it('offers a View routes CTA that navigates to /token-routes', () => {
    render(<ProxyLogsPage />)

    const cta = screen.getByRole('button', { name: 'View routes' })
    expect(cta).toBeInTheDocument()

    fireEvent.click(cta)
    expect(testState.navigate).toHaveBeenCalledTimes(1)
    expect(testState.navigate).toHaveBeenCalledWith({ to: '/token-routes' })
  })
})
