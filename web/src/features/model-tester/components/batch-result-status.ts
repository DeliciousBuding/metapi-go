import type { BatchProbeResult } from '../types'

export function statusKeyFor(result: BatchProbeResult): string {
  if (result.status === 'success') return 'modelTester.compare.statusSuccess'
  if (result.status === 'failure') return 'modelTester.compare.statusFailure'
  return 'modelTester.compare.statusAborted'
}

export function summarizeBatch(results: BatchProbeResult[]) {
  const succeeded = results.filter(
    (result) => result.status === 'success'
  ).length
  const failed = results.filter((result) => result.status === 'failure').length
  const aborted = results.filter((result) => result.status === 'aborted').length
  const summaryKey =
    aborted > 0
      ? 'modelTester.compare.summaryWithAborted'
      : 'modelTester.compare.summary'
  return { summaryKey, counts: { succeeded, failed, aborted } }
}
