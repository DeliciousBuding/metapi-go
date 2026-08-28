// Behavior tests for the account Models panel (#998): honest source and
// availability rendering, manual refresh action, manual add, explicit
// removal limited to manual rows, and loading/error/empty states. The panel
// renders real; only the feature's query/mutation hooks are mocked.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import type { AccountModelEntry, AccountModelsResponse } from '../api'
import { AccountModelsPanel } from '../components/account-models-panel'

const mockState = vi.hoisted(() => ({
  query: {
    data: undefined as AccountModelsResponse | undefined,
    isLoading: false,
    isError: false,
    isFetching: false,
  },
  refetch: vi.fn(),
  refreshMutate: vi.fn(),
  refreshPending: { value: false },
  refreshError: null as Error | null,
  manualMutate: vi.fn(),
  manualPending: { value: false },
  manualError: null as Error | null,
}))

vi.mock('../api', () => ({
  useAccountModels: () => ({
    ...mockState.query,
    refetch: mockState.refetch,
  }),
  useRefreshAccountModels: () => ({
    mutate: mockState.refreshMutate,
    isPending: mockState.refreshPending.value,
    isError: mockState.refreshError !== null,
    error: mockState.refreshError,
  }),
  useUpdateManualModels: () => ({
    mutate: mockState.manualMutate,
    isPending: mockState.manualPending.value,
    isError: mockState.manualError !== null,
    error: mockState.manualError,
  }),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    warning: vi.fn(),
  },
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
  mockState.query.data = undefined
  mockState.query.isLoading = false
  mockState.query.isError = false
  mockState.query.isFetching = false
  mockState.refetch.mockClear()
  mockState.refreshMutate.mockClear()
  mockState.refreshPending.value = false
  mockState.refreshError = null
  mockState.manualMutate.mockClear()
  mockState.manualPending.value = false
  mockState.manualError = null
})

afterEach(() => cleanup())

function entry(overrides: Partial<AccountModelEntry>): AccountModelEntry {
  return {
    name: 'model',
    available: true,
    latencyMs: null,
    disabled: false,
    isManual: false,
    checkedAt: null,
    ...overrides,
  }
}

describe('AccountModelsPanel', () => {
  it('shows a loading state before data arrives', () => {
    mockState.query.isLoading = true
    render(<AccountModelsPanel accountId={1} />)
    expect(screen.getByText('Loading models…')).toBeInTheDocument()
  })

  it('renders honest source and availability state per row', () => {
    mockState.query.data = {
      siteId: 2,
      siteName: 'OpenAI',
      totalCount: 3,
      models: [
        entry({ name: 'gpt-5.5', checkedAt: '2026-08-25T00:00:00.000Z' }),
        entry({ name: 'manual-x', isManual: true }),
        entry({ name: 'stale-y', available: false }),
      ],
    }
    render(<AccountModelsPanel accountId={1} />)

    expect(screen.getByText('gpt-5.5')).toBeInTheDocument()
    expect(screen.getByText('manual-x')).toBeInTheDocument()
    expect(screen.getByText('stale-y')).toBeInTheDocument()
    // Source + availability badges appear exactly once each.
    expect(screen.getAllByText('Manual')).toHaveLength(1)
    expect(screen.getAllByText('Unavailable')).toHaveLength(1)
    // Explicit removal is offered only for manual rows.
    expect(
      screen.getByRole('button', { name: 'Remove manual-x' })
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Remove gpt-5.5' })
    ).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Remove stale-y' })
    ).not.toBeInTheDocument()
  })

  it('shows the empty state when nothing is persisted yet', () => {
    mockState.query.data = { totalCount: 0, models: [] }
    render(<AccountModelsPanel accountId={1} />)
    expect(
      screen.getByText(
        'No models yet — refresh from upstream or add one manually.'
      )
    ).toBeInTheDocument()
  })

  it('surfaces load failures with a retry action', () => {
    mockState.query.isError = true
    render(<AccountModelsPanel accountId={1} />)
    expect(screen.getByRole('alert')).toHaveTextContent('Failed to load models')
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockState.refetch).toHaveBeenCalledTimes(1)
  })

  it('fires the upstream refresh action and shows honest failure inline', () => {
    const { rerender } = render(<AccountModelsPanel accountId={1} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Refresh from upstream' })
    )
    expect(mockState.refreshMutate).toHaveBeenCalledTimes(1)

    mockState.refreshError = new Error('model fetch failed: API key is invalid')
    rerender(<AccountModelsPanel accountId={1} />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Refresh failed: model fetch failed: API key is invalid'
    )
  })

  it('adds a manual model with the typed name', () => {
    mockState.query.data = { totalCount: 0, models: [] }
    render(<AccountModelsPanel accountId={1} />)

    const input = screen.getByRole('textbox', {
      name: 'New manual model name',
    })
    fireEvent.change(input, { target: { value: 'gpt-5-custom' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add' }))

    expect(mockState.manualMutate).toHaveBeenCalledTimes(1)
    const [payload] = mockState.manualMutate.mock.calls[0]
    expect(payload).toEqual({ models: ['gpt-5-custom'] })
  })

  it('removes a manual row explicitly', () => {
    mockState.query.data = {
      totalCount: 2,
      models: [
        entry({ name: 'gpt-5.5' }),
        entry({ name: 'manual-x', isManual: true }),
      ],
    }
    render(<AccountModelsPanel accountId={1} />)

    fireEvent.click(screen.getByRole('button', { name: 'Remove manual-x' }))
    expect(mockState.manualMutate).toHaveBeenCalledTimes(1)
    const [payload] = mockState.manualMutate.mock.calls[0]
    expect(payload).toEqual({ remove: ['manual-x'] })
  })
})
