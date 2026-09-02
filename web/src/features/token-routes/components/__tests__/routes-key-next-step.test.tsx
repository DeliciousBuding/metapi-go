// Behavior test for the routes page's journey step 3 → 4 handoff strip.
//
// The strip closes the audit's "routes built, nowhere to go" dead end, and its
// whole value depends on WHEN it shows. Locks the four visibility states:
//   (a) routes exist + zero keys  → the CTA renders and points at
//       /downstream-keys (the first-class page the left nav links);
//   (b) at least one key exists   → silent, so a finished setup is never
//       nagged;
//   (c) count still loading       → silent (no flash of a false gap);
//   (d) count failed to load      → silent (never claim "no keys" from an
//       unanswered request).

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import { api } from '@/lib/api'

import { RoutesKeyNextStep } from '../routes-key-next-step'

vi.mock('@/lib/api', () => ({
  api: {
    getDownstreamApiKeys: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to?: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

const mockGetKeys = vi.mocked(api.getDownstreamApiKeys)

function renderStrip() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <RoutesKeyNextStep />
    </QueryClientProvider>
  )
}

beforeEach(() => {
  mockGetKeys.mockReset()
})

afterEach(() => cleanup())

describe('RoutesKeyNextStep', () => {
  it('offers the downstream-keys CTA when routes exist but no key is issued', async () => {
    mockGetKeys.mockResolvedValue({ items: [] })

    renderStrip()

    const cta = await screen.findByRole('link', {
      name: /Issue a downstream key/i,
    })
    expect(cta).toHaveAttribute('href', '/downstream-keys')
    expect(screen.getByText(/no downstream key yet/i)).toBeInTheDocument()
  })

  it('stays silent once at least one key exists', async () => {
    mockGetKeys.mockResolvedValue({
      items: [{ id: 1, name: 'prod', enabled: true }],
    })

    renderStrip()

    await waitFor(() => expect(mockGetKeys).toHaveBeenCalledTimes(1))
    expect(
      screen.queryByRole('link', { name: /Issue a downstream key/i })
    ).not.toBeInTheDocument()
  })

  it('does not flash the CTA while the key count is still loading', () => {
    // Never settles: the strip must render nothing rather than treat the
    // unanswered count as zero.
    mockGetKeys.mockReturnValue(new Promise(() => {}) as never)

    renderStrip()

    expect(
      screen.queryByRole('link', { name: /Issue a downstream key/i })
    ).not.toBeInTheDocument()
  })

  it('does not claim a gap when the key count fails to load', async () => {
    mockGetKeys.mockRejectedValue(new Error('keys unavailable'))

    renderStrip()

    await waitFor(() => expect(mockGetKeys).toHaveBeenCalledTimes(1))
    expect(
      screen.queryByRole('link', { name: /Issue a downstream key/i })
    ).not.toBeInTheDocument()
  })
})
