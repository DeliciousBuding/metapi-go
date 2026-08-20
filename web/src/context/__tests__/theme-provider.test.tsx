// metapi-go/context — ThemeProvider focused tests.
// Guards the FOUC-bootstrap cleanup: index.html paints an inline background
// on <html> before React mounts; once the provider owns the theme it must
// drop those inline styles so overscroll reveals the themed body background.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

import { ThemeProvider } from '@/context/theme-provider'

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
  document.documentElement.classList.remove('light', 'dark')
})

afterEach(() => cleanup())

describe('ThemeProvider bootstrap cleanup', () => {
  it('removes the index.html inline background once mounted', async () => {
    const root = document.documentElement
    // Simulate the FOUC bootstrap state from index.html.
    root.style.setProperty('--bootstrap-background', '#ffffff')
    root.style.backgroundColor = 'var(--background, var(--bootstrap-background))'
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
