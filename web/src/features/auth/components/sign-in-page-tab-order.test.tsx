import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import '@/i18n/config'

import { SignInPage } from './sign-in-page'

// Pins the sign-in tab order closeout (F-line residual F). The corner
// InterfaceControls used to sit FIRST in the DOM, so the tab cycle was
// controls → form → docs link → (leave the page) → controls: keyboard users
// crossed a browser-chrome/body focus stop between the docs link and the
// controls. Moving the controls to the end of the DOM makes the cycle
// form → docs link → controls continuous. This test pins the resulting
// DOM order of tabbable elements (jsdom's proxy for the tab sequence).

vi.mock('./login-form', () => ({
  LoginForm: () => (
    <>
      <input aria-label='mock token input' />
      <button type='submit'>mock submit</button>
    </>
  ),
}))

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

afterEach(() => cleanup())

const TABBABLE_SELECTOR = [
  'a[href]',
  'button:not([disabled])',
  'input:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(', ')

function accessibleName(element: HTMLElement): string {
  return (
    element.getAttribute('aria-label') ?? element.textContent?.trim() ?? ''
  )
}

describe('sign-in page tab order', () => {
  it('reaches the corner interface controls only after the card content', () => {
    const { container } = render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    const tabOrder = [
      ...container.querySelectorAll<HTMLElement>(TABBABLE_SELECTOR),
    ].map(accessibleName)

    expect(tabOrder).toEqual([
      'mock token input',
      'mock submit',
      'Deployment docs',
      'Language',
      'Customize appearance',
      'Toggle theme',
    ])
  })

  it('keeps the deployment docs link between the form and the controls', () => {
    render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    const docsLink = screen.getByRole('link', { name: 'Deployment docs' })
    const languageButton = screen.getByRole('button', { name: 'Language' })
    const submitButton = screen.getByRole('button', { name: 'mock submit' })

    const position = (element: Element) =>
      [...document.querySelectorAll(TABBABLE_SELECTOR)].indexOf(element)

    expect(position(submitButton)).toBeLessThan(position(docsLink))
    expect(position(docsLink)).toBeLessThan(position(languageButton))
  })
})
