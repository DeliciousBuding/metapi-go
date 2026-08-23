// metapi-go/features/dashboard/hooks — shared realtime ops WebSocket.
//
// One module-level connection serves every realtime consumer (today snapshot
// strip + availability panel). Reference counting opens the socket on the
// first subscription and closes it when the last subscriber leaves, so
// switching dashboard tabs no longer tears down and re-dials the stream.
//
// Self-healing: the connection never gives up permanently. Consecutive
// failures escalate from exponential backoff (1s → 15s cap) into a slow
// 30s retry cadence after MAX_FAILS, and `visibilitychange` (tab visible) /
// `online` events reset the failure accounting and retry immediately. The
// `gaveUp` flag only marks the slow-retry phase for the UI — it always
// clears once the user (or the network) comes back.
//
// Power saving: while the tab is hidden the socket is closed entirely and
// re-dialed the moment the tab becomes visible again. During any gap the
// snapshot keeps the last sample + `lastFrameAt` so panels can render a
// "data as of HH:mm:ss" freshness marker instead of silently stale numbers.

import { getAuthToken } from '@/lib/auth-session'

import type {
  RealtimeOpsFrame,
  RealtimeOpsSample,
  RealtimeOpsSamplePoint,
} from '../types'

const MAX_FAILS = 5
const INITIAL_BACKOFF_MS = 1_000
const MAX_BACKOFF_MS = 15_000
// Cadence once MAX_FAILS consecutive failures are reached. Never stops.
const SLOW_RETRY_MS = 30_000
const SPARK_WINDOW = 60

/** Snapshot exposed to subscribers (immutable between state changes). */
export type RealtimeOpsState = {
  sample: RealtimeOpsSample
  /** Wall-clock ms of the last received frame; null before the first frame. */
  lastFrameAt: number | null
}

/** Public surface consumed by {@link useRealtimeOps}. */
export type RealtimeOpsConnection = {
  /** Subscribe to state changes; the returned fn unsubscribes. */
  subscribe: (listener: () => void) => () => void
  /** Current immutable snapshot (useSyncExternalStore contract). */
  getSnapshot: () => RealtimeOpsState
  /** Manual reconnect: resets failure accounting and re-dials immediately. */
  reconnect: () => void
}

export type RealtimeOpsConnectionOptions = {
  /** Token source; no token means no connection (panel hides itself). */
  getToken?: () => string | null
  /** WebSocket factory (tests inject fakes). */
  createWebSocket?: (url: string) => WebSocket
}

const IDLE_SAMPLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  uptimeSeconds: 0,
  spark: [],
  connected: false,
  gaveUp: false,
}

function buildWebSocketUrl(token: string): string | null {
  if (typeof window === 'undefined' || typeof window.location === 'undefined') {
    return null
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const host = window.location.host
  const encoded = encodeURIComponent(token)
  return `${protocol}//${host}/api/admin/ops/ws?token=${encoded}`
}

function computeQps(total: number, previousTotal: number | null): number {
  if (previousTotal === null || total < previousTotal) return 0
  return total - previousTotal
}

function computeSuccessRate(success: number, total: number): number {
  if (total <= 0) return 0
  return Math.min(1, success / total)
}

/**
 * Create a realtime ops connection. The module exports a single shared
 * instance ({@link realtimeOpsConnection}); the factory exists so tests can
 * run isolated instances with injected fakes.
 */
export function createRealtimeOpsConnection(
  options: RealtimeOpsConnectionOptions = {}
): RealtimeOpsConnection {
  const getToken = options.getToken ?? getAuthToken
  const createWebSocket =
    options.createWebSocket ?? ((url: string) => new WebSocket(url))

  const listeners = new Set<() => void>()
  let state: RealtimeOpsState = { sample: IDLE_SAMPLE, lastFrameAt: null }
  let socket: WebSocket | null = null
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null
  let failureCount = 0
  let backoffMs = INITIAL_BACKOFF_MS
  let previousTotal: number | null = null
  let spark: RealtimeOpsSamplePoint[] = []
  // True while at least one subscriber keeps the session alive.
  let sessionActive = false

  function emit() {
    for (const listener of listeners) listener()
  }

  function setState(next: RealtimeOpsState) {
    state = next
    emit()
  }

  function patchSample(patch: Partial<RealtimeOpsSample>) {
    setState({ ...state, sample: { ...state.sample, ...patch } })
  }

  function clearReconnectTimer() {
    if (reconnectTimer !== null) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
  }

  /** Detach handlers (so close() cannot feed failure accounting) and close. */
  function teardownSocket() {
    if (!socket) return
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
    socket = null
    // A qps delta across a teardown gap would read as one giant second; the
    // next frame after reconnect starts from a clean baseline instead.
    previousTotal = null
  }

  function connect() {
    if (!sessionActive) return
    // Paused-while-hidden: the visibilitychange handler re-dials on show.
    if (
      typeof document !== 'undefined' &&
      document.visibilityState === 'hidden'
    ) {
      return
    }
    const token = getToken()
    if (!token) return
    const url = buildWebSocketUrl(token)
    if (!url) return

    let nextSocket: WebSocket
    try {
      nextSocket = createWebSocket(url)
    } catch {
      return
    }
    socket = nextSocket

    nextSocket.onopen = () => {
      if (socket !== nextSocket) return
      failureCount = 0
      backoffMs = INITIAL_BACKOFF_MS
      patchSample({ connected: true, gaveUp: false })
    }

    nextSocket.onmessage = (event) => {
      if (socket !== nextSocket) return
      let frame: RealtimeOpsFrame | null = null
      try {
        frame = JSON.parse(event.data as string) as RealtimeOpsFrame
      } catch {
        return
      }
      if (!frame || !Array.isArray(frame.points)) return

      const latest = frame.points.at(-1)
      if (!latest) return

      const qps = computeQps(latest.total, previousTotal)
      previousTotal = latest.total
      const successRate = computeSuccessRate(latest.success, latest.total)

      spark = [...spark, { qps, successRate }]
      if (spark.length > SPARK_WINDOW) spark = spark.slice(-SPARK_WINDOW)

      setState({
        sample: {
          qps,
          successRate,
          lifetime: frame.lifetime,
          uptimeSeconds: frame.uptimeSeconds ?? 0,
          spark,
          connected: true,
          gaveUp: false,
        },
        lastFrameAt: Date.now(),
      })
    }

    nextSocket.onerror = () => {
      // Hand-off to onclose for reconnect accounting.
    }

    nextSocket.onclose = () => {
      if (socket !== nextSocket) return
      socket = null
      if (!sessionActive) return
      failureCount += 1
      const gaveUp = failureCount >= MAX_FAILS
      patchSample({ connected: false, gaveUp })
      // Escalate: exponential backoff up to the cap, then a slow but
      // unbounded retry cadence. visibility/online events shortcut both.
      const delayMs = gaveUp
        ? SLOW_RETRY_MS
        : Math.min(backoffMs * 2, MAX_BACKOFF_MS)
      if (!gaveUp) backoffMs = delayMs
      clearReconnectTimer()
      reconnectTimer = setTimeout(connect, delayMs)
    }
  }

  /** Close the socket without failure accounting (tab went hidden). */
  function pauseConnection() {
    clearReconnectTimer()
    teardownSocket()
    if (state.sample.connected || state.sample.gaveUp) {
      patchSample({ connected: false, gaveUp: false })
    }
  }

  /** Reset failure accounting and re-dial right away. */
  function reviveConnection() {
    if (!sessionActive) return
    failureCount = 0
    backoffMs = INITIAL_BACKOFF_MS
    clearReconnectTimer()
    teardownSocket()
    connect()
  }

  function handleVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      pauseConnection()
    } else {
      reviveConnection()
    }
  }

  function handleOnline() {
    reviveConnection()
  }

  function startSession() {
    sessionActive = true
    document.addEventListener('visibilitychange', handleVisibilityChange)
    window.addEventListener('online', handleOnline)
    connect()
  }

  function stopSession() {
    sessionActive = false
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    window.removeEventListener('online', handleOnline)
    clearReconnectTimer()
    teardownSocket()
    failureCount = 0
    backoffMs = INITIAL_BACKOFF_MS
    // Keep the last sample (incl. spark) + lastFrameAt so the next session
    // (e.g. switching dashboard tabs and back) shows the stale data with a
    // freshness marker instead of flashing empty; only the live flags reset.
    if (state.sample.connected || state.sample.gaveUp) {
      setState({
        ...state,
        sample: { ...state.sample, connected: false, gaveUp: false },
      })
    }
  }

  function subscribe(listener: () => void): () => void {
    listeners.add(listener)
    if (listeners.size === 1) startSession()
    return () => {
      listeners.delete(listener)
      if (listeners.size === 0) stopSession()
    }
  }

  function getSnapshot(): RealtimeOpsState {
    return state
  }

  function reconnect() {
    if (!sessionActive) return
    failureCount = 0
    backoffMs = INITIAL_BACKOFF_MS
    clearReconnectTimer()
    teardownSocket()
    spark = []
    setState({ sample: IDLE_SAMPLE, lastFrameAt: null })
    connect()
  }

  return { subscribe, getSnapshot, reconnect }
}

/**
 * The single shared realtime ops connection. Every `useRealtimeOps` call
 * subscribes to this instance; the socket opens on the first subscriber and
 * closes when the last one leaves.
 */
export const realtimeOpsConnection = createRealtimeOpsConnection()
