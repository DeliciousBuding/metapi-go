// Behavior test for the tokens-panel delete flow (#889): the confirmation
// used to close immediately after firing the mutation ("先关后删"), hiding
// the pending state. The dialog must now stay open — Cancel disabled, confirm
// spinner — until the mutation settles, and close on success.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import type { AccountToken } from '../../types'
import { TokensPanel } from '../components/tokens-panel'

const mockState = vi.hoisted(() => ({
  deleteCalls: [] as Array<{ id: number; onSuccess?: () => void }>,
  deletePending: { value: false },
  sampleToken: {
    id: 9,
    accountId: 1,
    name: 'relay key',
    token: '',
    tokenMasked: 'sk-…abc',
    tokenGroup: null,
    valueStatus: 'normal',
    source: '',
    enabled: true,
    isDefault: false,
    createdAt: '',
    updatedAt: '',
  } as AccountToken,
}))

vi.mock('../api', () => ({
  useAccountTokens: () => ({
    data: [mockState.sampleToken],
    isLoading: false,
  }),
  useCreateAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteAccountToken: () => ({
    mutate: (id: number, options?: { onSuccess?: () => void }) => {
      mockState.deletePending.value = true
      mockState.deleteCalls.push({ id, onSuccess: options?.onSuccess })
    },
    isPending: mockState.deletePending.value,
  }),
  useSetDefaultAccountToken: () => ({ mutate: vi.fn(), isPending: false }),
  useSyncAccountTokens: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useToggleAccountTokenEnabled: () => ({ mutate: vi.fn(), isPending: false }),
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
  mockState.deleteCalls = []
  mockState.deletePending.value = false
})

afterEach(() => cleanup())

afterAll(() => {
  vi.restoreAllMocks()
})

describe('TokensPanel delete dialog', () => {
  it('stays open while the delete is pending and closes on success', () => {
    const { rerender } = render(<TokensPanel accountId={1} />)

    // Open the delete confirmation from the row action.
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(screen.getByText('Delete token?')).toBeInTheDocument()

    // Confirm — the mutation fires but the dialog must stay open.
    const deleteButtons = screen.getAllByRole('button', { name: 'Delete' })
    const confirmButton = deleteButtons.at(-1)
    if (!confirmButton) throw new Error('confirm button not rendered')
    fireEvent.click(confirmButton)

    expect(mockState.deleteCalls).toHaveLength(1)
    expect(mockState.deleteCalls[0]?.id).toBe(9)

    // Re-render so the hook picks up the pending flag (TanStack Query would
    // trigger this re-render in production).
    rerender(<TokensPanel accountId={1} />)
    expect(screen.getByText('Delete token?')).toBeInTheDocument()
    // While pending, Cancel is disabled so the dialog can't be dismissed.
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()

    // Settle the mutation — the dialog closes.
    mockState.deletePending.value = false
    mockState.deleteCalls[0]?.onSuccess?.()
    rerender(<TokensPanel accountId={1} />)
    expect(screen.queryByText('Delete token?')).toBeNull()
  })
})
