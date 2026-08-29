// Behavior test for the downstream key sheet's upstream site restriction
// (#1026): the routing selector has always honored allowedSiteIds, but the
// management UI never exposed it. These tests pin the round-trip:
//   - edit mode pre-fills the checkbox list from item.allowedSiteIds
//   - toggling a site and saving sends allowedSiteIds in the update payload
//   - create mode sends the selected site scope as well
// Empty selection means "no site restriction" (selector treats an empty
// allow-list as unrestricted).

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

import '@/i18n/config'
import { Sheet, SheetContent } from '@/components/ui/sheet'

import { KeySheetForm } from '../keys-section'

const { mockCreateKey, mockUpdateKey, mockGetSites } = vi.hoisted(() => ({
  mockCreateKey: vi.fn(),
  mockUpdateKey: vi.fn(),
  mockGetSites: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    createDownstreamApiKey: mockCreateKey,
    updateDownstreamApiKey: mockUpdateKey,
    getSites: mockGetSites,
    getAccountsSnapshot: vi
      .fn()
      .mockResolvedValue({ accounts: [], sites: [], generatedAt: '' }),
    getAccountTokens: vi.fn().mockResolvedValue([]),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const sitesFixture = [
  { id: 1, name: 'Primary site' },
  { id: 2, name: 'Backup site' },
]

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
  mockGetSites.mockResolvedValue(sitesFixture)
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

function siteCheckbox(name: string): HTMLInputElement {
  return screen.getByLabelText(name) as HTMLInputElement
}

describe('downstream key site restriction (#1026)', () => {
  it('edit mode pre-fills the site scope from the stored key', async () => {
    renderKeySheetForm({
      id: 7,
      name: 'scoped-key',
      enabled: true,
      allowedSiteIds: [2],
    })

    await waitFor(() => {
      expect(siteCheckbox('Primary site').checked).toBe(false)
      expect(siteCheckbox('Backup site').checked).toBe(true)
    })
  })

  it('saving an edited scope sends allowedSiteIds in the update payload', async () => {
    renderKeySheetForm({
      id: 7,
      name: 'scoped-key',
      enabled: true,
      allowedSiteIds: [2],
    })
    await waitFor(() => {
      expect(siteCheckbox('Backup site').checked).toBe(true)
    })

    fireEvent.click(siteCheckbox('Primary site'))
    fireEvent.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(mockUpdateKey).toHaveBeenCalledTimes(1)
    })
    const [id, payload] = mockUpdateKey.mock.calls[0] as [
      number,
      { allowedSiteIds?: number[] },
    ]
    expect(id).toBe(7)
    expect(payload.allowedSiteIds).toEqual([1, 2])
  })

  it('create mode sends the selected site scope', async () => {
    renderKeySheetForm(null)
    await waitFor(() => {
      expect(screen.getByTestId('site-scope-picker')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByLabelText('Name'), {
      target: { value: 'new-key' },
    })
    fireEvent.change(screen.getByPlaceholderText('sk-…'), {
      target: { value: 'sk-abcdefgh12345678' },
    })
    fireEvent.click(siteCheckbox('Primary site'))
    fireEvent.click(screen.getByRole('button', { name: /create/i }))

    await waitFor(() => {
      expect(mockCreateKey).toHaveBeenCalledTimes(1)
    })
    const payload = mockCreateKey.mock.calls[0][0] as {
      allowedSiteIds?: number[]
    }
    expect(payload.allowedSiteIds).toEqual([1])
  })

  it('unchecking every site restores the unrestricted scope', async () => {
    renderKeySheetForm({
      id: 7,
      name: 'scoped-key',
      enabled: true,
      allowedSiteIds: [2],
    })
    await waitFor(() => {
      expect(siteCheckbox('Backup site').checked).toBe(true)
    })

    fireEvent.click(siteCheckbox('Backup site'))
    fireEvent.click(screen.getByRole('button', { name: /save/i }))

    await waitFor(() => {
      expect(mockUpdateKey).toHaveBeenCalledTimes(1)
    })
    const payload = mockUpdateKey.mock.calls[0][1] as {
      allowedSiteIds?: number[]
    }
    expect(payload.allowedSiteIds).toEqual([])
  })
})
