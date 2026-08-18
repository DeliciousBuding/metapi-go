// metapi-go/features/dashboard/hooks — live traffic WebSocket.
//
// Ported from legacy web/components/RealtimeOpsPanel.tsx. The ops stream
// pushes one frame per second: `{ lifetime, points: [{ ts, total, success }] }`.
// Browser WebSocket cannot send the Authorization header, so the access token
// travels as a query param (same contract as the legacy panel).
//
// Auto-reconnect uses exponential backoff (capped at 15s); after MAX_FAILS
// consecutive failures the hook gives up permanently (the panel then renders
// the disconnected state rather than retrying forever).

import { useEffect, useRef, useState } from 'react'

import { getAuthToken } from '@/lib/auth-session'

import type {
  RealtimeOpsFrame,
  RealtimeOpsSample,
  RealtimeOpsSamplePoint,
} from '../types'

const MAX_FAILS = 5
const MAX_BACKOFF_MS = 15_000
const SPARK_WINDOW = 60

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

const DISCONNECTED: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  spark: [],
  connected: false,
  gaveUp: true,
}

const IDLE: RealtimeOpsSample = {
  qps: 0,
  successRate: 0,
  lifetime: 0,
  spark: [],
  connected: false,
  gaveUp: false,
}

/**
 * Subscribe to the realtime ops WebSocket. Returns a rolling 60-sample
 * sparkline + derived qps / successRate / lifetime. Renders nothing usable
 * when there is no token (the panel hides itself in that case).
 */
export function useRealtimeOps(): RealtimeOpsSample {
  const [sample, setSample] = useState<RealtimeOpsSample>(IDLE)
  const socketRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const failsRef = useRef(0)
  const previousTotalRef = useRef<number | null>(null)
  const sparkRef = useRef<RealtimeOpsSamplePoint[]>([])

  useEffect(() => {
    const token = getAuthToken()
    if (!token) return undefined

    let disposed = false
    let backoff = 1000

    const connect = () => {
      if (disposed) return
      const url = buildWebSocketUrl(token)
      if (!url) return

      let socket: WebSocket
      try {
        socket = new WebSocket(url)
      } catch {
        return
      }
      socketRef.current = socket

      socket.onopen = () => {
        failsRef.current = 0
        backoff = 1000
        setSample((prev) => ({ ...prev, connected: true, gaveUp: false }))
      }

      socket.onmessage = (event) => {
        if (disposed) return
        let frame: RealtimeOpsFrame | null = null
        try {
          frame = JSON.parse(event.data as string) as RealtimeOpsFrame
        } catch {
          return
        }
        if (!frame || !Array.isArray(frame.points)) return

        const latest = frame.points.at(-1)
        if (!latest) return

        const qps = computeQps(latest.total, previousTotalRef.current)
        previousTotalRef.current = latest.total
        const successRate = computeSuccessRate(latest.success, latest.total)

        const nextSpark: RealtimeOpsSamplePoint[] = [
          ...sparkRef.current,
          { qps, successRate },
        ]
        if (nextSpark.length > SPARK_WINDOW) nextSpark.shift()
        sparkRef.current = nextSpark

        setSample({
          qps,
          successRate,
          lifetime: frame.lifetime,
          spark: nextSpark,
          connected: true,
          gaveUp: false,
        })
      }

      socket.onerror = () => {
        // Hand-off to onclose for reconnect accounting.
      }

      socket.onclose = () => {
        if (disposed) return
        socketRef.current = null
        setSample((prev) => ({ ...prev, connected: false }))
        failsRef.current += 1
        if (failsRef.current >= MAX_FAILS) {
          setSample(DISCONNECTED)
          return
        }
        backoff = Math.min(backoff * 2, MAX_BACKOFF_MS)
        reconnectTimer.current = setTimeout(connect, backoff)
      }
    }

    connect()

    return () => {
      disposed = true
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      const socket = socketRef.current
      if (socket) {
        socket.onopen = null
        socket.onmessage = null
        socket.onerror = null
        socket.onclose = null
        if (socket.readyState === WebSocket.OPEN) socket.close()
      }
      socketRef.current = null
    }
  }, [])

  return sample
}
