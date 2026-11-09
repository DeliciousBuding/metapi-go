// metapi-go/context — ThemeProvider focused tests.
// Guards the FOUC-bootstrap cleanup: index.html paints an inline background
// on <html> before React mounts; once the provider owns the theme it must
// drop those inline styles so overscroll reveals the themed body background.

import '@testing-library/jest-dom/vitest'
import {
  act,
  cleanup,
  render,
  renderHook,
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

import { ThemeProvider, useTheme } from '@/context/theme-provider'
import { THEME_SCALE_COOKIE } from '@/lib/theme-scale'

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
  document.cookie = 'vite-ui-theme=; Max-Age=0'
  document.cookie = `${THEME_SCALE_COOKIE}=; Max-Age=0; path=/`
  document.body.removeAttribute('data-theme-scale')
  document.documentElement.classList.remove('light', 'dark')
})

describe('ThemeProvider display size', () => {
  function renderProvider() {
    return renderHook(() => useTheme(), {
      wrapper: ({ children }) => <ThemeProvider>{children}</ThemeProvider>,
    })
  }

  it('retains the stored size and falls back from an unknown value', () => {
    document.cookie = `${THEME_SCALE_COOKIE}=lg; path=/`
    const stored = renderProvider()
    expect(stored.result.current.scale).toBe('lg')
    expect(document.body).toHaveAttribute('data-theme-scale', 'lg')
    stored.unmount()

    document.cookie = `${THEME_SCALE_COOKIE}=unknown; path=/`
    const fallback = renderProvider()
    expect(fallback.result.current.scale).toBe('default')
    expect(document.body).not.toHaveAttribute('data-theme-scale')
  })

  it('persists size changes and restores the default without changing theme', () => {
    const { result } = renderProvider()
    act(() => {
      result.current.setTheme('dark')
      result.current.setScale('xl')
    })
    expect(document.cookie).toContain(`${THEME_SCALE_COOKIE}=xl`)
    expect(document.body).toHaveAttribute('data-theme-scale', 'xl')
    expect(document.documentElement).toHaveClass('dark')

    act(() => result.current.setScale('default'))
    expect(document.cookie).not.toContain(`${THEME_SCALE_COOKIE}=`)
    expect(document.body).not.toHaveAttribute('data-theme-scale')
    expect(document.documentElement).toHaveClass('dark')
  })
})

afterEach(() => cleanup())

describe('ThemeProvider bootstrap cleanup', () => {
  it('removes the index.html inline background once mounted', async () => {
    const root = document.documentElement
    // Simulate the FOUC bootstrap state from index.html.
    root.style.setProperty('--bootstrap-background', '#ffffff')
    root.style.backgroundColor =
      'var(--background, var(--bootstrap-background))'
    expect(root.style.backgroundColor).not.toBe('')

    render(
      <ThemeProvider defaultTheme='light'>
        <div>content</div>
      </ThemeProvider>
    )

    await waitFor(() => {
      expect(root.style.backgroundColor).toBe('')
      expect(root.style.getPropertyValue('--bootstrap-background')).toBe('')
    })
    // The provider still applies the theme class in the same effect.
    expect(root.classList.contains('light')).toBe(true)
  })
})
