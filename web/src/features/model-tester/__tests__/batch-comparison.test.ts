// metapi-go/features/model-tester — batch comparison orchestration tests.
//
// `runBatchComparison` settles every probe independently with bounded
// concurrency and one shared abort; `sortBatchResults` orders successes by
// ascending latency ahead of failures. Both are pure helpers (the `run`
// boundary is mocked per-probe, matching the "only mock the network boundary"
// rule).

import { describe, expect, it, vi } from 'vitest'

import { runBatchComparison, sortBatchResults } from '../api'
import type { TestResponse } from '../types'

function response(overrides: Partial<TestResponse> = {}): TestResponse {
  return {
    content: 'ok',
    reasoningContent: '',
    doneReceived: true,
    statusCode: 200,
    latencyMs: 100,
    chunks: 1,
    rawEvents: [],
    empty: false,
    ...overrides,
  }
}

function deferred(): {
  promise: Promise<void>
  resolve: () => void
} {
  let resolvePromise: (() => void) | undefined
  const promise = new Promise<void>((resolve) => {
    resolvePromise = resolve
  })
  return {
    promise,
    resolve: () => resolvePromise?.(),
  }
}

describe('runBatchComparison', () => {
  it('settles each probe independently (mixed success/failure)', async () => {
    const results = await runBatchComparison([
      {
        channelId: 1,
        run: () => Promise.resolve(response({ latencyMs: 300 })),
      },
      { channelId: 2, run: () => Promise.reject(new Error('boom')) },
      {
        channelId: 3,
        run: () =>
          Promise.resolve(response({ latencyMs: 100, error: 'upstream' })),
      },
    ])
    expect(results).toEqual([
      { channelId: 1, status: 'success', statusCode: 200, latencyMs: 300 },
      { channelId: 2, status: 'failure', error: 'boom' },
      {
        channelId: 3,
        status: 'failure',
        statusCode: 200,
        latencyMs: 100,
        error: 'upstream',
      },
    ])
  })

  it('preserves input order regardless of settle timing', async () => {
    const results = await runBatchComparison([
      { channelId: 1, run: () => Promise.resolve(response({ latencyMs: 10 })) },
      { channelId: 2, run: () => Promise.resolve(response({ latencyMs: 1 })) },
    ])
    expect(results.map((result) => result.channelId)).toEqual([1, 2])
  })

  it('marks aborted probes with the aborted status', async () => {
    const abortError = new Error('This operation was aborted')
    abortError.name = 'AbortError'
    const results = await runBatchComparison([
      { channelId: 1, run: () => Promise.resolve(response({})) },
      { channelId: 2, run: () => Promise.reject(abortError) },
    ])
    expect(results[1]).toEqual({ channelId: 2, status: 'aborted' })
  })

  it('returns an empty array for an empty probe set', async () => {
    expect(await runBatchComparison([])).toEqual([])
  })

  it('never exceeds the configured concurrency', async () => {
    const gates = Array.from({ length: 4 }, () => deferred())
    let activeProbeCount = 0
    let maximumActiveProbeCount = 0
    let startedProbeCount = 0

    const comparison = runBatchComparison(
      gates.map((gate, index) => ({
        channelId: index + 1,
        run: async () => {
          activeProbeCount += 1
          startedProbeCount += 1
          maximumActiveProbeCount = Math.max(
            maximumActiveProbeCount,
            activeProbeCount
          )
          await gate.promise
          activeProbeCount -= 1
          return response()
        },
      })),
      { concurrency: 2 }
    )

    await vi.waitFor(() => expect(startedProbeCount).toBe(2))
    gates[0].resolve()
    await vi.waitFor(() => expect(startedProbeCount).toBe(3))
    gates[1].resolve()
    await vi.waitFor(() => expect(startedProbeCount).toBe(4))
    gates[2].resolve()
    gates[3].resolve()

    await comparison
    expect(maximumActiveProbeCount).toBe(2)
  })

  it('shares one abort signal across active and queued probes', async () => {
    const controller = new AbortController()
    const observedSignals: AbortSignal[] = []
    const abortError = new Error('This operation was aborted')
    abortError.name = 'AbortError'

    const resultsPromise = runBatchComparison(
      [1, 2, 3, 4].map((channelId) => ({
        channelId,
        run: (signal?: AbortSignal) => {
          if (!signal) throw new Error('missing abort signal')
          observedSignals.push(signal)
          if (signal.aborted) return Promise.reject(abortError)
          return new Promise<TestResponse>((_resolve, reject) => {
            signal.addEventListener('abort', () => reject(abortError), {
              once: true,
            })
          })
        },
      })),
      { concurrency: 2, signal: controller.signal }
    )

    await vi.waitFor(() => expect(observedSignals).toHaveLength(2))
    controller.abort()
    const results = await resultsPromise

    expect(observedSignals).toHaveLength(4)
    expect(
      observedSignals.every((signal) => signal === controller.signal)
    ).toBe(true)
    expect(results.every((result) => result.status === 'aborted')).toBe(true)
  })
})

describe('sortBatchResults', () => {
  it('sorts successes by ascending latency, then failures in input order', () => {
    const sorted = sortBatchResults([
      { channelId: 1, status: 'failure', error: 'x' },
      { channelId: 2, status: 'success', latencyMs: 300 },
      { channelId: 3, status: 'success', latencyMs: 100 },
      { channelId: 4, status: 'failure', error: 'y' },
    ])
    expect(sorted.map((result) => result.channelId)).toEqual([3, 2, 1, 4])
  })

  it('keeps input order for equal-latency successes (stable)', () => {
    const sorted = sortBatchResults([
      { channelId: 1, status: 'success', latencyMs: 100 },
      { channelId: 2, status: 'success', latencyMs: 100 },
    ])
    expect(sorted.map((result) => result.channelId)).toEqual([1, 2])
  })

  it('returns an empty array for empty input', () => {
    expect(sortBatchResults([])).toEqual([])
  })
})
