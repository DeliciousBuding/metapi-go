import { isAbortError } from './chat-probe'
import type { BatchProbeResult, TestResponse } from './types'

/**
 * Run a batch of channel probes with bounded concurrency and one shared abort
 * lifecycle, settling every request independently so one slow/failing channel
 * never erases the others. Returns one result per probe in input order.
 */
export async function runBatchComparison(
  probes: Array<{
    channelId: number
    run: (signal?: AbortSignal) => Promise<TestResponse>
  }>,
  options: {
    concurrency?: number
    signal?: AbortSignal
    onResult?: (result: BatchProbeResult, index: number) => void
  } = {}
): Promise<BatchProbeResult[]> {
  const concurrency = Math.max(1, options.concurrency ?? 3)
  const results: BatchProbeResult[] = Array.from({ length: probes.length })
  let nextIndex = 0

  async function worker(): Promise<void> {
    while (nextIndex < probes.length) {
      const index = nextIndex
      nextIndex += 1
      const probe = probes[index]
      const settled = await settleProbe(
        probe.channelId,
        probe.run,
        options.signal
      )
      results[index] = settled
      options.onResult?.(settled, index)
    }
  }

  const workerCount = Math.min(concurrency, probes.length)
  await Promise.all(Array.from({ length: workerCount }, () => worker()))
  return results
}

async function settleProbe(
  channelId: number,
  run: (signal?: AbortSignal) => Promise<TestResponse>,
  signal?: AbortSignal
): Promise<BatchProbeResult> {
  try {
    const summary = await run(signal)
    if (summary.error) {
      return {
        channelId,
        status: 'failure',
        statusCode: summary.statusCode,
        latencyMs: summary.latencyMs,
        ...(summary.firstByteLatencyMs !== undefined
          ? { firstByteLatencyMs: summary.firstByteLatencyMs }
          : {}),
        ...(summary.upstreamReportedModel !== undefined
          ? { upstreamReportedModel: summary.upstreamReportedModel }
          : {}),
        ...(summary.basis !== undefined ? { basis: summary.basis } : {}),
        error: summary.error,
        content: summary.content,
        reasoningContent: summary.reasoningContent,
        rawEvents: summary.rawEvents,
        promptTokens: summary.promptTokens,
        completionTokens: summary.completionTokens,
        totalTokens: summary.totalTokens,
        bodyTruncated: summary.bodyTruncated,
      }
    }
    return {
      channelId,
      status: 'success',
      statusCode: summary.statusCode,
      latencyMs: summary.latencyMs,
      ...(summary.firstByteLatencyMs !== undefined
        ? { firstByteLatencyMs: summary.firstByteLatencyMs }
        : {}),
      ...(summary.upstreamReportedModel !== undefined
        ? { upstreamReportedModel: summary.upstreamReportedModel }
        : {}),
      ...(summary.basis !== undefined ? { basis: summary.basis } : {}),
      content: summary.content,
      reasoningContent: summary.reasoningContent,
      rawEvents: summary.rawEvents,
      promptTokens: summary.promptTokens,
      completionTokens: summary.completionTokens,
      totalTokens: summary.totalTokens,
      bodyTruncated: summary.bodyTruncated,
    }
  } catch (error) {
    if (isAbortError(error)) return { channelId, status: 'aborted' }
    return {
      channelId,
      status: 'failure',
      error: error instanceof Error ? error.message : String(error),
    }
  }
}

/**
 * Order batch results for display: completed successes first, sorted by
 * ascending latency, then failures/aborted entries in their original input
 * order (a stable sort so reruns do not jumble otherwise-identical rows).
 */
export function sortBatchResults(
  results: BatchProbeResult[]
): BatchProbeResult[] {
  return results
    .map((result, index) => ({ result, index }))
    .sort((a, b) => {
      const aSuccess = a.result.status === 'success'
      const bSuccess = b.result.status === 'success'
      if (aSuccess && bSuccess) {
        const latencyDiff =
          (a.result.latencyMs ?? 0) - (b.result.latencyMs ?? 0)
        return latencyDiff !== 0 ? latencyDiff : a.index - b.index
      }
      if (aSuccess !== bSuccess) return aSuccess ? -1 : 1
      return a.index - b.index
    })
    .map((entry) => entry.result)
}
