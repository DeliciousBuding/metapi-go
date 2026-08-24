// Focused product-announcement behavior tests: dirty-close safety, query
// truth, shared cache invalidation, and strict link validation.

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
import { api, type Announcement } from '@/lib/api'
import { productAnnouncementKeys } from '@/lib/product-announcements'
import { toast } from '@/lib/toast'

import { AnnouncementsSection } from '../announcements-section'

vi.mock('@/lib/api', () => ({
  api: {
    getAnnouncements: vi.fn(),
    createAnnouncement: vi.fn(),
    updateAnnouncement: vi.fn(),
    deleteAnnouncement: vi.fn(),
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

const mockGetAnnouncements = vi.mocked(api.getAnnouncements)
const mockCreateAnnouncement = vi.mocked(api.createAnnouncement)
const mockUpdateAnnouncement = vi.mocked(api.updateAnnouncement)
const mockDeleteAnnouncement = vi.mocked(api.deleteAnnouncement)
const mockToastSuccess = vi.mocked(toast.success)

const existingAnnouncement: Announcement = {
  id: 7,
  title: 'Existing maintenance notice',
  message: 'Existing message',
  severity: 'warning',
  link: 'https://status.example.com/notice',
  enabled: true,
  createdAt: '2026-08-20 00:00:00',
  updatedAt: '2026-08-20 00:00:00',
}

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
  Object.defineProperty(HTMLElement.prototype, 'scrollIntoView', {
    configurable: true,
    value: vi.fn(),
  })
})

beforeEach(() => {
  mockGetAnnouncements.mockReset().mockResolvedValue({ items: [] })
  mockCreateAnnouncement.mockReset().mockResolvedValue({ items: [] })
  mockUpdateAnnouncement
    .mockReset()
    .mockResolvedValue({ success: true, revision: true })
  mockDeleteAnnouncement.mockReset().mockResolvedValue({ success: true })
  vi.mocked(toast.error).mockReset()
  mockToastSuccess.mockReset()
})

afterEach(() => cleanup())

function renderSection() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: 0 },
      mutations: { retry: false },
    },
  })
  const rendered = render(
    (
      <QueryClientProvider client={queryClient}>
        <AnnouncementsSection />
      </QueryClientProvider>
    ) as ReactElement
  )
  return { queryClient, ...rendered }
}

async function openCreateDialog() {
  fireEvent.click(
    await screen.findByRole('button', { name: 'New announcement' })
  )
  await screen.findByText('Create announcement')
}

function fillRequiredFields(link?: string) {
  fireEvent.change(screen.getByLabelText('Title'), {
    target: { value: 'Maintenance window' },
  })
  fireEvent.change(screen.getByLabelText('Message'), {
    target: { value: 'Downtime expected tonight.' },
  })
  if (link !== undefined) {
    fireEvent.change(screen.getByLabelText('Link'), {
      target: { value: link },
    })
  }
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

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await screen.findByText('Discard unsaved changes?')

    fireEvent.click(screen.getByRole('button', { name: 'Keep editing' }))
    await waitFor(() => {
      expect(screen.queryByText('Discard unsaved changes?')).toBeNull()
    })
    expect(screen.getByLabelText('Title')).toHaveValue('Maintenance window')

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    fireEvent.click(
      await screen.findByRole('button', { name: 'Discard changes' })
    )
    await waitFor(() => {
      expect(screen.queryByText('Create announcement')).toBeNull()
    })
  })
})

describe('AnnouncementsSection query truth', () => {
  it('shows a retryable load error instead of the empty state', async () => {
    mockGetAnnouncements
      .mockReset()
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({ items: [] })

    renderSection()

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Failed to load announcements: network down'
    )
    expect(
      screen.queryByText(
        'No announcements. Create one to surface a risk banner.'
      )
    ).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

    await waitFor(() => {
      expect(mockGetAnnouncements).toHaveBeenCalledTimes(2)
    })
    expect(
      await screen.findByText(
        'No announcements. Create one to surface a risk banner.'
      )
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
  })
})

describe('AnnouncementsSection link policy', () => {
  it.each([
    'javascript:alert(1)',
    'data:text/html,boom',
    'file:///tmp/notice',
    'ftp://example.com/notice',
    '//example.com/notice',
    '/relative/notice',
    'relative/notice',
    'not a url',
  ])('rejects unsafe link %s before submit', async (link) => {
    renderSection()
    await openCreateDialog()
    fillRequiredFields(link)

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    expect(
      await screen.findByText('Enter an absolute http:// or https:// URL.')
    ).toBeInTheDocument()
    expect(mockCreateAnnouncement).not.toHaveBeenCalled()
  })

  it.each([
    { label: 'empty', input: '', expected: undefined },
    {
      label: 'absolute https',
      input: '  https://docs.example.com/status  ',
      expected: 'https://docs.example.com/status',
    },
    {
      label: 'absolute http',
      input: 'http://status.example.com/notice',
      expected: 'http://status.example.com/notice',
    },
  ])(
    'accepts $label links and invalidates active announcements',
    async ({ input, expected }) => {
      const { queryClient } = renderSection()
      const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
      await openCreateDialog()
      fillRequiredFields(input)

      fireEvent.click(screen.getByRole('button', { name: 'Save' }))

      await waitFor(() => {
        expect(mockCreateAnnouncement).toHaveBeenCalledWith(
          expect.objectContaining({ link: expected })
        )
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: productAnnouncementKeys.list(),
      })
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: productAnnouncementKeys.active(),
      })
      expect(mockToastSuccess).toHaveBeenCalledWith('Announcement saved.')
    }
  )
})

describe('AnnouncementsSection CRUD cache truth', () => {
  it('invalidates active announcements after update', async () => {
    mockGetAnnouncements.mockResolvedValue({ items: [existingAnnouncement] })
    const { queryClient } = renderSection()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    fireEvent.click(await screen.findByRole('button', { name: 'Edit' }))
    fireEvent.change(screen.getByLabelText('Title'), {
      target: { value: 'Updated maintenance notice' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateAnnouncement).toHaveBeenCalledWith(
        7,
        expect.objectContaining({ title: 'Updated maintenance notice' })
      )
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: productAnnouncementKeys.active(),
    })
  })

  it('invalidates active announcements after delete', async () => {
    mockGetAnnouncements.mockResolvedValue({ items: [existingAnnouncement] })
    const { queryClient } = renderSection()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    fireEvent.click(await screen.findByRole('button', { name: 'Delete' }))
    const deleteButtons = await screen.findAllByRole('button', {
      name: 'Delete',
    })
    const confirmDeleteButton = deleteButtons.at(-1)
    expect(confirmDeleteButton).toBeDefined()
    if (!confirmDeleteButton) throw new Error('Delete confirmation not found')
    fireEvent.click(confirmDeleteButton)

    await waitFor(() => {
      expect(mockDeleteAnnouncement).toHaveBeenCalledWith(7)
    })
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: productAnnouncementKeys.active(),
    })
  })
})
