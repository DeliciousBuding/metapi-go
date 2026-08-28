// Behavior tests for the proxy-log detail drilldown links (2026-08-18
// multi-perspective review: channel/account/route/token IDs rendered as
// inert `#NNN`). Asserts the route/channel IDs are router links carrying
// the one-shot drilldown search params their target pages consume.

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
    writable: true,
    value: ResizeObserverStub,
  })
})

function makeLog(): ProxyLog {
  return {
    id: 11,
    createdAt: '2026-08-18T00:00:00Z',
    status: 'success',
    modelRequested: 'gpt-5.5',
  } as unknown as ProxyLog
}

afterEach(() => {
  cleanup()
  testState.detail = null
  testState.links = []
})

describe('ProxyLogDetailSheet drilldown links', () => {
  it('links the route id to the token-routes page with a routeId param', async () => {
    testState.detail = {
      ...makeLog(),
      routeId: 5,
      channelId: 8,
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    const routeLink = await screen.findByRole('link', { name: '#5' })
    expect(routeLink).toHaveAttribute('href', '/token-routes')
    expect(testState.links.some((link) => link.search.routeId === 5)).toBe(true)
  })

  it('links the channel id to the channels page with a channelId param', async () => {
    testState.detail = {
      ...makeLog(),
      routeId: 5,
      channelId: 8,
    } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    const channelLink = await screen.findByRole('link', { name: '#8' })
    expect(channelLink).toHaveAttribute('href', '/channels')
    expect(testState.links.some((link) => link.search.channelId === 8)).toBe(
      true
    )
  })

  it('renders a dash instead of a link when the ids are missing', async () => {
    testState.detail = { ...makeLog() } as ProxyLogDetail

    render(<ProxyLogDetailSheet log={makeLog()} open onOpenChange={() => {}} />)

    await screen.findByText(/Overview/i)
    expect(screen.queryAllByRole('link')).toHaveLength(0)
  })
})
