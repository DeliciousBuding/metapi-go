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
import { ThemeProvider } from '@/context/theme-provider'
import '@/i18n/config'

beforeAll(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
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
  document.cookie = 'vite-ui-theme=; Max-Age=0; path=/'
  document.cookie = 'theme_scale=; Max-Age=0; path=/'
  document.body.removeAttribute('data-theme-scale')
  document.documentElement.classList.remove('light', 'dark')
})

afterEach(() => cleanup())

async function openCustomizer() {
  render(
    <ThemeProvider>
      <ThemeCustomizer />
    </ThemeProvider>
  )
  fireEvent.click(screen.getByRole('button', { name: 'Customize appearance' }))
  return screen.findByRole('radiogroup', { name: 'Color scheme' })
}

describe('appearance settings', () => {
  it('offers only color scheme and display size, with readable selected states', async () => {
    const schemes = await openCustomizer()
    expect(
      screen.getByRole('dialog', { name: 'Appearance' })
    ).toBeInTheDocument()
    expect(screen.getAllByRole('radiogroup')).toHaveLength(2)
    expect(within(schemes).getAllByRole('radio')).toHaveLength(3)
    expect(within(schemes).getByRole('radio', { name: 'System' })).toBeChecked()

    const sizes = screen.getByRole('radiogroup', { name: 'Display size' })
    expect(within(sizes).getAllByRole('radio')).toHaveLength(4)
    expect(within(sizes).getByRole('radio', { name: 'Default' })).toBeChecked()
  })

  it('changes theme immediately and returns to following the system', async () => {
    const schemes = await openCustomizer()
    fireEvent.click(within(schemes).getByRole('radio', { name: 'Dark' }))
    await waitFor(() => expect(document.documentElement).toHaveClass('dark'))
    expect(document.cookie).toContain('vite-ui-theme=dark')

    fireEvent.click(within(schemes).getByRole('radio', { name: 'System' }))
    await waitFor(() => expect(document.documentElement).toHaveClass('light'))
    expect(within(schemes).getByRole('radio', { name: 'System' })).toBeChecked()
  })

  it('retains display size after remount and restores the default independently', async () => {
    await openCustomizer()
    let sizes = screen.getByRole('radiogroup', { name: 'Display size' })
    fireEvent.click(within(sizes).getByRole('radio', { name: 'Large' }))
    expect(document.body).toHaveAttribute('data-theme-scale', 'xl')
    expect(document.cookie).toContain('theme_scale=xl')

    cleanup()
    await openCustomizer()
    sizes = screen.getByRole('radiogroup', { name: 'Display size' })
    expect(within(sizes).getByRole('radio', { name: 'Large' })).toBeChecked()
    fireEvent.click(within(sizes).getByRole('radio', { name: 'Default' }))
    expect(document.body).not.toHaveAttribute('data-theme-scale')
    expect(document.cookie).not.toContain('theme_scale=')
  })
})
