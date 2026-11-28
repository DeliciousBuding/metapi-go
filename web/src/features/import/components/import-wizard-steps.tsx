import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

import type { ImportSitesResult } from '../types'
import { ImportConnectStep } from './import-connect-step'
import { ImportIdentifyStep } from './import-identify-step'
import { ImportResultStep } from './import-result-step'
import { ImportRoutesStep } from './import-routes-step'
import { ImportStepper, type ImportStep } from './stepper'
import type { useImportDraft } from './use-import-draft'

export function ImportWizardSteps({
  draft,
  result,
  isSubmitting,
}: {
  draft: ReturnType<typeof useImportDraft>
  result: ImportSitesResult | null
  isSubmitting: boolean
}) {
  const {
    step,
    candidates,
    currentIndex,
    sourceText,
    setSourceText,
    updateCandidate,
    existingUrls,
    bindField,
  } = draft
  const { t } = useTranslation()
  const steps: ImportStep[] = useMemo(
    () => [
      { id: 'source', label: t('import.steps.source') },
      { id: 'identify', label: t('import.steps.identify') },
      { id: 'connect', label: t('import.steps.connect') },
      { id: 'routes', label: t('import.steps.routes') },
      { id: 'done', label: t('import.steps.done') },
    ],
    [t]
  )

  const { isDetecting } = draft

  return (
    <>
      <ImportStepper steps={steps} currentIndex={currentIndex} />

      <div className='mt-2 min-h-64' aria-busy={isSubmitting || isDetecting}>
        {step === 'source' && (
          <div className='grid gap-3'>
            <Label htmlFor='import-source-urls'>
              {t('import.source.label')}
            </Label>
            <Textarea
              id='import-source-urls'
              {...bindField('source', setSourceText)}
              rows={8}
              value={sourceText}
              placeholder={t('import.source.placeholder')}
              className='font-mono text-xs'
              autoFocus
            />
            <p className='text-muted-foreground text-xs'>
              {t('import.source.hint')}
            </p>
          </div>
        )}

        {step === 'identify' && (
          <ImportIdentifyStep
            candidates={candidates}
            updateCandidate={updateCandidate}
            existingUrls={existingUrls}
            bindField={bindField}
          />
        )}
        {step === 'connect' && (
          <ImportConnectStep
            candidates={candidates}
            updateCandidate={updateCandidate}
          />
        )}
        {step === 'routes' && (
          <ImportRoutesStep
            candidates={candidates}
            updateCandidate={updateCandidate}
            bindField={bindField}
          />
        )}
        {step === 'done' && result && <ImportResultStep result={result} />}
      </div>
    </>
  )
}
