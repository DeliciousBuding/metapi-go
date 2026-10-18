// Behavior test for downstream-key create/update re-confirmation (#1034).
//
// Minting or rotating a downstream key is a long-lived credential write, so
// the backend demands the master token. The sheet must keep its submitted
// values, prompt once, and replay the same payload with the token.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
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

import { ReauthProvider } from '@/components/common/reauth-provider'
import { Sheet, SheetContent } from '@/components/ui/sheet'
import i18n from '@/i18n/config'

import { KeySheetForm } from '../key-sheet-form'

const { mockCreateKey, mockUpdateKey, mockGetSites, mockToastSuccess } =
  vi.hoisted(() => ({
    mockCreateKey: vi.fn(),
    mockUpdateKey: vi.fn(),
    mockGetSites: vi.fn(),
    mockToastSuccess: vi.fn(),
  }))

vi.mock('@/lib/api', () => ({
  api: {
    createDownstreamApiKey: mockCreateKey,
    updateDownstreamApiKey: mockUpdateKey,
    getSites: mockGetSites,
    getImportedUpstreams: vi.fn().mockResolvedValue({
      items: [],
      members: [],
    }),
    getAccountsSnapshot: vi
      .fn()
      .mockResolvedValue({ accounts: [], sites: [], generatedAt: '' }),
    getAccountTokens: vi.fn().mockResolvedValue([]),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: mockToastSuccess,
    error: vi.fn(),
  },
}))

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    }
  )
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
  mockToastSuccess.mockReset()
  mockGetSites.mockResolvedValue([])
})

afterEach(() => cleanup())

const text = (key: string) => String(i18n.t(key))

function reauthError() {
  return { response: { status: 403, data: { reauthRequired: true } } }
}

const knownKey = {
  id: 42,
  name: 'My downstream key',
  keyMasked: 'sk-…abc',
  groupName: 'prod',
  enabled: true,
  maxRequests: 1000,
  maxCost: 50,
  usedRequests: 10,
  usedCost: 5,
  expiresAt: null,
  supportedModels: '["gpt-5.5"]',
} as NonNullable<Parameters<typeof KeySheetForm>[0]['editingKey']>

function renderKeySheetForm(editingKey: typeof knownKey | null) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  const onDone = vi.fn()
  render(
    (
      <QueryClientProvider client={queryClient}>
        <ReauthProvider>
          <Sheet open onOpenChange={() => {}}>
            <SheetContent>
              <KeySheetForm
                editingKey={editingKey}
                onDone={onDone}
                candidateModels={[]}
              />
            </SheetContent>
          </Sheet>
        </ReauthProvider>
      </QueryClientProvider>
    ) as ReactElement
  )
  return { onDone }
}

function submitForm() {
  const form = document.querySelector('form')
  if (!form) throw new Error('KeySheetForm did not render its <form>')
  fireEvent.submit(form)
}

async function confirmReauth(token: string) {
  const input = await screen.findByLabelText(text('reauth.tokenLabel'))
  fireEvent.change(input, { target: { value: token } })
  fireEvent.click(screen.getByRole('button', { name: text('reauth.submit') }))
}

describe('KeySheetForm — master-token replay', () => {
  it('replays a create with the token without losing the form input', async () => {
    mockCreateKey.mockRejectedValueOnce(reauthError()).mockResolvedValueOnce({})
    const { onDone } = renderKeySheetForm(null)

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-12345678' },
    })
    submitForm()

    await waitFor(() => expect(mockCreateKey).toHaveBeenCalledTimes(1))
    expect(mockCreateKey.mock.calls[0]).toHaveLength(1)
    expect(onDone).not.toHaveBeenCalled()

    await confirmReauth('master-token')

    await waitFor(() => expect(mockCreateKey).toHaveBeenCalledTimes(2))
    expect(mockCreateKey.mock.calls[1]?.[1]).toBe('master-token')
    expect(mockCreateKey.mock.calls[1]?.[0]).toMatchObject({
      name: 'New key',
      key: 'sk-12345678',
    })
    // The replay reuses the same values; the sheet never reset the form.
    expect(screen.getByLabelText('Name')).toHaveValue('New key')
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  })

  it('replays an update with the token', async () => {
    mockUpdateKey.mockRejectedValueOnce(reauthError()).mockResolvedValueOnce({})
    const { onDone } = renderKeySheetForm(knownKey)

    submitForm()

    await waitFor(() => expect(mockUpdateKey).toHaveBeenCalledTimes(1))
    expect(mockUpdateKey.mock.calls[0]).toHaveLength(2)
    expect(onDone).not.toHaveBeenCalled()

    await confirmReauth('master-token')

    await waitFor(() => expect(mockUpdateKey).toHaveBeenCalledTimes(2))
    expect(mockUpdateKey.mock.calls[1]?.[0]).toBe(knownKey.id)
    expect(mockUpdateKey.mock.calls[1]?.[2]).toBe('master-token')
    await waitFor(() => expect(onDone).toHaveBeenCalledTimes(1))
  })

  it('does not prompt when the create succeeds immediately', async () => {
    mockCreateKey.mockResolvedValueOnce({})
    renderKeySheetForm(null)

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'New key' },
    })
    fireEvent.change(screen.getByLabelText('Key'), {
      target: { value: 'sk-12345678' },
    })
    submitForm()

    await waitFor(() => expect(mockCreateKey).toHaveBeenCalledTimes(1))
    expect(
      screen.queryByLabelText(text('reauth.tokenLabel'))
    ).not.toBeInTheDocument()
  })
})
