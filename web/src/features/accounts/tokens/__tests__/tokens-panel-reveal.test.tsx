// Behavior test for the upstream-token reveal flow (#1034).
//
// The token list only carries the masked form. Plaintext is fetched on an
// explicit eye click, behind master-token re-confirmation, and shown only for
// that row's session — never persisted or pre-fetched.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { ReauthProvider } from '@/components/common/reauth-provider'
import i18n from '@/i18n/config'

import type { AccountToken } from '../../types'
import { TokensPanel } from '../components/tokens-panel'

const mockState = vi.hoisted(() => ({
  getAccountTokenValue: vi.fn(),
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
    error: null,
    refetch: vi.fn(),
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

vi.mock('@/lib/api', () => ({
  api: {
    getAccountTokenValue: mockState.getAccountTokenValue,
    deleteAccountToken: vi.fn(),
  },
}))

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

beforeEach(() => {
  mockState.getAccountTokenValue.mockReset()
  mockState.sampleToken.valueStatus = 'normal'
})

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

function renderPanel() {
  return render(
    <ReauthProvider>
      <TokensPanel accountId={1} />
    </ReauthProvider>
  )
}

describe('TokensPanel — explicit token reveal', () => {
  it('prompts for the master token, then shows the plaintext', async () => {
    mockState.getAccountTokenValue.mockResolvedValueOnce({
      success: true,
      id: 9,
      name: 'relay key',
      token: 'sk-live-plain-123',
      tokenMasked: 'sk-…abc',
    })
    renderPanel()

    expect(screen.getByText('sk-…abc')).toBeInTheDocument()
    // Nothing is fetched before the operator asks for it.
    expect(mockState.getAccountTokenValue).not.toHaveBeenCalled()

    fireEvent.click(
      screen.getByRole('button', { name: text('common.revealSecret') })
    )

    // The click asks for the master token first; the plaintext request is
    // only sent after confirmation.
    expect(
      await screen.findByLabelText(text('reauth.tokenLabel'))
    ).toBeVisible()
    expect(mockState.getAccountTokenValue).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText(text('reauth.tokenLabel')), {
      target: { value: 'master-token' },
    })
    fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))

    await waitFor(() =>
      expect(mockState.getAccountTokenValue).toHaveBeenCalledTimes(1)
    )
    expect(mockState.getAccountTokenValue).toHaveBeenCalledWith(
      9,
      'master-token'
    )
    expect(await screen.findByText('sk-live-plain-123')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: text('common.hideSecret') })
    ).toBeInTheDocument()
  })

  it('keeps the inline error when the token is rejected', async () => {
    mockState.getAccountTokenValue.mockRejectedValue(reauthError())
    renderPanel()

    fireEvent.click(
      screen.getByRole('button', { name: text('common.revealSecret') })
    )
    const input = await screen.findByLabelText(text('reauth.tokenLabel'))
    fireEvent.change(input, { target: { value: 'wrong-token' } })
    fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))

    expect(await screen.findByText(text('reauth.errorInvalid'))).toBeVisible()
    expect(screen.queryByText('sk-live-plain-123')).toBeNull()
  })

  it('does not offer a reveal for a token that only has a masked value', () => {
    mockState.sampleToken.valueStatus = 'masked_pending'
    renderPanel()

    expect(
      screen.queryByRole('button', { name: text('common.revealSecret') })
    ).toBeNull()
    expect(mockState.getAccountTokenValue).not.toHaveBeenCalled()
  })
})
