import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import type { ImportSitesResult, ImportStepId } from '../types'

export function useImportAnnouncement(
  step: ImportStepId,
  count: number,
  isDetecting: boolean,
  result: ImportSitesResult | null
) {
  const { t } = useTranslation()
  // Polite live region: announce detection completion (only on the true→false
  // transition so the initial pre-detect render never fires a false "done")
  // and the final imported/skipped/failed summary once results land.
  const [liveMessage, setLiveMessage] = useState('')
  const wasDetectingRef = useRef(false)
  useEffect(() => {
    if (step === 'done' && result) {
      setLiveMessage(
        t('import.done.announce', {
          imported: result.imported,
          skipped: result.skipped,
          failed: result.failed,
        })
      )
      return
    }
    if (step === 'identify' && wasDetectingRef.current && !isDetecting) {
      setLiveMessage(t('import.identify.detectionComplete', { count }))
    }
    wasDetectingRef.current = isDetecting
  }, [step, result, count, isDetecting, t])
  return liveMessage
}
