import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
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
  tokens: [] as AccountToken[],
  create: vi.fn(),
  update: vi.fn(),
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
  useAccountTokens: () => ({ data: mockState.tokens, isLoading: false }),
  useCreateAccountToken: () => ({
    mutateAsync: mockState.create,
    isPending: false,
  }),
  useUpdateAccountToken: () => ({
    mutateAsync: mockState.update,
    isPending: false,
  }),
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
afterAll(() => vi.restoreAllMocks())

const text = (key: string) => String(i18n.t(key))

beforeEach(() => {
  mockState.tokens = []
  mockState.create.mockReset().mockResolvedValue({ success: true })
  mockState.update.mockReset().mockResolvedValue({ success: true })
})

describe('account token onboarding', () => {
  it('submits an upstream creation without requiring a pasted value', async () => {
    render(<TokensPanel accountId={7} />)
    fireEvent.click(
      screen.getByRole('button', { name: text('accounts.tokens.add') })
    )
    fireEvent.change(screen.getByLabelText(text('accounts.tokens.form.name')), {
      target: { value: 'upstream relay' },
    })
    expect(
      screen.getByText(text('accounts.tokens.form.createValueHint'))
    ).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: text('accounts.tokens.form.create') })
    )
    await waitFor(() => expect(mockState.create).toHaveBeenCalledTimes(1))
    expect(mockState.create.mock.calls[0][0]).toMatchObject({
      accountId: 7,
      name: 'upstream relay',
      group: 'default',
      unlimitedQuota: true,
    })
    expect(mockState.create.mock.calls[0][0]).not.toHaveProperty('token')
  })

  it('can edit metadata while leaving the stored secret unchanged', async () => {
    mockState.tokens = [makeToken()]
    render(<TokensPanel accountId={1} />)
    fireEvent.click(screen.getByTitle(text('accounts.tokens.edit')))
    fireEvent.change(screen.getByLabelText(text('accounts.tokens.form.name')), {
      target: { value: 'renamed relay' },
    })
    expect(
      screen.queryByLabelText(text('accounts.tokens.form.quota'))
    ).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: text('common.save') }))
    await waitFor(() => expect(mockState.update).toHaveBeenCalledTimes(1))
    expect(mockState.update).toHaveBeenCalledWith({
      id: 9,
      payload: { name: 'renamed relay', group: 'default' },
    })
  })

  it('hides non-operative limits when importing an existing value', () => {
    render(<TokensPanel accountId={7} />)
    fireEvent.click(
      screen.getByRole('button', { name: text('accounts.tokens.add') })
    )
    expect(
      screen.getByLabelText(text('accounts.tokens.form.quota'))
    ).toBeInTheDocument()
    fireEvent.change(
      screen.getByLabelText(text('accounts.tokens.form.value')),
      {
        target: { value: 'existing-relay-value' },
      }
    )
    expect(
      screen.queryByLabelText(text('accounts.tokens.form.quota'))
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText(text('accounts.tokens.form.expiresAt'))
    ).not.toBeInTheDocument()
    expect(
      screen.queryByLabelText(text('accounts.tokens.form.allowedIps'))
    ).not.toBeInTheDocument()
  })
})
