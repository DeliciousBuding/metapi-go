// metapi-go/features/model-tester — batch comparison orchestration tests.
//
// `runBatchComparison` settles every probe independently with bounded
// concurrency and one shared abort; `sortBatchResults` orders successes by
// ascending latency ahead of failures. Both are pure helpers (the `run`
// boundary is mocked per-probe, matching the "only mock the network boundary"
// rule).

import { describe, expect, it } from 'vitest'

import { runBatchComparison, sortBatchResults } from '../api'
import type { TestResponse } from '../types'

function response(overrides: Partial<TestResponse> = {}): TestResponse {
  return {
    content: 'ok',
    reasoningContent: '',
    doneReceived: true,
    latencyMs: 100,
    chunks: 1,
    rawEvents: [],
    empty: false,
    ...overrides,
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
      { channelId: 1, status: 'success', latencyMs: 300 },
      { channelId: 2, status: 'failure', error: 'boom' },
      { channelId: 3, status: 'failure', latencyMs: 100, error: 'upstream' },
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
