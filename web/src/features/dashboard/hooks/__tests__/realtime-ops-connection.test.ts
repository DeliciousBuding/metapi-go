// Behavior tests for the shared realtime ops WebSocket connection.
//
// Covers issue #889 (perf & stability):
//   (a) self-healing — the connection never gives up permanently. After
//       MAX_FAILS=5 consecutive failures it keeps retrying on a slow 30s
//       cadence, and visibilitychange(visible) / online events reset the
//       failure accounting and re-dial immediately;
//   (b) shared single instance — the socket opens on the first subscriber,
//       survives while any subscriber remains, and closes only when the last
//       one leaves (dashboard tab switches no longer re-dial);
//   (c) hidden-tab pause — while the tab is hidden no socket is held open,
//       and becoming visible re-dials immediately;
//   (d) freshness — lastFrameAt records the last frame so panels can render
//       a "data as of" marker during gaps.
//
// The WebSocket is a controllable fake injected through the factory; timers
// are faked so backoff delays are advanced deterministically.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  createRealtimeOpsConnection,
  type RealtimeOpsConnection,
} from '../realtime-ops-connection'

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

  /** Simulate a failed/dropped connection (fires onclose). */
  fail() {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.()
  }

  sendFrame(frame: unknown) {
    this.onmessage?.({ data: JSON.stringify(frame) })
  }
}

const NOOP_LISTENER = () => {}

const FIXED_NOW = new Date('2026-01-15T12:00:00Z').getTime()

function createConnection(): RealtimeOpsConnection {
  return createRealtimeOpsConnection({
    getToken: () => 'test-token',
    createWebSocket: (url) => new FakeWebSocket(url) as unknown as WebSocket,
  })
}

function setVisibility(state: 'visible' | 'hidden') {
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => state,
  })
  document.dispatchEvent(new Event('visibilitychange'))
}

function latestSocket(): FakeWebSocket {
  const instance = FakeWebSocket.instances.at(-1)
  if (!instance) throw new Error('expected a WebSocket instance')
  return instance
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(FIXED_NOW)
  FakeWebSocket.instances = []
})

afterEach(() => {
  setVisibility('visible')
  vi.useRealTimers()
})

describe('realtime ops shared connection — self-healing', () => {
  it('retries with escalating backoff after failures', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    expect(FakeWebSocket.instances).toHaveLength(1)

    latestSocket().fail()
    expect(FakeWebSocket.instances).toHaveLength(1)

    vi.advanceTimersByTime(1_999)
    expect(FakeWebSocket.instances).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(2)

    latestSocket().fail()
    vi.advanceTimersByTime(3_999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(3)

    unsubscribe()
  })

  it('never gives up permanently: slow retries continue after MAX_FAILS', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    // Fail attempts 1-4 (backoffs 2s, 4s, 8s, 15s) — still escalating.
    for (const delayMs of [2_000, 4_000, 8_000, 15_000]) {
      latestSocket().fail()
      vi.advanceTimersByTime(delayMs)
    }
    expect(FakeWebSocket.instances).toHaveLength(5)

    // Failure #5 crosses MAX_FAILS → gaveUp flag, but the retry clock keeps
    // running at the slow cadence instead of stopping forever.
    latestSocket().fail()
    expect(connection.getSnapshot().sample.gaveUp).toBe(true)

    vi.advanceTimersByTime(29_999)
    expect(FakeWebSocket.instances).toHaveLength(5)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(6)

    // The slow cadence repeats indefinitely…
    latestSocket().fail()
    expect(connection.getSnapshot().sample.gaveUp).toBe(true)
    vi.advanceTimersByTime(30_000)
    expect(FakeWebSocket.instances).toHaveLength(7)

    // …and a successful reconnect clears the gave-up state.
    latestSocket().open()
    const sample = connection.getSnapshot().sample
    expect(sample.connected).toBe(true)
    expect(sample.gaveUp).toBe(false)

    unsubscribe()
  })

  it('re-dials immediately when the tab becomes visible after failures', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    latestSocket().fail()
    latestSocket() // still attempt #1
    expect(FakeWebSocket.instances).toHaveLength(1)

    // Visible again → failure accounting resets and a new attempt fires
    // without waiting out the backoff.
    setVisibility('visible')
    expect(FakeWebSocket.instances).toHaveLength(2)

    latestSocket().open()
    expect(connection.getSnapshot().sample.connected).toBe(true)

    unsubscribe()
  })

  it('re-dials immediately on the online event after failures', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    latestSocket().fail()
    expect(FakeWebSocket.instances).toHaveLength(1)

    window.dispatchEvent(new Event('online'))
    expect(FakeWebSocket.instances).toHaveLength(2)

    unsubscribe()
  })
})

describe('realtime ops shared connection — hidden-tab pause', () => {
  it('closes the socket while hidden and re-dials on visible', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)
    latestSocket().open()
    expect(connection.getSnapshot().sample.connected).toBe(true)

    // Hide the tab: the live socket closes without counting as a failure.
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'hidden',
    })
    document.dispatchEvent(new Event('visibilitychange'))

    expect(latestSocket().close).toHaveBeenCalledTimes(1)
    expect(connection.getSnapshot().sample.connected).toBe(false)

    // No reconnect churn while hidden.
    vi.advanceTimersByTime(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)

    setVisibility('visible')
    expect(FakeWebSocket.instances).toHaveLength(2)
    latestSocket().open()
    expect(connection.getSnapshot().sample.connected).toBe(true)

    unsubscribe()
  })

  it('does not dial at all while subscribed in a hidden tab', () => {
    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'hidden',
    })

    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)
    expect(FakeWebSocket.instances).toHaveLength(0)

    setVisibility('visible')
    expect(FakeWebSocket.instances).toHaveLength(1)

    unsubscribe()
  })

  it('does not count a hidden-pause close as a failure', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)
    latestSocket().open()

    Object.defineProperty(document, 'visibilityState', {
      configurable: true,
      get: () => 'hidden',
    })
    document.dispatchEvent(new Event('visibilitychange'))

    setVisibility('visible')
    // If the pause had counted as a failure the next backoff would be 4s;
    // a clean slate means the first failure still schedules 2s.
    latestSocket().fail()
    vi.advanceTimersByTime(1_999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    vi.advanceTimersByTime(1)
    expect(FakeWebSocket.instances).toHaveLength(3)

    unsubscribe()
  })
})

describe('realtime ops shared connection — reference counting', () => {
  it('opens one socket for many subscribers and closes on the last unsubscribe', () => {
    const connection = createConnection()

    // Distinct listener identities: a Set dedupes identical references.
    const unsubscribeFirst = connection.subscribe(() => {})
    const unsubscribeSecond = connection.subscribe(() => {})
    const unsubscribeThird = connection.subscribe(() => {})
    expect(FakeWebSocket.instances).toHaveLength(1)

    unsubscribeFirst()
    unsubscribeSecond()
    expect(latestSocket().close).not.toHaveBeenCalled()

    unsubscribeThird()
    expect(latestSocket().close).toHaveBeenCalledTimes(1)
  })

  it('keeps the last sample + freshness after the last subscriber leaves', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)
    latestSocket().open()
    latestSocket().sendFrame({
      lifetime: 42,
      uptimeSeconds: 900,
      points: [{ ts: 1, total: 100, success: 99 }],
    })

    const liveSnapshot = connection.getSnapshot()
    expect(liveSnapshot.lastFrameAt).toBe(FIXED_NOW)
    expect(liveSnapshot.sample.qps).toBe(0)

    unsubscribe()

    const finalSnapshot = connection.getSnapshot()
    expect(finalSnapshot.sample.connected).toBe(false)
    // Stale data stays visible with its freshness marker for the next
    // session instead of flashing empty.
    expect(finalSnapshot.lastFrameAt).toBe(FIXED_NOW)
    expect(finalSnapshot.sample.lifetime).toBe(42)
    expect(finalSnapshot.sample.uptimeSeconds).toBe(900)
  })

  it('reopens a fresh socket when subscribers return after idle', () => {
    const connection = createConnection()

    const unsubscribeFirst = connection.subscribe(NOOP_LISTENER)
    unsubscribeFirst()
    expect(latestSocket().close).toHaveBeenCalledTimes(1)

    const unsubscribeSecond = connection.subscribe(NOOP_LISTENER)
    expect(FakeWebSocket.instances).toHaveLength(2)
    unsubscribeSecond()
  })
})

describe('realtime ops shared connection — frames & freshness', () => {
  it('derives qps deltas, success rate and lastFrameAt from frames', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)
    latestSocket().open()

    latestSocket().sendFrame({
      lifetime: 10,
      points: [{ ts: 1, total: 100, success: 90 }],
    })
    // First frame has no previous total → qps 0, not an invented spike.
    expect(connection.getSnapshot().sample.qps).toBe(0)
    expect(connection.getSnapshot().lastFrameAt).toBe(FIXED_NOW)

    vi.advanceTimersByTime(1_000)
    latestSocket().sendFrame({
      lifetime: 11,
      points: [{ ts: 2, total: 107, success: 105 }],
    })
    const sample = connection.getSnapshot().sample
    expect(sample.qps).toBe(7)
    expect(sample.successRate).toBeCloseTo(105 / 107)
    expect(sample.connected).toBe(true)
    expect(connection.getSnapshot().lastFrameAt).toBe(FIXED_NOW + 1_000)

    unsubscribe()
  })

  it('manual reconnect resets backoff and dials immediately', () => {
    const connection = createConnection()
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    // One failure leaves a backoff timer pending; manual reconnect must
    // bypass it and dial right away.
    latestSocket().fail()
    expect(FakeWebSocket.instances).toHaveLength(1)

    connection.reconnect()
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(connection.getSnapshot().sample.gaveUp).toBe(false)

    unsubscribe()
  })

  it('does not dial without an auth token', () => {
    const connection = createRealtimeOpsConnection({
      getToken: () => null,
      createWebSocket: (url) => new FakeWebSocket(url) as unknown as WebSocket,
    })
    const unsubscribe = connection.subscribe(NOOP_LISTENER)

    expect(FakeWebSocket.instances).toHaveLength(0)
    expect(connection.getSnapshot().sample.connected).toBe(false)

    unsubscribe()
  })
})
