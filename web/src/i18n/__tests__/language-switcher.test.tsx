// metapi-go/i18n — language switcher integration test.
// Drives the real AppHeader dropdown against the real i18n instance
// (config.ts side-effect init) and asserts the active language, the
// localStorage persistence and the <html lang> sync. `dir` is owned by
// DirectionProvider, so a language change must NOT touch it.

import '@testing-library/jest-dom/vitest'
import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
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

import { AppHeader } from '@/components/layout/components/app-header'
import { SidebarProvider } from '@/components/ui/sidebar'
import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'

// The header's attention bell polls GET /api/stats/attention — stub the
// transport so this i18n test never reaches the network.
vi.mock('@/lib/api', () => ({
  api: {
    getAttention: vi.fn().mockResolvedValue({ items: [], total: 0 }),
  },
}))

// AppHeader's brand is a router Link; this i18n test renders it without a
// RouterProvider, so degrade Link to a plain anchor while keeping the rest
// of the module (same pattern as user-menu.test.tsx / status-badge tests).
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({ to, children }: { to?: unknown; children?: ReactNode }) => (
      <a href={typeof to === 'string' ? to : '/'}>{children}</a>
    ),
  }
})

// jsdom has no window.matchMedia (ThemeProvider reads prefers-color-scheme)
// and no ResizeObserver (Base UI positions the dropdown popup with it).
const matchMediaStub = (query: string): MediaQueryList =>
  ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }) as unknown as MediaQueryList

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation(matchMediaStub),
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
  localStorage.clear()
  await i18n.changeLanguage('en')
})

// vitest runs without `globals: true`, so @testing-library/react's automatic
// afterEach cleanup never registers — unmount explicitly between tests.
afterEach(() => {
  cleanup()
})

function renderHeader() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <SidebarProvider defaultOpen={false}>
          <AppHeader />
        </SidebarProvider>
      </ThemeProvider>
    </QueryClientProvider>
  )
}

describe('language switcher', () => {
  it('renders the trigger with a localized aria-label', () => {
    renderHeader()
    expect(screen.getByRole('button', { name: 'Language' })).toBeInTheDocument()
  })

  it('lists English and 简体中文 with the active language checked', async () => {
    renderHeader()

    fireEvent.click(screen.getByRole('button', { name: 'Language' }))

    const englishItem = await screen.findByRole('menuitem', { name: 'English' })
    const chineseItem = screen.getByRole('menuitem', { name: '简体中文' })

    expect(englishItem).toHaveAttribute('aria-current', 'true')
    expect(chineseItem).not.toHaveAttribute('aria-current')
  })

  it('switches i18n.language to zhCN and syncs <html lang> on click', async () => {
    renderHeader()
    // Simulate an RTL user's direction choice — the language change must NOT
    // clobber it (dir is owned by DirectionProvider, not the i18n layer).
    document.documentElement.dir = 'rtl'

    fireEvent.click(screen.getByRole('button', { name: 'Language' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '简体中文' }))

    await waitFor(() => expect(i18n.language).toBe('zhCN'))
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(document.documentElement.dir).toBe('rtl')

    // The detector caches the choice in localStorage for next visit.
    expect(localStorage.getItem('i18nextLng')).toBe('zhCN')
  })

  it('switches back to English and updates <html lang> again', async () => {
    renderHeader()

    fireEvent.click(screen.getByRole('button', { name: 'Language' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '简体中文' }))
    await waitFor(() => expect(i18n.language).toBe('zhCN'))

    // After the switch the trigger label is localized ('语言').
    fireEvent.click(screen.getByRole('button', { name: '语言' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: 'English' }))

    await waitFor(() => expect(i18n.language).toBe('en'))
    expect(document.documentElement.lang).toBe('en')
  })
})
