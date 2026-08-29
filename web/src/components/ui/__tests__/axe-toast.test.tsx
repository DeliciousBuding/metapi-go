// Component-level axe gate for the Sonner toaster wrapper: a rendered toast
// must produce zero structural violations — the notification lives inside
// sonner's named aria-live region and list markup (jsdom skips
// color-contrast — that stays with the browser-level a11y-scan gate).
import '@testing-library/jest-dom/vitest'
import { act, cleanup, render, screen } from '@testing-library/react'
import axe from 'axe-core'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { Toaster } from '@/components/ui/sonner'
import { toast } from '@/lib/toast'
import '@/i18n/config'

beforeAll(() => {
  // sonner probes prefers-color-scheme under jsdom; report a static query.
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

describe('Toaster axe gate', () => {
  it('rendered success toast produces zero axe violations', async () => {
    const { container } = render(<Toaster />)

    act(() => {
      toast.success('Site saved')
    })
    await screen.findByText('Site saved')

    const results = await axe.run(container)
    expect(results.violations).toEqual([])
  })

  it('announces toasts inside a named live region', async () => {
    const { container } = render(<Toaster />)

    act(() => {
      toast.error('Check-in failed')
    })
    const message = await screen.findByText('Check-in failed')

    // Sonner wraps the toast list in a named section with aria-live.
    const region = container.querySelector('section[aria-label]')
    expect(region).not.toBeNull()
    expect(region!.getAttribute('aria-live')).toBe('polite')
    expect(region).toContainElement(message)
  })
})
