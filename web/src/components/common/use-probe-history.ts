// metapi-go/components/common — TanStack Query hook for the batch
// probe-history endpoints (handler/admin/probe_history.go). ONE bounded
// request per page render (channels or accounts dimension); the response is
// shaped into a map keyed by row id so table cells look up their history in
// O(1) and no per-row request ever happens.

import { useQuery } from '@tanstack/react-query'

import { api } from '@/lib/api'

import type { ProbeHistoryMap, ProbeResult } from './probe-health-bar'

/** Bars per row; matches the backend default and the documented 1–50 clamp. */
const PROBE_HISTORY_LIMIT = 20

export type ProbeHistoryDimension = 'channels' | 'accounts'

/**
 * Parse the `{limit, items}` envelope into a row-id map. A malformed body
 * throws so a broken response fails the query explicitly instead of
 * masquerading as an empty history (same contract as parseChannelsEnvelope).
 */
export function parseProbeHistoryEnvelope(
  result: unknown,
  idKey: 'channelId' | 'accountId'
): ProbeHistoryMap {
  if (!result || typeof result !== 'object') {
    throw new Error('Invalid probe history response')
  }
  const items = (result as { items?: unknown }).items
  if (!Array.isArray(items)) {
    throw new Error('Invalid probe history response')
  }
  const map: ProbeHistoryMap = {}
  for (const item of items) {
    if (!item || typeof item !== 'object') continue
    const record = item as Record<string, unknown>
    const id = record[idKey]
    const results = record.results
    if (typeof id !== 'number' || !Array.isArray(results)) continue
    map[id] = results as ProbeResult[]
  }
  return map
}

/**
 * Batch probe history for a table dimension. `data` is undefined while the
 * query is pending or failed — cells render their pending placeholder and
 * never block the table (the history is secondary decoration).
 */
export function useProbeHistory(dimension: ProbeHistoryDimension) {
  const idKey = dimension === 'channels' ? 'channelId' : 'accountId'
  return useQuery<ProbeHistoryMap>({
    queryKey: ['probe-history', dimension, PROBE_HISTORY_LIMIT],
    queryFn: async () => {
      const result =
        dimension === 'channels'
          ? await api.getChannelProbeHistory(PROBE_HISTORY_LIMIT)
          : await api.getAccountProbeHistory(PROBE_HISTORY_LIMIT)
      return parseProbeHistoryEnvelope(result, idKey)
    },
    staleTime: 30 * 1000,
  })
}
