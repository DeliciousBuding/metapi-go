// Behavior test for the credential-export dialog footer. The onboarding
// journey's final step ("make a test request") must not dead-end: the
// footer carries a secondary link to the model tester playground so a
// user can send a probe request without hunting for it in the sidebar.
//
// Mocks react-query (stable loading surface — the footer renders regardless
// of the export fetch state) and the router Link (assertion targets
// user-visible footer markup, not router context).

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { CredentialExportDialog } from '../credential-export-dialog'

const { mockUseQuery } = vi.hoisted(() => ({
  mockUseQuery: vi.fn(),
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: mockUseQuery,
}))

// The real TanStack Router Link needs a router context; render a plain
// anchor so the href is queryable via role=link + accessible name.
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

// Toast helpers pull in sonner; stub them so no portal side effects run.
vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

beforeAll(() => {
  // Base UI primitives query matchMedia for reduced-motion/pointer tests.
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

describe('CredentialExportDialog', () => {
  it('renders a test-request link to /model-tester in the footer', async () => {
    mockUseQuery.mockReturnValue({
      isLoading: true,
      isError: false,
      data: undefined,
      refetch: vi.fn(),
    })

    render(
      <CredentialExportDialog
        target={{ id: 1, name: 'demo-key', keyMasked: 'sk-***' }}
        onOpenChange={() => {}}
      />
    )

    const testRequestLink = await screen.findByRole('link', {
      name: 'Send a test request',
    })
    expect(testRequestLink).toBeInTheDocument()
    expect(testRequestLink).toHaveAttribute('href', '/model-tester')
  })
})
