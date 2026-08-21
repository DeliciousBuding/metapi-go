// metapi-go/layout — header user menu tests.
// Covers the menu content (version, About, documentation, sign-out) and the
// sign-out side effects (session storage cleared + navigate to /sign-in).

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import { ABOUT_INFO } from '@/features/about/api'
import i18n from '@/i18n/config'

import { UserMenu } from '../components/user-menu'

const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
    // Keep Base UI's render props (role, id, data attributes) on the anchor
    // so accessibility-role queries work against menu items.
    Link: ({
      to,
      children,
      ...rest
    }: { to?: unknown; children?: ReactNode } & Record<string, unknown>) => (
      <a href={typeof to === 'string' ? to : '/'} {...rest}>
        {children}
      </a>
    ),
  }
})

// The menu primitive probes browser APIs jsdom does not implement.
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

afterAll(() => {
  vi.restoreAllMocks()
})

beforeEach(async () => {
  navigateMock.mockReset()
  localStorage.clear()
  await i18n.changeLanguage('en')
})

afterEach(() => cleanup())

async function openMenu() {
  render(<UserMenu />)
  fireEvent.click(screen.getByRole('button', { name: 'User menu' }))
  return screen.findByRole('menu')
}

describe('user menu', () => {
  it('shows the product version, About, documentation and sign-out entries', async () => {
    await openMenu()

    expect(
      screen.getByText(`Metapi v${ABOUT_INFO.version}`)
    ).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /About/ })).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: /Documentation/ })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('menuitem', { name: /Sign out/ })
    ).toBeInTheDocument()
  })

  it('links the documentation entry to the project homepage', async () => {
    await openMenu()

    const docsItem = screen.getByRole('menuitem', { name: /Documentation/ })
    expect(docsItem).toHaveAttribute('href', ABOUT_INFO.homepage)
    expect(docsItem).toHaveAttribute('target', '_blank')
  })

  it('clears the persisted session and navigates to sign-in on sign-out', async () => {
    localStorage.setItem('auth_token', 'test-token')
    localStorage.setItem('auth_token_expires_at', String(Date.now() + 60_000))

    await openMenu()
    fireEvent.click(screen.getByRole('menuitem', { name: /Sign out/ }))

    expect(localStorage.getItem('auth_token')).toBeNull()
    expect(localStorage.getItem('auth_token_expires_at')).toBeNull()
    expect(navigateMock).toHaveBeenCalledWith({ to: '/sign-in' })
  })
})
