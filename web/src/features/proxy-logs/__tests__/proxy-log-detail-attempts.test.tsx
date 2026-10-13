// Behavior tests for the request-attempt view in the proxy-log detail sheet
// (route-decision D batch): sibling attempts correlated by requestId, terminal
// outcome from the highest-ordinal row, and honest partial markers.

import '@testing-library/jest-dom/vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'
import type { ProxyLogAttempt, ProxyLogRequestAttempts } from '@/lib/api'

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

function makeLog(): ProxyLog {
  return {
    id: 11,
    createdAt: '2026-10-13T00:00:00Z',
    status: 'success',
    modelRequested: 'gpt-6',
  } as unknown as ProxyLog
}

function makeAttempt(over: Partial<ProxyLogAttempt>): ProxyLogAttempt {
  return {
    id: 11,
    ordinal: 0,
    status: 'success',
    httpStatus: 200,
    channelId: 5,
    siteId: 1,
    siteName: 'site-a',
    accountUsername: 'user-a',
    modelActual: 'gpt-6',
    errorMessage: '',
    latencyMs: 120,
    createdAt: '2026-10-13T00:00:00Z',
    isCurrent: false,
    ...over,
  }
}

function makeAttempts(
  over: Partial<ProxyLogRequestAttempts>
): ProxyLogRequestAttempts {
  return {
    requestId: 'req-1',
    currentOrdinal: 0,
    partial: false,
    partialReasons: [],
    attempts: [],
    terminal: { status: 'success', httpStatus: 200, logId: 13 },
    ...over,
  }
}

afterEach(() => {
  cleanup()
  testState.detail = null
})

describe('ProxyLogDetailSheet request attempts', () => {
  it('lists sibling attempts in ordinal order with terminal outcome and current marker', async () => {
    const attempts = [
      makeAttempt({
        id: 11,
        ordinal: 0,
        status: 'failed',
        httpStatus: 502,
        errorMessage: 'upstream boom',
        isCurrent: true,
      }),
      makeAttempt({
        id: 12,
        ordinal: 1,
        status: 'failed',
        httpStatus: 429,
        siteName: 'site-b',
        errorMessage: 'rate limited',
      }),
      makeAttempt({
        id: 13,
        ordinal: 2,
        status: 'success',
        httpStatus: 200,
        siteName: 'site-c',
      }),
    ]
    testState.detail = {
      ...makeLog(),
      requestAttempts: makeAttempts({ attempts, currentOrdinal: 0 }),
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    expect(await screen.findByText('Request attempts')).toBeInTheDocument()
    expect(screen.getByText('This request made 3 attempts')).toBeInTheDocument()
    expect(screen.getByText('#1')).toBeInTheDocument()
    expect(screen.getByText('#3')).toBeInTheDocument()
    expect(screen.getByText('Current')).toBeInTheDocument()
    expect(screen.getByText('site-b')).toBeInTheDocument()
    expect(screen.getByText('rate limited')).toBeInTheDocument()
    expect(screen.queryByText('Incomplete record')).not.toBeInTheDocument()
  })

  it('switches the viewed log when a sibling attempt is clicked', async () => {
    const attempts = [
      makeAttempt({ id: 11, ordinal: 0, status: 'failed', httpStatus: 502 }),
      makeAttempt({
        id: 12,
        ordinal: 1,
        status: 'success',
        httpStatus: 200,
        isCurrent: true,
      }),
    ]
    testState.detail = {
      ...makeLog(),
      requestAttempts: makeAttempts({ attempts, currentOrdinal: 1 }),
    } as ProxyLogDetail
    const onViewAttempt = vi.fn()

    render(
      <ProxyLogDetailSheet
        log={makeLog()}
        open
        onOpenChange={() => {}}
        onViewAttempt={onViewAttempt}
      />
    )

    const currentButton = screen.getByText('#2').closest('button')
    expect(currentButton).toBeDisabled()
    const firstButton = screen.getByText('#1').closest('button')
    if (!firstButton) throw new Error('attempt row button missing')
    expect(firstButton).not.toBeDisabled()
    fireEvent.click(firstButton)
    expect(onViewAttempt).toHaveBeenCalledWith(
      expect.objectContaining({ id: 11, ordinal: 0 })
    )
  })

  it('marks ordinal gaps as incomplete with the honest reason', async () => {
    const attempts = [
      makeAttempt({
        id: 11,
        ordinal: 0,
        status: 'failed',
        httpStatus: 502,
        isCurrent: true,
      }),
      makeAttempt({ id: 13, ordinal: 2, status: 'failed', httpStatus: 502 }),
    ]
    testState.detail = {
      ...makeLog(),
      requestAttempts: makeAttempts({
        attempts,
        partial: true,
        partialReasons: ['ordinalGap'],
        terminal: { status: 'failed', httpStatus: 502, logId: 13 },
      }),
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    expect(await screen.findByText('Incomplete record')).toBeInTheDocument()
    expect(
      screen.getByText(/ordinals have gaps|left no row/)
    ).toBeInTheDocument()
  })

  it('shows the uncorrelated note instead of a list when requestId is missing', async () => {
    testState.detail = {
      ...makeLog(),
      requestAttempts: makeAttempts({
        requestId: null,
        attempts: [],
        partial: true,
        partialReasons: ['requestIdMissing'],
      }),
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    expect(await screen.findByText('Incomplete record')).toBeInTheDocument()
    expect(
      screen.getByText('Sibling attempts cannot be correlated')
    ).toBeInTheDocument()
    expect(screen.queryByText('#1')).not.toBeInTheDocument()
  })
})
