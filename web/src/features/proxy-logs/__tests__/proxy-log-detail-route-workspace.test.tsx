import '@testing-library/jest-dom/vitest'
import { cleanup, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'

import '@/i18n/config'

import { ProxyLogDetailSheet } from '../components/proxy-log-detail-sheet'
import type { ProxyLog, ProxyLogDetail } from '../types'

const testState = vi.hoisted(() => ({
  detail: null as ProxyLogDetail | null,
  links: [] as Array<{ to: string; search: Record<string, unknown> }>,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    to,
    search,
    children,
  }: {
    to: string
    search?: Record<string, unknown>
    children?: ReactNode
  }) => {
    testState.links.push({ to, search: search ?? {} })
    return <a href={to}>{children}</a>
  },
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
    value: ResizeObserverStub,
  })
})

function makeLog(): ProxyLog {
  return {
    id: 11,
    createdAt: '2026-10-15T00:00:00Z',
    status: 'failed',
    modelRequested: 'gpt-5.5',
  } as unknown as ProxyLog
}

afterEach(() => {
  cleanup()
  testState.detail = null
  testState.links = []
})

describe('ProxyLogDetailSheet current route diagnosis', () => {
  it('deep-links the recorded model, protocol, and key into the route workspace', async () => {
    testState.detail = {
      ...makeLog(),
      downstreamKeyId: 7,
      errorMessage: '[downstream:/v1/responses] upstream failed',
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    expect(
      await screen.findByRole('link', { name: 'Open route workspace' })
    ).toBeInTheDocument()
    expect(testState.links).toContainEqual({
      to: '/token-routes',
      search: {
        workspaceModel: 'gpt-5.5',
        workspaceEndpoint: '/v1/responses',
        workspaceKeyId: 7,
      },
    })
  })

  it('does not invent a route-workspace context for an unevaluable protocol', async () => {
    testState.detail = {
      ...makeLog(),
      downstreamKeyId: 7,
      errorMessage: '[downstream:/v1/images/generations] upstream failed',
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    await screen.findByText('Request ID (shared by retry attempts)')
    expect(
      screen.queryByRole('link', { name: 'Open route workspace' })
    ).not.toBeInTheDocument()
  })
})
