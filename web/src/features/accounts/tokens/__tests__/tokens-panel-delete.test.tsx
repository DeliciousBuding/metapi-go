// Behavior test for the tokens-panel delete flow — S7 删除+undo 档:
// the row action no longer opens a dialog; it triggers the shared
// undoable-delete helper with the account's token-list query key. The
// helper's own contract (optimistic removal / undo restore / deferred
// commit) is pinned in lib/__tests__/undoable-delete.test.tsx.

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
  undoableDelete: vi.fn(),
  deleteAccountToken: vi.fn(),
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
  accountTokenQueryKeys: {
    all: ['account-tokens'] as const,
    list: (accountId?: number) =>
      ['account-tokens', 'list', accountId ?? 'all'] as const,
  },
  useAccountTokens: () => ({
    data: [mockState.sampleToken],
    isLoading: false,
  }),
  useCreateAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateAccountToken: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useSetDefaultAccountToken: () => ({ mutate: vi.fn(), isPending: false }),
  useSyncAccountTokens: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useToggleAccountTokenEnabled: () => ({ mutate: vi.fn(), isPending: false }),
}))

vi.mock('../../api', () => ({
  accountQueryKeys: { all: ['accounts'] as const },
}))

vi.mock('@/lib/api', () => ({
  api: {
    deleteAccountToken: mockState.deleteAccountToken,
  },
}))

vi.mock('@/lib/undoable-delete', () => ({
  useUndoableDelete: () => mockState.undoableDelete,
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
  mockState.undoableDelete.mockClear()
  mockState.deleteAccountToken.mockReset()
  mockState.deleteAccountToken.mockResolvedValue({ success: true })
})

afterEach(() => cleanup())

afterAll(() => {
  vi.restoreAllMocks()
})

describe('TokensPanel delete (undo tier)', () => {
  it('triggers the undoable delete with the token and its list query key — no dialog', async () => {
    render(<TokensPanel accountId={1} />)

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    // No confirmation dialog at all.
    expect(screen.queryByText('Delete token?')).not.toBeInTheDocument()

    expect(mockState.undoableDelete).toHaveBeenCalledTimes(1)
    const params = mockState.undoableDelete.mock.calls[0]?.[0] as {
      item: AccountToken
      queryKey: readonly unknown[]
      removeFromCache: (
        data: AccountToken[],
        item: AccountToken
      ) => AccountToken[]
      deleteFn: (item: AccountToken) => Promise<unknown>
      title: string
      undoLabel: string
      errorTitle: string
      alsoInvalidate: Array<readonly unknown[]>
    }
    expect(params.item.id).toBe(9)
    expect(params.queryKey).toEqual(['account-tokens', 'list', 1])
    expect(params.title).toBe('Token deleted')
    expect(params.undoLabel).toBe('Undo')
    expect(params.alsoInvalidate).toEqual([['account-tokens'], ['accounts']])

    // The pure cache reducer drops exactly this token.
    expect(
      params
        .removeFromCache(
          [mockState.sampleToken, { ...mockState.sampleToken, id: 10 }],
          mockState.sampleToken
        )
        .map((row) => row.id)
    ).toEqual([10])

    // The deferred DELETE goes to the real api with the token id.
    await params.deleteFn(mockState.sampleToken)
    expect(mockState.deleteAccountToken).toHaveBeenCalledWith(9)
  })
})
