// Behavior test for the three downstream-key quota hints.
//
// maxRequests / maxCost / expiresAt rendered as bare labels with an
// "Unlimited" placeholder, so the operator had to guess the unit, the window
// and the timezone. The copy below is transcribed from the enforcement path,
// not invented — this test locks each load-bearing fact so the hints cannot
// drift back into vagueness (or away from the implementation):
//
//   maxRequests  auth/downstream.go: `used_requests >= max_requests` → 429.
//                A LIFETIME counter consumed by consumeManagedKeyRequest, not
//                a rate window (per-minute limiting is the separate max_rpm /
//                max_tpm columns, which this form does not expose).
//   maxCost      auth/downstream.go: `used_cost >= max_cost` → 429, where
//                used_cost accumulates billing.EstimatedCost — USD, per
//                service/pricing (USD-per-1M rates), recorded only on
//                successful requests (handler/proxy/proxy_log.go).
//   expiresAt    auth/downstream.go: past `time.Now()` → 403 key_expired,
//                compared against the server clock; the wire format is UTC
//                (see key-expires-at-wire.test.ts).
//   zero/empty   handler/admin/downstream_keys_normalize.go:
//                normalizeQuota{Float,Int}OrNull map null / 0 / "" / negatives
//                to NULL = unlimited.

import '@testing-library/jest-dom/vitest'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactElement } from 'react'
import {
  afterEach,
  beforeAll,
  beforeEach,
  describe,
  expect,
  it,
  vi,
} from 'vitest'

import '@/i18n/config'
import { Sheet, SheetContent } from '@/components/ui/sheet'

import { KeySheetForm } from '../keys-section'

const { mockGetSites } = vi.hoisted(() => ({ mockGetSites: vi.fn() }))

vi.mock('@/lib/api', () => ({
  api: {
    createDownstreamApiKey: vi.fn(),
    updateDownstreamApiKey: vi.fn(),
    getSites: mockGetSites,
    getAccountsSnapshot: vi
      .fn()
      .mockResolvedValue({ accounts: [], sites: [], generatedAt: '' }),
    getAccountTokens: vi.fn().mockResolvedValue([]),
  },
}))

vi.mock('@/lib/toast', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

beforeAll(() => {
  // base-ui primitives query matchMedia on render; jsdom leaves it undefined.
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
  mockGetSites.mockReset()
  mockGetSites.mockResolvedValue([])
})

afterEach(() => cleanup())

function renderCreateForm() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: 0 } },
  })
  return render(
    (
      <QueryClientProvider client={queryClient}>
        <Sheet open onOpenChange={() => {}}>
          <SheetContent>
            <KeySheetForm editingKey={null} onDone={vi.fn()} />
          </SheetContent>
        </Sheet>
      </QueryClientProvider>
    ) as ReactElement
  )
}

/** The hint text a given field exposes to assistive tech (aria-describedby). */
function describedByText(label: string): string {
  const describedBy = screen
    .getByLabelText(label)
    .getAttribute('aria-describedby')
  expect(describedBy).toBeTruthy()
  const nodes = (describedBy ?? '')
    .split(/\s+/)
    .map((id) => document.getElementById(id))
    .filter((node): node is HTMLElement => node !== null)
  return nodes.map((node) => node.textContent ?? '').join(' ')
}

describe('KeySheetForm quota hints', () => {
  it('explains max requests as a lifetime total gated by 429', () => {
    renderCreateForm()

    const hint = describedByText('Max requests')
    // Window semantics: the counter is cumulative, not per-minute.
    expect(hint).toMatch(/lifetime total/i)
    expect(hint).toMatch(/not a per-minute window/i)
    // Zero/empty contract + the status code the operator will actually see.
    expect(hint).toMatch(/empty or 0 for unlimited/i)
    expect(hint).toMatch(/429/)
  })

  it('explains max cost as a USD lifetime cap gated by 429', () => {
    renderCreateForm()

    const hint = describedByText('Max cost')
    // Currency unit — the field carried no unit at all before.
    expect(hint).toMatch(/USD/)
    expect(hint).toMatch(/lifetime spend cap/i)
    expect(hint).toMatch(/only successful requests are billed/i)
    expect(hint).toMatch(/empty or 0 for unlimited/i)
    expect(hint).toMatch(/429/)
  })

  it('explains expires at as a UTC instant gated by 403', () => {
    renderCreateForm()

    const hint = describedByText('Expires at')
    // Input is local wall-clock, comparison is UTC — the mismatch that made
    // the field silently unenforceable before the wire-format fix.
    expect(hint).toMatch(/local time/i)
    expect(hint).toMatch(/UTC/)
    expect(hint).toMatch(/403/)
    expect(hint).toMatch(/no expiry/i)
  })

  it('keeps the placeholder and the hints agreeing on "unlimited"', () => {
    renderCreateForm()

    // The placeholder was the ONLY semantics these fields had before; the
    // hints must not contradict it (settings.common.unlimited = "unlimited").
    expect(screen.getByLabelText('Max requests')).toHaveAttribute(
      'placeholder',
      'unlimited'
    )
    expect(screen.getByLabelText('Max cost')).toHaveAttribute(
      'placeholder',
      'unlimited'
    )
  })
})
