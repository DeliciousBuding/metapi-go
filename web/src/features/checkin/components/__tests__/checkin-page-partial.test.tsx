// Partial-failure honesty for the "run all check-ins" header action.
//
// POST /api/checkin/trigger answers 200 with `success: (failed == 0)` plus a
// per-account summary. When some accounts fail, the page must surface the
// real breakdown (partialFailed toast + counts) instead of collapsing into a
// swallowed error with no feedback. Regression guard: the summary branch used
// to be dead code because the mutation threw on `success:false` before the
// result could reach it.

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

import { CheckinPage } from '../checkin-page'

const testState = vi.hoisted(() => ({
  navigate: vi.fn(),
  triggerCheckinAll: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
  toastWarning: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => testState.navigate,
}))

vi.mock('@/components/data-table', () => ({
  DataTablePage: (props: { emptyAction?: ReactNode }) => (
    <div data-testid='table-slot'>{props.emptyAction}</div>
  ),
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
    filters: {
      status: '',
      reason: '',
      site: '',
      accountId: '',
      from: '',
      to: '',
    },
    updateUrlState: vi.fn(),
  }),
}))

vi.mock('@/components/common/query-error-banner', () => ({
  QueryErrorBanner: () => null,
}))

vi.mock('@/features/accounts', () => ({
  useAccounts: () => ({
    data: { accounts: [{ id: 1, username: 'acc-1' }] },
  }),
}))

vi.mock('@/features/sites/api', () => ({
  useSites: () => ({ data: [] }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: testState.toastSuccess,
    error: testState.toastError,
    warning: testState.toastWarning,
    info: vi.fn(),
  },
}))

// Only the transport layer is mocked: the real useManualCheckin hook runs,
// so the envelope handling under test is the production code path.
vi.mock('@/lib/api', () => ({
  api: {
    getCheckinLogs: vi.fn().mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      pageSize: 20,
    }),
    triggerCheckinAll: (...args: unknown[]) =>
      testState.triggerCheckinAll(...args),
    triggerCheckin: vi.fn(),
  },
}))

vi.mock('../checkin-columns', () => ({
  useCheckinColumns: () => [],
}))

vi.mock('../checkin-detail-sheet', () => ({
  CheckinDetailSheet: () => null,
}))

vi.mock('../manual-checkin-dialog', () => ({
  ManualCheckinDialog: () => null,
}))

beforeEach(() => {
  testState.navigate.mockReset()
  testState.triggerCheckinAll.mockReset()
  testState.toastSuccess.mockReset()
  testState.toastError.mockReset()
  testState.toastWarning.mockReset()
})

afterEach(() => cleanup())

function renderCheckinPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <CheckinPage />
    </QueryClientProvider>
  )
}

describe('CheckinPage — run-all partial failure honesty', () => {
  it('surfaces the per-account breakdown when some accounts fail', async () => {
    testState.triggerCheckinAll.mockResolvedValue({
      success: false,
      queued: false,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 3, success: 1, failed: 2, skipped: 0 },
    })

    renderCheckinPage()

    fireEvent.click(
      await screen.findByRole('button', {
        name: /Run all check-ins|执行全部签到/,
      })
    )

    await waitFor(() => {
      expect(testState.toastError).toHaveBeenCalledTimes(1)
    })
    const [title, options] = testState.toastError.mock.calls[0] ?? []
    expect(String(title)).toMatch(/partially failed|部分失败/)
    expect(String((options as { description?: string })?.description)).toMatch(
      /1|2|3/
    )
    expect(testState.toastSuccess).not.toHaveBeenCalled()
  })

  it('reports success only when every account succeeded', async () => {
    testState.triggerCheckinAll.mockResolvedValue({
      success: true,
      queued: false,
      status: 'completed',
      message: '签到执行完成',
      summary: { total: 2, success: 2, failed: 0, skipped: 0 },
    })

    renderCheckinPage()

    fireEvent.click(
      await screen.findByRole('button', {
        name: /Run all check-ins|执行全部签到/,
      })
    )

    await waitFor(() => {
      expect(testState.toastSuccess).toHaveBeenCalledTimes(1)
    })
    expect(testState.toastError).not.toHaveBeenCalled()
  })
})
