// Behavior tests for the proxy-log detail sheet's display formatting
// (issue #889): raw ISO timestamps must reach the user as localized
// date-time strings, and the retry counter must distinguish "no retry" (0)
// from "unknown" (—) instead of collapsing both into a falsy branch.

import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ProxyLogDetailSheet } from '../components/proxy-log-detail-sheet'
import type { ProxyLog, ProxyLogDetail } from '../types'

const testState = vi.hoisted(() => ({
  detail: null as ProxyLogDetail | null,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children?: ReactNode }) => <a>{children}</a>,
}))

vi.mock('../api', () => ({
  useProxyLog: () => ({
    data: testState.detail,
    isLoading: false,
    isFetching: false,
  }),
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

afterEach(() => {
  cleanup()
  testState.detail = null
})

// The backend stores created_at as a naive UTC string; the shared formatter
// must parse it as UTC and render it in the viewer's local zone. The
// expected text is computed with the same Intl options the formatter uses,
// so the assertion holds regardless of the CI machine's timezone.
const NAIVE_UTC_CREATED_AT = '2026-08-18 03:04:05'
const NAIVE_UTC_EPOCH_MS = Date.UTC(2026, 7, 18, 3, 4, 5)

function expectedLocalizedDateTime(): string {
  return new Intl.DateTimeFormat('en-US', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(NAIVE_UTC_EPOCH_MS)
}

function makeLog(overrides: Partial<ProxyLog> = {}): ProxyLog {
  return {
    id: 11,
    createdAt: NAIVE_UTC_CREATED_AT,
    status: 'success',
    modelRequested: 'gpt-4o',
    ...overrides,
  } as unknown as ProxyLog
}

describe('ProxyLogDetailSheet datetime display', () => {
  it('renders the created-at timestamp as a localized date-time, not raw ISO', async () => {
    testState.detail = { ...makeLog() } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    await screen.findByText(expectedLocalizedDateTime())
    // Neither the overview field nor the sheet description leaks the raw
    // naive-UTC string anymore.
    expect(screen.queryAllByText(NAIVE_UTC_CREATED_AT)).toHaveLength(0)
  })
})

describe('ProxyLogDetailSheet retry count', () => {
  it('renders a zero retry count as 0 (not a dash)', async () => {
    testState.detail = { ...makeLog(), retryCount: 0 } as ProxyLogDetail

    render(
      <ProxyLogDetailSheet
        log={makeLog({ retryCount: 0 })}
        open
        onOpenChange={() => {}}
      />
    )

    await screen.findByText('Overview')
    expect(screen.getByText('0')).toBeInTheDocument()
  })

  it('renders positive retry counts with the × multiplier', async () => {
    testState.detail = { ...makeLog(), retryCount: 3 } as ProxyLogDetail

    render(
      <ProxyLogDetailSheet
        log={makeLog({ retryCount: 3 })}
        open
        onOpenChange={() => {}}
      />
    )

    await screen.findByText('×3')
  })
})
