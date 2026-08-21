// metapi-go/features/dashboard/hooks — live traffic WebSocket hook.
//
// Thin subscription over the module-level shared realtime ops connection
// (see ./realtime-ops-connection): every consumer — today snapshot strip +
// availability panel — subscribes to ONE WebSocket instead of dialing its
// own per component instance, so switching dashboard tabs no longer tears
// down / re-dials the stream. The ops stream pushes one frame per second:
// `{ lifetime, points: [{ ts, total, success }] }`. Browser WebSocket cannot
// send the Authorization header, so the access token travels as a query
// param (same contract as the legacy panel).

import { useSyncExternalStore } from 'react'

import type { RealtimeOpsSample } from '../types'
import {
  realtimeOpsConnection,
  type RealtimeOpsState,
} from './realtime-ops-connection'

/** Return shape of {@link useRealtimeOps}. */
export type UseRealtimeOpsReturn = {
  sample: RealtimeOpsSample
  /**
   * Wall-clock ms of the last received frame (null before the first frame).
   * During reconnect / disconnect gaps panels render a "data as of" marker
   * from this instead of silently showing stale numbers.
   */
  lastFrameAt: number | null
  reconnect: () => void
}

/**
 * Subscribe to the shared realtime ops WebSocket. Returns a rolling
 * 60-sample sparkline + derived qps / successRate / lifetime, plus the
 * freshness of the last frame. Renders nothing usable when there is no
 * token (the panel hides itself in that case).
 *
 * The connection self-heals: exponential backoff escalates into an
 * unbounded slow retry after repeated failures, and tab-visible / online
 * events re-dial immediately. `sample.gaveUp` only marks the slow-retry
 * phase; `reconnect` forces an immediate attempt from any state.
 */
export function useRealtimeOps(): UseRealtimeOpsReturn {
  const state: RealtimeOpsState = useSyncExternalStore(
    realtimeOpsConnection.subscribe,
    realtimeOpsConnection.getSnapshot
  )
  return {
    sample: state.sample,
    lastFrameAt: state.lastFrameAt,
    reconnect: realtimeOpsConnection.reconnect,
  }
}
