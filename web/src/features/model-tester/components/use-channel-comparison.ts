import { useCallback, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toast } from '@/lib/toast'

import {
  buildChatPayload,
  runBatchComparison,
  runChatProbe,
  sortBatchResults,
} from '../api'
import type { TesterFormValues } from '../lib/tester-schema'
import type { BatchProbeResult, ChatMessage } from '../types'
import { summarizeBatch } from './batch-result-status'

function channelProbe(
  values: TesterFormValues,
  history: ChatMessage[],
  channelId: number
) {
  return {
    channelId,
    run: (signal?: AbortSignal) =>
      runChatProbe(buildChatPayload({ ...values, channelId }, history), signal),
  }
}

export function useChannelComparison() {
  const { t } = useTranslation()
  const [comparison, setComparison] = useState<BatchProbeResult[] | null>(null)
  const [comparisonTotal, setComparisonTotal] = useState(0)
  const [isComparing, setIsComparing] = useState(false)
  // Channels whose single-row re-run probe is in flight (per-row spinner in
  // the comparison table; a Set so independent rows never cross-talk).
  const [rerunningChannelIds, setRerunningChannelIds] = useState<
    ReadonlySet<number>
  >(new Set())

  // One AbortController per in-flight row re-run so the Stop button stops
  // them too, plus the last comparison's form values + history so a row can
  // be re-probed with the exact same payload without a new full batch.
  const rerunControllersRef = useRef(new Map<number, AbortController>())
  const lastComparisonRef = useRef<{
    values: TesterFormValues
    history: ChatMessage[]
  } | null>(null)

  const stopReruns = useCallback(() => {
    rerunControllersRef.current.forEach((controller) => controller.abort())
  }, [])

  async function compare(
    values: TesterFormValues,
    history: ChatMessage[],
    comparisonChannelIds: number[],
    signal: AbortSignal
  ) {
    lastComparisonRef.current = { values, history }
    setComparison([])
    setComparisonTotal(comparisonChannelIds.length)
    setIsComparing(true)
    try {
      const probes = comparisonChannelIds.map((channelId) =>
        channelProbe(values, history, channelId)
      )
      const results = await runBatchComparison(probes, {
        signal,
        onResult: (result) => {
          setComparison((prev) =>
            sortBatchResults([
              ...(prev ?? []).filter(
                (row) => row.channelId !== result.channelId
              ),
              result,
            ])
          )
        },
      })
      setComparison(sortBatchResults(results))
      const { summaryKey, counts } = summarizeBatch(results)
      toast.success(t(summaryKey, counts))
    } catch {
      toast.error(t('modelTester.toast.failed'))
    } finally {
      setIsComparing(false)
    }
  }

  // Re-run ONE comparison row with the exact payload the batch used. Reuses
  // the existing batch machinery — runBatchComparison settles the probe like
  // the original run (success / failure / abort), so no new API path — then
  // merges the fresh result over that channel's row and re-sorts. Failed and
  // stopped rows are re-runnable too; the updated row is the feedback.
  const handleRerunRow = useCallback(
    async (channelId: number) => {
      const context = lastComparisonRef.current
      if (!context || rerunningChannelIds.has(channelId)) return
      const controller = new AbortController()
      rerunControllersRef.current.set(channelId, controller)
      setRerunningChannelIds((prev) => new Set(prev).add(channelId))
      try {
        const [result] = await runBatchComparison(
          [channelProbe(context.values, context.history, channelId)],
          { signal: controller.signal }
        )
        setComparison((prev) =>
          prev
            ? sortBatchResults(
                prev.map((row) => (row.channelId === channelId ? result : row))
              )
            : prev
        )
      } finally {
        rerunControllersRef.current.delete(channelId)
        setRerunningChannelIds((prev) => {
          const next = new Set(prev)
          next.delete(channelId)
          return next
        })
      }
    },
    [rerunningChannelIds]
  )

  const handleCompareChange = useCallback((enabled: boolean) => {
    if (!enabled) {
      setComparison(null)
      setComparisonTotal(0)
    }
  }, [])

  function clear() {
    setComparison(null)
    lastComparisonRef.current = null
  }

  return {
    results: comparison,
    total: comparisonTotal,
    isComparing,
    requestedModel: lastComparisonRef.current?.values.model,
    rerunningChannelIds,
    hasReruns: () => rerunControllersRef.current.size > 0,
    compare,
    rerun: handleRerunRow,
    stopReruns,
    clear,
    hide: () => setComparison(null),
    onCompareChange: handleCompareChange,
  }
}
