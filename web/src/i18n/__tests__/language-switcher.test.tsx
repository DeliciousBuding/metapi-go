// metapi-go/i18n — language switcher integration test.
// Drives the real AppHeader dropdown against the real i18n instance
// (config.ts side-effect init) and asserts the active language, the
// localStorage persistence and the <html lang>/dir sync.

import '@testing-library/jest-dom/vitest'
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
import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'

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
  return render(
    <ThemeProvider>
      <AppHeader />
    </ThemeProvider>
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

    fireEvent.click(screen.getByRole('button', { name: 'Language' }))
    fireEvent.click(await screen.findByRole('menuitem', { name: '简体中文' }))

    await waitFor(() => expect(i18n.language).toBe('zhCN'))
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(document.documentElement.dir).toBe('ltr')

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
