// The page owns the in-memory conversation and request cancellation.
// Each synchronous response keeps its own diagnostics; comparisons keep
// their original request context without adding a conversation turn.

import { useSearch } from '@tanstack/react-router'
import { SquarePen } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import { Button } from '@/components/ui/button'
import { useChannels } from '@/features/channels'
import { asStringParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import { retainEnabledComparisonChannelIds } from '../lib/comparison-channels'
import type { TesterFormValues } from '../lib/tester-schema'
import { BatchResults } from './batch-results'
import { TestForm } from './test-form'
import { TestResponseViewer } from './test-response-viewer'
import { useChannelComparison } from './use-channel-comparison'
import { useDisableFailedChannels } from './use-disable-failed-channels'
import { useTesterConversation } from './use-tester-conversation'

export function ModelTesterPage() {
  const { t } = useTranslation()
  // The `/model-tester` route validates `?model=` via validateSearch, so the
  // deep-link param comes from `useSearch()` (typed) rather than a raw
  // `window.location.search` read.
  const { model, targetFormat, channelId } = useSearch({
    from: '/_authenticated/model-tester',
  })
  const modelParam = asStringParam(model)
  const defaultModel = modelParam?.trim() ? modelParam : undefined
  const defaultTargetFormat = targetFormat
  const defaultChannelId =
    typeof channelId === 'number' &&
    Number.isInteger(channelId) &&
    channelId > 0
      ? channelId
      : undefined

  const channelsQuery = useChannels()
  const [clearDialogOpen, setClearDialogOpen] = useState(false)
  const abortControllerRef = useRef<AbortController | null>(null)
  const conversation = useTesterConversation()
  const batch = useChannelComparison()
  const disableFailed = useDisableFailedChannels(batch.results)
  const { stopReruns } = batch
  const handleStop = useCallback(() => {
    abortControllerRef.current?.abort()
    stopReruns()
  }, [stopReruns])
  useEffect(() => handleStop, [handleStop])

  async function handleSubmit(values: TesterFormValues) {
    if (abortControllerRef.current || batch.hasReruns()) return false
    const comparisonChannelIds = retainEnabledComparisonChannelIds(
      values.channelIds ?? [],
      channelsQuery.data ?? []
    )
    if (values.compareChannels && comparisonChannelIds.length < 2) {
      toast.error(t('modelTester.form.errors.compareMinChannels'))
      return
    }

    const controller = new AbortController()
    abortControllerRef.current = controller

    try {
      if (values.compareChannels) {
        await batch.compare(
          values,
          conversation.messages,
          comparisonChannelIds,
          controller.signal
        )
        return
      }
      batch.hide()
      return await conversation.submit(values, controller)
    } finally {
      abortControllerRef.current = null
    }
  }

  function handleClearSession() {
    conversation.clear()
    batch.clear()
    setClearDialogOpen(false)
    toast.success(t('modelTester.toast.cleared'))
  }

  const { messages } = conversation
  const { results: comparison, isComparing, rerunningChannelIds } = batch
  const isRunning =
    conversation.isPending || isComparing || rerunningChannelIds.size > 0

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
        defaultTargetFormat={defaultTargetFormat}
        defaultChannelId={defaultChannelId}
        onSubmit={handleSubmit}
        onStop={handleStop}
        onCompareChange={batch.onCompareChange}
      >
        {comparison || isComparing ? (
          <BatchResults
            results={comparison ?? []}
            channels={channelsQuery.data ?? []}
            isRunning={isComparing}
            requestedModel={batch.requestedModel}
            totalCount={batch.total}
            rerunningChannelIds={rerunningChannelIds}
            onRerunRow={batch.rerun}
            onDisableFailed={() => disableFailed.setOpen(true)}
            isDisablingFailed={disableFailed.isPending}
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
        open={disableFailed.open}
        title={t('modelTester.disableFailed.title')}
        description={t('modelTester.disableFailed.description', {
          count: disableFailed.count,
        })}
        confirmLabel={t('modelTester.disableFailed.confirm')}
        cancelLabel={t('modelTester.disableFailed.cancel')}
        destructive
        onConfirm={() => {
          void disableFailed.confirm()
        }}
        onCancel={() => disableFailed.setOpen(false)}
      />
    </div>
  )
}
