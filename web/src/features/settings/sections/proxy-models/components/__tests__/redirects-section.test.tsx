// Behavior tests for the redirects section dangerous-op confirmations (#889):
// row delete and Apply both require an explicit ConfirmDialog step before the
// mutation fires, while Generate / Preview stay one-click.

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

import { RedirectsSection } from '../redirects-section'

const {
  mockGetRedirects,
  mockDeleteRedirect,
  mockApplyRedirects,
  mockGenerateRedirects,
} = vi.hoisted(() => ({
  mockGetRedirects: vi.fn(),
  mockDeleteRedirect: vi.fn(),
  mockApplyRedirects: vi.fn(),
  mockGenerateRedirects: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    getModelRedirects: mockGetRedirects,
    deleteModelRedirect: mockDeleteRedirect,
    applyModelRedirects: mockApplyRedirects,
    generateModelRedirects: mockGenerateRedirects,
    updateModelRedirect: vi.fn(),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
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
  mockGetRedirects.mockReset()
  mockDeleteRedirect.mockReset()
  mockApplyRedirects.mockReset()
  mockGenerateRedirects.mockReset()
  mockDeleteRedirect.mockResolvedValue({ success: true })
  mockApplyRedirects.mockResolvedValue({
    success: true,
    dryRun: false,
    removed: 1,
  })
  mockGenerateRedirects.mockResolvedValue({ success: true, created: 0 })
  mockGetRedirects.mockResolvedValue({
    items: [
      {
        id: 7,
        accountId: 1,
        username: 'ops',
        canonical: 'gpt-5.5',
        actual: 'gpt-5.4',
        source: 'manual',
        createdAt: '2026-01-01T00:00:00Z',
        updatedAt: '2026-01-01T00:00:00Z',
      },
    ],
  })
})

afterEach(() => cleanup())

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <RedirectsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

describe('RedirectsSection dangerous-op confirmations', () => {
  it('row delete opens a confirmation and only deletes after confirm', async () => {
    renderSection()

    const deleteButton = await screen.findByRole('button', { name: 'Delete' })
    fireEvent.click(deleteButton)

    // No delete before confirmation.
    expect(mockDeleteRedirect).not.toHaveBeenCalled()

    // The dialog confirm button is the last "Delete" on screen (row action
    // first, dialog action last).
    const deleteButtons = await screen.findAllByRole('button', {
      name: 'Delete',
    })
    const confirmDeleteButton = deleteButtons.at(-1)
    if (!confirmDeleteButton) throw new Error('confirm button not rendered')
    fireEvent.click(confirmDeleteButton)

    await waitFor(() => {
      expect(mockDeleteRedirect).toHaveBeenCalledWith(7)
    })
  })

  it('row delete cancel does not call the delete api', async () => {
    renderSection()

    const deleteButton = await screen.findByRole('button', { name: 'Delete' })
    fireEvent.click(deleteButton)

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(mockDeleteRedirect).not.toHaveBeenCalled()
  })

  it('Apply opens a confirmation and only applies after confirm', async () => {
    renderSection()

    await screen.findByText('gpt-5.5')
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))

    expect(mockApplyRedirects).not.toHaveBeenCalled()

    // Confirm inside the dialog (toolbar button first, dialog action last).
    const applyButtons = screen.getAllByRole('button', { name: 'Apply' })
    const confirmApplyButton = applyButtons.at(-1)
    if (!confirmApplyButton) throw new Error('confirm button not rendered')
    fireEvent.click(confirmApplyButton)

    await waitFor(() => {
      expect(mockApplyRedirects).toHaveBeenCalledWith(false)
    })
  })

  it('Preview (dry run) stays one-click — no confirmation', async () => {
    mockApplyRedirects.mockResolvedValueOnce({
      success: true,
      dryRun: true,
      candidates: [],
      count: 0,
    })
    renderSection()

    await screen.findByText('gpt-5.5')
    fireEvent.click(screen.getByRole('button', { name: 'Preview' }))

    await waitFor(() => {
      expect(mockApplyRedirects).toHaveBeenCalledWith(true)
    })
  })
})
