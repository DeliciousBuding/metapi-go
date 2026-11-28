import { useMutation } from '@tanstack/react-query'

import { buildChatPayload, runChatProbe } from './chat-probe'
import type { TestModelVariables, TestResponse } from './types'

export { runBatchComparison, sortBatchResults } from './batch-comparison'
export {
  buildChatPayload,
  runChatProbe,
  isAbortError,
  resolveTestResponseError,
} from './chat-probe'

/**
 * Run a single sync chat test for the viewer. Builds the payload from form
 * values + history, delegates to `runChatProbe`, then forwards the single
 * delta to `onDelta` and the summary to `onDone`.
 */
async function runChatTest(
  variables: TestModelVariables
): Promise<TestResponse> {
  const { payload, history, onDelta, onDone, signal } = variables
  const summary = await runChatProbe(buildChatPayload(payload, history), signal)
  onDelta?.({
    contentDelta: summary.content,
    reasoningDelta: summary.reasoningContent,
    done: summary.doneReceived,
  })
  onDone?.(summary)
  return summary
}

export function useTestModel() {
  return useMutation<TestResponse, Error, TestModelVariables>({
    mutationFn: runChatTest,
  })
}
