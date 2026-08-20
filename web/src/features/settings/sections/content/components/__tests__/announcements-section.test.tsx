// Behavior tests for the announcements form dirty-close guard (#889):
// closing the create/edit dialog with unsaved changes must route through the
// shared discard confirmation instead of silently dropping the edits, while a
// pristine form closes immediately.

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
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'

import { AnnouncementsSection } from '../announcements-section'

vi.mock('@/lib/api', () => ({
  api: {
    getAnnouncements: vi.fn().mockResolvedValue({ items: [] }),
    createAnnouncement: vi.fn().mockResolvedValue({ items: [] }),
    updateAnnouncement: vi.fn().mockResolvedValue({ success: true }),
    deleteAnnouncement: vi.fn().mockResolvedValue({ success: true }),
  },
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
        <AnnouncementsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
}

async function openCreateDialog() {
  fireEvent.click(
    await screen.findByRole('button', { name: 'New announcement' })
  )
  await screen.findByText('Create announcement')
}

describe('AnnouncementsSection dirty-close guard', () => {
  it('pristine form closes immediately without a discard prompt', async () => {
    renderSection()
    await openCreateDialog()

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))

    await waitFor(() => {
      expect(screen.queryByText('Create announcement')).toBeNull()
    })
    expect(screen.queryByText('Discard unsaved changes?')).toBeNull()
  })

  it('dirty form requires the discard confirmation to close', async () => {
    renderSection()
    await openCreateDialog()

    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'Maintenance window' },
    })

    // Cancel is intercepted by the discard confirmation.
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await screen.findByText('Discard unsaved changes?')

    // Keep editing leaves the form open with the typed value intact.
    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => {
      expect(screen.queryByText('Discard unsaved changes?')).toBeNull()
    })
    expect(screen.getByLabelText('Title')).toHaveValue('Maintenance window')

    // Discarding finally closes the dialog.
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Discard changes' })
    )
    await waitFor(() => {
      expect(screen.queryByText('Create announcement')).toBeNull()
    })
  })
})
