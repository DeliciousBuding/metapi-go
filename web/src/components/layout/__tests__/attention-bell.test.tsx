// metapi-go/layout — global attention bell tests.
//
// Pins the header alert surface added for issue #887: an operator on any page
// must see that attention items are pending (count badge + severity tone),
// be able to jump to the item's deep link through the router, and never get a
// silent failure — loading / error / empty each render their own state.
//
// `api.getAttention` and the router are stubbed; the bell is rendered
// standalone inside a QueryClientProvider (the header mounts it inside the
// app-wide provider from main.tsx).

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'

import { AttentionBell } from '../components/attention-bell'

vi.mock('@/lib/api', () => ({
  api: {
    getAttention: vi.fn(),
    markEventRead: vi.fn(),
    markSiteAnnouncementRead: vi.fn(),
  },
}))

const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    // Resolve `$param` placeholders so the footer link's href is assertable
    // without mounting a real router.
    Link: ({
      to,
      params,
      children,
      ...rest
    }: {
      to?: unknown
      params?: Record<string, string>
      children?: ReactNode
    } & Record<string, unknown>) => {
      const path = typeof to === 'string' ? to : '/'
      const resolved = Object.entries(params ?? {}).reduce(
        (href, [name, value]) => href.replace(`$${name}`, value),
        path
      )
      return (
        <a href={resolved} {...rest}>
          {children}
        </a>
      )
    },
  }
})

const mockGetAttention = vi.mocked(api.getAttention)
const mockMarkEventRead = vi.mocked(api.markEventRead)
const mockMarkSiteAnnouncementRead = vi.mocked(api.markSiteAnnouncementRead)

// Base UI positions the popover with browser APIs jsdom does not implement.
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

  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  Object.defineProperty(globalThis, 'ResizeObserver', {
    writable: true,
    value: ResizeObserverStub,
  })
})

beforeEach(async () => {
  mockGetAttention.mockReset()
  mockMarkEventRead.mockReset()
  mockMarkEventRead.mockResolvedValue({ success: true })
  mockMarkSiteAnnouncementRead.mockReset()
  mockMarkSiteAnnouncementRead.mockResolvedValue({ success: true })
  navigateMock.mockReset()
  await i18n.changeLanguage('en')
})

afterEach(() => cleanup())

function attentionItem(overrides: {
  severity: 'critical' | 'warning' | 'info'
  label: string
  target?: string
  category?: string
  params?: Record<string, string | number>
}) {
  return {
    category: 'expired_account',
    target: '/accounts?accountId=7',
    createdAt: new Date().toISOString(),
    ...overrides,
  }
}

function renderBell() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AttentionBell />
    </QueryClientProvider>
  )
}

function indicatorOf(container: HTMLElement) {
  return container.querySelector('[data-slot="attention-indicator"]')
}

async function openBell() {
  const rendered = renderBell()
  const trigger = await screen.findByRole('button', {
    name: /Attention items/,
  })
  fireEvent.click(trigger)
  await screen.findByRole('dialog')
  return rendered
}

describe('attention bell', () => {
  it('names the icon-only trigger and folds the pending count into the name', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({ severity: 'critical', label: 'Account expired: ops' }),
        attentionItem({ severity: 'warning', label: 'Low balance: relay' }),
      ],
      total: 2,
    })

    renderBell()

    expect(
      await screen.findByRole('button', { name: 'Attention items (2 pending)' })
    ).toBeInTheDocument()
  })

  it('renders a destructive count badge when a critical item is pending', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({ severity: 'warning', label: 'Low balance: relay' }),
        attentionItem({ severity: 'critical', label: 'Account expired: ops' }),
      ],
      total: 2,
    })

    const { container } = renderBell()

    await waitFor(() => expect(indicatorOf(container)).not.toBeNull())
    const indicator = indicatorOf(container)
    expect(indicator).toHaveTextContent('2')
    expect(indicator).toHaveAttribute('data-severity', 'critical')
    expect(indicator?.className).toContain('bg-destructive')
  })

  it('keeps the badge on the warning tone when no critical item is pending', async () => {
    mockGetAttention.mockResolvedValue({
      items: [attentionItem({ severity: 'warning', label: 'Site disabled' })],
      total: 1,
    })

    const { container } = renderBell()

    await waitFor(() => expect(indicatorOf(container)).not.toBeNull())
    const indicator = indicatorOf(container)
    expect(indicator).toHaveAttribute('data-severity', 'warning')
    expect(indicator?.className).toContain('bg-warning')
  })

  it('caps the badge at 9+ so a long backlog cannot widen the trigger', async () => {
    mockGetAttention.mockResolvedValue({
      items: Array.from({ length: 12 }, (_unused, index) =>
        attentionItem({ severity: 'warning', label: `Low balance ${index}` })
      ),
      total: 12,
    })

    const { container } = renderBell()

    await waitFor(() => expect(indicatorOf(container)).not.toBeNull())
    expect(indicatorOf(container)).toHaveTextContent('9+')
  })

  it('renders no badge and an empty state when nothing needs attention', async () => {
    mockGetAttention.mockResolvedValue({ items: [], total: 0 })

    const { container } = await openBell()

    expect(
      await screen.findByText('Nothing needs attention right now.')
    ).toBeInTheDocument()
    expect(indicatorOf(container)).toBeNull()
    expect(
      screen.getByRole('button', { name: 'Attention items' })
    ).toBeInTheDocument()
  })

  it('navigates to the item target through the router on click', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({
          severity: 'critical',
          label: 'Account expired: ops',
          target: '/accounts?accountId=7',
        }),
      ],
      total: 1,
    })

    await openBell()

    fireEvent.click(
      await screen.findByRole('button', { name: /Account expired: ops/ })
    )

    expect(navigateMock).toHaveBeenCalledWith({
      href: '/accounts?accountId=7',
    })
  })

  it('marks an event read when its bell item is opened', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({
          severity: 'warning',
          category: 'event',
          label: 'Upstream rate limited',
          target: '/settings/system-info/program-logs',
          params: { eventId: 12 },
        }),
      ],
      total: 1,
    })

    const { container } = await openBell()
    fireEvent.click(
      await screen.findByRole('button', { name: /Upstream rate limited/ })
    )

    expect(mockMarkEventRead).toHaveBeenCalledWith(12)
    expect(navigateMock).toHaveBeenCalledWith({
      href: '/settings/system-info/program-logs',
    })
    await waitFor(() => expect(indicatorOf(container)).toBeNull())
  })

  it('marks a site announcement read before navigating to the local notice page', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({
          category: 'site_announcement',
          severity: 'info',
          label: 'Upstream announcement: Maintenance',
          target: '/site-announcements',
          params: { announcementId: 33, title: 'Maintenance' },
        }),
      ],
      total: 1,
    })

    const { container } = await openBell()
    fireEvent.click(
      await screen.findByRole('button', {
        name: /Upstream announcement: Maintenance/,
      })
    )

    expect(mockMarkSiteAnnouncementRead).toHaveBeenCalledWith(33)
    expect(navigateMock).toHaveBeenCalledWith({ href: '/site-announcements' })
    await waitFor(() => expect(indicatorOf(container)).toBeNull())
  })

  it('shows a relative timestamp and links to the availability panel', async () => {
    mockGetAttention.mockResolvedValue({
      items: [
        attentionItem({
          severity: 'warning',
          label: 'Low balance: relay',
          target: '/accounts?accountId=3',
        }),
      ],
      total: 1,
    })

    const { container } = await openBell()

    expect(container.ownerDocument.querySelector('time')).not.toBeNull()
    expect(
      screen.getByRole('link', { name: 'View all attention items' })
    ).toHaveAttribute('href', '/dashboard/availability')
  })

  it('surfaces a load error instead of swallowing it', async () => {
    mockGetAttention.mockRejectedValue(new Error('attention unavailable'))

    await openBell()

    expect(
      await screen.findByText('Failed to load attention items.')
    ).toBeInTheDocument()
  })

  it('renders a skeleton while the attention query is in flight', async () => {
    mockGetAttention.mockReturnValue(new Promise(() => {}))

    await openBell()

    expect(
      document.body.querySelectorAll('[data-slot="skeleton"]').length
    ).toBeGreaterThan(0)
  })
})
