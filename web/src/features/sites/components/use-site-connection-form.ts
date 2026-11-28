import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import type { ConnectionTemplate } from '@/lib/platform-catalog'
import { toast } from '@/lib/toast'

import { useDetectSite } from '../api'
import {
  SITE_FORM_DEFAULT_VALUES,
  siteFormSchema,
  type SiteFormValues,
} from '../lib/sites-schema'
import type { Site } from '../types'
import { matchesTemplate, siteToFormValues } from './site-form-values'

export function useSiteConnectionForm(open: boolean, editingSite: Site | null) {
  const { t } = useTranslation()
  const isEditing = editingSite !== null

  const form = useForm<SiteFormValues>({
    resolver: zodResolver(siteFormSchema),
    defaultValues: SITE_FORM_DEFAULT_VALUES,
  })
  const detectSite = useDetectSite()
  const detectSiteAsync = detectSite.mutateAsync
  const [platformMode, setPlatformMode] = useState<'select' | 'custom'>(
    'select'
  )
  const [selectedTemplate, setSelectedTemplate] =
    useState<ConnectionTemplate | null>(null)
  const [templateFeedback, setTemplateFeedback] = useState('')
  const templateValues = useRef<
    Partial<Pick<SiteFormValues, 'name' | 'url' | 'platform'>>
  >({})

  useEffect(() => {
    if (!open) return
    if (editingSite) {
      form.reset(siteToFormValues(editingSite))
    } else {
      form.reset(SITE_FORM_DEFAULT_VALUES)
    }
    setPlatformMode('select')
    setSelectedTemplate(null)
    setTemplateFeedback('')
    templateValues.current = {}
  }, [open, editingSite, form])

  const watchedUrl = form.watch('url')
  const watchedPlatform = form.watch('platform')

  useEffect(() => {
    if (
      selectedTemplate?.group === 'api' &&
      !matchesTemplate(selectedTemplate, watchedUrl, watchedPlatform)
    ) {
      setSelectedTemplate(null)
      setTemplateFeedback(t('sites.templates.customized'))
    }
  }, [selectedTemplate, watchedUrl, watchedPlatform, t])

  // Auto-recognize platform when a URL is pasted and no platform has been
  // chosen yet. Unknown sites resolve to an empty result and stay manually
  // specifiable; a user-entered platform always wins over auto-detection.
  useEffect(() => {
    const url = watchedUrl.trim()
    if (!url || isEditing || watchedPlatform.trim() !== '') return

    const timer = window.setTimeout(() => {
      void (async () => {
        try {
          const detected = await detectSiteAsync(url)
          if (detected.platform && !form.getValues('platform').trim()) {
            form.setValue('platform', detected.platform, { shouldDirty: true })
          }
        } catch {
          // Unknown site: leave the platform empty for manual entry.
        }
      })()
    }, 600)

    return () => window.clearTimeout(timer)
  }, [watchedUrl, watchedPlatform, isEditing, detectSiteAsync, form])

  function handlePreset(preset: ConnectionTemplate) {
    const platform = form.getValues('platform').trim()
    if (
      platform &&
      platform !== preset.platform &&
      platform !== templateValues.current.platform
    ) {
      setTemplateFeedback(t('sites.templates.platformConflict'))
      return false
    }
    const nextValues: typeof templateValues.current = {}
    const preserved: string[] = []
    for (const [key, value] of [
      ['name', preset.name],
      ['url', preset.url],
      ['platform', preset.platform],
    ] as const) {
      const current = form.getValues(key)
      if (!current.trim() || current === templateValues.current[key]) {
        form.setValue(key, value, { shouldDirty: true })
        nextValues[key] = value
      } else {
        preserved.push(t(`sites.form.${key}`))
      }
    }
    templateValues.current = nextValues
    setSelectedTemplate(preset)
    setTemplateFeedback(
      preserved.length
        ? t('sites.templates.preserved', { fields: preserved.join(', ') })
        : ''
    )
    return true
  }

  async function handleDetect() {
    const url = watchedUrl.trim()
    if (!url) {
      // Surface the missing URL next to the field instead of a detached
      // toast — the operator needs to know WHICH input to fix.
      form.setError('url', { message: t('sites.form.detectRequiresUrl') })
      return
    }
    try {
      const detected = await detectSite.mutateAsync(url)
      if (detected.platform) {
        form.setValue('platform', detected.platform, { shouldDirty: true })
      }
      if (detected.externalCheckinUrl) {
        form.setValue('externalCheckinUrl', detected.externalCheckinUrl, {
          shouldDirty: true,
        })
      }
      toast.success(t('sites.form.detectSucceeded'))
    } catch {
      // http-client toasted
    }
  }
  return {
    form,
    selectedTemplate,
    templateFeedback,
    watchedUrl,
    watchedPlatform,
    handlePreset,
    platformMode,
    setPlatformMode,
    handleDetect,
    detecting: detectSite.isPending,
  }
}
