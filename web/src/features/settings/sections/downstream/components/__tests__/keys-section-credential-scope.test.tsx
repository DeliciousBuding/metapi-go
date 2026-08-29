// End-to-end behavior for the downstream key credential-ref form wiring
// (#1026 UI follow-up): GET strings parse into form state, submit serializes
// real arrays, empty dimensions send [] (unrestricted), and backend 400
// messages reach the operator.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import type { ReactElement } from 'react'
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
import { Sheet, SheetContent } from '@/components/ui/sheet'

import { KeySheetForm } from '../keys-section'

const {
  mockCreateKey,
  mockUpdateKey,
  mockGetSites,
  mockGetAccountsSnapshot,
  mockGetAccountTokens,
  mockToastError,
  mockToastSuccess,
} = vi.hoisted(() => ({
  mockCreateKey: vi.fn(),
  mockUpdateKey: vi.fn(),
  mockGetSites: vi.fn(),
  mockGetAccountsSnapshot: vi.fn(),
  mockGetAccountTokens: vi.fn(),
  mockToastError: vi.fn(),
  mockToastSuccess: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    createDownstreamApiKey: mockCreateKey,
    updateDownstreamApiKey: mockUpdateKey,
    getSites: mockGetSites,
    getAccountsSnapshot: mockGetAccountsSnapshot,
    getAccountTokens: mockGetAccountTokens,
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: mockToastSuccess, error: mockToastError },
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
  mockCreateKey.mockReset()
  mockUpdateKey.mockReset()
  mockGetSites.mockReset()
  mockGetAccountsSnapshot.mockReset()
  mockGetAccountTokens.mockReset()
  mockToastError.mockReset()
  mockToastSuccess.mockReset()
  mockGetSites.mockResolvedValue([{ id: 1, name: 'Alpha' }])
  mockGetAccountsSnapshot.mockResolvedValue({
    generatedAt: '2026-08-29T00:00:00Z',
    accounts: [
      { id: 11, siteId: 1, username: 'alice', apiTokenMasked: 'sk-***a' },
    ],
    sites: [{ id: 1, name: 'Alpha' }],
  })
  mockGetAccountTokens.mockResolvedValue([
    { id: 101, accountId: 11, name: 'Token A', tokenMasked: 'sk-***t' },
  ])
  mockCreateKey.mockResolvedValue({})
  mockUpdateKey.mockResolvedValue({})
})

afterEach(() => cleanup())

function renderKeySheetForm(
  editingKey: Parameters<typeof KeySheetForm>[0]['editingKey']
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <Sheet open onOpenChange={() => {}}>
          <SheetContent>
            <KeySheetForm editingKey={editingKey} onDone={vi.fn()} />
          </SheetContent>
        </Sheet>
      </QueryClientProvider>
    ) as ReactElement
  )
}

function bothPickers() {
  return screen.getAllByTestId('credential-ref-picker')
}

describe('downstream key credential refs (#1026)', () => {
  it('parses stored JSON strings back into the allow/exclude tree pickers', async () => {
    renderKeySheetForm({
      id: 7,
      name: 'scoped-key',
      enabled: true,
      allowedCredentialRefs: JSON.stringify([
        { kind: 'account_token', siteId: 1, accountId: 11, tokenId: 101 },
      ]),
      excludedCredentialRefs: JSON.stringify([
        { kind: 'default_api_key', siteId: 1, accountId: 11 },
      ]),
    })

    await waitFor(() => {
      expect(
        within(bothPickers()[0]).getByRole('checkbox', {
          name: /Token Token A/,
        })
      ).toBeChecked()
      expect(
        within(bothPickers()[1]).getByRole('checkbox', { name: /alice/ })
      ).toBeChecked()
    })
  })

  it('serializes both dimensions as canonical arrays on update', async () => {
    renderKeySheetForm({
      id: 7,
      name: 'scoped-key',
      enabled: true,
      allowedCredentialRefs: JSON.stringify([
        { kind: 'account_token', siteId: 1, accountId: 11, tokenId: 101 },
      ]),
      excludedCredentialRefs: JSON.stringify([
        { kind: 'default_api_key', siteId: 1, accountId: 11 },
      ]),
    })
    await waitFor(() => {
      expect(
        within(bothPickers()[0]).getByRole('checkbox', {
          name: /Token Token A/,
        })
      ).toBeChecked()
    })

    fireEvent.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => expect(mockUpdateKey).toHaveBeenCalledTimes(1))
    const [, payload] = mockUpdateKey.mock.calls[0] as [
      number,
      {
        allowedCredentialRefs?: unknown[]
        excludedCredentialRefs?: unknown[]
      },
    ]
    expect(payload.allowedCredentialRefs).toEqual([
      { kind: 'account_token', siteId: 1, accountId: 11, tokenId: 101 },
    ])
    expect(payload.excludedCredentialRefs).toEqual([
      { kind: 'default_api_key', siteId: 1, accountId: 11 },
    ])
  })

  it('sends empty arrays for both dimensions in create mode (unrestricted)', async () => {
    renderKeySheetForm(null)
    await waitFor(() => expect(bothPickers()).toHaveLength(2))

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'new-key' },
    })
    fireEvent.change(screen.getByPlaceholderText('sk-…'), {
      target: { value: 'sk-abcdefgh12345678' },
    })
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => expect(mockCreateKey).toHaveBeenCalledTimes(1))
    const payload = mockCreateKey.mock.calls[0][0] as {
      allowedCredentialRefs?: unknown[]
      excludedCredentialRefs?: unknown[]
    }
    expect(payload.allowedCredentialRefs).toEqual([])
    expect(payload.excludedCredentialRefs).toEqual([])
  })

  it('surfaces backend 400 messages containing the malformed entry index', async () => {
    mockCreateKey.mockRejectedValue({
      response: {
        status: 400,
        data: {
          error:
            'allowedCredentialRefs: credentialRefs[0] (account_token) requires a positive tokenId',
        },
      },
    })
    renderKeySheetForm(null)
    await waitFor(() => expect(bothPickers()).toHaveLength(2))

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'bad-key' },
    })
    fireEvent.change(screen.getByPlaceholderText('sk-…'), {
      target: { value: 'sk-abcdefgh12345678' },
    })
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => {
      expect(mockToastError).toHaveBeenCalledWith(
        'allowedCredentialRefs: credentialRefs[0] (account_token) requires a positive tokenId'
      )
    })
  })
})
