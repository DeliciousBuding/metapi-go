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

import { InterfaceControls } from '@/components/layout/components/interface-controls'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import i18n from '@/i18n/config'

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
  document.cookie = 'vite-ui-theme=; Max-Age=0; path=/'
  document.documentElement.classList.remove('light', 'dark')
  await i18n.changeLanguage('en')
})

afterEach(() => cleanup())

function renderControls() {
  return render(
    <ThemeProvider defaultTheme='light'>
      <ThemeCustomizationProvider>
        <InterfaceControls />
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

describe('interface controls', () => {
  it('exposes language, appearance, and color-scheme controls', () => {
    renderControls()

    expect(screen.getByRole('button', { name: 'Language' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Customize appearance' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Toggle theme' })
    ).toBeInTheDocument()
  })

  it('toggles the resolved color scheme instead of the stored system value', async () => {
    renderControls()
    await waitFor(() => expect(document.documentElement).toHaveClass('light'))

    fireEvent.click(screen.getByRole('button', { name: 'Toggle theme' }))

    await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
    expect(document.documentElement.style.colorScheme).toBe('dark')
  })

  // Checklist residual §7 #2 verification: the header's non-modal menus must
  // dismiss on Escape, not only on click-outside. Base UI Menu/Popover wire
  // this natively; these tests pin the behavior so a primitive swap cannot
  // silently drop it.
  it('dismisses the language menu on Escape', async () => {
    renderControls()

    fireEvent.click(screen.getByRole('button', { name: 'Language' }))
    const menu = await screen.findByRole('menu')

    fireEvent.keyDown(menu, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('menu')).not.toBeInTheDocument()
    })
  })

  it('dismisses the appearance popover on Escape', async () => {
    renderControls()

    fireEvent.click(
      screen.getByRole('button', { name: 'Customize appearance' })
    )
    const popover = await screen.findByRole('dialog')

    fireEvent.keyDown(popover, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })
})
