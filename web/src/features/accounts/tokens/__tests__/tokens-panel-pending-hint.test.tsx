// A token Metapi could not hydrate stays unusable, and the badge used to say
// only "Pending" — which reads as "wait". Operators waited; the token never
// became relayable, because the missing value is the upstream's masked display
// value and the fix is the sync action that already sits on the same account.
// These tests pin that the badge names the action, and that a healthy token
// does not carry the warning.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import i18n from 'i18next'
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
  error: null as Error | null,
  refetch: vi.fn(),
  tokens: [] as AccountToken[],
}))

function makeToken(overrides: Partial<AccountToken> = {}): AccountToken {
  return {
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
    ...overrides,
  } as AccountToken
}

vi.mock('../api', () => ({
  accountTokenQueryKeys: {
    all: ['account-tokens'] as const,
    list: (accountId?: number) =>
      ['account-tokens', 'list', accountId ?? 'all'] as const,
  },
  useAccountTokens: () => ({
    data: mockState.tokens,
    isLoading: false,
    error: mockState.error,
    refetch: mockState.refetch,
    isFetching: false,
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

vi.mock('@/lib/api', () => ({ api: { deleteAccountToken: vi.fn() } }))

vi.mock('@/lib/undoable-delete', () => ({ useUndoableDelete: () => vi.fn() }))

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

afterEach(() => cleanup())
beforeEach(async () => {
  await i18n.changeLanguage('en')
  mockState.error = null
  mockState.tokens = []
  mockState.refetch.mockReset()
})
afterAll(() => vi.restoreAllMocks())

const hint = () => String(i18n.t('accounts.tokens.pendingCompleteHint'))

describe('TokensPanel masked-pending guidance', () => {
  it('hands off an existing key by ID without revealing it', () => {
    mockState.tokens = [makeToken()]
    const configure = vi.fn()
    render(<TokensPanel accountId={1} onConfigureRoutes={configure} />)
    fireEvent.click(
      screen.getByRole('button', { name: 'Use this key in routes' })
    )
    expect(configure).toHaveBeenCalledExactlyOnceWith(9)
  })

  it.each([{ valueStatus: 'masked_pending' as const }, { enabled: false }])(
    'does not offer an unusable key (%j)',
    (overrides) => {
      mockState.tokens = [makeToken(overrides)]
      const configure = vi.fn()
      render(<TokensPanel accountId={1} onConfigureRoutes={configure} />)
      const button = screen.getByRole('button', {
        name: 'Use this key in routes',
      })
      expect(button).toBeDisabled()
      fireEvent.click(button)
      expect(configure).not.toHaveBeenCalled()
    }
  )

  it('shows an actionable load failure instead of an empty inventory', () => {
    mockState.error = new Error('private server diagnostic')
    render(<TokensPanel accountId={1} />)
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load API keys'
    )
    expect(
      screen.queryByText('private server diagnostic')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText(String(i18n.t('accounts.tokens.empty')))
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mockState.refetch).toHaveBeenCalledOnce()
  })

  it('does not offer a stale key after a failed refresh', () => {
    mockState.tokens = [makeToken()]
    mockState.error = new Error('refresh failed')
    render(<TokensPanel accountId={1} onConfigureRoutes={vi.fn()} />)
    expect(screen.getByText('relay key')).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'Use this key in routes' })
    ).not.toBeInTheDocument()
  })

  it('tells the operator which action completes an unusable token', () => {
    mockState.tokens = [makeToken({ valueStatus: 'masked_pending' })]

    render(<TokensPanel accountId={1} />)

    const badge = screen.getByTitle(hint())
    expect(badge).toBeInTheDocument()
    expect(badge).toHaveTextContent(
      String(i18n.t('accounts.tokens.pendingComplete'))
    )
  })

  it('does not warn about a token that hydrated normally', () => {
    mockState.tokens = [makeToken({ valueStatus: 'normal' })]

    render(<TokensPanel accountId={1} />)

    expect(screen.queryByTitle(hint())).not.toBeInTheDocument()
  })
})
