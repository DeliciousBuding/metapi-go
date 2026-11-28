import { useEffect, useRef } from 'react'
import type { UseFormReturn } from 'react-hook-form'

import { useChannels } from '@/features/channels'
import { useModels } from '@/features/models'

import type { TesterFormValues } from '../lib/tester-schema'
import type { TestTargetFormat } from '../types'

const LAST_MODEL_KEY = 'metapi-model-tester-last-model'

function readLastModel(): string {
  try {
    return window.localStorage.getItem(LAST_MODEL_KEY)?.trim() ?? ''
  } catch {
    return '' // Storage may be disabled by the browser.
  }
}

export function rememberModel(model: string): void {
  try {
    window.localStorage.setItem(LAST_MODEL_KEY, model)
  } catch {
    // The live selection still works when local persistence is unavailable.
  }
}

export function useTestCatalog(
  form: UseFormReturn<TesterFormValues>,
  {
    defaultModel,
    defaultTargetFormat,
    defaultChannelId,
    onCompareChange,
  }: {
    defaultModel?: string
    defaultTargetFormat?: TestTargetFormat
    defaultChannelId?: number
    onCompareChange?: (enabled: boolean) => void
  }
) {
  const modelsQuery = useModels()
  const channelsQuery = useChannels()
  const appliedDeepLink = useRef<string | null>(null)
  const appliedTargetFormat = useRef<TestTargetFormat | null>(null)
  const appliedChannelId = useRef<number | null>(null)

  useEffect(() => {
    if (!defaultTargetFormat) {
      appliedTargetFormat.current = null
      return
    }
    if (appliedTargetFormat.current !== defaultTargetFormat) {
      form.setValue('targetFormat', defaultTargetFormat)
      appliedTargetFormat.current = defaultTargetFormat
    }
  }, [defaultTargetFormat, form])

  useEffect(() => {
    if (!defaultChannelId) {
      appliedChannelId.current = null
      return
    }
    const channels = channelsQuery.data ?? []
    if (channels.length === 0) return
    if (
      appliedChannelId.current !== defaultChannelId &&
      channels.some((channel) => channel.id === defaultChannelId)
    ) {
      form.setValue('channelId', defaultChannelId)
      if (form.getValues('compareChannels')) {
        form.setValue('compareChannels', false)
        onCompareChange?.(false)
      }
      appliedChannelId.current = defaultChannelId
    }
  }, [defaultChannelId, channelsQuery.data, form, onCompareChange])

  // A deep link wins over local preference; once applied, a background model
  // refetch must not reset a model the user chose afterward. The saved model
  // is only restored while the field is empty and remains in the live catalog.
  useEffect(() => {
    const models = modelsQuery.data ?? []
    if (models.length === 0) return
    if (defaultModel) {
      if (
        appliedDeepLink.current !== defaultModel &&
        models.some((model) => model.name === defaultModel)
      ) {
        form.setValue('model', defaultModel, { shouldDirty: true })
        appliedDeepLink.current = defaultModel
      }
      return
    }
    appliedDeepLink.current = null
    if (form.getValues('model')) return
    const saved = readLastModel()
    if (saved && models.some((model) => model.name === saved)) {
      form.setValue('model', saved)
    }
  }, [defaultModel, modelsQuery.data, form])

  return { modelsQuery, channelsQuery }
}
