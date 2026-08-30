// Behavior tests for the redirects section dangerous-op tiers (#889, S7):
// row delete is the 删除+undo tier — no dialog; the row leaves immediately
// and the real DELETE fires only when the undo toast closes without 撤销.
// Apply keeps an explicit ConfirmDialog; Generate / Preview stay one-click.

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

const { mockToastMessage } = vi.hoisted(() => ({
  mockToastMessage: vi.fn(
    (_title: unknown, _options?: unknown) => 'undo-toast-id'
  ),
}))

vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
    message: mockToastMessage,
    dismiss: vi.fn(),
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
  mockToastMessage.mockClear()
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
  it('row delete removes the row immediately and commits when the undo window closes', async () => {
    renderSection()

    await screen.findByText('gpt-5.5')
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))

    // 删除+undo 档: no dialog, no immediate DELETE — the row is optimistically
    // gone and the undo toast carries the commit callbacks.
    await waitFor(() => {
      expect(screen.queryByText('gpt-5.5')).not.toBeInTheDocument()
    })
    expect(mockDeleteRedirect).not.toHaveBeenCalled()
    expect(mockToastMessage).toHaveBeenCalledTimes(1)
    const options = mockToastMessage.mock.calls[0]?.[1] as {
      action: { label: string; onClick: () => void }
      onAutoClose: () => void
    }
    expect(options.action.label).toBe('Undo')

    options.onAutoClose()

    await waitFor(() => {
      expect(mockDeleteRedirect).toHaveBeenCalledWith(7)
    })
  })

  it('undo restores the row and the delete never fires', async () => {
    renderSection()

    await screen.findByText('gpt-5.5')
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    await waitFor(() => {
      expect(screen.queryByText('gpt-5.5')).not.toBeInTheDocument()
    })

    const options = mockToastMessage.mock.calls[0]?.[1] as {
      action: { label: string; onClick: () => void }
      onAutoClose: () => void
    }
    options.action.onClick()

    // Snapshot restored…
    await screen.findByText('gpt-5.5')
    // …and the window closing afterwards must not fire the delete.
    options.onAutoClose()
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

describe('RedirectsSection empty-state CTA', () => {
  it('offers a Generate CTA in the empty state', async () => {
    mockGetRedirects.mockResolvedValue({ items: [] })
    renderSection()

    expect(await screen.findByText(/No model redirects/)).toBeInTheDocument()

    // The header actions carry a Generate button too; the empty-state CTA is
    // the last one in the tree.
    const generateButtons = screen.getAllByRole('button', { name: 'Generate' })
    const ctaButton = generateButtons.at(-1)
    if (!ctaButton) throw new Error('empty-state Generate CTA not rendered')
    fireEvent.click(ctaButton)

    await waitFor(() => {
      expect(mockGenerateRedirects).toHaveBeenCalledTimes(1)
    })
  })
})
