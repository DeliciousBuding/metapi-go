import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import { ThemeCustomizationProvider } from '@/context/theme-customization-provider'
import { ThemeProvider } from '@/context/theme-provider'
import '@/i18n/config'

import { SignInPage } from './sign-in-page'

vi.mock('./login-form', () => ({
  LoginForm: () => <div data-testid='login-form' />,
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

describe('sign-in page interface controls', () => {
  it('uses the same language and theme controls as the authenticated shell', () => {
    render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole('button', { name: 'Language' })).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Customize appearance' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Toggle theme' })
    ).toBeInTheDocument()
  })

  it('keeps the login task without deployment instructions or token-source guidance', () => {
    render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    expect(screen.getByRole('heading', { name: 'Metapi' })).toBeVisible()
    expect(screen.getByTestId('login-form')).toBeInTheDocument()
    expect(screen.queryByText(/AUTH_TOKEN|\.env/)).not.toBeInTheDocument()
    expect(
      screen.queryByText(/deployment administrator/i)
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })

  it('shows the token-changed notice only when the reason param says so', () => {
    const { unmount } = render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage noticeReason='tokenChanged' />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )

    expect(
      screen.getByText(
        'The admin token was updated. Sign in with your new token.'
      )
    ).toBeInTheDocument()
    unmount()

    render(
      <ThemeProvider defaultTheme='light'>
        <ThemeCustomizationProvider>
          <SignInPage />
        </ThemeCustomizationProvider>
      </ThemeProvider>
    )
    expect(
      screen.queryByText(
        'The admin token was updated. Sign in with your new token.'
      )
    ).not.toBeInTheDocument()
  })
})
