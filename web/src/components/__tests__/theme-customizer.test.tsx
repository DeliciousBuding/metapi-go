// metapi-go/components — theme customizer behavior tests.
// Covers the two session-polish additions: the color-scheme section (which
// returns the mode to "follow system") and the content-layout axis section.

import '@testing-library/jest-dom/vitest'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
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

import { ThemeCustomizer } from '@/components/theme-customizer'
import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import '@/i18n/config'

const THEME_COOKIE_NAME = 'vite-ui-theme'
const CONTENT_LAYOUT_COOKIE_NAME = 'theme_content_layout'

// jsdom has no window.matchMedia (ThemeProvider reads prefers-color-scheme)
// and no ResizeObserver (Base UI positions the popover with it).
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

beforeEach(() => {
  document.cookie = `${THEME_COOKIE_NAME}=; Max-Age=0`
  document.cookie = `${CONTENT_LAYOUT_COOKIE_NAME}=; Max-Age=0`
  document.body.removeAttribute('data-theme-content-layout')
  document.documentElement.classList.remove('light', 'dark')
})

afterEach(() => cleanup())

function renderCustomizer() {
  return render(
    <ThemeProvider defaultTheme='system'>
      <ThemeCustomizationProvider>
        <ThemeCustomizer />
      </ThemeCustomizationProvider>
    </ThemeProvider>
  )
}

async function openCustomizer() {
  renderCustomizer()
  fireEvent.click(screen.getByRole('button', { name: 'Customize appearance' }))
  return screen.findByRole('radiogroup', { name: 'Color scheme' })
}

describe('theme customizer color scheme section', () => {
  it('offers light, dark, and system options', async () => {
    await openCustomizer()

    const colorSchemeGroup = screen.getByRole('radiogroup', {
      name: 'Color scheme',
    })
    expect(
      within(colorSchemeGroup).getByRole('radio', { name: 'Light' })
    ).toBeInTheDocument()
    expect(
      within(colorSchemeGroup).getByRole('radio', { name: 'Dark' })
    ).toBeInTheDocument()
    expect(
      within(colorSchemeGroup).getByRole('radio', { name: 'System' })
    ).toBeInTheDocument()
  })

  it('applies an explicit dark choice and returns to system afterwards', async () => {
    await openCustomizer()

    const colorSchemeGroup = screen.getByRole('radiogroup', {
      name: 'Color scheme',
    })
    fireEvent.click(
      within(colorSchemeGroup).getByRole('radio', { name: 'Dark' })
    )

    await waitFor(() =>
      expect(document.documentElement.classList.contains('dark')).toBe(true)
    )
    expect(document.cookie).toContain(`${THEME_COOKIE_NAME}=dark`)

    fireEvent.click(
      within(colorSchemeGroup).getByRole('radio', { name: 'System' })
    )

    // matchMedia is mocked to light, so "system" resolves back to light.
    await waitFor(() =>
      expect(document.documentElement.classList.contains('light')).toBe(true)
    )
    expect(document.documentElement.classList.contains('dark')).toBe(false)
  })

  it('shows a section reset once the mode is customized, restoring the default', async () => {
    await openCustomizer()

    const colorSchemeGroup = screen.getByRole('radiogroup', {
      name: 'Color scheme',
    })
    fireEvent.click(
      within(colorSchemeGroup).getByRole('radio', { name: 'Dark' })
    )
    await waitFor(() =>
      expect(document.documentElement.classList.contains('dark')).toBe(true)
    )

    const colorSchemeSection = colorSchemeGroup.closest('section')
    fireEvent.click(
      within(colorSchemeSection as HTMLElement).getByRole('button', {
        name: 'Reset',
      })
    )

    await waitFor(() =>
      expect(document.documentElement.classList.contains('light')).toBe(true)
    )
    // resetTheme drops the persisted choice so the next visit follows system.
    expect(document.cookie).not.toContain(`${THEME_COOKIE_NAME}=`)
  })
})

describe('theme customizer content layout section', () => {
  it('offers full width and centered options', async () => {
    await openCustomizer()

    const contentLayoutGroup = screen.getByRole('radiogroup', {
      name: 'Content layout',
    })
    expect(
      within(contentLayoutGroup).getByRole('radio', { name: 'Full width' })
    ).toBeInTheDocument()
    expect(
      within(contentLayoutGroup).getByRole('radio', { name: 'Centered' })
    ).toBeInTheDocument()
  })

  it('selecting centered applies the body attribute and persists the cookie', async () => {
    await openCustomizer()

    const contentLayoutGroup = screen.getByRole('radiogroup', {
      name: 'Content layout',
    })
    fireEvent.click(
      within(contentLayoutGroup).getByRole('radio', { name: 'Centered' })
    )

    await waitFor(() =>
      expect(document.body.getAttribute('data-theme-content-layout')).toBe(
        'centered'
      )
    )
    expect(document.cookie).toContain(`${CONTENT_LAYOUT_COOKIE_NAME}=centered`)
  })

  it('the section reset returns to full width and clears the cookie', async () => {
    await openCustomizer()

    const contentLayoutGroup = screen.getByRole('radiogroup', {
      name: 'Content layout',
    })
    fireEvent.click(
      within(contentLayoutGroup).getByRole('radio', { name: 'Centered' })
    )
    await waitFor(() =>
      expect(document.body.getAttribute('data-theme-content-layout')).toBe(
        'centered'
      )
    )

    const contentLayoutSection = contentLayoutGroup.closest('section')
    fireEvent.click(
      within(contentLayoutSection as HTMLElement).getByRole('button', {
        name: 'Reset',
      })
    )

    await waitFor(() =>
      expect(document.body.getAttribute('data-theme-content-layout')).toBe(
        'full'
      )
    )
    expect(document.cookie).not.toContain(`${CONTENT_LAYOUT_COOKIE_NAME}=`)
  })
})
