import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { useSites, type Site } from '@/features/sites'

import { useDetectSite } from '../api'
import { canonicalizeUrl, parseUrlLines } from '../lib/utils'
import type { ImportCandidate, ImportStepId } from '../types'
import { makeCandidate, toImportSiteItem } from './import-candidate'
import { useImportFields } from './use-import-fields'

const STEP_ORDER: ImportStepId[] = [
  'source',
  'identify',
  'connect',
  'routes',
  'done',
]

export function useImportDraft() {
  const { t } = useTranslation()
  const sitesQuery = useSites()
  const detectSite = useDetectSite()
  const [step, setStep] = useState<ImportStepId>('source')
  const [sourceText, setSourceText] = useState('')
  const [candidates, setCandidates] = useState<ImportCandidate[]>([])
  const { bindField, clearInvalid, markInvalidAndFocusFirst } =
    useImportFields()
  const existingUrls = useMemo(() => {
    const urls = new Set<string>()
    for (const site of (sitesQuery.data ?? []) as Site[]) {
      const canonical = canonicalizeUrl(site.url)
      if (canonical) urls.add(canonical)
    }
    return urls
  }, [sitesQuery.data])

  const currentIndex = STEP_ORDER.indexOf(step)
  function reset() {
    setStep('source')
    setSourceText('')
    setCandidates([])
    clearInvalid()
  }
  function updateCandidate(id: string, patch: Partial<ImportCandidate>) {
    setCandidates((prev) =>
      prev.map((candidate) =>
        candidate.id === id ? { ...candidate, ...patch } : candidate
      )
    )
  }

  async function detectAll(items: ImportCandidate[]) {
    await Promise.all(
      items.map(async (item) => {
        if (item.platform !== '' || item.detecting) return
        updateCandidate(item.id, { detecting: true })
        try {
          const detected = await detectSite.mutateAsync(item.url)
          updateCandidate(item.id, {
            platform: detected.platform ?? '',
            confidence: detected.confidence ?? null,
            detected: detected.platform != null,
            detecting: false,
          })
        } catch {
          updateCandidate(item.id, { detecting: false })
        }
      })
    )
  }

  function handleSourceNext() {
    const urls = parseUrlLines(sourceText)
    if (urls.length === 0) {
      toast.error(t('import.source.empty'))
      markInvalidAndFocusFirst(['source'])
      return
    }
    clearInvalid()
    const initial = urls.map((url, index) => makeCandidate(url, index))
    setCandidates(initial)
    setStep('identify')
    void detectAll(initial)
  }

  function handleIdentifyNext() {
    const missing = candidates.filter(
      (candidate) => candidate.platform.trim() === ''
    )
    if (missing.length > 0) {
      toast.error(t('import.identify.missingPlatform'))
      markInvalidAndFocusFirst(
        missing.map((candidate) => `${candidate.id}-platform`)
      )
      return
    }
    clearInvalid()
    setStep('connect')
  }

  function validatedItems() {
    const invalidWeightKeys: string[] = []
    for (const candidate of candidates) {
      if (!Number.isFinite(candidate.weight) || candidate.weight < 0) {
        invalidWeightKeys.push(`${candidate.id}-weight`)
      }
    }
    if (invalidWeightKeys.length > 0) {
      toast.error(t('import.routes.invalidWeight'))
      markInvalidAndFocusFirst(invalidWeightKeys)
      return null
    }
    clearInvalid()
    return candidates.map(toImportSiteItem)
  }
  return {
    step,
    setStep,
    sourceText,
    setSourceText,
    candidates,
    existingUrls,
    currentIndex,
    bindField,
    updateCandidate,
    handleSourceNext,
    handleIdentifyNext,
    reset,
    validatedItems,
    isDetecting: candidates.some((candidate) => candidate.detecting),
    goBack: () => setStep(STEP_ORDER[Math.max(0, currentIndex - 1)]),
  }
}
