// Behavior test for useRealtimeOps over the module-level shared connection.
//
// Covers issue #889 (perf & stability): every component that calls
// useRealtimeOps (today snapshot strip + availability panel) must subscribe
// to ONE shared WebSocket — the socket opens on the first hook mount, stays
// open while any hook instance remains, closes when the last unmounts, and
// reopens cleanly for the next mount. Previously each component instance
// dialed its own socket and dashboard tab switches tore down + re-dialed.
//
// The global WebSocket is stubbed with a controllable fake; the ws-ticket
// mint endpoint (#1034) is mocked at the transport layer so the shared
// connection's async ticket exchange resolves deterministically.

import '@testing-library/jest-dom/vitest'
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

// The shared connection mints one-time tickets via transport.request before
// dialing (#1034); stub the endpoint so no real HTTP happens in tests.
vi.mock('@/lib/api/transport', () => ({
  request: vi.fn(async () => ({ ticket: 'test-ticket', expiresInSeconds: 60 })),
  buildQueryString: vi.fn(() => ''),
}))

import { useRealtimeOps } from '../use-realtime-ops'

type CloseHandler = ((event?: unknown) => void) | null
type MessageHandler = ((event: { data: string }) => void) | null

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3

  static instances: FakeWebSocket[] = []

  url: string
  readyState = FakeWebSocket.CONNECTING
  onopen: CloseHandler = null
  onmessage: MessageHandler = null
  onerror: CloseHandler = null
  onclose: CloseHandler = null
  close = vi.fn(() => {
    this.readyState = FakeWebSocket.CLOSED
  })

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  open() {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.()
  }

  sendFrame(frame: unknown) {
    this.onmessage?.({ data: JSON.stringify(frame) })
  }
}

/** Flush the async ticket exchange so dial side effects become visible. */
async function flushDial() {
  await act(async () => {
    for (let i = 0; i < 8; i += 1) {
      await Promise.resolve()
    }
  })
}

beforeEach(() => {
  FakeWebSocket.instances = []
  vi.stubGlobal('WebSocket', FakeWebSocket)
})

afterEach(() => {
  localStorage.clear()
  vi.unstubAllGlobals()
})

describe('useRealtimeOps shared connection', () => {
  it('opens a single socket for multiple concurrent hook instances', async () => {
    const firstHook = renderHook(() => useRealtimeOps())
    const secondHook = renderHook(() => useRealtimeOps())
    await flushDial()

    expect(FakeWebSocket.instances).toHaveLength(1)
    // #1034: the dial carries a one-time ticket, never a token query param.
    expect(FakeWebSocket.instances[0].url).toContain('ticket=test-ticket')

    // Both instances observe the same shared state.
    act(() => {
      FakeWebSocket.instances[0].open()
    })
    expect(firstHook.result.current.sample.connected).toBe(true)
    expect(secondHook.result.current.sample.connected).toBe(true)

    firstHook.unmount()
    // One subscriber remains — the socket must stay open.
    expect(FakeWebSocket.instances[0].close).not.toHaveBeenCalled()

    secondHook.unmount()
    // Last subscriber gone — the socket closes.
    expect(FakeWebSocket.instances[0].close).toHaveBeenCalledTimes(1)
  })

  it('reopens the shared socket after all instances unmounted', async () => {
    const firstHook = renderHook(() => useRealtimeOps())
    await flushDial()
    firstHook.unmount()
    expect(FakeWebSocket.instances).toHaveLength(1)
    expect(FakeWebSocket.instances[0].close).toHaveBeenCalledTimes(1)

    const secondHook = renderHook(() => useRealtimeOps())
    await flushDial()
    expect(FakeWebSocket.instances).toHaveLength(2)
    secondHook.unmount()
  })

  it('surfaces frames and the last-frame freshness to every instance', async () => {
    const firstHook = renderHook(() => useRealtimeOps())
    const secondHook = renderHook(() => useRealtimeOps())
    await flushDial()

    act(() => {
      const socket = FakeWebSocket.instances[0]
      socket.open()
      socket.sendFrame({
        lifetime: 30,
        uptimeSeconds: 600,
        points: [{ ts: 1, total: 12, success: 12 }],
      })
    })

    expect(firstHook.result.current.lastFrameAt).not.toBeNull()
    expect(secondHook.result.current.sample.lifetime).toBe(30)
    expect(secondHook.result.current.sample.uptimeSeconds).toBe(600)
    expect(secondHook.result.current.sample.spark).toHaveLength(1)

    firstHook.unmount()
    secondHook.unmount()
  })
})
