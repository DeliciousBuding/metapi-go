// The page owns the in-memory conversation and request cancellation.
// Each synchronous response keeps its own diagnostics; comparisons keep
// their original request context without adding a conversation turn.

import { useQueryClient } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { SquarePen } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { channelsKeys, useChannels } from '@/features/channels'
import { tokenRoutesApi } from '@/lib/api/token-routes'
import { asStringParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import {
  buildChatPayload,
  isAbortError,
  runBatchComparison,
  runChatProbe,
  sortBatchResults,
  useTestModel,
} from '../api'
import { retainEnabledComparisonChannelIds } from '../lib/comparison-channels'
import type { TesterFormValues } from '../lib/tester-schema'
import type { BatchProbeResult, ChatMessage } from '../types'
import { BatchResults } from './batch-results'
import { TestForm } from './test-form'
import { TestResponseViewer } from './test-response-viewer'

let messageIdCounter = 0

function nextMessageId(): string {
  messageIdCounter += 1
  return `message-${messageIdCounter}`
}

export function ModelTesterPage() {
  const { t } = useTranslation()
  // The `/model-tester` route validates `?model=` via validateSearch, so the
  // deep-link param comes from `useSearch()` (typed) rather than a raw
  // `window.location.search` read.
  const { model } = useSearch({ from: '/_authenticated/model-tester' })
  const modelParam = asStringParam(model)
  const defaultModel = modelParam?.trim() ? modelParam : undefined

  const testModel = useTestModel()
  const channelsQuery = useChannels()

  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [clearDialogOpen, setClearDialogOpen] = useState(false)
  const [disableFailedDialogOpen, setDisableFailedDialogOpen] = useState(false)
  const [isDisablingFailed, setIsDisablingFailed] = useState(false)
  const queryClient = useQueryClient()
  const [comparison, setComparison] = useState<BatchProbeResult[] | null>(null)
  const [isComparing, setIsComparing] = useState(false)
  // Channels whose single-row re-run probe is in flight (per-row spinner in
  // the comparison table; a Set so independent rows never cross-talk).
  const [rerunningChannelIds, setRerunningChannelIds] = useState<
    ReadonlySet<number>
  >(new Set())

  const abortControllerRef = useRef<AbortController | null>(null)
  // One AbortController per in-flight row re-run so the Stop button stops
  // them too, plus the last comparison's form values + history so a row can
  // be re-probed with the exact same payload without a new full batch.
  const rerunControllersRef = useRef(new Map<number, AbortController>())
  const lastComparisonRef = useRef<{
    values: TesterFormValues
    history: ChatMessage[]
  } | null>(null)

  useEffect(
    () => () => {
      abortControllerRef.current?.abort()
      rerunControllersRef.current.forEach((controller) => controller.abort())
    },
    []
  )

  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort()
    rerunControllersRef.current.forEach((controller) => controller.abort())
  }, [])

  const handleSubmit = useCallback(
    async (values: TesterFormValues) => {
      if (abortControllerRef.current || rerunControllersRef.current.size > 0)
        return false
      const comparisonChannelIds = retainEnabledComparisonChannelIds(
        values.channelIds ?? [],
        channelsQuery.data ?? []
      )
      if (values.compareChannels && comparisonChannelIds.length < 2) {
        toast.error(t('modelTester.form.errors.compareMinChannels'))
        return
      }

      const history = messages
      const controller = new AbortController()
      abortControllerRef.current = controller

      if (values.compareChannels) {
        lastComparisonRef.current = { values, history }
        setComparison(null)
        setIsComparing(true)
        try {
          const probes = comparisonChannelIds.map((channelId) => ({
            channelId,
            run: (signal?: AbortSignal) =>
              runChatProbe(
                buildChatPayload({ ...values, channelId }, history),
                signal
              ),
          }))
          const results = await runBatchComparison(probes, {
            signal: controller.signal,
          })
          setComparison(sortBatchResults(results))
          const succeeded = results.filter(
            (result) => result.status === 'success'
          ).length
          const failed = results.filter(
            (result) => result.status === 'failure'
          ).length
          const aborted = results.filter(
            (result) => result.status === 'aborted'
          ).length
          const summaryKey =
            aborted > 0
              ? 'modelTester.compare.summaryWithAborted'
              : 'modelTester.compare.summary'
          toast.success(
            t(summaryKey, {
              succeeded,
              failed,
              aborted,
            })
          )
        } catch {
          toast.error(t('modelTester.toast.failed'))
        } finally {
          setIsComparing(false)
          abortControllerRef.current = null
        }
        return
      }

      // Single runs join the conversation immediately; comparisons are
      // measurements and do not create an unmatched user-only turn.
      setComparison(null)
      setMessages((prev) => [
        ...prev,
        { id: nextMessageId(), role: 'user', content: values.prompt },
      ])

      try {
        const result = await testModel.mutateAsync({
          payload: values,
          history,
          signal: controller.signal,
        })
        if (controller.signal.aborted)
          throw new DOMException('This operation was aborted', 'AbortError')
        setMessages((prev) => [
          ...prev,
          {
            id: nextMessageId(),
            role: 'assistant',
            content: result.content,
            response: result,
            error: result.error,
            model: values.model,
          },
        ])
        if (!result.error && result.empty) {
          toast.warning(t('modelTester.toast.empty'))
        }
        return !result.error && !result.empty
      } catch (err) {
        if (controller.signal.aborted || isAbortError(err)) {
          setMessages((prev) => [
            ...prev,
            {
              id: nextMessageId(),
              role: 'assistant',
              content: '',
              error: t('modelTester.error.stopped'),
            },
          ])
          toast.info(t('modelTester.toast.stopped'))
        } else {
          const rawMessage =
            err instanceof Error ? err.message : 'modelTester.error.unknown'
          // Some thrown errors carry an i18n key (e.g.
          // `modelTester.error.notAvailable` / `sessionExpired`); translate
          // key-shaped messages so the viewer shows user-facing text.
          const message = rawMessage.startsWith('modelTester.')
            ? t(rawMessage)
            : rawMessage
          setMessages((prev) => [
            ...prev,
            {
              id: nextMessageId(),
              role: 'assistant',
              content: '',
              error: message,
            },
          ])
          toast.error(t('modelTester.toast.failed'))
        }
        return false
      } finally {
        abortControllerRef.current = null
      }
    },
    [channelsQuery.data, testModel, t, messages]
  )

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
          [
            {
              channelId,
              run: (signal?: AbortSignal) =>
                runChatProbe(
                  buildChatPayload(
                    { ...context.values, channelId },
                    context.history
                  ),
                  signal
                ),
            },
          ],
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

  const handleClearSession = useCallback(() => {
    setMessages([])
    setComparison(null)
    lastComparisonRef.current = null
    setClearDialogOpen(false)
    toast.success(t('modelTester.toast.cleared'))
  }, [t])

  // Channels whose comparison probe failed — the bulk disable action
  // targets exactly these rows.
  const failedChannelIds = useMemo(
    () =>
      (comparison ?? [])
        .filter((result) => result.status === 'failure')
        .map((result) => result.channelId),
    [comparison]
  )

  // One-click closure for failed comparison rows: after the operator
  // confirms, PUT /api/channels/batch disables exactly the failed channels
  // (marked manual_override server-side so rebuilds keep it). The envelope
  // reports per-item truth; a partial failure surfaces the precise
  // breakdown instead of a generic toast. AuditMiddleware records the call.
  const handleDisableFailedConfirmed = useCallback(async () => {
    setDisableFailedDialogOpen(false)
    if (failedChannelIds.length === 0 || isDisablingFailed) return
    setIsDisablingFailed(true)
    try {
      const result = await tokenRoutesApi.batchUpdateChannels(
        failedChannelIds.map((id) => ({ id, enabled: false }))
      )
      const succeededCount = result.successIds?.length ?? 0
      const failedItems = result.failedItems ?? []
      if (failedItems.length === 0) {
        toast.success(
          t('modelTester.disableFailed.success', { count: succeededCount })
        )
      } else {
        const detail = failedItems
          .map((item) => `#${item.id}: ${item.message}`)
          .join('; ')
        toast.warning(
          t('modelTester.disableFailed.partial', {
            succeeded: succeededCount,
            failed: failedItems.length,
            detail,
          })
        )
      }
      if (succeededCount > 0) {
        queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      }
    } catch {
      toast.error(t('modelTester.disableFailed.error'))
    } finally {
      setIsDisablingFailed(false)
    }
  }, [failedChannelIds, isDisablingFailed, queryClient, t])

  const isRunning =
    testModel.isPending || isComparing || rerunningChannelIds.size > 0

  return (
    <div className='flex h-full min-h-0 flex-col overflow-hidden'>
      <TestForm
        hasConversation={
          messages.length > 0 || Boolean(comparison) || isRunning
        }
        actions={
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => setClearDialogOpen(true)}
            disabled={isRunning || (messages.length === 0 && !comparison)}
          >
            <SquarePen className='size-4' />
            {t('modelTester.clear.button')}
          </Button>
        }
        isRunning={isRunning}
        defaultModel={defaultModel}
        onSubmit={handleSubmit}
        onStop={handleStop}
        onCompareChange={(enabled) => {
          if (!enabled) setComparison(null)
        }}
      >
        {comparison || isComparing ? (
          <BatchResults
            results={comparison ?? []}
            channels={channelsQuery.data ?? []}
            isRunning={isComparing}
            rerunningChannelIds={rerunningChannelIds}
            onRerunRow={handleRerunRow}
            onDisableFailed={() => setDisableFailedDialogOpen(true)}
            isDisablingFailed={isDisablingFailed}
          />
        ) : (
          <TestResponseViewer messages={messages} isRunning={isRunning} />
        )}
      </TestForm>
      <ConfirmDialog
        open={clearDialogOpen}
        title={t('modelTester.clear.title')}
        description={t('modelTester.clear.description')}
        confirmLabel={t('modelTester.clear.confirm')}
        cancelLabel={t('modelTester.clear.cancel')}
        onConfirm={handleClearSession}
        onCancel={() => setClearDialogOpen(false)}
      />

      <ConfirmDialog
        open={disableFailedDialogOpen}
        title={t('modelTester.disableFailed.title')}
        description={t('modelTester.disableFailed.description', {
          count: failedChannelIds.length,
        })}
        confirmLabel={t('modelTester.disableFailed.confirm')}
        cancelLabel={t('modelTester.disableFailed.cancel')}
        destructive
        onConfirm={() => {
          void handleDisableFailedConfirmed()
        }}
        onCancel={() => setDisableFailedDialogOpen(false)}
      />
    </div>
  )
}
