// Behavior test for the credential-export dialog's verify entry. The
// onboarding journey's final step ("prove the key works") lives INSIDE the
// dialog: after the master-token unlock the dialog holds the plain key, and
// the verify section runs the two real requests from the browser (the real
// client path). The old footer escape hatch to the channel-level model
// tester was removed — it tested channels with admin credentials, never the
// key the operator just created.
//
// Mocks react-query (stable data surface) and the router Link (assertion
// targets user-visible markup, not router context).

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
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
  Link: ({
    to,
    search,
    children,
  }: {
    to: string
    search?: Record<string, unknown>
    children: ReactNode
  }) => {
    const query = search
      ? '?' + new URLSearchParams(search as Record<string, string>).toString()
      : ''
    return <a href={to + query}>{children}</a>
  },
}))

// Toast helpers pull in sonner; stub them so no portal side effects run.
vi.mock('@/lib/toast', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const exportData = {
  success: true,
  formatVersion: '1',
  keyId: 1,
  keyName: 'demo-key',
  baseUrl: 'http://gateway.example.com',
  profiles: [
    {
      id: 'generic',
      label: 'Generic JSON',
      content: { apiKey: 'sk-plain-demo' },
    },
  ],
}

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

function renderDialog() {
  mockUseQuery.mockReturnValue({
    isLoading: false,
    isError: false,
    data: exportData,
    refetch: vi.fn(),
  })
  render(
    <CredentialExportDialog
      target={{ id: 1, name: 'demo-key', keyMasked: 'sk-***' }}
      onOpenChange={() => {}}
    />
  )
  // Unlock: the verify section (and the profiles) only render once the
  // master token is presented.
  fireEvent.change(screen.getByPlaceholderText('Paste the admin token'), {
    target: { value: 'master-token' },
  })
  fireEvent.click(screen.getByRole('button', { name: 'Unlock' }))
}

describe('CredentialExportDialog verify entry', () => {
  it('renders the verify section after unlock, and no channel-tester link', async () => {
    renderDialog()

    expect(await screen.findByText('Verify the connection')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Check available models' })
    ).toBeInTheDocument()
    // The old footer escape hatch tested channels with admin credentials —
    // it must not come back next to a key-scoped verify.
    expect(
      screen.queryByRole('link', { name: 'Send a test request' })
    ).toBeNull()
  })
})
